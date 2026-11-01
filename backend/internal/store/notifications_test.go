//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// sendsAt is when the tests' outbox is sent, after the reports.
var sendsAt = reportsAt.Add(time.Minute)

// webhook is an invented Discord webhook URL, numbered n.
func webhook(n int) string {
	return fmt.Sprintf("https://discord.com/api/webhooks/10000000000000000%d/token-%d", n, n)
}

// endpoint adds an endpoint named name taking events, and returns its id.
func endpoint(t *testing.T, a *AdminStore, name string, n int, events ...string) int64 {
	t.Helper()
	id, err := a.AddNotificationEndpoint(context.Background(), NotificationEndpoint{Name: name, URL: webhook(n), Format: "discord", Events: events}, reportsAt)
	if err != nil {
		t.Fatalf("AddNotificationEndpoint(%s): %v", name, err)
	}
	return id
}

// outboxFixture is reportsFixture with its link requests already answered and sent, the admin store on the admin
// login, and one endpoint taking every event.
func outboxFixture(t *testing.T) (*AdminStore, *PlayerStore, []int64) {
	t.Helper()
	a, p, _, _, matches := reportsFixture(t)
	admin := NewAdminStore(asAdminLogin(t, a.pool.(*pgxpool.Pool)))
	endpoint(t, admin, "Everything", 1, "link_request", "blitz_report", "blitz_flag")
	if got := sent(t, admin, sendsAt); len(got) != 0 {
		t.Fatalf("the fixture's answered requests sent %q, want nothing", got)
	}
	return admin, p, matches
}

// sent sends the outbox's due notifications at now and returns each as notificationLine writes it.
func sent(t *testing.T, a *AdminStore, now time.Time) []string {
	t.Helper()
	var got []string
	if _, err := a.SendNotifications(context.Background(), now, 10, func(_ context.Context, n Notification, _ NotificationEndpoint) error {
		got = append(got, notificationLine(n))
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	return got
}

// notificationLine writes what a notification tells, without its id.
func notificationLine(n Notification) string {
	if n.Kind == "link_request" {
		return fmt.Sprintf("%s name=%s sign-ins=%s", n.Kind, n.Name, strings.Join(n.SignIns, ","))
	}
	return fmt.Sprintf("%s %s/%s %q round=%d %s v %s result=%s by=%s corrected=%t flag=%s", n.Kind, n.Slug, n.Edition, n.Series,
		n.Round, n.A, n.B, n.Result, n.ReportedBy, n.Corrected, n.FlagReason)
}

func TestPlayersRequestsAndReportsAreNotified(t *testing.T) {
	ctx := context.Background()
	report := func(code string, g BlitzGame) func(t *testing.T, p *PlayerStore, matches []int64) {
		return func(t *testing.T, p *PlayerStore, matches []int64) {
			g.ShareCode = code
			if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], g, reportsAt); err != nil {
				t.Fatalf("ReportBlitzGame(%s): %v", code, err)
			}
		}
	}
	annWins, bobWins := report("REP-1", reported("")), report("REP-2", BlitzGame{A: finished(40, 1), B: finished(55, 2), Finished: finishedAt})
	tests := []struct {
		name  string
		steps []func(t *testing.T, p *PlayerStore, matches []int64)
		want  []string
	}{
		{"a report", []func(*testing.T, *PlayerStore, []int64){annWins},
			[]string{`blitz_report us_open/2026 "US Open" round=1 Ann v Bob result=a by=Ann corrected=false flag=`}},
		{"a report corrected before it is sent", []func(*testing.T, *PlayerStore, []int64){annWins, bobWins},
			[]string{`blitz_report us_open/2026 "US Open" round=1 Ann v Bob result=b by=Ann corrected=true flag=`}},
		{"a flagged report", []func(*testing.T, *PlayerStore, []int64){annWins, func(t *testing.T, p *PlayerStore, matches []int64) {
			if err := p.FlagBlitzResult(ctx, tokenHash("bob"), "us_open", "2026", matches[0], "I won on culture", reportsAt); err != nil {
				t.Fatalf("FlagBlitzResult: %v", err)
			}
		}}, []string{
			`blitz_report us_open/2026 "US Open" round=1 Ann v Bob result=a by=Ann corrected=false flag=I won on culture`,
			`blitz_flag us_open/2026 "US Open" round=1 Ann v Bob result=a by=Ann corrected=false flag=I won on culture`,
		}},
		{"a confirmed report, the confirmation not", []func(*testing.T, *PlayerStore, []int64){annWins, func(t *testing.T, p *PlayerStore, matches []int64) {
			if err := p.ConfirmBlitzResult(ctx, tokenHash("bob"), "us_open", "2026", matches[0], reportsAt); err != nil {
				t.Fatalf("ConfirmBlitzResult: %v", err)
			}
		}}, []string{`blitz_report us_open/2026 "US Open" round=1 Ann v Bob result=a by=Ann corrected=false flag=`}},
		{"a request to be linked", []func(*testing.T, *PlayerStore, []int64){func(t *testing.T, p *PlayerStore, _ []int64) {
			l := signedIn(t, p, discordID("100000000000000005", "eve", "Eve"), "eve")
			if err := p.RequestLink(ctx, l.ID, "  Eve_1 ", reportsAt); err != nil {
				t.Fatalf("RequestLink: %v", err)
			}
			if err := p.RequestLink(ctx, l.ID, "Eve_2", reportsAt); err != nil {
				t.Fatalf("RequestLink(changed): %v", err)
			}
		}}, []string{"link_request name=Eve_2 sign-ins=discord"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, p, matches := outboxFixture(t)
			for _, step := range tt.steps {
				step(t, p, matches)
			}
			if got := sent(t, a, sendsAt); !slices.Equal(got, tt.want) {
				t.Errorf("sent = %q, want %q", got, tt.want)
			}
			if got := sent(t, a, sendsAt.Add(time.Hour)); len(got) != 0 {
				t.Errorf("sent again = %q, want nothing", got)
			}
		})
	}
}

