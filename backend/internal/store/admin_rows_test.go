//go:build integration

package store

import (
	"context"
	"testing"
	"time"
)

func TestTournamentRowsListEachSeriesOnceWhenTwoShareAName(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"International S30 - Grandmaster game 1": day("2024-09-15"),
		"International S20 - Grandmaster game 1": day("2024-10-15"), "Intermezzo S24 - Bronze 1 game 1": day("2024-09-16")})
	if _, err := pool.Exec(ctx, "UPDATE series SET name = 'Same' WHERE slug IN ('international', 'intermezzo')"); err != nil {
		t.Fatal(err)
	}
	got, err := NewAdminStore(asAdminLogin(t, pool)).TournamentRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, s := range got {
		seen[s.Slug]++
	}
	if seen["international"] != 1 || seen["intermezzo"] != 1 {
		t.Errorf("entries per slug = %v, want one each", seen)
	}
}

func TestTournamentRowsListEachSeriesInBrief(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"International S30 - Grandmaster game 1": day("2024-09-15"), "International S31 - Grandmaster game 1": day("2024-10-20"),
	})
	if _, err := pool.Exec(ctx, `INSERT INTO cge_tournaments (cge_tournament, name, status, series_id, description)
		SELECT 21, 'International', 'running', series_id, repeat('word ', 60) FROM series WHERE slug = 'international'`); err != nil {
		t.Fatal(err)
	}
	a := NewAdminStore(asAdminLogin(t, pool))
	g, err := a.CreateGroup(ctx, Group{Name: "Leagues", Kind: "league", Members: []string{"international"}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := a.TournamentRows(ctx)
	if err != nil {
		t.Fatalf("TournamentRows: %v", err)
	}
	by := map[string]AdminTournamentRow{}
	for _, r := range rows {
		by[r.Slug] = r
	}
	intl, group := by["international"], by[g]
	if intl.Games != 2 || intl.FirstGame == nil || !intl.FirstGame.Equal(day("2024-09-15")) || intl.Parent != g || !intl.Rated || intl.Kind != "league" {
		t.Errorf("international = %+v, want 2 games from 2024-09-15, in %s, rated, a league", intl, g)
	}
	if n := len([]rune(intl.CGEAbout)); n != 200 {
		t.Errorf("CGE's description = %d characters, want the first 200", n)
	}
	if !group.IsGroup || group.Name != "Leagues" || group.Games != 0 {
		t.Errorf("group = %+v, want an empty group called Leagues", group)
	}
}
