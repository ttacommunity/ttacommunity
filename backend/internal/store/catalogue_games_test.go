//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

// gameKeys returns each game's season, stage, group and number, in the order given.
func gameKeys(games []CatalogueGame) []string {
	var out []string
	for _, g := range games {
		out = append(out, fmt.Sprintf("%s|%s|%s|%d", g.Season, g.Stage, g.Group, g.Game))
	}
	return out
}

func TestCatalogueGamesNarrowsBySeasonStageAndGroup(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Wimbledon 2023 stage 1":                  day("2023-07-15"),
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
		"Wimbledon 2024 Stage 1 - group 2 game 1": day("2024-07-03"),
		"Wimbledon 2024 Stage 7 - Final game 1":   day("2025-02-24"),
	})
	r := NewRatingsReader(asReader(t, pool))

	tests := []struct {
		name       string
		filter     CatalogueGamesFilter
		wantGames  []string
		wantGroups []string
	}{
		{"every game, newest first", CatalogueGamesFilter{},
			[]string{"2024|Stage 7|Final|1", "2024|Stage 1|group 2|1", "2024|Stage 1|group 1|1", "2023|Stage 1||0"}, nil},
		{"a season, with its groups", CatalogueGamesFilter{Season: "2024"},
			[]string{"2024|Stage 7|Final|1", "2024|Stage 1|group 2|1", "2024|Stage 1|group 1|1"}, []string{"Final", "group 1", "group 2"}},
		{"a stage", CatalogueGamesFilter{Season: "2024", Stage: "Stage 1"},
			[]string{"2024|Stage 1|group 2|1", "2024|Stage 1|group 1|1"}, []string{"group 1", "group 2"}},
		{"a group", CatalogueGamesFilter{Season: "2024", Stage: "Stage 1", Group: "group 2"},
			[]string{"2024|Stage 1|group 2|1"}, []string{"group 1", "group 2"}},
		{"a season with no such group", CatalogueGamesFilter{Season: "2023", Group: "group 9"}, nil, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.CatalogueGames(ctx, run, "wimbledon", tt.filter)
			if err != nil {
				t.Fatalf("CatalogueGames: %v", err)
			}
			if keys := gameKeys(got.Games); !reflect.DeepEqual(keys, tt.wantGames) {
				t.Errorf("games = %q, want %q", keys, tt.wantGames)
			}
			if got.Total != len(tt.wantGames) {
				t.Errorf("total = %d, want %d", got.Total, len(tt.wantGames))
			}
			if !reflect.DeepEqual(got.Groups, tt.wantGroups) {
				t.Errorf("groups = %q, want %q", got.Groups, tt.wantGroups)
			}
		})
	}
}

func TestCatalogueGamesGivesSeatsStateAndCode(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02"),
		"Intermezzo S24 - Bronze 1 game 2": day("2024-01-09"),
	})
	var id int64
	if err := pool.QueryRow(ctx, `UPDATE games SET state = 'never_played', share_code = 'ABCDEFGH'
		WHERE name = 'Intermezzo S24 - Bronze 1 game 2' RETURNING game_id`).Scan(&id); err != nil {
		t.Fatalf("marking a game never played: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE seats SET outcome = 'timed_out', culture = 0 WHERE game_id = $1 AND seat = 1", id); err != nil {
		t.Fatalf("adding a seat: %v", err)
	}
	var bob string
	if err := pool.QueryRow(ctx, "SELECT player_name FROM seats WHERE game_id = $1 AND seat = 2", id).Scan(&bob); err != nil {
		t.Fatalf("reading the game: %v", err)
	}

	got, err := NewRatingsReader(asReader(t, pool)).CatalogueGames(ctx, run, "intermezzo", CatalogueGamesFilter{})
	if err != nil {
		t.Fatalf("CatalogueGames: %v", err)
	}
	if len(got.Games) != 2 {
		t.Fatalf("games = %+v, want both, the never-played one included", got.Games)
	}
	g := got.Games[0]
	if g.State != "never_played" || g.Code != "ABCDEFGH" || g.ID != id {
		t.Errorf("newest game = %+v, want never played, code ABCDEFGH, id %d", g, id)
	}
	// Best place first; a timed-out seat has no culture; an outcome only where it is known.
	want := []GameSeat{{Name: bob, Score: 2}, {Name: "ann", Score: 0, Outcome: "timed_out"}}
	if !reflect.DeepEqual(g.Seats, want) {
		t.Errorf("seats = %+v, want %+v", g.Seats, want)
	}
	if got.Games[1].State != "played" || got.Games[1].Code != "" {
		t.Errorf("older game = %+v, want played, no code", got.Games[1])
	}
}

