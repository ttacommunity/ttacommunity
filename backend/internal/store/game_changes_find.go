// What admin's game changes look up: the game a change removes or replaces, by its code, with its seats, and a
// season's groups a game can be added to.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ChangeableGame is a game as a change sees it: its id, name, code, date, state, group (0 for none),
// number in it (nil for none) and seats, first place first.
type ChangeableGame struct {
	ID     int64
	Name   string
	Code   string
	Date   time.Time
	State  string
	Group  int64
	Number *int
	Seats  []GameSeatEntry
}

// SeasonStage is a stage of a season ("" for a season's only stage) and its groups.
type SeasonStage struct {
	Label  string
	Groups []SeasonGroup
}

// SeasonGroup is a group a game can be added to: its id, name, games in play, and the number a game added gets.
type SeasonGroup struct {
	ID         int64
	Name       string
	Games      int
	NextNumber int
}

// GameByCode returns the game with the replay code, typed in any case, with its seats, or ErrNoGame.
func (a *AdminStore) GameByCode(ctx context.Context, code string) (ChangeableGame, error) {
	var g ChangeableGame
	var players, outcomes []string
	var places []int
	var cultures []float64
	err := a.pool.QueryRow(ctx, `
		SELECT g.game_id, g.name, g.share_code, g.date, g.state, coalesce(g.group_id, 0), g.game_number,
		       coalesce(array_agg(p.name ORDER BY s.place, s.seat) FILTER (WHERE s.seat IS NOT NULL), '{}'),
		       coalesce(array_agg(s.place ORDER BY s.place, s.seat) FILTER (WHERE s.seat IS NOT NULL), '{}'),
		       coalesce(array_agg(coalesce(s.outcome, '') ORDER BY s.place, s.seat) FILTER (WHERE s.seat IS NOT NULL), '{}'),
		       coalesce(array_agg(s.culture ORDER BY s.place, s.seat) FILTER (WHERE s.seat IS NOT NULL), '{}')
		FROM games g LEFT JOIN seats s USING (game_id) LEFT JOIN players p USING (player_id)
		WHERE g.share_code = $1
		GROUP BY g.game_id`, typedCode(code)).Scan(&g.ID, &g.Name, &g.Code, &g.Date, &g.State, &g.Group, &g.Number,
		&players, &places, &outcomes, &cultures)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChangeableGame{}, ErrNoGame
	}
	if err != nil {
		return ChangeableGame{}, fmt.Errorf("finding the game coded %s: %w", code, err)
	}
	g.Seats = make([]GameSeatEntry, len(players))
	for i := range players {
		g.Seats[i] = GameSeatEntry{Player: players[i], Place: places[i], Outcome: outcomes[i], Score: cultures[i]}
	}
	return g, nil
}

// SeasonGroups returns the season's stages, in order, each with its groups, or ErrNoTournament when it has no group
// (or there is no such season).
func (a *AdminStore) SeasonGroups(ctx context.Context, slug, season string) ([]SeasonStage, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT st.stage_id, st.label, gr.group_id, gr.name,
		       count(m.game_id) FILTER (WHERE m.state NOT IN ('replaced', 'removed')),
		       coalesce(max(m.game_number), 0) + 1
		FROM series t JOIN tournaments se USING (series_id) JOIN stages st USING (tournament_id) JOIN groups gr USING (stage_id)
		LEFT JOIN games m USING (group_id)
		WHERE t.slug = $1 AND se.label = $2
		GROUP BY st.stage_id, gr.group_id
		ORDER BY st.stage_id, gr.name, gr.group_id`, slug, season)
	if err != nil {
		return nil, fmt.Errorf("reading %s %s's groups: %w", slug, season, err)
	}
	var out []SeasonStage
	var stage, last int64
	var label string
	var g SeasonGroup
	_, err = pgx.ForEachRow(rows, []any{&stage, &label, &g.ID, &g.Name, &g.Games, &g.NextNumber}, func() error {
		if len(out) == 0 || stage != last {
			out, last = append(out, SeasonStage{Label: label}), stage
		}
		out[len(out)-1].Groups = append(out[len(out)-1].Groups, g)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading %s %s's groups: %w", slug, season, err)
	}
	if len(out) == 0 {
		return nil, ErrNoTournament
	}
	return out, nil
}
