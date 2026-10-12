//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// blitzRoundOne is blitzWithEntrants with round 1 published: Ann v Bob, Cy v Dee. It returns the admin store,
// the reader, the entrants' ids and the two matches' ids.
func blitzRoundOne(t *testing.T) (*AdminStore, *RatingsReader, []int64, []int64) {
	t.Helper()
	a, reader, ids := blitzWithEntrants(t)
	ctx := context.Background()
	r, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Seed: 7, Matches: []BlitzMatch{{A: ids[0], B: &ids[1]}, {A: ids[2], B: &ids[3]}}})
	if err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", r.ID); err != nil {
		t.Fatalf("PublishBlitzRound: %v", err)
	}
	return a, reader, ids, []int64{r.Matches[0].ID, r.Matches[1].ID}
}

func finished(culture float64) BlitzSide { return BlitzSide{Culture: culture, Outcome: "finished"} }

var played = time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)

// matchOf finds a match in an edition's rounds.
func matchOf(t *testing.T, ed BlitzEdition, id int64) BlitzMatch {
	t.Helper()
	for _, r := range ed.Rounds {
		for _, m := range r.Matches {
			if m.ID == id {
				return m
			}
		}
	}
	t.Fatalf("match %d is in no round", id)
	return BlitzMatch{}
}

// resultLine writes a match's result as "result game=<code> <a culture>/<a outcome> <b culture>/<b outcome> ruling=<reason>".
func resultLine(m BlitzMatch) string {
	line := m.Result
	if m.Game != nil {
		line += fmt.Sprintf(" game=%s %g/%s %g/%s on %s", m.Game.ShareCode, m.Game.A.Culture, m.Game.A.Outcome, m.Game.B.Culture, m.Game.B.Outcome, m.Game.Played.Format("2006-01-02"))
	}
	if m.Ruling != "" {
		line += " ruling=" + m.Ruling
	}
	return line
}