func TestCatalogueGamesCountsAndFiltersCodes(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02"),
		"Intermezzo S24 - Bronze 1 game 2": day("2024-01-09"),
		"Intermezzo S24 - Bronze 1 game 3": day("2024-01-16"),
	})
	if _, err := pool.Exec(ctx, "UPDATE games SET share_code = 'CODE' || game_number WHERE game_number < 3"); err != nil {
		t.Fatalf("adding codes: %v", err)
	}
	r := NewRatingsReader(asReader(t, pool))

	all, err := r.CatalogueGames(ctx, run, "intermezzo", CatalogueGamesFilter{})
	if err != nil {
		t.Fatalf("CatalogueGames: %v", err)
	}
	if all.Total != 3 || all.Coded != 2 {
		t.Errorf("all games: total %d, coded %d; want 3 and 2", all.Total, all.Coded)
	}
	coded, err := r.CatalogueGames(ctx, run, "intermezzo", CatalogueGamesFilter{Coded: true})
	if err != nil {
		t.Fatalf("CatalogueGames(coded): %v", err)
	}
	if keys := gameKeys(coded.Games); !reflect.DeepEqual(keys, []string{"24||Bronze 1|2", "24||Bronze 1|1"}) || coded.Total != 2 || coded.Coded != 2 {
		t.Errorf("coded games = %q (total %d, coded %d), want games 2 and 1, 2 and 2", keys, coded.Total, coded.Coded)
	}
}

func TestCatalogueGamesPages(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	games := map[string]time.Time{}
	for n := 1; n <= catalogueGamesPage+1; n++ {
		games[fmt.Sprintf("Intermezzo S24 - Bronze 1 game %d", n)] = day("2024-01-02").AddDate(0, 0, n)
	}
	placedOn(t, pool, games)
	r := NewRatingsReader(asReader(t, pool))

	first, err := r.CatalogueGames(ctx, run, "intermezzo", CatalogueGamesFilter{Page: 1})
	if err != nil {
		t.Fatalf("CatalogueGames(page 1): %v", err)
	}
	if first.Page != 1 || first.Pages != 2 || len(first.Games) != catalogueGamesPage || first.Total != catalogueGamesPage+1 {
		t.Errorf("page 1: page %d of %d, %d games of %d; want 1 of 2, %d of %d", first.Page, first.Pages, len(first.Games), first.Total, catalogueGamesPage, catalogueGamesPage+1)
	}
	past, err := r.CatalogueGames(ctx, run, "intermezzo", CatalogueGamesFilter{Page: 9})
	if err != nil {
		t.Fatalf("CatalogueGames(page 9): %v", err)
	}
	if past.Page != 2 || len(past.Games) != 1 || past.Games[0].Game != 1 {
		t.Errorf("a page past the last: page %d with %d games, want page 2 with the oldest game", past.Page, len(past.Games))
	}
}

func TestCatalogueGamesRefusesAHiddenTournamentAndAnUnpublishedRun(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	if _, err := pool.Exec(ctx, `DELETE FROM ranking_series WHERE series_id = (SELECT series_id FROM series WHERE slug = 'wimbledon')`); err != nil {
		t.Fatalf("unrating wimbledon: %v", err)
	}
	r := NewRatingsReader(asReader(t, pool))

	for _, slug := range []string{"wimbledon", "no_such_cup"} {
		if _, err := r.CatalogueGames(ctx, run, slug, CatalogueGamesFilter{}); !errors.Is(err, ErrTournamentNotFound) {
			t.Errorf("CatalogueGames(%q): %v, want ErrTournamentNotFound", slug, err)
		}
	}
	unpublished := addRun(t, pool, "complete", time.Time{})
	if _, err := r.CatalogueGames(ctx, unpublished, "wimbledon", CatalogueGamesFilter{}); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("CatalogueGames of an unpublished run: %v, want ErrRunNotFound", err)
	}
}

