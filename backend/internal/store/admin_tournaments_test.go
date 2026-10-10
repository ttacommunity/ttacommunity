//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/tournamentcsv"

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

	nl := adminTournament(t, a, "nations_league")
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

func TestSetRegulationsReplacesAndAnEmptyTextRemoves(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	started := day("2024-07-01")
	if _, err := ReplaceCGETournaments(ctx, pool, []tournamentcsv.CGETournament{
		{ID: 4991, Name: "Wimbledon 2024", Status: "finished", Started: &started, Description: "CGE's rules.", Tournament: "wimbledon", Season: "2024"},
	}); err != nil {
		t.Fatal(err)
	}
	a := NewAdminStore(asAdminLogin(t, pool))
	regs := func() (string, string) {
		w := adminTournament(t, a, "wimbledon")
		return w.Regulations, w.CGERegulations
	}

	if mine, cge := regs(); mine != "" || cge != "CGE's rules." {
		t.Errorf("before = %q, %q; want none written and CGE's latest", mine, cge)
	}
	for _, text := range []string{"First.", "  Second.  "} {
		if err := a.SetRegulations(ctx, "wimbledon", text); err != nil {
			t.Fatalf("SetRegulations(%q): %v", text, err)
		}
	}
	if mine, _ := regs(); mine != "Second." {
		t.Errorf("regulations = %q, want the second, trimmed", mine)
	}
	if err := a.SetRegulations(ctx, "wimbledon", " "); err != nil {
		t.Fatalf("SetRegulations(empty): %v", err)
	}
	if mine, _ := regs(); mine != "" {
		t.Errorf("regulations after an empty text = %q, want none", mine)
	}
	if err := a.SetRegulations(ctx, "nope", "x"); !errors.Is(err, ErrNoTournament) {
		t.Errorf("SetRegulations(nope) = %v, want ErrNoTournament", err)
	}
}

