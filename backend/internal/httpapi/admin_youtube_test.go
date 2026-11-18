package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

func (*fakeAdmins) YouTubeFeeds(context.Context) ([]store.YouTubeFeed, error) {
	return []store.YouTubeFeed{{ID: 8, Channels: 3, Waiting: 1, CreatedAt: time.Date(2026, 11, 18, 1, 0, 0, 0, time.UTC)}}, nil
}

func (f *fakeAdmins) AddYouTubeFeed(context.Context, time.Time) (int64, error) {
	if f.youtubeThere {
		return 0, store.ErrYouTubeFeedExists
	}
	f.notes = append(f.notes, "add youtube")
	return 9, nil
}

func (f *fakeAdmins) SetYouTubeFeedPaused(_ context.Context, id int64, paused bool) error {
	if id != 8 {
		return store.ErrNoYouTubeFeed
	}
	f.notes = append(f.notes, fmt.Sprintf("youtube %d paused %t", id, paused))
	return nil
}

func (f *fakeAdmins) RemoveYouTubeFeed(_ context.Context, id int64) error {
	if id != 8 {
		return store.ErrNoYouTubeFeed
	}
	f.notes = append(f.notes, fmt.Sprintf("remove youtube %d", id))
	return nil
}

// youtubeDo is adminDoAs with the Movies hub callback set up when hub is.
func youtubeDo(t *testing.T, f *fakeAdmins, hub bool, who, method, path, origin, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Cf-Access-Jwt-Assertion", "good-"+who)
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	var h MovieHub
	if hub {
		h = &fakeHub{}
	}
	rec := httptest.NewRecorder()
	NewMux(&fakeRatings{}, f, nil, nil, h, nil, nil, slog.New(slog.DiscardHandler), &recordedRequests{}, stepClock(time.Millisecond)).ServeHTTP(rec, req)
	return rec
}

func TestAnAdminAddsPausesAndRemovesTheYouTubeFeed(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	f := &fakeAdmins{}
	rec := youtubeDo(t, f, true, owner, http.MethodGet, "/v1/admin/youtube-feeds", "", "")
	var list []YouTubeFeed
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 || list[0].Id != 8 || list[0].Channels != 3 || list[0].Waiting != 1 {
		t.Fatalf("GET feeds = %d %s, want feed 8 watching 3 channels, 1 stream waiting", rec.Code, rec.Body)
	}
	rec = youtubeDo(t, f, true, owner, http.MethodPost, "/v1/admin/youtube-feeds", site, "")
	var made YouTubeFeed
	if err := json.Unmarshal(rec.Body.Bytes(), &made); err != nil || rec.Code != http.StatusCreated || made.Id != 9 {
		t.Fatalf("POST feed = %d %s, want 201 with id 9", rec.Code, rec.Body)
	}
	for _, step := range []struct{ method, path, body string }{
		{http.MethodPut, "/v1/admin/youtube-feeds/8", `{"paused": true}`},
		{http.MethodDelete, "/v1/admin/youtube-feeds/8", ""},
	} {
		if rec := youtubeDo(t, f, true, owner, step.method, step.path, site, step.body); rec.Code != http.StatusNoContent {
			t.Errorf("%s %s = %d %s, want 204", step.method, step.path, rec.Code, rec.Body)
		}
	}
	if got, want := strings.Join(f.notes, "|"), "add youtube|youtube 8 paused true|remove youtube 8"; got != want {
		t.Errorf("changes = %q, want %q", got, want)
	}
	want := []string{owner + " youtube.feed.add 9", owner + " youtube.feed.set 8", owner + " youtube.feed.remove 8"}
	if strings.Join(f.audits, "|") != strings.Join(want, "|") {
		t.Errorf("audits = %q, want %q", f.audits, want)
	}
}

func TestYouTubeFeedChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site, owner, org = "https://ttac-admin.example", "111111111111111111", "222222222222222222"
	tests := []struct {
		name, who, method, path, origin, body string
		noHub, there                          bool
		want                                  int
	}{
		{name: "an organiser lists", who: org, method: http.MethodGet, path: "/v1/admin/youtube-feeds", want: http.StatusForbidden},
		{name: "an organiser adds", who: org, method: http.MethodPost, path: "/v1/admin/youtube-feeds", origin: site, want: http.StatusForbidden},
		{name: "an organiser pauses", who: org, method: http.MethodPut, path: "/v1/admin/youtube-feeds/8", origin: site, body: `{"paused": true}`, want: http.StatusForbidden},
		{name: "an organiser removes", who: org, method: http.MethodDelete, path: "/v1/admin/youtube-feeds/8", origin: site, want: http.StatusForbidden},
		{name: "another site", who: owner, method: http.MethodPost, path: "/v1/admin/youtube-feeds", origin: "https://evil.example", want: http.StatusForbidden},
		{name: "no hub callback here", who: owner, method: http.MethodPost, path: "/v1/admin/youtube-feeds", origin: site, noHub: true, want: http.StatusServiceUnavailable},
		{name: "one already there", who: owner, method: http.MethodPost, path: "/v1/admin/youtube-feeds", origin: site, there: true, want: http.StatusConflict},
		{name: "pausing one that is not there", who: owner, method: http.MethodPut, path: "/v1/admin/youtube-feeds/9", origin: site, body: `{"paused": true}`, want: http.StatusNotFound},
		{name: "removing one that is not there", who: owner, method: http.MethodDelete, path: "/v1/admin/youtube-feeds/9", origin: site, want: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{youtubeThere: tt.there}
			if rec := youtubeDo(t, f, !tt.noHub, tt.who, tt.method, tt.path, tt.origin, tt.body); rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.notes) != 0 || len(f.audits) != 0 {
				t.Errorf("changes = %q, audits = %q; want none", f.notes, f.audits)
			}
		})
	}
}
