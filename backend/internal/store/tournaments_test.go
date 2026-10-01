//go:build integration

package store

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/tournamentcsv"
)

func division(season int, name, player string) tournamentcsv.Division {
	return tournamentcsv.Division{Series: "international", Season: season, Division: name, Player: player, Source: "sheet"}
}

func rowsOf(t *testing.T, pool *pgxpool.Pool, query string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return got
}

const divisionRows = "SELECT concat_ws('|', series, season, division, player_name, source) FROM tournament_group_divisions ORDER BY season, player_name"

func TestReplaceDivisionsWritesKnownPlayersOnly(t *testing.T) {
	pool := migrated(t)
	addPlayers(t, pool, "ann", "bob")

	got, err := ReplaceDivisions(context.Background(), pool, []tournamentcsv.Division{
		division(20, "Grandmaster", "ann"), division(20, "Grandmaster", "ghost"), division(21, "Master", "bob"),
	})
	if err != nil {
		t.Fatalf("ReplaceDivisions: %v", err)
	}
	if want := (FactsReplaced{Written: 2, Unknown: 1}); got != want {
		t.Errorf("ReplaceDivisions = %+v, want %+v", got, want)
	}
	want := []string{"international|20|Grandmaster|ann|sheet", "international|21|Master|bob|sheet"}
	if rows := rowsOf(t, pool, divisionRows); !slices.Equal(rows, want) {
		t.Errorf("divisions = %q, want %q", rows, want)
	}
}

func TestReplaceDivisionsReplacesEveryEarlierRow(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "ann", "bob")
	if _, err := ReplaceDivisions(ctx, pool, []tournamentcsv.Division{division(20, "Grandmaster", "ann")}); err != nil {
		t.Fatalf("first ReplaceDivisions: %v", err)
	}

	if _, err := ReplaceDivisions(ctx, pool, []tournamentcsv.Division{division(22, "Gold", "bob")}); err != nil {
		t.Fatalf("second ReplaceDivisions: %v", err)
	}
	if rows, want := rowsOf(t, pool, divisionRows), []string{"international|22|Gold|bob|sheet"}; !slices.Equal(rows, want) {
		t.Errorf("divisions after a second load = %q, want %q", rows, want)
	}
}

func TestReplaceDivisionsIsAllOrNothing(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "ann", "bob")
	if _, err := ReplaceDivisions(ctx, pool, []tournamentcsv.Division{division(20, "Grandmaster", "ann")}); err != nil {
		t.Fatalf("first ReplaceDivisions: %v", err)
	}

	bad := []tournamentcsv.Division{division(21, "Gold", "bob"), division(0, "Gold", "ann")}
	if _, err := ReplaceDivisions(ctx, pool, bad); err == nil {
		t.Fatal("ReplaceDivisions with season 0: no error, want one")
	}
	if rows, want := rowsOf(t, pool, divisionRows), []string{"international|20|Grandmaster|ann|sheet"}; !slices.Equal(rows, want) {
		t.Errorf("divisions after a refused load = %q, want the first load's %q", rows, want)
	}
}

const placementRows = "SELECT concat_ws('|', series, season, division, player_name, place, coalesce(points::text, '-'), reason, source) " +
	"FROM tournament_official_placements ORDER BY season, place, player_name"

func TestReplacePlacementsWritesKnownPlayersOnly(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "ann", "bob")
	pts := 25.0
	place := func(season, place int, player string, points *float64) tournamentcsv.Placement {
		return tournamentcsv.Placement{Series: "intermezzo", Season: season, Division: "Grandmaster", Player: player,
			Place: place, Points: points, Reason: "no games", Source: "sheet"}
	}

	got, err := ReplacePlacements(ctx, pool, []tournamentcsv.Placement{place(24, 1, "ann", &pts), place(24, 2, "ghost", nil), place(31, 2, "bob", nil)})
	if err != nil {
		t.Fatalf("ReplacePlacements: %v", err)
	}
	if want := (FactsReplaced{Written: 2, Unknown: 1}); got != want {
		t.Errorf("ReplacePlacements = %+v, want %+v", got, want)
	}
	want := []string{"intermezzo|24|Grandmaster|ann|1|25|no games|sheet", "intermezzo|31|Grandmaster|bob|2|-|no games|sheet"}
	if rows := rowsOf(t, pool, placementRows); !slices.Equal(rows, want) {
		t.Errorf("placements = %q, want %q", rows, want)
	}

	if _, err := ReplacePlacements(ctx, pool, []tournamentcsv.Placement{place(31, 2, "bob", nil), place(0, 1, "ann", nil)}); err == nil {
		t.Fatal("ReplacePlacements with season 0: no error, want one")
	}
	if rows := rowsOf(t, pool, placementRows); !slices.Equal(rows, want) {
		t.Errorf("placements after a refused load = %q, want %q", rows, want)
	}
}

