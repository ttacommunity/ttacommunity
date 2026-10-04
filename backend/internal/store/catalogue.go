package store

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/gamename"
)

// Classified counts what ClassifyMatches did: games placed, and each name it
// could not read, once, sorted.
type Classified struct {
	Placed   int
	Unplaced []string
}

// ClassifyMatches places every unplaced match in the tournament catalogue by
// its name, adding tournaments, seasons and stages as needed, in one
// transaction. A tournament already listed keeps its name. A match already
// placed is left alone; one whose name gamename cannot read stays unplaced.
func ClassifyMatches(ctx context.Context, pool *pgxpool.Pool) (Classified, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Classified{}, fmt.Errorf("starting to place matches: %w", err)
	}
	defer tx.Rollback(ctx)

	var slugs, names, kinds, tops []string
	for _, t := range gamename.Tournaments {
		slugs, names, kinds = append(slugs, t.Slug), append(names, t.Name), append(kinds, t.Kind)
		tops = append(tops, strings.Join(t.TopDivisions, "|"))
	}
	// A tournament first seen here is rated in the community ranking, as all were on the original site.
	if _, err := tx.Exec(ctx, `
		WITH added AS (
			INSERT INTO tournaments (slug, name, original_name, kind, top_divisions)
			SELECT slug, name, name, kind, coalesce(string_to_array(nullif(tops, ''), '|'), '{}')
			FROM unnest($1::text[], $2::text[], $3::text[], $4::text[]) AS x(slug, name, kind, tops)
			ON CONFLICT (slug) DO NOTHING
			RETURNING tournament_id
		)
		INSERT INTO ranking_tournaments (ranking_id, tournament_id)
		SELECT r.ranking_id, a.tournament_id FROM rankings r CROSS JOIN added a WHERE r.slug = 'community'`, slugs, names, kinds, tops); err != nil {
		return Classified{}, fmt.Errorf("adding tournaments: %w", err)
	}

	rows, err := tx.Query(ctx, "SELECT match_id, tournament FROM matches WHERE stage_id IS NULL ORDER BY match_id")
	if err != nil {
		return Classified{}, fmt.Errorf("reading unplaced matches: %w", err)
	}
	var (
		ids                                []int64
		tournaments, seasons, stages, grps []string
		games                              []int32
		unplaced                           = map[string]bool{}
	)
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return Classified{}, fmt.Errorf("reading unplaced matches: %w", err)
		}
		p, ok := gamename.Parse(name)
		if !ok {
			unplaced[name] = true
			continue
		}
		ids = append(ids, id)
		tournaments, seasons, stages = append(tournaments, p.Tournament), append(seasons, p.Season), append(stages, p.Stage)
		grps, games = append(grps, p.Group), append(games, int32(p.Game))
	}
	if err := rows.Err(); err != nil {
		return Classified{}, fmt.Errorf("reading unplaced matches: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO tournament_seasons (tournament_id, label, original_name)
		SELECT DISTINCT t.tournament_id, p.season, p.season
		FROM unnest($1::text[], $2::text[]) AS p(slug, season)
		JOIN tournaments t USING (slug)
		ON CONFLICT (tournament_id, label) DO NOTHING`, tournaments, seasons); err != nil {
		return Classified{}, fmt.Errorf("adding seasons: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tournament_stages (season_id, label, original_name)
		SELECT DISTINCT s.season_id, p.stage, p.stage
		FROM unnest($1::text[], $2::text[], $3::text[]) AS p(slug, season, stage)
		JOIN tournaments t USING (slug)
		JOIN tournament_seasons s ON s.tournament_id = t.tournament_id AND s.label = p.season
		ON CONFLICT (season_id, label) DO NOTHING`, tournaments, seasons, stages); err != nil {
		return Classified{}, fmt.Errorf("adding stages: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE matches m
		SET stage_id = st.stage_id, group_label = p.grp, game_number = nullif(p.game, 0)
		FROM unnest($1::bigint[], $2::text[], $3::text[], $4::text[], $5::text[], $6::int[])
		         AS p(match_id, slug, season, stage, grp, game)
		JOIN tournaments t ON t.slug = p.slug
		JOIN tournament_seasons s ON s.tournament_id = t.tournament_id AND s.label = p.season
		JOIN tournament_stages st ON st.season_id = s.season_id AND st.label = p.stage
		WHERE m.match_id = p.match_id`, ids, tournaments, seasons, stages, grps, games)
	if err != nil {
		return Classified{}, fmt.Errorf("placing matches: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Classified{}, fmt.Errorf("committing the placements: %w", err)
	}

	got := Classified{Placed: int(tag.RowsAffected())}
	for n := range unplaced {
		got.Unplaced = append(got.Unplaced, n)
	}
	slices.Sort(got.Unplaced)
	return got, nil
}
