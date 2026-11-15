package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/movies"
	"ttacommunity/backend/internal/store"
)

const (
	hubLinked   = "UC0000000000000000000001"
	hubUnlinked = "UC0000000000000000000002"
)

var hubSecret = []byte("invented-hub-secret")

// fakeHub links hubLinked, records what the callback asks of it, and fails a store with storeErr.
type fakeHub struct {
	confirmed []string
	stored    []string
	removed   []string
	storeErr  error
}

func (f *fakeHub) Secret() []byte { return hubSecret }

func (f *fakeHub) ConfirmMovieHub(_ context.Context, channelID, mode string, leaseSeconds int, _ time.Time) (bool, error) {
	f.confirmed = append(f.confirmed, fmt.Sprintf("%s|%s|%d", channelID, mode, leaseSeconds))
	return channelID == hubLinked && mode == "subscribe" || channelID != hubLinked && mode == "unsubscribe", nil
}

func (f *fakeHub) ClassifyPushed(_ context.Context, e movies.PushedEntry) (store.PushedVideo, error) {
	return store.PushedVideo{VideoID: e.VideoID, ChannelID: e.ChannelID, Title: e.Title, PublishedAt: e.PublishedAt, TTA: true}, nil
}

func (f *fakeHub) StorePushedVideo(_ context.Context, v store.PushedVideo, _ time.Time) error {
	if f.storeErr != nil {
		return f.storeErr
	}
	if v.ChannelID != hubLinked {
		return store.ErrNoChannel
	}
	f.stored = append(f.stored, v.VideoID+"|"+v.Title)
	return nil
}

func (f *fakeHub) RemovePushedVideo(_ context.Context, videoID, channelID string, _ time.Time) error {
	f.removed = append(f.removed, videoID+"|"+channelID)
	return nil
}

func hubSign(secret []byte, body string) string {
	mac := hmac.New(sha1.New, secret)
	mac.Write([]byte(body))
	return "sha1=" + hex.EncodeToString(mac.Sum(nil))
}

func hubUpload(channel, video, title string) string {
	return `<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns="http://www.w3.org/2005/Atom"><entry>` +
		`<yt:videoId>` + video + `</yt:videoId><yt:channelId>` + channel + `</yt:channelId><title>` + title + `</title>` +
		`<published>2026-11-15T08:00:00+00:00</published></entry></feed>`
}

func hubServe(hub MovieHub, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	NewMux(&fakeRatings{}, nil, nil, nil, hub, nil, slog.New(slog.DiscardHandler), &recordedRequests{}, stepClock(time.Millisecond)).ServeHTTP(rec, req)
	return rec
}

func hubPost(hub MovieHub, body, signature string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/movies/hub", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/atom+xml")
	if signature != "" {
		req.Header.Set("X-Hub-Signature", signature)
	}
	return hubServe(hub, req)
}

func hubCheck(hub MovieHub, mode, topic, lease string) *httptest.ResponseRecorder {
	q := url.Values{"hub.mode": {mode}, "hub.topic": {topic}, "hub.challenge": {"invented-challenge"}}
	if lease != "" {
		q.Set("hub.lease_seconds", lease)
	}
	return hubServe(hub, httptest.NewRequest(http.MethodGet, "/v1/movies/hub?"+q.Encode(), nil))
}

func TestTheHubCallbackTakesOnlySignedPushes(t *testing.T) {
	body := hubUpload(hubLinked, "vid00000001", "Through the Ages final")
	tests := []struct {
		name      string
		noHub     bool
		signature string
		wantCode  int
	}{
		{"no hub secret set up here", true, hubSign(hubSecret, body), http.StatusNotFound},
		{"no signature", false, "", http.StatusUnauthorized},
		{"signed by another secret", false, hubSign([]byte("another-secret"), body), http.StatusUnauthorized},
		{"a body changed after signing", false, hubSign(hubSecret, hubUpload(hubLinked, "vid00000001", "Other")), http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := &fakeHub{}
			var h MovieHub = hub
			if tt.noHub {
				h = nil
			}
			rec := hubPost(h, body, tt.signature)
			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if len(hub.stored) != 0 || len(hub.removed) != 0 {
				t.Errorf("a refused push reached the store (stored %q, removed %q), want nothing", hub.stored, hub.removed)
			}
			if got := rec.Header().Get("Cache-Control"); got != cacheNone {
				t.Errorf("Cache-Control = %q, want %q", got, cacheNone)
			}
		})
	}
}

