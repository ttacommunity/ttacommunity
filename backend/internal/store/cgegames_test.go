//go:build integration

package store

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/cgegames"
	"ttacommunity/backend/internal/matchcsv"
)

// ourGame loads one placed game, its seats in this order, played on day.
func ourGame(t *testing.T, pool *pgxpool.Pool, line int, name, day string, seats ...matchcsv.Player) int64 {
	t.Helper()
	ctx := context.Background()
	var batch int64
	if err := pool.QueryRow(ctx, "INSERT INTO import_batches (source) VALUES ('admin') RETURNING batch_id").Scan(&batch); err != nil {
		t.Fatalf("starting a batch: %v", err)
	}
	var id int64
	err := pool.QueryRow(ctx, `
		INSERT INTO matches (batch_id, source_file, source_line, tournament, date, player_count, player1, score1, player2, score2, player3, score3)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING match_id`,
		batch, "m.csv", line, name, day, len(seats), seats[0].Name, seats[0].Score, seats[1].Name, seats[1].Score,
		seatName(seats, 2), seatScore(seats, 2)).Scan(&id)
	if err != nil {
		t.Fatalf("adding %s: %v", name, err)
	}
	if _, err := ClassifyMatches(ctx, pool); err != nil {
		t.Fatalf("ClassifyMatches: %v", err)
	}
	return id
}

func seatName(s []matchcsv.Player, i int) any {
	if i < len(s) {
		return s[i].Name
	}
	return nil
}

func seatScore(s []matchcsv.Player, i int) any {
	if i < len(s) {
		return s[i].Score
	}
	return nil
}

func cgeGame(finished string, seats ...cgegames.Seat) cgegames.Game {
	d, _ := time.Parse(time.DateOnly, finished)
	return cgegames.Game{Tournament: "international", CGETournament: 21, CGESeason: 7, Group: "Bronze 1", Number: 2, Finished: &d, Code: "TAPEZONA", Seats: seats}
}

const seatRows = `SELECT concat_ws('|', match_id, seat, player_name, outcome, cge_user, banned) FROM match_seats ORDER BY match_id, seat`

func TestAttachCGEGamesRecordsCGEsFactsOnTheMatchingGame(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	id := ourGame(t, pool, 2, "International S16", "2021-12-05",
		matchcsv.Player{Name: "snowcrush", Score: -5}, matchcsv.Player{Name: "rap2", Score: 215}, matchcsv.Player{Name: "elracha", Score: -2})

	got, err := AttachCGEGames(ctx, pool, []cgegames.Game{cgeGame("2021-12-02",
		cgegames.Seat{Name: "rap2", Score: 215, Outcome: cgegames.Finished, CGEUser: 152158},
		cgegames.Seat{Name: "elracha", Score: -2, Outcome: cgegames.Resigned, CGEUser: 124851, Banned: true},
		cgegames.Seat{Name: "snowcrush", Score: -5, Outcome: cgegames.TimedOut, CGEUser: 21731},
	)})
	if err != nil {
		t.Fatalf("AttachCGEGames: %v", err)
	}
	if got.Matched != 1 || len(got.Unmatched) != 0 {
		t.Errorf("AttachCGEGames = %+v, want 1 matched", got)
	}
	// Seats keep our order: snowcrush is our player 1.
	want := []string{
		joinf(id, 1, "snowcrush|timed_out|21731|f"), joinf(id, 2, "rap2|finished|152158|f"), joinf(id, 3, "elracha|resigned|124851|t"),
	}
	if rows := rowsOf(t, pool, seatRows); !slices.Equal(rows, want) {
		t.Errorf("seats = %q, want %q", rows, want)
	}
	game := rowsOf(t, pool, `SELECT concat_ws('|', cge_tournament, cge_season, cge_finished, replay_code, group_label, game_number, date, score1) FROM matches`)
	if want := []string{"21|7|2021-12-02|TAPEZONA|Bronze 1|2|2021-12-05|-5"}; !slices.Equal(game, want) {
		t.Errorf("game = %q, want %q (our date and scores unchanged)", game, want)
	}
}

