package store

import (
	"context"
	"fmt"
	"slices"
	"strconv"
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

	var slugs, names, kinds, tops, p2, p3, p4, seasonNames, stageNames, scorings, codes, results []string
	for _, t := range gamename.Tournaments {
		slugs, names, kinds = append(slugs, t.Slug), append(names, t.Name), append(kinds, t.Kind)
		rules := gamename.Rules[t.Slug]
		scorings, codes, results = append(scorings, rules.CommunityScoring), append(codes, rules.CommunityCode), append(results, rules.Results)
		tops = append(tops, strings.Join(t.TopDivisions, "|"))
		points := gamename.Scoring[t.Slug]
		p2, p3, p4 = append(p2, joinPoints(points[2])), append(p3, joinPoints(points[3])), append(p4, joinPoints(points[4]))
		levels := gamename.LevelNames[t.Slug]
		seasonNames, stageNames = append(seasonNames, levels[0]), append(stageNames, levels[1])
	}
	// A tournament first seen here is rated in the community ranking, as all were on the original site.
	if _, err := tx.Exec(ctx, `
		WITH added AS (
			INSERT INTO series (slug, name, original_name, kind, top_divisions, tournament_noun, stage_noun,
			                    community_scoring, community_code, results_rules)
			SELECT slug, name, name, kind, coalesce(string_to_array(nullif(tops, ''), '|'), '{}'), season_name, stage_name,
			       nullif(scoring, ''), nullif(code, ''), nullif(results, '')
			FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $10::text[], $11::text[],
			            $12::text[]) AS x(slug, name, kind, tops, season_name, stage_name, scoring, code, results)
			ON CONFLICT (slug) DO NOTHING
			RETURNING series_id, slug
		), ranked AS (
			INSERT INTO ranking_series (ranking_id, series_id)
			SELECT r.ranking_id, a.series_id FROM rankings r CROSS JOIN added a WHERE r.slug = 'community'
		)
		INSERT INTO settings (series_id, points_2p, points_3p, points_4p, source)
		SELECT a.series_id, string_to_array(nullif(x.p2, ''), ',')::float8[], string_to_array(nullif(x.p3, ''), ',')::float8[],
		       string_to_array(nullif(x.p4, ''), ',')::float8[], 'catalogue'
		FROM added a JOIN unnest($1::text[], $7::text[], $8::text[], $9::text[]) AS x(slug, p2, p3, p4) USING (slug)
		WHERE x.p2 <> '' OR x.p3 <> '' OR x.p4 <> ''`,
		slugs, names, kinds, tops, seasonNames, stageNames, p2, p3, p4, scorings, codes, results); err != nil {
		return Classified{}, fmt.Errorf("adding tournaments: %w", err)
	}

	rows, err := tx.Query(ctx, "SELECT game_id, name FROM games WHERE group_id IS NULL ORDER BY game_id")
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
		INSERT INTO tournaments (series_id, label, original_name)
		SELECT DISTINCT se.series_id, p.season, p.season
		FROM unnest($1::text[], $2::text[]) AS p(slug, season)
		JOIN series se USING (slug)
		ON CONFLICT (series_id, label) DO NOTHING`, tournaments, seasons); err != nil {
		return Classified{}, fmt.Errorf("adding seasons: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO stages (tournament_id, label, original_name)
		SELECT DISTINCT t.tournament_id, p.stage, p.stage
		FROM unnest($1::text[], $2::text[], $3::text[]) AS p(slug, season, stage)
		JOIN series se USING (slug)
		JOIN tournaments t ON t.series_id = se.series_id AND t.label = p.season
		ON CONFLICT (tournament_id, label) DO NOTHING`, tournaments, seasons, stages); err != nil {
		return Classified{}, fmt.Errorf("adding stages: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO groups (stage_id, name)
		SELECT DISTINCT st.stage_id, p.grp
		FROM unnest($1::text[], $2::text[], $3::text[], $4::text[]) AS p(slug, season, stage, grp)
		JOIN series se USING (slug)
		JOIN tournaments t ON t.series_id = se.series_id AND t.label = p.season
		JOIN stages st ON st.tournament_id = t.tournament_id AND st.label = p.stage
		ON CONFLICT DO NOTHING`, tournaments, seasons, stages, grps); err != nil {
		return Classified{}, fmt.Errorf("adding groups: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE games g
		SET group_id = gr.group_id, game_number = nullif(p.game, 0)
		FROM unnest($1::bigint[], $2::text[], $3::text[], $4::text[], $5::text[], $6::int[])
		         AS p(game_id, slug, season, stage, grp, game)
		JOIN series se ON se.slug = p.slug
		JOIN tournaments t ON t.series_id = se.series_id AND t.label = p.season
		JOIN stages st ON st.tournament_id = t.tournament_id AND st.label = p.stage
		JOIN groups gr ON gr.stage_id = st.stage_id AND gr.name = p.grp AND gr.cge_tournament IS NULL
		WHERE g.game_id = p.game_id`, ids, tournaments, seasons, stages, grps, games)
	if err != nil {
		return Classified{}, fmt.Errorf("placing matches: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO group_members (group_id, player_id)
		SELECT DISTINCT g.group_id, s.player_id FROM games g JOIN seats s USING (game_id)
		WHERE g.game_id = ANY($1) ON CONFLICT DO NOTHING`, ids); err != nil {
		return Classified{}, fmt.Errorf("adding group members: %w", err)
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

// joinPoints writes points per place as "5,2,0", or "" for none.
func joinPoints(points []float64) string {
	parts := make([]string, len(points))
	for i, p := range points {
		parts[i] = strconv.FormatFloat(p, 'g', -1, 64)
	}
	return strings.Join(parts, ",")
}
