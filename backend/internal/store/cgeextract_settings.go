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
	StageID  int64
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
	return mergeStages(ctx, pool, stages)
}

// mergeStages is PlanCGEStages, reading through q: the pool, or the load's transaction.
func mergeStages(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, stages []cgeextract.StageSettings) ([]CGEStageWrite, error) {
	var writes []CGEStageWrite
	values := map[int64][][]any{}
	for _, s := range stages {
		var id int64
		var here bool
		err := q.QueryRow(ctx, `
			SELECT st.stage_id, t.run_here FROM stages st JOIN tournaments t USING (tournament_id)
			WHERE st.stage_id IN (SELECT stage_id FROM stages WHERE cge_tournament = $1 AND cge_stage_number = $2
			                      UNION SELECT stage_id FROM groups WHERE cge_tournament = $1 AND cge_stage_number = $2)
			ORDER BY st.stage_id LIMIT 1`,
			s.TournamentID, s.StageNumber).Scan(&id, &here)
		if err == pgx.ErrNoRows || here {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("finding our stage for CGE stage %d: %w", s.StageID, err)
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
// has made its stages, replacing what an earlier load stored in each column it gives.
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
		var params, sets []string
		for i, c := range columns {
			args = append(args, w.Settings[c])
			params = append(params, fmt.Sprintf("$%d", i+2))
			sets = append(sets, c+" = excluded."+c)
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO settings (stage_id, `+strings.Join(columns, ", ")+`, source) VALUES ($1, `+strings.Join(params, ", ")+`, 'cge')
			ON CONFLICT (stage_id) DO UPDATE SET `+strings.Join(sets, ", ")+`, source = 'cge'`, args...)
		if err != nil {
			return err
		}
		got.SettingsWritten += int(tag.RowsAffected())
	}
	return nil
}
