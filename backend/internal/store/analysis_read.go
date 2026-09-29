package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// CalibrationRow is how well one model predicted games in one format, over
// every period together: Mode is "standard" or "walk_forward".
type CalibrationRow struct {
	Mode              string
	Model             string
	Format            int
	Matches           int
	Brier, ECE        float64
	LogLoss, Accuracy *float64
	Bins              []byte
}

// ModelRating is one player's rating and rank in one page model.
type ModelRating struct {
	Player    string
	Model     string
	Rating    float64
	Rank      *int
	Opponents int
}

// Calibration returns a published run's aggregated calibration, standard and
// walk-forward, for every model and format.
func (r *RatingsReader) Calibration(ctx context.Context, run int64) ([]CalibrationRow, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT 'standard', model_type, player_count, matches_evaluated, brier_score, ece, log_loss, accuracy, bin_data
		FROM standard_calibration WHERE run_id = $1 AND period_month = 'AGGREGATED'
		UNION ALL
		SELECT 'walk_forward', model_type, player_count, matches_evaluated, brier_score, ece, log_loss, accuracy, bin_data
		FROM walk_forward_calibration WHERE run_id = $1 AND period_month = 'AGGREGATED'
		ORDER BY 1, 2, 3`, run)
	if err != nil {
		return nil, fmt.Errorf("reading run %d's calibration: %w", run, err)
	}
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CalibrationRow, error) {
		var c CalibrationRow
		err := row.Scan(&c.Mode, &c.Model, &c.Format, &c.Matches, &c.Brier, &c.ECE, &c.LogLoss, &c.Accuracy, &c.Bins)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("reading run %d's calibration: %w", run, err)
	}
	return got, nil
}

// AnalysisRatings returns a published run's ratings in PlayerPageModels for a
// format, and the players active as upstream's analysis counts them: more than
// 10 games in the 365 days up to the run's newest game.
func (r *RatingsReader) AnalysisRatings(ctx context.Context, run int64, format int) ([]ModelRating, map[string]bool, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return nil, nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT player_name, model_type, rating, rank, opponents_count FROM player_ratings
		WHERE run_id = $1 AND player_count = $2 AND model_type = ANY($3)
		ORDER BY player_name, model_type`, run, format, PlayerPageModels)
	if err != nil {
		return nil, nil, fmt.Errorf("reading run %d's ratings: %w", run, err)
	}
	ratings, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ModelRating, error) {
		var m ModelRating
		err := row.Scan(&m.Player, &m.Model, &m.Rating, &m.Rank, &m.Opponents)
		return m, err
	})
	if err != nil {
		return nil, nil, fmt.Errorf("reading run %d's ratings: %w", run, err)
	}

	rows, err = r.pool.Query(ctx, `
		WITH run_games AS (
			SELECT DISTINCT m.match_id, m.date, m.player1, m.player2, m.player3, m.player4
			FROM matches m JOIN pairwise_matches p ON p.match_id = m.match_id AND p.run_id = $1),
		recent AS (
			SELECT * FROM run_games WHERE date >= (SELECT max(date) FROM run_games) - 365)
		SELECT name FROM (
			SELECT player1 AS name FROM recent UNION ALL SELECT player2 FROM recent
			UNION ALL SELECT player3 FROM recent WHERE player3 IS NOT NULL
			UNION ALL SELECT player4 FROM recent WHERE player4 IS NOT NULL) seats
		GROUP BY name HAVING count(*) > 10`, run)
	if err != nil {
		return nil, nil, fmt.Errorf("reading run %d's active players: %w", run, err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, nil, fmt.Errorf("reading run %d's active players: %w", run, err)
	}
	active := make(map[string]bool, len(names))
	for _, n := range names {
		active[n] = true
	}
	return ratings, active, nil
}
