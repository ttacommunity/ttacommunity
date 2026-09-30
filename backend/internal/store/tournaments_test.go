//go:build integration

package store

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"ttacommunity/backend/internal/divisioncsv"
)

func division(season int, name, player string) divisioncsv.Division {
	return divisioncsv.Division{Series: "international", Season: season, Division: name, Player: player, Source: "sheet"}
}

func recordedDivisions(t *testing.T, pool interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}) []divisioncsv.Division {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		"SELECT series, season, division, player_name, source FROM tournament_group_divisions ORDER BY season, player_name")
	if err != nil {
		t.Fatalf("reading divisions: %v", err)
	}
	got, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (divisioncsv.Division, error) {
		var d divisioncsv.Division
		err := r.Scan(&d.Series, &d.Season, &d.Division, &d.Player, &d.Source)
		return d, err
	})
	if err != nil {
		t.Fatalf("reading divisions: %v", err)
	}
	return got
}

func TestReplaceDivisionsWritesKnownPlayersOnly(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "ann", "bob")

	got, err := ReplaceDivisions(ctx, pool, []divisioncsv.Division{
		division(20, "Grandmaster", "ann"), division(20, "Grandmaster", "ghost"), division(21, "Master", "bob"),
	})
	if err != nil {
		t.Fatalf("ReplaceDivisions: %v", err)
	}
	if want := (DivisionsReplaced{Written: 2, Unknown: 1}); got != want {
		t.Errorf("ReplaceDivisions = %+v, want %+v", got, want)
	}
	want := []divisioncsv.Division{division(20, "Grandmaster", "ann"), division(21, "Master", "bob")}
	if rows := recordedDivisions(t, pool); !slices.Equal(rows, want) {
		t.Errorf("divisions = %v, want %v", rows, want)
	}
}

func TestReplaceDivisionsReplacesEveryEarlierRow(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "ann", "bob")
	if _, err := ReplaceDivisions(ctx, pool, []divisioncsv.Division{division(20, "Grandmaster", "ann")}); err != nil {
		t.Fatalf("first ReplaceDivisions: %v", err)
	}

	if _, err := ReplaceDivisions(ctx, pool, []divisioncsv.Division{division(22, "Gold", "bob")}); err != nil {
		t.Fatalf("second ReplaceDivisions: %v", err)
	}
	if rows, want := recordedDivisions(t, pool), []divisioncsv.Division{division(22, "Gold", "bob")}; !slices.Equal(rows, want) {
		t.Errorf("divisions after a second load = %v, want %v", rows, want)
	}
}

func TestReplaceDivisionsIsAllOrNothing(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "ann", "bob")
	if _, err := ReplaceDivisions(ctx, pool, []divisioncsv.Division{division(20, "Grandmaster", "ann")}); err != nil {
		t.Fatalf("first ReplaceDivisions: %v", err)
	}

	bad := []divisioncsv.Division{division(21, "Gold", "bob"), division(0, "Gold", "ann")}
	if _, err := ReplaceDivisions(ctx, pool, bad); err == nil {
		t.Fatal("ReplaceDivisions with season 0: no error, want one")
	}
	if rows, want := recordedDivisions(t, pool), []divisioncsv.Division{division(20, "Grandmaster", "ann")}; !slices.Equal(rows, want) {
		t.Errorf("divisions after a refused load = %v, want the first load's %v", rows, want)
	}
}
