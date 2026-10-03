// Badges in admin: made once, given to players, and shown beside their title at once.

package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Badge is one badge admin made, with the players who hold it (by name).
type Badge struct {
	ID          int64
	Label       string
	Description string
	Players     []string
}

var (
	// ErrNoBadge means no badge has that id, or the player does not hold it.
	ErrNoBadge = errors.New("no such badge, or the player does not hold it")
	// ErrBadBadge means a badge's label is empty or over 16 characters.
	ErrBadBadge = errors.New("a badge's label is 1 to 16 characters")
	// ErrBadgeExists means another badge has that label.
	ErrBadgeExists = errors.New("another badge has that label")
)

// Badges returns every badge with its players, by label.
func (a *AdminStore) Badges(ctx context.Context) ([]Badge, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT b.badge_id, b.label, b.description,
		       coalesce(array_agg(pb.player_name ORDER BY lower(pb.player_name)) FILTER (WHERE pb.player_name IS NOT NULL), '{}')
		FROM badges b LEFT JOIN player_badges pb USING (badge_id)
		GROUP BY b.badge_id ORDER BY lower(b.label)`)
	if err != nil {
		return nil, fmt.Errorf("reading badges: %w", err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Badge])
	if err != nil {
		return nil, fmt.Errorf("reading badges: %w", err)
	}
	return got, nil
}

// cleanBadge trims a badge's label and description, or returns ErrBadBadge.
func cleanBadge(b Badge) (Badge, error) {
	b.Label, b.Description = strings.TrimSpace(b.Label), strings.TrimSpace(b.Description)
	if b.Label == "" || utf8.RuneCountInString(b.Label) > 16 {
		return b, ErrBadBadge
	}
	return b, nil
}

// labelTaken turns a unique violation on the label into ErrBadgeExists.
func labelTaken(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return ErrBadgeExists
	}
	return err
}

// AddBadge makes a badge and returns its id, or ErrBadBadge or ErrBadgeExists.
func (a *AdminStore) AddBadge(ctx context.Context, b Badge) (int64, error) {
	b, err := cleanBadge(b)
	if err != nil {
		return 0, err
	}
	var id int64
	err = a.pool.QueryRow(ctx, "INSERT INTO badges (label, description) VALUES ($1, $2) RETURNING badge_id", b.Label, b.Description).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("adding a badge: %w", labelTaken(err))
	}
	return id, nil
}

// SetBadge changes a badge's label and description, or returns ErrNoBadge, ErrBadBadge or ErrBadgeExists.
func (a *AdminStore) SetBadge(ctx context.Context, b Badge) error {
	b, err := cleanBadge(b)
	if err != nil {
		return err
	}
	tag, err := a.pool.Exec(ctx, "UPDATE badges SET label = $2, description = $3 WHERE badge_id = $1", b.ID, b.Label, b.Description)
	if err != nil {
		return fmt.Errorf("changing a badge: %w", labelTaken(err))
	}
	if tag.RowsAffected() == 0 {
		return ErrNoBadge
	}
	return nil
}

// RemoveBadge removes a badge from everyone who holds it, or returns ErrNoBadge.
func (a *AdminStore) RemoveBadge(ctx context.Context, id int64) error {
	tag, err := a.pool.Exec(ctx, "DELETE FROM badges WHERE badge_id = $1", id)
	if err != nil {
		return fmt.Errorf("removing a badge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoBadge
	}
	return nil
}

// GiveBadge gives a player a badge (again is no change), or returns ErrNoBadge or ErrNoPlayer.
func (a *AdminStore) GiveBadge(ctx context.Context, id int64, name string) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var badge, player bool
		err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM badges WHERE badge_id = $1), EXISTS (SELECT 1 FROM players WHERE name = $2)`,
			id, name).Scan(&badge, &player)
		switch {
		case err != nil:
			return fmt.Errorf("giving a badge: %w", err)
		case !badge:
			return ErrNoBadge
		case !player:
			return ErrNoPlayer
		}
		if _, err := tx.Exec(ctx, "INSERT INTO player_badges (player_name, badge_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", name, id); err != nil {
			return fmt.Errorf("giving a badge: %w", err)
		}
		return nil
	})
}

// TakeBadge takes a badge from a player, or returns ErrNoBadge when they do not hold it.
func (a *AdminStore) TakeBadge(ctx context.Context, id int64, name string) error {
	tag, err := a.pool.Exec(ctx, "DELETE FROM player_badges WHERE badge_id = $1 AND player_name = $2", id, name)
	if err != nil {
		return fmt.Errorf("taking a badge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoBadge
	}
	return nil
}
