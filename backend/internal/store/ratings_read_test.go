//go:build integration

package store

import (
	"context"
	"errors"
	"reflect"
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
		"INSERT INTO current_run (run_id) VALUES ($1) ON CONFLICT (ranking_id) DO UPDATE SET run_id = excluded.run_id", run)
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
	_, err := pool.Exec(ctx, `UPDATE rating_runs SET engine_version = 'abc1234', finished_at = $2, games = 135393,
		players = 3526, pairs = 388431, first_game = '2017-03-04', last_game = '2026-08-31' WHERE run_id = $1`, now, at.Add(-time.Minute))
	if err != nil {
		t.Fatalf("recording run stats: %v", err)
	}

	got, err := r.CurrentRun(ctx)
	if err != nil {
		t.Fatalf("CurrentRun: %v", err)
	}
	if got.ID != now || !got.PublishedAt.Equal(at) {
		t.Errorf("CurrentRun = %+v, want run %d published %s", got, now, at)
	}
	if got.FirstGame == nil || got.LastGame == nil ||
		got.FirstGame.Format(time.DateOnly) != "2017-03-04" || got.LastGame.Format(time.DateOnly) != "2026-08-31" {
		t.Errorf("CurrentRun games = %v to %v, want this run's: 2017-03-04 to 2026-08-31", got.FirstGame, got.LastGame)
	}
	if got.Games == nil || *got.Games != 135393 || got.Players == nil || *got.Players != 3526 ||
		got.EngineVersion != "abc1234" || got.FinishedAt == nil || !got.FinishedAt.Equal(at.Add(-time.Minute)) {
		t.Errorf("CurrentRun = %+v, want the run's recorded stats: 135393 games, 3526 players, engine abc1234, finished a minute before", got)
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

func TestHistoryGivesTheSnapshotOnOrBeforeADateAsUpstream(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := addRun(t, pool, "complete", time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC))
	addHistory(t, pool, run, "glicko2_daneo", 0, "ann", "2026-03-31", 1500)
	addHistory(t, pool, run, "glicko2_daneo", 0, "cat", "2026-03-31", 1450)
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
	want := []HistoryRow{{Name: "ann", Rating: 1690, RD: 30, CRating: 1600}}
	if !gotAt.Equal(day("2026-04-30")) || !slices.Equal(rows, want) {
		t.Errorf("History = %v %+v, want the 2026-04-30 snapshot %+v (cat is not in it, bob had no rating yet)", gotAt, rows, want)
	}

	early := day("2025-01-01")
	gotAt, rows, err = r.History(ctx, run, "glicko2_daneo", 0, &early)
	if err != nil || !gotAt.Equal(day("2026-03-31")) || len(rows) != 2 {
		t.Errorf("History before any snapshot = %v %+v %v, want the first, 2026-03-31, with ann and cat", gotAt, rows, err)
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

func TestRunLastGame(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	recorded := addRun(t, pool, "complete", time.Now())
	unrecorded := addRun(t, pool, "complete", time.Now())
	if _, err := pool.Exec(ctx, "UPDATE rating_runs SET last_game = '2026-08-31' WHERE run_id = $1", recorded); err != nil {
		t.Fatalf("recording the last game: %v", err)
	}

	got, err := r.RunLastGame(ctx, recorded)
	if err != nil || got == nil || got.Format(time.DateOnly) != "2026-08-31" {
		t.Errorf("RunLastGame(recorded) = %v, %v; want 2026-08-31", got, err)
	}
	if got, err := r.RunLastGame(ctx, unrecorded); err != nil || got != nil {
		t.Errorf("RunLastGame(unrecorded) = %v, %v; want nil, nil", got, err)
	}
	if _, err := r.RunLastGame(ctx, 999999); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("RunLastGame(unknown) err = %v, want ErrRunNotFound", err)
	}
}

func addRanking(t *testing.T, pool *pgxpool.Pool, slug string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "INSERT INTO rankings (slug, name) VALUES ($1, $1)", slug); err != nil {
		t.Fatalf("adding ranking %s: %v", slug, err)
	}
}

func TestCurrentRunIsTheCommunityRankings(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	community := publishedRun(t, pool)
	addRanking(t, pool, "blitz")
	var blitz int64
	if err := pool.QueryRow(ctx, `INSERT INTO rating_runs (engine_version, input_fingerprint, status, published_at, ranking_id)
		VALUES ('v1', 'f1', 'complete', now(), (SELECT ranking_id FROM rankings WHERE slug = 'blitz')) RETURNING run_id`).Scan(&blitz); err != nil {
		t.Fatalf("adding a blitz run: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO current_run (ranking_id, run_id) VALUES ((SELECT ranking_id FROM rankings WHERE slug = 'blitz'), $1)`, blitz); err != nil {
		t.Fatalf("making the blitz run current: %v", err)
	}
	makeCurrent(t, pool, community)

	got, err := NewRatingsReader(asReader(t, pool)).CurrentRun(ctx)
	if err != nil {
		t.Fatalf("CurrentRun: %v", err)
	}
	if got.ID != community {
		t.Errorf("CurrentRun = run %d, want the community ranking's run %d (blitz's is %d)", got.ID, community, blitz)
	}
}

func TestARankingsCurrentRunMustBeItsOwn(t *testing.T) {
	pool := migrated(t)
	community := publishedRun(t, pool)
	addRanking(t, pool, "blitz")
	_, err := pool.Exec(context.Background(), `INSERT INTO current_run (ranking_id, run_id) VALUES ((SELECT ranking_id FROM rankings WHERE slug = 'blitz'), $1)`, community)
	if err == nil {
		t.Errorf("a community run was made the blitz ranking's current run")
	}
}

func giveBadges(t *testing.T, pool *pgxpool.Pool, name string, labels ...string) {
	t.Helper()
	for _, l := range labels {
		if _, err := pool.Exec(context.Background(), `
			WITH b AS (INSERT INTO badges (label, description) VALUES ($1, $1 || ' description')
			           ON CONFLICT (label) DO UPDATE SET label = excluded.label RETURNING badge_id)
			INSERT INTO player_badges (player_name, badge_id) SELECT $2, badge_id FROM b`, l, name); err != nil {
			t.Fatalf("giving %s the %s badge: %v", name, l, err)
		}
	}
}

func TestLeaderboardGivesEachPlayersBadgesAtOnce(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := addRun(t, pool, "complete", time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC))
	addRating(t, pool, run, "glicko2_daneo", 0, "ann", intp(1), 1900)
	addRating(t, pool, run, "glicko2_daneo", 0, "bob", intp(2), 1700)
	giveBadges(t, pool, "ann", "TD", "Organiser")

	got, err := r.Leaderboard(ctx, run, "glicko2_daneo", 0)
	if err != nil {
		t.Fatalf("Leaderboard: %v", err)
	}
	want := []ExtraBadge{{Label: "Organiser", Description: "Organiser description"}, {Label: "TD", Description: "TD description"}}
	if !reflect.DeepEqual(got[0].Badges, want) || len(got[1].Badges) != 0 {
		t.Errorf("badges = %+v and %+v, want ann's two by label and none for bob", got[0].Badges, got[1].Badges)
	}
}
