//go:build integration

package store

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/gamename"
	"ttacommunity/backend/internal/matchcsv"
)

func named(line int, tournament string) matchcsv.Match {
	m := match("m.csv", line, "ann", fmt.Sprintf("bob%d", line))
	m.Tournament = tournament
	return m
}

func loadNamed(t *testing.T, pool *pgxpool.Pool, names ...string) {
	t.Helper()
	var ms []matchcsv.Match
	for i, n := range names {
		ms = append(ms, named(i+2, n))
	}
	if _, err := LoadMatches(context.Background(), pool, ms, nil); err != nil {
		t.Fatalf("LoadMatches: %v", err)
	}
}

const placedRows = `
	SELECT concat_ws('|', m.tournament, t.slug, s.label, st.label, m.group_label, coalesce(m.game_number, 0))
	FROM matches m
	JOIN tournament_stages st USING (stage_id)
	JOIN tournament_seasons s USING (season_id)
	JOIN tournaments t USING (tournament_id)
	ORDER BY m.match_id`

func TestClassifyMatchesPlacesEachGameInTheCatalogue(t *testing.T) {
	pool := migrated(t)
	loadNamed(t, pool,
		"Intermezzo S24 - Bronze 1 game 3",
		"Intermezzo S24 - Bronze 1 game 4",
		"Worlds 2024 Stage 4 - Lower 12 game 3",
		"RL_tb_s06 - Best Baron 1 game 1",
	)

	got, err := ClassifyMatches(context.Background(), pool)
	if err != nil {
		t.Fatalf("ClassifyMatches: %v", err)
	}
	if got.Placed != 4 || len(got.Unplaced) != 0 {
		t.Errorf("ClassifyMatches = %+v, want 4 placed and none unplaced", got)
	}
	want := []string{
		"Intermezzo S24 - Bronze 1 game 3|intermezzo|24||Bronze 1|3",
		"Intermezzo S24 - Bronze 1 game 4|intermezzo|24||Bronze 1|4",
		"Worlds 2024 Stage 4 - Lower 12 game 3|worlds|2024|Stage 4|Lower 12|3",
		"RL_tb_s06 - Best Baron 1 game 1|royal_league|6|Tiebreak|Best Baron 1|1",
	}
	if rows := rowsOf(t, pool, placedRows); !slices.Equal(rows, want) {
		t.Errorf("placed = %q, want %q", rows, want)
	}
	if n := count(t, pool, "tournament_stages"); n != 3 {
		t.Errorf("stages = %d, want 3 (one per stage, however many games)", n)
	}
	if name := rowsOf(t, pool, "SELECT name FROM tournaments WHERE slug = 'worlds'"); !slices.Equal(name, []string{"World Championship"}) {
		t.Errorf("worlds is named %q, want World Championship", name)
	}
}

func TestClassifyMatchesLeavesNamesItCannotReadUnplaced(t *testing.T) {
	pool := migrated(t)
	loadNamed(t, pool, "Friendly game", "International S10", "Friendly game")

	got, err := ClassifyMatches(context.Background(), pool)
	if err != nil {
		t.Fatalf("ClassifyMatches: %v", err)
	}
	if got.Placed != 1 || !slices.Equal(got.Unplaced, []string{"Friendly game"}) {
		t.Errorf("ClassifyMatches = %+v, want 1 placed and Friendly game unplaced once", got)
	}
	if n := rowsOf(t, pool, "SELECT tournament FROM matches WHERE stage_id IS NULL"); len(n) != 2 {
		t.Errorf("unplaced matches = %q, want both Friendly games", n)
	}
}

func TestClassifyMatchesPlacesOnlyUnplacedGames(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	loadNamed(t, pool, "International S10", "International S11")
	if _, err := ClassifyMatches(ctx, pool); err != nil {
		t.Fatalf("first ClassifyMatches: %v", err)
	}
	// A placement set by hand, such as from CGE, is not read again from the name.
	if _, err := pool.Exec(ctx, "UPDATE matches SET group_label = 'from CGE' WHERE tournament = 'International S10'"); err != nil {
		t.Fatalf("setting a group: %v", err)
	}

	got, err := ClassifyMatches(ctx, pool)
	if err != nil {
		t.Fatalf("second ClassifyMatches: %v", err)
	}
	if got.Placed != 0 {
		t.Errorf("second ClassifyMatches placed %d, want 0", got.Placed)
	}
	if g := rowsOf(t, pool, "SELECT group_label FROM matches WHERE tournament = 'International S10'"); !slices.Equal(g, []string{"from CGE"}) {
		t.Errorf("group = %q, want the one set by hand", g)
	}
	if n := count(t, pool, "tournament_seasons"); n != 2 {
		t.Errorf("seasons = %d, want 2", n)
	}
}