func TestAttachCGEGamesLeavesAGameWithOtherScoresUnmatched(t *testing.T) {
	pool := migrated(t)
	ourGame(t, pool, 2, "International S16", "2021-12-05", matchcsv.Player{Name: "ann", Score: 100}, matchcsv.Player{Name: "bob", Score: 90})

	got, err := AttachCGEGames(context.Background(), pool, []cgegames.Game{cgeGame("2021-12-02",
		cgegames.Seat{Name: "ann", Score: 101, Outcome: cgegames.Finished, CGEUser: 1},
		cgegames.Seat{Name: "bob", Score: 90, Outcome: cgegames.Finished, CGEUser: 2},
	)})
	if err != nil {
		t.Fatalf("AttachCGEGames: %v", err)
	}
	if got.Matched != 0 || len(got.Unmatched) != 1 || count(t, pool, "match_seats") != 0 {
		t.Errorf("AttachCGEGames = %+v, want the game unmatched and no seats", got)
	}
}

func TestAttachCGEGamesPairsTwinGamesByTheNearestDay(t *testing.T) {
	pool := migrated(t)
	ann, bob := matchcsv.Player{Name: "ann", Score: 100}, matchcsv.Player{Name: "bob", Score: 90}
	early := ourGame(t, pool, 2, "International S16", "2021-11-01", ann, bob)
	late := ourGame(t, pool, 3, "International S16", "2021-12-20", ann, bob)
	seat := func(n string, s float64) cgegames.Seat {
		return cgegames.Seat{Name: n, Score: s, Outcome: cgegames.Finished, CGEUser: 1}
	}
	g1, g2 := cgeGame("2021-12-18", seat("ann", 100), seat("bob", 90)), cgeGame("2021-11-03", seat("ann", 100), seat("bob", 90))
	g1.Code, g2.Code = "LATE", "EARLY"

	if _, err := AttachCGEGames(context.Background(), pool, []cgegames.Game{g1, g2}); err != nil {
		t.Fatalf("AttachCGEGames: %v", err)
	}
	got := rowsOf(t, pool, "SELECT concat_ws('|', match_id, replay_code) FROM matches ORDER BY match_id")
	if want := []string{joinf(early, "EARLY"), joinf(late, "LATE")}; !slices.Equal(got, want) {
		t.Errorf("codes = %q, want %q", got, want)
	}
}

func TestAttachCGEGamesAgainReplacesTheSeats(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	ourGame(t, pool, 2, "International S16", "2021-12-05", matchcsv.Player{Name: "ann", Score: 100}, matchcsv.Player{Name: "bob", Score: 90})
	g := cgeGame("2021-12-02",
		cgegames.Seat{Name: "ann", Score: 100, Outcome: cgegames.Finished, CGEUser: 1},
		cgegames.Seat{Name: "bob", Score: 90, Outcome: cgegames.Finished, CGEUser: 2})
	for range 2 {
		if _, err := AttachCGEGames(ctx, pool, []cgegames.Game{g}); err != nil {
			t.Fatalf("AttachCGEGames: %v", err)
		}
	}
	if n := count(t, pool, "match_seats"); n != 2 {
		t.Errorf("seats after two attaches = %d, want 2", n)
	}
}

func TestAttachCGEGamesKeepsAGroupTheGameAlreadyHas(t *testing.T) {
	pool := migrated(t)
	ourGame(t, pool, 2, "International S30 - Gold 1 game 4", "2021-12-05", matchcsv.Player{Name: "ann", Score: 100}, matchcsv.Player{Name: "bob", Score: 90})
	g := cgeGame("2021-12-02",
		cgegames.Seat{Name: "ann", Score: 100, Outcome: cgegames.Finished, CGEUser: 1},
		cgegames.Seat{Name: "bob", Score: 90, Outcome: cgegames.Finished, CGEUser: 2})

	if _, err := AttachCGEGames(context.Background(), pool, []cgegames.Game{g}); err != nil {
		t.Fatalf("AttachCGEGames: %v", err)
	}
	if got := rowsOf(t, pool, "SELECT concat_ws('|', group_label, game_number) FROM matches"); !slices.Equal(got, []string{"Gold 1|4"}) {
		t.Errorf("group = %q, want the one the name gives", got)
	}
}

