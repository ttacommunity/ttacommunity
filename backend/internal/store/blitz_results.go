// Blitz results: a match decided by a played game, written as a game with two seats under the edition's
// "Round N" stage so the tournament's page shows it at once, or by the organisers' ruling, which may overrule
// a game. The admin login writes games and never deletes one (CheckLimits).
package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// BlitzSide is one player's side of a played game: the culture, and how it ended (finished, resigned or
// timed_out).
type BlitzSide struct {
	Culture float64
	Outcome string
}

// BlitzGame is a played game as recorded: the share code, each side as the match names them, and when it finished.
type BlitzGame struct {
	ShareCode string
	A, B      BlitzSide
	Finished  time.Time
}

// ErrNoBlitzMatch means no match of the edition has that id.
var ErrNoBlitzMatch = errors.New("no such match")

// ErrBlitzBye means a result was recorded on a bye.
var ErrBlitzBye = errors.New("a bye has no result")

// ErrBadBlitzGame means a game outside what can be recorded.
var ErrBadBlitzGame = errors.New("a game has a share code of at most 32 characters, each player's culture (0 or more) and " +
	"outcome (finished, resigned or timed_out), at least one of them finished, and the day it was played")

// ErrBadBlitzRuling means a ruling outside what can be recorded.
var ErrBadBlitzRuling = errors.New("a ruling is a, b, draw or double_loss, with a reason of at most 1000 characters")

// ErrBlitzRoundUndecided means a match of an earlier round has no result yet, so the next round cannot be made.
var ErrBlitzRoundUndecided = errors.New("waiting for results")

// ErrBlitzKnockoutWinner means a ruling on a knockout match that names no winner: someone has to go through.
var ErrBlitzKnockoutWinner = errors.New("a knockout match needs a winner: rule for one player")

// ErrBlitzMatchDecided means a round's matches were replaced without a match that has a result: it stays.
var ErrBlitzMatchDecided = errors.New("a match with a result stays")

// ErrBlitzRoundPlayed means a round with a result cannot be deleted.
var ErrBlitzRoundPlayed = errors.New("a round with a result cannot be deleted")

// BlitzShareCodeError means the share code is already on another game: that one.
type BlitzShareCodeError struct{ GameID int64 }

func (e *BlitzShareCodeError) Error() string {
	return fmt.Sprintf("the share code is already on game %d", e.GameID)
}

var blitzResults = []string{"a", "b", "draw", "double_loss"}

var blitzOutcomes = []string{"finished", "resigned", "timed_out"}

// validBlitzGame trims the share code and reports whether the game is within bounds.
func validBlitzGame(g *BlitzGame) bool {
	g.ShareCode = strings.TrimSpace(g.ShareCode)
	if g.ShareCode == "" || utf8.RuneCountInString(g.ShareCode) > 32 || g.Finished.IsZero() {
		return false
	}
	for _, s := range []BlitzSide{g.A, g.B} {
		if s.Culture < 0 || !slices.Contains(blitzOutcomes, s.Outcome) {
			return false
		}
	}
	return g.A.Outcome == "finished" || g.B.Outcome == "finished"
}

// blitzOutcome decides a game: the higher culture wins and equal cultures draw; a resigned or timed-out player
// loses with culture 0. It returns the result and each side as recorded, with its place.
func blitzOutcome(g BlitzGame) (result string, a, b BlitzSide, placeA, placeB int) {
	a, b = g.A, g.B
	if a.Outcome != "finished" {
		a.Culture = 0
	}
	if b.Outcome != "finished" {
		b.Culture = 0
	}
	switch {
	case a.Outcome != "finished", b.Outcome == "finished" && b.Culture > a.Culture:
		return "b", a, b, 2, 1
	case b.Outcome != "finished", a.Culture > b.Culture:
		return "a", a, b, 1, 2
	}
	return "draw", a, b, 1, 1
}

// blitzMatchRow is a match as recording a result needs it: its round and the round's kind, each side's entry,
// name and knockout seed, its game.
type blitzMatchRow struct {
	Round      int
	Kind       string
	SeedA      *int
	SeedB      *int
	A          int64
	B          *int64
	NameA      string
	NameB      string
	GameID     *int64
	SeriesName string
}

// blitzMatchOf finds a match of the edition, or returns ErrNoBlitzMatch.
func blitzMatchOf(ctx context.Context, tx pgx.Tx, edID, matchID int64) (blitzMatchRow, error) {
	var m blitzMatchRow
	err := tx.QueryRow(ctx, `
		SELECT r.number, r.kind, m.seed_a, m.seed_b, m.entry_a, m.entry_b, ma.tta_name, coalesce(mb.tta_name, ''), m.game_id, s.name
		FROM blitz_matches m JOIN blitz_rounds r USING (round_id)
		JOIN tournaments t USING (tournament_id) JOIN series s USING (series_id)
		JOIN blitz_entries a ON a.entry_id = m.entry_a JOIN members ma ON ma.member_id = a.member_id
		LEFT JOIN blitz_entries b ON b.entry_id = m.entry_b LEFT JOIN members mb ON mb.member_id = b.member_id
		WHERE m.match_id = $1 AND r.tournament_id = $2`, matchID, edID).Scan(&m.Round, &m.Kind, &m.SeedA, &m.SeedB, &m.A, &m.B, &m.NameA, &m.NameB, &m.GameID, &m.SeriesName)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNoBlitzMatch
	}
	if err != nil {
		return m, fmt.Errorf("finding match %d: %w", matchID, err)
	}
	return m, nil
}

