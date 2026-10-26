//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
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
	add("walk_forward_calibration", run, "glicko2_std", 3, "AGGREGATED", 0.22)
	add("walk_forward_calibration", run, "glicko2_std", 3, "2024-01", 0.30)
	add("standard_calibration", other, "glicko2_daneo", 0, "AGGREGATED", 0.40)

	got, err := r.Calibration(ctx, run)
	if err != nil {
		t.Fatalf("Calibration: %v", err)
	}
	want := `[{standard glicko2_daneo 0 100 0.21 0.01 0.6 64.5 {"counts": [1, 2]}} {walk_forward glicko2_std 3 100 0.22 0.01 0.6 64.5 {"counts": [1, 2]}}]`
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

func TestAnalysisRatingsGivesThePageModelsInAFormat(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann")
	addRating(t, pool, run, "glicko2_daneo", 0, "ann", ptr(1), 1800)
	addRating(t, pool, run, "glicko2_daneo_softer", 0, "ann", ptr(2), 1700)
	addRating(t, pool, run, "glicko2_std_retro", 0, "ann", ptr(1), 1900)
	addRating(t, pool, run, "glicko2_daneo", 2, "ann", ptr(1), 1500)

	ratings, err := r.AnalysisRatings(context.Background(), run, 0)
	if err != nil {
		t.Fatalf("AnalysisRatings: %v", err)
	}
	if len(ratings) != 2 || ratings[0].Model != "glicko2_daneo" || ratings[1].Model != "glicko2_daneo_softer" || ratings[0].Opponents != 9 {
		t.Errorf("ratings = %+v, want ann's two page-model ratings in format 0", ratings)
	}
}

