//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// withRatings gives a run a few result rows, so deleting it has something to remove and sizing something to count.
func withRatings(t *testing.T, pool *pgxpool.Pool, run int64, n int) {
	t.Helper()
	for i := range n {
		if _, err := pool.Exec(context.Background(), `
			INSERT INTO player_ratings (run_id, model_type, player_count, player_name, rating, rd, sigma, c_rating, rank,
			                            games_played, opponents_count, wins, losses, draws, win_rate)
			VALUES ($1, 'glicko2', 0, $2, 1500, 50, 0.06, 1350, $3, 1, 1, 1, 0, 0, 1)`, run, "player"+string(rune('a'+i)), i+1); err != nil {
			t.Fatalf("adding ratings: %v", err)
		}
	}
}

func TestProtectingARunKeepsItFromDeletion(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	a := NewAdminStore(asAdminLogin(t, pool))
	run := addRun(t, pool, "complete", time.Time{})

	if err := a.SetRunProtected(ctx, run, true); err != nil {
		t.Fatalf("SetRunProtected: %v", err)
	}
	if err := a.StartDeleteRun(ctx, run, false); !errors.Is(err, ErrRunProtected) {
		t.Errorf("StartDeleteRun of a protected run = %v, want ErrRunProtected", err)
	}
	if err := a.SetRunProtected(ctx, run, false); err != nil {
		t.Fatalf("unprotecting: %v", err)
	}
	if err := a.StartDeleteRun(ctx, run, false); err != nil {
		t.Errorf("StartDeleteRun once unprotected = %v, want nil", err)
	}
	runs, _ := a.Runs(ctx)
	if len(runs) != 1 || !runs[0].Deleting || runs[0].Protected {
		t.Errorf("runs = %+v, want the run marked deleting", runs)
	}
	if err := a.SetRunProtected(ctx, 999, true); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("SetRunProtected of no run = %v, want ErrRunNotFound", err)
	}
}

func TestStartDeleteRunRefusesTheCurrentAndARunningRun(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	a := NewAdminStore(asAdminLogin(t, pool))
	current := addRun(t, pool, "complete", time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC))
	makeCurrent(t, pool, current)
	running := addRun(t, pool, "running", time.Time{})

	if err := a.StartDeleteRun(ctx, current, false); !errors.Is(err, ErrRunCurrent) {
		t.Errorf("deleting the current run = %v, want ErrRunCurrent", err)
	}
	if err := a.StartDeleteRun(ctx, running, true); !errors.Is(err, ErrRunRunning) {
		t.Errorf("deleting a run while the engine runs = %v, want ErrRunRunning", err)
	}
	// The engine is not going: the run crashed and stays "running" in the database; it can go.
	if err := a.StartDeleteRun(ctx, running, false); err != nil {
		t.Errorf("deleting a crashed run = %v, want nil", err)
	}
	if err := a.StartDeleteRun(ctx, 999, false); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("deleting no run = %v, want ErrRunNotFound", err)
	}
}

func TestDeleteRunRemovesTheRunAndItsResults(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	a := NewAdminStore(asAdminLogin(t, pool))
	keep := addRun(t, pool, "complete", time.Time{})
	gone := addRun(t, pool, "complete", time.Time{})
	withRatings(t, pool, keep, 2)
	withRatings(t, pool, gone, 3)

	if err := a.DeleteRun(ctx, gone); !errors.Is(err, ErrRunNotDeleting) {
		t.Errorf("DeleteRun before StartDeleteRun = %v, want ErrRunNotDeleting", err)
	}
	if err := a.StartDeleteRun(ctx, gone, false); err != nil {
		t.Fatalf("StartDeleteRun: %v", err)
	}
	if n := len(runPartitions(t, pool, gone)); n == 0 {
		t.Fatalf("run %d has no partitions before deleting: nothing to prove", gone)
	}
	if err := a.DeleteRun(ctx, gone); err != nil {
		t.Fatalf("DeleteRun: %v", err)
	}
	var runs, ratingsKept, ratingsGone int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM rating_runs),
		(SELECT count(*) FROM player_ratings WHERE run_id = $1), (SELECT count(*) FROM player_ratings WHERE run_id = $2)`,
		keep, gone).Scan(&runs, &ratingsKept, &ratingsGone); err != nil {
		t.Fatal(err)
	}
	if runs != 1 || ratingsKept != 2 || ratingsGone != 0 {
		t.Errorf("after deleting: %d runs, %d kept ratings, %d of the deleted run's; want 1, 2, 0", runs, ratingsKept, ratingsGone)
	}
	if left := runPartitions(t, pool, gone); len(left) != 0 {
		t.Errorf("the deleted run's partitions left = %v, want none", left)
	}
	if kept := runPartitions(t, pool, keep); len(kept) == 0 {
		t.Errorf("the kept run's partitions = none, want them kept")
	}
}

// runPartitions lists the partitions that hold run's rows.
func runPartitions(t *testing.T, pool *pgxpool.Pool, run int64) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT c.relname::text FROM pg_class c
		WHERE c.relispartition AND pg_get_expr(c.relpartbound, c.oid) = format('FOR VALUES IN (%L)', $1::bigint)`, run)
	if err != nil {
		t.Fatalf("listing run %d's partitions: %v", run, err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("listing run %d's partitions: %v", run, err)
	}
	return names
}

