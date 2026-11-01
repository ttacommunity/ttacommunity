// Blitz threads: each edition's organisers, whom every round's Discord thread messages tell players to ping, and
// the edition's own text for those messages.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// BlitzOrganiser is a member who organises an edition: their id, TTA username and Discord name.
type BlitzOrganiser struct {
	MemberID          int64
	Name, DiscordName string
}

// BlitzThreadText is an edition's own text for its thread messages, placeholders and all (checked by the caller).
type BlitzThreadText struct{ Message, NoDiscord string }

// ErrNoBlitzOrganiser means the member does not organise the edition.
var ErrNoBlitzOrganiser = errors.New("not an organiser of the edition")

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
	return a.inBlitzEdition(ctx, slug, edition, func(tx pgx.Tx, id int64) error {
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
	return a.inBlitzEdition(ctx, slug, edition, func(tx pgx.Tx, id int64) error {
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

// blitzThreadText reads an edition's own thread text, nil while it has none; only admin's login may.
func blitzThreadText(ctx context.Context, db conn, slug, edition string) (*BlitzThreadText, error) {
	var t BlitzThreadText
	err := db.QueryRow(ctx, `
		SELECT x.message, x.no_discord
		FROM blitz_thread_texts x JOIN tournaments t USING (tournament_id) JOIN series s USING (series_id)
		WHERE s.slug = $1 AND t.label = $2`, slug, edition).Scan(&t.Message, &t.NoDiscord)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s %s's thread text: %w", slug, edition, err)
	}
	return &t, nil
}

// SetBlitzThreadText replaces the edition's thread text, or returns ErrNoBlitzEdition.
func (a *AdminStore) SetBlitzThreadText(ctx context.Context, slug, edition string, t BlitzThreadText) error {
	return a.inBlitzEdition(ctx, slug, edition, func(tx pgx.Tx, id int64) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO blitz_thread_texts (tournament_id, message, no_discord) VALUES ($1, $2, $3)
			ON CONFLICT (tournament_id) DO UPDATE SET message = excluded.message, no_discord = excluded.no_discord`,
			id, t.Message, t.NoDiscord); err != nil {
			return fmt.Errorf("setting %s %s's thread text: %w", slug, edition, err)
		}
		return nil
	})
}

// ResetBlitzThreadText takes the edition's own thread text away, so its threads use the default, or returns
// ErrNoBlitzEdition.
func (a *AdminStore) ResetBlitzThreadText(ctx context.Context, slug, edition string) error {
	return a.inBlitzEdition(ctx, slug, edition, func(tx pgx.Tx, id int64) error {
		if _, err := tx.Exec(ctx, "DELETE FROM blitz_thread_texts WHERE tournament_id = $1", id); err != nil {
			return fmt.Errorf("resetting %s %s's thread text: %w", slug, edition, err)
		}
		return nil
	})
}
