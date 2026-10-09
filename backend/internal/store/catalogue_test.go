//go:build integration

package store

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

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
	SELECT concat_ws('|', g.name, se.slug, t.label, st.label, gr.name, coalesce(g.game_number, 0))
	FROM games g
	JOIN groups gr USING (group_id)
	JOIN stages st USING (stage_id)
	JOIN tournaments t USING (tournament_id)
	JOIN series se USING (series_id)
	ORDER BY g.game_id`

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
	if n := count(t, pool, "stages"); n != 3 {
		t.Errorf("stages = %d, want 3 (one per stage, however many games)", n)
	}
	if name := rowsOf(t, pool, "SELECT name FROM series WHERE slug = 'worlds'"); !slices.Equal(name, []string{"World Championship"}) {
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
	if n := rowsOf(t, pool, "SELECT name FROM games WHERE group_id IS NULL"); len(n) != 2 {
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
	if _, err := pool.Exec(ctx, "UPDATE groups SET name = 'from CGE' WHERE group_id = (SELECT group_id FROM games WHERE name = 'International S10')"); err != nil {
		t.Fatalf("setting a group: %v", err)
	}

	got, err := ClassifyMatches(ctx, pool)
	if err != nil {
		t.Fatalf("second ClassifyMatches: %v", err)
	}
	if got.Placed != 0 {
		t.Errorf("second ClassifyMatches placed %d, want 0", got.Placed)
	}
	if g := rowsOf(t, pool, "SELECT gr.name FROM games g JOIN groups gr USING (group_id) WHERE g.name = 'International S10'"); !slices.Equal(g, []string{"from CGE"}) {
		t.Errorf("group = %q, want the one set by hand", g)
	}
	if n := count(t, pool, "tournaments"); n != 2 {
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
	if _, err := pool.Exec(ctx, "UPDATE series SET name = 'Duel Curve League' WHERE slug = 'duel_curve'"); err != nil {
		t.Fatalf("renaming: %v", err)
	}
	if _, err := ClassifyMatches(ctx, pool); err != nil {
		t.Fatalf("second ClassifyMatches: %v", err)
	}
	if name := rowsOf(t, pool, "SELECT name FROM series WHERE slug = 'duel_curve'"); !slices.Equal(name, []string{"Duel Curve League"}) {
		t.Errorf("name = %q, want the admin's", name)
	}
}

const communityRows = `
	SELECT t.slug FROM ranking_series rt
	JOIN rankings r USING (ranking_id) JOIN series t USING (series_id)
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
	if _, err := pool.Exec(ctx, `DELETE FROM ranking_series WHERE series_id = (SELECT series_id FROM series WHERE slug = 'duel_curve')`); err != nil {
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
	rows, err := pool.Query(ctx, "SELECT slug, kind, top_divisions, original_name = name FROM series")
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

	if _, err := pool.Exec(ctx, "UPDATE tournaments SET label = 'Renamed' WHERE label = '2024'"); err != nil {
		t.Fatal(err)
	}
	var original string
	if err := pool.QueryRow(ctx, "SELECT original_name FROM tournaments WHERE label = 'Renamed'").Scan(&original); err != nil || original != "2024" {
		t.Errorf("a renamed season's original name = %q, %v; want 2024", original, err)
	}
	var stages int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM stages WHERE original_name <> label OR original_name = '' AND label <> ''").Scan(&stages); err != nil || stages != 0 {
		t.Errorf("%d stages without their original name (%v)", stages, err)
	}
}