const correctionRows = "SELECT concat_ws('|', series, name_season, coalesce(season::text, '-'), reason, source) " +
	"FROM tournament_season_corrections ORDER BY name_season"

func TestReplaceCorrectionsReplacesEveryEarlierRow(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	seven := 7
	fix := func(named int, season *int) tournamentcsv.Correction {
		return tournamentcsv.Correction{Series: "intermezzo", NameSeason: named, Season: season, Reason: "r", Source: "s"}
	}
	if _, err := ReplaceCorrections(ctx, pool, []tournamentcsv.Correction{fix(9, &seven)}); err != nil {
		t.Fatalf("first ReplaceCorrections: %v", err)
	}

	got, err := ReplaceCorrections(ctx, pool, []tournamentcsv.Correction{fix(8, &seven), fix(7, nil)})
	if err != nil {
		t.Fatalf("ReplaceCorrections: %v", err)
	}
	if want := (FactsReplaced{Written: 2}); got != want {
		t.Errorf("ReplaceCorrections = %+v, want %+v", got, want)
	}
	if rows, want := rowsOf(t, pool, correctionRows), []string{"intermezzo|7|-|r|s", "intermezzo|8|7|r|s"}; !slices.Equal(rows, want) {
		t.Errorf("corrections = %q, want %q", rows, want)
	}
}

const finishDateRows = "SELECT concat_ws('|', series, season, finish_date, source) FROM tournament_finish_dates ORDER BY series, season"

func TestReplaceFinishDatesReplacesEveryEarlierRow(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	date := func(series string, season int, day string) tournamentcsv.FinishDate {
		d, err := time.Parse(time.DateOnly, day)
		if err != nil {
			t.Fatal(err)
		}
		return tournamentcsv.FinishDate{Series: series, Season: season, Date: d, Source: "s"}
	}
	if _, err := ReplaceFinishDates(ctx, pool, []tournamentcsv.FinishDate{date("intermezzo", 1, "2019-03-24")}); err != nil {
		t.Fatalf("first ReplaceFinishDates: %v", err)
	}

	got, err := ReplaceFinishDates(ctx, pool, []tournamentcsv.FinishDate{date("international", 34, "2026-08-14"), date("worlds", 2025, "2025-12-15")})
	if err != nil {
		t.Fatalf("ReplaceFinishDates: %v", err)
	}
	if want := (FactsReplaced{Written: 2}); got != want {
		t.Errorf("ReplaceFinishDates = %+v, want %+v", got, want)
	}
	if rows, want := rowsOf(t, pool, finishDateRows), []string{"international|34|2026-08-14|s", "worlds|2025|2025-12-15|s"}; !slices.Equal(rows, want) {
		t.Errorf("finish dates = %q, want %q", rows, want)
	}
}

const honourRows = "SELECT concat_ws('|', series, player_name, gold, silver, bronze, coalesce(points, '-'), source) " +
	"FROM tournament_career_honours ORDER BY series, player_name"

func TestReplaceHonoursWritesKnownPlayersOnly(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "ann", "bob")
	points := "8,5"
	honours := []tournamentcsv.Honour{
		{Series: "sodium_ladder", Player: "ann", Gold: 5, Silver: 2, Points: &points, Source: "s"},
		{Series: "q_and_d", Player: "Martin Pecheur", Gold: 3, Source: "q"},
		{Series: "q_and_d", Player: "bob", Gold: 2, Silver: 2, Bronze: 1, Source: "q"},
	}

	got, err := ReplaceHonours(ctx, pool, honours)
	if err != nil {
		t.Fatalf("ReplaceHonours: %v", err)
	}
	if want := (FactsReplaced{Written: 2, Unknown: 1}); got != want {
		t.Errorf("ReplaceHonours = %+v, want %+v", got, want)
	}
	if rows, want := rowsOf(t, pool, honourRows), []string{"q_and_d|bob|2|2|1|-|q", "sodium_ladder|ann|5|2|0|8,5|s"}; !slices.Equal(rows, want) {
		t.Errorf("honours = %q, want %q", rows, want)
	}
}

