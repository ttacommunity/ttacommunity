//go:build integration

package store

// The fixtures most store tests share: a migrated database, its logins, runs, players and placed games.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func migrated(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := newDB(t)
	if err := MigrateRatings(context.Background(), pool); err != nil {
		t.Fatalf("MigrateRatings: %v", err)
	}
	return pool
}

// asReader opens a second pool on pool's database whose connections act as ratings_read.
func asReader(t *testing.T, pool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	cfg := pool.Config().Copy()
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE ratings_read")
		return err
	}
	reader, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("opening a ratings_read pool: %v", err)
	}
	t.Cleanup(reader.Close)
	return reader
}

// asAdminLogin returns a pool on the same database acting as ratings_admin, as the API's admin login does.
func asAdminLogin(t *testing.T, pool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	cfg := pool.Config().Copy()
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE ratings_admin")
		return err
	}
	admin, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("opening a ratings_admin pool: %v", err)
	}
	t.Cleanup(admin.Close)
	return admin
}

func day(s string) time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return d
}

// addRun inserts a run with status, published at publishedAt unless it is zero.
func addRun(t *testing.T, pool *pgxpool.Pool, status string, publishedAt time.Time) int64 {
	t.Helper()
	var published *time.Time
	if !publishedAt.IsZero() {
		published = &publishedAt
	}
	var id int64
	err := pool.QueryRow(context.Background(),
		"INSERT INTO rating_runs (engine_version, input_fingerprint, status, published_at) VALUES ('v1', 'f1', $1, $2) RETURNING run_id",
		status, published).Scan(&id)
	if err != nil {
		t.Fatalf("inserting a %s run: %v", status, err)
	}
	return id
}

func publishedRun(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	return addRun(t, pool, "complete", time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC))
}

func makeCurrent(t *testing.T, pool *pgxpool.Pool, run int64) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		"INSERT INTO current_run (run_id) VALUES ($1) ON CONFLICT (ranking_id) DO UPDATE SET run_id = excluded.run_id", run)
	if err != nil {
		t.Fatalf("making run %d current: %v", run, err)
	}
}

func addPlayers(t *testing.T, pool *pgxpool.Pool, names ...string) {
	t.Helper()
	for _, n := range names {
		if _, err := pool.Exec(context.Background(), "INSERT INTO players (name) VALUES ($1)", n); err != nil {
			t.Fatalf("adding player %s: %v", n, err)
		}
	}
}

func addRating(t *testing.T, pool *pgxpool.Pool, run int64, model string, format int, name string, rank *int, cRating float64) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO player_ratings (run_id, model_type, player_count, player_name, rating, rd, sigma, c_rating, rank,
			rank_delta, games_played, opponents_count, wins, losses, draws, win_rate, last_played)
		VALUES ($1, $2, $3, $4, $5::float8 + 100, 50, 0.06, $5::float8, $6, -2, 12, 9, 7, 4, 1, 58.3, '2025-02-17')`,
		run, model, format, name, cRating, rank)
	if err != nil {
		t.Fatalf("inserting %s's rating in run %d: %v", name, run, err)
	}
}

// placedOn loads games by name, each played on its date, and places them in the catalogue.
func placedOn(t *testing.T, pool *pgxpool.Pool, games map[string]time.Time) {
	t.Helper()
	ctx := context.Background()
	var names []string
	for n := range games {
		names = append(names, n)
	}
	loadNamed(t, pool, names...)
	for n, d := range games {
		if _, err := pool.Exec(ctx, "UPDATE matches SET date = $2 WHERE tournament = $1", n, d); err != nil {
			t.Fatalf("dating %s: %v", n, err)
		}
	}
	if _, err := ClassifyMatches(ctx, pool); err != nil {
		t.Fatalf("ClassifyMatches: %v", err)
	}
}

// ptr returns a pointer to a copy of v, for optional fields in test data.
func ptr[T any](v T) *T { return &v }
