//go:build integration

package store

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/google/go-cmp/cmp"
	"github.com/pressly/goose/v3"
)

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

func TestCheckPublishedRefusesARunNotPublished(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	at := time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		run  int64
		want error
	}{
		{"published", addRun(t, pool, "complete", at), nil},
		{"complete, not published", addRun(t, pool, "complete", time.Time{}), ErrRunNotFound},
		{"unknown", 999999, ErrRunNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := r.CheckPublished(ctx, tt.run); !errors.Is(err, tt.want) {
				t.Errorf("CheckPublished of the %s run = %v, want %v", tt.name, err, tt.want)
			}
		})
	}
}

func TestLeaderboard(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	at := time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC)

	run := addRun(t, pool, "complete", at)
	addRating(t, pool, run, "glicko2_daneo", 0, "cat", ptr(3), 1500)
	addRating(t, pool, run, "glicko2_daneo", 0, "ann", ptr(1), 1900)
	addRating(t, pool, run, "glicko2_daneo", 0, "unranked", nil, 1000)
	addRating(t, pool, run, "glicko2_daneo", 0, "bob", ptr(2), 1700)
	addRating(t, pool, run, "glicko2_daneo", 2, "duel-only", ptr(1), 2000)
	addRating(t, pool, run, "glicko2_std", 0, "whr-only", ptr(1), 2000)
	other := addRun(t, pool, "complete", at)
	addRating(t, pool, other, "glicko2_daneo", 0, "other-run", ptr(1), 2000)

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
		Rank: ptr(1), RankDelta: -2, Name: "ann", Rating: 2000, RD: 50, CRating: 1900,
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

func TestLeaderboardExcludesDeletedAndBannedPlayers(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	at := time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC)
	run := addRun(t, pool, "complete", at)
	addPlayers(t, pool, "ann", "bob", "cat")
	addRating(t, pool, run, "glicko2_daneo", 0, "ann", ptr(1), 1900)
	addRating(t, pool, run, "glicko2_daneo", 0, "bob", ptr(2), 1700)
	addRating(t, pool, run, "glicko2_daneo", 0, "cat", ptr(3), 1500)
	for _, q := range []string{
		"INSERT INTO player_accounts (cge_user, player_id, name, deleted) SELECT 1, player_id, 'ann', true FROM players WHERE name = 'ann'",
		"INSERT INTO player_accounts (cge_user, player_id, name, banned) SELECT 2, player_id, 'bob', true FROM players WHERE name = 'bob'",
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("setting up player account: %v", err)
		}
	}

	got, err := r.Leaderboard(ctx, run, "glicko2_daneo", 0)
	if err != nil {
		t.Fatalf("Leaderboard: %v", err)
	}
	var names []string
	for _, p := range got {
		names = append(names, p.Name)
	}
	if want := []string{"cat"}; !slices.Equal(names, want) {
		t.Errorf("Leaderboard names = %q, want %q (ann and bob omitted)", names, want)
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
		addRating(t, pool, run, "glicko2_daneo", 0, "ann", ptr(1), 1900)
	}
}

func addTitle(t *testing.T, pool *pgxpool.Pool, run int64, name, title string, count int, reason string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		"INSERT INTO player_titles (run_id, player_id, title, title_count, badge_reason) VALUES ($1, $2, $3, $4, $5)",
		run, playerID(t, pool, name), title, count, reason)
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
	addRating(t, pool, run, "glicko2_daneo", 0, "ann", ptr(1), 1900)
	addRating(t, pool, run, "glicko2_daneo", 0, "bob", ptr(2), 1700)
	addRating(t, pool, run, "glicko2_daneo", 0, "cy", ptr(3), 1500)
	addTitle(t, pool, run, "ann", "GM", 3, "Played in Royal League Grandmaster Season 16")
	addTitle(t, pool, other, "bob", "M", 1, "from another run")

	got, err := r.Leaderboard(ctx, run, "glicko2_daneo", 0)
	if err != nil {
		t.Fatalf("Leaderboard: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("Leaderboard has %d rows, want 3", len(got))
	}
	ann, bob, cy := got[0], got[1], got[2]
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
	if cy.Name != "cy" || cy.Country != nil || cy.Title != nil {
		t.Errorf("a player with no member = %+v, want kept with no country or title", cy)
	}
}