func TestReplaceCommunityKeepsNamesThatAreNotPlayers(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "ann")
	if _, err := ReplaceCommunityStandings(ctx, pool, []tournamentcsv.Standing{{Edition: "2024", SheetRow: 9, Rank: 9, Player: "old", Points: 1, Events: 1, Source: "s"}}); err != nil {
		t.Fatalf("first ReplaceCommunityStandings: %v", err)
	}
	standings := []tournamentcsv.Standing{
		{Edition: "2024", SheetRow: 1, Rank: 1, Player: "ann", Points: 7657, Events: 11, Source: "s"},
		{Edition: "2024", SheetRow: 2, Rank: 2, Player: "Martin Pecheur", Points: 6687, Events: 13, Source: "s"},
	}
	got, err := ReplaceCommunityStandings(ctx, pool, standings)
	if err != nil {
		t.Fatalf("ReplaceCommunityStandings: %v", err)
	}
	if want := (FactsReplaced{Written: 2}); got != want {
		t.Errorf("ReplaceCommunityStandings = %+v, want %+v", got, want)
	}
	const standingRows = "SELECT concat_ws('|', edition, sheet_row, rank, player_name, points, events, source) FROM community_standings ORDER BY rank"
	if rows, want := rowsOf(t, pool, standingRows), []string{"2024|1|1|ann|7657|11|s", "2024|2|2|Martin Pecheur|6687|13|s"}; !slices.Equal(rows, want) {
		t.Errorf("standings = %q, want %q", rows, want)
	}

	scores := []tournamentcsv.Score{
		{Edition: "2024", Player: "ann", Event: "wrld", Score: 1600, Source: "s"},
		{Edition: "2024", Player: "Martin Pecheur", Event: "qd", Score: 450, Source: "s"},
	}
	if got, err := ReplaceCommunityScores(ctx, pool, scores); err != nil || got != (FactsReplaced{Written: 2}) {
		t.Fatalf("ReplaceCommunityScores = %+v, %v; want 2 written", got, err)
	}
	const scoreRows = "SELECT concat_ws('|', edition, player_name, event, score, source) FROM community_scores ORDER BY score"
	if rows, want := rowsOf(t, pool, scoreRows), []string{"2024|Martin Pecheur|qd|450|s", "2024|ann|wrld|1600|s"}; !slices.Equal(rows, want) {
		t.Errorf("scores = %q, want %q", rows, want)
	}
}

const ruleRows = "SELECT concat_ws('|', series, season, timeout_penalty, scoring_3p, scoring_4p, tiebreaks, source) FROM tournament_season_rules ORDER BY series, season"

func TestReplaceRulesReplacesEveryEarlierRow(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	rule := func(season int, penalty float64) tournamentcsv.Rule {
		return tournamentcsv.Rule{Series: "intermezzo", Season: season, TimeoutPenalty: penalty, Scoring3p: "5-2-0", Source: "s"}
	}
	if _, err := ReplaceRules(ctx, pool, []tournamentcsv.Rule{rule(9, 0)}); err != nil {
		t.Fatalf("first ReplaceRules: %v", err)
	}

	got, err := ReplaceRules(ctx, pool, []tournamentcsv.Rule{rule(6, -1), rule(7, 0)})
	if err != nil {
		t.Fatalf("ReplaceRules: %v", err)
	}
	if want := (FactsReplaced{Written: 2}); got != want {
		t.Errorf("ReplaceRules = %+v, want %+v", got, want)
	}
	if rows, want := rowsOf(t, pool, ruleRows), []string{"intermezzo|6|-1|5-2-0|||s", "intermezzo|7|0|5-2-0|||s"}; !slices.Equal(rows, want) {
		t.Errorf("rules = %q, want %q", rows, want)
	}
}
