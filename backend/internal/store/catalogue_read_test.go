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

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"ttacommunity/backend/internal/tournamentcsv"
)

func TestCatalogueListsEachTournamentWithGames(t *testing.T) {
	pool := migrated(t)
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1":        day("2024-01-02"),
		"Intermezzo S24 - Bronze 1 game 2":        day("2024-02-03"),
		"Intermezzo S25 - Wood 1 game 1":          day("2024-05-01"),
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
		"Friendly game":                           day("2024-08-01"),
	})

	withSpans(t, pool, run)
	view, err := NewRatingsReader(asReader(t, pool)).Catalogue(context.Background(), run)
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	// Each game here has ann and one other player of its own; the friendly game is placed nowhere.
	want := []CatalogueEntry{
		{Slug: "intermezzo", Name: "Intermezzo", Seasons: 2, Rankings: []string{"community"}, Span: GameSpan{Games: 3, Players: 4, First: day("2024-01-02"), Last: day("2024-05-01")}},
		{Slug: "wimbledon", Name: "Wimbledon", Seasons: 1, Rankings: []string{"community"}, Span: GameSpan{Games: 1, Players: 2, First: day("2024-07-01"), Last: day("2024-07-01")}},
	}
	if diff := cmp.Diff(want, view.Tournaments, cmpopts.IgnoreFields(CatalogueEntry{}, "Card")); diff != "" {
		t.Errorf("Catalogue tournaments (-want +got):\n%s", diff)
	}
}

func TestCatalogueTournamentGivesSeasonsInOrderWithStagesAndPodium(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Wimbledon 2024 Stage 7 - Final game 1":   day("2025-02-24"),
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
		"Wimbledon 2023 stage 1":                  day("2023-07-15"),
	})
	if _, err := ReplacePodiums(ctx, pool, []tournamentcsv.Podium{
		{Cup: "wimbledon", Season: "2024", Place: 1, Player: "ann", Source: "CGE", Finished: day("2025-02-24")},
	}); err != nil {
		t.Fatalf("ReplacePodiums: %v", err)
	}

	withSpans(t, pool, run)
	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(ctx, run, "wimbledon")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	stage := func(label string, d time.Time) CatalogueStage {
		return CatalogueStage{Label: label, Span: GameSpan{Games: 1, Players: 2, First: d, Last: d}}
	}
	want := []CatalogueSeason{
		{Label: "2023", Span: GameSpan{Games: 1, Players: 2, First: day("2023-07-15"), Last: day("2023-07-15")},
			Stages: []CatalogueStage{stage("Stage 1", day("2023-07-15"))}, Podium: []PodiumPlace{}},
		{Label: "2024", Span: GameSpan{Games: 2, Players: 3, First: day("2024-07-01"), Last: day("2025-02-24")},
			Stages: []CatalogueStage{stage("Stage 1", day("2024-07-01")), stage("Stage 7", day("2025-02-24"))},
			Podium: []PodiumPlace{{Place: 1, Name: "ann", Finished: day("2025-02-24"), Source: "CGE"}}},
	}
	onlyShape := cmpopts.IgnoreFields(CatalogueSeason{}, "InChampions", "CGE", "Note")
	if diff := cmp.Diff(want, got.Seasons, onlyShape); diff != "" {
		t.Errorf("wimbledon seasons (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"ann", "bob2", "bob3", "bob4"}, got.Players); diff != "" {
		t.Errorf("wimbledon players (-want +got):\n%s", diff)
	}
}

func TestCatalogueTournamentSaysWhatItCallsItsSeasonsAndStages(t *testing.T) {
	pool := migrated(t)
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Slow Burn S3 - group 1 game 1":           day("2024-07-01"),
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
	})
	r := NewRatingsReader(asReader(t, pool))
	for _, tt := range []struct{ slug, season, stage string }{
		{"slow_burn", "Edition", "Season"},
		{"wimbledon", "", ""},
	} {
		t.Run(tt.slug, func(t *testing.T) {
			withSpans(t, pool, run)
			got, err := r.CatalogueTournament(context.Background(), run, tt.slug)
			if err != nil {
				t.Fatalf("CatalogueTournament(%q): %v", tt.slug, err)
			}
			if got.SeasonName != tt.season || got.StageName != tt.stage {
				t.Errorf("names = %q, %q, want %q, %q", got.SeasonName, got.StageName, tt.season, tt.stage)
			}
		})
	}
}

