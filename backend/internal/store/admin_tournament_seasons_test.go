//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"ttacommunity/backend/internal/tournamentcsv"
)

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
	if err := a.AddSeason(ctx, "international", "Season 1"); !errors.Is(err, ErrSeasonExists) {
		t.Errorf(`AddSeason("Season 1") after "1" = %v, want ErrSeasonExists: the site adds "Season" itself`, err)
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

func TestSetPodiumReplacesASeasonsPodiumAndClearPodiumRemovesIt(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"Worlds 2025 Stage 1 - Upper 1 game 1": day("2025-03-01")})
	a := NewAdminStore(asAdminLogin(t, pool))

	first := []PodiumEntry{{Place: 1, Name: "alder440"}, {Place: 2, Name: "kittiwake"}, {Place: 3, Name: "Eider"}}
	if err := a.SetPodium(ctx, "worlds", "2025", first, "From CGE's final"); err != nil {
		t.Fatalf("SetPodium: %v", err)
	}
	if err := a.SetPodium(ctx, "worlds", "2025", []PodiumEntry{{Place: 1, Name: "kittiwake"}}, ""); err != nil {
		t.Fatalf("SetPodium again: %v", err)
	}
	se := adminTournament(t, a, "worlds").Seasons[0]
	if diff := cmp.Diff([]PodiumEntry{{Place: 1, Name: "kittiwake"}}, se.Podium); diff != "" {
		t.Errorf("podium, only the second one (-want +got):\n%s", diff)
	}
	if se.PodiumNote != "" {
		t.Errorf("podium note = %q, want none", se.PodiumNote)
	}

	if err := a.ClearPodium(ctx, "worlds", "2025"); err != nil {
		t.Fatalf("ClearPodium: %v", err)
	}
	if p := adminTournament(t, a, "worlds").Seasons[0].Podium; len(p) != 0 {
		t.Errorf("podium after clearing = %+v", p)
	}
}

func TestSetPodiumRefusesWhatIsNotAPodium(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"Worlds 2025 Stage 1 - Upper 1 game 1": day("2025-03-01")})
	a := NewAdminStore(asAdminLogin(t, pool))
	for _, tt := range []struct {
		name, slug, season string
		podium             []PodiumEntry
		want               error
	}{
		{"no such season", "worlds", "1999", []PodiumEntry{{Place: 1, Name: "a"}}, ErrNoTournament},
		{"no such tournament", "nope", "2025", []PodiumEntry{{Place: 1, Name: "a"}}, ErrNoTournament},
		{"nobody", "worlds", "2025", nil, ErrBadPodium},
		{"a place twice", "worlds", "2025", []PodiumEntry{{Place: 1, Name: "a"}, {Place: 1, Name: "b"}}, ErrBadPodium},
		{"a player twice", "worlds", "2025", []PodiumEntry{{Place: 1, Name: "a"}, {Place: 2, Name: "a"}}, ErrBadPodium},
		{"fourth place", "worlds", "2025", []PodiumEntry{{Place: 4, Name: "a"}}, ErrBadPodium},
		{"no name", "worlds", "2025", []PodiumEntry{{Place: 1, Name: " "}}, ErrBadPodium},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := a.SetPodium(ctx, tt.slug, tt.season, tt.podium, ""); !errors.Is(err, tt.want) {
				t.Errorf("SetPodium = %v, want %v", err, tt.want)
			}
		})
	}
	var n int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM tournament_podium_overrides").Scan(&n)
	if n != 0 {
		t.Errorf("%d podium rows after only refusals", n)
	}
}

func TestASeasonCanBeLeftOutOfTheChampions(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
		"Wimbledon 2025 Stage 1 - group 1 game 1": day("2025-07-01"),
	})
	a := NewAdminStore(asAdminLogin(t, pool))
	if err := a.SetSeasonInChampions(ctx, "wimbledon", "2024", false); err != nil {
		t.Fatalf("SetSeasonInChampions: %v", err)
	}
	withSpans(t, pool, run)
	page, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(ctx, run, "wimbledon")
	if err != nil {
		t.Fatal(err)
	}
	for _, se := range page.Seasons {
		if se.InChampions != (se.Label != "2024") {
			t.Errorf("season %s in champions = %v", se.Label, se.InChampions)
		}
	}
	for _, se := range adminTournament(t, a, "wimbledon").Seasons {
		if se.InChampions != (se.Label != "2024") {
			t.Errorf("admin season %s in champions = %v", se.Label, se.InChampions)
		}
	}
	if err := a.SetSeasonInChampions(ctx, "wimbledon", "1999", false); !errors.Is(err, ErrNoTournament) {
		t.Errorf("no such season = %v, want ErrNoTournament", err)
	}
}

