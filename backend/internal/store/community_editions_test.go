//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCommunityEditionsCRUD(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	r := NewRatingsReader(asReader(t, pool))

	// List initially
	initial, err := a.AdminCommunityEditions(ctx)
	if err != nil {
		t.Fatalf("AdminCommunityEditions error: %v", err)
	}

	// Create a new edition
	ed := CommunityEdition{
		Slug:           "race26_test",
		Name:           "Race to 2026 Test",
		Title:          "Official Race 2026 Title",
		ShortName:      "Race26T",
		Badge:          "⭐ Community",
		Maintainer:     "dee",
		MaintainerNote: "Integration test note",
		Year:           2026,
		CountingEvents: 8,
		Visible:        true,
		SortOrder:      100,
	}
	if err := a.CreateCommunityEdition(ctx, ed); err != nil {
		t.Fatalf("CreateCommunityEdition error: %v", err)
	}

	// Read visible editions from public reader
	pubList, err := r.CommunityEditions(ctx)
	if err != nil {
		t.Fatalf("CommunityEditions error: %v", err)
	}
	var found bool
	for _, e := range pubList {
		if e.Slug == "race26_test" {
			found = true
			if e.Name != ed.Name || e.CountingEvents != 8 {
				t.Errorf("got edition %+v, want %+v", e, ed)
			}
		}
	}
	if !found {
		t.Errorf("new edition not found in public list: %+v", pubList)
	}

	// Update edition: make invisible and change name
	newName := "Race 2026 Renamed"
	invisible := false
	if err := a.SetCommunityEdition(ctx, "race26_test", CommunityEditionChange{
		Name:    &newName,
		Visible: &invisible,
	}); err != nil {
		t.Fatalf("SetCommunityEdition error: %v", err)
	}

	// Verify not in public visible list anymore
	pubListAfter, err := r.CommunityEditions(ctx)
	if err != nil {
		t.Fatalf("CommunityEditions after error: %v", err)
	}
	for _, e := range pubListAfter {
		if e.Slug == "race26_test" {
			t.Errorf("expected race26_test to be hidden, but was visible: %+v", e)
		}
	}

	// Still visible in admin list
	adminList, err := a.AdminCommunityEditions(ctx)
	if err != nil {
		t.Fatalf("AdminCommunityEditions error: %v", err)
	}
	if len(adminList) != len(initial)+1 {
		t.Errorf("admin list count got %d, want %d", len(adminList), len(initial)+1)
	}
}

// addEdition creates a shown community edition for year with the scores given as (player, event, score).
func addEdition(t *testing.T, pool *pgxpool.Pool, slug string, year int, scores ...any) {
	t.Helper()
	ctx := context.Background()
	if err := NewAdminStore(pool).CreateCommunityEdition(ctx, CommunityEdition{Slug: slug, Name: slug, Year: year, Visible: true}); err != nil {
		t.Fatalf("CreateCommunityEdition(%s): %v", slug, err)
	}
	for i := 0; i+2 < len(scores); i += 3 {
		if _, err := pool.Exec(ctx, "INSERT INTO community_scores VALUES ($1, $2, $3, $4, 'sheet')", slug, scores[i], scores[i+1], scores[i+2]); err != nil {
			t.Fatalf("adding a score to %s: %v", slug, err)
		}
	}
}

func TestCommunityLeaderboardLayoutRefusesAnUnknownOrHiddenEdition(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	addEdition(t, pool, "hidden", 2025, "ann", "wrld", 100.0)
	shown := false
	if err := NewAdminStore(pool).SetCommunityEdition(ctx, "hidden", CommunityEditionChange{Visible: &shown}); err != nil {
		t.Fatalf("SetCommunityEdition: %v", err)
	}
	r := NewRatingsReader(asReader(t, pool))
	for _, slug := range []string{"unknown", "hidden"} {
		t.Run(slug, func(t *testing.T) {
			if _, err := r.CommunityLeaderboardLayout(ctx, run, slug); !errors.Is(err, ErrNoCommunityEdition) {
				t.Errorf("CommunityLeaderboardLayout(%s) = %v, want ErrNoCommunityEdition", slug, err)
			}
		})
	}
}

func TestCommunityLeaderboardLayoutHasAColumnForEachEventItsRowsScored(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	addEdition(t, pool, "e25", 2025, "ann", "wrld", 2000.0, "bob", "ic_30", 500.0)
	if _, err := pool.Exec(ctx, `INSERT INTO community_edition_points VALUES
		($1, 'e25', 'ann', 'im_20', 300, 'computed', true), ($1, '2025', 'ann', 'rl_05', 300, 'computed', true)`, run); err != nil {
		t.Fatalf("adding computed points: %v", err)
	}

	got, err := NewRatingsReader(asReader(t, pool)).CommunityLeaderboardLayout(ctx, run, "e25")
	if err != nil {
		t.Fatalf("CommunityLeaderboardLayout: %v", err)
	}
	var ids []string
	for _, c := range got.Categories {
		for _, col := range c.Columns {
			ids = append(ids, col.ID)
		}
	}
	if !slices.Equal(ids, []string{"wrld", "ic_30"}) {
		t.Errorf("columns = %q, want wrld and ic_30: the events its rows have points in", ids)
	}
}

