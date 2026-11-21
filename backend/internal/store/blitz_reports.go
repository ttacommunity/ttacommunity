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

// ErrBlitzFinishedAhead is a report of a game finishing after now.
var ErrBlitzFinishedAhead = errors.New("a game cannot finish after now")

// ErrBlitzBestOfNotOffered means best of 3 in a round that does not offer it.
var ErrBlitzBestOfNotOffered = errors.New("this round's matches are one game")

// ErrBlitzBestOfClosed means a match's games can no longer be agreed: they are, game 1 is in, or an organiser ruled.
var ErrBlitzBestOfClosed = errors.New("the match's games are settled: best of 3 is agreed before game 1")

// ErrBadBlitzBestOf means an answer about best of 3 that does not fit: one that is none, or accepting, declining or
// withdrawing a proposal that is not there to answer.
var ErrBadBlitzBestOf = errors.New("propose best of 3, or accept or decline the opponent's proposal, or withdraw your own")

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
	case "ahead":
		return ErrBlitzFinishedAhead
	case "no-game":
		return ErrNoBlitzGame
	case "not-needed":
		return ErrBlitzGameNotNeeded
	case "not-offered":
		return ErrBlitzBestOfNotOffered
	case "best-of-closed":
		return ErrBlitzBestOfClosed
	case "bad-best-of":
		return ErrBadBlitzBestOf
	}
	return fmt.Errorf("unexpected answer %q", got)
}

// ReportBlitzGame records game number of the match matchID, in the shown edition, as the player whose session hashes
// to tokenHash, at now; reporting again corrects it while players may. It returns ErrBlitzShareCodeNeeded,
// ErrBadBlitzGame, ErrBlitzFinishedAhead, ErrNoSession, ErrNotLinked, ErrMemberBanned, ErrNoBlitzEdition, ErrNoBlitzMatch, ErrNotYourMatch,
// ErrBlitzBye, ErrBlitzRoundClosed, ErrNoBlitzGame, ErrBlitzGameNotNeeded, ErrBlitzResultLocked, or a
// *BlitzShareCodeError. A report is queued to be notified.
func (s *PlayerStore) ReportBlitzGame(ctx context.Context, tokenHash []byte, slug, edition string, matchID int64, number int, g BlitzGame, now time.Time) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := reportBlitzGame(ctx, tx, tokenHash, slug, edition, matchID, number, g, now); err != nil {
			return err
		}
		return queueBlitzNotification(ctx, tx, "blitz_report", matchID, number, now)
	})
}

// reportBlitzGame records a player's report through report_blitz_game on q, returning ReportBlitzGame's errors.
func reportBlitzGame(ctx context.Context, q conn, tokenHash []byte, slug, edition string, matchID int64, number int, g BlitzGame, now time.Time) error {
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
	if err := q.QueryRow(ctx, "SELECT report_blitz_game($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)",
		tokenHash, now, slug, edition, matchID, number, g.ShareCode, g.Finished, result, sides).Scan(&got); err != nil {
		return fmt.Errorf("reporting a game: %w", err)
	}
	return reportError(got, "reported")
}

// ConfirmBlitzResult confirms the opponent's report of game number of the match matchID, as the player whose session
// hashes to tokenHash, at now: players can no longer change it. It returns ReportBlitzGame's errors for the session and
// match, or ErrNoBlitzGame.
func (s *PlayerStore) ConfirmBlitzResult(ctx context.Context, tokenHash []byte, slug, edition string, matchID int64, number int, now time.Time) error {
	return s.answerBlitzReport(ctx, tokenHash, slug, edition, matchID, number, "confirmed", "", now)
}

// FlagBlitzResult flags the opponent's report of game number of the match matchID as wrong, saying why, as the player
// whose session hashes to tokenHash, at now: players can no longer change it, and an organiser resolves it. It returns
// ErrBadBlitzFlag, ErrNoBlitzGame, or ReportBlitzGame's errors for the session and match.
func (s *PlayerStore) FlagBlitzResult(ctx context.Context, tokenHash []byte, slug, edition string, matchID int64, number int, reason string, now time.Time) error {
	if strings.TrimSpace(reason) == "" {
		return ErrBadBlitzFlag
	}
	return s.answerBlitzReport(ctx, tokenHash, slug, edition, matchID, number, "flagged", reason, now)
}

// answerBlitzReport confirms or flags a game's report through answer_blitz_report; a flag is notified.
func (s *PlayerStore) answerBlitzReport(ctx context.Context, tokenHash []byte, slug, edition string, matchID int64, number int, answer, reason string, now time.Time) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var got string
		if err := tx.QueryRow(ctx, "SELECT answer_blitz_report($1, $2, $3, $4, $5, $6, $7, nullif($8, ''))",
			tokenHash, now, slug, edition, matchID, number, answer, reason).Scan(&got); err != nil {
			return fmt.Errorf("answering a report: %w", err)
		}
		if err := reportError(got, answer); err != nil || answer != "flagged" {
			return err
		}
		return queueBlitzNotification(ctx, tx, "blitz_flag", matchID, number, now)
	})
}

// AnswerBlitzBestOf answers for the match matchID whether to play best of 3, as the player whose session hashes to
// tokenHash, at now: propose it, accept or decline the opponent's proposal, or withdraw their own. A proposal answered
// by one of the opponent's is agreed. It returns ErrBadBlitzBestOf, ErrBlitzBestOfNotOffered, ErrBlitzBestOfClosed, or
// ReportBlitzGame's errors for the session and match.
func (s *PlayerStore) AnswerBlitzBestOf(ctx context.Context, tokenHash []byte, slug, edition string, matchID int64, answer string, now time.Time) error {
	var got string
	if err := s.pool.QueryRow(ctx, "SELECT answer_blitz_best_of($1, $2, $3, $4, $5, $6)", tokenHash, now, slug, edition, matchID, answer).Scan(&got); err != nil {
		return fmt.Errorf("answering best of 3: %w", err)
	}
	return reportError(got, "answered")
}

// KeepBlitzReport resolves the flagged report of game number of a match by keeping its result; players still cannot
// change it. It returns ErrNoBlitzEdition, ErrNoBlitzMatch or ErrBlitzNotFlagged.
func (a *AdminStore) KeepBlitzReport(ctx context.Context, slug, edition string, matchID int64, number int) error {
	return a.inBlitzEdition(ctx, slug, edition, func(tx pgx.Tx, edID int64) error {
		if _, err := blitzMatchOf(ctx, tx, edID, matchID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "UPDATE blitz_match_games SET report = 'resolved' WHERE match_id = $1 AND number = $2 AND report = 'flagged'", matchID, number)
		if err != nil {
			return fmt.Errorf("keeping a report: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrBlitzNotFlagged
		}
		return nil
	})
}
