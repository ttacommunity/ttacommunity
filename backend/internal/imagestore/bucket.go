package imagestore

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"ttacommunity/backend/internal/sigv4"
)

var (
	// ErrExists means an image is already stored under the key.
	ErrExists = errors.New("imagestore: the image is already stored")
	// ErrBadKey means a key is not one Check would make.
	ErrBadKey = errors.New("imagestore: not an image key")
)

// bucketName is S3's rule for bucket names, which R2 shares.
var bucketName = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

// Config is one bucket: the S3 endpoint (R2's is https://<account>.r2.cloudflarestorage.com), the bucket's name,
// the public address its objects are served from, and a key pair allowed to read and write it.
type Config struct {
	Endpoint, Bucket, PublicURL string
	Credentials                 sigv4.Credentials
}

// Object is a stored image and its public address.
type Object struct {
	Key, URL string
	Size     int64
	Uploaded time.Time
}

// Bucket stores images in one bucket through the S3 API.
type Bucket struct {
	cfg  Config
	http *http.Client
	now  func() time.Time
}

// New returns a Bucket for cfg, calling it through hc and signing with now's time. It refuses a config missing
// a field, a bucket name S3 would refuse, or an address that is not https (an endpoint may be a stub on 127.0.0.1).
func New(cfg Config, hc *http.Client, now func() time.Time) (*Bucket, error) {
	switch {
	case cfg.Credentials.AccessKeyID == "" || cfg.Credentials.SecretAccessKey == "":
		return nil, errors.New("imagestore: the access key id and secret are needed")
	case !bucketName.MatchString(cfg.Bucket):
		return nil, fmt.Errorf("imagestore: %q is not a bucket name", cfg.Bucket)
	case !strings.HasPrefix(cfg.Endpoint, "https://") && !strings.HasPrefix(cfg.Endpoint, "http://127.0.0.1:"):
		return nil, errors.New("imagestore: the endpoint must be https, or a stub on 127.0.0.1")
	case !strings.HasPrefix(cfg.PublicURL, "https://"):
		return nil, errors.New("imagestore: the public url must be https")
	}
	cfg.Endpoint = strings.TrimRight(cfg.Endpoint, "/")
	cfg.PublicURL = strings.TrimRight(cfg.PublicURL, "/")
	return &Bucket{cfg: cfg, http: hc, now: now}, nil
}

// Put stores img for good and returns its public address, or ErrExists when it is already stored.
func (b *Bucket) Put(ctx context.Context, img Image) (string, error) {
	if !ValidKey(img.Key) {
		return "", ErrBadKey
	}
	head, err := b.do(ctx, http.MethodHead, "/"+img.Key, nil, nil, nil)
	if err != nil {
		return "", err
	}
	switch head.StatusCode {
	case http.StatusOK:
		return "", ErrExists
	case http.StatusNotFound:
	default:
		return "", fmt.Errorf("imagestore: checking %s: status %d", img.Key, head.StatusCode)
	}
	resp, err := b.do(ctx, http.MethodPut, "/"+img.Key, nil, img.Body, http.Header{
		"Content-Type":  {img.ContentType},
		"Cache-Control": {"public, max-age=31536000, immutable"},
	})
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("imagestore: storing %s: status %d", img.Key, resp.StatusCode)
	}
	return b.cfg.PublicURL + "/" + img.Key, nil
}

// listResult is the part of ListObjectsV2's answer we read.
type listResult struct {
	Contents []struct {
		Key          string
		Size         int64
		LastModified time.Time
	}
	IsTruncated           bool
	NextContinuationToken string
}

// List returns every image in the bucket, newest first.
func (b *Bucket) List(ctx context.Context) ([]Object, error) {
	var objects []Object
	token := ""
	for {
		q := url.Values{"list-type": {"2"}}
		if token != "" {
			q.Set("continuation-token", token)
		}
		resp, err := b.do(ctx, http.MethodGet, "", q, nil, nil)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("imagestore: listing %s: status %d", b.cfg.Bucket, resp.StatusCode)
		}
		var page listResult
		if err := xml.Unmarshal(resp.body, &page); err != nil {
			return nil, fmt.Errorf("imagestore: reading the list of %s: %w", b.cfg.Bucket, err)
		}
		for _, c := range page.Contents {
			objects = append(objects, Object{Key: c.Key, URL: b.cfg.PublicURL + "/" + c.Key, Size: c.Size, Uploaded: c.LastModified.UTC()})
		}
		if !page.IsTruncated || page.NextContinuationToken == "" {
			break
		}
		token = page.NextContinuationToken
	}
	sort.SliceStable(objects, func(i, j int) bool { return objects[i].Uploaded.After(objects[j].Uploaded) })
	return objects, nil
}

// Delete removes the image stored under key; removing one that is not there is not an error. It refuses a key
// Check would not make with ErrBadKey.
func (b *Bucket) Delete(ctx context.Context, key string) error {
	if !ValidKey(key) {
		return ErrBadKey
	}
	resp, err := b.do(ctx, http.MethodDelete, "/"+key, nil, nil, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("imagestore: deleting %s: status %d", key, resp.StatusCode)
	}
	return nil
}

// response is a status and a body read in full.
type response struct {
	StatusCode int
	body       []byte
}

// maxResponse caps what is read from the store: a list page of 1,000 keys is well under it.
const maxResponse = 4 << 20

// do sends a signed request for path within the bucket.
func (b *Bucket) do(ctx context.Context, method, path string, q url.Values, body []byte, h http.Header) (response, error) {
	u := b.cfg.Endpoint + "/" + b.cfg.Bucket + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return response{}, fmt.Errorf("imagestore: building a request: %w", err)
	}
	for name, vs := range h {
		req.Header[name] = vs
	}
	hash := sigv4.PayloadHash(body)
	req.Header.Set("X-Amz-Content-Sha256", hash)
	sigv4.Sign(req, b.cfg.Credentials, "auto", "s3", hash, b.now())
	resp, err := b.http.Do(req)
	if err != nil {
		return response{}, fmt.Errorf("imagestore: %s %s: %w", method, b.cfg.Bucket, err)
	}
	defer resp.Body.Close()
	read, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return response{}, fmt.Errorf("imagestore: reading %s %s: %w", method, b.cfg.Bucket, err)
	}
	return response{StatusCode: resp.StatusCode, body: read}, nil
}
