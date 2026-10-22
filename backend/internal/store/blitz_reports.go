// Players' blitz reports: a linked player reports their own match's game, which counts at once; the reporter may
// correct it until the opponent confirms or flags it. The player login writes them only through the database's
// report_blitz_game and answer_blitz_report, which check the session and the match.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrNotLinked means the account is not linked to a member yet.
var ErrNotLinked = errors.New("the account is not linked to a TTA username yet")

// ErrNotYourMatch means the signed-in player does not play in the match.
var ErrNotYourMatch = errors.New("only the match's players report it")

// ErrBlitzRoundClosed means the match's round is over: the next round is paired, and only an admin changes it.
var ErrBlitzRoundClosed = errors.New("the next round is paired: ask an organiser to change this result")

// ErrBlitzResultLocked means players can no longer change the match's result: an organiser decided it, the
// opponent reported it, or it was confirmed or flagged.
var ErrBlitzResultLocked = errors.New("this result can no longer be changed here: ask an organiser")

// ErrBlitzShareCodeNeeded means a player's report has no share code.
var ErrBlitzShareCodeNeeded = errors.New("a reported game needs its share code")

// ErrBadBlitzFlag means a flag without a reason, or with one over 500 characters.
var ErrBadBlitzFlag = errors.New("a flag says what is wrong, in at most 500 characters")

// ErrBlitzNotFlagged means an organiser kept a report that is not flagged.
var ErrBlitzNotFlagged = errors.New("only a flagged report is kept")

// reportError turns what report_blitz_game and answer_blitz_report answer into an error, nil for want.
func reportError(got, want string) error {
	if code, ok := strings.CutPrefix(got, "taken:"); ok {
		id, err := strconv.ParseInt(code, 10, 64)
		if err != nil {
			return fmt.Errorf("reading the game holding a share code: %w", err)
		}
		return &BlitzShareCodeError{GameID: id}
	}
	switch got {
	case want:
		return nil
	case "signed-out":
		return ErrNoSession
	case "not-linked":
		return ErrNotLinked
	case "banned":
		return ErrMemberBanned
	case "no-edition":
		return ErrNoBlitzEdition
	case "no-match":
		return ErrNoBlitzMatch
	case "not-yours":
		return ErrNotYourMatch
	case "bye":
		return ErrBlitzBye
	case "closed":
		return ErrBlitzRoundClosed
	case "locked":
		return ErrBlitzResultLocked
	case "bad":
		return ErrBadBlitzFlag
	}
	return fmt.Errorf("unexpected answer %q", got)
}

// ReportBlitzGame records the game of the match matchID, in the shown edition, as the player whose session hashes to
// tokenHash, at now; reporting again corrects it while players may. It returns ErrBlitzShareCodeNeeded,
// ErrBadBlitzGame, ErrNoSession, ErrNotLinked, ErrMemberBanned, ErrNoBlitzEdition, ErrNoBlitzMatch, ErrNotYourMatch,
// ErrBlitzBye, ErrBlitzRoundClosed, ErrBlitzResultLocked, or a *BlitzShareCodeError.
func (s *PlayerStore) ReportBlitzGame(ctx context.Context, tokenHash []byte, slug, edition string, matchID int64, g BlitzGame, now time.Time) error {
	if strings.TrimSpace(g.ShareCode) == "" {
		return ErrBlitzShareCodeNeeded
	}
	if !validBlitzGame(&g) {
		return ErrBadBlitzGame
	}
	result, a, b, placeA, placeB := blitzOutcome(g)
	sides, err := json.Marshal([]map[string]any{
		{"outcome": a.Outcome, "culture": a.Culture, "place": placeA, "position": a.Turn - 1},
		{"outcome": b.Outcome, "culture": b.Culture, "place": placeB, "position": b.Turn - 1},
	})
	if err != nil {
		return fmt.Errorf("reporting a game: %w", err)
	}
	var got string
	if err := s.pool.QueryRow(ctx, "SELECT report_blitz_game($1, $2, $3, $4, $5, $6, $7, $8, $9)",
		tokenHash, now, slug, edition, matchID, g.ShareCode, g.Finished, result, sides).Scan(&got); err != nil {
		return fmt.Errorf("reporting a game: %w", err)
	}
	return reportError(got, "reported")
}

// ConfirmBlitzResult confirms the opponent's report of the match matchID, as the player whose session hashes to
// tokenHash, at now: players can no longer change it. It returns ReportBlitzGame's errors for the session and match.
func (s *PlayerStore) ConfirmBlitzResult(ctx context.Context, tokenHash []byte, slug, edition string, matchID int64, now time.Time) error {
	return s.answerBlitzReport(ctx, tokenHash, slug, edition, matchID, "confirmed", "", now)
}

// FlagBlitzResult flags the opponent's report of the match matchID as wrong, saying why, as the player whose session
// hashes to tokenHash, at now: players can no longer change it, and an organiser resolves it. It returns
// ErrBadBlitzFlag, or ReportBlitzGame's errors for the session and match.
func (s *PlayerStore) FlagBlitzResult(ctx context.Context, tokenHash []byte, slug, edition string, matchID int64, reason string, now time.Time) error {
	if strings.TrimSpace(reason) == "" {
		return ErrBadBlitzFlag
	}
	return s.answerBlitzReport(ctx, tokenHash, slug, edition, matchID, "flagged", reason, now)
}

// answerBlitzReport confirms or flags a report through answer_blitz_report.
func (s *PlayerStore) answerBlitzReport(ctx context.Context, tokenHash []byte, slug, edition string, matchID int64, answer, reason string, now time.Time) error {
	var got string
	if err := s.pool.QueryRow(ctx, "SELECT answer_blitz_report($1, $2, $3, $4, $5, $6, nullif($7, ''))",
		tokenHash, now, slug, edition, matchID, answer, reason).Scan(&got); err != nil {
		return fmt.Errorf("answering a report: %w", err)
	}
	return reportError(got, answer)
}

// KeepBlitzReport resolves a flagged report by keeping its result; players still cannot change it. It returns
// ErrNoBlitzEdition, ErrNoBlitzMatch or ErrBlitzNotFlagged.
func (a *AdminStore) KeepBlitzReport(ctx context.Context, slug, edition string, matchID int64) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		edID, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		if _, err := blitzMatchOf(ctx, tx, edID, matchID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "UPDATE blitz_matches SET report = 'resolved' WHERE match_id = $1 AND report = 'flagged'", matchID)
		if err != nil {
			return fmt.Errorf("keeping a report: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrBlitzNotFlagged
		}
		return nil
	})
}