func TestCatalogueTournamentRefusesATournamentWithoutGames(t *testing.T) {
	pool := migrated(t)
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02")})
	r := NewRatingsReader(asReader(t, pool))

	for _, slug := range []string{"no_such_cup", "wimbledon"} {
		t.Run(slug, func(t *testing.T) {
			withSpans(t, pool, run)
			if _, err := r.CatalogueTournament(context.Background(), run, slug); !errors.Is(err, ErrTournamentNotFound) {
				t.Errorf("CatalogueTournament(%q) = %v, want ErrTournamentNotFound", slug, err)
			}
		})
	}
}

func TestCatalogueListsOnlyRatedCGETournaments(t *testing.T) {
	pool := migrated(t)
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	addCGETournaments(t, pool,
		tournamentcsv.CGETournament{ID: 4991, Name: "Wimbledon 2024", Status: "finished", Organiser: "an organiser",
			Created: date("2024-06-01"), Finished: date("2025-02-24"), Tournament: "wimbledon", Season: "2024"},
		tournamentcsv.CGETournament{ID: 5373, Name: "Nations Cup", Status: "finished", Finished: date("2025-11-19"),
			Tournament: "nations_cup", Season: "2025"},
		tournamentcsv.CGETournament{ID: 56, Name: "Totally Friendly Tournament for six", Status: "finished",
			Started: date("2020-04-14"), Finished: date("2020-06-27")},
	)

	withSpans(t, pool, run)
	view, err := NewRatingsReader(asReader(t, pool)).Catalogue(context.Background(), run)
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	// The Nations Cup is in no ranking, and the friendly tournament neither rated nor visible: neither is listed.
	want := []CGEEntry{
		{ID: 4991, Name: "Wimbledon 2024", Status: "finished", Organiser: "an organiser", Started: date("2024-06-01"), Finished: date("2025-02-24"), Tournament: "wimbledon", Rated: true},
	}
	if diff := cmp.Diff(want, view.CGE); diff != "" {
		t.Errorf("CGE tournaments (-want +got):\n%s", diff)
	}
}

func TestCatalogueShowsAnUnratedTournamentOnlyWhenMadeVisible(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1":        day("2024-01-02"),
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
	})
	setTournament(t, pool, "wimbledon", TournamentSettings{Rated: ptr(false)})
	addCGETournaments(t, pool,
		tournamentcsv.CGETournament{ID: 56, Name: "Totally Friendly Tournament for six", Status: "finished", Finished: date("2020-06-27")},
		tournamentcsv.CGETournament{ID: 57, Name: "4 players, 3 opponents", Status: "finished", Finished: date("2020-04-23")},
	)
	showCGETournament(t, pool, 57)
	r := NewRatingsReader(asReader(t, pool))
	listed := func() []string {
		t.Helper()
		withSpans(t, pool, run)
		view, err := r.Catalogue(ctx, run)
		if err != nil {
			t.Fatalf("Catalogue: %v", err)
		}
		var got []string
		for _, e := range view.Tournaments {
			got = append(got, e.Slug)
		}
		for _, c := range view.CGE {
			got = append(got, fmt.Sprint(c.ID))
		}
		return got
	}

	if got, want := listed(), []string{"intermezzo", "57"}; !slices.Equal(got, want) {
		t.Errorf("listed = %q, want %q: not unrated wimbledon or CGE 56", got, want)
	}
	withSpans(t, pool, run)
	if _, err := r.CatalogueTournament(ctx, run, "wimbledon"); !errors.Is(err, ErrTournamentNotFound) {
		t.Errorf("CatalogueTournament(unrated, hidden wimbledon) = %v, want ErrTournamentNotFound", err)
	}

	setTournament(t, pool, "wimbledon", TournamentSettings{Visible: ptr(true)})
	if got, want := listed(), []string{"intermezzo", "wimbledon", "57"}; !slices.Equal(got, want) {
		t.Errorf("listed once wimbledon is visible = %q, want %q", got, want)
	}
	withSpans(t, pool, run)
	if _, err := r.CatalogueTournament(ctx, run, "wimbledon"); err != nil {
		t.Errorf("CatalogueTournament(visible wimbledon) = %v, want no error", err)
	}
}

func TestCatalogueCountsOnlyPlayedGames(t *testing.T) {
	pool := migrated(t)
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02"),
		"Intermezzo S24 - Bronze 1 game 2": day("2024-01-09"),
	})
	setGameState(t, pool, "Intermezzo S24 - Bronze 1 game 2", "never_played")

	withSpans(t, pool, run)
	view, err := NewRatingsReader(asReader(t, pool)).Catalogue(context.Background(), run)
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	want := GameSpan{Games: 1, Players: 2, First: day("2024-01-02"), Last: day("2024-01-02")}
	if diff := cmp.Diff(want, view.Tournaments[0].Span); diff != "" {
		t.Errorf("intermezzo, the never-played game not counted (-want +got):\n%s", diff)
	}
}

