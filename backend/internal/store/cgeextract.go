package store

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/cgeextract"
)

// CGEPlan is how CGE's extract meets our games, before anything is written: its games, how they match ours,
// the accounts each name has played under, and our series for each linked CGE tournament. RunHere counts the
// games it leaves out, of CGE tournaments linked to seasons run on this site.
type CGEPlan struct {
	Games    []cgeextract.Game
	Ours     []cgeextract.OurGame
	Matching cgeextract.Matching
	Names    map[string][]int
	Series   map[int]int64
	RunHere  int
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
	return plan, nil
}