func TestRefusedRequestsAndReportsAreNotNotified(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		refused func(p *PlayerStore, matches []int64) error
	}{
		{"a report of someone else's match", func(p *PlayerStore, matches []int64) error {
			return p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[1], reported("REP-1"), reportsAt)
		}},
		{"a report without a share code", func(p *PlayerStore, matches []int64) error {
			return p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], reported(""), reportsAt)
		}},
		{"a flag of a match not reported", func(p *PlayerStore, matches []int64) error {
			return p.FlagBlitzResult(ctx, tokenHash("bob"), "us_open", "2026", matches[0], "wrong", reportsAt)
		}},
		{"a linked player asking to be linked", func(p *PlayerStore, _ []int64) error {
			l, err := p.Session(ctx, tokenHash("ann"), reportsAt)
			if err != nil {
				return fmt.Errorf("Session: %w", err)
			}
			return p.RequestLink(ctx, l.ID, "Someone", reportsAt)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, p, matches := outboxFixture(t)
			if err := tt.refused(p, matches); err == nil {
				t.Fatal("the request was taken, want it refused")
			}
			if got := sent(t, a, sendsAt); len(got) != 0 {
				t.Errorf("sent = %q, want nothing", got)
			}
		})
	}
}

func TestANotificationAnsweredBeforeItIsSentIsDropped(t *testing.T) {
	a, p, _ := outboxFixture(t)
	ctx := context.Background()
	l := signedIn(t, p, discordID("100000000000000005", "eve", "Eve"), "eve")
	if err := p.RequestLink(ctx, l.ID, "Eve", reportsAt); err != nil {
		t.Fatalf("RequestLink: %v", err)
	}
	if err := a.RefuseLink(ctx, pendingRequest(t, a, l), "not a TTA player", reportsAt); err != nil {
		t.Fatalf("RefuseLink: %v", err)
	}
	if got := sent(t, a, sendsAt); len(got) != 0 {
		t.Errorf("sent = %q, want nothing for an answered request", got)
	}
}

