//go:build integration

package store

import (
	"context"
	"slices"
	"testing"

	"ttacommunity/backend/internal/tournamentcsv"
)

func TestReplaceCommunityQuartersWritesKnownTournamentsOnly(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "INSERT INTO tournaments (slug, name, kind) VALUES ('wimbledon', 'Wimbledon', 'cup')"); err != nil {
		t.Fatal(err)
	}
	quarter := func(tournament, season string, year, q int) tournamentcsv.CommunityQuarter {
		return tournamentcsv.CommunityQuarter{Tournament: tournament, Season: season, Year: year, Quarter: q, Source: "tab"}
	}
	if _, err := ReplaceCommunityQuarters(ctx, pool, []tournamentcsv.CommunityQuarter{quarter("wimbledon", "2020", 2020, 3)}); err != nil {
		t.Fatalf("first ReplaceCommunityQuarters: %v", err)
	}
	got, err := ReplaceCommunityQuarters(ctx, pool, []tournamentcsv.CommunityQuarter{quarter("wimbledon", "2025", 2026, 1), quarter("snail_cup", "2024", 2024, 4)})
	if err != nil {
		t.Fatalf("ReplaceCommunityQuarters: %v", err)
	}
	if want := (FactsReplaced{Written: 1, Unknown: 1}); got != want {
		t.Errorf("ReplaceCommunityQuarters = %+v, want %+v", got, want)
	}
	const rows = "SELECT concat_ws('|', tournament, season, year, quarter, source) FROM community_quarters"
	if got, want := rowsOf(t, pool, rows), []string{"wimbledon|2025|2026|1|tab"}; !slices.Equal(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
}
