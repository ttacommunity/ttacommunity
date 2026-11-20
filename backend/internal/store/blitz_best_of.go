// Best of 3 in a blitz knockout: a round offers it, and a match is three games once its players agree, or when an
// organiser sets it. The players' own answers go through answer_blitz_best_of (blitz_reports.go).
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrBlitzBestOfAgreed means a round set back to one game while a match of it is three games or has a proposal.
var ErrBlitzBestOfAgreed = errors.New("a match of the round is agreed or proposed as best of 3: set it back to one game first")

// ErrBlitzBestOfPlayed means a match set back to one game when it has a second game.
var ErrBlitzBestOfPlayed = errors.New("the match has played a second game: reset its result first")

// validBestOf returns ErrBadBlitzBestOf unless games is 1 or 3.
func validBestOf(games int) error {
	if games != 1 && games != 3 {
		return ErrBadBlitzBestOf
	}
	return nil
}

// SetBlitzRoundBestOf sets whether a knockout round's matches are one game (1) or may be best of 3 (3). It returns
// ErrBadBlitzBestOf, ErrNoBlitzEdition, ErrNoBlitzRound, ErrBlitzBestOfNotOffered for a Swiss round, or, going back to one game,
// ErrBlitzBestOfAgreed.
func (a *AdminStore) SetBlitzRoundBestOf(ctx context.Context, slug, edition string, roundID int64, bestOf int) error {
	if err := validBestOf(bestOf); err != nil {
		return err
	}
	return a.inBlitzEdition(ctx, slug, edition, func(tx pgx.Tx, edID int64) error {
		kind, _, err := blitzRoundOf(ctx, tx, edID, roundID)
		if err != nil {
			return err
		}
		if kind != "knockout" && bestOf != 1 {
			return ErrBlitzBestOfNotOffered
		}
		var agreed bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM blitz_matches WHERE round_id = $1 AND (games > 1 OR proposed_by IS NOT NULL))",
			roundID).Scan(&agreed); err != nil {
			return fmt.Errorf("setting round %d's games: %w", roundID, err)
		}
		if agreed && bestOf == 1 {
			return ErrBlitzBestOfAgreed
		}
		if _, err := tx.Exec(ctx, "UPDATE blitz_rounds SET best_of = $2 WHERE round_id = $1", roundID, bestOf); err != nil {
			return fmt.Errorf("setting round %d's games: %w", roundID, err)
		}
		return nil
	})
}

// SetBlitzMatchBestOf sets a match of a round offering best of 3 to one game or three, as an organiser decides,
// dropping any proposal, and works its result out again from its games. It returns ErrBadBlitzBestOf,
// ErrNoBlitzEdition, ErrNoBlitzMatch, ErrBlitzBye, ErrBlitzBestOfNotOffered, or, going back to one game,
// ErrBlitzBestOfPlayed.
func (a *AdminStore) SetBlitzMatchBestOf(ctx context.Context, slug, edition string, matchID int64, games int) error {
	if err := validBestOf(games); err != nil {
		return err
	}
	return a.inBlitzEdition(ctx, slug, edition, func(tx pgx.Tx, edID int64) error {
		if _, err := pairedMatchOf(ctx, tx, edID, matchID); err != nil {
			return err
		}
		var offered, played bool
		if err := tx.QueryRow(ctx, `
			SELECT r.best_of = 3, EXISTS (SELECT 1 FROM blitz_match_games g WHERE g.match_id = m.match_id AND g.number > 1)
			FROM blitz_matches m JOIN blitz_rounds r USING (round_id) WHERE m.match_id = $1`, matchID).Scan(&offered, &played); err != nil {
			return fmt.Errorf("setting match %d's games: %w", matchID, err)
		}
		switch {
		case !offered:
			return ErrBlitzBestOfNotOffered
		case played && games == 1:
			return ErrBlitzBestOfPlayed
		}
		if _, err := tx.Exec(ctx, "UPDATE blitz_matches SET games = $2, proposed_by = NULL WHERE match_id = $1", matchID, games); err != nil {
			return fmt.Errorf("setting match %d's games: %w", matchID, err)
		}
		if _, err := tx.Exec(ctx, "SELECT blitz_match_result($1, now())", matchID); err != nil {
			return fmt.Errorf("setting match %d's games: %w", matchID, err)
		}
		return nil
	})
}