func TestCommunityLeaderboardLayoutTakesEachMaximumFromItsPointsTable(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	for _, q := range []string{
		"INSERT INTO series (slug, name, community_code) VALUES ('worlds', 'Worlds', 'wrld'), ('international', 'International', 'ic_{}')",
		`INSERT INTO community_points (tournament, from_edition, group_name, place, points, source) VALUES
			('worlds', 2024, 'Final', 1, 2500, 's'), ('worlds', 2024, 'Final', 2, 1800, 's'), ('worlds', 2026, 'Final', 1, 3000, 's'),
			('international', 2026, 'Grandmaster', 1, 1000, 's')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("adding points tables: %v", err)
		}
	}
	addEdition(t, pool, "e25", 2025, "ann", "wrld", 470.0, "ann", "ic_30", 90.0, "ann", "qd", 600.0)

	got, err := NewRatingsReader(asReader(t, pool)).CommunityLeaderboardLayout(ctx, run, "e25")
	if err != nil {
		t.Fatalf("CommunityLeaderboardLayout: %v", err)
	}
	maxima := map[string]string{}
	for _, c := range got.Categories {
		for _, col := range c.Columns {
			maxima[col.ID] = fmt.Sprint(orNone(col.MaxPts))
		}
	}
	for _, s := range got.Summary {
		maxima["summary "+s.ID] = fmt.Sprint(orNone(s.MaxPts))
	}
	want := map[string]string{
		"wrld": "2500", "ic_30": "1000", "qd": "none",
		"summary wrld": "2500", "summary ic": "1000", "summary qd": "none",
	}
	if diff := cmp.Diff(want, maxima); diff != "" {
		t.Errorf("maxima: the table in force in 2025, else the earliest, and none without a table (-want +got):\n%s", diff)
	}
}

func TestCommunityLeaderboardLayoutNamesTheChampionReigningInItsYear(t *testing.T) {
	tests := []struct {
		name, champion, desc string
		records              []string
	}{
		{"no champion", "", "", nil},
		{"the latest gold up to its year", "bob", "Reigning World Champion (2024)",
			[]string{"ann|2023", "bob|2024", "cat|2026"}},
		{"a hidden champion", DeletedPlayer, "Reigning World Champion (2024)", []string{"dan|2024"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			ctx := context.Background()
			run := publishedRun(t, pool)
			addPlayers(t, pool, "dan")
			hideAccount(t, pool, "dan", "banned")
			for i, rec := range tt.records {
				name, season, _ := strings.Cut(rec, "|")
				_, err := pool.Exec(ctx, `INSERT INTO tournament_records VALUES ($1, $2, $3, 'World Championship', $4, 'Final',
					'1 / 4', '', 'gold', '', '', false)`, run, name, i+1, season)
				if err != nil {
					t.Fatalf("adding %s's record: %v", rec, err)
				}
			}
			addEdition(t, pool, "e25", 2025, "ann", "wrld", 100.0)

			got, err := NewRatingsReader(asReader(t, pool)).CommunityLeaderboardLayout(ctx, run, "e25")
			if err != nil {
				t.Fatalf("CommunityLeaderboardLayout: %v", err)
			}
			if got.Info.WC != tt.champion || got.Info.WCDesc != tt.desc {
				t.Errorf("champion = %q, %q; want %q, %q", got.Info.WC, got.Info.WCDesc, tt.champion, tt.desc)
			}
		})
	}
}

// orNone returns what p points to, or "none" for nil.
func orNone(p *int) any {
	if p == nil {
		return "none"
	}
	return *p
}

func TestCommunityLeaderboardLayoutFallsBackToTheRecordedPodiumOfItsYear(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	if _, err := pool.Exec(ctx, `INSERT INTO tournament_cup_podiums (cup, season, place, player_name, source, finished) VALUES
		('worlds', '2023', 1, 'ann', 's', '2023-12-01'), ('worlds', '2024', 1, 'bob', 's', '2024-12-01'),
		('worlds', '2026', 1, 'cat', 's', '2026-12-01')`); err != nil {
		t.Fatalf("adding podiums: %v", err)
	}
	addEdition(t, pool, "e25", 2025, "ann", "wrld", 100.0)

	got, err := NewRatingsReader(asReader(t, pool)).CommunityLeaderboardLayout(ctx, run, "e25")
	if err != nil {
		t.Fatalf("CommunityLeaderboardLayout: %v", err)
	}
	if got.Info.WC != "bob" || got.Info.WCDesc != "World Champion (2024)" {
		t.Errorf("champion = %q, %q; want bob, World Champion (2024): the latest podium up to 2025", got.Info.WC, got.Info.WCDesc)
	}
}

func TestCommunityEditionSlugsListsEveryEdition(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	hidden := CommunityEdition{Slug: "2031", Name: "Invented 2031", Title: "Invented 2031", ShortName: "2031", Year: 2031, CountingEvents: 5}
	if err := a.CreateCommunityEdition(ctx, hidden); err != nil {
		t.Fatalf("CreateCommunityEdition: %v", err)
	}
	got, err := CommunityEditionSlugs(ctx, pool)
	if err != nil {
		t.Fatalf("CommunityEditionSlugs: %v", err)
	}
	for _, want := range []string{"2024", "race26", "2031"} {
		if !slices.Contains(got, want) {
			t.Errorf("CommunityEditionSlugs() = %q, want it to hold %q", got, want)
		}
	}
}
