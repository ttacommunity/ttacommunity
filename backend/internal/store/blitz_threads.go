// Blitz threads: each edition's organisers, whom every round's Discord thread messages tell players to ping, and
// each round's deadline to play by.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// BlitzOrganiser is a member who organises an edition: their id, TTA username and Discord name.
type BlitzOrganiser struct {
	MemberID          int64
	Name, DiscordName string
}

// ErrNoBlitzOrganiser means the member does not organise the edition.
var ErrNoBlitzOrganiser = errors.New("not an organiser of the edition")

// ErrBlitzRoundFinished means every game of the round has a result: its deadline no longer changes.
var ErrBlitzRoundFinished = errors.New("the round is finished: its deadline no longer changes")

// blitzOrganisers reads an edition's organisers in the order added; only admin's login may.
func blitzOrganisers(ctx context.Context, db conn, slug, edition string) ([]BlitzOrganiser, error) {
	rows, err := db.Query(ctx, `
		SELECT o.member_id, m.tta_name, m.discord_name
		FROM blitz_organisers o JOIN members m USING (member_id)
		JOIN tournaments t ON t.tournament_id = o.tournament_id JOIN series s USING (series_id)
		WHERE s.slug = $1 AND t.label = $2
		ORDER BY o.added_at, o.member_id`, slug, edition)
	if err != nil {
		return nil, fmt.Errorf("reading %s %s's organisers: %w", slug, edition, err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (BlitzOrganiser, error) {
		var o BlitzOrganiser
		return o, row.Scan(&o.MemberID, &o.Name, &o.DiscordName)
	})
	if err != nil {
		return nil, fmt.Errorf("reading %s %s's organisers: %w", slug, edition, err)
	}
	return out, nil
}

// AddBlitzOrganiser makes a member an organiser of the edition (adding one again changes nothing), or returns
// ErrNoBlitzEdition or ErrNoMember.
func (a *AdminStore) AddBlitzOrganiser(ctx context.Context, slug, edition string, member int64) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		id, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		var known bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM members WHERE member_id = $1)", member).Scan(&known); err != nil {
			return fmt.Errorf("adding an organiser to %s %s: %w", slug, edition, err)
		}
		if !known {
			return ErrNoMember
		}
		if _, err := tx.Exec(ctx, "INSERT INTO blitz_organisers (tournament_id, member_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", id, member); err != nil {
			return fmt.Errorf("adding an organiser to %s %s: %w", slug, edition, err)
		}
		return nil
	})
}

// RemoveBlitzOrganiser stops a member organising the edition, or returns ErrNoBlitzEdition or ErrNoBlitzOrganiser.
func (a *AdminStore) RemoveBlitzOrganiser(ctx context.Context, slug, edition string, member int64) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		id, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "DELETE FROM blitz_organisers WHERE tournament_id = $1 AND member_id = $2", id, member)
		if err != nil {
			return fmt.Errorf("removing an organiser of %s %s: %w", slug, edition, err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNoBlitzOrganiser
		}
		return nil
	})
}

// SetBlitzRoundDeadline sets the deadline to play a round by, draft or published, and the zone it was given in
// (checked by the caller), or returns ErrNoBlitzEdition, ErrNoBlitzRound or, once every game of the round has a
// result, ErrBlitzRoundFinished.
func (a *AdminStore) SetBlitzRoundDeadline(ctx context.Context, slug, edition string, roundID int64, at time.Time, zone string) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		id, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		if _, err := blitzRoundKind(ctx, tx, id, roundID); err != nil {
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
