// CGE's extract, its settings: the ones we lack, each on our stage, tournament or series for it.
package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"ttacommunity/backend/internal/cgeextract"
)

// writeSettings fills the settings we lack from CGE's: a stage's on our stage for it, a cup year's on its
// tournament, a league's on its series. A setting we hold is kept.
func writeSettings(ctx context.Context, tx pgx.Tx, plan CGEPlan, in CGEInput, got *CGEApplied) error {
	for _, s := range in.Settings {
		target, id, err := settingsTarget(ctx, tx, plan, in, s)
		if err != nil {
			return err
		}
		if id == 0 {
			continue
		}
		points := func(n int) any {
			if p := s.Points[n]; p != nil {
				return *p
			}
			return nil
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO settings (`+target+`, timeout_penalty, points_2p, points_3p, points_4p, tiebreak_2p, tiebreak_3p_plus,
			                      ignore_ai_players, each_plays, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'cge')
			ON CONFLICT (`+target+`) DO UPDATE SET
			    timeout_penalty = coalesce(settings.timeout_penalty, excluded.timeout_penalty),
			    points_2p = coalesce(settings.points_2p, excluded.points_2p),
			    points_3p = coalesce(settings.points_3p, excluded.points_3p),
			    points_4p = coalesce(settings.points_4p, excluded.points_4p),
			    tiebreak_2p = coalesce(settings.tiebreak_2p, excluded.tiebreak_2p),
			    tiebreak_3p_plus = coalesce(settings.tiebreak_3p_plus, excluded.tiebreak_3p_plus),
			    ignore_ai_players = coalesce(settings.ignore_ai_players, excluded.ignore_ai_players),
			    each_plays = coalesce(settings.each_plays, excluded.each_plays),
			    source = CASE WHEN settings.source IN ('', 'catalogue') THEN 'cge' ELSE settings.source END`,
			id, s.TimeoutPenalty, points(2), points(3), points(4), s.Tiebreak2p, s.Tiebreak3pPlus, s.IgnoreAI, s.EachPlays)
		if err != nil {
			return err
		}
		got.SettingsWritten += int(tag.RowsAffected())
	}
	return nil
}

// settingsTarget returns the settings column and our id CGE's settings row belongs to, or 0 for none, as for
// a season run on this site.
func settingsTarget(ctx context.Context, tx pgx.Tx, plan CGEPlan, in CGEInput, s cgeextract.Settings) (string, int64, error) {
	var id int64
	if s.StageID != 0 {
		stage := in.Extract.Stages[s.StageID]
		err := tx.QueryRow(ctx, `
			SELECT stage_id FROM stages WHERE cge_tournament = $1 AND cge_stage_number = $2
			UNION SELECT stage_id FROM groups WHERE cge_tournament = $1 AND cge_stage_number = $2 LIMIT 1`,
			s.TournamentID, stage.Number).Scan(&id)
		if err == pgx.ErrNoRows {
			return "", 0, nil
		}
		return "stage_id", id, err
	}
	var tournament *int64
	var here bool
	err := tx.QueryRow(ctx, `SELECT c.tournament_id, coalesce(t.run_here, false) FROM cge_tournaments c
		LEFT JOIN tournaments t USING (tournament_id) WHERE c.cge_tournament = $1`, s.TournamentID).Scan(&tournament, &here)
	if err != nil && err != pgx.ErrNoRows {
		return "", 0, err
	}
	if here {
		return "", 0, nil
	}
	if tournament != nil {
		return "tournament_id", *tournament, nil
	}
	return "series_id", plan.Series[s.TournamentID], nil
}