func TestAFailedNotificationIsTriedAgainLaterThenDropped(t *testing.T) {
	a, p, matches := outboxFixture(t)
	ctx := context.Background()
	if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); err != nil {
		t.Fatalf("ReportBlitzGame: %v", err)
	}
	failing := func(context.Context, Notification, NotificationEndpoint) error { return errors.New("Discord is down") }
	at, tries := sendsAt, 0
	for ; tries < 50; tries++ {
		got, err := a.SendNotifications(ctx, at, 10, failing)
		if err != nil {
			t.Fatalf("SendNotifications: %v", err)
		}
		if got.Failed == 0 {
			break
		}
		if got := sent(t, a, at); len(got) != 0 {
			t.Fatalf("try %d: sent again at once = %q, want nothing until later", tries+1, got)
		}
		at = at.Add(2 * time.Hour)
	}
	if tries != MaxNotificationAttempts {
		t.Errorf("failed %d times, want %d before it is dropped", tries, MaxNotificationAttempts)
	}
	if got := sent(t, a, at.Add(24*time.Hour)); len(got) != 0 {
		t.Errorf("sent after it was dropped = %q, want nothing", got)
	}
}

func TestAFailedNotificationIsSentOnTheNextTry(t *testing.T) {
	a, p, matches := outboxFixture(t)
	ctx := context.Background()
	if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); err != nil {
		t.Fatalf("ReportBlitzGame: %v", err)
	}
	got, err := a.SendNotifications(ctx, sendsAt, 10, func(context.Context, Notification, NotificationEndpoint) error { return errors.New("Discord is down") })
	if err != nil || got != (NotificationsSent{Failed: 1}) {
		t.Fatalf("SendNotifications(failing) = %+v, %v; want 1 failed", got, err)
	}
	if got := sent(t, a, sendsAt.Add(2*time.Hour)); len(got) != 1 {
		t.Errorf("sent on the next try = %q, want the report", got)
	}
}

func TestANotificationBeingSentIsNotSentTwice(t *testing.T) {
	a, p, matches := outboxFixture(t)
	ctx := context.Background()
	if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); err != nil {
		t.Fatalf("ReportBlitzGame: %v", err)
	}
	var meanwhile []string
	got, err := a.SendNotifications(ctx, sendsAt, 10, func(context.Context, Notification, NotificationEndpoint) error {
		meanwhile = sent(t, a, sendsAt)
		return nil
	})
	if err != nil || got != (NotificationsSent{Sent: 1}) {
		t.Fatalf("SendNotifications = %+v, %v; want 1 sent", got, err)
	}
	if len(meanwhile) != 0 {
		t.Errorf("a second sender meanwhile sent %q, want nothing", meanwhile)
	}
}

