//go:build integration

package store

// The fixtures most store tests share: a migrated database, its logins, runs, players and placed games.

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func migrated(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if _, again := parallel.LoadOrStore(t, true); !again {
		t.Parallel() // each test has its own database
	}
	if template.once.Do(func() { template.name, template.err = migrateTemplate() }); template.err != nil {
		t.Fatalf("migrating the template database: %v", template.err)
	}
	return newDBFrom(t, template.name)
}

// parallel holds the tests already marked parallel, as a test that asks for two databases may mark itself only once.
var parallel sync.Map

// template is one migrated database per test run, which migrated copies: much faster than migrating each time.
var template struct {
	once sync.Once
	name string
	err  error
}

// migrateTemplate creates and migrates the template database, closing every connection to it so it can be copied.
func migrateTemplate() (string, error) {
	ctx := context.Background()
	base, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil || base.Host == "" {
		return "", fmt.Errorf("TEST_DATABASE_URL is not set or not a URL: integration tests need a database")
	}
	admin, err := pgx.Connect(ctx, base.String())
	if err != nil {
		return "", fmt.Errorf("connecting to the test server: %w", err)
	}
	defer admin.Close(ctx)
	if os.Getenv("TEST_DATABASE_THROWAWAY") == "1" {
		if err := skipFlushes(ctx, admin); err != nil {
			return "", err
		}
	}
	name := "ratings_template_" + rand.Text()[:12]
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return "", fmt.Errorf("creating %s: %w", name, err)
	}
	base.Path = "/" + name
	pool, err := pgxpool.New(ctx, base.String())
	if err != nil {
		return name, fmt.Errorf("opening %s: %w", name, err)
	}
	defer pool.Close()
	if err := MigrateRatings(ctx, pool); err != nil {
		return name, fmt.Errorf("MigrateRatings: %w", err)
	}
	return name, nil
}

// skipFlushes turns off the server's flushes to disk, which make each test's CREATE and DROP DATABASE slow.
// Only for a server thrown away after the run, as CI's is: a crash can corrupt it
// (https://www.postgresql.org/docs/current/non-durability.html).
func skipFlushes(ctx context.Context, admin *pgx.Conn) error {
	for _, stmt := range []string{"ALTER SYSTEM SET fsync = off", "ALTER SYSTEM SET full_page_writes = off", "SELECT pg_reload_conf()"} {
		if _, err := admin.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return nil
}

// TestMain drops the template database once every test has run.
func TestMain(m *testing.M) {
	code := m.Run()
	if template.name != "" {
		if admin, err := pgx.Connect(context.Background(), os.Getenv("TEST_DATABASE_URL")); err == nil {
			if _, err := admin.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{template.name}.Sanitize()+" WITH (FORCE)"); err != nil {
				fmt.Fprintf(os.Stderr, "dropping %s: %v\n", template.name, err)
				code = 1
			}
			admin.Close(context.Background())
		}
	}
	os.Exit(code)
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

// asBackupLogin returns a pool on the same database acting as ratings_backup, as the backup job's login does.
func asBackupLogin(t *testing.T, pool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	cfg := pool.Config().Copy()
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE ratings_backup")
		return err
	}
	backup, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("opening a ratings_backup pool: %v", err)
	}
	t.Cleanup(backup.Close)
	return backup
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

// asPlayerLogin returns a pool on the same database acting as ratings_player, as the API's player login does.
func asPlayerLogin(t *testing.T, pool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	cfg := pool.Config().Copy()
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE ratings_player")
		return err
	}
	player, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("opening a ratings_player pool: %v", err)
	}
	t.Cleanup(player.Close)
	return player
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

// withSpans records run's catalogue spans from the games placed so far, as the engine does in a run.
func withSpans(t *testing.T, pool *pgxpool.Pool, run int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "DELETE FROM catalogue_spans WHERE run_id = $1", run); err != nil {
		t.Fatalf("clearing run %d's catalogue spans: %v", run, err)
	}
	if _, err := pool.Exec(ctx, "SELECT derive_catalogue_spans($1)", run); err != nil {
		t.Fatalf("deriving run %d's catalogue spans: %v", run, err)
	}
}

// withSeatStats records run's seat statistics from its pairs and the seats so far, as the engine does in a run.
func withSeatStats(t *testing.T, pool *pgxpool.Pool, run int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "DELETE FROM player_seat_stats WHERE run_id = $1", run); err != nil {
		t.Fatalf("clearing run %d's seat stats: %v", run, err)
	}
	if _, err := pool.Exec(ctx, "SELECT derive_player_seat_stats($1)", run); err != nil {
		t.Fatalf("deriving run %d's seat stats: %v", run, err)
	}
}