func TestAttachCGEGamesCountsNeverPlayedGamesApart(t *testing.T) {
	pool := migrated(t)
	g := cgeGame("2021-12-02",
		cgegames.Seat{Name: "ann", Score: -5, Outcome: cgegames.TimedOut, CGEUser: 1},
		cgegames.Seat{Name: "bob", Score: -5, Outcome: cgegames.TimedOut, CGEUser: 2})
	g.Unplayed, g.Finished = true, nil

	got, err := AttachCGEGames(context.Background(), pool, []cgegames.Game{g})
	if err != nil {
		t.Fatalf("AttachCGEGames: %v", err)
	}
	if got.NeverPlayed != 1 || got.Matched != 0 || len(got.Unmatched) != 0 {
		t.Errorf("AttachCGEGames = %+v, want 1 never played, set apart", got)
	}
}

// joinf joins values with "|", as concat_ws does in the queries above.
func joinf(parts ...any) string {
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = fmt.Sprint(p)
	}
	return strings.Join(out, "|")
}

func TestAttachCGEGamesTakesAnUnfinishedSeatAtWhateverScoreOurFileGave(t *testing.T) {
	pool := migrated(t)
	// Older match files score a timeout -1 and a resignation 0; CGE's convention is -5 and minus the place.
	ourGame(t, pool, 2, "International S16", "2021-12-05",
		matchcsv.Player{Name: "ann", Score: 180}, matchcsv.Player{Name: "bob", Score: 0}, matchcsv.Player{Name: "cat", Score: -1})

	got, err := AttachCGEGames(context.Background(), pool, []cgegames.Game{cgeGame("2021-12-02",
		cgegames.Seat{Name: "ann", Score: 180, Outcome: cgegames.Finished, CGEUser: 1},
		cgegames.Seat{Name: "bob", Score: -2, Outcome: cgegames.Resigned, CGEUser: 2},
		cgegames.Seat{Name: "cat", Score: -5, Outcome: cgegames.TimedOut, CGEUser: 3},
	)})
	if err != nil {
		t.Fatalf("AttachCGEGames: %v", err)
	}
	if got.Matched != 1 {
		t.Errorf("AttachCGEGames = %+v, want the game matched", got)
	}
	if s := rowsOf(t, pool, "SELECT concat_ws('|', score2, score3) FROM matches"); !slices.Equal(s, []string{"0|-1"}) {
		t.Errorf("our scores = %q, want them unchanged", s)
	}
}

func TestAttachCGEGamesStillNeedsAFinishedSeatsScore(t *testing.T) {
	pool := migrated(t)
	ourGame(t, pool, 2, "International S16", "2021-12-05", matchcsv.Player{Name: "ann", Score: 180}, matchcsv.Player{Name: "bob", Score: 5})

	got, err := AttachCGEGames(context.Background(), pool, []cgegames.Game{cgeGame("2021-12-02",
		cgegames.Seat{Name: "ann", Score: 180, Outcome: cgegames.Finished, CGEUser: 1},
		cgegames.Seat{Name: "bob", Score: -5, Outcome: cgegames.TimedOut, CGEUser: 2},
	)})
	if err != nil {
		t.Fatalf("AttachCGEGames: %v", err)
	}
	if got.Matched != 0 {
		t.Errorf("AttachCGEGames = %+v: a timed-out seat matched a positive score", got)
	}
}

