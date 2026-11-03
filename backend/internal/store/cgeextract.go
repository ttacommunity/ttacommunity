package store

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/cgeextract"
)

// CGEPlan is how CGE's extract meets our games, before anything is written: its games, how they match ours,
// the accounts each name has played under, and our series for each linked CGE tournament. RunHere counts the
// games it leaves out, of CGE tournaments linked to seasons run on this site. Removed are our games CGE removed
// (their CGE tournament is in the extract, but not their session), RemovedPlayed how many of them were played.
type CGEPlan struct {
	Games         []cgeextract.Game
	Ours          []cgeextract.OurGame
	Matching      cgeextract.Matching
	Names         map[string][]int
	Series        map[int]int64
	RunHere       int
	Removed       []int64
	RemovedPlayed int
}

// PlanCGEExtract reads our games and matches the extract's to them, confirmed pairs first, writing nothing.
// The games of a CGE tournament linked to a season run on this site are left out.
func PlanCGEExtract(ctx context.Context, pool *pgxpool.Pool, x cgeextract.Extract, confirmed map[string]int64) (CGEPlan, error) {
	plan := CGEPlan{Names: map[string][]int{}, Series: map[int]int64{}}
	rows, err := pool.Query(ctx, `SELECT c.cge_tournament FROM cge_tournaments c JOIN tournaments t USING (tournament_id) WHERE t.run_here`)
	if err != nil {
		return CGEPlan{}, fmt.Errorf("reading the CGE tournaments run on this site: %w", err)
	}
	here, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return CGEPlan{}, fmt.Errorf("reading the CGE tournaments run on this site: %w", err)
	}
	for _, g := range cgeextract.Games(x.Seats) {
		if slices.Contains(here, g.TournamentID) {
			plan.RunHere++
			continue
		}
		plan.Games = append(plan.Games, g)
	}
	addName := func(name string, account int) {
		k := strings.ToLower(name)
		for _, a := range plan.Names[k] {
			if a == account {
				return
			}
		}
		plan.Names[k] = append(plan.Names[k], account)
	}
	for _, s := range x.Seats {
		if s.PlayerID != 0 {
			addName(s.PlayerName, s.PlayerID)
		}
	}
	rows, err = pool.Query(ctx, `
		SELECT p.name, s.cge_user FROM seats s JOIN players p USING (player_id) WHERE s.cge_user IS NOT NULL
		UNION SELECT name, cge_user FROM player_accounts`)
	if err != nil {
		return CGEPlan{}, fmt.Errorf("reading the names accounts played under: %w", err)
	}
	var name string
	var account int
	for rows.Next() {
		if err := rows.Scan(&name, &account); err != nil {
			return CGEPlan{}, fmt.Errorf("reading the names accounts played under: %w", err)
		}
		addName(name, account)
	}
	if err := rows.Err(); err != nil {
		return CGEPlan{}, fmt.Errorf("reading the names accounts played under: %w", err)
	}

	// A CGE tournament is linked through the catalogue, or through the stages and groups its games sit in.
	rows, err = pool.Query(ctx, `
		SELECT cge_tournament, series_id FROM cge_tournaments WHERE series_id IS NOT NULL
		UNION SELECT st.cge_tournament, t.series_id FROM stages st JOIN tournaments t USING (tournament_id)
		      WHERE st.cge_tournament IS NOT NULL
		UNION SELECT gr.cge_tournament, t.series_id FROM groups gr JOIN stages st USING (stage_id)
		      JOIN tournaments t USING (tournament_id) WHERE gr.cge_tournament IS NOT NULL`)
	if err != nil {
		return CGEPlan{}, fmt.Errorf("reading the CGE tournaments we link: %w", err)
	}
	var cgeTournament int
	var series int64
	for rows.Next() {
		if err := rows.Scan(&cgeTournament, &series); err != nil {
			return CGEPlan{}, fmt.Errorf("reading the CGE tournaments we link: %w", err)
		}
		plan.Series[cgeTournament] = series
	}
	if err := rows.Err(); err != nil {
		return CGEPlan{}, fmt.Errorf("reading the CGE tournaments we link: %w", err)
	}

	rows, err = pool.Query(ctx, `
		SELECT g.game_id, coalesce(l.series_id, 0), coalesce(g.share_code, ''), s.seat, p.name,
		       coalesce(s.cge_user, 0), coalesce(s.recorded_score, s.culture)
		FROM games g JOIN game_lines l USING (game_id) JOIN seats s USING (game_id) JOIN players p USING (player_id)
		ORDER BY g.game_id, s.seat`)
	if err != nil {
		return CGEPlan{}, fmt.Errorf("reading our games: %w", err)
	}
	var ours []cgeextract.OurGame
	for rows.Next() {
		var g cgeextract.OurGame
		var s cgeextract.OurSeat
		if err := rows.Scan(&g.ID, &g.Series, &g.ShareCode, &s.Seat, &s.Name, &s.Account, &s.Score); err != nil {
			return CGEPlan{}, fmt.Errorf("reading our games: %w", err)
		}
		if n := len(ours); n == 0 || ours[n-1].ID != g.ID {
			ours = append(ours, g)
		}
		ours[len(ours)-1].Seats = append(ours[len(ours)-1].Seats, s)
	}
	if err := rows.Err(); err != nil {
		return CGEPlan{}, fmt.Errorf("reading our games: %w", err)
	}
	plan.Ours = ours
	plan.Matching = cgeextract.Match(plan.Games, ours, plan.Names, plan.Series, confirmed)
	if err := planRemoved(ctx, pool, x, here, &plan); err != nil {
		return CGEPlan{}, fmt.Errorf("reading the games CGE removed: %w", err)
	}
	if len(plan.Removed) > 0 {
		// A game CGE removed is no copy of a CGE game: matched again without them, none is held back for one.
		plan.Ours = slices.DeleteFunc(ours, func(o cgeextract.OurGame) bool { return slices.Contains(plan.Removed, o.ID) })
		plan.Matching = cgeextract.Match(plan.Games, plan.Ours, plan.Names, plan.Series, confirmed)
	}
	return plan, nil
}

// planRemoved finds our games CGE removed: in a CGE tournament the extract holds, not of a season run on this site
// (here), with a CGE session the extract does not hold, and matched to none of its games.
func planRemoved(ctx context.Context, pool *pgxpool.Pool, x cgeextract.Extract, here []int, plan *CGEPlan) error {
	sessions := map[int]bool{}
	for _, s := range x.Seats {
		sessions[s.SessionID] = true
	}
	rows, err := pool.Query(ctx, `
		SELECT g.game_id, g.state = 'played' FROM games g
		JOIN groups gr USING (group_id) JOIN stages st USING (stage_id) JOIN tournaments t USING (tournament_id)
		WHERE g.cge_session IS NOT NULL AND NOT t.run_here
		  AND coalesce(gr.cge_tournament, st.cge_tournament) = ANY($2) AND NOT coalesce(gr.cge_tournament, st.cge_tournament) = ANY($3)
		  AND NOT EXISTS (SELECT FROM unnest($1::int[]) AS s(id) WHERE s.id = g.cge_session)
		ORDER BY g.game_id`, slices.Collect(maps.Keys(sessions)), slices.Collect(maps.Keys(x.Tournaments)), here)
	if err != nil {
		return err
	}
	var id int64
	var played bool
	_, err = pgx.ForEachRow(rows, []any{&id, &played}, func() error {
		if _, matched := plan.Matching.Matched[id]; matched {
			return nil
		}
		plan.Removed = append(plan.Removed, id)
		if played {
			plan.RemovedPlayed++
		}
		return nil
	})
	return err
}