func addHistory(t *testing.T, pool *pgxpool.Pool, run int64, model string, format int, name, period string, cRating float64) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO rating_history (run_id, model_type, player_count, player_id, period_date, rating, rd, c_rating)
		VALUES ($1, $2, $3, $4, $5, $6::float8 + 90, 30, $6::float8)`, run, model, format, playerID(t, pool, name), period, cRating)
	if err != nil {
		t.Fatalf("adding %s's history at %s: %v", name, period, err)
	}
}

func addWindow(t *testing.T, pool *pgxpool.Pool, run int64, model string, format int, window, name, period string, rank int, cRating float64) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO rating_windows (run_id, model_type, player_count, delta_window, period_date, player_id, rating, rd, c_rating, rank)
		VALUES ($1, $2, $3, $4, $5, $6, $7::float8 + 90, 30, $7::float8, $8)`,
		run, model, format, window, period, playerID(t, pool, name), cRating, rank)
	if err != nil {
		t.Fatalf("adding %s's %s board in run %d: %v", name, window, run, err)
	}
}

// startedAt records that run started at.
func startedAt(t *testing.T, pool *pgxpool.Pool, run int64, at time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "UPDATE rating_runs SET started_at = $2 WHERE run_id = $1", run, at); err != nil {
		t.Fatalf("starting run %d at %v: %v", run, at, err)
	}
}