// newCGE is a CGE game placed in our international season "35", which holds no games yet.
func newCGE(group string, number int, finished string, seats ...cgegames.Seat) cgegames.Game {
	g := cgeGame(finished, seats...)
	g.CGESeason, g.Group, g.Number, g.Season, g.Code = 26, group, number, "35", fmt.Sprintf("NEW%s%d", group, number)
	g.Name = fmt.Sprintf("International S35 - %s game %d", group, number)
	return g
}

func played(name string, score float64, user int) cgegames.Seat {
	return cgegames.Seat{Name: name, Score: score, Outcome: cgegames.Finished, CGEUser: user}
}

func timedOut(name string, user int) cgegames.Seat {
	return cgegames.Seat{Name: name, Score: -5, Outcome: cgegames.TimedOut, CGEUser: user}
}

const addedRows = `
	SELECT concat_ws('|', m.tournament, m.date, m.state, s.label, m.group_label, m.game_number, m.cge_season,
	                 m.player1, m.score1, m.player2, m.score2)
	FROM matches m JOIN tournament_stages st USING (stage_id) JOIN tournament_seasons s USING (season_id)
	WHERE m.cge_tournament IS NOT NULL ORDER BY m.date, m.game_number`

func TestAddCGEGamesAddsAGameOfASeasonWeHoldNoneOf(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	ourGame(t, pool, 2, "International S34", "2026-08-13", matchcsv.Player{Name: "ann", Score: 1}, matchcsv.Player{Name: "bob", Score: 0})

	got, err := AddCGEGames(ctx, pool, []cgegames.Game{newCGE("Gold 1", 3, "2026-09-20", played("cat", 200, 3), played("ann", 150, 1))}, nil)
	if err != nil {
		t.Fatalf("AddCGEGames: %v", err)
	}
	if got.Added != 1 {
		t.Errorf("AddCGEGames = %+v, want 1 added", got)
	}
	if rows := rowsOf(t, pool, addedRows); !slices.Equal(rows, []string{"International S35 - Gold 1 game 3|2026-09-20|played|35|Gold 1|3|26|cat|200|ann|150"}) {
		t.Errorf("added = %q", rows)
	}
	if n := count(t, pool, "match_seats"); n != 2 {
		t.Errorf("seats = %d, want 2", n)
	}
	if p := rowsOf(t, pool, "SELECT name FROM players WHERE name = 'cat'"); len(p) != 1 {
		t.Errorf("the new player cat was not added: %q", p)
	}
}

// catalogued fills the tournament catalogue, as the load does before any game is added.
func catalogued(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := ClassifyMatches(context.Background(), pool); err != nil {
		t.Fatalf("ClassifyMatches: %v", err)
	}
}

func TestAddCGEGamesAddsANeverPlayedGameDatedByItsGroup(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	catalogued(t, pool)
	never := newCGE("Gold 1", 2, "2026-01-01", timedOut("ann", 1), timedOut("bob", 2))
	never.Unplayed, never.Finished = true, nil

	got, err := AddCGEGames(ctx, pool, []cgegames.Game{
		newCGE("Gold 1", 1, "2026-09-10", played("ann", 100, 1), played("bob", 90, 2)),
		never,
		newCGE("Gold 1", 3, "2026-09-25", played("bob", 100, 2), played("ann", 90, 1)),
	}, nil)
	if err != nil {
		t.Fatalf("AddCGEGames: %v", err)
	}
	if got.Added != 3 || got.NeverPlayed != 1 {
		t.Errorf("AddCGEGames = %+v, want 3 added, 1 of them never played", got)
	}
	rows := rowsOf(t, pool, `SELECT concat_ws('|', game_number, state, date) FROM matches WHERE game_number = 2`)
	if !slices.Equal(rows, []string{"2|never_played|2026-09-25"}) {
		t.Errorf("never-played game = %q, want state never_played dated by its group's last game", rows)
	}
}