func TestBlitzGamesAreRecordedCorrectedAndRead(t *testing.T) {
	a, reader, _, matches := blitzRoundOne(t)
	ctx := context.Background()
	annBob, cyDee := matches[0], matches[1]
	if err := a.RecordBlitzGame(ctx, "us_open", "2026", annBob, BlitzGame{ShareCode: "ABCD1234", A: finished(250), B: finished(200), Played: played}); err != nil {
		t.Fatalf("RecordBlitzGame: %v", err)
	}
	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	if ed.TiebreakSeed == 0 {
		t.Errorf("the edition's tiebreak seed = 0, want one drawn when its row was made")
	}
	m := matchOf(t, ed, annBob)
	if want := "a game=ABCD1234 250/finished 200/finished on 2026-10-14"; resultLine(m) != want || m.GameID == nil || m.DecidedAt == nil {
		t.Fatalf("Ann v Bob = %q (game %v, decided %v), want %q with a game id and a time", resultLine(m), m.GameID, m.DecidedAt, want)
	}
	gameID := *m.GameID

	var name, stage, group, state, code, date string
	var seats []string
	if err := a.pool.QueryRow(ctx, `
		SELECT g.name, st.label, gr.name, g.state, g.share_code, g.date::text,
		       array_agg(s.player_name || ' ' || s.culture || ' ' || s.place || ' ' || s.outcome ORDER BY s.seat)
		FROM games g JOIN groups gr USING (group_id) JOIN stages st USING (stage_id) JOIN seats s USING (game_id)
		WHERE g.game_id = $1 GROUP BY 1, 2, 3, 4, 5, 6`, gameID).Scan(&name, &stage, &group, &state, &code, &date, &seats); err != nil {
		t.Fatalf("reading the game: %v", err)
	}
	if got := fmt.Sprint(name, "|", stage, "|", group, "|", state, "|", code, "|", date, "|", seats); got != "US Open 2026 Round 1|Round 1||played|ABCD1234|2026-10-14|[Ann 250 1 finished Bob 200 2 finished]" {
		t.Errorf("the game = %s, want it under stage Round 1, played, with Ann first and Bob second", got)
	}
	var players int
	if err := a.pool.QueryRow(ctx, "SELECT count(*) FROM players WHERE name IN ('Ann', 'Bob')").Scan(&players); err != nil || players != 2 {
		t.Errorf("players Ann and Bob = %d, %v; want both (Ann added, Bob kept)", players, err)
	}

	// A correction changes the one game: Bob wins now, then equal cultures draw, with no second game row.
	if err := a.RecordBlitzGame(ctx, "us_open", "2026", annBob, BlitzGame{ShareCode: "ABCD1234", A: finished(250), B: finished(260), Played: played}); err != nil {
		t.Fatalf("RecordBlitzGame(correction): %v", err)
	}
	if ed, _ = a.BlitzEdition(ctx, "us_open", "2026"); resultLine(matchOf(t, ed, annBob)) != "b game=ABCD1234 250/finished 260/finished on 2026-10-14" {
		t.Errorf("after the correction: %q, want Bob's win", resultLine(matchOf(t, ed, annBob)))
	}
	if err := a.RecordBlitzGame(ctx, "us_open", "2026", annBob, BlitzGame{ShareCode: "ABCD1234", A: finished(250), B: finished(250), Played: played}); err != nil {
		t.Fatalf("RecordBlitzGame(draw): %v", err)
	}
	ed, _ = a.BlitzEdition(ctx, "us_open", "2026")
	if m := matchOf(t, ed, annBob); m.Result != "draw" || *m.GameID != gameID {
		t.Errorf("after the draw: %q game %v, want draw on game %d", m.Result, m.GameID, gameID)
	}
	var games int
	if err := a.pool.QueryRow(ctx, "SELECT count(*) FROM games").Scan(&games); err != nil || games != 1 {
		t.Errorf("games after three recordings = %d, %v; want 1", games, err)
	}
	if err := a.pool.QueryRow(ctx, "SELECT array_agg(place ORDER BY seat) FROM seats WHERE game_id = $1", gameID).Scan(&seats); err != nil || fmt.Sprint(seats) != "[1 1]" {
		t.Errorf("a draw's places = %v, %v; want both first", seats, err)
	}

	// A resigned player loses with culture 0 and the second place, whatever was typed.
	if err := a.RecordBlitzGame(ctx, "us_open", "2026", cyDee, BlitzGame{ShareCode: "EFGH5678", A: finished(180), B: BlitzSide{Culture: 190, Outcome: "resigned"}, Played: played}); err != nil {
		t.Fatalf("RecordBlitzGame(resigned): %v", err)
	}
	public, err := reader.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("the site's BlitzEdition: %v", err)
	}
	if got := resultLine(matchOf(t, public, cyDee)); got != "a game=EFGH5678 180/finished 0/resigned on 2026-10-14" {
		t.Errorf("the site's Cy v Dee = %q, want Cy's win with Dee at 0, resigned", got)
	}
}