// addDataLoad records a data load made at, whose last finished game was on lastGame.
func addDataLoad(t *testing.T, pool *pgxpool.Pool, at time.Time, lastGame string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		WITH b AS (INSERT INTO import_batches (source, created_at) VALUES ('load', $1) RETURNING batch_id)
		INSERT INTO data_loads (batch_id, last_game) SELECT batch_id, $2 FROM b`, at, lastGame)
	if err != nil {
		t.Fatalf("adding a data load at %v: %v", at, err)
	}
}

func TestHistoryOfAWindowIsTheRunsBoardAtItsStart(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	run := publishedRun(t, pool)
	addWindow(t, pool, run, "glicko2_daneo", 0, "month", "ann", "2026-09-30", 2, 1500)
	addWindow(t, pool, run, "glicko2_daneo", 0, "month", "bob", "2026-09-30", 1, 1600)
	addWindow(t, pool, run, "glicko2_daneo", 0, "quarter", "ann", "2026-07-31", 1, 9999)
	addWindow(t, pool, run, "glicko2_daneo", 2, "month", "ann", "2026-09-30", 1, 9999)
	addWindow(t, pool, run, "glicko2_std", 0, "month", "ann", "2026-09-30", 1, 9999)

	at, rows, err := r.History(context.Background(), run, "glicko2_daneo", 0, "month")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	want := []HistoryRow{{Name: "ann", Rating: 1590, RD: 30, CRating: 1500, Rank: 2}, {Name: "bob", Rating: 1690, RD: 30, CRating: 1600, Rank: 1}}
	if !at.Equal(day("2026-09-30")) || !slices.Equal(rows, want) {
		t.Errorf("History(month) = %v %+v, want 2026-09-30 %+v", at, rows, want)
	}
}

func TestHistoryOfTheLastUpdateIsTheBoardMadeFromTheLoadBeforeTheLatest(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	at := func(month time.Month, d, h int) time.Time { return time.Date(2026, month, d, h, 0, 0, 0, time.UTC) }
	addDataLoad(t, pool, at(10, 8, 3), "2026-10-06")
	addDataLoad(t, pool, at(11, 4, 2), "2026-11-03")
	runs := []struct {
		name               string
		started, published time.Time
		cRating            float64
	}{
		{"before October's load", at(10, 1, 0), at(10, 1, 1), 1100},
		{"from October's load", at(10, 30, 8), at(10, 30, 9), 1200},
		{"from October's load, published last", at(10, 31, 5), at(10, 31, 6), 1300},
		{"from October's load, never published", at(11, 1, 0), time.Time{}, 1400},
		{"the run asked for", at(11, 4, 3), at(11, 4, 4), 1500},
	}
	var asked int64
	for _, rn := range runs {
		run := addRun(t, pool, "complete", rn.published)
		startedAt(t, pool, run, rn.started)
		addRating(t, pool, run, "glicko2_daneo", 0, "ann", ptr(1), rn.cRating)
		asked = run
	}
	addDataLoad(t, pool, at(11, 5, 0), "2026-11-04")

	on, rows, err := r.History(context.Background(), asked, "glicko2_daneo", 0, "last_update")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	want := []HistoryRow{{Name: "ann", Rating: 1400, RD: 50, CRating: 1300, Rank: 1}}
	if !on.Equal(day("2026-10-06")) || !slices.Equal(rows, want) {
		t.Errorf("History(last_update) = %v %+v, want October's load's last game 2026-10-06 and the board %+v", on, rows, want)
	}
}

func TestHistoryWithNoBoardToCompareWithIsErrNoPastBoard(t *testing.T) {
	tests := []struct {
		name, window string
		loads        int
	}{
		{"a window the run saved no board for", "year", 2},
		{"the last update with one data load", "last_update", 1},
		{"the last update with no run made from the load before", "last_update", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			r := NewRatingsReader(asReader(t, pool))
			for i := range tt.loads {
				addDataLoad(t, pool, time.Date(2026, 10, 1+i, 0, 0, 0, 0, time.UTC), "2026-09-30")
			}
			run := publishedRun(t, pool)
			startedAt(t, pool, run, time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC))
			addRating(t, pool, run, "glicko2_daneo", 0, "ann", ptr(1), 1500)
			addWindow(t, pool, run, "glicko2_daneo", 0, "month", "ann", "2026-09-30", 1, 1500)

			if _, _, err := r.History(context.Background(), run, "glicko2_daneo", 0, tt.window); !errors.Is(err, ErrNoPastBoard) {
				t.Errorf("History(%s): err = %v, want ErrNoPastBoard", tt.window, err)
			}
		})
	}
}

func TestHistoryRefusesAnUnknownWindow(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	run := publishedRun(t, pool)
	if _, _, err := r.History(context.Background(), run, "glicko2_daneo", 0, "week"); err == nil || errors.Is(err, ErrNoPastBoard) {
		t.Errorf("History(week): err = %v, want an unknown window refused", err)
	}
}

func TestLeaderboardJoinsTheRunsCareerPeak(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := addRun(t, pool, "complete", time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC))
	other := addRun(t, pool, "complete", time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC))
	addRating(t, pool, run, "glicko2_daneo", 0, "ann", ptr(1), 1900)
	addRating(t, pool, run, "glicko2_daneo", 0, "bob", ptr(2), 1700)
	for _, p := range []struct {
		run  int64
		name string
	}{{run, "ann"}, {other, "bob"}} {
		addPeakTitle(t, pool, p.run, p.name, "GM", 2024)
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
			INSERT INTO player_badges (player_id, badge_id) SELECT $2, badge_id FROM b`, l, playerID(t, pool, name)); err != nil {
			t.Fatalf("giving %s the %s badge: %v", name, l, err)
		}
	}
}

