//go:build integration

package store

// Test data builders: each sets up one fact, through the store's own writers where there is one,
// so a schema change is fixed here and not in every test.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/tournamentcsv"
)

// setTournament changes a series' settings as admin does.
func setTournament(t *testing.T, pool *pgxpool.Pool, slug string, s TournamentSettings) {
	t.Helper()
	if err := NewAdminStore(pool).SetTournament(context.Background(), slug, s); err != nil {
		t.Fatalf("SetTournament(%s): %v", slug, err)
	}
}

// addPlacements records organisers' placements, adding their players, and replacing any recorded before.
func addPlacements(t *testing.T, pool *pgxpool.Pool, places ...tournamentcsv.Placement) {
	t.Helper()
	var names []string
	for _, p := range places {
		names = append(names, p.Player)
	}
	ensurePlayers(t, pool, names...)
	got, err := ReplacePlacements(context.Background(), pool, places)
	if err != nil || got.Unknown != 0 {
		t.Fatalf("ReplacePlacements = %+v, %v; want every placement written", got, err)
	}
}

// ensurePlayers adds each named player that is not one yet.
func ensurePlayers(t *testing.T, pool *pgxpool.Pool, names ...string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"INSERT INTO players (name) SELECT unnest($1::text[]) ON CONFLICT (name) DO NOTHING", names); err != nil {
		t.Fatalf("adding players %q: %v", names, err)
	}
}

// addCGETournaments loads CGE's catalogue, replacing any loaded before.
func addCGETournaments(t *testing.T, pool *pgxpool.Pool, rows ...tournamentcsv.CGETournament) {
	t.Helper()
	if _, err := ReplaceCGETournaments(context.Background(), pool, rows); err != nil {
		t.Fatalf("ReplaceCGETournaments: %v", err)
	}
}

// showCGETournament lists a CGE tournament that is in no series.
func showCGETournament(t *testing.T, pool *pgxpool.Pool, id int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "UPDATE cge_tournaments SET visible = true WHERE cge_tournament = $1", id); err != nil {
		t.Fatalf("showing CGE tournament %d: %v", id, err)
	}
}

// setGameState sets the state of every game named like pattern (SQL LIKE).
func setGameState(t *testing.T, pool *pgxpool.Pool, pattern, state string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "UPDATE games SET state = $2 WHERE name LIKE $1", pattern, state); err != nil {
		t.Fatalf("setting %s %s: %v", pattern, state, err)
	}
}

// setOutcome records how a player's game ended (finished, resigned or timed_out) in every game named like pattern;
// a timed-out player has no culture.
func setOutcome(t *testing.T, pool *pgxpool.Pool, pattern, player, outcome string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		UPDATE seats s SET outcome = $3, culture = CASE WHEN $3 = 'timed_out' THEN 0 ELSE s.culture END
		FROM games g WHERE g.game_id = s.game_id AND g.name LIKE $1 AND s.player_name = $2`, pattern, player, outcome); err != nil {
		t.Fatalf("setting %s's outcome in %s: %v", player, pattern, err)
	}
}

// setShareCode gives the game named name CGE's share code.
func setShareCode(t *testing.T, pool *pgxpool.Pool, name, code string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "UPDATE games SET share_code = $2 WHERE name = $1", name, code); err != nil {
		t.Fatalf("coding %s: %v", name, err)
	}
}

// addPeakTitle records the engine's peak title for a player in a run.
func addPeakTitle(t *testing.T, pool *pgxpool.Pool, run int64, name, title string, year int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"INSERT INTO player_peak_titles (run_id, player_name, peak_title, peak_year) VALUES ($1, $2, $3, $4)",
		run, name, title, year); err != nil {
		t.Fatalf("adding %s's peak title: %v", name, err)
	}
}

// date returns a pointer to the day s, for optional dates in test data.
func date(s string) *time.Time { return ptr(day(s)) }