func TestASeasonCanBeLeftOutOfTheCommunityLeaderboard(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
		"Wimbledon 2025 Stage 1 - group 1 game 1": day("2025-07-01"),
	})
	a := NewAdminStore(asAdminLogin(t, pool))
	if err := a.SetSeasonInCommunity(ctx, "wimbledon", "2024", false); err != nil {
		t.Fatalf("SetSeasonInCommunity: %v", err)
	}
	for _, se := range adminTournament(t, a, "wimbledon").Seasons {
		if se.InCommunity != (se.Label != "2024") {
			t.Errorf("season %s in the community leaderboard = %v", se.Label, se.InCommunity)
		}
	}
	if err := a.SetSeasonInCommunity(ctx, "wimbledon", "1999", false); !errors.Is(err, ErrNoTournament) {
		t.Errorf("no such season = %v, want ErrNoTournament", err)
	}
}

func TestASeasonCanBeRunOnThisSite(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
		"Wimbledon 2025 Stage 1 - group 1 game 1": day("2025-07-01"),
	})
	a := NewAdminStore(asAdminLogin(t, pool))
	run := true
	if err := a.ChangeSeason(ctx, "wimbledon", "2024", SeasonChange{RunHere: &run}); err != nil {
		t.Fatalf("ChangeSeason: %v", err)
	}
	for _, se := range adminTournament(t, a, "wimbledon").Seasons {
		if se.RunHere != (se.Label == "2024") {
			t.Errorf("season %s run on this site = %v", se.Label, se.RunHere)
		}
	}
	if err := a.ChangeSeason(ctx, "wimbledon", "1999", SeasonChange{RunHere: &run}); !errors.Is(err, ErrNoTournament) {
		t.Errorf("no such season = %v, want ErrNoTournament", err)
	}
}

func TestAdminSetsASeasonsOrganiser(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
		"Wimbledon 2025 Stage 1 - group 1 game 1": day("2025-07-01"),
	})
	addCGETournaments(t, pool, tournamentcsv.CGETournament{ID: 71, Name: "Wimbledon 2025", Status: "finished",
		Organiser: "Lantern_Keeper", Tournament: "wimbledon", Season: "2025"})
	a := NewAdminStore(asAdminLogin(t, pool))
	set := func(season, name string) {
		t.Helper()
		if err := a.ChangeSeason(ctx, "wimbledon", season, SeasonChange{Organiser: &name}); err != nil {
			t.Fatalf("ChangeSeason(%s, organiser %q): %v", season, name, err)
		}
	}

	set("2024", "Quiet_Heron")
	set("2025", "Quiet_Heron")
	set("2025", "")
	tests := []struct{ season, organiser, cge string }{
		{"2024", "Quiet_Heron", ""},
		{"2025", "", "Lantern_Keeper"},
	}
	for _, tt := range tests {
		se := wimbledonSeason(t, a, tt.season)
		if se.Organiser != tt.organiser || se.CGEOrganiser != tt.cge {
			t.Errorf("season %s organiser = %q, CGE's %q; want %q, %q", tt.season, se.Organiser, se.CGEOrganiser, tt.organiser, tt.cge)
		}
	}
	name := "Quiet_Heron"
	if err := a.ChangeSeason(ctx, "wimbledon", "1999", SeasonChange{Organiser: &name}); !errors.Is(err, ErrNoTournament) {
		t.Errorf("no such season = %v, want ErrNoTournament", err)
	}
}

func TestAdminSetsASeasonsTimeoutPenalty(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S6 - Bronze 1 game 1": day("2020-05-01"),
		"Intermezzo S7 - Bronze 1 game 1": day("2020-08-01"),
	})
	a := NewAdminStore(asAdminLogin(t, pool))
	penalties := func() map[string]float64 {
		t.Helper()
		got := map[string]float64{}
		for _, se := range adminTournament(t, a, "intermezzo").Seasons {
			got[se.Label] = se.TimeoutPenalty
		}
		return got
	}
	if err := a.SetSeasonPenalty(ctx, "intermezzo", "6", -1); err != nil {
		t.Fatalf("SetSeasonPenalty: %v", err)
	}
	if got := penalties(); got["6"] != -1 || got["7"] != 0 {
		t.Errorf("penalties = %v, want S6 -1, S7 0", got)
	}
	if err := a.SetSeasonPenalty(ctx, "intermezzo", "6", 0); err != nil {
		t.Fatalf("SetSeasonPenalty: %v", err)
	}
	if got := penalties(); got["6"] != 0 {
		t.Errorf("after setting 0, S6 penalty = %v, want 0", got["6"])
	}
	if err := a.SetSeasonPenalty(ctx, "intermezzo", "7", 1); !errors.Is(err, ErrBadPenalty) {
		t.Errorf("a positive penalty = %v, want ErrBadPenalty", err)
	}
	if err := a.SetSeasonPenalty(ctx, "intermezzo", "99", -1); !errors.Is(err, ErrNoTournament) {
		t.Errorf("no such season = %v, want ErrNoTournament", err)
	}
}