func TestLeaderboardGivesEachPlayersBadgesAtOnce(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := addRun(t, pool, "complete", time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC))
	addRating(t, pool, run, "glicko2_daneo", 0, "ann", ptr(1), 1900)
	addRating(t, pool, run, "glicko2_daneo", 0, "bob", ptr(2), 1700)
	giveBadges(t, pool, "ann", "TD", "Organiser")

	got, err := r.Leaderboard(ctx, run, "glicko2_daneo", 0)
	if err != nil {
		t.Fatalf("Leaderboard: %v", err)
	}
	want := []ExtraBadge{{Label: "Organiser", Description: "Organiser description"}, {Label: "TD", Description: "TD description"}}
	if diff := cmp.Diff(want, got[0].Badges); diff != "" {
		t.Errorf("ann's badges, by label (-want +got):\n%s", diff)
	}
	if len(got[1].Badges) != 0 {
		t.Errorf("bob's badges = %+v, want none", got[1].Badges)
	}
}

func TestEveryReaderRefusesAnUnpublishedRun(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "Ann", "Bob")
	run := addRun(t, pool, "complete", time.Time{})
	r := NewRatingsReader(asReader(t, pool))
	readers := map[string]func() error{
		"Calibration":     func() error { _, err := r.Calibration(ctx, run); return err },
		"AnalysisRatings": func() error { _, err := r.AnalysisRatings(ctx, run, 0); return err },
		"ActivePlayers":   func() error { _, err := r.ActivePlayers(ctx, run); return err },
		"ComparedPlayers": func() error { _, err := r.ComparedPlayers(ctx, run, 0, nil); return err },
		"Activity":        func() error { _, err := r.Activity(ctx, run, day("2026-01-01")); return err },
		"Catalogue":       func() error { _, err := r.Catalogue(ctx, run); return err },
		"CatalogueTournament": func() error {
			_, err := r.CatalogueTournament(ctx, run, "wimbledon")
			return err
		},
		"CatalogueGames": func() error {
			_, err := r.CatalogueGames(ctx, run, "wimbledon", CatalogueGamesFilter{})
			return err
		},
		"RecentPodiums":        func() error { _, err := r.RecentPodiums(ctx, run, 12, 8); return err },
		"RunLastGame":          func() error { _, err := r.RunLastGame(ctx, run); return err },
		"Leaderboard":          func() error { _, err := r.Leaderboard(ctx, run, "glicko2_daneo", 0); return err },
		"History":              func() error { _, _, err := r.History(ctx, run, "glicko2_daneo", 0, "month"); return err },
		"TournamentSeries":     func() error { _, err := r.TournamentSeries(ctx, run, "international"); return err },
		"HallOfFame":           func() error { _, err := r.HallOfFame(ctx, run); return err },
		"PlayerAchievements":   func() error { _, _, _, err := r.PlayerAchievements(ctx, run, "Ann"); return err },
		"CommunityLeaderboard": func() error { _, err := r.CommunityLeaderboard(ctx, run, "2024"); return err },
		"YearBoard":            func() error { _, err := r.YearBoard(ctx, run, 2025, "glicko2_daneo", 0); return err },
		"Years":                func() error { _, err := r.Years(ctx, run); return err },
		"Player":               func() error { _, err := r.Player(ctx, run, "Ann"); return err },
		"PlayerHistory":        func() error { _, _, err := r.PlayerHistory(ctx, run, "Ann", 0); return err },
		"PlayerMatches":        func() error { _, _, err := r.PlayerMatches(ctx, run, "Ann", 0, 1, false); return err },
		"RatingsOf": func() error {
			_, err := r.RatingsOf(ctx, run, "glicko2_daneo", 0, []string{"Ann"})
			return err
		},
		"HistoricalRatingsOf": func() error {
			_, err := r.HistoricalRatingsOf(ctx, run, "glicko2_daneo", 0, []PlayerAt{{Name: "Ann", Month: "2024-05"}})
			return err
		},
		"PlayerRivals": func() error { _, _, err := r.PlayerRivals(ctx, run, "Ann", 0, true, false); return err },
		"PlayerCareer": func() error { _, _, _, err := r.PlayerCareer(ctx, run, "Ann", 0, "glicko2_daneo"); return err },
		"Versus":       func() error { _, _, _, err := r.Versus(ctx, run, "Ann", "Bob", true, false); return err },
		"PlayerMatrix": func() error { _, _, err := r.PlayerMatrix(ctx, run, "Ann", 0, nil); return err },
		"PlayerSeats":  func() error { _, err := r.PlayerSeats(ctx, run, "Ann", false); return err },
		"SeatAnalysis": func() error { _, err := r.SeatAnalysis(ctx, run, 0, "all"); return err },
	}
	for name, read := range readers {
		if err := read(); !errors.Is(err, ErrRunNotFound) {
			t.Errorf("%s of an unpublished run: err = %v, want ErrRunNotFound", name, err)
		}
	}
}