func TestAddCGEGamesReportsAGameThatMayBeACopyOfOurs(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	ourGame(t, pool, 2, "International S16", "2021-12-20", matchcsv.Player{Name: "ann", Score: 100}, matchcsv.Player{Name: "bob", Score: 90})
	early := cgeGame("2021-12-01", played("cat", 100, 3), played("dan", 90, 4))
	early.Season, early.Name, early.Code = "16", "International S16 - Bronze 1 game 2", "EARLY"
	same := cgeGame("2022-01-10", played("ann", 101, 1), played("bob", 90, 2))
	same.Season, same.Name, same.Code, same.Number = "16", "International S16 - Bronze 1 game 3", "SAME", 3

	got, err := AddCGEGames(ctx, pool, []cgegames.Game{early, same}, nil)
	if err != nil {
		t.Fatalf("AddCGEGames: %v", err)
	}
	// Before our last game of the season, or with the same players as one of ours: either may be ours under other scores.
	if got.Added != 0 || len(got.Conflicts) != 2 {
		t.Errorf("AddCGEGames = %+v, want both reported, none added", got)
	}
}

func TestAddCGEGamesAddsAPossibleCopyConfirmedNew(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	ourGame(t, pool, 2, "International S16", "2021-12-20", matchcsv.Player{Name: "ann", Score: 100}, matchcsv.Player{Name: "bob", Score: 90})
	early := cgeGame("2021-12-01", played("cat", 100, 3), played("dan", 90, 4))
	early.Season, early.Name, early.Code = "16", "International S16 - Bronze 1 game 2", "EARLY"
	same := cgeGame("2022-01-10", played("ann", 101, 1), played("bob", 90, 2))
	same.Season, same.Name, same.Code, same.Number = "16", "International S16 - Bronze 1 game 3", "SAME", 3

	got, err := AddCGEGames(ctx, pool, []cgegames.Game{early, same}, map[string]bool{"SAME": true})
	if err != nil {
		t.Fatalf("AddCGEGames: %v", err)
	}
	if got.Added != 1 || len(got.Conflicts) != 1 || got.Conflicts[0].Code != "EARLY" {
		t.Errorf("AddCGEGames = %+v, want SAME added as confirmed new, EARLY still reported", got)
	}
}

func TestAddCGEGamesAddsALaterGameOfASeasonWeHold(t *testing.T) {
	pool := migrated(t)
	ourGame(t, pool, 2, "International S16", "2021-12-20", matchcsv.Player{Name: "ann", Score: 100}, matchcsv.Player{Name: "bob", Score: 90})
	later := cgeGame("2022-01-10", played("cat", 100, 3), played("dan", 90, 4))
	later.Season, later.Name = "16", "International S16 - Bronze 1 game 2"

	got, err := AddCGEGames(context.Background(), pool, []cgegames.Game{later}, nil)
	if err != nil {
		t.Fatalf("AddCGEGames: %v", err)
	}
	if got.Added != 1 {
		t.Errorf("AddCGEGames = %+v, want the later game added", got)
	}
}

func TestAddCGEGamesAgainAddsNothingMore(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	catalogued(t, pool)
	never := newCGE("Gold 1", 2, "", timedOut("ann", 1), timedOut("bob", 2))
	never.Unplayed, never.Finished = true, nil
	games := []cgegames.Game{newCGE("Gold 1", 1, "2026-09-10", played("ann", 100, 1), played("bob", 90, 2)), never}
	for range 2 {
		if _, err := AddCGEGames(ctx, pool, games, nil); err != nil {
			t.Fatalf("AddCGEGames: %v", err)
		}
	}
	if n := count(t, pool, "matches"); n != 2 {
		t.Errorf("matches after adding twice = %d, want 2", n)
	}
}

func TestAddCGEGamesNeverAddsAGameWithoutASettledPlace(t *testing.T) {
	pool := migrated(t)
	g := newCGE("Gold 1", 1, "2026-09-10", played("ann", 100, 1), played("bob", 90, 2))
	g.Name, g.Season = "", ""

	got, err := AddCGEGames(context.Background(), pool, []cgegames.Game{g}, nil)
	if err != nil {
		t.Fatalf("AddCGEGames: %v", err)
	}
	if got.Added != 0 || got.Unsettled != 1 || count(t, pool, "matches") != 0 {
		t.Errorf("AddCGEGames = %+v, want it counted unsettled and not added", got)
	}
}

