// Players in admin: titles set by hand, which replace the engine's from its next run.

package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// TitleOverride is a player's title as admin sets it: one the engine gives (but WC, which comes from the
// World Championship), or "" to take the title away; with the reason, shown when hovering the badge.
type TitleOverride struct {
	Name, Title, Reason string
}

// AdminTitles are the titles admin can set; WC comes from the World Championship's podium.
var AdminTitles = []string{"SGM", "WCS", "WCB", "GM", "M", "P", "G", "S", "B", "W", ""}

var (
	// ErrNoPlayer means no player has that name.
	ErrNoPlayer = errors.New("no player has that name")
	// ErrBadTitle means a title admin cannot set, or no reason.
	ErrBadTitle = errors.New("a title the engine gives (not WC), or none, with a reason")
	// ErrNoTitle means admin has set no title for that player.
	ErrNoTitle = errors.New("no title is set in admin for that player")
)

// TitleOverrides returns every title set in admin, by player name.
func (a *AdminStore) TitleOverrides(ctx context.Context) ([]TitleOverride, error) {
	rows, err := a.pool.Query(ctx, "SELECT player_name, title, reason FROM player_title_overrides ORDER BY lower(player_name), player_name")
	if err != nil {
		return nil, fmt.Errorf("reading title overrides: %w", err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowToStructByPos[TitleOverride])
	if err != nil {
		return nil, fmt.Errorf("reading title overrides: %w", err)
	}
	return got, nil
}

// SetTitle sets a player's title, used from the next run. It returns ErrNoPlayer or ErrBadTitle.
func (a *AdminStore) SetTitle(ctx context.Context, o TitleOverride) error {
	o.Reason = strings.TrimSpace(o.Reason)
	known := false
	for _, t := range AdminTitles {
		known = known || t == o.Title
	}
	if !known || o.Reason == "" {
		return ErrBadTitle
	}
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM players WHERE name = $1)", o.Name).Scan(&exists); err != nil {
			return fmt.Errorf("setting a title: %w", err)
		}
		if !exists {
			return ErrNoPlayer
		}
		_, err := tx.Exec(ctx, `INSERT INTO player_title_overrides (player_name, title, reason) VALUES ($1, $2, $3)
			ON CONFLICT (player_name) DO UPDATE SET title = excluded.title, reason = excluded.reason`, o.Name, o.Title, o.Reason)
		if err != nil {
			return fmt.Errorf("setting a title: %w", err)
		}
		return nil
	})
}

// ClearTitle removes a player's title set in admin, so the next run derives it again, or returns ErrNoTitle.
func (a *AdminStore) ClearTitle(ctx context.Context, name string) error {
	tag, err := a.pool.Exec(ctx, "DELETE FROM player_title_overrides WHERE player_name = $1", name)
	if err != nil {
		return fmt.Errorf("clearing a title: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoTitle
	}
	return nil
}
