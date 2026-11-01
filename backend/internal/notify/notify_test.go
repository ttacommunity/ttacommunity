package notify_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/notify"
	"ttacommunity/backend/internal/store"
)

const admin = "https://admin.example.test"

func blitz(kind, result string) store.Notification {
	return store.Notification{Kind: kind, Slug: "us_open", Series: "US Open", Edition: "2026", Round: 2, A: "Ann", B: "Bob",
		Result: result, ReportedBy: "Ann", FlagReason: "I won on culture"}
}

func TestText(t *testing.T) {
	corrected := blitz("blitz_report", "b")
	corrected.Corrected = true
	byBob := blitz("blitz_flag", "draw")
	byBob.ReportedBy = "Bob"
	tests := []struct {
		name string
		n    store.Notification
		want string
	}{
		{"a new player linked automatically", store.Notification{Kind: "sign_up", Name: "Dee", SignIns: []string{"discord"}, LinkedHow: "match"},
			"**New player**: someone signed in with Discord for the first time and was linked automatically to **Dee**. <" + admin + "/#members>"},
		{"a new player not linked", store.Notification{Kind: "sign_up", SignIns: []string{"email"}},
			"**New player**: someone signed in with email for the first time and is not linked to a TTA username yet. <" + admin + "/#members>"},
		{"a new player an admin linked before this was sent", store.Notification{Kind: "sign_up", Name: "Dee", SignIns: []string{"google"}, LinkedHow: "approved"},
			"**New player**: someone signed in with Google for the first time and was linked by an admin to **Dee**. <" + admin + "/#members>"},
		{"a link request", store.Notification{Kind: "link_request", Name: "Eve", SignIns: []string{"discord"}},
			"**Link request**: a player who signs in with Discord asks to be linked to **Eve**. Answer it in admin: <" + admin + "/#members>"},
		{"a link request from an account with several sign-ins", store.Notification{Kind: "link_request", Name: "Eve", SignIns: []string{"discord", "email", "google"}},
			"**Link request**: a player who signs in with Discord, email and Google asks to be linked to **Eve**. Answer it in admin: <" + admin + "/#members>"},
		{"a report", blitz("blitz_report", "a"),
			"**Blitz result reported**: US Open 2026, round 2: Ann beat Bob, reported by Ann. <" + admin + "/blitz/?t=us_open&e=2026>"},
		{"a correction", corrected,
			"**Blitz result corrected**: US Open 2026, round 2: Bob beat Ann, reported by Ann. <" + admin + "/blitz/?t=us_open&e=2026>"},
		{"a double loss", blitz("blitz_report", "double_loss"),
			"**Blitz result reported**: US Open 2026, round 2: Ann and Bob both lost, reported by Ann. <" + admin + "/blitz/?t=us_open&e=2026>"},
		{"a flag", blitz("blitz_flag", "a"),
			"**Blitz report flagged**: US Open 2026, round 2: Bob says Ann's report (Ann beat Bob) is wrong: I won on culture. <" + admin + "/blitz/?t=us_open&e=2026>"},
		{"a flag of the second player's report", byBob,
			"**Blitz report flagged**: US Open 2026, round 2: Ann says Bob's report (Ann and Bob drew) is wrong: I won on culture. <" + admin + "/blitz/?t=us_open&e=2026>"},
		{"names written as typed, not as markdown or mentions", store.Notification{Kind: "link_request", Name: "@everyone <@100000000000000001> **x**", SignIns: []string{"email"}},
			`**Link request**: a player who signs in with email asks to be linked to **@everyone \<@100000000000000001\> \*\*x\*\***. Answer it in admin: <` + admin + "/#members>"},
		{"an edition's address escaped", store.Notification{Kind: "blitz_report", Slug: "us_open", Series: "US Open", Edition: "Season 1 & 2", Round: 1, A: "Ann", B: "Bob", Result: "a", ReportedBy: "Ann"},
			"**Blitz result reported**: US Open Season 1 & 2, round 1: Ann beat Bob, reported by Ann. <" + admin + "/blitz/?t=us_open&e=Season+1+%26+2>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := notify.Text(tt.n, admin); got != tt.want {
				t.Errorf("Text(%+v)\n got = %q\nwant %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestMessagePutsTheEndpointsPrefixFirst(t *testing.T) {
	n := store.Notification{Kind: "link_request", Name: "Eve", SignIns: []string{"discord"}}
	tests := []struct{ prefix, want string }{
		{"", notify.Text(n, admin)},
		{"[dev]", `\[dev\] ` + notify.Text(n, admin)},
	}
	for _, tt := range tests {
		if got := notify.Message(n, store.NotificationEndpoint{Prefix: tt.prefix}, admin); got != tt.want {
			t.Errorf("Message with prefix %q = %q, want %q", tt.prefix, got, tt.want)
		}
	}
	if got, want := notify.TestMessage(store.NotificationEndpoint{Prefix: "[dev]"}), `\[dev\] `+notify.TestText; got != want {
		t.Errorf("TestMessage = %q, want %q", got, want)
	}
}

// discord is a stub Discord webhook answering status, recording the body it was sent.
func discord(t *testing.T, status int, got *map[string]any) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("request = %s %s, want POST of application/json", r.Method, r.Header.Get("Content-Type"))
		}
		if err := json.Unmarshal(body, got); err != nil {
			t.Errorf("body %q is not JSON: %v", body, err)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestPostSendsTheMessageWithNoMentionsAllowed(t *testing.T) {
	var got map[string]any
	s := discord(t, http.StatusNoContent, &got)
	if err := notify.NewDiscord(s.Client()).Post(context.Background(), s.URL+"/api/webhooks/1/token", "hello @everyone"); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if got["content"] != "hello @everyone" {
		t.Errorf("content = %v, want the message", got["content"])
	}
	mentions, _ := json.Marshal(got["allowed_mentions"])
	if string(mentions) != `{"parse":[]}` {
		t.Errorf("allowed_mentions = %s, want {\"parse\":[]}", mentions)
	}
}

func TestPostCutsAMessageToDiscordsLimit(t *testing.T) {
	var got map[string]any
	s := discord(t, http.StatusNoContent, &got)
	if err := notify.NewDiscord(s.Client()).Post(context.Background(), s.URL, strings.Repeat("é", 2500)); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if n := len([]rune(got["content"].(string))); n != 2000 {
		t.Errorf("content is %d characters, want 2000", n)
	}
}

func TestPostFailsWithoutSayingTheURL(t *testing.T) {
	refusing := func(status int) string {
		var got map[string]any
		return discord(t, status, &got).URL + "/api/webhooks/1/secret-token"
	}
	tests := []struct {
		name, url  string
		wantStatus int
	}{
		{"a webhook that is gone", refusing(http.StatusNotFound), http.StatusNotFound},
		{"rate limited", refusing(http.StatusTooManyRequests), http.StatusTooManyRequests},
		{"Discord failing", refusing(http.StatusInternalServerError), http.StatusInternalServerError},
		{"nothing listening", "http://127.0.0.1:1/api/webhooks/1/secret-token", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := notify.NewDiscord(&http.Client{Timeout: 5 * time.Second}).Post(context.Background(), tt.url, "hello")
			if err == nil {
				t.Fatal("Post: nil, want an error")
			}
			if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "127.0.0.1") {
				t.Errorf("error %q holds the URL", err)
			}
			var refused *notify.RefusedError
			if got := errors.As(err, &refused); got != (tt.wantStatus != 0) || (got && refused.Status != tt.wantStatus) {
				t.Errorf("error = %v, want a RefusedError of status %d only when Discord answered", err, tt.wantStatus)
			}
		})
	}
}
