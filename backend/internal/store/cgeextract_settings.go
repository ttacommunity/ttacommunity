// CGE's extract, its settings: each CGE stage's, on our stage for it.
package store

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/cgeextract"
)

// CGEStageWrite is what the load writes on one stage of ours from the CGE stages for it: each setting they all
// agree on, by column, and the columns they disagree on (sorted), which keep what the stage had.
type CGEStageWrite struct {
	StageID int64
	// Stage names it: its series, season and label ("Royal League 6, Tiebreak"), for those it has disagreements on.
	Stage    string
	Settings map[string]any
	Disagree []string
}

// stageColumns are the settings columns a CGE stage gives, in stageValues' order.
var stageColumns = []string{"timeout_penalty", "points_2p", "points_3p", "points_4p", "ignore_ai_players", "each_plays_2p",
	"each_plays_3p", "each_plays_4p", "tiebreak_2p", "tiebreak_3p_plus", "winner_score_minimum"}

// stageValues returns s's settings in stageColumns' order.
func stageValues(s cgeextract.StageSettings) []any {
	var least any
	if s.WinnerScoreMinimum != nil {
		least = *s.WinnerScoreMinimum
	}
	return []any{s.TimeoutPenalty, s.Points[2], s.Points[3], s.Points[4], s.IgnoreAI, s.EachPlays[2], s.EachPlays[3],
		s.EachPlays[4], s.Tiebreak2p, s.Tiebreak3pPlus, least}
}

// PlanCGEStages finds our stage for each of CGE's stages (through our stages or groups linked to it; none for a
// season run on this site) and what to write on it: the settings every CGE stage for it agrees on. It writes
// nothing, so a stage the load would make is not in it; the load writes that one too.
func PlanCGEStages(ctx context.Context, pool *pgxpool.Pool, stages []cgeextract.StageSettings) ([]CGEStageWrite, error) {
	writes, err := mergeStages(ctx, pool, stages)
	if err != nil {
		return nil, err
	}
	for i, w := range writes {
		if len(w.Disagree) == 0 {
			continue
		}
		if err := pool.QueryRow(ctx, `SELECT se.name || ' ' || t.label || ', ' || st.label FROM stages st
			JOIN tournaments t USING (tournament_id) JOIN series se USING (series_id) WHERE st.stage_id = $1`,
			w.StageID).Scan(&writes[i].Stage); err != nil {
			return nil, fmt.Errorf("naming stage %d: %w", w.StageID, err)
		}
	}
	return writes, nil
}

// rowQuerier is the pool or the load's transaction.
type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// cgeStage returns our stage for CGE tournament's stage number, through our stages or groups linked to it, or 0
// for none or for one of a season run on this site.
func cgeStage(ctx context.Context, q rowQuerier, tournament, number int) (int64, error) {
	var id int64
	var here bool
	err := q.QueryRow(ctx, `
		SELECT st.stage_id, t.run_here FROM stages st JOIN tournaments t USING (tournament_id)
		WHERE st.stage_id IN (SELECT stage_id FROM stages WHERE cge_tournament = $1 AND cge_stage_number = $2
		                      UNION SELECT stage_id FROM groups WHERE cge_tournament = $1 AND cge_stage_number = $2)
		ORDER BY st.stage_id LIMIT 1`, tournament, number).Scan(&id, &here)
	if err == pgx.ErrNoRows || here {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("finding our stage for CGE tournament %d's stage %d: %w", tournament, number, err)
	}
	return id, nil
}