func TestCatalogueTournamentGivesLeagueChampions(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Grandmaster game 1": day("2024-01-02"),
		"Intermezzo S25 - Grandmaster game 1": day("2024-05-01"),
	})
	if _, err := pool.Exec(ctx, `INSERT INTO tournament_standings (run_id, series, season, division, group_label, player_name, place, points, games, placings)
		VALUES ($1, 'intermezzo', 24, 'Grandmaster', 'Grandmaster', 'ann', 1, 20, 6, '{}'), ($1, 'intermezzo', 24, 'Grandmaster', 'Grandmaster', 'bob', 2, 12, 6, '{}'),
		       ($1, 'intermezzo', 24, 'Gold', 'Gold 1', 'cat', 1, 25, 6, '{}'), ($1, 'intermezzo', 25, 'Grandmaster', 'Grandmaster', 'ann', 1, 18, 6, '{}')`, run); err != nil {
		t.Fatalf("adding standings: %v", err)
	}
	addPlacements(t, pool, tournamentcsv.Placement{Series: "intermezzo", Season: 25, Division: "Grandmaster", Player: "dan",
		Place: 1, Points: ptr(21.0), Reason: "the sheet differs", Source: "the organisers' sheet"})

	withSpans(t, pool, run)
	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(ctx, run, "intermezzo")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	// The top division's standings give the podium; a recorded placement replaces a season's derived one.
	want := [][]PodiumPlace{
		{{Place: 1, Name: "ann", Finished: day("2024-01-02"), Source: "Grandmaster standings"}, {Place: 2, Name: "bob", Finished: day("2024-01-02"), Source: "Grandmaster standings"}},
		{{Place: 1, Name: "dan", Finished: day("2024-05-01"), Source: "the organisers' sheet"}},
	}
	if diff := cmp.Diff(want, podiums(got)); diff != "" {
		t.Errorf("intermezzo podiums (-want +got):\n%s", diff)
	}
}

