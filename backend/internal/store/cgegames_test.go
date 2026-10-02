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
