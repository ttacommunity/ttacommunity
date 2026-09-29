//go:build integration

package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

func makeCurrent(t *testing.T, pool *pgxpool.Pool, run int64) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		"INSERT INTO current_run (run_id) VALUES ($1) ON CONFLICT (only_row) DO UPDATE SET run_id = excluded.run_id", run)
	if err != nil {
		t.Fatalf("making run %d current: %v", run, err)
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

func intp(n int) *int { return &n }

func TestRatingsReadRoleCanOnlySelect(t *testing.T) {
	pool := migrated(t)
	reader := asReader(t, pool)
	ctx := context.Background()

	if _, err := reader.Exec(ctx, "SELECT count(*) FROM player_ratings"); err != nil {
		t.Fatalf("SELECT as ratings_read: %v", err)
	}
	writes := []string{
		"INSERT INTO rating_runs (engine_version, input_fingerprint) VALUES ('v1', 'f1')",
		"UPDATE player_ratings SET rating = 0",
		"DELETE FROM current_run",
		"CREATE TABLE sneaky (id int)",
	}
	for _, sql := range writes {
		_, err := reader.Exec(ctx, sql)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Errorf("%q as ratings_read: err = %v, want permission denied (42501)", sql, err)
		}
	}
}

func TestCheckReadOnly(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()

	if err := NewRatingsReader(asReader(t, pool)).CheckReadOnly(ctx); err != nil {
		t.Errorf("CheckReadOnly as ratings_read: %v, want nil", err)
	}
	if err := NewRatingsReader(pool).CheckReadOnly(ctx); !errors.Is(err, ErrCanWrite) {
		t.Errorf("CheckReadOnly as the owner: err = %v, want ErrCanWrite", err)
	}
	if err := NewRatingsReader(newDB(t)).CheckReadOnly(ctx); !errors.Is(err, ErrCanWrite) {
		t.Errorf("CheckReadOnly as the owner of an unmigrated database: err = %v, want ErrCanWrite", err)
	}
}

func TestPing(t *testing.T) {
	reader := asReader(t, migrated(t))
	ctx := context.Background()

	if err := NewRatingsReader(reader).Ping(ctx); err != nil {
		t.Errorf("Ping on an open pool: %v, want nil", err)
	}
	reader.Close()
	if err := NewRatingsReader(reader).Ping(ctx); err == nil {
		t.Error("Ping on a closed pool: nil, want an error")
	}
}

func TestCheckReadOnlyCatchesAnyWritableTable(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "GRANT INSERT ON refused_rows TO ratings_read"); err != nil {
		t.Fatalf("granting INSERT on refused_rows: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "REVOKE INSERT ON refused_rows FROM ratings_read") })

	if err := NewRatingsReader(asReader(t, pool)).CheckReadOnly(ctx); !errors.Is(err, ErrCanWrite) {
		t.Errorf("CheckReadOnly with INSERT on refused_rows: err = %v, want ErrCanWrite", err)
	}
}

func TestCurrentRun(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()

	if _, err := r.CurrentRun(ctx); !errors.Is(err, ErrNoPublishedRun) {
		t.Fatalf("CurrentRun with nothing published: err = %v, want ErrNoPublishedRun", err)
	}

	at := time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC)
	old := addRun(t, pool, "complete", at.Add(-time.Hour))
	now := addRun(t, pool, "complete", at)
	makeCurrent(t, pool, old)
	makeCurrent(t, pool, now)

	got, err := r.CurrentRun(ctx)
	if err != nil {
		t.Fatalf("CurrentRun: %v", err)
	}
	if got.ID != now || !got.PublishedAt.Equal(at) {
		t.Errorf("CurrentRun = %+v, want run %d published %s", got, now, at)
	}
}

