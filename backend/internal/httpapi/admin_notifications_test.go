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

	"ttacommunity/backend/internal/notify"
	"ttacommunity/backend/internal/store"
)

// testWebhook is an invented Discord webhook URL.
const testWebhook = "https://discord.com/api/webhooks/100000000000000001/token"

func (*fakeAdmins) NotificationEndpoints(context.Context) ([]store.NotificationEndpoint, error) {
	return []store.NotificationEndpoint{{ID: 3, Name: "Admins", URL: testWebhook, Format: "discord", Events: []string{"link_request"}, Prefix: "[dev]"}}, nil
}

func (*fakeAdmins) NotificationEndpoint(_ context.Context, id int64) (store.NotificationEndpoint, error) {
	if id != 3 {
		return store.NotificationEndpoint{}, store.ErrNoNotificationEndpoint
	}
	return store.NotificationEndpoint{ID: 3, Name: "Admins", URL: testWebhook, Format: "discord"}, nil
}

// endpointNote writes an endpoint as the fake records it, or refuses one named "bad" as the database would.
func endpointNote(verb string, e store.NotificationEndpoint) (string, error) {
	if e.Name == "bad" {
		return "", store.ErrBadNotificationEndpoint
	}
	return fmt.Sprintf("%s endpoint %d %s %s %s %v %s", verb, e.ID, e.Name, e.URL, e.Format, e.Events, e.Prefix), nil
}

func (f *fakeAdmins) AddNotificationEndpoint(_ context.Context, e store.NotificationEndpoint, _ time.Time) (int64, error) {
	note, err := endpointNote("add", e)
	if err != nil {
		return 0, err
	}
	f.notes = append(f.notes, note)
	return 4, nil
}

func (f *fakeAdmins) SetNotificationEndpoint(_ context.Context, e store.NotificationEndpoint) error {
	if e.ID != 3 {
		return store.ErrNoNotificationEndpoint
	}
	note, err := endpointNote("set", e)
	if err != nil {
		return err
	}
	f.notes = append(f.notes, note)
	return nil
}

func (f *fakeAdmins) RemoveNotificationEndpoint(_ context.Context, id int64) error {
	if id != 3 {
		return store.ErrNoNotificationEndpoint
	}
	f.notes = append(f.notes, fmt.Sprintf("remove endpoint %d", id))
	return nil
}

func (f *fakeAdmins) PostTestNotification(_ context.Context, e store.NotificationEndpoint) error {
	f.notes = append(f.notes, fmt.Sprintf("test endpoint %d", e.ID))
	return f.postErr
}

func TestAnAdminAddsChangesAndRemovesNotificationEndpoints(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	f := &fakeAdmins{}
	rec := adminDoAs(t, f, owner, http.MethodGet, "/v1/admin/notification-endpoints", "", "")
	var list []NotificationEndpoint
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 || list[0].Url != testWebhook || list[0].Prefix != "[dev]" {
		t.Fatalf("GET endpoints = %d %s, want the endpoint with its URL", rec.Code, rec.Body)
	}
	rec = adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/notification-endpoints", site,
		`{"name": "Organisers", "url": "`+testWebhook+`", "events": ["blitz_report", "blitz_flag"]}`)
	var made NotificationEndpoint
	if err := json.Unmarshal(rec.Body.Bytes(), &made); err != nil || rec.Code != http.StatusCreated || made.Id != 4 || made.Format != "discord" {
		t.Fatalf("POST endpoint = %d %s, want 201 with id 4, a discord endpoint", rec.Code, rec.Body)
	}
	for _, step := range []struct{ method, path, body string }{
		{http.MethodPut, "/v1/admin/notification-endpoints/3", `{"name": "Admins", "url": "` + testWebhook + `", "format": "discord", "events": [], "prefix": "[dev]"}`},
		{http.MethodDelete, "/v1/admin/notification-endpoints/3", ""},
	} {
		if rec := adminDoAs(t, f, owner, step.method, step.path, site, step.body); rec.Code != http.StatusNoContent {
			t.Errorf("%s %s = %d %s, want 204", step.method, step.path, rec.Code, rec.Body)
		}
	}
	want := "add endpoint 0 Organisers " + testWebhook + " discord [blitz_report blitz_flag] |set endpoint 3 Admins " + testWebhook + " discord [] [dev]|remove endpoint 3"
	if got := strings.Join(f.notes, "|"); got != want {
		t.Errorf("changes = %q, want %q", got, want)
	}
	if want := []string{owner + " notification.endpoint.add Organisers", owner + " notification.endpoint.set 3", owner + " notification.endpoint.remove 3"}; strings.Join(f.audits, "|") != strings.Join(want, "|") {
		t.Errorf("audits = %q, want %q", f.audits, want)
	}
}

func TestNotificationEndpointChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site, owner, org = "https://ttac-admin.example", "111111111111111111", "222222222222222222"
	good := `{"name": "Admins", "url": "` + testWebhook + `", "events": ["link_request"]}`
	tests := []struct {
		name, who, method, path, origin, body string
		want                                  int
	}{
		{"an organiser lists", org, http.MethodGet, "/v1/admin/notification-endpoints", "", "", http.StatusForbidden},
		{"an organiser adds", org, http.MethodPost, "/v1/admin/notification-endpoints", site, good, http.StatusForbidden},
		{"an organiser changes", org, http.MethodPut, "/v1/admin/notification-endpoints/3", site, good, http.StatusForbidden},
		{"an organiser removes", org, http.MethodDelete, "/v1/admin/notification-endpoints/3", site, "", http.StatusForbidden},
		{"an organiser tests", org, http.MethodPost, "/v1/admin/notification-endpoints/3/test", site, "", http.StatusForbidden},
		{"another site", owner, http.MethodPost, "/v1/admin/notification-endpoints", "https://evil.example", good, http.StatusForbidden},
		{"an endpoint the database refuses", owner, http.MethodPost, "/v1/admin/notification-endpoints", site, `{"name": "bad", "url": "x", "events": []}`, http.StatusBadRequest},
		{"an unknown event", owner, http.MethodPost, "/v1/admin/notification-endpoints", site, `{"name": "Admins", "url": "` + testWebhook + `", "events": ["pizza"]}`, http.StatusBadRequest},
		{"another format", owner, http.MethodPost, "/v1/admin/notification-endpoints", site, `{"name": "Admins", "url": "` + testWebhook + `", "format": "slack", "events": []}`, http.StatusBadRequest},
		{"a change the database refuses", owner, http.MethodPut, "/v1/admin/notification-endpoints/3", site, `{"name": "bad", "url": "x", "events": []}`, http.StatusBadRequest},
		{"changing one that is not there", owner, http.MethodPut, "/v1/admin/notification-endpoints/9", site, good, http.StatusNotFound},
		{"removing one that is not there", owner, http.MethodDelete, "/v1/admin/notification-endpoints/9", site, "", http.StatusNotFound},
		{"testing one that is not there", owner, http.MethodPost, "/v1/admin/notification-endpoints/9/test", site, "", http.StatusNotFound},
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

func TestTestingANotificationEndpointSaysHowDiscordAnswered(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	tests := []struct {
		name      string
		postErr   error
		delivered bool
		message   string
	}{
		{"posted", nil, true, "Posted: look for the test message in the channel"},
		{"a webhook that is gone", &notify.RefusedError{Status: http.StatusNotFound}, false,
			"Discord answered 404: the webhook was deleted, or the URL is wrong"},
		{"refused otherwise", &notify.RefusedError{Status: http.StatusTooManyRequests}, false, "Discord answered 429"},
		{"Discord unreachable", errors.New("posting to Discord: i/o timeout"), false, "Discord could not be reached"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{postErr: tt.postErr}
			rec := adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/notification-endpoints/3/test", site, "")
			var got NotificationTest
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
				t.Fatalf("POST test = %d %s, want 200", rec.Code, rec.Body)
			}
			if got.Delivered != tt.delivered || got.Message != tt.message {
				t.Errorf("test = %+v, want delivered %t, %q", got, tt.delivered, tt.message)
			}
			if strings.Join(f.notes, "|") != "test endpoint 3" {
				t.Errorf("posted = %q, want one test to endpoint 3", f.notes)
			}
		})
	}
}