// withYearRecords records run's player records by format and year from its pairs so far, as the engine does in a run.
func withYearRecords(t *testing.T, pool *pgxpool.Pool, run int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "DELETE FROM player_year_records WHERE run_id = $1", run); err != nil {
		t.Fatalf("clearing run %d's year records: %v", run, err)
	}
	if _, err := pool.Exec(ctx, "SELECT derive_player_year_records($1)", run); err != nil {
		t.Fatalf("deriving run %d's year records: %v", run, err)
	}
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

// hideAccount gives the player name a CGE account marked how, "deleted" or "banned", which hides them on the site.
func hideAccount(t *testing.T, pool *pgxpool.Pool, name, how string) {
	t.Helper()
	tag, err := pool.Exec(context.Background(), `
		INSERT INTO player_accounts (cge_user, player_id, name, deleted, banned)
		SELECT (SELECT coalesce(max(cge_user), 0) + 1 FROM player_accounts), player_id, name, $2 = 'deleted', $2 = 'banned'
		FROM players WHERE name = $1`, name, how)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("marking %s's account %s: %v (%d rows)", name, how, err, tag.RowsAffected())
	}
}

func addRating(t *testing.T, pool *pgxpool.Pool, run int64, model string, format int, name string, rank *int, cRating float64) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO player_ratings (run_id, model_type, player_count, player_name, rating, rd, sigma, c_rating, rank,
			rank_delta, games_played, opponents_count, wins, losses, draws, win_rate, last_played)
		VALUES ($1, $2, $3, $4, $5::float8 + 100, 50, 0.06, $5::float8, $6, -2, 12, 9, 7, 4, 1, 58.3, '2025-02-17')`,
		run, model, format, name, cRating, rank)
	if err != nil {
		t.Fatalf("inserting %s's rating in run %d: %v", name, run, err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO rating_history (run_id, model_type, player_count, player_name, period_date, rating, rd, c_rating)
		VALUES ($1, $2, $3, $4, '2025-01-01', $5::float8 + 100, 50, $5::float8)
		ON CONFLICT (run_id, model_type, player_count, player_name, period_date) DO NOTHING`,
		run, model, format, name, cRating)
	if err != nil {
		t.Fatalf("inserting %s's rating history in run %d: %v", name, run, err)
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
		if _, err := pool.Exec(ctx, "UPDATE games SET date = $2 WHERE name = $1", n, d); err != nil {
			t.Fatalf("dating %s: %v", n, err)
		}
	}
	if _, err := ClassifyMatches(ctx, pool); err != nil {
		t.Fatalf("ClassifyMatches: %v", err)
	}
}

// ptr returns a pointer to a copy of v, for optional fields in test data.
func ptr[T any](v T) *T { return &v }

// setSeats replaces the seats of every game named like pattern (SQL LIKE), adding players as needed; each
// seat's place follows its score, and its culture is the score, or 0 below 0.
func setSeats(t *testing.T, pool *pgxpool.Pool, pattern string, seats ...Seat) {
	t.Helper()
	ctx := context.Background()
	var names []string
	var scores []float64
	for _, s := range seats {
		names, scores = append(names, s.Name), append(scores, s.Score)
	}
	steps := []struct {
		q    string
		args []any
	}{
		{"DELETE FROM seats WHERE game_id IN (SELECT game_id FROM games WHERE name LIKE $1)", []any{pattern}},
		{"INSERT INTO players (name) SELECT unnest($1::text[]) ON CONFLICT (name) DO NOTHING", []any{names}},
		{`INSERT INTO seats (game_id, seat, player_id, player_name, culture, recorded_score, place)
		  SELECT g.game_id, s.seat, p.player_id, s.name, greatest(s.score, 0), s.score,
		         rank() OVER (PARTITION BY g.game_id ORDER BY s.score DESC)
		  FROM games g CROSS JOIN unnest($2::text[], $3::float8[]) WITH ORDINALITY AS s(name, score, seat)
		  JOIN players p ON p.name = s.name WHERE g.name LIKE $1`, []any{pattern, names, scores}},
	}
	for _, step := range steps {
		if _, err := pool.Exec(ctx, step.q, step.args...); err != nil {
			t.Fatalf("setting the seats of %s: %v\n%s", pattern, err, step.q)
		}
	}
}