func TestLeaderboard(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	at := time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC)

	run := addRun(t, pool, "complete", at)
	addRating(t, pool, run, "glicko2_daneo", 0, "cat", intp(3), 1500)
	addRating(t, pool, run, "glicko2_daneo", 0, "ann", intp(1), 1900)
	addRating(t, pool, run, "glicko2_daneo", 0, "unranked", nil, 1000)
	addRating(t, pool, run, "glicko2_daneo", 0, "bob", intp(2), 1700)
	addRating(t, pool, run, "glicko2_daneo", 2, "duel-only", intp(1), 2000)
	addRating(t, pool, run, "whr", 0, "whr-only", intp(1), 2000)
	other := addRun(t, pool, "complete", at)
	addRating(t, pool, other, "glicko2_daneo", 0, "other-run", intp(1), 2000)

	got, err := r.Leaderboard(ctx, run, "glicko2_daneo", 0)
	if err != nil {
		t.Fatalf("Leaderboard: %v", err)
	}
	var names []string
	for _, p := range got {
		names = append(names, p.Name)
	}
	if want := []string{"ann", "bob", "cat", "unranked"}; !slices.Equal(names, want) {
		t.Errorf("Leaderboard names = %q, want %q", names, want)
	}

	want := LeaderboardRow{
		Rank: intp(1), RankDelta: -2, Name: "ann", Rating: 2000, RD: 50, CRating: 1900,
		Games: 12, Opponents: 9, Wins: 7, Losses: 4, Draws: 1, WinRate: 58.3,
		LastPlayed: new(time.Date(2025, 2, 17, 0, 0, 0, 0, time.UTC)),
	}
	if g := got[0]; *g.Rank != *want.Rank || g.RankDelta != want.RankDelta || g.Rating != want.Rating ||
		g.RD != want.RD || g.CRating != want.CRating || g.Games != want.Games || g.Opponents != want.Opponents ||
		g.Wins != want.Wins || g.Losses != want.Losses || g.Draws != want.Draws || g.WinRate != want.WinRate ||
		g.LastPlayed == nil || !g.LastPlayed.Equal(*want.LastPlayed) {
		t.Errorf("Leaderboard[0] = %+v, want %+v", g, want)
	}
	if got[3].Rank != nil {
		t.Errorf("unranked player's rank = %d, want nil", *got[3].Rank)
	}
}

func TestLeaderboardServesOnlyPublishedRuns(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	at := time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC)

	published := addRun(t, pool, "complete", at)
	tests := []struct {
		name string
		run  int64
	}{
		{name: "complete, never published", run: addRun(t, pool, "complete", time.Time{})},
		{name: "running", run: addRun(t, pool, "running", time.Time{})},
		{name: "failed", run: addRun(t, pool, "failed", time.Time{})},
		{name: "no such run", run: published + 1000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addRatingIfRun(t, pool, tt.run)
			if _, err := r.Leaderboard(ctx, tt.run, "glicko2_daneo", 0); !errors.Is(err, ErrRunNotFound) {
				t.Errorf("Leaderboard(run %d): err = %v, want ErrRunNotFound", tt.run, err)
			}
		})
	}

	got, err := r.Leaderboard(ctx, published, "glicko2_daneo", 0)
	if err != nil || len(got) != 0 {
		t.Errorf("Leaderboard(published run, no rows) = %v, %v; want an empty list", got, err)
	}
}

// addRatingIfRun gives an existing run a row, so a refusal cannot come from an empty run.
func addRatingIfRun(t *testing.T, pool *pgxpool.Pool, run int64) {
	t.Helper()
	var exists bool
	err := pool.QueryRow(context.Background(), "SELECT EXISTS (SELECT 1 FROM rating_runs WHERE run_id = $1)", run).Scan(&exists)
	if err != nil {
		t.Fatalf("looking up run %d: %v", run, err)
	}
	if exists {
		addRating(t, pool, run, "glicko2_daneo", 0, "ann", intp(1), 1900)
	}
}

func addTitle(t *testing.T, pool *pgxpool.Pool, run int64, name, title string, count int, reason string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		"INSERT INTO player_titles (run_id, player_name, title, title_count, badge_reason) VALUES ($1, $2, $3, $4, $5)",
		run, name, title, count, reason)
	if err != nil {
		t.Fatalf("adding %s's title in run %d: %v", name, run, err)
	}
}

func TestLeaderboardJoinsPlayerCountryAndTheRunsTitle(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	at := time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC)

	addPlayers(t, pool, "ann", "bob")
	if _, err := SetCountries(ctx, pool, map[string]string{"ann": "PL"}); err != nil {
		t.Fatalf("SetCountries: %v", err)
	}
	run := addRun(t, pool, "complete", at)
	other := addRun(t, pool, "complete", at)
	addRating(t, pool, run, "glicko2_daneo", 0, "ann", intp(1), 1900)
	addRating(t, pool, run, "glicko2_daneo", 0, "bob", intp(2), 1700)
	addRating(t, pool, run, "glicko2_daneo", 0, "not-a-player", intp(3), 1500)
	addTitle(t, pool, run, "ann", "GM", 3, "Played in Royal League Grandmaster Season 16")
	addTitle(t, pool, other, "bob", "M", 1, "from another run")

	got, err := r.Leaderboard(ctx, run, "glicko2_daneo", 0)
	if err != nil {
		t.Fatalf("Leaderboard: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("Leaderboard has %d rows, want 3", len(got))
	}
	ann, bob, stray := got[0], got[1], got[2]
	if ann.Country == nil || *ann.Country != "PL" {
		t.Errorf("ann's country = %v, want PL", ann.Country)
	}
	want := Title{Code: "GM", Count: 3, Reason: "Played in Royal League Grandmaster Season 16"}
	if ann.Title == nil || *ann.Title != want {
		t.Errorf("ann's title = %+v, want %+v", ann.Title, want)
	}
	if bob.Country != nil || bob.Title != nil {
		t.Errorf("bob = country %v, title %+v; want neither (his title is another run's)", bob.Country, bob.Title)
	}
	if stray.Name != "not-a-player" || stray.Country != nil || stray.Title != nil {
		t.Errorf("row without a player = %+v, want it kept with no country or title", stray)
	}
}

