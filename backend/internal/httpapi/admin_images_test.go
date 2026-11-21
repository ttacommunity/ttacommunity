package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/imagestore"
)

// pngHead is enough of a PNG for it to be recognised by its content.
const pngHead = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"

// fakeImages is one bucket in memory, served from https://assets.example.test. putErr, when set, is what Put
// answers; writes counts the puts and deletes that reached it.
type fakeImages struct {
	objects []imagestore.Object
	putErr  error
	listErr error
	writes  int
}

func (f *fakeImages) Put(_ context.Context, img imagestore.Image) (string, error) {
	f.writes++
	if f.putErr != nil {
		return "", f.putErr
	}
	for _, o := range f.objects {
		if o.Key == img.Key {
			return "", imagestore.ErrExists
		}
	}
	url := "https://assets.example.test/" + img.Key
	f.objects = append([]imagestore.Object{{Key: img.Key, URL: url, Size: int64(len(img.Body)), Uploaded: time.Date(2026, 11, 21, 9, 30, 0, 0, time.UTC)}}, f.objects...)
	return url, nil
}

func (f *fakeImages) List(context.Context) ([]imagestore.Object, error) {
	return f.objects, f.listErr
}

func (f *fakeImages) Delete(_ context.Context, key string) error {
	if !imagestore.ValidKey(key) {
		return imagestore.ErrBadKey
	}
	f.writes++
	f.objects = slices.DeleteFunc(f.objects, func(o imagestore.Object) bool { return o.Key == key })
	return nil
}

// imagesDo sends an admin request as who, with images set up as the assets bucket when it is not nil.
func imagesDo(t *testing.T, a *fakeAdmins, images *fakeImages, who, method, path, origin string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Cf-Access-Jwt-Assertion", "good-"+who)
	req.Header.Set("Content-Type", "application/octet-stream")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	imagesMux(a, images).ServeHTTP(rec, req)
	return rec
}

func imagesMux(a Admins, images *fakeImages) http.Handler {
	var buckets map[ImageBucket]ImageStore
	if images != nil {
		buckets = map[ImageBucket]ImageStore{Assets: images}
	}
	return NewMux(&fakeRatings{}, a, nil, nil, nil, nil, nil, buckets, slog.New(slog.DiscardHandler), &recordedRequests{}, stepClock(time.Millisecond))
}

func TestAdminsAndOrganisersUploadListAndDeleteImages(t *testing.T) {
	const site = "https://ttac-admin.example"
	for _, who := range []string{"111111111111111111", "222222222222222222"} {
		t.Run(who, func(t *testing.T) {
			a, images := &fakeAdmins{}, &fakeImages{}
			body := []byte(pngHead + strings.Repeat("x", 2<<20))
			rec := imagesDo(t, a, images, who, http.MethodPost, "/v1/admin/images/assets?name=Blitz%20Bolts.png", site, body)
			var made StoredImage
			if err := json.Unmarshal(rec.Body.Bytes(), &made); err != nil || rec.Code != http.StatusCreated {
				t.Fatalf("POST image = %d %s, want 201", rec.Code, rec.Body)
			}
			if !strings.HasPrefix(made.Key, "blitz-bolts-") || made.Url != "https://assets.example.test/"+made.Key || made.Size != int64(len(body)) {
				t.Errorf("stored = %+v, want key blitz-bolts-…, its url and %d bytes", made, len(body))
			}

			rec = imagesDo(t, a, images, who, http.MethodGet, "/v1/admin/images/assets", "", nil)
			var list []StoredImage
			if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 || list[0].Key != made.Key {
				t.Fatalf("GET images = %d %s, want the one stored", rec.Code, rec.Body)
			}

			if rec := imagesDo(t, a, images, who, http.MethodDelete, "/v1/admin/images/assets/"+made.Key, site, nil); rec.Code != http.StatusNoContent {
				t.Fatalf("DELETE image = %d %s, want 204", rec.Code, rec.Body)
			}
			if len(images.objects) != 0 {
				t.Errorf("objects left = %d, want 0", len(images.objects))
			}
			want := []string{who + " image.upload assets/" + made.Key, who + " image.delete assets/" + made.Key}
			if !slices.Equal(a.audits, want) {
				t.Errorf("audits = %q, want %q", a.audits, want)
			}
		})
	}
}

