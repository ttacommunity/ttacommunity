//go:build integration

package store

import (
	"context"
	"slices"
	"testing"
	"time"
)

// remindersQueued queues the reminders due at at and returns every reminder in the outbox as "endpoint round".
func remindersQueued(t *testing.T, a *AdminStore, at time.Time) []string {
	t.Helper()
	if _, err := a.QueueBlitzReminders(context.Background(), at); err != nil {
		t.Fatalf("QueueBlitzReminders: %v", err)
	}
	return rowsOf(t, a.snapshot, `SELECT e.name || ' ' || r.number FROM notification_outbox o
		JOIN notification_endpoints e ON e.endpoint_id = o.endpoint_id JOIN blitz_rounds r USING (round_id)
		WHERE o.kind = 'blitz_reminder' ORDER BY 1`)
}

// remindedBy adds an endpoint name taking blitz reminders hours before a deadline.
func remindedBy(t *testing.T, a *AdminStore, name string, n, hours int) {
	t.Helper()
	id := endpoint(t, a, name, n, "blitz_reminder")
	if _, err := a.pool.Exec(context.Background(), "UPDATE notification_endpoints SET blitz_reminder_hours = $2 WHERE endpoint_id = $1", id, hours); err != nil {
		t.Fatalf("setting %s's hours: %v", name, err)
	}
}

// roundDue publishes round 1, Ann against Bob and Cy against Dee, with its deadline at deadline.
func roundDue(t *testing.T, a *AdminStore, ids []int64, deadline time.Time) int64 {
	t.Helper()
	id := publishedRound(t, a, [2]int64{ids[0], ids[1]}, [2]int64{ids[2], ids[3]})
	if err := a.SetBlitzRoundDeadline(context.Background(), "us_open", "2026", id, deadline, "UTC"); err != nil {
		t.Fatalf("SetBlitzRoundDeadline: %v", err)
	}
	return id
}

func TestAReminderIsQueuedOnceForEachEndpointAtItsHours(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	now := time.Now()
	remindedBy(t, a, "Day", 1, 24)
	remindedBy(t, a, "Two days", 2, 48)
	endpoint(t, a, "Rounds only", 3, "blitz_round")
	roundDue(t, a, ids, now.Add(60*time.Hour))
	steps := []struct {
		name string
		at   time.Time
		want []string
	}{
		{"before either endpoint's hours", now, nil},
		{"within two days", now.Add(13 * time.Hour), []string{"Two days 1"}},
		{"a pass later", now.Add(14 * time.Hour), []string{"Two days 1"}},
		{"within a day", now.Add(37 * time.Hour), []string{"Day 1", "Two days 1"}},
		{"past the deadline", now.Add(61 * time.Hour), []string{"Day 1", "Two days 1"}},
	}
	for _, s := range steps {
		if got := remindersQueued(t, a, s.at); !slices.Equal(got, s.want) {
			t.Errorf("%s: reminders = %q, want %q", s.name, got, s.want)
		}
	}
}

func TestNoReminderIsQueuedWhenItWouldTellNothing(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, a *AdminStore, round int64)
	}{
		{"every game played", func(t *testing.T, a *AdminStore, round int64) {
			ed, _ := a.BlitzEdition(context.Background(), "us_open", "2026")
			for _, m := range ed.Rounds[0].Matches {
				if err := a.RecordBlitzRuling(context.Background(), "us_open", "2026", m.ID, "a", "played"); err != nil {
					t.Fatalf("RecordBlitzRuling: %v", err)
				}
			}
		}},
		{"the tournament hidden since", func(t *testing.T, a *AdminStore, _ int64) {
			hidden := false
			if err := a.SetTournament(context.Background(), "us_open", TournamentSettings{Visible: &hidden}); err != nil {
				t.Fatalf("SetTournament: %v", err)
			}
		}},
		{"the endpoint paused", func(t *testing.T, a *AdminStore, _ int64) {
			if _, err := a.pool.Exec(context.Background(), "UPDATE notification_endpoints SET paused = true"); err != nil {
				t.Fatalf("pausing: %v", err)
			}
		}},
		{"the round taken back", func(t *testing.T, a *AdminStore, round int64) { deleteRound(t, a, round) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _, ids := blitzWithEntrants(t)
			now := time.Now()
			remindedBy(t, a, "Day", 1, 24)
			round := roundDue(t, a, ids, now.Add(30*time.Hour))
			tt.setup(t, a, round)
			if got := remindersQueued(t, a, now.Add(7*time.Hour)); len(got) != 0 {
				t.Errorf("reminders = %q, want none", got)
			}
		})
	}
}

func TestARoundPublishedWithinTheHoursIsNotReminded(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	now := time.Now()
	remindedBy(t, a, "Day", 1, 24)
	roundDue(t, a, ids, now.Add(10*time.Hour))
	if got := remindersQueued(t, a, now.Add(time.Hour)); len(got) != 0 {
		t.Errorf("reminders = %q, want none: its round post went out within the day", got)
	}
}

func TestAReminderIsSentOnlyToItsEndpoint(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	now := time.Now()
	remindedBy(t, a, "Day", 1, 24)
	endpoint(t, a, "Everything blitz", 2, "blitz_round", "blitz_finished")
	roundDue(t, a, ids, now.Add(30*time.Hour))
	blitzSent(t, a)
	remindersQueued(t, a, now.Add(7*time.Hour))
	var got []string
	if _, err := a.SendNotifications(context.Background(), now.Add(8*time.Hour), 50, func(_ context.Context, n Notification, e NotificationEndpoint) error {
		got = append(got, e.Name+" "+n.Kind)
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	if want := []string{"Day blitz_reminder"}; !slices.Equal(got, want) {
		t.Errorf("sent = %q, want %q", got, want)
	}
}

func TestMigration100DropsOnlyRemindersGoingDown(t *testing.T) {
	pool, a := blitzFixture(t)
	endpoint(t, a, "Both", 1, "blitz_round", "blitz_reminder")
	migrateTo(t, pool, 99, true)
	if got := rowsOf(t, pool, "SELECT array_to_string(events, ',') FROM notification_endpoints"); !slices.Equal(got, []string{"blitz_round"}) {
		t.Errorf("events at 99 = %q, want only blitz_round", got)
	}
	migrateTo(t, pool, 100, false)
}