func wimbledonSeason(t *testing.T, a *AdminStore, label string) AdminSeason {
	t.Helper()
	for _, se := range adminTournament(t, a, "wimbledon").Seasons {
		if se.Label == label {
			return se
		}
	}
	t.Fatalf("wimbledon has no season %s", label)
	return AdminSeason{}
}

func wimbledon2023(t *testing.T) *AdminStore {
	t.Helper()
	pool := migrated(t)
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2023 stage 1": day("2023-07-15")})
	return NewAdminStore(asAdminLogin(t, pool))
}

func TestChangeSeasonAppliesEveryPart(t *testing.T) {
	a := wimbledon2023(t)
	rated, note := false, "Played on grass."

	if err := a.ChangeSeason(context.Background(), "wimbledon", "2023", SeasonChange{Rated: &rated, Note: &note}); err != nil {
		t.Fatalf("ChangeSeason: %v", err)
	}
	if got := wimbledonSeason(t, a, "2023"); got.Rated || got.Note != note {
		t.Errorf("after the change: rated %v, note %q; want false, %q", got.Rated, got.Note, note)
	}
}

func TestChangeSeasonRefusedChangesNothing(t *testing.T) {
	rated, note, penalty := false, "Played on grass.", 1.0
	tests := []struct {
		name string
		c    SeasonChange
		want error
	}{
		{"a penalty above zero", SeasonChange{Rated: &rated, Note: &note, TimeoutPenalty: &penalty}, ErrBadPenalty},
		{"a note for a stage that does not exist", SeasonChange{Rated: &rated, Note: &note,
			StageNotes: map[string]string{"no_such": "A stage note."}}, ErrNoTournament},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := wimbledon2023(t)

			if err := a.ChangeSeason(context.Background(), "wimbledon", "2023", tt.c); !errors.Is(err, tt.want) {
				t.Errorf("ChangeSeason = %v, want %v", err, tt.want)
			}
			if got := wimbledonSeason(t, a, "2023"); !got.Rated || got.Note != "" {
				t.Errorf("after a refused change: rated %v, note %q; want it unchanged", got.Rated, got.Note)
			}
		})
	}
}

func TestMoveSeasonMovesBetweenTournaments(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"International S30 - Grandmaster game 1": day("2024-09-15"),
		"Intermezzo S24 - Bronze 1 game 1":       day("2024-09-16"),
	})
	a := NewAdminStore(asAdminLogin(t, pool))

	if err := a.SetNote(ctx, "international", "30", "", "Top tier season."); err != nil {
		t.Fatalf("SetNote: %v", err)
	}

	dest := "intermezzo"
	newLabel := "99"
	if err := a.ChangeSeason(ctx, "international", "30", SeasonChange{MoveToTournament: &dest, NewSeason: &newLabel}); err != nil {
		t.Fatalf("ChangeSeason MoveSeason: %v", err)
	}

	intnl := adminTournament(t, a, "international")
	for _, se := range intnl.Seasons {
		if se.Label == "30" {
			t.Errorf("international still has season 30 after move")
		}
	}

	im := adminTournament(t, a, "intermezzo")
	found := false
	for _, se := range im.Seasons {
		if se.Label == "99" {
			found = true
			if se.Note != "Top tier season." {
				t.Errorf("season 99 note = %q, want %q", se.Note, "Top tier season.")
			}
		}
	}
	if !found {
		t.Errorf("intermezzo does not have moved season 99")
	}

	// Refuses collision with existing season
	destIntnl := "international"
	existingLabel := "99"
	if err := a.MoveSeason(ctx, "intermezzo", "99", dest, existingLabel); !errors.Is(err, ErrSeasonExists) {
		t.Errorf("moving to existing label = %v, want ErrSeasonExists", err)
	}
	_ = destIntnl
}
