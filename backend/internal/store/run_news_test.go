//go:build integration

package store

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// newsSent sends what is due and returns each post as "endpoint kind run before".
func newsSent(t *testing.T, a *AdminStore) []string {
	t.Helper()
	var got []string
	if _, err := a.SendNotifications(context.Background(), time.Now().Add(time.Hour), 50, func(_ context.Context, n Notification, e NotificationEndpoint) error {
		got = append(got, fmt.Sprintf("%s %s %d %d", e.Name, n.Kind, n.Run.ID, n.Run.BeforeID))
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	slices.Sort(got)
	return got
}

// runOf adds a complete, unpublished run of the ranking with slug, with games games.
func runOf(t *testing.T, pool *pgxpool.Pool, slug string, games int) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `INSERT INTO rating_runs (engine_version, input_fingerprint, status, ranking_id, games)
		SELECT 'v1', 'f1', 'complete', ranking_id, $2 FROM rankings WHERE slug = $1 RETURNING run_id`, slug, games).Scan(&id); err != nil {
		t.Fatalf("adding a %s run: %v", slug, err)
	}
	return id
}

func publish(t *testing.T, a *AdminStore, run int64) {
	t.Helper()
	if err := a.Publish(context.Background(), run); err != nil {
		t.Fatalf("Publish(%d): %v", run, err)
	}
}

func TestPublishingACommunityRunQueuesItsNewsOnce(t *testing.T) {
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	endpoint(t, a, "Runs", 1, "ratings_run")
	endpoint(t, a, "Top", 2, "ratings_top10")
	first, second := runOf(t, pool, "community", 100), runOf(t, pool, "community", 120)

	publish(t, a, first)
	if got, want := newsSent(t, a), []string{fmt.Sprintf("Runs ratings_run %d 0", first)}; !slices.Equal(got, want) {
		t.Errorf("the first run ever = %q, want %q: no top 10 to compare with", got, want)
	}
	publish(t, a, second)
	want := []string{fmt.Sprintf("Runs ratings_run %d %d", second, first), fmt.Sprintf("Top ratings_top10 %d %d", second, first)}
	if got := newsSent(t, a); !slices.Equal(got, want) {
		t.Errorf("a new run = %q, want %q", got, want)
	}
	publish(t, a, first)
	publish(t, a, second)
	if got := newsSent(t, a); len(got) != 0 {
		t.Errorf("taking a run back and publishing it again = %q, want nothing", got)
	}
}

func TestPublishingAnotherRankingsRunQueuesNothing(t *testing.T) {
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	endpoint(t, a, "Runs", 1, "ratings_run", "ratings_top10")
	publish(t, a, runOf(t, pool, "superblitz", 10))
	publish(t, a, runOf(t, pool, "superblitz", 12))
	if got := newsSent(t, a); len(got) != 0 {
		t.Errorf("superblitz runs = %q, want nothing", got)
	}
}

func TestARunDeletedBeforeItsNewsIsSentPostsNothing(t *testing.T) {
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	endpoint(t, a, "Runs", 1, "ratings_run", "ratings_top10")
	first, second := runOf(t, pool, "community", 100), runOf(t, pool, "community", 120)
	publish(t, a, first)
	newsSent(t, a)
	publish(t, a, second)
	publish(t, a, first)
	if _, err := pool.Exec(context.Background(), "DELETE FROM rating_runs WHERE run_id = $1", second); err != nil {
		t.Fatalf("deleting run %d: %v", second, err)
	}
	if got := newsSent(t, a); len(got) != 0 {
		t.Errorf("sent = %q, want nothing of a deleted run", got)
	}
}

func TestARunsNewsCountsTitlesGainedAndLost(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	a := NewAdminStore(asAdminLogin(t, pool))
	endpoint(t, a, "Runs", 1, "ratings_run")
	before, after := runOf(t, pool, "community", 100), runOf(t, pool, "community", 120)
	for _, p := range []struct{ name, was, now string }{
		{"up", "W", "GM"},
		{"first title", "", "B"},
		{"world champion", "SGM", "WC"},
		{"down", "GM", "M"},
		{"title gone", "P", ""},
		{"same", "G", "G"},
		{"hidden, up", "W", "GM"},
	} {
		if p.was != "" {
			addTitle(t, pool, before, p.name, p.was, 1, "test")
		}
		if p.now != "" {
			addTitle(t, pool, after, p.name, p.now, 1, "test")
		}
	}
	if _, err := pool.Exec(ctx, "INSERT INTO player_accounts (cge_user, player_id, name, banned) VALUES (100001, $1, 'hidden, up', true)",
		playerID(t, pool, "hidden, up")); err != nil {
		t.Fatalf("banning a player: %v", err)
	}
	publish(t, a, before)
	newsSent(t, a)
	publish(t, a, after)
	var got []RunNotice
	if _, err := a.SendNotifications(ctx, time.Now().Add(time.Hour), 50, func(_ context.Context, n Notification, _ NotificationEndpoint) error {
		got = append(got, n.Run)
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	if len(got) != 1 || got[0].Games != 120 || got[0].TitlesGained != 3 || got[0].TitlesLost != 2 {
		t.Errorf("news = %+v, want 120 games, 3 titles gained (up, first title, world champion) and 2 lost (down, title gone)", got)
	}
}

func TestMigration97DropsOnlyRunNewsGoingDown(t *testing.T) {
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	endpoint(t, a, "Both", 1, "sign_up", "ratings_run", "ratings_top10")
	publish(t, a, runOf(t, pool, "community", 100))
	migrateTo(t, pool, 96, true)
	if got := rowsOf(t, pool, "SELECT array_to_string(events, ',') FROM notification_endpoints"); !slices.Equal(got, []string{"sign_up"}) {
		t.Errorf("events at 96 = %q, want only sign_up", got)
	}
	if got := rowsOf(t, pool, "SELECT kind FROM notification_outbox"); len(got) != 0 {
		t.Errorf("outbox at 96 = %q, want the run's news gone", got)
	}
	migrateTo(t, pool, 97, false)
}
