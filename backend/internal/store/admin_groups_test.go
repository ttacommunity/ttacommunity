//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	return adminTournament(t, a, slug)
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

func TestSetTournamentRenamesAnyTournamentOrGroup(t *testing.T) {
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
	newName := "International Championship Renamed"
	if err := a.SetTournament(ctx, "international", TournamentSettings{Name: &newName}); err != nil {
		t.Fatalf("renaming a tournament: %v", err)
	}
	if got := listed(t, a, "international").Name; got != newName {
		t.Errorf("renamed tournament name = %q, want %q", got, newName)
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

func TestSeasonCountsCountEachStagesAndSeasonsGamesAndPlayers(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"Worlds 2024 Stage 1 - Group 1 game 1": day("2024-01-01"), "Worlds 2024 Stage 1 - Group 2 game 1": day("2024-01-02"),
		"Worlds 2024 Stage 2 - Final game 1": day("2024-02-01"),
	})
	setSeats(t, pool, "Worlds 2024 Stage 1 - Group 1 game 1", Seat{"ann", 200}, Seat{"bob", 150}, Seat{"cy", 100})
	setSeats(t, pool, "Worlds 2024 Stage 1 - Group 2 game 1", Seat{"dee", 200}, Seat{"eve", 150})
	setSeats(t, pool, "Worlds 2024 Stage 2 - Final game 1", Seat{"ann", 200}, Seat{"dee", 150})
	if _, err := pool.Exec(ctx, "UPDATE games SET state = 'running' WHERE name = 'Worlds 2024 Stage 1 - Group 2 game 1'"); err != nil {
		t.Fatal(err)
	}
	a := NewAdminStore(asAdminLogin(t, pool))

	got, err := a.SeasonCounts(ctx, "worlds")
	if err != nil {
		t.Fatal(err)
	}
	var counts []string
	for _, se := range got {
		counts = append(counts, fmt.Sprintf("%s: %d games %d finished %d players", se.Season, se.Games, se.Finished, se.Players))
		for _, st := range se.Stages {
			counts = append(counts, fmt.Sprintf("%s #%d %d games %d finished %d players", st.Stage, st.Number, st.Games, st.Finished, st.Players))
		}
	}
	if got, want := strings.Join(counts, ", "), "2024: 3 games 2 finished 5 players, Stage 1 #1 2 games 1 finished 5 players, Stage 2 #2 1 games 1 finished 2 players"; got != want {
		t.Errorf("counts = %q, want %q", got, want)
	}
	if _, err := a.SeasonCounts(ctx, "no_such"); !errors.Is(err, ErrNoTournament) {
		t.Errorf("SeasonCounts(no_such) = %v, want ErrNoTournament", err)
	}
}

func TestTournamentGivesItsSeasonsAndStagesTheirCounts(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"Worlds 2024 Stage 1 - Group 1 game 1": day("2024-01-01"), "Worlds 2024 Stage 2 - Final game 1": day("2024-02-01"),
	})
	setSeats(t, pool, "Worlds 2024 Stage 1 - Group 1 game 1", Seat{"ann", 200}, Seat{"bob", 150}, Seat{"cy", 100})
	setSeats(t, pool, "Worlds 2024 Stage 2 - Final game 1", Seat{"ann", 200}, Seat{"bob", 150})
	a := NewAdminStore(asAdminLogin(t, pool))

	w := adminTournament(t, a, "worlds")
	var counts []string
	for _, se := range w.Seasons {
		counts = append(counts, fmt.Sprintf("%s: %d/%d games %d players", se.Label, se.Finished, se.Games, se.Players))
		for _, st := range se.Stages {
			counts = append(counts, fmt.Sprintf("%s #%d %d/%d games %d players", st.Label, st.Number, st.Finished, st.Games, st.Players))
		}
	}
	if got, want := strings.Join(counts, ", "), "2024: 2/2 games 3 players, Stage 1 #1 1/1 games 3 players, Stage 2 #2 1/1 games 2 players"; got != want {
		t.Errorf("counts = %q, want %q", got, want)
	}
	if _, err := a.Tournament(ctx, "no_such"); !errors.Is(err, ErrNoTournament) {
		t.Errorf("Tournament(no_such) = %v, want ErrNoTournament", err)
	}
}

func TestTournamentCountsTheFinishedGamesOfASeasonWithoutStages(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"International S30 - Gold 2 game 1": day("2025-01-01"), "International S30 - Gold 2 game 2": day("2025-01-02"),
	})
	if _, err := pool.Exec(ctx, "UPDATE games SET state = 'running' WHERE name = 'International S30 - Gold 2 game 2'"); err != nil {
		t.Fatal(err)
	}
	a := NewAdminStore(asAdminLogin(t, pool))

	se := adminTournament(t, a, "international").Seasons[0]
	if se.Label != "30" || len(se.Stages) != 0 || se.Finished != 1 || se.Games != 2 {
		t.Errorf("season = %s with %d stages, %d/%d games; want 30 with no stages, 1/2 games", se.Label, len(se.Stages), se.Finished, se.Games)
	}
}