func TestCatalogueTournamentLinksEachSeasonToCGE(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1":        day("2024-01-02"),
		"Intermezzo S25 - Bronze 1 game 1":        day("2024-05-01"),
		"Intermezzo S26 - Bronze 1 game 1":        day("2024-09-01"),
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
		"Wimbledon 2024 Stage 7 - Final game 1":   day("2025-02-24"),
	})
	if _, err := pool.Exec(ctx, `UPDATE groups gr SET cge_tournament = 19, cge_stage_number = substr(g.name, 13, 2)::int - 5 FROM games g
		 WHERE g.group_id = gr.group_id AND g.name LIKE 'Intermezzo S2_ -%' AND g.name NOT LIKE 'Intermezzo S26%';
		UPDATE groups gr SET cge_tournament = 4991, cge_stage_number = 1 FROM games g WHERE g.group_id = gr.group_id AND g.name LIKE 'Wimbledon%Stage 1%';
		UPDATE groups gr SET cge_tournament = 4991, cge_stage_number = 7 FROM games g WHERE g.group_id = gr.group_id AND g.name LIKE 'Wimbledon%Stage 7%'`); err != nil {
		t.Fatalf("linking games to CGE: %v", err)
	}
	r := NewRatingsReader(asReader(t, pool))

	// A CGE tournament that holds several of our seasons links each to its own CGE season; one that holds one season links whole.
	for slug, want := range map[string][][]CGELink{
		"intermezzo": {{{Tournament: 19, Season: 19}}, {{Tournament: 19, Season: 20}}, {}},
		"wimbledon":  {{{Tournament: 4991}}},
	} {
		t.Run(slug, func(t *testing.T) {
			withSpans(t, pool, run)
			got, err := r.CatalogueTournament(ctx, run, slug)
			if err != nil {
				t.Fatalf("CatalogueTournament: %v", err)
			}
			if diff := cmp.Diff(want, cgeLinks(got)); diff != "" {
				t.Errorf("CGE links by season (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCatalogueTournamentFindsAPlacementForALabelledSeason(t *testing.T) {
	pool := migrated(t)
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Worlds 1 stage 8": day("2022-07-28")})
	addPlacements(t, pool, tournamentcsv.Placement{Series: "worlds", Season: 2022, Division: "Final", Player: "Ember_Wolf",
		Place: 1, Reason: "CGE final standings", Source: "CGE /tournaments/detail/3391/8"})

	withSpans(t, pool, run)
	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(context.Background(), run, "worlds")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	if s := got.Seasons[0]; s.Label != "2022 (Age A)" {
		t.Errorf("season = %q, want 2022 (Age A)", s.Label)
	}
	want := []PodiumPlace{{Place: 1, Name: "Ember_Wolf", Finished: day("2022-07-28"), Source: "CGE /tournaments/detail/3391/8"}}
	if diff := cmp.Diff(want, got.Seasons[0].Podium); diff != "" {
		t.Errorf("2022 podium (-want +got):\n%s", diff)
	}
}

func TestCatalogueTournamentGivesATeamPodiumFromItsTopDivision(t *testing.T) {
	pool := migrated(t)
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"NL_s10": day("2024-09-15")})
	place := func(division, player string, points float64) tournamentcsv.Placement {
		return tournamentcsv.Placement{Series: "nations_league", Season: 10, Division: division, Player: player,
			Place: 1, Points: ptr(points), Reason: "r", Source: "s"}
	}
	addPlacements(t, pool, place("Apple", "Dahlia", 36), place("Apple", "Drover", 36), place("Banana", "ann", 40))

	withSpans(t, pool, run)
	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(context.Background(), run, "nations_league")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	if s := got.Seasons[0]; s.Label != "10" {
		t.Errorf("season = %q, want 10", s.Label)
	}
	want := []PodiumPlace{
		{Place: 1, Name: "Dahlia", Finished: day("2024-09-15"), Source: "s"},
		{Place: 1, Name: "Drover", Finished: day("2024-09-15"), Source: "s"},
	}
	if diff := cmp.Diff(want, got.Seasons[0].Podium); diff != "" {
		t.Errorf("season 10 podium (-want +got):\n%s", diff)
	}
}

func TestReplaceCGEStagesLinksSeasonsWithoutMatchedGames(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1":        day("2024-01-02"),
		"Intermezzo S25 - Bronze 1 game 1":        day("2024-05-01"),
		"Wimbledon 2024 Stage 7 - Final game 1":   day("2025-02-24"),
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
	})
	old := []tournamentcsv.CGEStage{{Tournament: "intermezzo", Season: "25", CGETournament: 19, CGESeason: 99}}
	if _, err := ReplaceCGEStages(ctx, pool, old); err != nil {
		t.Fatalf("ReplaceCGEStages(old): %v", err)
	}
	got, err := ReplaceCGEStages(ctx, pool, []tournamentcsv.CGEStage{
		{Tournament: "intermezzo", Season: "24", CGETournament: 19, CGESeason: 19},
		{Tournament: "intermezzo", Season: "25", CGETournament: 19, CGESeason: 20},
		{Tournament: "wimbledon", Season: "2024", Stage: "Stage 1", CGETournament: 4991, CGESeason: 1},
		{Tournament: "wimbledon", Season: "2024", Stage: "Stage 7", CGETournament: 4991, CGESeason: 7},
		{Tournament: "intermezzo", Season: "26", CGETournament: 19, CGESeason: 21},
	})
	if err != nil {
		t.Fatalf("ReplaceCGEStages: %v", err)
	}
	if want := (FactsReplaced{Written: 4, Unknown: 1}); got != want {
		t.Errorf("ReplaceCGEStages = %+v, want %+v: one for a stage we lack", got, want)
	}
	r := NewRatingsReader(asReader(t, pool))
	for slug, want := range map[string][][]CGELink{
		"intermezzo": {{{Tournament: 19, Season: 19}}, {{Tournament: 19, Season: 20}}},
		"wimbledon":  {{{Tournament: 4991}}},
	} {
		t.Run(slug, func(t *testing.T) {
			withSpans(t, pool, run)
			cat, err := r.CatalogueTournament(ctx, run, slug)
			if err != nil {
				t.Fatalf("CatalogueTournament: %v", err)
			}
			if diff := cmp.Diff(want, cgeLinks(cat)); diff != "" {
				t.Errorf("CGE links by season (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCatalogueTournamentGivesALaddersTopTierWins(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Mercurial Season 20 - 3-Neon 2 game 4":    day("2024-03-01"),
		"Intermezzo S24 - Grandmaster game 1":      day("2024-01-02"),
		"Mercurial Season 21 - 4-Beryllium game 1": day("2024-04-01"),
	})
	addPlayers(t, pool, "cat")
	src := "the organisers' Hall of Fame"
	got, err := ReplaceLadderWins(ctx, pool, []tournamentcsv.LadderWins{
		{Ladder: "mercurial_ladder", Player: "ann", Wins: 1, Source: src},
		{Ladder: "mercurial_ladder", Player: "cat", Wins: 3.5, Source: src},
		{Ladder: "mercurial_ladder", Player: "nobody", Wins: 2, Source: src},
		{Ladder: "intermezzo", Player: "cat", Wins: 1, Source: src},
		{Ladder: "snail_ladder", Player: "cat", Wins: 1, Source: src},
	})
	if err != nil {
		t.Fatalf("ReplaceLadderWins: %v", err)
	}
	if want := (FactsReplaced{Written: 2, Unknown: 3}); got != want {
		t.Errorf("ReplaceLadderWins = %+v, want %+v: 3 not a player or not a ladder", got, want)
	}
	r := NewRatingsReader(asReader(t, pool))
	withSpans(t, pool, run)
	ladder, err := r.CatalogueTournament(ctx, run, "mercurial_ladder")
	if err != nil {
		t.Fatalf("CatalogueTournament(mercurial_ladder): %v", err)
	}
	if !ladder.Ladder {
		t.Errorf("mercurial_ladder is not a ladder, want one")
	}
	want := []LadderWin{{Name: "cat", Wins: 3.5, Source: src}, {Name: "ann", Wins: 1, Source: src}}
	if diff := cmp.Diff(want, ladder.TopTierWins); diff != "" {
		t.Errorf("mercurial_ladder top-tier wins (-want +got):\n%s", diff)
	}
	withSpans(t, pool, run)
	league, err := r.CatalogueTournament(ctx, run, "intermezzo")
	if err != nil {
		t.Fatalf("CatalogueTournament(intermezzo): %v", err)
	}
	if league.Ladder || len(league.TopTierWins) != 0 {
		t.Errorf("intermezzo: ladder %v, wins %+v; want neither", league.Ladder, league.TopTierWins)
	}
}

func TestCatalogueTournamentListsItsPlayers(t *testing.T) {
	pool := migrated(t)
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02"),
		"Intermezzo S25 - Bronze 2 game 1": day("2024-05-01"),
	})
	withSpans(t, pool, run)
	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(context.Background(), run, "intermezzo")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	// ann plays both games, each bob one: three players, each once, in order.
	if len(got.Players) != 3 || got.Players[0] != "ann" || !slices.IsSorted(got.Players) {
		t.Errorf("players = %q, want ann and two bobs, each once, in order", got.Players)
	}
}

func TestCatalogueTournamentKeepsAPlayOffApartFromItsYear(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Worlds 2 - stage 2":                            day("2023-03-01"),
		"Worlds 2023 Play Off Stage 1 - group 1 game 1": day("2023-12-01"),
	})
	addPlacements(t, pool, tournamentcsv.Placement{Series: "worlds", Season: 2023, Division: "Final", Player: "ann", Place: 1, Reason: "r", Source: "s"})
	notes := []tournamentcsv.SeasonNote{
		{Tournament: "worlds", Season: "2023 Play Off", Note: "A second chance for the 2023 entrants.", Source: "CGE"},
		{Tournament: "worlds", Season: "2023 Play Off", Stage: "Stage 1", Note: "The play-off's first round.", Source: "CGE"},
	}
	if got, err := ReplaceSeasonNotes(ctx, pool, notes); err != nil || got.Written != 2 {
		t.Fatalf("ReplaceSeasonNotes = %+v, %v; want 2 written", got, err)
	}

	withSpans(t, pool, run)
	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(ctx, run, "worlds")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	if len(got.Seasons) != 2 || got.Seasons[0].Label != "2023" || got.Seasons[1].Label != "2023 Play Off" {
		t.Fatalf("seasons = %+v, want 2023 then its play-off", got.Seasons)
	}
	if len(got.Seasons[0].Podium) != 1 || len(got.Seasons[1].Podium) != 0 {
		t.Errorf("podiums = %+v and %+v, want the 2023 placement on 2023 only", got.Seasons[0].Podium, got.Seasons[1].Podium)
	}
	if got.Seasons[1].Note != notes[0].Note || got.Seasons[0].Note != "" {
		t.Errorf("notes = %q and %q, want the play-off's only", got.Seasons[0].Note, got.Seasons[1].Note)
	}
	if st := got.Seasons[1].Stages; len(st) != 1 || st[0].Note != notes[1].Note {
		t.Errorf("play-off stages = %+v, want stage 1 with its note", st)
	}
}

