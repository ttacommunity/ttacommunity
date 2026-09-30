//go:build integration

package store

import (
	"context"
	"slices"
	"testing"

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
