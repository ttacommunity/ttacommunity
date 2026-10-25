// Blitz deadlines: each round's deadline to play by, and the zone it was set in.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrBlitzRoundFinished means every game of the round has a result: its deadline no longer changes.
var ErrBlitzRoundFinished = errors.New("the round is finished: its deadline no longer changes")

// SetBlitzRoundDeadline sets the deadline to play a round by, draft or published, and the zone it was given in
// (checked by the caller), or returns ErrNoBlitzEdition, ErrNoBlitzRound or, once every game of the round has a
// result, ErrBlitzRoundFinished.
func (a *AdminStore) SetBlitzRoundDeadline(ctx context.Context, slug, edition string, roundID int64, at time.Time, zone string) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		id, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		if _, _, err := blitzRoundOf(ctx, tx, id, roundID); err != nil {
			return err
		}
		var games, decided int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FILTER (WHERE entry_b IS NOT NULL), count(*) FILTER (WHERE entry_b IS NOT NULL AND result IS NOT NULL)
			FROM blitz_matches WHERE round_id = $1`, roundID).Scan(&games, &decided); err != nil {
			return fmt.Errorf("setting round %d's deadline: %w", roundID, err)
		}
		if games > 0 && decided == games {
			return ErrBlitzRoundFinished
		}
		if _, err := tx.Exec(ctx, "UPDATE blitz_rounds SET deadline = $2, deadline_zone = $3 WHERE round_id = $1", roundID, at, zone); err != nil {
			return fmt.Errorf("setting round %d's deadline: %w", roundID, err)
		}
		return nil
	})
}