func TestCatalogueTournamentGivesEachEditionsRulesFromCGE(t *testing.T) {
	pool := migrated(t)
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	addCGETournaments(t, pool,
		tournamentcsv.CGETournament{ID: 4369, Name: "Wimbledon 2023", Status: "finished", Started: date("2023-07-01"), Description: "The first Wimbledon.", Tournament: "wimbledon", Season: "2023"},
		tournamentcsv.CGETournament{ID: 4991, Name: "Wimbledon 2024", Status: "finished", Started: date("2024-07-01"), Description: "The second edition.", Tournament: "wimbledon", Season: "2024"},
		tournamentcsv.CGETournament{ID: 9999, Name: "Wimbledon 2027", Status: "upcoming", Tournament: "wimbledon", Season: "2027"},
	)
	withSpans(t, pool, run)
	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(context.Background(), run, "wimbledon")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	// Newest edition first; one without a description has no rules to show.
	want := []EditionRules{
		{CGETournament: 4991, Name: "Wimbledon 2024", Description: "The second edition."},
		{CGETournament: 4369, Name: "Wimbledon 2023", Description: "The first Wimbledon."},
	}
	if diff := cmp.Diff(want, got.Rules); diff != "" {
		t.Errorf("wimbledon rules (-want +got):\n%s", diff)
	}
}

