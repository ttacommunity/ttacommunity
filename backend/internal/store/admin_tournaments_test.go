//go:build integration

package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func adminTournament(t *testing.T, ts []AdminTournament, slug string) AdminTournament {
	t.Helper()
	for _, x := range ts {
		if x.Slug == slug {
			return x
		}
	}
	t.Fatalf("no tournament %s in %+v", slug, ts)
	return AdminTournament{}
}

func TestAdminTournamentsListSettingsSeasonsAndNotes(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"Worlds 2025 Stage 1 - Upper 1 game 1": day("2025-03-01"),
		"Worlds 2025 Stage 2 - Upper 1 game 1": day("2025-04-01"),
		"NL_s10":                               day("2024-09-15"),
	})
	a := NewAdminStore(asAdminLogin(t, pool))
	if err := a.SetNote(ctx, "worlds", "2025", "Stage 2", "The upper bracket."); err != nil {
		t.Fatalf("SetNote: %v", err)
	}

	got, err := a.Tournaments(ctx)
	if err != nil {
		t.Fatalf("Tournaments: %v", err)
	}
	worlds := adminTournament(t, got, "worlds")
	if !worlds.Rated || worlds.Visible || worlds.Community {
		t.Errorf("worlds settings = %+v, want rated, hidden, and not yet in the community leaderboard (no migration row)", worlds)
	}
	want := []AdminSeason{{Label: "2025", HasGames: true, Stages: []AdminStage{{Label: "Stage 1"}, {Label: "Stage 2", Note: "The upper bracket."}}}}
	if !reflect.DeepEqual(worlds.Seasons, want) {
		t.Errorf("worlds seasons = %+v, want %+v", worlds.Seasons, want)
	}
}

func TestSetTournamentChangesOnlyWhatItIsGiven(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"NL_s10": day("2024-09-15")})
	a := NewAdminStore(asAdminLogin(t, pool))
	yes, no := true, false

	if err := a.SetTournament(ctx, "nations_league", TournamentSettings{Visible: &yes, Community: &yes}); err != nil {
		t.Fatalf("SetTournament: %v", err)
	}
	if err := a.SetTournament(ctx, "nations_league", TournamentSettings{Rated: &no}); err != nil {
		t.Fatalf("SetTournament(unrate): %v", err)
	}

	got, err := a.Tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	nl := adminTournament(t, got, "nations_league")
	if !nl.Visible || !nl.Community || nl.Rated {
		t.Errorf("nations_league = %+v, want visible, in the community leaderboard, not rated", nl)
	}
	if err := a.SetTournament(ctx, "no_such", TournamentSettings{Visible: &yes}); !errors.Is(err, ErrNoTournament) {
		t.Errorf("SetTournament(no_such) = %v, want ErrNoTournament", err)
	}
}

func TestSetNoteReplacesAndAnEmptyNoteRemoves(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"NL_s10": day("2024-09-15")})
	a := NewAdminStore(asAdminLogin(t, pool))
	note := func() string {
		var n string
		_ = pool.QueryRow(ctx, "SELECT coalesce(max(note), '') FROM tournament_season_notes WHERE tournament = 'nations_league'").Scan(&n)
		return n
	}

	for _, n := range []string{"First.", "Second."} {
		if err := a.SetNote(ctx, "nations_league", "10", "", n); err != nil {
			t.Fatalf("SetNote(%s): %v", n, err)
		}
	}
	if got := note(); got != "Second." {
		t.Errorf("note = %q, want Second.", got)
	}
	if err := a.SetNote(ctx, "nations_league", "10", "", ""); err != nil {
		t.Fatalf("SetNote(empty): %v", err)
	}
	if got := note(); got != "" {
		t.Errorf("note = %q after an empty one, want none", got)
	}
	if err := a.SetNote(ctx, "nations_league", "99", "", "x"); !errors.Is(err, ErrNoTournament) {
		t.Errorf("SetNote on a season that does not exist = %v, want ErrNoTournament", err)
	}
}

func TestAddSeasonAddsAnEmptySeasonAndRemoveSeasonOnlyAnEmptyOne(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"International S2 g1": day("2017-10-06")})
	a := NewAdminStore(asAdminLogin(t, pool))

	if err := a.AddSeason(ctx, "international", "1"); err != nil {
		t.Fatalf("AddSeason: %v", err)
	}
	if err := a.AddSeason(ctx, "international", "1"); !errors.Is(err, ErrSeasonExists) {
		t.Errorf("AddSeason twice = %v, want ErrSeasonExists", err)
	}
	if err := a.AddSeason(ctx, "no_such", "1"); !errors.Is(err, ErrNoTournament) {
		t.Errorf("AddSeason(no_such) = %v, want ErrNoTournament", err)
	}
	if err := a.SetNote(ctx, "international", "1", "", "Before the app."); err != nil {
		t.Errorf("a note on the new season: %v", err)
	}
	if err := a.RemoveSeason(ctx, "international", "2"); !errors.Is(err, ErrSeasonHasGames) {
		t.Errorf("RemoveSeason(2, with games) = %v, want ErrSeasonHasGames", err)
	}
	if err := a.RemoveSeason(ctx, "international", "1"); err != nil {
		t.Errorf("RemoveSeason(1): %v", err)
	}
	if err := a.RemoveSeason(ctx, "international", "1"); !errors.Is(err, ErrNoTournament) {
		t.Errorf("RemoveSeason(1) twice = %v, want ErrNoTournament", err)
	}
}

func TestAdminTournamentsListSeasonsLatestFirst(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"International S3 g1":  day("2018-01-06"),
		"International S2 g1":  day("2017-10-06"),
		"International S10 g1": day("2020-01-06"),
	})
	a := NewAdminStore(asAdminLogin(t, pool))
	if err := a.AddSeason(ctx, "international", "1"); err != nil {
		t.Fatal(err)
	}
	got, err := a.Tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, s := range adminTournament(t, got, "international").Seasons {
		labels = append(labels, s.Label)
	}
	if !reflect.DeepEqual(labels, []string{"10", "3", "2", "1"}) {
		t.Errorf("seasons = %v, want 10, 3, 2, 1", labels)
	}
}
