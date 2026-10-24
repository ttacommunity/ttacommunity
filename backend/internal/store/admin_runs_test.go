//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunsListsNewestFirstWithProgressAndTheCurrentRun(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	old := addRun(t, pool, "complete", time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC))
	makeCurrent(t, pool, old)
	running := addRun(t, pool, "running", time.Time{})
	if _, err := pool.Exec(ctx, "INSERT INTO run_progress (run_id, line) VALUES ($1, '[3/10] models 2p | elapsed 1m00s | eta 3m00s')", running); err != nil {
		t.Fatal(err)
	}

	got, err := NewAdminStore(asAdminLogin(t, pool)).Runs(ctx)
	if err != nil {
		t.Fatalf("Runs: %v", err)
	}
	if len(got) != 2 || got[0].ID != running || got[1].ID != old {
		t.Fatalf("runs = %+v, want %d then %d", got, running, old)
	}
	if got[0].Status != "running" || got[0].Progress != "[3/10] models 2p | elapsed 1m00s | eta 3m00s" || got[0].Current {
		t.Errorf("running run = %+v", got[0])
	}
	if !got[1].Current || got[1].PublishedAt == nil {
		t.Errorf("current run = %+v, want current and published", got[1])
	}
}

func TestPrewarmPlayerPagesLoadsARunsPagesForAdminOnly(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob")
	addGame(t, pool, run, "2026-01-01", "Duel", Seat{"ann", 150}, Seat{"bob", 100})

	blocks, err := NewAdminStore(asAdminLogin(t, pool)).PrewarmPlayerPages(ctx, run)
	if err != nil || blocks == 0 {
		t.Errorf("PrewarmPlayerPages(%d) = %d, %v; want blocks loaded", run, blocks, err)
	}
	if _, err := asReader(t, pool).Exec(ctx, "SELECT prewarm_player_pages($1)", run); err == nil {
		t.Error("the read login prewarmed a run, want permission denied")
	}
}

func TestPublishMakesACompleteRunCurrentAndRefusesOthers(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	a := NewAdminStore(asAdminLogin(t, pool))
	first := addRun(t, pool, "complete", time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC))
	makeCurrent(t, pool, first)
	second := addRun(t, pool, "complete", time.Time{})
	running := addRun(t, pool, "running", time.Time{})
	failed := addRun(t, pool, "failed", time.Time{})

	if err := a.Publish(ctx, second); err != nil {
		t.Fatalf("Publish(%d): %v", second, err)
	}
	runs, _ := a.Runs(ctx)
	for _, r := range runs {
		if (r.ID == second) != r.Current || (r.ID == second && r.PublishedAt == nil) {
			t.Errorf("after publishing %d, run %+v", second, r)
		}
	}
	if err := a.Publish(ctx, first); err != nil {
		t.Errorf("rolling back to %d: %v", first, err)
	}
	for _, id := range []int64{running, failed} {
		if err := a.Publish(ctx, id); !errors.Is(err, ErrRunNotComplete) {
			t.Errorf("Publish(%d) = %v, want ErrRunNotComplete", id, err)
		}
	}
	if err := a.Publish(ctx, 9999); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("Publish(9999) = %v, want ErrRunNotFound", err)
	}
}

func TestRunChangesCountsHowFarRatingsMoved(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	before := addRun(t, pool, "complete", time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC))
	after := addRun(t, pool, "complete", time.Time{})
	for _, r := range []struct {
		name          string
		before, after float64
	}{{"ann", 1500, 1500.5}, {"bob", 1500, 1520}, {"cat", 1500, 1560}, {"dan", 1500, 1380}, {"eve", 1500, 1500}} {
		addRating(t, pool, before, "glicko2_daneo", 0, r.name, nil, r.before)
		addRating(t, pool, after, "glicko2_daneo", 0, r.name, nil, r.after)
	}
	addRating(t, pool, before, "glicko2_daneo", 0, "gone", nil, 1500)
	addRating(t, pool, after, "glicko2_daneo", 0, "new", nil, 1500)

	got, err := NewAdminStore(asAdminLogin(t, pool)).RunChanges(ctx, after, before)
	if err != nil {
		t.Fatalf("RunChanges: %v", err)
	}
	if got.Ratings != 6 || got.RatingsBefore != 6 || got.New != 1 || got.Gone != 1 {
		t.Errorf("counts = %+v, want 6 and 6 ratings, 1 new, 1 gone", got)
	}
	if got.Moved1 != 3 || got.Moved10 != 3 || got.Moved50 != 2 || got.Moved100 != 1 {
		t.Errorf("moved = >1 %d, >10 %d, >50 %d, >100 %d; want 3, 3, 2, 1", got.Moved1, got.Moved10, got.Moved50, got.Moved100)
	}
	if len(got.Biggest) != 4 || got.Biggest[0].Name != "dan" || got.Biggest[0].Change != -120 {
		t.Errorf("biggest = %+v, want dan -120 first and the four who moved alone", got.Biggest)
	}
	if _, err := NewAdminStore(asAdminLogin(t, pool)).RunChanges(ctx, after, 9999); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("RunChanges against no run = %v, want ErrRunNotFound", err)
	}
}