// mergeStages is PlanCGEStages, reading through q.
func mergeStages(ctx context.Context, q rowQuerier, stages []cgeextract.StageSettings) ([]CGEStageWrite, error) {
	var writes []CGEStageWrite
	values := map[int64][][]any{}
	for _, s := range stages {
		id, err := cgeStage(ctx, q, s.TournamentID, s.StageNumber)
		if err != nil {
			return nil, err
		}
		if id == 0 {
			continue
		}
		if _, ok := values[id]; !ok {
			writes = append(writes, CGEStageWrite{StageID: id, Settings: map[string]any{}})
		}
		values[id] = append(values[id], stageValues(s))
	}
	for i, w := range writes {
		for c, column := range stageColumns {
			first := values[w.StageID][0][c]
			if slices.ContainsFunc(values[w.StageID], func(v []any) bool { return !reflect.DeepEqual(v[c], first) }) {
				writes[i].Disagree = append(writes[i].Disagree, column)
				continue
			}
			w.Settings[column] = first
		}
		slices.Sort(writes[i].Disagree)
	}
	return writes, nil
}

// writeSettings writes each stage's settings from CGE's stages file, merged as PlanCGEStages does once the load
// has made its stages, replacing what an earlier load stored in each column it gives; it counts the stages changed.
func writeSettings(ctx context.Context, tx pgx.Tx, _ CGEPlan, in CGEInput, got *CGEApplied) error {
	writes, err := mergeStages(ctx, tx, in.Stages)
	if err != nil {
		return err
	}
	for _, w := range writes {
		if len(w.Settings) == 0 {
			continue
		}
		columns := slices.Sorted(maps.Keys(w.Settings))
		args := []any{w.StageID}
		var params, sets, ours, theirs []string
		for i, c := range columns {
			args = append(args, w.Settings[c])
			params = append(params, fmt.Sprintf("$%d", i+2))
			sets = append(sets, c+" = excluded."+c)
			ours, theirs = append(ours, "settings."+c), append(theirs, "excluded."+c)
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO settings (stage_id, `+strings.Join(columns, ", ")+`, source) VALUES ($1, `+strings.Join(params, ", ")+`, 'cge')
			ON CONFLICT (stage_id) DO UPDATE SET `+strings.Join(sets, ", ")+`, source = 'cge'
			WHERE (`+strings.Join(ours, ", ")+`, settings.source) IS DISTINCT FROM (`+strings.Join(theirs, ", ")+`, 'cge')`, args...)
		if err != nil {
			return err
		}
		got.SettingsWritten += int(tag.RowsAffected())
	}
	return nil
}

// writeStages records on each stage of ours its CGE stages' times, from the games' rows, and their name, from the
// stages file. A stage of ours spans its CGE stages: their earliest start and latest finish, widened, never
// narrowed, by a later delivery; a name only when they share one.
func writeStages(ctx context.Context, tx pgx.Tx, _ CGEPlan, in CGEInput, _ *CGEApplied) error {
	for _, id := range slices.Sorted(maps.Keys(in.Extract.Stages)) {
		st := in.Extract.Stages[id]
		ours, err := cgeStage(ctx, tx, st.TournamentID, st.Number)
		if err != nil {
			return err
		}
		if ours == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `UPDATE stages SET cge_started_at = least(cge_started_at, $2), cge_finished_at = greatest(cge_finished_at, $3)
			WHERE stage_id = $1`, ours, st.Started, st.Finished); err != nil {
			return err
		}
	}
	names := map[int64][]string{}
	var order []int64
	for _, s := range in.Stages {
		ours, err := cgeStage(ctx, tx, s.TournamentID, s.StageNumber)
		if err != nil {
			return err
		}
		if ours == 0 {
			continue
		}
		if _, ok := names[ours]; !ok {
			order = append(order, ours)
		}
		names[ours] = append(names[ours], s.StageName)
	}
	for _, ours := range order {
		var name *string
		if n := names[ours]; !slices.ContainsFunc(n, func(o string) bool { return o != n[0] }) && n[0] != "" {
			name = &n[0]
		}
		if _, err := tx.Exec(ctx, "UPDATE stages SET cge_name = $2 WHERE stage_id = $1", ours, name); err != nil {
			return err
		}
	}
	return nil
}