func TestActivePlayersPlayedMoreThanTenGamesInTheRunsLastYear(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob", "cy")
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

	active, err := r.ActivePlayers(context.Background(), run)
	if err != nil {
		t.Fatalf("ActivePlayers: %v", err)
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
	setCountries(t, pool, map[string]string{"bob": "PL", "Zed": "PL"})
	for name, c := range map[string]float64{"Annika": 1700, "hannah": 1800, "bob": 1600, "Zed": 1900} {
		addRating(t, pool, run, "glicko2_daneo", 0, name, ptr(1), c)
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
	first, middle, last := DefaultCompared[0], DefaultCompared[3], DefaultCompared[5]
	addPlayers(t, pool, last, middle, first, "someone")
	addRating(t, pool, run, "glicko2_std", 3, middle, ptr(4), 1800)
	addRating(t, pool, run, "glicko2_daneo", 3, middle, nil, 1900)
	addRating(t, pool, run, "glicko2_std_retro", 3, middle, ptr(1), 1950)
	addRating(t, pool, run, "glicko2_daneo", 0, middle, ptr(1), 1850)

	got, err := r.ComparedPlayers(ctx, run, 3, nil)
	if err != nil {
		t.Fatalf("ComparedPlayers: %v", err)
	}
	var names []string
	for _, p := range got {
		names = append(names, p.Name)
	}
	if diff := cmp.Diff([]string{first, middle, last}, names); diff != "" {
		t.Fatalf("names, the default players present in upstream's order (-want +got):\n%s", diff)
	}
	want := []ComparedRating{
		{Model: "glicko2_daneo", Rating: 2000, RD: 50, CRating: 1900, Opponents: 9, WinRate: 58.3},
		{Model: "glicko2_std", Rank: ptr(4), Rating: 1900, RD: 50, CRating: 1800, Opponents: 9, WinRate: 58.3},
	}
	if diff := cmp.Diff(want, got[1].Ratings); diff != "" {
		t.Errorf("%s's ratings, format 3 only and no retro model (-want +got):\n%s", middle, diff)
	}
}

func TestActivityCountsPlayersGamesTitlesAndCountries(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	other := publishedRun(t, pool)
	addPlayers(t, pool, "gm1", "gm2", "m1", "p1", "s1", "fr1", "cz1", "unrated")
	setCountries(t, pool, map[string]string{"gm1": "PL", "gm2": "PL", "m1": "PL", "p1": "PL", "s1": "PL", "fr1": "FR", "cz1": "CZ", "unrated": "DE"})
	for name, rating := range map[string]float64{"gm1": 1900, "gm2": 1951, "m1": 1800, "p1": 1700, "s1": 1500, "fr1": 1600, "cz1": 1550} {
		addRating(t, pool, run, "glicko2_std", 0, name, ptr(1), rating-100)
	}
	addRating(t, pool, run, "glicko2_std", 2, "s1", ptr(1), 2400)
	addRating(t, pool, other, "glicko2_std", 0, "unrated", ptr(1), 1500)
	// Active means a last game after the cutoff, in any model or format.
	_, err := pool.Exec(ctx, `UPDATE player_ratings SET last_played = CASE WHEN player_count = 2 THEN '2025-06-01'::date
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
	withFormatCounts(t, pool, run)
	withFormatCounts(t, pool, other)

	got, err := r.Activity(ctx, run, day("2025-05-31"))
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if got.Active != 1 || got.Inactive != 7 {
		t.Errorf("active, inactive = %d, %d, want 1 (s1, by their 2-player game), 7 (gm1 on the cutoff, the rest before or unrated)", got.Active, got.Inactive)
	}
	if fmt.Sprint(got.Formats) != "[{2 1 1} {3 2 6}]" {
		t.Errorf("formats = %v, want the run's games and their pairs only: [{2 1 1} {3 2 6}]", got.Formats)
	}
	if fmt.Sprint(got.Titles) != "[{GM 2} {S 1} {P 1} {M 1}]" {
		t.Errorf("titles = %v, want this run's, most first, ties in reverse code order as upstream's SQLite", got.Titles)
	}
	want := "[{PL 5 1925.5 1883.667 1837.75 1770.2} {FR 1 <nil> <nil> <nil> 1600} {CZ 1 <nil> <nil> <nil> 1550}]"
	if s := countryText(got.Countries); s != want {
		t.Errorf("countries = %s\nwant %s (Standard, all formats; titles from this run; unrated players left out; ties in reverse code order)", s, want)
	}
}

func TestActivityReadsTheFormatsItsRunRecorded(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addGame(t, pool, run, "2025-01-01", "Cup", Seat{"ann", 1}, Seat{"bob", 0})
	withFormatCounts(t, pool, run)
	addGame(t, pool, run, "2025-01-02", "Cup", Seat{"ann", 1}, Seat{"bob", 0}, Seat{"cat", 0})

	got, err := r.Activity(ctx, run, day("2025-05-31"))
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if fmt.Sprint(got.Formats) != "[{2 1 1}]" {
		t.Errorf("formats = %v, want [{2 1 1}], as recorded before the 3-player game was added", got.Formats)
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

func TestSeatAnalysisGroupsByPlayerCountAndFiltersByTier(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)

	addPlayers(t, pool, "gm1", "gm2", "m1", "m2")
	addRating(t, pool, run, "glicko2_daneo", 0, "gm1", ptr(1), 1700)
	addRating(t, pool, run, "glicko2_daneo", 0, "gm2", ptr(2), 1700)
	addRating(t, pool, run, "glicko2_daneo", 0, "m1", ptr(3), 1550)
	addRating(t, pool, run, "glicko2_daneo", 0, "m2", ptr(4), 1550)

	addGame(t, pool, run, "2025-01-01", "Cup 1", Seat{"gm1", 100}, Seat{"gm2", 90})
	addGame(t, pool, run, "2025-01-02", "Cup 2", Seat{"m1", 90}, Seat{"m2", 100})
	addGame(t, pool, run, "2025-01-03", "Cup 3", Seat{"m1", 95}, Seat{"m2", 95})
	withSeatTiers(t, pool, run)

	gotAll, err := r.SeatAnalysis(ctx, run, 2, "all")
	if err != nil {
		t.Fatalf("SeatAnalysis(all): %v", err)
	}
	if len(gotAll) != 1 || gotAll[0].TotalGames != 3 {
		t.Fatalf("SeatAnalysis(all) = %+v, want 1 group with 3 total games", gotAll)
	}
	if gotAll[0].Seats[0].Wins != 1 || gotAll[0].Seats[0].Draws != 1 || gotAll[0].Seats[0].WinRate != 50.0 {
		t.Errorf("Seat 1 = %+v, want Wins 1, Draws 1, WinRate 50.0", gotAll[0].Seats[0])
	}
	if gotAll[0].Seats[1].Wins != 1 || gotAll[0].Seats[1].Draws != 1 || gotAll[0].Seats[1].WinRate != 50.0 {
		t.Errorf("Seat 2 = %+v, want Wins 1, Draws 1, WinRate 50.0", gotAll[0].Seats[1])
	}

	gotGM, err := r.SeatAnalysis(ctx, run, 2, "grandmaster")
	if err != nil {
		t.Fatalf("SeatAnalysis(grandmaster): %v", err)
	}
	if len(gotGM) != 1 || gotGM[0].TotalGames != 1 {
		t.Fatalf("SeatAnalysis(grandmaster) = %+v, want 1 game", gotGM)
	}
	if gotGM[0].Seats[0].Wins != 1 || gotGM[0].Seats[0].Draws != 0 || gotGM[0].Seats[0].WinRate != 100.0 {
		t.Errorf("GM Seat 1 win rate = %v, want 100.0", gotGM[0].Seats[0].WinRate)
	}

	gotM, err := r.SeatAnalysis(ctx, run, 2, "master")
	if err != nil {
		t.Fatalf("SeatAnalysis(master): %v", err)
	}
	if len(gotM) != 1 || gotM[0].TotalGames != 2 {
		t.Fatalf("SeatAnalysis(master) = %+v, want 2 games", gotM)
	}
	if gotM[0].Seats[0].Wins != 0 || gotM[0].Seats[0].Draws != 1 || gotM[0].Seats[0].WinRate != 25.0 {
		t.Errorf("Master Seat 1 = %+v, want Wins 0, Draws 1, WinRate 25.0", gotM[0].Seats[0])
	}
	if gotM[0].Seats[1].Wins != 1 || gotM[0].Seats[1].Draws != 1 || gotM[0].Seats[1].WinRate != 75.0 {
		t.Errorf("Master Seat 2 = %+v, want Wins 1, Draws 1, WinRate 75.0", gotM[0].Seats[1])
	}
}

func TestSeatAnalysisFractionalWinSharesForMultiwayTies(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)

	addPlayers(t, pool, "p1", "p2", "p3", "p4")
	addRating(t, pool, run, "glicko2_daneo", 0, "p1", ptr(1), 1600)
	addRating(t, pool, run, "glicko2_daneo", 0, "p2", ptr(2), 1600)
	addRating(t, pool, run, "glicko2_daneo", 0, "p3", ptr(3), 1600)
	addRating(t, pool, run, "glicko2_daneo", 0, "p4", ptr(4), 1600)

	// 3-player game: 3-way tie for 1st place -> each gets 1/3 win share (33.33% win rate)
	addGame(t, pool, run, "2025-01-01", "Cup 3P", Seat{"p1", 100}, Seat{"p2", 100}, Seat{"p3", 100})

	// 4-player game: 4-way tie for 1st place -> each gets 1/4 win share (25.0% win rate)
	addGame(t, pool, run, "2025-01-02", "Cup 4P", Seat{"p1", 100}, Seat{"p2", 100}, Seat{"p3", 100}, Seat{"p4", 100})
	withSeatTiers(t, pool, run)

	got3P, err := r.SeatAnalysis(ctx, run, 3, "all")
	if err != nil {
		t.Fatalf("SeatAnalysis(3P): %v", err)
	}
	if len(got3P) != 1 || got3P[0].TotalGames != 1 {
		t.Fatalf("got3P = %+v, want 1 group with 1 game", got3P)
	}
	for i, s := range got3P[0].Seats {
		if s.Wins != 0 || s.Draws != 1 || s.WinRate != 33.33 {
			t.Errorf("3P Seat %d = %+v, want Wins 0, Draws 1, WinRate 33.33", i+1, s)
		}
	}

	got4P, err := r.SeatAnalysis(ctx, run, 4, "all")
	if err != nil {
		t.Fatalf("SeatAnalysis(4P): %v", err)
	}
	if len(got4P) != 1 || got4P[0].TotalGames != 1 {
		t.Fatalf("got4P = %+v, want 1 group with 1 game", got4P)
	}
	for i, s := range got4P[0].Seats {
		if s.Wins != 0 || s.Draws != 1 || s.WinRate != 25.0 {
			t.Errorf("4P Seat %d = %+v, want Wins 0, Draws 1, WinRate 25.0", i+1, s)
		}
	}
}

func TestSeatAnalysisUsesRatingWhenPlayedToDetermineTier(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)

	addPlayers(t, pool, "rising1", "rising2")
	// End-of-run ratings: 1800 (GM tier)
	addRating(t, pool, run, "glicko2_daneo", 0, "rising1", ptr(1), 1800)
	addRating(t, pool, run, "rising2", 0, "rising2", ptr(2), 1800)

	// Historical rating as of 2024-01-01: 1400 (Intermediate tier)
	addHistory(t, pool, run, "glicko2_daneo", 0, "rising1", "2024-01-01", 1400)
	addHistory(t, pool, run, "glicko2_daneo", 0, "rising2", "2024-01-01", 1400)

	// Game played on 2024-01-15 (between the 1400 snapshot and later evolution)
	addGame(t, pool, run, "2024-01-15", "Early Duel", Seat{"rising1", 100}, Seat{"rising2", 90})

	// Historical rating evolves to 1800 by 2025-06-01
	addHistory(t, pool, run, "glicko2_daneo", 0, "rising1", "2025-06-01", 1800)
	addHistory(t, pool, run, "glicko2_daneo", 0, "rising2", "2025-06-01", 1800)

	// Game played on 2025-06-15 (at GM level)
	addGame(t, pool, run, "2025-06-15", "Late Duel", Seat{"rising1", 100}, Seat{"rising2", 90})
	withSeatTiers(t, pool, run)

	// Check grandmaster tier: should ONLY contain the 2025 game (1 game), NOT the 2024 game
	gmGroups, err := r.SeatAnalysis(ctx, run, 2, "grandmaster")
	if err != nil {
		t.Fatalf("SeatAnalysis(grandmaster): %v", err)
	}
	if len(gmGroups) != 1 || gmGroups[0].TotalGames != 1 {
		t.Fatalf("gmGroups = %+v, want exactly 1 game played when players were GM tier", gmGroups)
	}

	// Check intermediate tier: should contain the 2024 game (1 game)
	interGroups, err := r.SeatAnalysis(ctx, run, 2, "intermediate")
	if err != nil {
		t.Fatalf("SeatAnalysis(intermediate): %v", err)
	}
	if len(interGroups) != 1 || interGroups[0].TotalGames != 1 {
		t.Fatalf("interGroups = %+v, want exactly 1 game played when players were Intermediate tier", interGroups)
	}
}

func TestSeatAnalysisReadsTheCountsItsRunRecorded(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "gm1", "gm2")
	addRating(t, pool, run, "glicko2_daneo", 0, "gm1", ptr(1), 1700)
	addRating(t, pool, run, "glicko2_daneo", 0, "gm2", ptr(2), 1700)
	game := addGame(t, pool, run, "2025-01-01", "Cup 1", Seat{"gm1", 100}, Seat{"gm2", 90})
	withSeatTiers(t, pool, run)
	if _, err := pool.Exec(ctx, "UPDATE seats SET place = 3 - place WHERE game_id = $1", game); err != nil {
		t.Fatalf("swapping the places: %v", err)
	}

	got, err := r.SeatAnalysis(ctx, run, 0, "grandmaster")
	if err != nil {
		t.Fatalf("SeatAnalysis(0, grandmaster): %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("SeatAnalysis(0, grandmaster) has %d groups, want 3", len(got))
	}
	if w := got[0].Seats[0].Wins; w != 1 {
		t.Errorf("seat 1's wins = %d, want 1, as recorded before the places were swapped", w)
	}
}

func TestSeatAnalysisRefusesARunUnpublishedSinceItWasComputed(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)

	if _, err := r.SeatAnalysis(ctx, run, 0, "all"); err != nil {
		t.Fatalf("SeatAnalysis: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE rating_runs SET published_at = NULL WHERE run_id = $1", run); err != nil {
		t.Fatalf("unpublishing run %d: %v", run, err)
	}
	if _, err := r.SeatAnalysis(ctx, run, 0, "all"); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("SeatAnalysis after unpublishing = %v, want ErrRunNotFound", err)
	}
}

func TestComparedPlayersLeavesOutHiddenPlayers(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	first, second := DefaultCompared[0], DefaultCompared[1]
	addPlayers(t, pool, "anna", "hanna", first, second)
	hideAccount(t, pool, "anna", "deleted")
	hideAccount(t, pool, first, "deleted")

	tests := []struct {
		name  string
		terms []string
		want  []string
	}{
		{"a search", []string{"ann"}, []string{"hanna"}},
		{"the default players", nil, []string{second}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.ComparedPlayers(ctx, run, 0, tt.terms)
			if err != nil {
				t.Fatalf("ComparedPlayers(%q): %v", tt.terms, err)
			}
			var names []string
			for _, p := range got {
				names = append(names, p.Name)
			}
			if !slices.Equal(names, tt.want) {
				t.Errorf("ComparedPlayers(%q) = %q, want %q: anna and %s are hidden", tt.terms, names, tt.want, first)
			}
		})
	}
}

func TestPlayerNamesLikeFindsNamesAloneStrongestFirst(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "Annika", "hannah", "bob", "Zed")
	setCountries(t, pool, map[string]string{"Zed": "AN"})
	for name, c := range map[string]float64{"Annika": 1700, "hannah": 1800, "bob": 1600, "Zed": 1900} {
		addRating(t, pool, run, "glicko2_daneo", 0, name, ptr(1), c)
	}
	tests := []struct {
		term  string
		limit int
		want  string
	}{
		{"AN", 5, "[hannah Annika]"},
		{"AN", 1, "[hannah]"},
		{"b", 5, "[bob]"},
		{"a_n", 5, "[]"},
	}
	for _, tt := range tests {
		got, err := r.PlayerNamesLike(ctx, run, tt.term, tt.limit)
		if err != nil || fmt.Sprint(got) != tt.want {
			t.Errorf("PlayerNamesLike(%q, %d) = %v, %v; want %s: names containing it, by GlickoD C rating, no country matches", tt.term, tt.limit, got, err, tt.want)
		}
	}
}