func TestAddCGEGamesRefusesATournamentTheCatalogueLacks(t *testing.T) {
	pool := migrated(t)
	g := newCGE("Gold 1", 1, "2026-09-10", played("ann", 100, 1), played("bob", 90, 2))
	g.Tournament = "snail_cup"

	if _, err := AddCGEGames(context.Background(), pool, []cgegames.Game{g}, nil); err == nil || !strings.Contains(err.Error(), "snail_cup") {
		t.Errorf("AddCGEGames(unknown tournament) = %v, want an error naming snail_cup", err)
	}
}

func TestAttachCGEGamesKnowsAGameWhosePlayerWasRenamed(t *testing.T) {
	pool := migrated(t)
	id := ourGame(t, pool, 2, "International S16", "2021-12-05",
		matchcsv.Player{Name: "gigilamoroso", Score: 244}, matchcsv.Player{Name: "peracottari", Score: 185})

	g := cgeGame("2021-12-02", played("gigi lamoroso", 244, 7), played("peracottari", 185, 8))
	g.Season = "16"
	got, err := AttachCGEGames(context.Background(), pool, []cgegames.Game{g})
	if err != nil {
		t.Fatalf("AttachCGEGames: %v", err)
	}
	if got.Matched != 1 || got.Renamed != 1 {
		t.Errorf("AttachCGEGames = %+v, want 1 matched, 1 renamed", got)
	}
	want := []string{joinf(id, 1, "gigilamoroso|gigi lamoroso|7"), joinf(id, 2, "peracottari|peracottari|8")}
	if rows := rowsOf(t, pool, `SELECT concat_ws('|', match_id, seat, player_name, cge_name, cge_user) FROM match_seats ORDER BY seat`); !slices.Equal(rows, want) {
		t.Errorf("seats = %q, want %q (our name, then CGE's)", rows, want)
	}
}

func TestAttachCGEGamesKnowsARenameOnlyInTheSameSeason(t *testing.T) {
	for _, season := range []string{"17", ""} {
		pool := migrated(t)
		ourGame(t, pool, 2, "International S16", "2021-12-05",
			matchcsv.Player{Name: "Voidray", Score: 55}, matchcsv.Player{Name: "LittleBob", Score: 0})
		g := cgeGame("2021-12-05", played("FreddyCrugerKiller", 55, 7), cgegames.Seat{Name: "LittleBob", Score: -2, Outcome: cgegames.Resigned, CGEUser: 8})
		g.Season = season

		got, err := AttachCGEGames(context.Background(), pool, []cgegames.Game{g})
		if err != nil {
			t.Fatalf("AttachCGEGames: %v", err)
		}
		if got.Matched != 0 || got.Renamed != 0 {
			t.Errorf("CGE game placed in season %q: %+v, want no match with our S16 game", season, got)
		}
	}
}

func TestAttachCGEGamesMatchesExactlyBeforeByRename(t *testing.T) {
	pool := migrated(t)
	id := ourGame(t, pool, 2, "International S16", "2021-12-05",
		matchcsv.Player{Name: "Magesolo", Score: 0}, matchcsv.Player{Name: "luckrider", Score: -1})
	// The first CGE game fits ours under a rename; the second is ours exactly, and must win it.
	rename := cgeGame("2021-12-04", played("Magesolo", 0, 1), cgegames.Seat{Name: "Ritocito", Score: -2, Outcome: cgegames.Resigned, CGEUser: 2})
	rename.Season, rename.Code = "16", "COJAZABE"
	exact := cgeGame("2021-12-05", cgegames.Seat{Name: "Magesolo", Score: -1, Outcome: cgegames.Resigned, CGEUser: 1}, timedOut("luckrider", 3))
	exact.Season, exact.Code, exact.Number = "16", "KITINAGE", 3

	got, err := AttachCGEGames(context.Background(), pool, []cgegames.Game{rename, exact})
	if err != nil {
		t.Fatalf("AttachCGEGames: %v", err)
	}
	var code string
	if err := pool.QueryRow(context.Background(), "SELECT replay_code FROM matches WHERE match_id = $1", id).Scan(&code); err != nil {
		t.Fatalf("reading the game: %v", err)
	}
	if got.Matched != 1 || got.Renamed != 0 || code != "KITINAGE" {
		t.Errorf("AttachCGEGames = %+v, code %q; want ours matched exactly to KITINAGE", got, code)
	}
}

