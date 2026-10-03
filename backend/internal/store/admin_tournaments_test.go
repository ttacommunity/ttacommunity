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
	want := []AdminSeason{{Label: "2025", Stages: []AdminStage{{Label: "Stage 1"}, {Label: "Stage 2", Note: "The upper bracket."}}}}
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
