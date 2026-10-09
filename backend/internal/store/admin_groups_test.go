//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func groupedSeries(t *testing.T) *AdminStore {
	t.Helper()
	pool := migrated(t)
	placedOn(t, pool, map[string]time.Time{
		"International S30 - Grandmaster game 1": day("2024-09-15"),
		"International S31 - Grandmaster game 1": day("2024-10-20"),
		"Intermezzo S24 - Bronze 1 game 1":       day("2024-09-16"),
		"NL_s10":                                 day("2024-09-17"),
	})
	return NewAdminStore(asAdminLogin(t, pool))
}

func group(t *testing.T, a *AdminStore, name, kind string, members ...string) string {
	t.Helper()
	slug, err := a.CreateGroup(context.Background(), Group{Name: name, Kind: kind, Members: members})
	if err != nil {
		t.Fatalf("CreateGroup(%s): %v", name, err)
	}
	return slug
}

func listed(t *testing.T, a *AdminStore, slug string) AdminTournament {
	t.Helper()
	got, err := a.Tournaments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return adminTournament(t, got, slug)
}

func TestCreateGroupMakesAGroupHoldingItsMembers(t *testing.T) {
	a := groupedSeries(t)
	slug := group(t, a, "The Big Leagues", "league", "international", "intermezzo")

	if slug != "the_big_leagues" {
		t.Errorf("slug = %q, want the_big_leagues", slug)
	}
	if g := listed(t, a, slug); !g.IsGroup || g.Name != "The Big Leagues" || g.Kind != "league" {
		t.Errorf("group = %+v, want a league group called The Big Leagues", g)
	}
	for _, m := range []string{"international", "intermezzo"} {
		if p := listed(t, a, m).Parent; p != slug {
			t.Errorf("%s's parent = %q, want %q", m, p, slug)
		}
	}
	if series := group(t, a, "Quick and Dirty", "series"); listed(t, a, series).Kind != "series" {
		t.Errorf("a series group's kind = %q, want series", listed(t, a, series).Kind)
	}
	if again := group(t, a, "The big leagues!", "group"); again != "the_big_leagues_2" {
		t.Errorf("a second group of that name = %q, want the_big_leagues_2", again)
	}
}

func TestCreateGroupRefusesWhatItCannotMake(t *testing.T) {
	tests := []struct {
		name  string
		group Group
		want  error
	}{
		{"no name", Group{Name: " ", Kind: "cup"}, ErrBadGroup},
		{"a name with no letters", Group{Name: "2024!", Kind: "cup"}, ErrBadGroup},
		{"no such kind", Group{Name: "Cups", Kind: "bowl"}, ErrBadKind},
		{"no such member", Group{Name: "Cups", Kind: "cup", Members: []string{"international", "no_such"}}, ErrNoTournament},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := groupedSeries(t)
			if _, err := a.CreateGroup(context.Background(), tt.group); !errors.Is(err, tt.want) {
				t.Errorf("CreateGroup = %v, want %v", err, tt.want)
			}
			if p := listed(t, a, "international").Parent; p != "" {
				t.Errorf("international's parent = %q after a refused group, want none", p)
			}
		})
	}
}

func TestSetMembersReplacesWhatAGroupHolds(t *testing.T) {
	a := groupedSeries(t)
	ctx := context.Background()
	g := group(t, a, "Leagues", "league", "international", "intermezzo")

	if err := a.SetMembers(ctx, g, []string{"intermezzo", "nations_league"}); err != nil {
		t.Fatalf("SetMembers: %v", err)
	}
	for slug, want := range map[string]string{"international": "", "intermezzo": g, "nations_league": g} {
		if p := listed(t, a, slug).Parent; p != want {
			t.Errorf("%s's parent = %q, want %q", slug, p, want)
		}
	}
}