func TestSetCardReplacesAndAnEmptyCardRemoves(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	a := NewAdminStore(asAdminLogin(t, pool))
	card := func() Card {
		t.Helper()
		return adminTournament(t, a, "wimbledon").Card
	}

	if got := card(); got != (Card{}) {
		t.Errorf("card before any = %+v, want empty", got)
	}
	if err := a.SetCard(ctx, "wimbledon", Card{Icon: " 🎾 ", Tagline: " Grass-court slam ", Summary: " Played each July. "}); err != nil {
		t.Fatalf("SetCard: %v", err)
	}
	if got, want := card(), (Card{Icon: "🎾", Tagline: "Grass-court slam", Summary: "Played each July."}); got != want {
		t.Errorf("card = %+v, want %+v, trimmed", got, want)
	}
	if err := a.SetCard(ctx, "wimbledon", Card{}); err != nil {
		t.Fatalf("SetCard(empty): %v", err)
	}
	if got := card(); got != (Card{}) {
		t.Errorf("card after an empty one = %+v, want none", got)
	}
	for _, tt := range []struct {
		name, slug string
		c          Card
		want       error
	}{
		{"no such tournament", "nope", Card{Icon: "x"}, ErrNoTournament},
		{"a long icon", "wimbledon", Card{Icon: "🎾🎾🎾🎾🎾🎾🎾🎾🎾"}, ErrBadCard},
		{"a long description", "wimbledon", Card{Tagline: strings.Repeat("x", 61)}, ErrBadCard},
		{"a long information box", "wimbledon", Card{Summary: strings.Repeat("x", 1001)}, ErrBadCard},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := a.SetCard(ctx, tt.slug, tt.c); !errors.Is(err, tt.want) {
				t.Errorf("SetCard = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSetStoriesReplacesATournamentsStoriesInOrder(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	a := NewAdminStore(asAdminLogin(t, pool))
	r := NewRatingsReader(asReader(t, pool))
	stories := func() ([]Story, []Story) {
		t.Helper()
		withSpans(t, pool, run)
		page, err := r.CatalogueTournament(ctx, run, "wimbledon")
		if err != nil {
			t.Fatal(err)
		}
		return adminTournament(t, a, "wimbledon").Stories, page.Stories
	}

	if inAdmin, onPage := stories(); len(inAdmin) != 0 || len(onPage) != 0 {
		t.Errorf("stories before any = %+v, %+v", inAdmin, onPage)
	}
	want := []Story{
		{Player: "sora", Badge: "2024 Champion", Title: "The grass-court crown", Text: "sora beat wheatear 3-2 in the final."},
		{Player: "wheatear", Badge: "", Title: "Runner-up", Text: "Two finals in a row."},
	}
	if err := a.SetStories(ctx, "wimbledon", want); err != nil {
		t.Fatalf("SetStories: %v", err)
	}
	inAdmin, onPage := stories()
	if diff := cmp.Diff(want, inAdmin); diff != "" {
		t.Errorf("stories in admin (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(want, onPage); diff != "" {
		t.Errorf("stories on the page (-want +got):\n%s", diff)
	}
	if err := a.SetStories(ctx, "wimbledon", want[1:]); err != nil {
		t.Fatalf("SetStories(one): %v", err)
	}
	inAdmin, _ = stories()
	if diff := cmp.Diff(want[1:], inAdmin); diff != "" {
		t.Errorf("stories after replacing (-want +got):\n%s", diff)
	}
	for _, tt := range []struct {
		name, slug string
		s          []Story
		want       error
	}{
		{"no such tournament", "nope", want, ErrNoTournament},
		{"no player", "wimbledon", []Story{{Title: "t", Text: "x"}}, ErrBadStory},
		{"no title", "wimbledon", []Story{{Player: "p", Text: "x"}}, ErrBadStory},
		{"no story", "wimbledon", []Story{{Player: "p", Title: "t"}}, ErrBadStory},
		{"a long story", "wimbledon", []Story{{Player: "p", Title: "t", Text: strings.Repeat("x", 1001)}}, ErrBadStory},
		{"too many", "wimbledon", make([]Story, 13), ErrBadStory},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := a.SetStories(ctx, tt.slug, tt.s); !errors.Is(err, tt.want) {
				t.Errorf("SetStories = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestAdminSetsWhichSeasonsAndPlayerCountsAreRated(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"Wimbledon 2023 stage 1":                  day("2023-07-15"),
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
	})
	a := NewAdminStore(asAdminLogin(t, pool))
	if err := a.SetTournament(ctx, "wimbledon", TournamentSettings{RatedPlayerCounts: []int{4, 2}}); err != nil {
		t.Fatalf("SetTournament(player counts): %v", err)
	}
	if err := a.SetSeasonRated(ctx, "wimbledon", "2023", false); err != nil {
		t.Fatalf("SetSeasonRated: %v", err)
	}
	w := adminTournament(t, a, "wimbledon")
	if diff := cmp.Diff([]int{2, 4}, w.RatedPlayerCounts); diff != "" {
		t.Errorf("rated player counts (-want +got):\n%s", diff)
	}
	for _, se := range w.Seasons {
		if se.Rated != (se.Label != "2023") {
			t.Errorf("season %s rated = %v", se.Label, se.Rated)
		}
	}
	for _, tt := range []struct {
		name string
		err  error
		want error
	}{
		{"no player count", a.SetTournament(ctx, "wimbledon", TournamentSettings{RatedPlayerCounts: []int{}}), ErrBadPlayerCounts},
		{"a 5-player count", a.SetTournament(ctx, "wimbledon", TournamentSettings{RatedPlayerCounts: []int{2, 5}}), ErrBadPlayerCounts},
		{"no such season", a.SetSeasonRated(ctx, "wimbledon", "1999", false), ErrNoTournament},
	} {
		if !errors.Is(tt.err, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, tt.err, tt.want)
		}
	}
}

func TestAdminSetsWhatATournamentCallsItsSeasonsAndStages(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	a := NewAdminStore(asAdminLogin(t, pool))
	names := func() (string, string) {
		t.Helper()
		w := adminTournament(t, a, "wimbledon")
		return w.SeasonName, w.StageName
	}
	if season, stage := names(); season != "" || stage != "" {
		t.Errorf("before = %q, %q, want both empty (the defaults)", season, stage)
	}
	if err := a.SetTournament(ctx, "wimbledon", TournamentSettings{SeasonName: ptr(" Edition "), StageName: ptr("Round")}); err != nil {
		t.Fatalf("SetTournament: %v", err)
	}
	if season, stage := names(); season != "Edition" || stage != "Round" {
		t.Errorf("after = %q, %q, want Edition, Round", season, stage)
	}
	if err := a.SetTournament(ctx, "wimbledon", TournamentSettings{StageName: ptr("")}); err != nil {
		t.Fatalf("SetTournament: %v", err)
	}
	if season, stage := names(); season != "Edition" || stage != "" {
		t.Errorf("after clearing the stage name = %q, %q, want Edition and the default", season, stage)
	}
	if err := a.SetTournament(ctx, "wimbledon", TournamentSettings{SeasonName: ptr(strings.Repeat("x", 33))}); !errors.Is(err, ErrBadLevelName) {
		t.Errorf("a 33-character name = %v, want ErrBadLevelName", err)
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

func TestAdminSetsATournamentsKindAndTopDivisions(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	a := NewAdminStore(asAdminLogin(t, pool))
	if err := a.SetTournament(ctx, "wimbledon", TournamentSettings{Kind: "league", TopDivisions: []string{" Gold ", "Silver"}}); err != nil {
		t.Fatalf("SetTournament: %v", err)
	}
	w := adminTournament(t, a, "wimbledon")
	if w.Kind != "league" || !slices.Equal(w.TopDivisions, []string{"Gold", "Silver"}) || w.OriginalName != "Wimbledon" {
		t.Errorf("wimbledon = kind %q, top %v, original %q", w.Kind, w.TopDivisions, w.OriginalName)
	}
	if err := a.SetTournament(ctx, "wimbledon", TournamentSettings{Kind: "tour"}); !errors.Is(err, ErrBadKind) {
		t.Errorf("an unknown kind = %v, want ErrBadKind", err)
	}
}

func TestAdminSetsATournamentsScoring(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	a := NewAdminStore(asAdminLogin(t, pool))
	scoring := func() map[int][]float64 {
		t.Helper()
		return adminTournament(t, a, "wimbledon").Points
	}
	if err := a.SetTournament(ctx, "wimbledon", TournamentSettings{Points: map[int][]float64{3: {5, 2, 0}, 4: {6, 3, 1, 0}}}); err != nil {
		t.Fatalf("SetTournament: %v", err)
	}
	if got := scoring(); !slices.Equal(got[3], []float64{5, 2, 0}) || !slices.Equal(got[4], []float64{6, 3, 1, 0}) || len(got[2]) != 0 {
		t.Errorf("scoring = %v, want 3p 5-2-0, 4p 6-3-1-0, no 2p", got)
	}
	if err := a.SetTournament(ctx, "wimbledon", TournamentSettings{Points: map[int][]float64{3: {}}}); err != nil {
		t.Fatalf("SetTournament: %v", err)
	}
	if got := scoring(); len(got[3]) != 0 || !slices.Equal(got[4], []float64{6, 3, 1, 0}) {
		t.Errorf("after clearing 3p, scoring = %v, want only 4p 6-3-1-0", got)
	}
	for _, tt := range []struct {
		name   string
		points map[int][]float64
	}{
		{"a place missing", map[int][]float64{3: {5, 2}}},
		{"a worse place scoring more", map[int][]float64{3: {2, 5, 0}}},
		{"negative points", map[int][]float64{2: {2, -1}}},
		{"five players", map[int][]float64{5: {5, 4, 3, 2, 1}}},
	} {
		if err := a.SetTournament(ctx, "wimbledon", TournamentSettings{Points: tt.points}); !errors.Is(err, ErrBadScoring) {
			t.Errorf("%s: %v, want ErrBadScoring", tt.name, err)
		}
	}
	if got := scoring(); !slices.Equal(got[4], []float64{6, 3, 1, 0}) {
		t.Errorf("after refusals, scoring = %v, want 4p 6-3-1-0 unchanged", got)
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

func TestACardKeepsItsSubtitleAndCadence(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	a := NewAdminStore(asAdminLogin(t, pool))
	want := Card{Icon: "🎾", Tagline: "Annual 2-Player Knock-Out", Summary: "Grass.", Subtitle: "2-Player Knock-Out Format", Cadence: "Annual (Every July)"}
	if err := a.SetCard(ctx, "wimbledon", want); err != nil {
		t.Fatalf("SetCard: %v", err)
	}
	withSpans(t, pool, run)
	page, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(ctx, run, "wimbledon")
	if err != nil || page.Card != want {
		t.Errorf("card on the page = %+v, %v; want %+v", page.Card, err, want)
	}
	if err := a.SetCard(ctx, "wimbledon", Card{Cadence: strings.Repeat("x", 61)}); !errors.Is(err, ErrBadCard) {
		t.Errorf("a long cadence = %v, want ErrBadCard", err)
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

func TestChangeTournamentAppliesEveryPart(t *testing.T) {
	a := groupedSeries(t)
	ctx := context.Background()
	g := group(t, a, "Leagues", "league", "international")
	name, regulations := "Leagues renamed", "Best of three."

	err := a.ChangeTournament(ctx, g, TournamentChange{
		Settings:    &TournamentSettings{Name: &name},
		Card:        &Card{Icon: "🏆"},
		Regulations: &regulations,
		Stories:     &[]Story{{Player: "Player One", Title: "First", Text: "Won it."}},
		Members:     &[]string{"intermezzo"},
	})
	if err != nil {
		t.Fatalf("ChangeTournament: %v", err)
	}
	got := listed(t, a, g)
	if got.Name != name || got.Card.Icon != "🏆" || got.Regulations != regulations || len(got.Stories) != 1 {
		t.Errorf("after the change: name %q, card %+v, regulations %q, %d stories; want every part set",
			got.Name, got.Card, got.Regulations, len(got.Stories))
	}
	if p := listed(t, a, "intermezzo").Parent; p != g {
		t.Errorf("intermezzo's parent = %q, want %q", p, g)
	}
}

func TestChangeTournamentRefusedChangesNothing(t *testing.T) {
	name, regulations := "Leagues renamed", "Best of three."
	tests := []struct {
		name string
		c    TournamentChange
		want error
	}{
		{"a member that does not exist", TournamentChange{Settings: &TournamentSettings{Name: &name}, Card: &Card{Icon: "🏆"},
			Regulations: &regulations, Members: &[]string{"no_such"}}, ErrNoTournament},
		{"a story without a title", TournamentChange{Settings: &TournamentSettings{Name: &name}, Card: &Card{Icon: "🏆"},
			Stories: &[]Story{{Player: "Player One", Text: "Won it."}}}, ErrBadStory},
		{"a card too long", TournamentChange{Settings: &TournamentSettings{Name: &name}, Card: &Card{Icon: strings.Repeat("x", 9)}},
			ErrBadCard},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := groupedSeries(t)
			g := group(t, a, "Leagues", "league", "international")

			if err := a.ChangeTournament(context.Background(), g, tt.c); !errors.Is(err, tt.want) {
				t.Errorf("ChangeTournament = %v, want %v", err, tt.want)
			}
			got := listed(t, a, g)
			if got.Name != "Leagues" || got.Card != (Card{}) || got.Regulations != "" || len(got.Stories) != 0 {
				t.Errorf("after a refused change: name %q, card %+v, regulations %q, %d stories; want it unchanged",
					got.Name, got.Card, got.Regulations, len(got.Stories))
			}
			if p := listed(t, a, "international").Parent; p != g {
				t.Errorf("international's parent = %q after a refused change, want %q", p, g)
			}
		})
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