// blitzResultsIn returns ErrBlitzRoundUndecided, naming the players, when a match of the edition has no result,
// or a knockout match no winner.
func blitzResultsIn(ctx context.Context, tx pgx.Tx, edID int64) error {
	rows, err := tx.Query(ctx, `
		SELECT ma.tta_name || ' v ' || mb.tta_name || CASE WHEN m.result IS NULL THEN '' ELSE ' (no winner yet)' END
		FROM blitz_matches m JOIN blitz_rounds r USING (round_id)
		JOIN blitz_entries a ON a.entry_id = m.entry_a JOIN members ma ON ma.member_id = a.member_id
		JOIN blitz_entries b ON b.entry_id = m.entry_b JOIN members mb ON mb.member_id = b.member_id
		WHERE r.tournament_id = $1 AND (m.result IS NULL OR (r.kind = 'knockout' AND m.result NOT IN ('a', 'b')))
		ORDER BY r.number, m.match_id`, edID)
	if err != nil {
		return fmt.Errorf("checking for results: %w", err)
	}
	open, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("checking for results: %w", err)
	}
	if len(open) > 0 {
		return fmt.Errorf("%w: %s", ErrBlitzRoundUndecided, strings.Join(open, ", "))
	}
	return nil
}

// blitzPlayer finds the ratings' player of a name, without case, adding one for a name they do not know.
func blitzPlayer(ctx context.Context, tx pgx.Tx, name string) (id int64, spelled string, err error) {
	if _, err := tx.Exec(ctx, "INSERT INTO players (name) SELECT $1 WHERE NOT EXISTS (SELECT 1 FROM players WHERE lower(name) = lower($1))", name); err != nil {
		return 0, "", fmt.Errorf("adding a player: %w", err)
	}
	if err := tx.QueryRow(ctx, "SELECT player_id, name FROM players WHERE lower(name) = lower($1) ORDER BY player_id LIMIT 1", name).Scan(&id, &spelled); err != nil {
		return 0, "", fmt.Errorf("finding a player: %w", err)
	}
	return id, spelled, nil
}