func TestSetMembersRefusesWhatAGroupCannotHold(t *testing.T) {
	tests := []struct {
		name, group string
		members     []string
		want        error
	}{
		{"a group holding its own group", "inner", []string{"outer"}, ErrGroupCycle},
		{"a group holding itself", "outer", []string{"outer"}, ErrGroupCycle},
		{"a tournament holding others", "international", []string{"intermezzo"}, ErrNotAGroup},
		{"no such member", "outer", []string{"no_such"}, ErrNoTournament},
		{"no such group", "no_such", []string{"intermezzo"}, ErrNoTournament},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := groupedSeries(t)
			ctx := context.Background()
			// outer holds inner, which holds nations_league.
			inner := group(t, a, "Inner", "league", "nations_league")
			group(t, a, "Outer", "group", inner)
			if err := a.SetMembers(ctx, tt.group, tt.members); !errors.Is(err, tt.want) {
				t.Errorf("SetMembers(%s, %v) = %v, want %v", tt.group, tt.members, err, tt.want)
			}
			if p := listed(t, a, "nations_league").Parent; p != inner {
				t.Errorf("nations_league's parent = %q after a refused change, want %q", p, inner)
			}
		})
	}
}

func TestSetTournamentRenamesOnlyAGroup(t *testing.T) {
	a := groupedSeries(t)
	ctx := context.Background()
	g := group(t, a, "Leagues", "league")
	name := "Great Leagues"

	if err := a.SetTournament(ctx, g, TournamentSettings{Name: &name}); err != nil {
		t.Fatalf("SetTournament(rename group): %v", err)
	}
	if got := listed(t, a, g).Name; got != name {
		t.Errorf("name = %q, want %q", got, name)
	}
	if err := a.SetTournament(ctx, "international", TournamentSettings{Name: &name}); !errors.Is(err, ErrNotAGroup) {
		t.Errorf("renaming a tournament = %v, want ErrNotAGroup", err)
	}
}

func TestSetTournamentMovesASeriesOnlyIntoAGroup(t *testing.T) {
	a := groupedSeries(t)
	ctx := context.Background()
	g := group(t, a, "Leagues", "league")

	if err := a.SetTournament(ctx, "intermezzo", TournamentSettings{Parent: &g}); err != nil {
		t.Fatalf("SetTournament(into a group): %v", err)
	}
	if p := listed(t, a, "intermezzo").Parent; p != g {
		t.Errorf("parent = %q, want %q", p, g)
	}
	into := "international"
	if err := a.SetTournament(ctx, "nations_league", TournamentSettings{Parent: &into}); !errors.Is(err, ErrNotAGroup) {
		t.Errorf("moving into a tournament = %v, want ErrNotAGroup", err)
	}
}

func TestAdminTournamentsGiveEachSeriesItsGamesAndDates(t *testing.T) {
	a := groupedSeries(t)
	got := listed(t, a, "international")
	if got.Games != 2 || got.FirstGame == nil || !got.FirstGame.Equal(day("2024-09-15")) ||
		got.LastGame == nil || !got.LastGame.Equal(day("2024-10-20")) {
		t.Errorf("games, first, last = %d, %v, %v; want 2, 2024-09-15, 2024-10-20", got.Games, got.FirstGame, got.LastGame)
	}
}

func TestSetTournamentGivesOnlyAGroupAGroupsType(t *testing.T) {
	tests := []struct{ kind string }{{"series"}, {"group"}}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			a := groupedSeries(t)
			ctx := context.Background()
			g := group(t, a, "Leagues", "league")
			if err := a.SetTournament(ctx, g, TournamentSettings{Kind: tt.kind}); err != nil {
				t.Errorf("SetTournament(group, %s) = %v, want nil", tt.kind, err)
			}
			if err := a.SetTournament(ctx, "international", TournamentSettings{Kind: tt.kind}); !errors.Is(err, ErrBadKind) {
				t.Errorf("SetTournament(tournament, %s) = %v, want ErrBadKind", tt.kind, err)
			}
		})
	}
}