func TestHistoryLeavesOutHiddenPlayers(t *testing.T) {
	for _, window := range []string{"month", "last_update"} {
		t.Run(window, func(t *testing.T) {
			pool := migrated(t)
			r := NewRatingsReader(asReader(t, pool))
			addPlayers(t, pool, "ann", "cat")
			hideAccount(t, pool, "ann", "deleted")
			addDataLoad(t, pool, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "2026-09-30")
			before := publishedRun(t, pool)
			startedAt(t, pool, before, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC))
			addDataLoad(t, pool, time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), "2026-10-02")
			run := publishedRun(t, pool)
			startedAt(t, pool, run, time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC))
			for i, name := range []string{"ann", "cat"} {
				addRating(t, pool, before, "glicko2_daneo", 0, name, ptr(i+1), 1600)
				addWindow(t, pool, run, "glicko2_daneo", 0, "month", name, "2026-09-30", i+1, 1600)
			}

			_, rows, err := r.History(context.Background(), run, "glicko2_daneo", 0, window)
			if err != nil {
				t.Fatalf("History: %v", err)
			}
			if len(rows) != 1 || rows[0].Name != "cat" {
				t.Errorf("History(%s) = %+v, want cat only: ann's account is deleted", window, rows)
			}
		})
	}
}

func TestMigration77sDownTakesTheDataLoadsAwayAndUpBringsThemBack(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	present := func() []string { return rowsOf(t, pool, "SELECT coalesce(to_regclass('data_loads')::text, 'none')") }
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 76); return err }); err != nil {
		t.Fatalf("migrating down to 76: %v", err)
	}
	if got := present(); !slices.Equal(got, []string{"none"}) {
		t.Errorf("at 76, data_loads = %q, want none", got)
	}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.Up(ctx); return err }); err != nil {
		t.Fatalf("migrating up again: %v", err)
	}
	if got := present(); !slices.Equal(got, []string{"data_loads"}) {
		t.Errorf("up again: %q, want data_loads", got)
	}
}

func TestMigration76sDownTakesTheWindowBoardsAwayAndUpGivesEveryRunAPartition(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	var run int64
	if err := pool.QueryRow(ctx, "INSERT INTO rating_runs (engine_version, input_fingerprint) VALUES ('v1', 'f1') RETURNING run_id").Scan(&run); err != nil {
		t.Fatalf("adding a run: %v", err)
	}
	partitions := func() []string {
		return rowsOf(t, pool, `SELECT c.relname FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid
			WHERE i.inhparent = to_regclass('rating_windows') ORDER BY 1`)
	}
	want := []string{"rating_windows_run" + strconv.FormatInt(run, 10)}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 75); return err }); err != nil {
		t.Fatalf("migrating down to 75: %v", err)
	}
	if got := rowsOf(t, pool, "SELECT coalesce(to_regclass('rating_windows')::text, 'none')"); !slices.Equal(got, []string{"none"}) {
		t.Errorf("at 75, rating_windows = %q, want none", got)
	}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.Up(ctx); return err }); err != nil {
		t.Fatalf("migrating up again: %v", err)
	}
	if got := partitions(); !slices.Equal(got, want) {
		t.Errorf("up again: partitions %q, want %q", got, want)
	}
}
