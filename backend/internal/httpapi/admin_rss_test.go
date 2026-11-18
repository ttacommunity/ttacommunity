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

func (*fakeAdmins) RSSFeeds(context.Context) ([]store.RSSFeed, error) {
	checked := time.Date(2026, 11, 18, 1, 0, 0, 0, time.UTC)
	return []store.RSSFeed{{ID: 5, URL: "https://news.example.com/feed.xml", Title: "Example news", CheckedAt: &checked, LastItemAt: &checked}}, nil
}

func (f *fakeAdmins) AddRSSFeed(_ context.Context, url string, _ time.Time) (int64, error) {
	switch url {
	case "https://news.example.com/bad feed":
		return 0, store.ErrBadRSSFeed
	case "https://news.example.com/feed.xml":
		return 0, store.ErrRSSFeedExists
	}
	f.notes = append(f.notes, "add rss "+url)
	return 6, nil
}

func (f *fakeAdmins) SetRSSFeed(_ context.Context, id int64, paused bool) error {
	if id != 5 {
		return store.ErrNoRSSFeed
	}
	f.notes = append(f.notes, fmt.Sprintf("rss %d paused %t", id, paused))
	return nil
}

func (f *fakeAdmins) RemoveRSSFeed(_ context.Context, id int64) error {
	if id != 5 {
		return store.ErrNoRSSFeed
	}
	f.notes = append(f.notes, fmt.Sprintf("remove rss %d", id))
	return nil
}

func TestAnAdminAddsPausesAndRemovesRSSFeeds(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	f := &fakeAdmins{}
	rec := adminDoAs(t, f, owner, http.MethodGet, "/v1/admin/rss-feeds", "", "")
	var list []RSSFeed
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 || list[0].Title != "Example news" || list[0].LastError != nil {
		t.Fatalf("GET feeds = %d %s, want Example news without an error", rec.Code, rec.Body)
	}
	rec = adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/rss-feeds", site, `{"url": " https://blog.example.com/atom.xml "}`)
	var made RSSFeed
	if err := json.Unmarshal(rec.Body.Bytes(), &made); err != nil || rec.Code != http.StatusCreated || made.Id != 6 || made.Url != "https://blog.example.com/atom.xml" {
		t.Fatalf("POST feed = %d %s, want 201 with id 6 for the address trimmed", rec.Code, rec.Body)
	}
	for _, step := range []struct{ method, path, body string }{
		{http.MethodPut, "/v1/admin/rss-feeds/5", `{"paused": true}`},
		{http.MethodDelete, "/v1/admin/rss-feeds/5", ""},
	} {
		if rec := adminDoAs(t, f, owner, step.method, step.path, site, step.body); rec.Code != http.StatusNoContent {
			t.Errorf("%s %s = %d %s, want 204", step.method, step.path, rec.Code, rec.Body)
		}
	}
	if got, want := strings.Join(f.notes, "|"), "add rss https://blog.example.com/atom.xml|rss 5 paused true|remove rss 5"; got != want {
		t.Errorf("changes = %q, want %q", got, want)
	}
	want := []string{owner + " rss.feed.add https://blog.example.com/atom.xml", owner + " rss.feed.set 5", owner + " rss.feed.remove 5"}
	if strings.Join(f.audits, "|") != strings.Join(want, "|") {
		t.Errorf("audits = %q, want %q", f.audits, want)
	}
}

func TestRSSFeedChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site, owner, org = "https://ttac-admin.example", "111111111111111111", "222222222222222222"
	const good = `{"url": "https://blog.example.com/atom.xml"}`
	tests := []struct {
		name, who, method, path, origin, body string
		want                                  int
	}{
		{"an organiser lists", org, http.MethodGet, "/v1/admin/rss-feeds", "", "", http.StatusForbidden},
		{"an organiser adds", org, http.MethodPost, "/v1/admin/rss-feeds", site, good, http.StatusForbidden},
		{"an organiser pauses", org, http.MethodPut, "/v1/admin/rss-feeds/5", site, `{"paused": true}`, http.StatusForbidden},
		{"an organiser removes", org, http.MethodDelete, "/v1/admin/rss-feeds/5", site, "", http.StatusForbidden},
		{"another site", owner, http.MethodPost, "/v1/admin/rss-feeds", "https://evil.example", good, http.StatusForbidden},
		{"an address the database refuses", owner, http.MethodPost, "/v1/admin/rss-feeds", site, `{"url": "https://news.example.com/bad feed"}`, http.StatusBadRequest},
		{"localhost", owner, http.MethodPost, "/v1/admin/rss-feeds", site, `{"url": "https://localhost/feed"}`, http.StatusBadRequest},
		{"a loopback address", owner, http.MethodPost, "/v1/admin/rss-feeds", site, `{"url": "https://127.0.0.1:8080/feed"}`, http.StatusBadRequest},
		{"a private address", owner, http.MethodPost, "/v1/admin/rss-feeds", site, `{"url": "https://10.0.0.5/feed"}`, http.StatusBadRequest},
		{"the cloud's metadata address", owner, http.MethodPost, "/v1/admin/rss-feeds", site, `{"url": "https://169.254.169.254/latest"}`, http.StatusBadRequest},
		{"an IPv6 loopback", owner, http.MethodPost, "/v1/admin/rss-feeds", site, `{"url": "https://[::1]/feed"}`, http.StatusBadRequest},
		{"one already there", owner, http.MethodPost, "/v1/admin/rss-feeds", site, `{"url": "https://news.example.com/feed.xml"}`, http.StatusConflict},
		{"no address", owner, http.MethodPost, "/v1/admin/rss-feeds", site, `{"url": " "}`, http.StatusBadRequest},
		{"pausing one that is not there", owner, http.MethodPut, "/v1/admin/rss-feeds/9", site, `{"paused": true}`, http.StatusNotFound},
		{"removing one that is not there", owner, http.MethodDelete, "/v1/admin/rss-feeds/9", site, "", http.StatusNotFound},
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