func addHistory(t *testing.T, pool *pgxpool.Pool, run int64, model string, format int, name, period string, cRating float64) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO rating_history (run_id, model_type, player_count, player_name, period_date, rating, rd, c_rating)
		VALUES ($1, $2, $3, $4, $5, $6::float8 + 90, 30, $6::float8)`, run, model, format, name, period, cRating)
	if err != nil {
		t.Fatalf("adding %s's history at %s: %v", name, period, err)
	}
}

func day(s string) time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestHistoryGivesEachPlayersLatestRatingOnOrBeforeADate(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := addRun(t, pool, "complete", time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC))
	addHistory(t, pool, run, "glicko2_daneo", 0, "ann", "2026-03-31", 1500)
	addHistory(t, pool, run, "glicko2_daneo", 0, "ann", "2026-04-30", 1600)
	addHistory(t, pool, run, "glicko2_daneo", 0, "ann", "2026-05-31", 1700)
	addHistory(t, pool, run, "glicko2_daneo", 0, "bob", "2026-05-31", 1400)
	addHistory(t, pool, run, "glicko2_daneo", 2, "ann", "2026-04-30", 9999)
	addHistory(t, pool, run, "whr", 0, "ann", "2026-04-30", 9999)

	at := day("2026-05-01")
	gotAt, rows, err := r.History(ctx, run, "glicko2_daneo", 0, &at)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if !gotAt.Equal(at) {
		t.Errorf("at = %v, want %v", gotAt, at)
	}
	want := []HistoryRow{{Name: "ann", Rating: 1690, RD: 30, CRating: 1600}}
	if !slices.Equal(rows, want) {
		t.Errorf("History = %+v, want %+v (bob had no rating yet)", rows, want)
	}
}

func TestHistoryWithoutADateUsesThePreviousPeriod(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := addRun(t, pool, "complete", time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC))
	addHistory(t, pool, run, "glicko2_daneo", 0, "ann", "2026-06-30", 1500)
	addHistory(t, pool, run, "glicko2_daneo", 0, "ann", "2026-07-31", 1600)
	addHistory(t, pool, run, "glicko2_daneo", 0, "bob", "2026-08-31", 1700)

	gotAt, rows, err := r.History(ctx, run, "glicko2_daneo", 0, nil)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if !gotAt.Equal(day("2026-07-31")) {
		t.Errorf("at = %v, want the previous period 2026-07-31", gotAt)
	}
	if len(rows) != 1 || rows[0].Name != "ann" || rows[0].CRating != 1600 {
		t.Errorf("History = %+v, want ann at 1600", rows)
	}
}

func TestHistoryRefusesUnpublishedRunsAndEmptyHistory(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	hidden := addRun(t, pool, "complete", time.Time{})
	addHistory(t, pool, hidden, "glicko2_daneo", 0, "ann", "2026-07-31", 1600)
	if _, _, err := r.History(ctx, hidden, "glicko2_daneo", 0, nil); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("History of an unpublished run: err = %v, want ErrRunNotFound", err)
	}

	empty := addRun(t, pool, "complete", time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC))
	if _, _, err := r.History(ctx, empty, "glicko2_daneo", 0, nil); !errors.Is(err, ErrNoHistory) {
		t.Errorf("History with no periods: err = %v, want ErrNoHistory", err)
	}
}

func TestLeaderboardJoinsTheRunsCareerPeak(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := addRun(t, pool, "complete", time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC))
	other := addRun(t, pool, "complete", time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC))
	addRating(t, pool, run, "glicko2_daneo", 0, "ann", intp(1), 1900)
	addRating(t, pool, run, "glicko2_daneo", 0, "bob", intp(2), 1700)
	for _, p := range []struct {
		run  int64
		name string
	}{{run, "ann"}, {other, "bob"}} {
		if _, err := pool.Exec(ctx, "INSERT INTO player_peak_titles (run_id, player_name, peak_title, peak_year) VALUES ($1, $2, 'GM', 2024)", p.run, p.name); err != nil {
			t.Fatalf("adding %s's peak: %v", p.name, err)
		}
	}

	got, err := r.Leaderboard(ctx, run, "glicko2_daneo", 0)
	if err != nil {
		t.Fatalf("Leaderboard: %v", err)
	}
	if p := got[0].Peak; p == nil || p.Code != "GM" || p.Year == nil || *p.Year != 2024 {
		t.Errorf("ann's peak = %+v, want GM in 2024", p)
	}
	if got[1].Peak != nil {
		t.Errorf("bob's peak = %+v, want none (his is another run's)", got[1].Peak)
	}
}