func TestMigrationThreeNamesSlowBurnsLevelsAndRenamesItsStages(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	down := func(to int64) {
		t.Helper()
		if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, to); return err }); err != nil {
			t.Fatalf("migrating down to %d: %v", to, err)
		}
	}
	down(2)
	for _, q := range []string{
		`INSERT INTO tournaments (slug, name, original_name, kind) VALUES ('slow_burn', 'Slow Burn', 'Slow Burn', 'cup'), ('wimbledon', 'Wimbledon', 'Wimbledon', 'cup')`,
		`INSERT INTO tournament_seasons (tournament_id, label) SELECT tournament_id, l FROM tournaments, (VALUES ('1 (2024)'), ('2 (2026)')) v(l) WHERE slug = 'slow_burn'`,
		`INSERT INTO tournament_seasons (tournament_id, label) SELECT tournament_id, '2024' FROM tournaments WHERE slug = 'wimbledon'`,
		`INSERT INTO tournament_stages (season_id, label, original_name) SELECT season_id, st, st FROM tournament_seasons,
		   (VALUES ('Stage 1'), ('Stage 15'), ('Final')) v(st)`,
		`INSERT INTO tournament_season_notes (tournament, season, stage, note, source) VALUES ('slow_burn', '1 (2024)', 'Stage 15', 'the final', 'test')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("building the old shape: %v\n%s", err, q)
		}
	}
	stages := `SELECT concat_ws('|', t.slug, se.label, st.label, st.original_name) FROM tournament_stages st
		JOIN tournament_seasons se USING (season_id) JOIN tournaments t USING (tournament_id) ORDER BY 1`

	if err := migrateRatingsTo(ctx, pool, 3); err != nil {
		t.Fatalf("migrateRatingsTo(3): %v", err)
	}
	want := []string{
		"slow_burn|1 (2024)|Final|Final", "slow_burn|1 (2024)|Season 1|Season 1", "slow_burn|1 (2024)|Season 15|Season 15",
		"slow_burn|2 (2026)|Final|Final", "slow_burn|2 (2026)|Season 1|Season 1", "slow_burn|2 (2026)|Season 15|Season 15",
		"wimbledon|2024|Final|Final", "wimbledon|2024|Stage 1|Stage 1", "wimbledon|2024|Stage 15|Stage 15",
	}
	if got := slices.Sorted(slices.Values(rowsOf(t, pool, stages))); !slices.Equal(got, slices.Sorted(slices.Values(want))) {
		t.Errorf("stages = %q, want %q", got, want)
	}
	if got := rowsOf(t, pool, `SELECT concat_ws('|', slug, season_name, stage_name) FROM tournaments ORDER BY slug`); !slices.Equal(got, []string{"slow_burn|Edition|Season", "wimbledon||"}) {
		t.Errorf("names = %q, want Slow Burn's set and Wimbledon's default", got)
	}
	if got := rowsOf(t, pool, `SELECT stage FROM tournament_season_notes`); !slices.Equal(got, []string{"Season 15"}) {
		t.Errorf("note's stage = %q, want Season 15", got)
	}

	down(2)
	if got := rowsOf(t, pool, `SELECT stage FROM tournament_season_notes`); !slices.Equal(got, []string{"Stage 15"}) {
		t.Errorf("after down, note's stage = %q, want Stage 15", got)
	}
	if got := rowsOf(t, pool, stages+" LIMIT 1"); len(got) != 1 || got[0] != "slow_burn|1 (2024)|Final|Final" {
		t.Errorf("after down, first stage = %q", got)
	}
}

func TestClassifyMatchesGivesNewSeriesTheEnginesRules(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	loadNamed(t, pool, "Wimbledon 2024 Stage 1 - group 1 game 1", "International S30 - Grandmaster game 1", "ML_s12",
		"WC 2023 Stage 1 - Group 1 game 1")
	if _, err := ClassifyMatches(ctx, pool); err != nil {
		t.Fatalf("ClassifyMatches: %v", err)
	}
	tests := []struct{ slug, scoring, code, results string }{
		{"international", "league", "ic_{}", "international"},
		{"worlds", "worlds", "wrld", "worlds"},
		{"wimbledon", "knockout", "wmbl", ""},
		{"mercurial_ladder", "", "ml", ""},
	}
	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			var scoring, code, results string
			err := pool.QueryRow(ctx, `SELECT coalesce(community_scoring, ''), coalesce(community_code, ''),
				coalesce(results_rules, '') FROM series WHERE slug = $1`, tt.slug).Scan(&scoring, &code, &results)
			if err != nil {
				t.Fatal(err)
			}
			if scoring != tt.scoring || code != tt.code || results != tt.results {
				t.Errorf("community scoring, code, results rules = %q, %q, %q, want %q, %q, %q",
					scoring, code, results, tt.scoring, tt.code, tt.results)
			}
		})
	}
}
