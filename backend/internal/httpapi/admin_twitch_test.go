package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
	"ttacommunity/backend/internal/twitch"
)

// The fake knows Twitch feed 3, some_streamer, subscribed to both events.
var twitchThree = store.TwitchFeed{ID: 3, Login: "some_streamer", UserID: "100000001", DisplayName: "Some_Streamer",
	Subscriptions: []store.TwitchSubscription{{Type: "stream.offline", ID: "sub-2", Status: "enabled"}, {Type: "stream.online", ID: "sub-1", Status: "enabled"}}}

func (f *fakeAdmins) TwitchFeeds(context.Context) ([]store.TwitchFeed, error) {
	return []store.TwitchFeed{twitchThree}, nil
}

func (f *fakeAdmins) TwitchFeed(_ context.Context, id int64) (store.TwitchFeed, error) {
	if id != 3 {
		return store.TwitchFeed{}, store.ErrNoTwitchFeed
	}
	return twitchThree, nil
}

func (f *fakeAdmins) TwitchUser(_ context.Context, login string) (twitch.User, error) {
	if f.twitchErr != nil {
		return twitch.User{}, f.twitchErr
	}
	if login != "other_streamer" && login != "some_streamer" {
		return twitch.User{}, twitch.ErrNoUser
	}
	return twitch.User{ID: "100000002", Login: login, DisplayName: "Other_Streamer"}, nil
}

func (f *fakeAdmins) AddTwitchFeed(_ context.Context, login, userID, displayName string, _ time.Time) (int64, error) {
	if login == "some_streamer" {
		return 0, store.ErrTwitchFeedExists
	}
	f.notes = append(f.notes, fmt.Sprintf("add %s %s %s", login, userID, displayName))
	return 4, nil
}

func (f *fakeAdmins) SubscribeTwitch(_ context.Context, typ, userID string) (twitch.Subscription, error) {
	f.notes = append(f.notes, "subscribe "+typ+" "+userID)
	return twitch.Subscription{ID: "sub-" + typ, Status: "webhook_callback_verification_pending"}, nil
}

func (f *fakeAdmins) SetTwitchSubscription(_ context.Context, feed int64, typ, id, status string, _ time.Time) error {
	f.notes = append(f.notes, fmt.Sprintf("record %d %s %s %s", feed, typ, id, status))
	return nil
}

func (f *fakeAdmins) UnsubscribeTwitch(_ context.Context, id string) error {
	f.notes = append(f.notes, "unsubscribe "+id)
	return nil
}

func (f *fakeAdmins) SetTwitchFeedPaused(_ context.Context, id int64, paused bool) error {
	if id != 3 {
		return store.ErrNoTwitchFeed
	}
	f.twitchPaused = &paused
	return nil
}

func (f *fakeAdmins) RemoveTwitchFeed(_ context.Context, id int64) error {
	if id != 3 {
		return store.ErrNoTwitchFeed
	}
	f.notes = append(f.notes, fmt.Sprintf("remove %d", id))
	return nil
}

