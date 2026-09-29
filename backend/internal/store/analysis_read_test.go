//go:build integration

package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestCalibrationGivesTheAggregatedRowsOfBothModes(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	other := publishedRun(t, pool)
	add := func(table string, run int64, model string, format int, month string, brier float64) {
		t.Helper()
		_, err := pool.Exec(ctx, `INSERT INTO `+table+` (run_id, model_type, player_count, period_month, matches_evaluated,
			brier_score, ece, log_loss, accuracy, bin_data) VALUES ($1, $2, $3, $4, 100, $5, 0.01, 0.6, 64.5, '{"counts":[1,2]}')`,
			run, model, format, month, brier)
		if err != nil {
			t.Fatalf("adding %s: %v", table, err)
		}
	}
	add("standard_calibration", run, "glicko2_daneo", 0, "AGGREGATED", 0.21)
	add("walk_forward_calibration", run, "whr", 3, "AGGREGATED", 0.22)
	add("walk_forward_calibration", run, "whr", 3, "2024-01", 0.30)
	add("standard_calibration", other, "glicko2_daneo", 0, "AGGREGATED", 0.40)

	got, err := r.Calibration(ctx, run)
	if err != nil {
		t.Fatalf("Calibration: %v", err)
	}
	want := `[{standard glicko2_daneo 0 100 0.21 0.01 0.6 64.5 {"counts": [1, 2]}} {walk_forward whr 3 100 0.22 0.01 0.6 64.5 {"counts": [1, 2]}}]`
	if s := fmt.Sprint(calibText(got)); s != want {
		t.Errorf("Calibration = %s\nwant %s", s, want)
	}
}

func calibText(rows []CalibrationRow) []string {
	var out []string
	for _, c := range rows {
		out = append(out, fmt.Sprintf("{%s %s %d %d %v %v %v %v %s}", c.Mode, c.Model, c.Format, c.Matches, c.Brier, c.ECE, *c.LogLoss, *c.Accuracy, c.Bins))
	}
	return out
}

func TestAnalysisRatingsGivesThePageModelsAndWhoIsActive(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob", "cy")
	addRating(t, pool, run, "glicko2_daneo", 0, "ann", intp(1), 1800)
	addRating(t, pool, run, "whr_softer", 0, "ann", intp(2), 1700)
	addRating(t, pool, run, "glicko2_std_retro", 0, "ann", intp(1), 1900)
	addRating(t, pool, run, "glicko2_daneo", 2, "ann", intp(1), 1500)
	// In the year to the newest game (2025-09-11): ann 11 games, bob 10 (and one older), cy 11, one a day
	// after the cutoff; each opponent otherwise plays too few to count.
	start := day("2025-09-01")
	for i := range 10 {
		addGame(t, pool, run, start.AddDate(0, 0, i).Format(time.DateOnly), "Cup", Seat{"ann", 9}, Seat{"bob", 5})
	}
	addGame(t, pool, run, "2025-09-11", "Cup", Seat{"ann", 9}, Seat{"x1", 5})
	addGame(t, pool, run, "2024-08-31", "Cup", Seat{"bob", 9}, Seat{"x2", 5})
	for i := range 10 {
		addGame(t, pool, run, start.AddDate(0, 0, i).Format(time.DateOnly), "Cup", Seat{"cy", 9}, Seat{fmt.Sprint("y", i), 5})
	}
	addGame(t, pool, run, "2024-09-11", "Cup", Seat{"cy", 9}, Seat{"x3", 5})

	ratings, active, err := r.AnalysisRatings(ctx, run, 0)
	if err != nil {
		t.Fatalf("AnalysisRatings: %v", err)
	}
	if len(ratings) != 2 || ratings[0].Model != "glicko2_daneo" || ratings[1].Model != "whr_softer" || ratings[0].Opponents != 9 {
		t.Errorf("ratings = %+v, want ann's two page-model ratings in format 0", ratings)
	}
	if fmt.Sprint(active) != "map[ann:true cy:true]" {
		t.Errorf("active = %v, want ann and cy (more than 10 games from 2024-09-11 to 2025-09-11)", active)
	}
}
