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
// result, ErrBlitzRoundFinished. A published round's games then get their threads queued, and those posted a notice
// of a deadline moved.
func (a *AdminStore) SetBlitzRoundDeadline(ctx context.Context, slug, edition string, roundID int64, at time.Time, zone string) error {
	return a.inBlitzEdition(ctx, slug, edition, func(tx pgx.Tx, id int64) error {
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
		var moved bool
		if err := tx.QueryRow(ctx, "SELECT published_at IS NOT NULL AND deadline IS DISTINCT FROM $2 FROM blitz_rounds WHERE round_id = $1",
			roundID, at).Scan(&moved); err != nil {
			return fmt.Errorf("setting round %d's deadline: %w", roundID, err)
		}
		if _, err := tx.Exec(ctx, "UPDATE blitz_rounds SET deadline = $2, deadline_zone = $3 WHERE round_id = $1", roundID, at, zone); err != nil {
			return fmt.Errorf("setting round %d's deadline: %w", roundID, err)
		}
		if moved {
			if err := queueDeadlineMoved(ctx, tx, roundID); err != nil {
				return err
			}
		}
		return queueRoundThreads(ctx, tx, roundID)
	})
}
