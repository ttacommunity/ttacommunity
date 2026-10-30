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
	addPlayers(t, pool, "sora", "wheatear")
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

func TestAdminSetsATournamentsFormatInBrief(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	a := NewAdminStore(asAdminLogin(t, pool))
	if err := a.SetTournament(ctx, "wimbledon", TournamentSettings{FormatSummary: ptr(" Knockout duels. ")}); err != nil {
		t.Fatalf("SetTournament: %v", err)
	}
	if got := adminTournament(t, a, "wimbledon").FormatSummary; got != "Knockout duels." {
		t.Errorf("format in brief = %q, want it trimmed", got)
	}
	if err := a.SetTournament(ctx, "wimbledon", TournamentSettings{FormatSummary: ptr("")}); err != nil {
		t.Fatalf("SetTournament: %v", err)
	}
	if got := adminTournament(t, a, "wimbledon").FormatSummary; got != "" {
		t.Errorf("after clearing, format in brief = %q, want none", got)
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

func TestAdminSetsALeaguesRules(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	a := NewAdminStore(asAdminLogin(t, pool))
	yes, four := true, 4
	rules := func() (string, bool, int) {
		t.Helper()
		w := adminTournament(t, a, "wimbledon")
		return fmt.Sprint(w.Divisions), w.SuperGM, w.RecordMinGames
	}

	if err := a.SetTournament(ctx, "wimbledon", TournamentSettings{Divisions: []Division{{" Gold ", 1}, {"Silver", 2}, {"Bronze", 2}},
		SuperGM: &yes, RecordMinGames: &four}); err != nil {
		t.Fatalf("SetTournament: %v", err)
	}
	if divisions, sgm, games := rules(); divisions != "[{Gold 1} {Silver 2} {Bronze 2}]" || !sgm || games != 4 {
		t.Errorf("rules = %s, Super GM %v, %d games; want Gold 1, Silver 2, Bronze 2, Super GM, 4 games", divisions, sgm, games)
	}
	for _, tt := range []struct {
		name string
		s    TournamentSettings
	}{
		{"a tier above 7", TournamentSettings{Divisions: []Division{{"Gold", 8}}}},
		{"a tier below 1", TournamentSettings{Divisions: []Division{{"Gold", 0}}}},
		{"a division listed twice", TournamentSettings{Divisions: []Division{{"Gold", 1}, {" Gold", 2}}}},
		{"a division without a name", TournamentSettings{Divisions: []Division{{" ", 1}}}},
		{"negative games", TournamentSettings{RecordMinGames: new(-1)}},
	} {
		if err := a.SetTournament(ctx, "wimbledon", tt.s); !errors.Is(err, ErrBadLeagueRules) {
			t.Errorf("%s: %v, want ErrBadLeagueRules", tt.name, err)
		}
	}
	if divisions, _, games := rules(); divisions != "[{Gold 1} {Silver 2} {Bronze 2}]" || games != 4 {
		t.Errorf("after refusals, rules = %s, %d games; want them unchanged", divisions, games)
	}
	if err := a.SetTournament(ctx, "wimbledon", TournamentSettings{Divisions: []Division{}}); err != nil {
		t.Fatalf("SetTournament(no divisions): %v", err)
	}
	if divisions, _, _ := rules(); divisions != "[]" {
		t.Errorf("after clearing, divisions = %s, want none", divisions)
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

func TestStoriesAndPodiumsRefuseANameThatIsNoPlayers(t *testing.T) {
	tests := []struct {
		name string
		set  func(context.Context, *AdminStore, string) error
	}{
		{"a story", func(ctx context.Context, a *AdminStore, player string) error {
			return a.SetStories(ctx, "worlds", []Story{{Player: player, Title: "Won", Text: "Won it."}})
		}},
		{"a podium", func(ctx context.Context, a *AdminStore, player string) error {
			return a.SetPodium(ctx, "worlds", "2025", []PodiumEntry{{Place: 1, Name: player}}, "")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			ctx := context.Background()
			placedOn(t, pool, map[string]time.Time{"Worlds 2025 Stage 1 - Upper 1 game 1": day("2025-03-01")})
			addPlayers(t, pool, "kittiwake")
			a := NewAdminStore(asAdminLogin(t, pool))
			if err := tt.set(ctx, a, "kittiwake"); err != nil {
				t.Fatalf("setting a player's: %v", err)
			}
			for _, name := range []string{"ghost", "Kittiwake"} {
				if err := tt.set(ctx, a, name); !errors.Is(err, ErrNoPlayer) {
					t.Errorf("setting %s naming %q = %v, want ErrNoPlayer", tt.name, name, err)
				}
			}
		})
	}
}