func TestAttachCGEGamesMatchesAPlacedGameOnlyInItsSeason(t *testing.T) {
	pool := migrated(t)
	ourGame(t, pool, 2, "International S16", "2021-12-05", matchcsv.Player{Name: "ann", Score: 100}, matchcsv.Player{Name: "bob", Score: 90})
	g := cgeGame("2021-12-05", played("ann", 100, 1), played("bob", 90, 2))
	g.Season = "17"
	got, err := AttachCGEGames(context.Background(), pool, []cgegames.Game{g})
	if err != nil {
		t.Fatalf("AttachCGEGames: %v", err)
	}
	if got.Matched != 0 {
		t.Errorf("AttachCGEGames = %+v, want a game placed in S17 not matched to our S16 game", got)
	}
}

func TestAttachCGEGamesClearsAGameThatNoLongerMatches(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	id := ourGame(t, pool, 2, "International S16", "2021-12-05",
		matchcsv.Player{Name: "ann", Score: 100}, matchcsv.Player{Name: "bob", Score: 90})
	first := cgeGame("2021-12-05", played("ann", 100, 1), played("bob", 90, 2))
	if _, err := AttachCGEGames(ctx, pool, []cgegames.Game{first}); err != nil {
		t.Fatalf("AttachCGEGames(first): %v", err)
	}
	// The same CGE game, read again, turns out to be other players': ours keeps no trace of it.
	again := cgeGame("2021-12-05", played("cat", 100, 3), played("dan", 90, 4))
	if _, err := AttachCGEGames(ctx, pool, []cgegames.Game{again}); err != nil {
		t.Fatalf("AttachCGEGames(again): %v", err)
	}
	var cge *int32
	var code *string
	if err := pool.QueryRow(ctx, "SELECT cge_tournament, replay_code FROM matches WHERE match_id = $1", id).Scan(&cge, &code); err != nil {
		t.Fatalf("reading the game: %v", err)
	}
	if cge != nil || code != nil || count(t, pool, "match_seats") != 0 {
		t.Errorf("game after a match that no longer holds: CGE tournament %v, code %v, %d seats; want none", cge, code, count(t, pool, "match_seats"))
	}
}

func TestAttachCGEGamesNeedsAllButOnePlayer(t *testing.T) {
	pool := migrated(t)
	ourGame(t, pool, 2, "International S16", "2021-12-05",
		matchcsv.Player{Name: "ann", Score: 244}, matchcsv.Player{Name: "bob", Score: 185}, matchcsv.Player{Name: "cat", Score: 100})

	got, err := AttachCGEGames(context.Background(), pool, []cgegames.Game{cgeGame("2021-12-02",
		played("ann2", 244, 1), played("bob2", 185, 2), played("cat", 100, 3))})
	if err != nil {
		t.Fatalf("AttachCGEGames: %v", err)
	}
	if got.Matched != 0 {
		t.Errorf("AttachCGEGames = %+v: two names differ, want no match", got)
	}
}

