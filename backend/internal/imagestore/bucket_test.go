package imagestore_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ttacommunity/backend/internal/imagestore"
	"ttacommunity/backend/internal/sigv4"
)

var at = time.Date(2026, 11, 21, 9, 30, 0, 0, time.UTC)

// fakeS3 is one bucket of an S3 API: it refuses a request not signed for the test's key, and lists pageSize objects
// a page. It records each object's headers.
type fakeS3 struct {
	mu       sync.Mutex
	objects  map[string]http.Header
	order    []string
	pageSize int
	status   int // when set, every request gets it
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, _ := io.ReadAll(r.Body)
	sum := sha256.Sum256(body)
	if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=test-key/20261121/auto/s3/aws4_request, ") ||
		r.Header.Get("X-Amz-Content-Sha256") != hex.EncodeToString(sum[:]) || r.Header.Get("X-Amz-Date") != "20261121T093000Z" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	if r.URL.Path == "/assets" && r.Method == http.MethodGet {
		f.list(w, r)
		return
	}
	key, ok := strings.CutPrefix(r.URL.Path, "/assets/")
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_, exists := f.objects[key]
	switch r.Method {
	case http.MethodHead:
		if !exists {
			w.WriteHeader(http.StatusNotFound)
		}
	case http.MethodPut:
		h := r.Header.Clone()
		h.Set("Size", fmt.Sprint(len(body)))
		f.objects[key] = h
		f.order = append(f.order, key)
	case http.MethodDelete:
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (f *fakeS3) list(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("list-type") != "2" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var keys []string
	for _, k := range f.order {
		if _, ok := f.objects[k]; ok {
			keys = append(keys, k)
		}
	}
	start := 0
	if tok := r.URL.Query().Get("continuation-token"); tok != "" {
		_, _ = fmt.Sscan(tok, &start)
	}
	end := min(start+f.pageSize, len(keys))
	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
	for i, k := range keys[start:end] {
		fmt.Fprintf(w, "<Contents><Key>%s</Key><Size>%s</Size><LastModified>2026-11-%02dT10:00:00.000Z</LastModified></Contents>",
			k, f.objects[k].Get("Size"), start+i+1)
	}
	if end < len(keys) {
		fmt.Fprintf(w, "<IsTruncated>true</IsTruncated><NextContinuationToken>%d</NextContinuationToken>", end)
	} else {
		fmt.Fprint(w, "<IsTruncated>false</IsTruncated>")
	}
	fmt.Fprint(w, "</ListBucketResult>")
}

func newBucket(t *testing.T, f *fakeS3) *imagestore.Bucket {
	t.Helper()
	if f.objects == nil {
		f.objects = map[string]http.Header{}
	}
	if f.pageSize == 0 {
		f.pageSize = 1000
	}
	s := httptest.NewServer(f)
	t.Cleanup(s.Close)
	b, err := imagestore.New(imagestore.Config{
		Endpoint:    s.URL,
		Bucket:      "assets",
		PublicURL:   "https://assets.example.test",
		Credentials: sigv4.Credentials{AccessKeyID: "test-key", SecretAccessKey: "test-secret"},
	}, s.Client(), func() time.Time { return at })
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustCheck(t *testing.T, file string, body []byte) imagestore.Image {
	t.Helper()
	img, err := imagestore.Check(file, body)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestPutStoresAnImageUnderItsKeyForGood(t *testing.T) {
	f := &fakeS3{}
	b := newBucket(t, f)
	img := mustCheck(t, "bolts.png", png)
	url, err := b.Put(context.Background(), img)
	if err != nil {
		t.Fatalf("Put error = %v", err)
	}
	if want := "https://assets.example.test/" + img.Key; url != want {
		t.Errorf("Put url = %q, want %q", url, want)
	}
	h, ok := f.objects[img.Key]
	if !ok {
		t.Fatalf("no object stored under %q", img.Key)
	}
	for name, want := range map[string]string{"Content-Type": "image/png", "Cache-Control": "public, max-age=31536000, immutable"} {
		if got := h.Get(name); got != want {
			t.Errorf("stored %s = %q, want %q", name, got, want)
		}
	}
}

func TestPutRefusesAnImageAlreadyStored(t *testing.T) {
	f := &fakeS3{}
	b := newBucket(t, f)
	img := mustCheck(t, "bolts.png", png)
	if _, err := b.Put(context.Background(), img); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Put(context.Background(), img); !errors.Is(err, imagestore.ErrExists) {
		t.Errorf("second Put error = %v, want %v", err, imagestore.ErrExists)
	}
	if len(f.order) != 1 {
		t.Errorf("objects written = %d, want 1", len(f.order))
	}
}

func TestListReturnsEveryPageNewestFirst(t *testing.T) {
	f := &fakeS3{pageSize: 2}
	b := newBucket(t, f)
	var keys []string
	for i, body := range [][]byte{png, jpeg, gif, webp, padded(png, 64)} {
		img := mustCheck(t, fmt.Sprintf("img-%d", i), body)
		if _, err := b.Put(context.Background(), img); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, img.Key)
	}
	got, err := b.List(context.Background())
	if err != nil {
		t.Fatalf("List error = %v", err)
	}
	if len(got) != len(keys) {
		t.Fatalf("List returned %d objects, want %d", len(got), len(keys))
	}
	for i, o := range got {
		want := keys[len(keys)-1-i]
		if o.Key != want || o.URL != "https://assets.example.test/"+want {
			t.Errorf("List[%d] = %q at %q, want %q", i, o.Key, o.URL, want)
		}
	}
	if newest := got[0]; newest.Size != 64 || !newest.Uploaded.Equal(time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("newest = %d bytes at %v, want 64 bytes at 2026-11-05 10:00 UTC", newest.Size, newest.Uploaded)
	}
}

func TestDeleteRemovesAnImage(t *testing.T) {
	f := &fakeS3{}
	b := newBucket(t, f)
	img := mustCheck(t, "bolts.png", png)
	if _, err := b.Put(context.Background(), img); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.objects[img.Key]; !ok {
		t.Fatalf("object %q not stored before the delete", img.Key)
	}
	if err := b.Delete(context.Background(), img.Key); err != nil {
		t.Fatalf("Delete error = %v", err)
	}
	if _, ok := f.objects[img.Key]; ok {
		t.Errorf("object %q still stored", img.Key)
	}
}

func TestDeleteRefusesAKeyCheckWouldNotMake(t *testing.T) {
	f := &fakeS3{}
	b := newBucket(t, f)
	f.objects["notes.txt"] = http.Header{}
	for _, key := range []string{"notes.txt", "../other/x-0123456789ab.png", ""} {
		if err := b.Delete(context.Background(), key); !errors.Is(err, imagestore.ErrBadKey) {
			t.Errorf("Delete(%q) error = %v, want %v", key, err, imagestore.ErrBadKey)
		}
	}
	if _, ok := f.objects["notes.txt"]; !ok {
		t.Error("a refused key was deleted")
	}
}

func TestARefusedRequestIsAnError(t *testing.T) {
	ctx := context.Background()
	img := mustCheck(t, "bolts.png", png)
	calls := map[string]func(b *imagestore.Bucket) error{
		"put":    func(b *imagestore.Bucket) error { _, err := b.Put(ctx, img); return err },
		"list":   func(b *imagestore.Bucket) error { _, err := b.List(ctx); return err },
		"delete": func(b *imagestore.Bucket) error { return b.Delete(ctx, img.Key) },
	}
	for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError} {
		for name, call := range calls {
			t.Run(fmt.Sprintf("%s %d", name, status), func(t *testing.T) {
				b := newBucket(t, &fakeS3{status: status})
				if err := call(b); err == nil || errors.Is(err, imagestore.ErrExists) {
					t.Errorf("%s error = %v, want a failure", name, err)
				}
			})
		}
	}
}

func TestNewRefusesAnIncompleteOrUnsafeConfig(t *testing.T) {
	good := imagestore.Config{
		Endpoint:    "https://account.r2.cloudflarestorage.com",
		Bucket:      "assets",
		PublicURL:   "https://assets.example.test",
		Credentials: sigv4.Credentials{AccessKeyID: "test-key", SecretAccessKey: "test-secret"},
	}
	tests := []struct {
		name   string
		change func(c *imagestore.Config)
		ok     bool
	}{
		{"complete", func(c *imagestore.Config) {}, true},
		{"a local endpoint", func(c *imagestore.Config) { c.Endpoint = "http://127.0.0.1:9000" }, true},
		{"no endpoint", func(c *imagestore.Config) { c.Endpoint = "" }, false},
		{"a plain-http endpoint", func(c *imagestore.Config) { c.Endpoint = "http://account.r2.cloudflarestorage.com" }, false},
		{"no bucket", func(c *imagestore.Config) { c.Bucket = "" }, false},
		{"a bucket with a slash", func(c *imagestore.Config) { c.Bucket = "assets/x" }, false},
		{"no public url", func(c *imagestore.Config) { c.PublicURL = "" }, false},
		{"a plain-http public url", func(c *imagestore.Config) { c.PublicURL = "http://assets.example.test" }, false},
		{"no access key", func(c *imagestore.Config) { c.Credentials.AccessKeyID = "" }, false},
		{"no secret", func(c *imagestore.Config) { c.Credentials.SecretAccessKey = "" }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := good
			tt.change(&c)
			_, err := imagestore.New(c, http.DefaultClient, time.Now)
			if (err == nil) != tt.ok {
				t.Errorf("New(%s) error = %v, want ok = %v", tt.name, err, tt.ok)
			}
		})
	}
}

func TestConfigStringHidesTheSecret(t *testing.T) {
	c := imagestore.Config{Bucket: "assets", Credentials: sigv4.Credentials{AccessKeyID: "k", SecretAccessKey: "a-secret-value"}}
	if s := fmt.Sprint(c); strings.Contains(s, "a-secret-value") {
		t.Errorf("Config prints as %q, showing the secret", s)
	}
}
