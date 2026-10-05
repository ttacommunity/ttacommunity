//go:build integration

package store

import (
	"context"
	"slices"
	"testing"

	"ttacommunity/backend/internal/tournamentcsv"
)

func TestReplaceCommunityPointsWritesKnownTournamentsOnly(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "INSERT INTO tournaments (slug, name, kind) VALUES ('international', 'International Championship', 'league')"); err != nil {
		t.Fatal(err)
	}
	cell := func(tournament string, edition int, group string, place, points int) tournamentcsv.CommunityPoints {
		return tournamentcsv.CommunityPoints{Tournament: tournament, FromEdition: edition, Group: group, Place: place, Points: points, Source: "sheet"}
	}
	if _, err := ReplaceCommunityPoints(ctx, pool, []tournamentcsv.CommunityPoints{cell("international", 2025, "Wood", 9, 1)}); err != nil {
		t.Fatalf("first ReplaceCommunityPoints: %v", err)
	}
	got, err := ReplaceCommunityPoints(ctx, pool, []tournamentcsv.CommunityPoints{
		cell("international", 2025, "Grandmaster", 1, 1000),
		cell("international", 2024, "Wood", 2, 15),
		cell("snail_cup", 2025, "Final", 1, 1000),
	})
	if err != nil {
		t.Fatalf("ReplaceCommunityPoints: %v", err)
	}
	if want := (FactsReplaced{Written: 2, Unknown: 1}); got != want {
		t.Errorf("ReplaceCommunityPoints = %+v, want %+v", got, want)
	}
	const rows = "SELECT concat_ws('|', tournament, from_edition, group_name, place, points, source) FROM community_points ORDER BY from_edition"
	if got, want := rowsOf(t, pool, rows), []string{"international|2024|Wood|2|15|sheet", "international|2025|Grandmaster|1|1000|sheet"}; !slices.Equal(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
}