func TestCatalogueTournamentListsASeasonWithoutGamesInNumberOrder(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"International S2 g1": day("2017-10-06"),
		"International S3 g1": day("2018-01-06"),
	})
	admin := NewAdminStore(pool)
	if err := admin.AddSeason(ctx, "international", "1"); err != nil {
		t.Fatalf("AddSeason: %v", err)
	}
	if err := admin.SetNote(ctx, "international", "1", "", "Played before the app."); err != nil {
		t.Fatalf("SetNote: %v", err)
	}

	withSpans(t, pool, run)
	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(ctx, run, "international")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	var labels []string
	for _, s := range got.Seasons {
		labels = append(labels, s.Label)
	}
	if want := []string{"1", "2", "3"}; !slices.Equal(labels, want) {
		t.Fatalf("seasons = %q, want %q", labels, want)
	}
	s1 := got.Seasons[0]
	if s1.Span.Games != 0 || !s1.Span.First.IsZero() || s1.Note != "Played before the app." {
		t.Errorf("season 1 = %+v, want no games and its note", s1)
	}
}

func TestCatalogueTournamentShowsAnAdminPodiumOverAnyOther(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Worlds 1 stage 8": day("2022-07-28")})
	addPlacements(t, pool, tournamentcsv.Placement{Series: "worlds", Season: 2022, Division: "Final", Player: "Ember_Wolf",
		Place: 1, Reason: "CGE final standings", Source: "CGE /tournaments/detail/3391/8"})
	podium := []PodiumEntry{{Place: 1, Name: "Linnet"}, {Place: 2, Name: "Ember_Wolf"}}
	if err := NewAdminStore(pool).SetPodium(ctx, "worlds", "2022 (Age A)", podium, "The organiser's ruling"); err != nil {
		t.Fatalf("SetPodium: %v", err)
	}

	withSpans(t, pool, run)
	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(ctx, run, "worlds")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	want := []PodiumPlace{
		{Place: 1, Name: "Linnet", Finished: day("2022-07-28"), Source: "Set in admin: The organiser's ruling"},
		{Place: 2, Name: "Ember_Wolf", Finished: day("2022-07-28"), Source: "Set in admin"},
	}
	if diff := cmp.Diff(want, got.Seasons[0].Podium); diff != "" {
		t.Errorf("2022 podium (-want +got):\n%s", diff)
	}
}

func TestCatalogueTournamentGivesTheRegulationsWrittenInAdmin(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	r := NewRatingsReader(asReader(t, pool))

	withSpans(t, pool, run)
	got, err := r.CatalogueTournament(ctx, run, "wimbledon")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	if got.Regulations != "" {
		t.Errorf("regulations before any = %q, want none", got.Regulations)
	}
	if err := NewAdminStore(pool).SetRegulations(ctx, "wimbledon", "Best of three."); err != nil {
		t.Fatalf("SetRegulations: %v", err)
	}
	withSpans(t, pool, run)
	if got, err = r.CatalogueTournament(ctx, run, "wimbledon"); err != nil || got.Regulations != "Best of three." {
		t.Errorf("regulations = %q, %v; want admin's", got.Regulations, err)
	}
}

func TestCatalogueGivesEachTournamentsCard(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	want := Card{Icon: "🎾", Tagline: "Grass-court slam", Summary: "Played each July."}
	if err := NewAdminStore(pool).SetCard(ctx, "wimbledon", want); err != nil {
		t.Fatalf("SetCard: %v", err)
	}
	r := NewRatingsReader(asReader(t, pool))

	withSpans(t, pool, run)
	view, err := r.Catalogue(ctx, run)
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	i := slices.IndexFunc(view.Tournaments, func(e CatalogueEntry) bool { return e.Slug == "wimbledon" })
	if i < 0 {
		t.Fatalf("no wimbledon in %+v", view.Tournaments)
	}
	if got := view.Tournaments[i].Card; got != want {
		t.Errorf("card in the list = %+v, want %+v", got, want)
	}
	withSpans(t, pool, run)
	one, err := r.CatalogueTournament(ctx, run, "wimbledon")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	if one.Card != want {
		t.Errorf("card on the page = %+v, want %+v", one.Card, want)
	}
}