func TestAddCGEGamesReportsAGameOfOursWithARenamedPlayer(t *testing.T) {
	pool := migrated(t)
	ourGame(t, pool, 2, "International S16", "2021-12-05",
		matchcsv.Player{Name: "gigilamoroso", Score: 244}, matchcsv.Player{Name: "peracottari", Score: 185}, matchcsv.Player{Name: "fraction", Score: 123})
	later := cgeGame("2022-01-10", played("gigi lamoroso", 244, 7), played("peracottari", 185, 8), played("fraction", 123, 9))
	later.Season, later.Name = "16", "International S16 - Bronze 1 game 2"

	got, err := AddCGEGames(context.Background(), pool, []cgegames.Game{later}, nil)
	if err != nil {
		t.Fatalf("AddCGEGames: %v", err)
	}
	if got.Added != 0 || len(got.Conflicts) != 1 {
		t.Errorf("AddCGEGames = %+v, want it reported, not added", got)
	}
}

func TestAttachCGEGamesKnowsAGameOursRecordsWithoutATimedOutPlayer(t *testing.T) {
	pool := migrated(t)
	id := ourGame(t, pool, 2, "International S18", "2022-06-05",
		matchcsv.Player{Name: "Benni", Score: 240}, matchcsv.Player{Name: "Jans", Score: 229}, matchcsv.Player{Name: "Lammoth", Score: 208})

	got, err := AttachCGEGames(context.Background(), pool, []cgegames.Game{cgeGame("2022-06-27",
		played("Benni", 240, 1), played("Jans", 229, 2), played("Lammoth", 208, 3), timedOut("gigi lamoroso", 4))})
	if err != nil {
		t.Fatalf("AttachCGEGames: %v", err)
	}
	if got.Matched != 1 || got.LacksTimedOut != 1 {
		t.Errorf("AttachCGEGames = %+v, want 1 matched, lacking a timed-out seat", got)
	}
	if rows := rowsOf(t, pool, `SELECT concat_ws('|', match_id, seat, player_name) FROM match_seats ORDER BY seat`); len(rows) != 3 || rows[0] != joinf(id, 1, "Benni") {
		t.Errorf("seats = %q, want our three", rows)
	}
}

func TestAddCGEGamesReportsAGameOursRecordsWithoutATimedOutPlayer(t *testing.T) {
	pool := migrated(t)
	ourGame(t, pool, 2, "International S18", "2022-06-05",
		matchcsv.Player{Name: "Benni", Score: 240}, matchcsv.Player{Name: "Jans", Score: 229}, matchcsv.Player{Name: "Lammoth", Score: 208})
	later := cgeGame("2022-06-27", played("Benni", 240, 1), played("Jans", 229, 2), played("Lammoth", 208, 3), timedOut("gigi lamoroso", 4))
	later.Season, later.Name = "18", "International S18 - Silver 11 game 9"

	got, err := AddCGEGames(context.Background(), pool, []cgegames.Game{later}, nil)
	if err != nil {
		t.Fatalf("AddCGEGames: %v", err)
	}
	if got.Added != 0 || len(got.Conflicts) != 1 {
		t.Errorf("AddCGEGames = %+v, want it reported, not added", got)
	}
}

func TestAttachCGEGamesKnowsARenamedPlayerWhoTimedOutUnderTheOlderScore(t *testing.T) {
	pool := migrated(t)
	ourGame(t, pool, 2, "International S19", "2022-09-04",
		matchcsv.Player{Name: "jcc", Score: 267}, matchcsv.Player{Name: "Cleveman", Score: 256}, matchcsv.Player{Name: "gigilamoroso", Score: -1})

	g := cgeGame("2022-10-23", played("jcc", 267, 1), played("Cleveman", 256, 2), timedOut("gigi lamoroso", 4))
	g.Season = "19"
	got, err := AttachCGEGames(context.Background(), pool, []cgegames.Game{g})
	if err != nil {
		t.Fatalf("AttachCGEGames: %v", err)
	}
	if got.Matched != 1 || got.Renamed != 1 {
		t.Errorf("AttachCGEGames = %+v, want 1 matched, renamed", got)
	}
}