func TestDeleteRunRefusesARunProtectedSinceItWasMarked(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	a := NewAdminStore(asAdminLogin(t, pool))
	run := addRun(t, pool, "complete", time.Time{})
	if err := a.StartDeleteRun(ctx, run, false); err != nil {
		t.Fatalf("StartDeleteRun: %v", err)
	}
	if err := a.SetRunProtected(ctx, run, true); err != nil {
		t.Fatalf("protecting a run marked for deletion: %v", err)
	}
	// Protecting it unmarks it, so the background delete finds nothing to do.
	if err := a.DeleteRun(ctx, run); !errors.Is(err, ErrRunNotDeleting) {
		t.Errorf("DeleteRun of a run protected after marking = %v, want ErrRunNotDeleting", err)
	}
	runs, _ := a.Runs(ctx)
	if len(runs) != 1 || runs[0].Deleting {
		t.Errorf("runs = %+v, want the run kept and no longer deleting", runs)
	}
}

func TestARunsSizeIsTheOneRecordedAsItEndedWhateverIsDeletedSince(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	a := NewAdminStore(asAdminLogin(t, pool))
	kept := addRun(t, pool, "complete", time.Time{})
	gone := addRun(t, pool, "complete", time.Time{})
	for run, n := range map[int64]int{kept: 1, gone: 3} {
		withRatings(t, pool, run, n)
		if _, err := pool.Exec(ctx, "UPDATE rating_runs SET result_bytes = $2, disk_bytes = $3 WHERE run_id = $1", run, 1000*n, 3000*n); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.StartDeleteRun(ctx, gone, false); err != nil {
		t.Fatalf("StartDeleteRun: %v", err)
	}
	if err := a.DeleteRun(ctx, gone); err != nil {
		t.Fatalf("DeleteRun: %v", err)
	}

	runs, err := a.Runs(ctx)
	if err != nil {
		t.Fatalf("Runs: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != kept || runs[0].Bytes != 1000 || runs[0].DiskBytes != 3000 {
		t.Errorf("runs after deleting run %d = %+v, want run %d alone at 1000 bytes, 3000 on disk", gone, runs, kept)
	}
}

func TestANewRunStartsUnprotected(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	a := NewAdminStore(asAdminLogin(t, pool))
	run := addRun(t, pool, "complete", time.Time{})
	runs, err := a.Runs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != run || runs[0].Protected {
		t.Errorf("runs = %+v, want a new run unprotected", runs)
	}
}

func TestTheAdminLoginDeletesARunOnlyThroughItsGuards(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	admin := asAdminLogin(t, pool)
	run := addRun(t, pool, "complete", time.Time{})
	if _, err := admin.Exec(ctx, "SELECT delete_run($1)", run); err == nil {
		t.Errorf("delete_run of a run not marked for deletion succeeded, want refused")
	}
	if _, err := pool.Exec(ctx, "UPDATE rating_runs SET deleting = true, protected = true WHERE run_id = $1", run); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "SELECT delete_run($1)", run); err == nil {
		t.Errorf("delete_run of a protected run succeeded, want refused")
	}
	if err := NewAdminStore(admin).CheckLimits(ctx); err != nil {
		t.Errorf("CheckLimits after migration 49 = %v, want the admin login still limited", err)
	}
}
