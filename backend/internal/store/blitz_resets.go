// Blitz resets: a match's result, a round's results, or a whole edition's rounds undone, with the games they wrote,
// as if never played.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrBlitzNotLatest means a result is reset only in the edition's latest round.
var ErrBlitzNotLatest = errors.New("only the latest round's results can be reset: reset and delete the rounds after it first")

// latestRound returns ErrBlitzNotLatest unless round number is edition edID's latest.
func latestRound(ctx context.Context, tx pgx.Tx, edID int64, number int) error {
	var latest int
	if err := tx.QueryRow(ctx, "SELECT coalesce(max(number), 0) FROM blitz_rounds WHERE tournament_id = $1", edID).Scan(&latest); err != nil {
		return fmt.Errorf("finding the latest round: %w", err)
	}
	if number != latest {
		return ErrBlitzNotLatest
	}
	return nil
}

// dropBlitzGames deletes games an edition's matches no longer hold, through delete_blitz_game: their seats, the rows
// ratings runs keep for them, and the import batches, group members, groups and "Round N" stages they leave empty.
func dropBlitzGames(ctx context.Context, tx pgx.Tx, games []int64) error {
	for _, id := range games {
		if _, err := tx.Exec(ctx, "SELECT delete_blitz_game($1)", id); err != nil {
			return fmt.Errorf("deleting game %d: %w", id, err)
		}
	}
	return nil
}

// undecide clears the results of the matches matching where ($1 its value), and returns the games they held.
func undecide(ctx context.Context, tx pgx.Tx, where string, value int64) ([]int64, error) {
	rows, err := tx.Query(ctx, "SELECT m.game_id FROM blitz_matches m WHERE m.game_id IS NOT NULL AND "+where, value)
	if err != nil {
		return nil, fmt.Errorf("finding the games: %w", err)
	}
	games, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, fmt.Errorf("finding the games: %w", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE blitz_matches m SET result = NULL, game_id = NULL, ruling = '', decided_at = NULL, reported_by = NULL, report = NULL, flag_reason = '' WHERE "+where, value); err != nil {
		return nil, fmt.Errorf("clearing the results: %w", err)
	}
	return games, nil
}

// ResetBlitzResult undoes a match's result in the edition's latest round: no result, ruling or game, as if never
// played; resetting it again changes nothing. It returns ErrNoBlitzEdition, ErrNoBlitzMatch, ErrBlitzBye or
// ErrBlitzNotLatest.
func (a *AdminStore) ResetBlitzResult(ctx context.Context, slug, edition string, matchID int64) error {
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
		if err := latestRound(ctx, tx, edID, m.Round); err != nil {
			return err
		}
		games, err := undecide(ctx, tx, "m.match_id = $1", matchID)
		if err != nil {
			return err
		}
		return dropBlitzGames(ctx, tx, games)
	})
}

// ResetBlitzRound undoes every result in the edition's latest round, its pairing kept (so it can then be deleted).
// It returns ErrNoBlitzEdition, ErrNoBlitzRound or ErrBlitzNotLatest.
func (a *AdminStore) ResetBlitzRound(ctx context.Context, slug, edition string, roundID int64) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		edID, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		_, number, err := blitzRoundOf(ctx, tx, edID, roundID)
		if err != nil {
			return err
		}
		if err := latestRound(ctx, tx, edID, number); err != nil {
			return err
		}
		games, err := undecide(ctx, tx, "m.round_id = $1", roundID)
		if err != nil {
			return err
		}
		return dropBlitzGames(ctx, tx, games)
	})
}

// ResetBlitzEdition starts an edition again: every round, result and game goes, every dropped entrant is in again,
// every skip and starting rank goes. Entrants, organisers, format, points and thread text stay. It returns ErrNoBlitzEdition.
func (a *AdminStore) ResetBlitzEdition(ctx context.Context, slug, edition string) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		edID, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		games, err := undecide(ctx, tx, "m.round_id IN (SELECT round_id FROM blitz_rounds WHERE tournament_id = $1)", edID)
		if err != nil {
			return err
		}
		for _, q := range []string{
			"DELETE FROM blitz_rounds WHERE tournament_id = $1",
			"UPDATE blitz_entries SET status = 'active', starting_rank = NULL WHERE tournament_id = $1 AND (status = 'dropped' OR starting_rank IS NOT NULL)",
			"DELETE FROM blitz_skips s USING blitz_entries x WHERE s.entry_id = x.entry_id AND x.tournament_id = $1",
		} {
			if _, err := tx.Exec(ctx, q, edID); err != nil {
				return fmt.Errorf("resetting %s %s: %w", slug, edition, err)
			}
		}
		return dropBlitzGames(ctx, tx, games)
	})
}