func TestBlitzGamesThatCannotBeRecorded(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	eve, err := a.AddBlitzEntrants(ctx, "us_open", "2026", []BlitzEntrant{{Name: "Eve", Zone: "UTC"}})
	if err != nil {
		t.Fatalf("AddBlitzEntrants(Eve): %v", err)
	}
	r1, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Seed: 8, Matches: []BlitzMatch{{A: ids[0], B: &ids[1]}, {A: ids[2], B: &ids[3]}, {A: eve[0]}}})
	if err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	matches, bye := []int64{r1.Matches[0].ID, r1.Matches[1].ID}, r1.Matches[2]
	if err := a.RecordBlitzGame(ctx, "us_open", "2026", matches[0], BlitzGame{ShareCode: "ABCD1234", A: finished(250), B: finished(200), Played: played}); err != nil {
		t.Fatalf("RecordBlitzGame: %v", err)
	}
	game := BlitzGame{ShareCode: "EFGH5678", A: finished(180), B: finished(170), Played: played}
	tests := []struct {
		name  string
		match int64
		game  BlitzGame
		want  error
	}{
		{"an unknown match", 999, game, ErrNoBlitzMatch},
		{"a bye", bye.ID, game, ErrBlitzBye},
		{"a code already on another game", matches[1], BlitzGame{ShareCode: "ABCD1234", A: finished(180), B: finished(170), Played: played}, &BlitzShareCodeError{}},
		{"no code", matches[1], BlitzGame{A: finished(180), B: finished(170), Played: played}, ErrBadBlitzGame},
		{"an outcome that is not finished, resigned or timed out", matches[1], BlitzGame{ShareCode: "EFGH5678", A: finished(180), B: BlitzSide{Outcome: "lost"}, Played: played}, ErrBadBlitzGame},
		{"a negative culture", matches[1], BlitzGame{ShareCode: "EFGH5678", A: finished(-1), B: finished(170), Played: played}, ErrBadBlitzGame},
		{"neither player finished", matches[1], BlitzGame{ShareCode: "EFGH5678", A: BlitzSide{Outcome: "resigned"}, B: BlitzSide{Outcome: "timed_out"}, Played: played}, ErrBadBlitzGame},
		{"no day played", matches[1], BlitzGame{ShareCode: "EFGH5678", A: finished(180), B: finished(170)}, ErrBadBlitzGame},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := a.RecordBlitzGame(ctx, "us_open", "2026", tt.match, tt.game)
			var taken *BlitzShareCodeError
			if _, ok := tt.want.(*BlitzShareCodeError); ok {
				if !errors.As(err, &taken) || taken.GameID == 0 {
					t.Errorf("RecordBlitzGame = %v, want a BlitzShareCodeError naming the game", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("RecordBlitzGame = %v, want %v", err, tt.want)
			}
		})
	}
	var games int
	if err := a.pool.QueryRow(ctx, "SELECT count(*) FROM games").Scan(&games); err != nil || games != 1 {
		t.Errorf("games after the refusals = %d, %v; want the one recorded", games, err)
	}
}