// blitzRoundGroup finds the group the round's games sit in, under the edition's stage "Round N", making both
// when the round has none yet.
func blitzRoundGroup(ctx context.Context, tx pgx.Tx, edID int64, round int) (int64, error) {
	label := fmt.Sprintf("Round %d", round)
	var stageID, groupID int64
	if _, err := tx.Exec(ctx, "INSERT INTO stages (tournament_id, label) VALUES ($1, $2) ON CONFLICT (tournament_id, label) DO NOTHING", edID, label); err != nil {
		return 0, fmt.Errorf("adding stage %s: %w", label, err)
	}
	if err := tx.QueryRow(ctx, "SELECT stage_id FROM stages WHERE tournament_id = $1 AND label = $2", edID, label).Scan(&stageID); err != nil {
		return 0, fmt.Errorf("finding stage %s: %w", label, err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO groups (stage_id, name) VALUES ($1, '') ON CONFLICT DO NOTHING", stageID); err != nil {
		return 0, fmt.Errorf("adding %s's group: %w", label, err)
	}
	if err := tx.QueryRow(ctx, "SELECT group_id FROM groups WHERE stage_id = $1 AND name = '' AND cge_tournament IS NULL", stageID).Scan(&groupID); err != nil {
		return 0, fmt.Errorf("finding %s's group: %w", label, err)
	}
	return groupID, nil
}

// RecordBlitzGame records a played game on a match, or corrects the game it already has (the one game: no
// duplicate), and sets the match's result from it, clearing any ruling. A drawn knockout game stays a draw as a
// game, and the match goes to the higher seed. It returns ErrNoBlitzEdition,
// ErrNoBlitzMatch, ErrBlitzBye, ErrBadBlitzGame, or a *BlitzShareCodeError for a code on another game.
func (a *AdminStore) RecordBlitzGame(ctx context.Context, slug, edition string, matchID int64, g BlitzGame) error {
	if !validBlitzGame(&g) {
		return ErrBadBlitzGame
	}
	result, sideA, sideB, placeA, placeB := blitzOutcome(g)
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		edID, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		m, err := blitzMatchOf(ctx, tx, edID, matchID)
		if err != nil {
			return err
		}
		if m.B == nil {
			return ErrBlitzBye
		}
		if m.Kind == "knockout" && result == "draw" {
			result = "b"
			if *m.SeedA < *m.SeedB {
				result = "a"
			}
		}
		var other int64
		switch err := tx.QueryRow(ctx, "SELECT game_id FROM games WHERE share_code = $1 AND game_id IS DISTINCT FROM $2", g.ShareCode, m.GameID).Scan(&other); {
		case err == nil:
			return &BlitzShareCodeError{GameID: other}
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("checking the share code: %w", err)
		}
		playerA, nameA, err := blitzPlayer(ctx, tx, m.NameA)
		if err != nil {
			return err
		}
		playerB, nameB, err := blitzPlayer(ctx, tx, m.NameB)
		if err != nil {
			return err
		}
		groupID, err := blitzRoundGroup(ctx, tx, edID, m.Round)
		if err != nil {
			return err
		}
		name := fmt.Sprintf("%s %s Round %d", m.SeriesName, edition, m.Round)
		// A game's day is its finish's day in UTC, as for the games CGE's data brings.
		date := g.Finished.UTC().Truncate(24 * time.Hour)
		gameID := m.GameID
		if gameID == nil {
			var batch int64
			if err := tx.QueryRow(ctx, "INSERT INTO import_batches (source) VALUES ('admin') RETURNING batch_id").Scan(&batch); err != nil {
				return fmt.Errorf("recording a game: %w", err)
			}
			gameID = new(int64)
			if err := tx.QueryRow(ctx, `
				INSERT INTO games (batch_id, source_file, source_line, name, date, finished_at, share_code, state, group_id)
				VALUES ($1, $2, 0, $3, $4, $5, $6, 'played', $7) RETURNING game_id`,
				batch, fmt.Sprintf("blitz match %d", matchID), name, date, g.Finished, g.ShareCode, groupID).Scan(gameID); err != nil {
				return fmt.Errorf("recording a game: %w", err)
			}
			for seat, s := range []struct {
				player int64
				name   string
				side   BlitzSide
				place  int
			}{{playerA, nameA, sideA, placeA}, {playerB, nameB, sideB, placeB}} {
				if _, err := tx.Exec(ctx, `
					INSERT INTO seats (game_id, seat, player_id, player_name, outcome, culture, place) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
					*gameID, seat+1, s.player, s.name, s.side.Outcome, s.side.Culture, s.place); err != nil {
					return fmt.Errorf("recording a game's seats: %w", err)
				}
			}
		} else {
			if _, err := tx.Exec(ctx, "UPDATE games SET share_code = $2, date = $3, finished_at = $4, name = $5, group_id = $6 WHERE game_id = $1",
				*gameID, g.ShareCode, date, g.Finished, name, groupID); err != nil {
				return fmt.Errorf("correcting a game: %w", err)
			}
			for seat, s := range []struct {
				player int64
				name   string
				side   BlitzSide
				place  int
			}{{playerA, nameA, sideA, placeA}, {playerB, nameB, sideB, placeB}} {
				if _, err := tx.Exec(ctx, `
					UPDATE seats SET player_id = $3, player_name = $4, outcome = $5, culture = $6, place = $7 WHERE game_id = $1 AND seat = $2`,
					*gameID, seat+1, s.player, s.name, s.side.Outcome, s.side.Culture, s.place); err != nil {
					return fmt.Errorf("correcting a game's seats: %w", err)
				}
			}
		}
		if _, err := tx.Exec(ctx, "INSERT INTO group_members (group_id, player_id) VALUES ($1, $2), ($1, $3) ON CONFLICT DO NOTHING", groupID, playerA, playerB); err != nil {
			return fmt.Errorf("recording a game's players: %w", err)
		}
		if _, err := tx.Exec(ctx, "UPDATE blitz_matches SET result = $2, game_id = $3, ruling = '', decided_at = now() WHERE match_id = $1", matchID, result, *gameID); err != nil {
			return fmt.Errorf("recording a result: %w", err)
		}
		return nil
	})
}

// RecordBlitzRuling decides a match by the organisers' ruling (a, b, draw or double_loss; a or b in the knockout)
// with its reason; a game the match has stays. It returns ErrNoBlitzEdition, ErrNoBlitzMatch, ErrBlitzBye,
// ErrBadBlitzRuling or ErrBlitzKnockoutWinner.
func (a *AdminStore) RecordBlitzRuling(ctx context.Context, slug, edition string, matchID int64, result, reason string) error {
	reason = strings.TrimSpace(reason)
	if !slices.Contains(blitzResults, result) || reason == "" || utf8.RuneCountInString(reason) > 1000 {
		return ErrBadBlitzRuling
	}
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		edID, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		m, err := blitzMatchOf(ctx, tx, edID, matchID)
		if err != nil {
			return err
		}
		if m.B == nil {
			return ErrBlitzBye
		}
		if m.Kind == "knockout" && result != "a" && result != "b" {
			return ErrBlitzKnockoutWinner
		}
		if _, err := tx.Exec(ctx, "UPDATE blitz_matches SET result = $2, ruling = $3, decided_at = now() WHERE match_id = $1", matchID, result, reason); err != nil {
			return fmt.Errorf("recording a ruling: %w", err)
		}
		return nil
	})
}