func TestClassifyMatchesKeepsATournamentRenamedInAdmin(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	loadNamed(t, pool, "DC_s02")
	if _, err := ClassifyMatches(ctx, pool); err != nil {
		t.Fatalf("first ClassifyMatches: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE tournaments SET name = 'Duel Curve League' WHERE slug = 'duel_curve'"); err != nil {
		t.Fatalf("renaming: %v", err)
	}
	if _, err := ClassifyMatches(ctx, pool); err != nil {
		t.Fatalf("second ClassifyMatches: %v", err)
	}
	if name := rowsOf(t, pool, "SELECT name FROM tournaments WHERE slug = 'duel_curve'"); !slices.Equal(name, []string{"Duel Curve League"}) {
		t.Errorf("name = %q, want the admin's", name)
	}
}

const communityRows = `
	SELECT t.slug FROM ranking_tournaments rt
	JOIN rankings r USING (ranking_id) JOIN tournaments t USING (tournament_id)
	WHERE r.slug = 'community' ORDER BY t.slug`

func TestClassifyMatchesRatesANewTournamentInTheCommunityRanking(t *testing.T) {
	pool := migrated(t)
	loadNamed(t, pool, "International S10")
	if _, err := ClassifyMatches(context.Background(), pool); err != nil {
		t.Fatalf("ClassifyMatches: %v", err)
	}
	got := rowsOf(t, pool, communityRows)
	if !slices.Contains(got, "international") || len(got) != len(gamename.Tournaments) {
		t.Errorf("community ranking = %q, want every catalogue tournament", got)
	}
}

func TestClassifyMatchesKeepsATournamentTakenOutOfARanking(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	loadNamed(t, pool, "International S10")
	if _, err := ClassifyMatches(ctx, pool); err != nil {
		t.Fatalf("first ClassifyMatches: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM ranking_tournaments WHERE tournament_id = (SELECT tournament_id FROM tournaments WHERE slug = 'duel_curve')`); err != nil {
		t.Fatalf("taking duel_curve out: %v", err)
	}
	if _, err := ClassifyMatches(ctx, pool); err != nil {
		t.Fatalf("second ClassifyMatches: %v", err)
	}
	if got := rowsOf(t, pool, communityRows); slices.Contains(got, "duel_curve") {
		t.Errorf("community ranking = %q: placing games again put duel_curve back", got)
	}
}

func TestClassifyMatchesGivesNewTournamentsTheirKindAndKeepsOriginalNames(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	loadNamed(t, pool, "Wimbledon 2024 Stage 1 - group 1 game 1", "International S30 - Grandmaster game 1", "ML_s12")
	if _, err := ClassifyMatches(ctx, pool); err != nil {
		t.Fatalf("ClassifyMatches: %v", err)
	}
	kinds := map[string]string{}
	tops := map[string][]string{}
	rows, err := pool.Query(ctx, "SELECT slug, kind, top_divisions, original_name = name FROM tournaments")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var slug, kind string
		var top []string
		var kept bool
		if err := rows.Scan(&slug, &kind, &top, &kept); err != nil {
			t.Fatal(err)
		}
		kinds[slug], tops[slug] = kind, top
		if !kept {
			t.Errorf("%s's original name differs from its name when first added", slug)
		}
	}
	if kinds["wimbledon"] != "cup" || kinds["international"] != "league" || kinds["mercurial_ladder"] != "ladder" {
		t.Errorf("kinds = %v", kinds)
	}
	if !slices.Equal(tops["international"], []string{"Grandmaster", "Diamond"}) || len(tops["wimbledon"]) != 0 {
		t.Errorf("top divisions = %v", tops)
	}

	if _, err := pool.Exec(ctx, "UPDATE tournament_seasons SET label = 'Renamed' WHERE label = '2024'"); err != nil {
		t.Fatal(err)
	}
	var original string
	if err := pool.QueryRow(ctx, "SELECT original_name FROM tournament_seasons WHERE label = 'Renamed'").Scan(&original); err != nil || original != "2024" {
		t.Errorf("a renamed season's original name = %q, %v; want 2024", original, err)
	}
	var stages int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM tournament_stages WHERE original_name <> label OR original_name = '' AND label <> ''").Scan(&stages); err != nil || stages != 0 {
		t.Errorf("%d stages without their original name (%v)", stages, err)
	}
}
