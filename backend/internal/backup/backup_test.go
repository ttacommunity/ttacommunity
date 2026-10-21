package backup_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"filippo.io/age"

	"ttacommunity/backend/internal/backup"
)

// settings returns a complete environment for ConfigFrom, encrypting to identity's recipient.
func settings(identity *age.X25519Identity, endpoint string) map[string]string {
	return map[string]string{
		"BACKUP_AGE_RECIPIENT":     identity.Recipient().String(),
		"BACKUP_ENDPOINT":          endpoint,
		"BACKUP_BUCKET":            "ttac-backups",
		"BACKUP_ACCESS_KEY_ID":     "key-id-1",
		"BACKUP_SECRET_ACCESS_KEY": "secret-key-1",
	}
}

func newIdentity(t *testing.T) *age.X25519Identity {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestConfigFromRefusesAMissingOrUnsafeSetting(t *testing.T) {
	private := newIdentity(t).String()
	tests := []struct {
		name, key, value string
	}{
		{"no recipient", "BACKUP_AGE_RECIPIENT", ""},
		{"no endpoint", "BACKUP_ENDPOINT", ""},
		{"no bucket", "BACKUP_BUCKET", ""},
		{"no key id", "BACKUP_ACCESS_KEY_ID", ""},
		{"no secret key", "BACKUP_SECRET_ACCESS_KEY", ""},
		{"an endpoint without TLS", "BACKUP_ENDPOINT", "http://example.test"},
		{"an endpoint that is not a URL", "BACKUP_ENDPOINT", "example.test"},
		{"a recipient that is not an age key", "BACKUP_AGE_RECIPIENT", "age1notakey"},
		{"a private key as the recipient", "BACKUP_AGE_RECIPIENT", private},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := settings(newIdentity(t), "https://example.test")
			env[tt.key] = tt.value
			if _, err := backup.ConfigFrom(func(k string) string { return env[k] }); err == nil {
				t.Errorf("ConfigFrom with %s = %q: no error, want a refusal", tt.key, tt.value)
			}
		})
	}
}

func TestConfigFromReadsEverySetting(t *testing.T) {
	id := newIdentity(t)
	env := settings(id, "https://example.test")
	cfg, err := backup.ConfigFrom(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("ConfigFrom: %v", err)
	}
	got := []string{cfg.Recipient.String(), cfg.Endpoint, cfg.Bucket, cfg.AccessKeyID, string(cfg.SecretAccessKey)}
	want := []string{id.Recipient().String(), "https://example.test", "ttac-backups", "key-id-1", "secret-key-1"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("config = %q, want %q", got, want)
	}
}

func TestConfigNeverPrintsTheSecretKey(t *testing.T) {
	env := settings(newIdentity(t), "https://example.test")
	cfg, err := backup.ConfigFrom(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("ConfigFrom: %v", err)
	}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		if s := fmt.Sprintf(verb, cfg); strings.Contains(s, "secret-key-1") {
			t.Errorf("Sprintf(%q, cfg) = %s, holds the secret key", verb, s)
		}
	}
}

// bucket is a stand-in for R2 that keeps the last request it was sent and answers with status.
type bucket struct {
	mu     sync.Mutex
	status int
	method string
	path   string
	body   []byte
	calls  int
}

func (b *bucket) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.method, b.path, b.body = r.Method, r.URL.Path, body
	b.calls++
	if b.status != http.StatusOK {
		w.WriteHeader(b.status)
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>Access Denied</Message></Error>`)
	}
}

// put runs backup.Put against a stand-in bucket answering with status, writing with write into a new folder.
func put(t *testing.T, status int, write func(io.Writer) error) (*bucket, *age.X25519Identity, string, int64, error) {
	t.Helper()
	b := &bucket{status: status}
	srv := httptest.NewTLSServer(b)
	t.Cleanup(srv.Close)
	id := newIdentity(t)
	env := settings(id, srv.URL)
	cfg, err := backup.ConfigFrom(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("ConfigFrom: %v", err)
	}
	dir := t.TempDir()
	n, err := backup.Put(context.Background(), backup.NewClient(cfg, srv.Client()), cfg, dir, "ratings/x.zip.age", write)
	return b, id, dir, n, err
}

func leftIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestPutUploadsTheBackupEncryptedToTheRecipient(t *testing.T) {
	b, id, dir, n, err := put(t, http.StatusOK, func(w io.Writer) error {
		_, err := io.WriteString(w, "a backup")
		return err
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if b.method != http.MethodPut || b.path != "/ttac-backups/ratings/x.zip.age" {
		t.Errorf("request = %s %s, want PUT /ttac-backups/ratings/x.zip.age", b.method, b.path)
	}
	if n != int64(len(b.body)) {
		t.Errorf("Put returned %d bytes, the bucket got %d", n, len(b.body))
	}
	r, err := age.Decrypt(bytes.NewReader(b.body), id)
	if err != nil {
		t.Fatalf("decrypting what the bucket got: %v", err)
	}
	if got, _ := io.ReadAll(r); string(got) != "a backup" {
		t.Errorf("decrypted upload = %q, want %q", got, "a backup")
	}
	if left := leftIn(t, dir); len(left) != 0 {
		t.Errorf("Put left %q in its folder", left)
	}
}

func TestPutReportsARefusedUpload(t *testing.T) {
	_, _, dir, _, err := put(t, http.StatusForbidden, func(w io.Writer) error {
		_, err := io.WriteString(w, "a backup")
		return err
	})
	if err == nil {
		t.Error("Put to a bucket that refuses it: no error")
	}
	if left := leftIn(t, dir); len(left) != 0 {
		t.Errorf("Put left %q in its folder", left)
	}
}

func TestPutUploadsNothingWhenTheBackupFails(t *testing.T) {
	failed := errors.New("the database went away")
	b, _, dir, _, err := put(t, http.StatusOK, func(w io.Writer) error {
		io.WriteString(w, "half a backup")
		return failed
	})
	if !errors.Is(err, failed) {
		t.Errorf("Put = %v, want the backup's error", err)
	}
	if b.calls != 0 {
		t.Errorf("the bucket got %d requests, want none", b.calls)
	}
	if left := leftIn(t, dir); len(left) != 0 {
		t.Errorf("Put left %q in its folder", left)
	}
}

func TestKeyNamesTheBackupByItsTimeInUTC(t *testing.T) {
	sydney, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"in UTC", time.Date(2026, 10, 21, 3, 0, 5, 0, time.UTC), "ratings/2026-10-21T030005Z.zip.age"},
		{"in another zone", time.Date(2026, 10, 21, 14, 0, 5, 0, sydney), "ratings/2026-10-21T030005Z.zip.age"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := backup.Key(tt.at); got != tt.want {
				t.Errorf("Key(%v) = %q, want %q", tt.at, got, tt.want)
			}
		})
	}
}
