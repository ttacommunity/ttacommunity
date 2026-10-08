//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"ttacommunity/backend/internal/tournamentcsv"
)

func TestCatalogueListsEachTournamentWithGames(t *testing.T) {
	pool := migrated(t)
	run := publishedRun(t, pool)
	if _, err := pool.Exec(context.Background(), "DELETE FROM tournament_cards"); err != nil {
		t.Fatal(err)
	}
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1":        day("2024-01-02"),
		"Intermezzo S24 - Bronze 1 game 2":        day("2024-02-03"),
		"Intermezzo S25 - Wood 1 game 1":          day("2024-05-01"),
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
		"Friendly game":                           day("2024-08-01"),
	})

	view, err := NewRatingsReader(asReader(t, pool)).Catalogue(context.Background(), run)
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	got := view.Tournaments
	// Each game here has ann and one other player of its own; the friendly game is placed nowhere.
	want := []CatalogueEntry{
		{Slug: "intermezzo", Name: "Intermezzo", Seasons: 2, Rankings: []string{"community"}, Span: GameSpan{Games: 3, Players: 4, First: day("2024-01-02"), Last: day("2024-05-01")}},
		{Slug: "wimbledon", Name: "Wimbledon", Seasons: 1, Rankings: []string{"community"}, Span: GameSpan{Games: 1, Players: 2, First: day("2024-07-01"), Last: day("2024-07-01")}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Catalogue = %+v, want %+v", got, want)
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
	if _, err := pool.Exec(ctx, `INSERT INTO tournament_cup_podiums (cup, season, final, place, player_name, source, finished)
		VALUES ('wimbledon', '2024', '', 1, 'ann', 'CGE', '2025-02-24')`); err != nil {
		t.Fatalf("adding a podium: %v", err)
	}

	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(ctx, run, "wimbledon")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	stage := func(label string, d time.Time) CatalogueStage {
		return CatalogueStage{Label: label, Span: GameSpan{Games: 1, Players: 2, First: d, Last: d}}
	}
	type season struct {
		Label  string
		Span   GameSpan
		Stages []CatalogueStage
		Podium []PodiumPlace
	}
	want := []season{
		{Label: "2023", Span: GameSpan{Games: 1, Players: 2, First: day("2023-07-15"), Last: day("2023-07-15")},
			Stages: []CatalogueStage{stage("Stage 1", day("2023-07-15"))}, Podium: []PodiumPlace{}},
		{Label: "2024", Span: GameSpan{Games: 2, Players: 3, First: day("2024-07-01"), Last: day("2025-02-24")},
			Stages: []CatalogueStage{stage("Stage 1", day("2024-07-01")), stage("Stage 7", day("2025-02-24"))},
			Podium: []PodiumPlace{{Place: 1, Name: "ann", Final: "", Finished: day("2025-02-24"), Source: "CGE"}}},
	}
	seasons := make([]season, len(got.Seasons))
	for i, s := range got.Seasons {
		seasons[i] = season{s.Label, s.Span, s.Stages, s.Podium}
	}
	if got.Slug != "wimbledon" || got.Name != "Wimbledon" || !reflect.DeepEqual(seasons, want) {
		t.Errorf("CatalogueTournament %s %q seasons = %+v, want %+v", got.Slug, got.Name, seasons, want)
	}
	if !reflect.DeepEqual(got.Players, []string{"ann", "bob2", "bob3", "bob4"}) {
		t.Errorf("players = %v", got.Players)
	}
}

func TestCatalogueTournamentSaysWhatItCallsItsSeasonsAndStages(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
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
		got, err := r.CatalogueTournament(ctx, run, tt.slug)
		if err != nil {
			t.Fatalf("CatalogueTournament(%q): %v", tt.slug, err)
		}
		if got.SeasonName != tt.season || got.StageName != tt.stage {
			t.Errorf("%s names = %q, %q, want %q, %q", tt.slug, got.SeasonName, got.StageName, tt.season, tt.stage)
		}
	}
}

func TestCatalogueTournamentRefusesATournamentWithoutGames(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02")})
	r := NewRatingsReader(asReader(t, pool))

	for _, slug := range []string{"no_such_cup", "wimbledon"} {
		if _, err := r.CatalogueTournament(ctx, run, slug); !errors.Is(err, ErrTournamentNotFound) {
			t.Errorf("CatalogueTournament(%q): %v, want ErrTournamentNotFound", slug, err)
		}
	}
}

