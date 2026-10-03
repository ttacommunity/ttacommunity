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

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/tournamentcsv"
)

// placedOn loads games by name, each played on its date, and places them in the catalogue.
func placedOn(t *testing.T, pool *pgxpool.Pool, games map[string]time.Time) {
	t.Helper()
	ctx := context.Background()
	var names []string
	for n := range games {
		names = append(names, n)
	}
	loadNamed(t, pool, names...)
	for n, d := range games {
		if _, err := pool.Exec(ctx, "UPDATE matches SET date = $2 WHERE tournament = $1", n, d); err != nil {
			t.Fatalf("dating %s: %v", n, err)
		}
	}
	if _, err := ClassifyMatches(ctx, pool); err != nil {
		t.Fatalf("ClassifyMatches: %v", err)
	}
}

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

func TestCatalogueNeedsAPublishedRun(t *testing.T) {
	pool := migrated(t)
	run := addRun(t, pool, "complete", time.Time{})
	if _, err := NewRatingsReader(asReader(t, pool)).Catalogue(context.Background(), run); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("Catalogue of an unpublished run: %v, want ErrRunNotFound", err)
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
	want := CatalogueTournament{Slug: "wimbledon", Name: "Wimbledon", Seasons: []CatalogueSeason{
		{Label: "2023", Span: GameSpan{Games: 1, Players: 2, First: day("2023-07-15"), Last: day("2023-07-15")},
			Stages: []CatalogueStage{stage("Stage 1", day("2023-07-15"))}, Podium: []PodiumPlace{}, CGE: []CGELink{}},
		{Label: "2024", Span: GameSpan{Games: 2, Players: 3, First: day("2024-07-01"), Last: day("2025-02-24")},
			Stages: []CatalogueStage{stage("Stage 1", day("2024-07-01")), stage("Stage 7", day("2025-02-24"))},
			Podium: []PodiumPlace{{Place: 1, Name: "ann", Final: "", Finished: day("2025-02-24"), Source: "CGE"}}, CGE: []CGELink{}},
	}, TopTierWins: []LadderWin{}, Players: []string{"ann", "bob2", "bob3", "bob4"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CatalogueTournament = %+v, want %+v", got, want)
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
		`INSERT INTO tournaments (slug, name) VALUES ('nations_cup', 'Nations Cup')`,
		`INSERT INTO cge_tournaments (cge_tournament, name, status, organiser, created, started, finished, tournament_id, season) VALUES
			(4991, 'Wimbledon 2024', 'finished', 'an organiser', '2024-06-01', NULL, '2025-02-24', (SELECT tournament_id FROM tournaments WHERE slug = 'wimbledon'), '2024'),
			(5373, 'Nations Cup', 'finished', '', NULL, NULL, '2025-11-19', (SELECT tournament_id FROM tournaments WHERE slug = 'nations_cup'), '2025'),
			(56, 'Totally Friendly Tournament for six', 'finished', '', NULL, '2020-04-14', '2020-06-27', NULL, '')`,
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
		`DELETE FROM ranking_tournaments WHERE tournament_id = (SELECT tournament_id FROM tournaments WHERE slug = 'wimbledon')`,
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

	if _, err := pool.Exec(ctx, "UPDATE tournaments SET visible = true WHERE slug = 'wimbledon'"); err != nil {
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
	if _, err := pool.Exec(ctx, "UPDATE matches SET state = 'never_played' WHERE tournament = 'Intermezzo S24 - Bronze 1 game 2'"); err != nil {
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
	if _, err := pool.Exec(ctx, `UPDATE matches SET cge_tournament = 19, cge_season = substr(tournament, 13, 2)::int - 5 WHERE tournament LIKE 'Intermezzo S2_ -%' AND tournament NOT LIKE 'Intermezzo S26%';
		UPDATE matches SET cge_tournament = 4991, cge_season = 1 WHERE tournament LIKE 'Wimbledon%Stage 1%';
		UPDATE matches SET cge_tournament = 4991, cge_season = 7 WHERE tournament LIKE 'Wimbledon%Stage 7%'`); err != nil {
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
	})
	if err != nil {
		t.Fatalf("ReplaceLadderWins: %v", err)
	}
	if got.Written != 2 || got.Unknown != 1 {
		t.Errorf("ReplaceLadderWins = %+v, want 2 written and 1 not a player", got)
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
	notes := []tournamentcsv.SeasonNote{{Tournament: "worlds", Season: "2023 Play Off", Note: "A second chance for the 2023 entrants.", Source: "CGE"}}
	if got, err := ReplaceSeasonNotes(ctx, pool, notes); err != nil || got.Written != 1 {
		t.Fatalf("ReplaceSeasonNotes = %+v, %v; want 1 written", got, err)
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
}
