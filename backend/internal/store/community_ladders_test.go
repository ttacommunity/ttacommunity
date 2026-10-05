//go:build integration

package store

import (
	"context"
	"slices"
	"testing"

	"ttacommunity/backend/internal/tournamentcsv"
)

func TestReplaceCommunityLaddersWritesKnownTournamentsOnly(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "INSERT INTO tournaments (slug, name, kind) VALUES ('sodium_ladder', 'Sodium Ladder', 'ladder')"); err != nil {
		t.Fatal(err)
	}
	got, err := ReplaceCommunityLadders(ctx, pool, []tournamentcsv.CommunityLadder{
		{Tournament: "sodium_ladder", FromEdition: 2025, Ratio: 0.8, HalfStepsFrom: 4, Shares: []float64{1, 0.5, 0.166, 0}, Rounds: 8, Source: "tab"},
		{Tournament: "mercurial_ladder", FromEdition: 2024, Ratio: 0.8, Shares: []float64{1, 0.4, 0}, Rounds: 10, Source: "tab"},
	})
	if err != nil {
		t.Fatalf("ReplaceCommunityLadders: %v", err)
	}
	if want := (FactsReplaced{Written: 1, Unknown: 1}); got != want {
		t.Errorf("ReplaceCommunityLadders = %+v, want %+v", got, want)
	}
	const rows = "SELECT concat_ws('|', tournament, from_edition, ratio, coalesce(half_steps_from::text, '-'), shares, rounds, source) FROM community_ladders"
	if got, want := rowsOf(t, pool, rows), []string{"sodium_ladder|2025|0.8|4|{1,0.5,0.166,0}|8|tab"}; !slices.Equal(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
}