func TestEachEndpointGetsTheEventsItTakes(t *testing.T) {
	a, p, matches := outboxFixture(t)
	ctx := context.Background()
	if err := a.RemoveNotificationEndpoint(ctx, mustEndpointID(t, a, "Everything")); err != nil {
		t.Fatalf("RemoveNotificationEndpoint: %v", err)
	}
	endpoint(t, a, "Admins", 2, "link_request")
	endpoint(t, a, "Organisers", 3, "blitz_report", "blitz_flag")
	endpoint(t, a, "Flags", 4, "blitz_flag")
	l := signedIn(t, p, discordID("100000000000000005", "eve", "Eve"), "eve")
	if err := p.RequestLink(ctx, l.ID, "Eve", reportsAt); err != nil {
		t.Fatalf("RequestLink: %v", err)
	}
	if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); err != nil {
		t.Fatalf("ReportBlitzGame: %v", err)
	}
	if err := p.FlagBlitzResult(ctx, tokenHash("bob"), "us_open", "2026", matches[0], "wrong", reportsAt); err != nil {
		t.Fatalf("FlagBlitzResult: %v", err)
	}
	var got []string
	if _, err := a.SendNotifications(ctx, sendsAt, 10, func(_ context.Context, n Notification, e NotificationEndpoint) error {
		got = append(got, n.Kind+" to "+e.Name)
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	if want := []string{"link_request to Admins", "blitz_report to Organisers", "blitz_flag to Organisers", "blitz_flag to Flags"}; !slices.Equal(got, want) {
		t.Errorf("sent = %q, want %q", got, want)
	}
}

func TestANotificationNoEndpointTakesIsDropped(t *testing.T) {
	a, p, matches := outboxFixture(t)
	ctx := context.Background()
	if err := a.RemoveNotificationEndpoint(ctx, mustEndpointID(t, a, "Everything")); err != nil {
		t.Fatalf("RemoveNotificationEndpoint: %v", err)
	}
	if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); err != nil {
		t.Fatalf("ReportBlitzGame: %v", err)
	}
	if got := sent(t, a, sendsAt); len(got) != 0 {
		t.Fatalf("sent with no endpoint = %q, want nothing", got)
	}
	endpoint(t, a, "Late", 2, "blitz_report")
	if got := sent(t, a, sendsAt); len(got) != 0 {
		t.Errorf("sent to an endpoint added later = %q, want nothing: it was dropped", got)
	}
}

func TestARetryPostsOnlyToTheEndpointsThatFailed(t *testing.T) {
	a, p, matches := outboxFixture(t)
	ctx := context.Background()
	endpoint(t, a, "Down", 2, "blitz_report")
	if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], reported("REP-1"), reportsAt); err != nil {
		t.Fatalf("ReportBlitzGame: %v", err)
	}
	var got []string
	send := func(down bool) func(context.Context, Notification, NotificationEndpoint) error {
		return func(_ context.Context, _ Notification, e NotificationEndpoint) error {
			got = append(got, e.Name)
			if down && e.Name == "Down" {
				return errors.New("unreachable")
			}
			return nil
		}
	}
	if done, err := a.SendNotifications(ctx, sendsAt, 10, send(true)); err != nil || done != (NotificationsSent{Failed: 1}) {
		t.Fatalf("SendNotifications(one down) = %+v, %v; want 1 failed", done, err)
	}
	if done, err := a.SendNotifications(ctx, sendsAt.Add(time.Hour), 10, send(false)); err != nil || done != (NotificationsSent{Sent: 1}) {
		t.Fatalf("SendNotifications(again) = %+v, %v; want 1 sent", done, err)
	}
	if want := []string{"Everything", "Down", "Down"}; !slices.Equal(got, want) {
		t.Errorf("posted to %q, want %q", got, want)
	}
}

// mustEndpointID is the id of the endpoint named name.
func mustEndpointID(t *testing.T, a *AdminStore, name string) int64 {
	t.Helper()
	list, err := a.NotificationEndpoints(context.Background())
	if err != nil {
		t.Fatalf("NotificationEndpoints: %v", err)
	}
	for _, e := range list {
		if e.Name == name {
			return e.ID
		}
	}
	t.Fatalf("no endpoint named %s in %+v", name, list)
	return 0
}