func TestBlitzRulingsDecideAMatch(t *testing.T) {
	a, reader, _, matches := blitzRoundOne(t)
	ctx := context.Background()
	annBob, cyDee := matches[0], matches[1]
	if err := a.RecordBlitzRuling(ctx, "us_open", "2026", cyDee, "double_loss", "Neither player answered the scheduling messages"); err != nil {
		t.Fatalf("RecordBlitzRuling: %v", err)
	}
	public, err := reader.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("the site's BlitzEdition: %v", err)
	}
	if got := resultLine(matchOf(t, public, cyDee)); got != "double_loss ruling=Neither player answered the scheduling messages" {
		t.Errorf("the ruling read back = %q", got)
	}

	// A ruling may overrule a played game, which stays; recording the game again puts its result back.
	if err := a.RecordBlitzGame(ctx, "us_open", "2026", annBob, BlitzGame{ShareCode: "ABCD1234", A: finished(250), B: finished(200), Played: played}); err != nil {
		t.Fatalf("RecordBlitzGame: %v", err)
	}
	if err := a.RecordBlitzRuling(ctx, "us_open", "2026", annBob, "b", "Ann used a banned account"); err != nil {
		t.Fatalf("RecordBlitzRuling(overrule): %v", err)
	}
	ed, _ := a.BlitzEdition(ctx, "us_open", "2026")
	if got := resultLine(matchOf(t, ed, annBob)); got != "b game=ABCD1234 250/finished 200/finished on 2026-10-14 ruling=Ann used a banned account" {
		t.Errorf("the overruled game = %q, want Bob's win by ruling with the game kept", got)
	}
	if err := a.RecordBlitzGame(ctx, "us_open", "2026", annBob, BlitzGame{ShareCode: "ABCD1234", A: finished(250), B: finished(200), Played: played}); err != nil {
		t.Fatalf("RecordBlitzGame(again): %v", err)
	}
	if ed, _ = a.BlitzEdition(ctx, "us_open", "2026"); resultLine(matchOf(t, ed, annBob)) != "a game=ABCD1234 250/finished 200/finished on 2026-10-14" {
		t.Errorf("the game recorded again = %q, want Ann's win and no ruling", resultLine(matchOf(t, ed, annBob)))
	}

	for _, tt := range []struct {
		name, result, reason string
		match                int64
		want                 error
	}{
		{"no reason", "a", "  ", annBob, ErrBadBlitzRuling},
		{"a result that is not a, b, draw or double_loss", "c", "x", annBob, ErrBadBlitzRuling},
		{"an unknown match", "a", "x", 999, ErrNoBlitzMatch},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := a.RecordBlitzRuling(ctx, "us_open", "2026", tt.match, tt.result, tt.reason); !errors.Is(err, tt.want) {
				t.Errorf("RecordBlitzRuling = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestBlitzRoundsWaitForResultsAndKeepDecidedMatches(t *testing.T) {
	a, _, ids, matches := blitzRoundOne(t)
	ctx := context.Background()
	ann, bob, cy, dee := ids[0], ids[1], ids[2], ids[3]
	next := BlitzRound{Seed: 9, Matches: []BlitzMatch{{A: ann, B: &cy}, {A: bob, B: &dee}}}
	if _, err := a.AddBlitzRound(ctx, "us_open", "2026", next); !errors.Is(err, ErrBlitzRoundUndecided) || !containsAll(err.Error(), "Ann", "Bob", "Cy", "Dee") {
		t.Errorf("round 2 before any result = %v, want ErrBlitzRoundUndecided naming both matches", err)
	}
	if err := a.RecordBlitzGame(ctx, "us_open", "2026", matches[0], BlitzGame{ShareCode: "ABCD1234", A: finished(250), B: finished(200), Played: played}); err != nil {
		t.Fatalf("RecordBlitzGame: %v", err)
	}
	if _, err := a.AddBlitzRound(ctx, "us_open", "2026", next); !errors.Is(err, ErrBlitzRoundUndecided) || containsAll(err.Error(), "Ann") {
		t.Errorf("round 2 with Cy v Dee open = %v, want ErrBlitzRoundUndecided naming only Cy and Dee", err)
	}

	// A decided match stays when the round's matches are replaced; the undecided one may change.
	if err := a.SetBlitzRoundMatches(ctx, "us_open", "2026", roundID(t, a, 1), []BlitzMatch{{A: ann, B: &bob}, {A: cy}, {A: dee}}); err != nil {
		t.Errorf("replacing round 1 keeping Ann v Bob = %v, want nil", err)
	}
	if err := a.SetBlitzRoundMatches(ctx, "us_open", "2026", roundID(t, a, 1), []BlitzMatch{{A: ann, B: &cy}, {A: bob, B: &dee}}); !errors.Is(err, ErrBlitzMatchDecided) {
		t.Errorf("replacing round 1 without Ann v Bob = %v, want ErrBlitzMatchDecided", err)
	}
	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	if m := matchOf(t, ed, matches[0]); m.Result != "a" || m.Game == nil {
		t.Errorf("Ann v Bob after the replacements = %q, want the recorded win kept", resultLine(m))
	}
	if err := a.DeleteBlitzRound(ctx, "us_open", "2026", roundID(t, a, 1)); !errors.Is(err, ErrBlitzRoundPlayed) {
		t.Errorf("deleting a round with a result = %v, want ErrBlitzRoundPlayed", err)
	}

	// Byes count as decided: round 2 opens once Cy and Dee each sit out.
	if _, err := a.AddBlitzRound(ctx, "us_open", "2026", next); err != nil {
		t.Errorf("round 2 with only byes open = %v, want nil", err)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

func roundID(t *testing.T, a *AdminStore, number int) int64 {
	t.Helper()
	ed, err := a.BlitzEdition(context.Background(), "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	for _, r := range ed.Rounds {
		if r.Number == number {
			return r.ID
		}
	}
	t.Fatalf("no round %d", number)
	return 0
}