func TestAnAdminAddsPausesAndRemovesTwitchStreamers(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	f := &fakeAdmins{}
	rec := adminDoAs(t, f, owner, http.MethodGet, "/v1/admin/twitch-feeds", "", "")
	var list []TwitchFeed
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 || list[0].Login != "some_streamer" || len(list[0].Subscriptions) != 2 {
		t.Fatalf("GET = %d %s, want some_streamer with both subscriptions", rec.Code, rec.Body)
	}
	rec = adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/twitch-feeds", site, `{"login": "https://www.twitch.tv/Other_Streamer"}`)
	var made TwitchFeed
	if err := json.Unmarshal(rec.Body.Bytes(), &made); err != nil || rec.Code != http.StatusCreated || made.Id != 4 || len(made.Subscriptions) != 2 {
		t.Fatalf("POST = %d %s, want 201, feed 4 with its two subscriptions", rec.Code, rec.Body)
	}
	for _, step := range []struct{ method, path, body string }{
		{http.MethodPut, "/v1/admin/twitch-feeds/3", `{"paused": true}`},
		{http.MethodDelete, "/v1/admin/twitch-feeds/3", ""},
	} {
		if rec := adminDoAs(t, f, owner, step.method, step.path, site, step.body); rec.Code != http.StatusNoContent {
			t.Errorf("%s %s = %d %s, want 204", step.method, step.path, rec.Code, rec.Body)
		}
	}
	want := []string{"add other_streamer 100000002 Other_Streamer",
		"subscribe stream.online 100000002", "record 4 stream.online sub-stream.online webhook_callback_verification_pending",
		"subscribe stream.offline 100000002", "record 4 stream.offline sub-stream.offline webhook_callback_verification_pending",
		"unsubscribe sub-2", "unsubscribe sub-1", "remove 3"}
	if got := strings.Join(f.notes, "|"); got != strings.Join(want, "|") {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	if f.twitchPaused == nil || !*f.twitchPaused {
		t.Errorf("paused = %v, want true", f.twitchPaused)
	}
	wantAudits := owner + " twitch_feed.add other_streamer|" + owner + " twitch_feed.set 3|" + owner + " twitch_feed.remove 3"
	if got := strings.Join(f.audits, "|"); got != wantAudits {
		t.Errorf("audits = %q, want %q", got, wantAudits)
	}
}

func TestTwitchStreamerChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site, owner, org = "https://ttac-admin.example", "111111111111111111", "222222222222222222"
	good := `{"login": "other_streamer"}`
	tests := []struct {
		name, who, method, path, origin, body string
		twitchErr                             error
		want                                  int
	}{
		{"an organiser lists", org, http.MethodGet, "/v1/admin/twitch-feeds", "", "", nil, http.StatusForbidden},
		{"an organiser adds", org, http.MethodPost, "/v1/admin/twitch-feeds", site, good, nil, http.StatusForbidden},
		{"an organiser pauses", org, http.MethodPut, "/v1/admin/twitch-feeds/3", site, `{"paused": true}`, nil, http.StatusForbidden},
		{"an organiser removes", org, http.MethodDelete, "/v1/admin/twitch-feeds/3", site, "", nil, http.StatusForbidden},
		{"another site adds", owner, http.MethodPost, "/v1/admin/twitch-feeds", "https://evil.example", good, nil, http.StatusForbidden},
		{"a name Twitch does not know", owner, http.MethodPost, "/v1/admin/twitch-feeds", site, `{"login": "nobody_here"}`, nil, http.StatusBadRequest},
		{"no name", owner, http.MethodPost, "/v1/admin/twitch-feeds", site, `{"login": "  "}`, nil, http.StatusBadRequest},
		{"a streamer already listed", owner, http.MethodPost, "/v1/admin/twitch-feeds", site, `{"login": "some_streamer"}`, nil, http.StatusConflict},
		{"Twitch not set up here", owner, http.MethodPost, "/v1/admin/twitch-feeds", site, good, ErrTwitchNotSetUp, http.StatusServiceUnavailable},
		{"Twitch unreachable", owner, http.MethodPost, "/v1/admin/twitch-feeds", site, good, errors.New("down"), http.StatusServiceUnavailable},
		{"pausing no such feed", owner, http.MethodPut, "/v1/admin/twitch-feeds/9", site, `{"paused": true}`, nil, http.StatusNotFound},
		{"removing no such feed", owner, http.MethodDelete, "/v1/admin/twitch-feeds/9", site, "", nil, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{twitchErr: tt.twitchErr}
			if rec := adminDoAs(t, f, tt.who, tt.method, tt.path, tt.origin, tt.body); rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.audits) != 0 || strings.Contains(strings.Join(f.notes, "|"), "subscribe") {
				t.Errorf("changes = %q, audits = %q; want none", f.notes, f.audits)
			}
		})
	}
}

func TestAnEndpointsFeedOptionsReachTheStoreAndComeBack(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	f := &fakeAdmins{}
	body := `{"name": "Twitch", "url": "` + testWebhook + `", "events": [], "feeds": [7],
		"feed_options": [{"feed": 7, "categories": ["Through the Ages"], "keywords": ["tta"], "was_live": true}]}`
	rec := adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/notification-endpoints", site, body)
	var made NotificationEndpoint
	if err := json.Unmarshal(rec.Body.Bytes(), &made); err != nil || rec.Code != http.StatusCreated {
		t.Fatalf("POST = %d %s, want 201", rec.Code, rec.Body)
	}
	if len(made.FeedOptions) != 1 || made.FeedOptions[0].Feed != 7 || !made.FeedOptions[0].WasLive || made.FeedOptions[0].Keywords[0] != "tta" {
		t.Errorf("answered options %+v, want feed 7's", made.FeedOptions)
	}
	if got, want := f.feedOptions, "[{Feed:7 Categories:[Through the Ages] Keywords:[tta] WasLive:true}]"; got != want {
		t.Errorf("stored options %q, want %q", got, want)
	}
}
