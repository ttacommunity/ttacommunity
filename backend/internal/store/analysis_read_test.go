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

func TestComparedPlayersFindsNamesOrCountriesStrongestFirst(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "Annika", "hannah", "bob", "Zed")
	if _, err := pool.Exec(ctx, "UPDATE players SET country_code = 'PL' WHERE name IN ('bob', 'Zed')"); err != nil {
		t.Fatalf("setting countries: %v", err)
	}
	for name, c := range map[string]float64{"Annika": 1700, "hannah": 1800, "bob": 1600, "Zed": 1900} {
		addRating(t, pool, run, "glicko2_daneo", 0, name, intp(1), c)
	}
	addTitle(t, pool, run, "hannah", "GM", 2, "WC 2024")

	got, err := r.ComparedPlayers(ctx, run, 0, []string{"ann", "pl"})
	if err != nil {
		t.Fatalf("ComparedPlayers: %v", err)
	}
	var names []string
	for _, p := range got {
		names = append(names, p.Name)
	}
	if fmt.Sprint(names) != "[Zed hannah Annika bob]" {
		t.Errorf("names = %v, want every name containing ann and every PL player, by GlickoD C rating", names)
	}
	if got[1].Title == nil || *got[1].Title != "GM" || got[0].Country == nil || *got[0].Country != "PL" {
		t.Errorf("hannah's title = %v, Zed's country = %v, want GM and PL", got[1].Title, got[0].Country)
	}

	got, err = r.ComparedPlayers(ctx, run, 0, []string{"a_n"})
	if err != nil || len(got) != 0 {
		t.Errorf("ComparedPlayers(a_n) = %v, %v, want no players: _ is a character, not a wildcard", got, err)
	}
}

func TestComparedPlayersDefaultsToUpstreamsSixInItsOrderWithTheirRatings(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "saru", "a440", "Genghisip", "someone")
	addRating(t, pool, run, "whr", 3, "a440", intp(4), 1800)
	addRating(t, pool, run, "glicko2_daneo", 3, "a440", nil, 1900)
	addRating(t, pool, run, "glicko2_std_retro", 3, "a440", intp(1), 1950)
	addRating(t, pool, run, "glicko2_daneo", 0, "a440", intp(1), 1850)

	got, err := r.ComparedPlayers(ctx, run, 3, nil)
	if err != nil {
		t.Fatalf("ComparedPlayers: %v", err)
	}
	var names []string
	for _, p := range got {
		names = append(names, p.Name)
	}
	if fmt.Sprint(names) != "[Genghisip a440 saru]" {
		t.Errorf("names = %v, want the default players present, in upstream's order", names)
	}
	a := got[1].Ratings
	if len(a) != 2 || a[0].Model != "glicko2_daneo" || a[0].Rank != nil || a[1].Model != "whr" || *a[1].Rank != 4 ||
		a[1].Rating != 1900 || a[1].RD != 50 || a[1].CRating != 1800 || a[1].Opponents != 9 || a[1].WinRate != 58.3 {
		t.Errorf("a440's ratings = %+v, want GlickoD then WHR in format 3, no retro model", a)
	}
}

func TestActivityCountsPlayersGamesTitlesAndCountries(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	other := publishedRun(t, pool)
	addPlayers(t, pool, "gm1", "gm2", "m1", "p1", "s1", "fr1", "cz1", "unrated")
	_, err := pool.Exec(ctx, `UPDATE players SET country_code = CASE WHEN name = 'fr1' THEN 'FR' WHEN name = 'cz1' THEN 'CZ'
		WHEN name = 'unrated' THEN 'DE' ELSE 'PL' END`)
	if err != nil {
		t.Fatalf("setting countries: %v", err)
	}
	for name, rating := range map[string]float64{"gm1": 1900, "gm2": 1951, "m1": 1800, "p1": 1700, "s1": 1500, "fr1": 1600, "cz1": 1550} {
		addRating(t, pool, run, "glicko2_std", 0, name, intp(1), rating-100)
	}
	addRating(t, pool, run, "glicko2_std", 2, "s1", intp(1), 2400)
	addRating(t, pool, other, "glicko2_std", 0, "unrated", intp(1), 1500)
	// Active means a last game after the cutoff, in any model or format.
	_, err = pool.Exec(ctx, `UPDATE player_ratings SET last_played = CASE WHEN player_count = 2 THEN '2025-06-01'::date
		ELSE '2025-05-31'::date END WHERE run_id = $1 AND player_name IN ('gm1', 's1')`, run)
	if err != nil {
		t.Fatalf("setting last played: %v", err)
	}
	for name, title := range map[string]string{"gm1": "GM", "gm2": "GM", "m1": "M", "p1": "P", "s1": "S"} {
		addTitle(t, pool, run, name, title, 1, "x")
	}
	addTitle(t, pool, other, "fr1", "GM", 1, "x")
	addGame(t, pool, run, "2025-01-01", "Cup", Seat{"gm1", 1}, Seat{"m1", 0})
	addGame(t, pool, run, "2025-01-02", "Cup", Seat{"gm1", 1}, Seat{"m1", 0}, Seat{"p1", 0})
	addGame(t, pool, run, "2025-01-03", "Cup", Seat{"gm1", 1}, Seat{"m1", 0}, Seat{"p1", 0})
	addGame(t, pool, other, "2025-01-04", "Cup", Seat{"gm1", 1}, Seat{"m1", 0})

	got, err := r.Activity(ctx, run, day("2025-05-31"))
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if got.Active != 1 || got.Inactive != 7 {
		t.Errorf("active, inactive = %d, %d, want 1 (s1, by their 2-player game), 7 (gm1 on the cutoff, the rest before or unrated)", got.Active, got.Inactive)
	}
	if fmt.Sprint(got.Formats) != "[{2 1} {3 2}]" {
		t.Errorf("formats = %v, want the run's games only: [{2 1} {3 2}]", got.Formats)
	}
	if fmt.Sprint(got.Titles) != "[{GM 2} {S 1} {P 1} {M 1}]" {
		t.Errorf("titles = %v, want this run's, most first, ties in reverse code order as upstream's SQLite", got.Titles)
	}
	want := "[{PL 5 1925.5 1883.667 1837.75 1770.2} {FR 1 <nil> <nil> <nil> 1600} {CZ 1 <nil> <nil> <nil> 1550}]"
	if s := countryText(got.Countries); s != want {
		t.Errorf("countries = %s\nwant %s (Standard, all formats; titles from this run; unrated players left out; ties in reverse code order)", s, want)
	}
}

func countryText(cs []CountryRatings) string {
	f := func(x *float64) string {
		if x == nil {
			return "<nil>"
		}
		return fmt.Sprint(float64(int(*x*1000+0.5)) / 1000)
	}
	var out []string
	for _, c := range cs {
		out = append(out, fmt.Sprintf("{%s %d %s %s %s %v}", c.Country, c.Players, f(c.GM), f(c.MPlus), f(c.PPlus), c.All))
	}
	return fmt.Sprint(out)
}
