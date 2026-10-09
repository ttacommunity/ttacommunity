// How many games and players each season and stage of a tournament holds, for admin's seasons card.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// SeasonCount is how many games and players a season holds, across its stages, and each stage's.
type SeasonCount struct {
	Season         string
	Games, Players int
	Stages         []StageCount
}

// StageCount is one stage's number (CGE's, else the last number in its label, else 0), how many games it holds and
// how many of them were played, and how many players are in them.
type StageCount struct {
	Stage                    string
	Number                   int
	Games, Finished, Players int
}

// SeasonCounts returns each season of the tournament slug with its games and players, and each stage's, in label
// order; ErrNoTournament when there is no such tournament.
func (a *AdminStore) SeasonCounts(ctx context.Context, slug string) ([]SeasonCount, error) {
	var id int64
	if err := a.pool.QueryRow(ctx, "SELECT series_id FROM series WHERE slug = $1", slug).Scan(&id); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoTournament
	} else if err != nil {
		return nil, fmt.Errorf("counting %s's games: %w", slug, err)
	}
	rows, err := a.pool.Query(ctx, `
		WITH st AS (SELECT s.stage_id, s.label, t.label AS season, t.tournament_id,
		                   coalesce(s.cge_stage_number, substring(s.label FROM '([0-9]+)[^0-9]*$')::int, 0) AS number
		            FROM stages s JOIN tournaments t USING (tournament_id) WHERE t.series_id = $1),
		sp AS (SELECT DISTINCT st.stage_id, s.player_id FROM st JOIN groups gr USING (stage_id)
		       JOIN games g USING (group_id) JOIN seats s USING (game_id)),
		gc AS (SELECT gr.stage_id, count(*)::int AS games, (count(*) FILTER (WHERE g.state = 'played'))::int AS finished
		       FROM st JOIN groups gr USING (stage_id) JOIN games g USING (group_id) GROUP BY gr.stage_id),
		pc AS (SELECT stage_id, count(*)::int AS players FROM sp GROUP BY stage_id),
		season AS (SELECT st.tournament_id, count(DISTINCT sp.player_id)::int AS players FROM sp JOIN st USING (stage_id) GROUP BY 1)
		SELECT st.season, st.label, st.number, coalesce(gc.games, 0), coalesce(gc.finished, 0), coalesce(pc.players, 0),
		       coalesce(sn.players, 0)
		FROM st LEFT JOIN gc USING (stage_id) LEFT JOIN pc USING (stage_id) LEFT JOIN season sn USING (tournament_id)
		ORDER BY st.season, st.stage_id`, id)
	if err != nil {
		return nil, fmt.Errorf("counting %s's games: %w", slug, err)
	}
	var out []SeasonCount
	var season, stage string
	var number, games, finished, players, seasonPlayers int
	if _, err := pgx.ForEachRow(rows, []any{&season, &stage, &number, &games, &finished, &players, &seasonPlayers}, func() error {
		if len(out) == 0 || out[len(out)-1].Season != season {
			out = append(out, SeasonCount{Season: season, Players: seasonPlayers, Stages: []StageCount{}})
		}
		c := &out[len(out)-1]
		c.Games += games
		c.Stages = append(c.Stages, StageCount{Stage: stage, Number: number, Games: games, Finished: finished, Players: players})
		return nil
	}); err != nil {
		return nil, fmt.Errorf("counting %s's games: %w", slug, err)
	}
	return out, nil
}
