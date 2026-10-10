//go:build integration

package store

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func adminTournament(t *testing.T, a *AdminStore, slug string) AdminTournament {
	t.Helper()
	got, err := a.Tournament(context.Background(), slug)
	if err != nil {
		t.Fatalf("Tournament(%s): %v", slug, err)
	}
	return got
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

	worlds := adminTournament(t, a, "worlds")
	if !worlds.Rated || worlds.Visible || worlds.Community {
		t.Errorf("worlds settings = %+v, want rated, hidden, and not yet in the community leaderboard (no migration row)", worlds)
	}
	var got2 []string
	for _, se := range worlds.Seasons {
		got2 = append(got2, fmt.Sprintf("%s games=%v note=%q", se.Label, se.HasGames, se.Note))
		for _, st := range se.Stages {
			got2 = append(got2, fmt.Sprintf("  %s note=%q", st.Label, st.Note))
		}
	}
	want := []string{`2025 games=true note=""`, `  Stage 1 note=""`, `  Stage 2 note="The upper bracket."`}
	if diff := cmp.Diff(want, got2); diff != "" {
		t.Errorf("worlds seasons (-want +got):\n%s", diff)
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
	var labels []string
	for _, s := range adminTournament(t, a, "international").Seasons {
		labels = append(labels, s.Label)
	}
	if diff := cmp.Diff([]string{"10", "3", "2", "1"}, labels); diff != "" {
		t.Errorf("seasons (-want +got):\n%s", diff)
	}
}

func TestATournamentFirstSeenStartsWithItsLevelNames(t *testing.T) {
	pool := migrated(t)
	placedOn(t, pool, map[string]time.Time{
		"Slow Burn S3 - group 1 game 1":           day("2024-07-01"),
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
	})
	a := NewAdminStore(asAdminLogin(t, pool))
	if sb := adminTournament(t, a, "slow_burn"); sb.SeasonName != "Edition" || sb.StageName != "Season" {
		t.Errorf("slow_burn names = %q, %q, want Edition, Season", sb.SeasonName, sb.StageName)
	}
	if w := adminTournament(t, a, "wimbledon"); w.SeasonName != "" || w.StageName != "" {
		t.Errorf("wimbledon names = %q, %q, want the defaults", w.SeasonName, w.StageName)
	}
}

func TestATournamentFirstSeenStartsWithItsScoring(t *testing.T) {
	pool := migrated(t)
	placedOn(t, pool, map[string]time.Time{
		"International S30 - Gold 2 game 1":       day("2025-01-01"),
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
	})
	a := NewAdminStore(asAdminLogin(t, pool))
	if got := adminTournament(t, a, "international").Points; !slices.Equal(got[3], []float64{5, 2, 0}) || !slices.Equal(got[4], []float64{6, 3, 1, 0}) {
		t.Errorf("international scoring = %v, want 3p 5-2-0, 4p 6-3-1-0", got)
	}
	if got := adminTournament(t, a, "wimbledon").Points; len(got[2])+len(got[3])+len(got[4]) != 0 {
		t.Errorf("wimbledon scoring = %v, want none", got)
	}
}

func TestARenamedSeasonAndStageKeepTheirOriginalNames(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	for _, q := range []string{
		"UPDATE tournaments SET label = 'Season 2024' WHERE label = '2024'",
		"UPDATE stages SET label = 'Groups' WHERE label = 'Stage 1'",
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	a := NewAdminStore(asAdminLogin(t, pool))
	se := adminTournament(t, a, "wimbledon").Seasons[0]
	if se.Label != "Season 2024" || se.OriginalName != "2024" || len(se.Stages) != 1 ||
		se.Stages[0].Label != "Groups" || se.Stages[0].OriginalName != "Stage 1" {
		t.Errorf("season = %+v", se)
	}
}

func TestAdminTournamentsSuggestASeasonsPodium(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"Worlds 2024 Stage 1 - Group 1 game 1": day("2024-01-01"), "Worlds 2024 Stage 1 - Group 2 game 1": day("2024-01-02"),
		"Worlds 2024 Stage 2 - Final game 1": day("2024-02-01"), "Worlds 2024 Stage 2 - Final game 2": day("2024-02-02"),
		"Worlds 2025 Stage 2 - Upper game 1": day("2025-02-01"), "Worlds 2025 Stage 2 - Lower game 1": day("2025-02-01"),
		"Worlds 2026 Stage 2 - Final game 1":   day("2026-02-01"),
		"Worlds 2027 Stage 1 - Group 1 game 1": day("2027-01-01"),
	})
	setSeats(t, pool, "Worlds 2024 Stage 2 - Final game 1", Seat{"ann", 200}, Seat{"bob", 150}, Seat{"cy", 100})
	setSeats(t, pool, "Worlds 2024 Stage 2 - Final game 2", Seat{"bob", 210}, Seat{"ann", 190}, Seat{"cy", 80})
	setSeats(t, pool, "Worlds 2026 Stage 2 - Final game 1", Seat{"dee", 200}, Seat{"eve", 150}, Seat{"fay", 100})
	for _, q := range []string{
		`INSERT INTO cge_tournaments (cge_tournament, name, status) VALUES (900, 'WC 2027', 'Stage 1 of 6')`,
		`UPDATE stages SET cge_tournament = 900, cge_stage_number = 1 WHERE tournament_id = (SELECT tournament_id FROM tournaments WHERE label = '2027')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tournament_official_placements (series, season, division, player_name, place, reason, source)
		VALUES ('worlds', 2026, 'Final', 'fay', 1, 'test', 'test'), ('worlds', 2026, 'Final', 'eve', 2, 'test', 'test')`); err != nil {
		t.Fatal(err)
	}
	a := NewAdminStore(asAdminLogin(t, pool))
	seasons := map[string]AdminSeason{}
	for _, s := range adminTournament(t, a, "worlds").Seasons {
		seasons[s.Label] = s
	}
	tests := []struct{ season, from, want string }{
		{"2024", "final_stage", "1 ann, 2 bob, 3 cy"},
		{"2025", "", ""},
		{"2026", "recorded", "1 fay, 2 eve"},
		{"2027", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.season, func(t *testing.T) {
			s := seasons[tt.season]
			var places []string
			for _, p := range s.Suggested {
				places = append(places, fmt.Sprintf("%d %s", p.Place, p.Name))
			}
			if got := strings.Join(places, ", "); got != tt.want || s.SuggestedFrom != tt.from {
				t.Errorf("suggested %q from %q, want %q from %q", got, s.SuggestedFrom, tt.want, tt.from)
			}
		})
	}
}
