// Admin's list of tournaments and groups: one brief row each, without seasons.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// AdminTournamentRow is one tournament or group as admin's list shows it: its settings, where it sits, how many
// games it holds and when, admin's description and the start of CGE's.
type AdminTournamentRow struct {
	Slug, Name, Kind, Parent string
	IsGroup                  bool
	Description              string
	// CGEAbout is the first 200 characters of CGE's latest description of it, on one line.
	CGEAbout                  string
	Visible, Rated, Community bool
	Games                     int
	FirstGame, LastGame       *time.Time
}

// TournamentRows returns every tournament and group in brief, in name order.
func (a *AdminStore) TournamentRows(ctx context.Context) ([]AdminTournamentRow, error) {
	rows, err := a.pool.Query(ctx, `
		WITH g AS (SELECT t.series_id, count(*)::int AS n, min(m.date) AS first, max(m.date) AS last
		           FROM games m JOIN groups gr USING (group_id) JOIN stages st USING (stage_id)
		           JOIN tournaments t USING (tournament_id) GROUP BY t.series_id)
		SELECT s.slug, s.name, s.kind, coalesce(p.slug, ''), s.is_group, s.description,
		       left(regexp_replace(coalesce((SELECT c.description FROM cge_tournaments c
		                                     WHERE c.series_id = s.series_id AND c.description <> ''
		                                     ORDER BY coalesce(c.started, c.created, c.finished) DESC NULLS LAST, c.cge_tournament DESC
		                                     LIMIT 1), ''), '\s+', ' ', 'g'), 200),
		       s.visible, EXISTS (SELECT 1 FROM ranking_series rt JOIN rankings r USING (ranking_id)
		                          WHERE r.slug = 'community' AND rt.series_id = s.series_id),
		       s.in_community_leaderboard, coalesce(g.n, 0), g.first, g.last
		FROM series s LEFT JOIN series p ON p.series_id = s.parent_id LEFT JOIN g ON g.series_id = s.series_id
		ORDER BY s.name, s.slug`)
	if err != nil {
		return nil, fmt.Errorf("listing tournaments for admin: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (AdminTournamentRow, error) {
		var r AdminTournamentRow
		return r, row.Scan(&r.Slug, &r.Name, &r.Kind, &r.Parent, &r.IsGroup, &r.Description, &r.CGEAbout,
			&r.Visible, &r.Rated, &r.Community, &r.Games, &r.FirstGame, &r.LastGame)
	})
	if err != nil {
		return nil, fmt.Errorf("listing tournaments for admin: %w", err)
	}
	return out, nil
}
