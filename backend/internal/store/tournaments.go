package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/divisioncsv"
)

// DivisionsReplaced counts what ReplaceDivisions did: rows written, and rows
// naming someone who is not a player.
type DivisionsReplaced struct {
	Written int
	Unknown int
}

// ReplaceDivisions replaces every recorded tournament division with divs, in
// one transaction. A row naming someone who is not a player is counted, not
// written.
func ReplaceDivisions(ctx context.Context, pool *pgxpool.Pool, divs []divisioncsv.Division) (DivisionsReplaced, error) {
	series := make([]string, len(divs))
	seasons := make([]int32, len(divs))
	names := make([]string, len(divs))
	players := make([]string, len(divs))
	sources := make([]string, len(divs))
	for i, d := range divs {
		series[i], seasons[i], names[i], players[i], sources[i] = d.Series, int32(d.Season), d.Division, d.Player, d.Source
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return DivisionsReplaced{}, fmt.Errorf("starting the divisions load: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "DELETE FROM tournament_group_divisions"); err != nil {
		return DivisionsReplaced{}, fmt.Errorf("clearing recorded divisions: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO tournament_group_divisions (series, season, division, player_name, source)
		SELECT d.series, d.season, d.division, p.name, d.source
		FROM unnest($1::text[], $2::int[], $3::text[], $4::text[], $5::text[])
		     AS d(series, season, division, player, source)
		JOIN players p ON p.name = d.player`,
		series, seasons, names, players, sources)
	if err != nil {
		return DivisionsReplaced{}, fmt.Errorf("writing recorded divisions: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return DivisionsReplaced{}, fmt.Errorf("committing recorded divisions: %w", err)
	}
	written := int(tag.RowsAffected())
	return DivisionsReplaced{Written: written, Unknown: len(divs) - written}, nil
}
