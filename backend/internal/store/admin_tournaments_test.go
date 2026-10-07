//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/tournamentcsv"
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
	var got2 []string
	for _, se := range worlds.Seasons {
		got2 = append(got2, fmt.Sprintf("%s games=%v note=%q", se.Label, se.HasGames, se.Note))
		for _, st := range se.Stages {
			got2 = append(got2, fmt.Sprintf("  %s note=%q", st.Label, st.Note))
		}
	}
	want := []string{`2025 games=true note=""`, `  Stage 1 note=""`, `  Stage 2 note="The upper bracket."`}
	if !reflect.DeepEqual(got2, want) {
		t.Errorf("worlds seasons = %q, want %q", got2, want)
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

func TestSetPodiumReplacesASeasonsPodiumAndClearPodiumRemovesIt(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"Worlds 2025 Stage 1 - Upper 1 game 1": day("2025-03-01")})
	a := NewAdminStore(asAdminLogin(t, pool))

	first := []PodiumEntry{{Place: 1, Name: "a440"}, {Place: 2, Name: "kyshantry"}, {Place: 3, Name: "Eepogi"}}
	if err := a.SetPodium(ctx, "worlds", "2025", first, "From CGE's final"); err != nil {
		t.Fatalf("SetPodium: %v", err)
	}
	if err := a.SetPodium(ctx, "worlds", "2025", []PodiumEntry{{Place: 1, Name: "kyshantry"}}, ""); err != nil {
		t.Fatalf("SetPodium again: %v", err)
	}
	got, err := a.Tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	se := adminTournament(t, got, "worlds").Seasons[0]
	if !reflect.DeepEqual(se.Podium, []PodiumEntry{{Place: 1, Name: "kyshantry"}}) || se.PodiumNote != "" {
		t.Errorf("podium = %+v %q, want only the second one", se.Podium, se.PodiumNote)
	}

	if err := a.ClearPodium(ctx, "worlds", "2025"); err != nil {
		t.Fatalf("ClearPodium: %v", err)
	}
	got, _ = a.Tournaments(ctx)
	if p := adminTournament(t, got, "worlds").Seasons[0].Podium; len(p) != 0 {
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
		ts, err := a.Tournaments(ctx)
		if err != nil {
			t.Fatal(err)
		}
		w := adminTournament(t, ts, "wimbledon")
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
		ts, err := a.Tournaments(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return adminTournament(t, ts, "wimbledon").Card
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
		ts, err := a.Tournaments(ctx)
		if err != nil {
			t.Fatal(err)
		}
		page, err := r.CatalogueTournament(ctx, run, "wimbledon")
		if err != nil {
			t.Fatal(err)
		}
		return adminTournament(t, ts, "wimbledon").Stories, page.Stories
	}

	if inAdmin, onPage := stories(); len(inAdmin) != 0 || len(onPage) != 0 {
		t.Errorf("stories before any = %+v, %+v", inAdmin, onPage)
	}
	want := []Story{
		{Player: "saru", Badge: "2024 Champion", Title: "The grass-court crown", Text: "saru beat whsvin 3-2 in the final."},
		{Player: "whsvin", Badge: "", Title: "Runner-up", Text: "Two finals in a row."},
	}
	if err := a.SetStories(ctx, "wimbledon", want); err != nil {
		t.Fatalf("SetStories: %v", err)
	}
	if inAdmin, onPage := stories(); !reflect.DeepEqual(inAdmin, want) || !reflect.DeepEqual(onPage, want) {
		t.Errorf("stories = %+v and %+v, want %+v in order on both", inAdmin, onPage, want)
	}
	if err := a.SetStories(ctx, "wimbledon", want[1:]); err != nil {
		t.Fatalf("SetStories(one): %v", err)
	}
	if inAdmin, _ := stories(); !reflect.DeepEqual(inAdmin, want[1:]) {
		t.Errorf("stories after replacing = %+v, want only the second", inAdmin)
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
	ts, err := a.Tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	w := adminTournament(t, ts, "wimbledon")
	if !reflect.DeepEqual(w.RatedPlayerCounts, []int{2, 4}) {
		t.Errorf("rated player counts = %v, want 2 and 4, in order", w.RatedPlayerCounts)
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

func TestAdminSetsATournamentsKindAndTopDivisions(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	a := NewAdminStore(asAdminLogin(t, pool))
	if err := a.SetTournament(ctx, "wimbledon", TournamentSettings{Kind: "league", TopDivisions: []string{" Gold ", "Silver"}}); err != nil {
		t.Fatalf("SetTournament: %v", err)
	}
	ts, err := a.Tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	w := adminTournament(t, ts, "wimbledon")
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
		ts, err := a.Tournaments(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return adminTournament(t, ts, "wimbledon").Points
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
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"International S30 - Gold 2 game 1":       day("2025-01-01"),
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
	})
	ts, err := NewAdminStore(asAdminLogin(t, pool)).Tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := adminTournament(t, ts, "international").Points; !slices.Equal(got[3], []float64{5, 2, 0}) || !slices.Equal(got[4], []float64{6, 3, 1, 0}) {
		t.Errorf("international scoring = %v, want 3p 5-2-0, 4p 6-3-1-0", got)
	}
	if got := adminTournament(t, ts, "wimbledon").Points; len(got[2])+len(got[3])+len(got[4]) != 0 {
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
		"UPDATE tournament_seasons SET label = 'Season 2024' WHERE label = '2024'",
		"UPDATE tournament_stages SET label = 'Groups' WHERE label = 'Stage 1'",
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	ts, err := NewAdminStore(asAdminLogin(t, pool)).Tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	se := adminTournament(t, ts, "wimbledon").Seasons[0]
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
	page, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(ctx, run, "wimbledon")
	if err != nil {
		t.Fatal(err)
	}
	for _, se := range page.Seasons {
		if se.InChampions != (se.Label != "2024") {
			t.Errorf("season %s in champions = %v", se.Label, se.InChampions)
		}
	}
	ts, err := a.Tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, se := range adminTournament(t, ts, "wimbledon").Seasons {
		if se.InChampions != (se.Label != "2024") {
			t.Errorf("admin season %s in champions = %v", se.Label, se.InChampions)
		}
	}
	if err := a.SetSeasonInChampions(ctx, "wimbledon", "1999", false); !errors.Is(err, ErrNoTournament) {
		t.Errorf("no such season = %v, want ErrNoTournament", err)
	}
}
