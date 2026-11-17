package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

func (*fakeAdmins) RedditFeeds(context.Context) ([]store.RedditFeed, error) {
	checked := time.Date(2026, 11, 17, 1, 0, 0, 0, time.UTC)
	return []store.RedditFeed{{ID: 3, Subreddit: "example", CheckedAt: &checked, LastError: "Reddit answered 429", LastErrorAt: &checked}}, nil
}

func (f *fakeAdmins) AddRedditFeed(_ context.Context, subreddit string, _ time.Time) (int64, error) {
	switch subreddit {
	case "bad name":
		return 0, store.ErrBadRedditFeed
	case "example":
		return 0, store.ErrRedditFeedExists
	}
	f.notes = append(f.notes, "add feed "+subreddit)
	return 4, nil
}

func (f *fakeAdmins) SetRedditFeedPaused(_ context.Context, id int64, paused bool) error {
	if id != 3 {
		return store.ErrNoRedditFeed
	}
	f.notes = append(f.notes, fmt.Sprintf("feed %d paused %t", id, paused))
	return nil
}

func (f *fakeAdmins) RemoveRedditFeed(_ context.Context, id int64) error {
	if id != 3 {
		return store.ErrNoRedditFeed
	}
	f.notes = append(f.notes, fmt.Sprintf("remove feed %d", id))
	return nil
}

func TestAnAdminAddsPausesAndRemovesRedditFeeds(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	f := &fakeAdmins{}
	rec := adminDoAs(t, f, owner, http.MethodGet, "/v1/admin/reddit-feeds", "", "")
	var list []RedditFeed
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 || list[0].Subreddit != "example" ||
		list[0].LastError == nil || *list[0].LastError != "Reddit answered 429" || list[0].LastPostAt != nil {
		t.Fatalf("GET feeds = %d %s, want example with its last error and no post yet", rec.Code, rec.Body)
	}
	rec = adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/reddit-feeds", site, `{"subreddit": " r/throughtheages "}`)
	var made RedditFeed
	if err := json.Unmarshal(rec.Body.Bytes(), &made); err != nil || rec.Code != http.StatusCreated || made.Id != 4 || made.Subreddit != "throughtheages" {
		t.Fatalf("POST feed = %d %s, want 201 with id 4 for throughtheages", rec.Code, rec.Body)
	}
	for _, step := range []struct{ method, path, body string }{
		{http.MethodPut, "/v1/admin/reddit-feeds/3", `{"paused": true}`},
		{http.MethodDelete, "/v1/admin/reddit-feeds/3", ""},
	} {
		if rec := adminDoAs(t, f, owner, step.method, step.path, site, step.body); rec.Code != http.StatusNoContent {
			t.Errorf("%s %s = %d %s, want 204", step.method, step.path, rec.Code, rec.Body)
		}
	}
	if got, want := strings.Join(f.notes, "|"), "add feed throughtheages|feed 3 paused true|remove feed 3"; got != want {
		t.Errorf("changes = %q, want %q", got, want)
	}
	want := []string{owner + " reddit.feed.add throughtheages", owner + " reddit.feed.set 3", owner + " reddit.feed.remove 3"}
	if strings.Join(f.audits, "|") != strings.Join(want, "|") {
		t.Errorf("audits = %q, want %q", f.audits, want)
	}
}

func TestRedditFeedChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site, owner, org = "https://ttac-admin.example", "111111111111111111", "222222222222222222"
	tests := []struct {
		name, who, method, path, origin, body string
		want                                  int
	}{
		{"an organiser lists", org, http.MethodGet, "/v1/admin/reddit-feeds", "", "", http.StatusForbidden},
		{"an organiser adds", org, http.MethodPost, "/v1/admin/reddit-feeds", site, `{"subreddit": "new"}`, http.StatusForbidden},
		{"an organiser pauses", org, http.MethodPut, "/v1/admin/reddit-feeds/3", site, `{"paused": true}`, http.StatusForbidden},
		{"an organiser removes", org, http.MethodDelete, "/v1/admin/reddit-feeds/3", site, "", http.StatusForbidden},
		{"another site", owner, http.MethodPost, "/v1/admin/reddit-feeds", "https://evil.example", `{"subreddit": "new"}`, http.StatusForbidden},
		{"a name Reddit cannot have", owner, http.MethodPost, "/v1/admin/reddit-feeds", site, `{"subreddit": "bad name"}`, http.StatusBadRequest},
		{"one already there", owner, http.MethodPost, "/v1/admin/reddit-feeds", site, `{"subreddit": "example"}`, http.StatusConflict},
		{"no subreddit", owner, http.MethodPost, "/v1/admin/reddit-feeds", site, `{}`, http.StatusBadRequest},
		{"pausing one that is not there", owner, http.MethodPut, "/v1/admin/reddit-feeds/9", site, `{"paused": true}`, http.StatusNotFound},
		{"removing one that is not there", owner, http.MethodDelete, "/v1/admin/reddit-feeds/9", site, "", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			if rec := adminDoAs(t, f, tt.who, tt.method, tt.path, tt.origin, tt.body); rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.notes) != 0 || len(f.audits) != 0 {
				t.Errorf("changes = %q, audits = %q; want none", f.notes, f.audits)
			}
		})
	}
}
