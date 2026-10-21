// Package backup encrypts a backup of the ratings database with age and puts it in an S3-compatible bucket (R2).
package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"filippo.io/age"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Config says where a backup goes and who can read it.
type Config struct {
	Recipient       *age.X25519Recipient
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey Secret
}

// Secret is a credential that never prints.
type Secret string

func (Secret) String() string   { return "[redacted]" }
func (Secret) GoString() string { return "[redacted]" }

// ConfigFrom reads BACKUP_AGE_RECIPIENT, BACKUP_ENDPOINT, BACKUP_BUCKET, BACKUP_ACCESS_KEY_ID and
// BACKUP_SECRET_ACCESS_KEY with getenv. It refuses a missing setting, an endpoint that is not https, and a
// recipient that is not an age X25519 public key.
func ConfigFrom(getenv func(string) string) (Config, error) {
	var missing []error
	for _, k := range []string{"BACKUP_AGE_RECIPIENT", "BACKUP_ENDPOINT", "BACKUP_BUCKET", "BACKUP_ACCESS_KEY_ID", "BACKUP_SECRET_ACCESS_KEY"} {
		if getenv(k) == "" {
			missing = append(missing, fmt.Errorf("%s is not set", k))
		}
	}
	if err := errors.Join(missing...); err != nil {
		return Config{}, err
	}
	r, err := age.ParseX25519Recipient(getenv("BACKUP_AGE_RECIPIENT"))
	if err != nil {
		return Config{}, errors.New("BACKUP_AGE_RECIPIENT is not an age public key (age1…)")
	}
	endpoint := getenv("BACKUP_ENDPOINT")
	if u, err := url.Parse(endpoint); err != nil || u.Scheme != "https" || u.Host == "" {
		return Config{}, fmt.Errorf("BACKUP_ENDPOINT %q is not an https URL", endpoint)
	}
	return Config{
		Recipient:       r,
		Endpoint:        endpoint,
		Bucket:          getenv("BACKUP_BUCKET"),
		AccessKeyID:     getenv("BACKUP_ACCESS_KEY_ID"),
		SecretAccessKey: Secret(getenv("BACKUP_SECRET_ACCESS_KEY")),
	}, nil
}

// NewClient returns an S3 client for cfg's endpoint and key, sending its requests with hc.
func NewClient(cfg Config, hc *http.Client) *s3.Client {
	return s3.New(s3.Options{
		Region:       "auto",
		BaseEndpoint: aws.String(cfg.Endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, string(cfg.SecretAccessKey), ""),
		UsePathStyle: true,
		HTTPClient:   hc,
		// R2 does not take every checksum newer SDKs send by default.
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
}

// Put writes what write produces, encrypted to cfg's recipient, to a new file in dir, then puts that file in
// cfg's bucket as key and removes it. It returns the encrypted size. Nothing unencrypted reaches the disk.
func Put(ctx context.Context, client *s3.Client, cfg Config, dir, key string, write func(io.Writer) error) (int64, error) {
	f, err := os.CreateTemp(dir, "backup-*.age")
	if err != nil {
		return 0, fmt.Errorf("backing up: %w", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	enc, err := age.Encrypt(f, cfg.Recipient)
	if err != nil {
		return 0, fmt.Errorf("backing up: encrypting: %w", err)
	}
	if err := write(enc); err != nil {
		return 0, err
	}
	if err := enc.Close(); err != nil {
		return 0, fmt.Errorf("backing up: encrypting: %w", err)
	}
	size, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, fmt.Errorf("backing up: %w", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("backing up: %w", err)
	}
	if _, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(cfg.Bucket),
		Key:           aws.String(key),
		Body:          f,
		ContentLength: aws.Int64(size),
	}); err != nil {
		return 0, fmt.Errorf("backing up: uploading %s: %w", key, err)
	}
	return size, nil
}

// Key names the backup taken at at, in UTC: ratings/2026-10-21T030005Z.zip.age.
func Key(at time.Time) string {
	return "ratings/" + at.UTC().Format("2006-01-02T150405Z") + ".zip.age"
}