func TestCatalogueTournamentSaysWhetherItIsRated(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	r := NewRatingsReader(asReader(t, pool))
	withSpans(t, pool, run)
	if got, err := r.CatalogueTournament(ctx, run, "wimbledon"); err != nil || !got.Rated {
		t.Errorf("rated = %v, %v; want rated", got.Rated, err)
	}
	setTournament(t, pool, "wimbledon", TournamentSettings{Rated: ptr(false), Visible: ptr(true)})
	withSpans(t, pool, run)
	if got, err := r.CatalogueTournament(ctx, run, "wimbledon"); err != nil || got.Rated {
		t.Errorf("rated after leaving the ranking = %v, %v; want not rated", got.Rated, err)
	}
}

func TestCatalogueTournamentGivesItsHighestWinningScores(t *testing.T) {
	pool := migrated(t)
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
		"Wimbledon 2024 Stage 1 - group 1 game 2": day("2024-07-02"),
		"Wimbledon 2024 Stage 1 - group 1 game 3": day("2024-07-03"),
	})
	setSeats(t, pool, "%game 1", Seat{"ann", 120}, Seat{"bob", 80})
	setSeats(t, pool, "%game 2", Seat{"ann", 90}, Seat{"cat", 150})
	setSeats(t, pool, "%game 3", Seat{"dan", 999}, Seat{"eve", 0})
	setGameState(t, pool, "%game 3", "never_played")

	withSpans(t, pool, run)
	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(context.Background(), run, "wimbledon")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	want := []GameRecord{
		{Name: "cat", Score: 150, Opponents: []Seat{{Name: "ann", Score: 90}}, Tournament: "Wimbledon 2024 Stage 1 - group 1 game 2", Date: day("2024-07-02")},
		{Name: "ann", Score: 120, Opponents: []Seat{{Name: "bob", Score: 80}}, Tournament: "Wimbledon 2024 Stage 1 - group 1 game 1", Date: day("2024-07-01")},
	}
	if diff := cmp.Diff(want, got.GameRecords); diff != "" {
		t.Errorf("game records, a game never played left out (-want +got):\n%s", diff)
	}
}

func TestCatalogueTournamentMasksDeletedAndBannedSeats(t *testing.T) {
	pool := migrated(t)
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
	})
	setSeats(t, pool, "%game 1", Seat{"ann", 120}, Seat{"bob", 80})
	ctx := context.Background()
	for _, q := range []string{
		"INSERT INTO player_accounts (cge_user, player_id, name, deleted) SELECT 1, player_id, 'ann', true FROM players WHERE name = 'ann'",
		"INSERT INTO player_accounts (cge_user, player_id, name, banned) SELECT 2, player_id, 'bob', true FROM players WHERE name = 'bob'",
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("setting up player account: %v", err)
		}
	}

	withSpans(t, pool, run)
	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(ctx, run, "wimbledon")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	want := []GameRecord{
		{Name: "Deleted player", Score: 120, Opponents: []Seat{{Name: "Deleted player", Score: 80}}, Tournament: "Wimbledon 2024 Stage 1 - group 1 game 1", Date: day("2024-07-01")},
	}
	if diff := cmp.Diff(want, got.GameRecords); diff != "" {
		t.Errorf("masked game records (-want +got):\n%s", diff)
	}
	if slices.Contains(got.Players, "ann") || slices.Contains(got.Players, "bob") {
		t.Errorf("got.Players = %v, want deleted/banned players excluded from suggestions", got.Players)
	}
}

func TestCatalogueTournamentTakesItsKindFromTheDatabase(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	r := NewRatingsReader(asReader(t, pool))
	withSpans(t, pool, run)
	if got, err := r.CatalogueTournament(ctx, run, "wimbledon"); err != nil || got.Ladder {
		t.Errorf("wimbledon as a cup: ladder = %v, %v; want a cup", got.Ladder, err)
	}
	setTournament(t, pool, "wimbledon", TournamentSettings{Kind: "ladder"})
	withSpans(t, pool, run)
	if got, err := r.CatalogueTournament(ctx, run, "wimbledon"); err != nil || !got.Ladder {
		t.Errorf("wimbledon made a ladder: ladder = %v, %v; want a ladder", got.Ladder, err)
	}
}