func TestCatalogueListsOnlyRatedCGETournaments(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	for _, q := range []string{
		`INSERT INTO series (slug, name) VALUES ('nations_cup', 'Nations Cup')`,
		`INSERT INTO tournaments (series_id, label) SELECT series_id, '2025' FROM series WHERE slug = 'nations_cup'`,
		`INSERT INTO cge_tournaments (cge_tournament, name, status, organiser, created, started, finished, series_id, tournament_id) VALUES
			(4991, 'Wimbledon 2024', 'finished', 'an organiser', '2024-06-01', NULL, '2025-02-24', (SELECT series_id FROM series WHERE slug = 'wimbledon'),
			 (SELECT tournament_id FROM tournaments WHERE label = '2024')),
			(5373, 'Nations Cup', 'finished', '', NULL, NULL, '2025-11-19', (SELECT series_id FROM series WHERE slug = 'nations_cup'),
			 (SELECT tournament_id FROM tournaments WHERE label = '2025')),
			(56, 'Totally Friendly Tournament for six', 'finished', '', NULL, '2020-04-14', '2020-06-27', NULL, NULL)`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	view, err := NewRatingsReader(asReader(t, pool)).Catalogue(ctx, run)
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	ptr := func(s string) *time.Time { d := day(s); return &d }
	// The friendly tournament is neither rated nor visible, so it is not listed.
	want := []CGEEntry{
		{ID: 4991, Name: "Wimbledon 2024", Status: "finished", Organiser: "an organiser", Started: ptr("2024-06-01"), Finished: ptr("2025-02-24"), Tournament: "wimbledon", Rated: true},
	}
	if !reflect.DeepEqual(view.CGE, want) {
		t.Errorf("CGE = %+v\nwant %+v", view.CGE, want)
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
	for _, q := range []string{
		`DELETE FROM ranking_series WHERE series_id = (SELECT series_id FROM series WHERE slug = 'wimbledon')`,
		`INSERT INTO cge_tournaments (cge_tournament, name, status, finished, visible) VALUES
			(56, 'Totally Friendly Tournament for six', 'finished', '2020-06-27', false),
			(57, '4 players, 3 opponents', 'finished', '2020-04-23', true)`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	r := NewRatingsReader(asReader(t, pool))
	slugs := func() []string {
		t.Helper()
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

	if got := slugs(); !slices.Equal(got, []string{"intermezzo", "57"}) {
		t.Errorf("listed = %q, want intermezzo (rated) and CGE 57 (visible), not unrated wimbledon or CGE 56", got)
	}
	if _, err := r.CatalogueTournament(ctx, run, "wimbledon"); !errors.Is(err, ErrTournamentNotFound) {
		t.Errorf("CatalogueTournament(unrated, hidden wimbledon): %v, want ErrTournamentNotFound", err)
	}

	if _, err := pool.Exec(ctx, "UPDATE series SET visible = true WHERE slug = 'wimbledon'"); err != nil {
		t.Fatalf("making wimbledon visible: %v", err)
	}
	if got := slugs(); !slices.Equal(got, []string{"intermezzo", "wimbledon", "57"}) {
		t.Errorf("listed = %q, want wimbledon too once visible", got)
	}
	if _, err := r.CatalogueTournament(ctx, run, "wimbledon"); err != nil {
		t.Errorf("CatalogueTournament(visible wimbledon): %v", err)
	}
}

func TestCatalogueCountsOnlyPlayedGames(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02"),
		"Intermezzo S24 - Bronze 1 game 2": day("2024-01-09"),
	})
	if _, err := pool.Exec(ctx, "UPDATE games SET state = 'never_played' WHERE name = 'Intermezzo S24 - Bronze 1 game 2'"); err != nil {
		t.Fatalf("marking a game never played: %v", err)
	}

	view, err := NewRatingsReader(asReader(t, pool)).Catalogue(ctx, run)
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	if got := view.Tournaments[0].Span; got.Games != 1 || got.Players != 2 || !got.Last.Equal(day("2024-01-02")) {
		t.Errorf("intermezzo = %+v, want 1 game (the never-played one not counted), last on 2024-01-02", got)
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
	if _, err := pool.Exec(ctx, `INSERT INTO tournament_official_placements (series, season, division, player_name, place, points, reason, source)
		VALUES ('intermezzo', 25, 'Grandmaster', 'dan', 1, 21, 'the sheet differs', 'the organisers'' sheet')`); err != nil {
		t.Fatalf("adding a placement: %v", err)
	}

	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(ctx, run, "intermezzo")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	// The top division's standings give the podium; a recorded placement replaces a season's derived one.
	want := [][]PodiumPlace{
		{{Place: 1, Name: "ann", Finished: day("2024-01-02"), Source: "Grandmaster standings"}, {Place: 2, Name: "bob", Finished: day("2024-01-02"), Source: "Grandmaster standings"}},
		{{Place: 1, Name: "dan", Finished: day("2024-05-01"), Source: "the organisers' sheet"}},
	}
	for i, s := range got.Seasons {
		if !reflect.DeepEqual(s.Podium, want[i]) {
			t.Errorf("season %s podium = %+v, want %+v", s.Label, s.Podium, want[i])
		}
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
		got, err := r.CatalogueTournament(ctx, run, slug)
		if err != nil {
			t.Fatalf("CatalogueTournament(%s): %v", slug, err)
		}
		for i, s := range got.Seasons {
			if !reflect.DeepEqual(s.CGE, want[i]) {
				t.Errorf("%s season %s CGE = %+v, want %+v", slug, s.Label, s.CGE, want[i])
			}
		}
	}
}

func TestCatalogueTournamentFindsAPlacementForALabelledSeason(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Worlds 1 stage 8": day("2022-07-28")})
	if _, err := pool.Exec(ctx, `INSERT INTO tournament_official_placements (series, season, division, player_name, place, points, reason, source)
		VALUES ('worlds', 2022, 'Final', 'Ender_Wiggin', 1, NULL, 'CGE final standings', 'CGE /tournaments/detail/3391/8')`); err != nil {
		t.Fatalf("adding a placement: %v", err)
	}

	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(ctx, run, "worlds")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	want := []PodiumPlace{{Place: 1, Name: "Ender_Wiggin", Finished: day("2022-07-28"), Source: "CGE /tournaments/detail/3391/8"}}
	if s := got.Seasons[0]; s.Label != "2022 (Age A)" || !reflect.DeepEqual(s.Podium, want) {
		t.Errorf("season %q podium = %+v, want %+v", s.Label, s.Podium, want)
	}
}

func TestCatalogueTournamentGivesATeamPodiumFromItsTopDivision(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"NL_s10": day("2024-09-15")})
	if _, err := pool.Exec(ctx, `INSERT INTO tournament_official_placements (series, season, division, player_name, place, points, reason, source)
		VALUES ('nations_league', 10, 'Apple', 'DANeo', 1, 36, 'r', 's'), ('nations_league', 10, 'Apple', 'DrRerNate', 1, 36, 'r', 's'),
		       ('nations_league', 10, 'Banana', 'ann', 1, 40, 'r', 's')`); err != nil {
		t.Fatalf("adding placements: %v", err)
	}

	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(ctx, run, "nations_league")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	want := []PodiumPlace{
		{Place: 1, Name: "DANeo", Finished: day("2024-09-15"), Source: "s"},
		{Place: 1, Name: "DrRerNate", Finished: day("2024-09-15"), Source: "s"},
	}
	if s := got.Seasons[0]; s.Label != "10" || !reflect.DeepEqual(s.Podium, want) {
		t.Errorf("season %q podium = %+v, want %+v", s.Label, s.Podium, want)
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
	if got.Written != 4 || got.Unknown != 1 {
		t.Errorf("ReplaceCGEStages = %+v, want 4 written, 1 for a stage we lack", got)
	}
	r := NewRatingsReader(asReader(t, pool))
	for slug, want := range map[string][][]CGELink{
		"intermezzo": {{{Tournament: 19, Season: 19}}, {{Tournament: 19, Season: 20}}},
		"wimbledon":  {{{Tournament: 4991}}},
	} {
		cat, err := r.CatalogueTournament(ctx, run, slug)
		if err != nil {
			t.Fatalf("CatalogueTournament(%s): %v", slug, err)
		}
		for i, s := range cat.Seasons {
			if !reflect.DeepEqual(s.CGE, want[i]) {
				t.Errorf("%s season %s CGE = %+v, want %+v", slug, s.Label, s.CGE, want[i])
			}
		}
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
	if got.Written != 2 || got.Unknown != 3 {
		t.Errorf("ReplaceLadderWins = %+v, want 2 written, and 3 not a player or not a ladder", got)
	}
	r := NewRatingsReader(asReader(t, pool))
	ladder, err := r.CatalogueTournament(ctx, run, "mercurial_ladder")
	if err != nil {
		t.Fatalf("CatalogueTournament(mercurial_ladder): %v", err)
	}
	want := []LadderWin{{Name: "cat", Wins: 3.5, Source: src}, {Name: "ann", Wins: 1, Source: src}}
	if !ladder.Ladder || !reflect.DeepEqual(ladder.TopTierWins, want) {
		t.Errorf("mercurial_ladder: ladder %v, wins %+v; want a ladder with %+v", ladder.Ladder, ladder.TopTierWins, want)
	}
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
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02"),
		"Intermezzo S25 - Bronze 2 game 1": day("2024-05-01"),
	})
	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(ctx, run, "intermezzo")
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
	if _, err := pool.Exec(ctx, `INSERT INTO tournament_official_placements (series, season, division, player_name, place, points, reason, source)
		VALUES ('worlds', 2023, 'Final', 'ann', 1, NULL, 'r', 's')`); err != nil {
		t.Fatalf("adding a placement: %v", err)
	}
	notes := []tournamentcsv.SeasonNote{
		{Tournament: "worlds", Season: "2023 Play Off", Note: "A second chance for the 2023 entrants.", Source: "CGE"},
		{Tournament: "worlds", Season: "2023 Play Off", Stage: "Stage 1", Note: "The play-off's first round.", Source: "CGE"},
	}
	if got, err := ReplaceSeasonNotes(ctx, pool, notes); err != nil || got.Written != 2 {
		t.Fatalf("ReplaceSeasonNotes = %+v, %v; want 2 written", got, err)
	}

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
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	started := func(s string) *time.Time { d := day(s); return &d }
	if _, err := ReplaceCGETournaments(ctx, pool, []tournamentcsv.CGETournament{
		{ID: 4369, Name: "Wimbledon 2023", Status: "finished", Started: started("2023-07-01"), Description: "The first Wimbledon.", Tournament: "wimbledon", Season: "2023"},
		{ID: 4991, Name: "Wimbledon 2024", Status: "finished", Started: started("2024-07-01"), Description: "The second edition.", Tournament: "wimbledon", Season: "2024"},
		{ID: 9999, Name: "Wimbledon 2027", Status: "upcoming", Description: "", Tournament: "wimbledon", Season: "2027"},
	}); err != nil {
		t.Fatalf("ReplaceCGETournaments: %v", err)
	}
	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(ctx, run, "wimbledon")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	// Newest edition first; one without a description has no rules to show.
	want := []EditionRules{
		{CGETournament: 4991, Name: "Wimbledon 2024", Description: "The second edition."},
		{CGETournament: 4369, Name: "Wimbledon 2023", Description: "The first Wimbledon."},
	}
	if !reflect.DeepEqual(got.Rules, want) {
		t.Errorf("rules = %+v, want %+v", got.Rules, want)
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
	if _, err := pool.Exec(ctx, `INSERT INTO tournaments (series_id, label)
		SELECT series_id, '1' FROM series WHERE slug = 'international'`); err != nil {
		t.Fatalf("adding season 1: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tournament_season_notes (tournament, season, stage, note, source)
		VALUES ('international', '1', '', 'Played before the app.', 's')`); err != nil {
		t.Fatal(err)
	}

	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(ctx, run, "international")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	var labels []string
	for _, s := range got.Seasons {
		labels = append(labels, s.Label)
	}
	if !slices.Equal(labels, []string{"1", "2", "3"}) {
		t.Fatalf("seasons = %v, want 1, 2, 3", labels)
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
	if _, err := pool.Exec(ctx, `INSERT INTO tournament_official_placements (series, season, division, player_name, place, points, reason, source)
		VALUES ('worlds', 2022, 'Final', 'Ender_Wiggin', 1, NULL, 'CGE final standings', 'CGE /tournaments/detail/3391/8')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tournament_podium_overrides (tournament, season, place, player_name, note)
		VALUES ('worlds', '2022 (Age A)', 1, 'LaoHuang', 'The organiser''s ruling'), ('worlds', '2022 (Age A)', 2, 'Ender_Wiggin', '')`); err != nil {
		t.Fatal(err)
	}

	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(ctx, run, "worlds")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	want := []PodiumPlace{
		{Place: 1, Name: "LaoHuang", Finished: day("2022-07-28"), Source: "Set in admin: The organiser's ruling"},
		{Place: 2, Name: "Ender_Wiggin", Finished: day("2022-07-28"), Source: "Set in admin"},
	}
	if s := got.Seasons[0]; !reflect.DeepEqual(s.Podium, want) {
		t.Errorf("season %q podium = %+v, want %+v", s.Label, s.Podium, want)
	}
}

func TestCatalogueTournamentGivesTheRegulationsWrittenInAdmin(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	r := NewRatingsReader(asReader(t, pool))

	got, err := r.CatalogueTournament(ctx, run, "wimbledon")
	if err != nil || got.Regulations != "" {
		t.Fatalf("regulations before any = %q, %v; want none", got.Regulations, err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO tournament_regulations VALUES ('wimbledon', 'Best of three.')"); err != nil {
		t.Fatal(err)
	}
	if got, _ = r.CatalogueTournament(ctx, run, "wimbledon"); got.Regulations != "Best of three." {
		t.Errorf("regulations = %q, want admin's", got.Regulations)
	}
}

func TestCatalogueGivesEachTournamentsCard(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	r := NewRatingsReader(asReader(t, pool))
	card := func() (Card, Card) {
		t.Helper()
		view, err := r.Catalogue(ctx, run)
		if err != nil {
			t.Fatalf("Catalogue: %v", err)
		}
		one, err := r.CatalogueTournament(ctx, run, "wimbledon")
		if err != nil {
			t.Fatalf("CatalogueTournament: %v", err)
		}
		for _, e := range view.Tournaments {
			if e.Slug == "wimbledon" {
				return e.Card, one.Card
			}
		}
		t.Fatalf("no wimbledon in %+v", view.Tournaments)
		return Card{}, Card{}
	}

	if inList, onPage := card(); inList != (Card{}) || onPage != (Card{}) {
		t.Errorf("cards before any = %+v, %+v; want empty", inList, onPage)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO tournament_cards VALUES ('wimbledon', '🎾', 'Grass-court slam', 'Played each July.')"); err != nil {
		t.Fatal(err)
	}
	want := Card{Icon: "🎾", Tagline: "Grass-court slam", Summary: "Played each July."}
	if inList, onPage := card(); inList != want || onPage != want {
		t.Errorf("cards = %+v, %+v; want %+v on both", inList, onPage, want)
	}
}

func TestCatalogueTournamentSaysWhetherItIsRated(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	r := NewRatingsReader(asReader(t, pool))
	if got, err := r.CatalogueTournament(ctx, run, "wimbledon"); err != nil || !got.Rated {
		t.Fatalf("rated = %v, %v; want rated", got.Rated, err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM ranking_series"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE series SET visible = true WHERE slug = 'wimbledon'"); err != nil {
		t.Fatal(err)
	}
	if got, err := r.CatalogueTournament(ctx, run, "wimbledon"); err != nil || got.Rated {
		t.Errorf("rated after leaving every ranking = %v, %v; want not rated", got.Rated, err)
	}
}

func TestCatalogueTournamentGivesItsHighestWinningScores(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
		"Wimbledon 2024 Stage 1 - group 1 game 2": day("2024-07-02"),
		"Wimbledon 2024 Stage 1 - group 1 game 3": day("2024-07-03"),
	})
	setSeats(t, pool, "%game 1", Seat{"ann", 120}, Seat{"bob", 80})
	setSeats(t, pool, "%game 2", Seat{"ann", 90}, Seat{"cat", 150})
	setSeats(t, pool, "%game 3", Seat{"dan", 999}, Seat{"eve", 0})
	if _, err := pool.Exec(ctx, "UPDATE games SET state = 'never_played' WHERE name LIKE '%game 3'"); err != nil {
		t.Fatal(err)
	}

	got, err := NewRatingsReader(asReader(t, pool)).CatalogueTournament(ctx, run, "wimbledon")
	if err != nil {
		t.Fatalf("CatalogueTournament: %v", err)
	}
	want := []GameRecord{
		{Name: "cat", Score: 150, Opponents: []Seat{{Name: "ann", Score: 90}}, Tournament: "Wimbledon 2024 Stage 1 - group 1 game 2", Date: day("2024-07-02")},
		{Name: "ann", Score: 120, Opponents: []Seat{{Name: "bob", Score: 80}}, Tournament: "Wimbledon 2024 Stage 1 - group 1 game 1", Date: day("2024-07-01")},
	}
	if !reflect.DeepEqual(got.GameRecords, want) {
		t.Errorf("game records = %+v, want %+v (a game never played left out)", got.GameRecords, want)
	}
}

func TestCatalogueTournamentTakesItsKindAndTopDivisionsFromTheDatabase(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	r := NewRatingsReader(asReader(t, pool))
	if got, err := r.CatalogueTournament(ctx, run, "wimbledon"); err != nil || got.Ladder {
		t.Fatalf("wimbledon as a cup: ladder = %v, %v", got.Ladder, err)
	}
	if _, err := pool.Exec(ctx, "UPDATE series SET kind = 'ladder' WHERE slug = 'wimbledon'"); err != nil {
		t.Fatal(err)
	}
	if got, err := r.CatalogueTournament(ctx, run, "wimbledon"); err != nil || !got.Ladder {
		t.Errorf("wimbledon made a ladder in the database: ladder = %v, %v", got.Ladder, err)
	}
}