func TestImageChangesAreRefusedWithoutReachingTheBucket(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	image := []byte(pngHead)
	tests := []struct {
		name, method, path, origin string
		body                       []byte
		notSetUp                   bool
		want                       int
	}{
		{"upload from another site", http.MethodPost, "/v1/admin/images/assets?name=a.png", "https://evil.example", image, false, http.StatusForbidden},
		{"upload without an Origin", http.MethodPost, "/v1/admin/images/assets?name=a.png", "", image, false, http.StatusForbidden},
		{"delete from another site", http.MethodDelete, "/v1/admin/images/assets/a-0123456789ab.png", "https://evil.example", nil, false, http.StatusForbidden},
		{"upload without a name", http.MethodPost, "/v1/admin/images/assets", site, image, false, http.StatusBadRequest},
		{"upload of text", http.MethodPost, "/v1/admin/images/assets?name=a.png", site, []byte("hello"), false, http.StatusBadRequest},
		{"upload of an svg", http.MethodPost, "/v1/admin/images/assets?name=a.svg", site, []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), false, http.StatusBadRequest},
		{"upload over 5 MB", http.MethodPost, "/v1/admin/images/assets?name=a.png", site, []byte(pngHead + strings.Repeat("x", imagestore.MaxSize)), false, http.StatusRequestEntityTooLarge},
		{"upload far over 5 MB", http.MethodPost, "/v1/admin/images/assets?name=a.png", site, []byte(pngHead + strings.Repeat("x", 3*imagestore.MaxSize)), false, http.StatusRequestEntityTooLarge},
		{"upload to an unknown bucket", http.MethodPost, "/v1/admin/images/other?name=a.png", site, image, false, http.StatusNotFound},
		{"upload to a bucket not set up", http.MethodPost, "/v1/admin/images/assets?name=a.png", site, image, true, http.StatusServiceUnavailable},
		{"list of an unknown bucket", http.MethodGet, "/v1/admin/images/other", "", nil, false, http.StatusNotFound},
		{"list of a bucket not set up", http.MethodGet, "/v1/admin/images/assets", "", nil, true, http.StatusServiceUnavailable},
		{"delete of a key not an image's", http.MethodDelete, "/v1/admin/images/assets/notes.txt", site, nil, false, http.StatusBadRequest},
		{"delete from an unknown bucket", http.MethodDelete, "/v1/admin/images/other/a-0123456789ab.png", site, nil, false, http.StatusNotFound},
		{"delete from a bucket not set up", http.MethodDelete, "/v1/admin/images/assets/a-0123456789ab.png", site, nil, true, http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, images := &fakeAdmins{}, &fakeImages{}
			bucket := images
			if tt.notSetUp {
				bucket = nil
			}
			rec := imagesDo(t, a, bucket, owner, tt.method, tt.path, tt.origin, tt.body)
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if images.writes != 0 || len(a.audits) != 0 {
				t.Errorf("writes = %d, audits = %q, want none", images.writes, a.audits)
			}
		})
	}
}

func TestUploadingAnImageStoredAlreadyIsAConflict(t *testing.T) {
	a, images := &fakeAdmins{}, &fakeImages{}
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	path := "/v1/admin/images/assets?name=a.png"
	if rec := imagesDo(t, a, images, owner, http.MethodPost, path, site, []byte(pngHead)); rec.Code != http.StatusCreated {
		t.Fatalf("first upload = %d %s, want 201", rec.Code, rec.Body)
	}
	if rec := imagesDo(t, a, images, owner, http.MethodPost, path, site, []byte(pngHead)); rec.Code != http.StatusConflict {
		t.Errorf("second upload = %d %s, want 409", rec.Code, rec.Body)
	}
}

func TestABucketFailureIsAServerErrorThatHidesIt(t *testing.T) {
	a := &fakeAdmins{}
	images := &fakeImages{putErr: errors.New("imagestore: PUT secret-bucket: dial tcp 10.0.0.1:443")}
	rec := imagesDo(t, a, images, "111111111111111111", http.MethodPost, "/v1/admin/images/assets?name=a.png", "https://ttac-admin.example", []byte(pngHead))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret-bucket") {
		t.Errorf("POST with a failing bucket = %d %s, want a 500 that does not repeat the error", rec.Code, rec.Body)
	}
}

func TestTheAssetsAreListedForEveryone(t *testing.T) {
	const stored = "bolts-0123456789ab.png"
	images := &fakeImages{objects: []imagestore.Object{{Key: stored, URL: "https://assets.example.test/" + stored, Size: 9, Uploaded: time.Date(2026, 11, 21, 9, 30, 0, 0, time.UTC)}}}
	rec := httptest.NewRecorder()
	imagesMux(nil, images).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/assets", nil))
	var list []StoredImage
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || rec.Code != http.StatusOK || len(list) != 1 || list[0].Url != images.objects[0].URL {
		t.Fatalf("GET /v1/assets = %d %s, want the one image", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheCurrent {
		t.Errorf("Cache-Control = %q, want %q", got, cacheCurrent)
	}
}

func TestTheAssetsListAnswersWhenItCannot(t *testing.T) {
	tests := []struct {
		name   string
		images *fakeImages
		want   int
	}{
		{"no assets bucket here", nil, http.StatusNotFound},
		{"the bucket cannot be listed", &fakeImages{listErr: errors.New("imagestore: listing: status 500")}, http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			imagesMux(nil, tt.images).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/assets", nil))
			if rec.Code != tt.want {
				t.Errorf("GET /v1/assets = %d %s, want %d", rec.Code, rec.Body, tt.want)
			}
			if body, _ := io.ReadAll(rec.Body); strings.Contains(string(body), "status 500") {
				t.Errorf("body %s repeats the bucket's error", body)
			}
		})
	}
}