func TestAdminKeepsNotificationEndpoints(t *testing.T) {
	a, _, _ := outboxFixture(t)
	ctx := context.Background()
	id, err := a.AddNotificationEndpoint(ctx, NotificationEndpoint{Name: "  Admins ", URL: " " + webhook(2) + " ", Format: "discord",
		Events: []string{"link_request", "blitz_flag", "link_request"}, Prefix: " [dev] "}, reportsAt)
	if err != nil {
		t.Fatalf("AddNotificationEndpoint: %v", err)
	}
	got, err := a.NotificationEndpoint(ctx, id)
	if err != nil {
		t.Fatalf("NotificationEndpoint: %v", err)
	}
	if line := fmt.Sprintf("%s %s %q %s", got.Name, got.URL, got.Events, got.Prefix); line != `Admins `+webhook(2)+` ["blitz_flag" "link_request"] [dev]` {
		t.Errorf("the endpoint = %s, want it trimmed, its events once each", line)
	}
	got.Events, got.Name = []string{}, "Nobody"
	if err := a.SetNotificationEndpoint(ctx, got); err != nil {
		t.Fatalf("SetNotificationEndpoint: %v", err)
	}
	if got, err := a.NotificationEndpoint(ctx, id); err != nil || got.Name != "Nobody" || len(got.Events) != 0 {
		t.Errorf("after the change = %+v, %v; want Nobody with no events", got, err)
	}
	if err := a.RemoveNotificationEndpoint(ctx, id); err != nil {
		t.Fatalf("RemoveNotificationEndpoint: %v", err)
	}
	if _, err := a.NotificationEndpoint(ctx, id); !errors.Is(err, ErrNoNotificationEndpoint) {
		t.Errorf("NotificationEndpoint after removing: %v, want ErrNoNotificationEndpoint", err)
	}
	for name, err := range map[string]error{
		"SetNotificationEndpoint":    a.SetNotificationEndpoint(ctx, NotificationEndpoint{ID: id, Name: "x", URL: webhook(2), Format: "discord"}),
		"RemoveNotificationEndpoint": a.RemoveNotificationEndpoint(ctx, id),
	} {
		if !errors.Is(err, ErrNoNotificationEndpoint) {
			t.Errorf("%s of a removed endpoint: %v, want ErrNoNotificationEndpoint", name, err)
		}
	}
}

func TestTheDatabaseRefusesABadNotificationEndpoint(t *testing.T) {
	good := NotificationEndpoint{Name: "Admins", URL: webhook(2), Format: "discord", Events: []string{"link_request"}}
	tests := []struct {
		name   string
		change func(e *NotificationEndpoint)
	}{
		{"no name", func(e *NotificationEndpoint) { e.Name = "  " }},
		{"a name over 100 characters", func(e *NotificationEndpoint) { e.Name = strings.Repeat("a", 101) }},
		{"a URL that is not Discord's", func(e *NotificationEndpoint) { e.URL = "https://example.com/api/webhooks/100000000000000002/token" }},
		{"a Discord URL over http", func(e *NotificationEndpoint) { e.URL = strings.Replace(webhook(2), "https", "http", 1) }},
		{"a Discord URL that is not a webhook", func(e *NotificationEndpoint) { e.URL = "https://discord.com/channels/100000000000000002/2" }},
		{"a Discord webhook URL with more after it", func(e *NotificationEndpoint) { e.URL = webhook(2) + "/../x" }},
		{"another format", func(e *NotificationEndpoint) { e.Format = "slack" }},
		{"no format", func(e *NotificationEndpoint) { e.Format = "" }},
		{"an unknown event", func(e *NotificationEndpoint) { e.Events = []string{"link_request", "pizza"} }},
		{"a prefix over 20 characters", func(e *NotificationEndpoint) { e.Prefix = strings.Repeat("a", 21) }},
	}
	a, _, _ := outboxFixture(t)
	ctx := context.Background()
	id := endpoint(t, a, "Kept", 3, "link_request")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := good
			tt.change(&e)
			if _, err := a.AddNotificationEndpoint(ctx, e, reportsAt); !errors.Is(err, ErrBadNotificationEndpoint) {
				t.Errorf("AddNotificationEndpoint: %v, want ErrBadNotificationEndpoint", err)
			}
			e.ID = id
			if err := a.SetNotificationEndpoint(ctx, e); !errors.Is(err, ErrBadNotificationEndpoint) {
				t.Errorf("SetNotificationEndpoint: %v, want ErrBadNotificationEndpoint", err)
			}
		})
	}
}
