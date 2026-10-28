// Masking players with a deleted or banned CGE account: their names hidden, or shown as DeletedPlayer.
package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// DeletedPlayer is what a player whose CGE accounts are deleted or banned is masked as.
const DeletedPlayer = "Deleted player"

// notHiddenPlayer excludes players who have any deleted or banned CGE account.
const notHiddenPlayer = `NOT EXISTS (
	SELECT 1 FROM player_accounts pa
	WHERE pa.player_id = p.player_id AND (pa.deleted OR pa.banned)
)`

// hiddenName is true when the name in column is, ignoring case, a player with any deleted or banned CGE account.
func hiddenName(column string) string {
	return `EXISTS (
	SELECT 1 FROM players hp JOIN player_accounts pa USING (player_id)
	WHERE lower(hp.name) = lower(` + column + `) AND (pa.deleted OR pa.banned)
)`
}

// HiddenNames returns the subset of names whose players, matched ignoring case, have a deleted or banned
// CGE account: the names a public answer shows as DeletedPlayer or leaves out.
func (r *RatingsReader) HiddenNames(ctx context.Context, names []string) (map[string]bool, error) {
	if len(names) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT n FROM unnest($1::text[]) n WHERE `+hiddenName("n"), names)
	if err != nil {
		return nil, fmt.Errorf("checking hidden players: %w", err)
	}
	hidden, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("checking hidden players: %w", err)
	}
	if len(hidden) == 0 {
		return nil, nil
	}
	set := make(map[string]bool, len(hidden))
	for _, n := range hidden {
		set[n] = true
	}
	return set, nil
}

// maskNames replaces each of names whose player has a deleted or banned CGE account with DeletedPlayer.
func (r *RatingsReader) maskNames(ctx context.Context, names []*string) error {
	var list []string
	seen := map[string]bool{}
	for _, n := range names {
		if !seen[*n] {
			seen[*n] = true
			list = append(list, *n)
		}
	}
	hidden, err := r.HiddenNames(ctx, list)
	if err != nil {
		return err
	}
	for _, n := range names {
		if hidden[*n] {
			*n = DeletedPlayer
		}
	}
	return nil
}

// maskMatchSeats replaces the names of seats whose players are deleted or banned with DeletedPlayer.
func (r *RatingsReader) maskMatchSeats(ctx context.Context, matches []PlayerMatch) error {
	var names []*string
	for i := range matches {
		for j := range matches[i].Seats {
			names = append(names, &matches[i].Seats[j].Name)
		}
	}
	return r.maskNames(ctx, names)
}