func TestRecentPodiumsGivesEachTournamentsLatestSeasonWithAPodium(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Wimbledon 2023 stage 1":                  day("2023-07-15"),
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
		"Intermezzo S23 - Grandmaster game 1":     day("2023-09-02"),
		"Intermezzo S24 - Grandmaster game 1":     day("2024-01-02"),
		"International S2 g1":                     day("2017-10-06"),
	})
	if _, err := ReplacePodiums(ctx, pool, []tournamentcsv.Podium{
		{Cup: "wimbledon", Season: "2023", Place: 1, Player: "ann", Source: "CGE", Finished: day("2023-07-20")},
		{Cup: "international", Season: "3", Place: 1, Player: "ann", Source: "CGE", Finished: day("2018-02-01")},
	}); err != nil {
		t.Fatalf("ReplacePodiums: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tournament_standings (run_id, series, season, division, group_label, player_name, place, points, games, placings)
		VALUES ($1, 'intermezzo', 23, 'Grandmaster', 'Grandmaster', 'ann', 2, 12, 6, '{}'),
		       ($1, 'intermezzo', 24, 'Grandmaster', 'Grandmaster', 'ann', 1, 20, 6, '{}')`, run); err != nil {
		t.Fatalf("adding standings: %v", err)
	}
	if err := NewAdminStore(pool).AddSeason(ctx, "international", "3"); err != nil {
		t.Fatalf("AddSeason: %v", err)
	}

	withSpans(t, pool, run)
	got, err := NewRatingsReader(asReader(t, pool)).RecentPodiums(ctx, run, 12, 8)
	if err != nil {
		t.Fatalf("RecentPodiums: %v", err)
	}
	want := []string{
		"intermezzo 24 2024-01-02 ann",
		"wimbledon 2023 2023-07-15 ann",
		"international 3 2018-02-01 ann",
	}
	if diff := cmp.Diff(want, podiumLines(got)); diff != "" {
		t.Errorf("recent podiums, finished last first: the league's standings, wimbledon's 2023 as 2024 has none, "+
			"and a season without games finished when its podium was (-want +got):\n%s", diff)
	}
}

func TestRecentPodiumsLooksAtTheMostRecentlyPlayedTournamentsOnly(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Wimbledon 2023 stage 1":                  day("2023-07-15"),
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
		"Intermezzo S24 - Grandmaster game 1":     day("2024-01-02"),
	})
	if _, err := ReplacePodiums(ctx, pool, []tournamentcsv.Podium{
		{Cup: "wimbledon", Season: "2023", Place: 1, Player: "ann", Source: "CGE", Finished: day("2023-07-20")},
		{Cup: "intermezzo", Season: "24", Place: 1, Player: "ann", Source: "CGE", Finished: day("2024-01-02")},
	}); err != nil {
		t.Fatalf("ReplacePodiums: %v", err)
	}
	withSpans(t, pool, run)
	r := NewRatingsReader(asReader(t, pool))

	tests := []struct {
		name          string
		played, shown int
		want          []string
	}{
		{"only the most recently played tournament", 1, 8, []string{"wimbledon 2023 2023-07-15 ann"}},
		{"only the latest finished podium", 2, 1, []string{"intermezzo 24 2024-01-02 ann"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.RecentPodiums(ctx, run, tt.played, tt.shown)
			if err != nil {
				t.Fatalf("RecentPodiums(%d, %d): %v", tt.played, tt.shown, err)
			}
			if diff := cmp.Diff(tt.want, podiumLines(got)); diff != "" {
				t.Errorf("RecentPodiums(%d, %d) (-want +got):\n%s", tt.played, tt.shown, diff)
			}
		})
	}
}

// podiumLines gives each recent podium as "slug season finished names".
func podiumLines(ps []RecentPodium) []string {
	var got []string
	for _, p := range ps {
		var names []string
		for _, place := range p.Podium {
			names = append(names, place.Name)
		}
		got = append(got, fmt.Sprintf("%s %s %s %s", p.Slug, p.Season, p.Finished.Format(time.DateOnly), strings.Join(names, ",")))
	}
	return got
}

// podiums returns each season's podium, in order.
func podiums(c CatalogueTournament) [][]PodiumPlace {
	var got [][]PodiumPlace
	for _, s := range c.Seasons {
		got = append(got, s.Podium)
	}
	return got
}

// cgeLinks returns each season's CGE links, in order.
func cgeLinks(c CatalogueTournament) [][]CGELink {
	var got [][]CGELink
	for _, s := range c.Seasons {
		got = append(got, s.CGE)
	}
	return got
}