func TestCatalogueGamesNarrowsToAPlayer(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02"),
		"Intermezzo S24 - Bronze 1 game 2": day("2024-01-09"),
		"Intermezzo S25 - Bronze 2 game 1": day("2024-05-01"),
	})
	var bob string
	if err := pool.QueryRow(ctx, "SELECT s.player_name FROM seats s JOIN games g USING (game_id) WHERE g.name = 'Intermezzo S24 - Bronze 1 game 2' AND s.seat = 2").Scan(&bob); err != nil {
		t.Fatalf("reading a player: %v", err)
	}
	r := NewRatingsReader(asReader(t, pool))

	// Every game has ann; each bob plays one. A name matches whatever its case.
	for _, tt := range []struct {
		player string
		filter CatalogueGamesFilter
		want   []string
	}{
		{"ANN", CatalogueGamesFilter{Player: "ANN"}, []string{"25||Bronze 2|1", "24||Bronze 1|2", "24||Bronze 1|1"}},
		{bob, CatalogueGamesFilter{Player: bob}, []string{"24||Bronze 1|2"}},
		{"ann in S25", CatalogueGamesFilter{Player: "ann", Season: "25"}, []string{"25||Bronze 2|1"}},
		{"nobody", CatalogueGamesFilter{Player: "nobody"}, nil},
	} {
		got, err := r.CatalogueGames(ctx, run, "intermezzo", tt.filter)
		if err != nil {
			t.Fatalf("CatalogueGames(%s): %v", tt.player, err)
		}
		if keys := gameKeys(got.Games); !reflect.DeepEqual(keys, tt.want) || got.Total != len(tt.want) {
			t.Errorf("CatalogueGames(%s) = %q (total %d), want %q", tt.player, keys, got.Total, tt.want)
		}
	}
}

func TestAnUnconfirmedGameIsListedButNotCounted(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02"),
		"Intermezzo S24 - Bronze 1 game 2": day("2024-01-09"),
	})
	if _, err := pool.Exec(ctx, "UPDATE games SET state = 'unconfirmed' WHERE name = 'Intermezzo S24 - Bronze 1 game 2'"); err != nil {
		t.Fatalf("marking a game unconfirmed: %v", err)
	}
	r := NewRatingsReader(asReader(t, pool))

	games, err := r.CatalogueGames(ctx, run, "intermezzo", CatalogueGamesFilter{})
	if err != nil {
		t.Fatalf("CatalogueGames: %v", err)
	}
	if len(games.Games) != 2 || games.Games[0].State != "unconfirmed" {
		t.Errorf("games = %+v, want both, the newest unconfirmed", games.Games)
	}
	view, err := r.Catalogue(ctx, run)
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	if got := view.Tournaments[0].Span.Games; got != 1 {
		t.Errorf("intermezzo counts %d games, want 1: an unconfirmed game is not counted", got)
	}
}

func TestCatalogueGamesMarksWhichGamesAreRated(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Wimbledon 2023 stage 1":                  day("2023-07-15"),
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
		"Wimbledon 2024 Stage 1 - group 2 game 1": day("2024-07-03"),
		"Wimbledon 2024 Stage 7 - Final game 1":   day("2025-02-24"),
	})
	for _, q := range []string{
		"UPDATE tournaments SET rated = false WHERE label = '2023'",
		"INSERT INTO players (name) VALUES ('cat')",
		`INSERT INTO seats (game_id, seat, player_id, player_name, culture, recorded_score, place)
		 SELECT game_id, 3, (SELECT player_id FROM players WHERE name = 'cat'), 'cat', 1, 1, 2 FROM games WHERE name LIKE '%group 2 game 1'`,
		"UPDATE series SET rated_player_counts = '{2}' WHERE slug = 'wimbledon'",
		"UPDATE games SET state = 'never_played' WHERE name LIKE '%Final game 1'",
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	r := NewRatingsReader(asReader(t, pool))
	rated := func() map[string]bool {
		t.Helper()
		got, err := r.CatalogueGames(ctx, run, "wimbledon", CatalogueGamesFilter{})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, g := range got.Games {
			out[fmt.Sprintf("%s|%s|%s", g.Season, g.Stage, g.Group)] = g.Rated
		}
		return out
	}

	want := map[string]bool{
		"2023|Stage 1|":        false,
		"2024|Stage 1|group 1": true,
		"2024|Stage 1|group 2": false,
		"2024|Stage 7|Final":   false,
	}
	if got := rated(); !reflect.DeepEqual(got, want) {
		t.Errorf("rated = %v, want %v", got, want)
	}
	for _, q := range []string{"DELETE FROM ranking_series", "UPDATE series SET visible = true"} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if got := rated(); got["2024|Stage 1|group 1"] {
		t.Errorf("a game of a tournament in no ranking is rated: %v", got)
	}
}