func TestTheHubCallbackIsOffWithoutASecretForChecksToo(t *testing.T) {
	rec := hubCheck(nil, "subscribe", movies.Topic(hubLinked), "432000")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestTheHubCallbackStoresAPushedUpload(t *testing.T) {
	hub := &fakeHub{}
	body := hubUpload(hubLinked, "vid00000001", "Through the Ages final")
	rec := hubPost(hub, body, hubSign(hubSecret, body))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body %s", rec.Code, rec.Body)
	}
	if want := []string{"vid00000001|Through the Ages final"}; !slices.Equal(hub.stored, want) {
		t.Errorf("stored = %q, want %q", hub.stored, want)
	}
}

func TestTheHubCallbackIgnoresAnUnlinkedChannelsUpload(t *testing.T) {
	hub := &fakeHub{}
	body := hubUpload(hubUnlinked, "vid00000002", "Through the Ages final")
	rec := hubPost(hub, body, hubSign(hubSecret, body))
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204, so the hub does not retry", rec.Code)
	}
	if len(hub.stored) != 0 {
		t.Errorf("stored = %q, want nothing", hub.stored)
	}
}

func TestTheHubCallbackRemovesADeletedVideo(t *testing.T) {
	hub := &fakeHub{}
	body := `<feed xmlns:at="http://purl.org/atompub/tombstones/1.0" xmlns="http://www.w3.org/2005/Atom">` +
		`<at:deleted-entry ref="yt:video:vid00000001"><at:by><uri>https://www.youtube.com/channel/` + hubLinked + `</uri></at:by></at:deleted-entry></feed>`
	rec := hubPost(hub, body, hubSign(hubSecret, body))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if want := []string{"vid00000001|" + hubLinked}; !slices.Equal(hub.removed, want) {
		t.Errorf("removed = %q, want %q", hub.removed, want)
	}
}

func TestTheHubCallbackAsksForARetryWhenStoringFails(t *testing.T) {
	hub := &fakeHub{storeErr: errors.New("database down")}
	body := hubUpload(hubLinked, "vid00000001", "Through the Ages final")
	if rec := hubPost(hub, body, hubSign(hubSecret, body)); rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500, so the hub sends it again", rec.Code)
	}
}

func TestTheHubCallbackRefusesASignedBodyThatIsNotAtom(t *testing.T) {
	hub := &fakeHub{}
	body := "not xml <"
	if rec := hubPost(hub, body, hubSign(hubSecret, body)); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestTheHubCallbackConfirmsOnlyRequestsWeMade(t *testing.T) {
	tests := []struct {
		name          string
		mode, topic   string
		lease         string
		wantCode      int
		wantConfirmed []string
	}{
		{"a subscribe to a linked channel", "subscribe", movies.Topic(hubLinked), "432000", http.StatusOK, []string{hubLinked + "|subscribe|432000"}},
		{"a subscribe without a lease", "subscribe", movies.Topic(hubLinked), "", http.StatusOK, []string{hubLinked + "|subscribe|0"}},
		{"a subscribe to an unlinked channel", "subscribe", movies.Topic(hubUnlinked), "432000", http.StatusNotFound, []string{hubUnlinked + "|subscribe|432000"}},
		{"an unsubscribe from an unlinked channel", "unsubscribe", movies.Topic(hubUnlinked), "", http.StatusOK, []string{hubUnlinked + "|unsubscribe|0"}},
		{"a topic that is not a channel's feed", "subscribe", "https://example.com/feed", "432000", http.StatusNotFound, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := &fakeHub{}
			rec := hubCheck(hub, tt.mode, tt.topic, tt.lease)
			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if tt.wantCode == http.StatusOK && rec.Body.String() != "invented-challenge" {
				t.Errorf("body = %q, want the challenge echoed", rec.Body)
			}
			if tt.wantCode != http.StatusOK && strings.Contains(rec.Body.String(), "invented-challenge") {
				t.Errorf("body = %q echoes the challenge for a request we did not make", rec.Body)
			}
			if !slices.Equal(hub.confirmed, tt.wantConfirmed) {
				t.Errorf("asked the store %q, want %q", hub.confirmed, tt.wantConfirmed)
			}
		})
	}
}
