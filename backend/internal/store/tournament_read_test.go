//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func classifyGame(t *testing.T, pool *pgxpool.Pool, run, match int64, season int, division, source *string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO tournament_games (run_id, match_id, series, season, division, group_label, division_source)
		VALUES ($1, $2, 'international', $3, $4, $4, $5)`, run, match, season, division, source)
	if err != nil {
		t.Fatalf("classifying game %d: %v", match, err)
	}
}

func ptr(s string) *string { return &s }

func TestTournamentSeriesCountsGamesAndSeasons(t *testing.T) {
	pool := migrated(t)
	run := publishedRun(t, pool)
	for season, division := range map[int]*string{20: nil, 34: ptr("Silver")} {
		id := addGame(t, pool, 0, "2025-01-02", fmt.Sprintf("International S%d", season), Seat{"ann", 9}, Seat{"bob", 5})
		var source *string
		if division != nil {
			source = ptr("name")
		}
		classifyGame(t, pool, run, id, season, division, source)
	}

	got, err := NewRatingsReader(asReader(t, pool)).TournamentSeries(context.Background(), run, "international")
	if err != nil {
		t.Fatalf("TournamentSeries: %v", err)
	}
	if got.Games != 2 || got.Seasons != 34 {
		t.Errorf("games %d, seasons %d; want 2 and 34", got.Games, got.Seasons)
	}
	if got.Board == nil || got.SeasonRecords == nil || got.GameRecords == nil {
		t.Errorf("empty lists = %v %v %v, want empty slices", got.Board, got.SeasonRecords, got.GameRecords)
	}
}

func TestTournamentSeriesOrdersTheBoardByMedalsThenPoints(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	for _, b := range []struct {
		name                 string
		gold, silver, bronze int
		points               float64
		title                string
	}{
		{"cat", 1, 0, 0, 40, "GM"}, {"ann", 2, 0, 0, 10, "WC"}, {"bob", 1, 0, 0, 50, "GM"}, {"dan", 0, 3, 1, 90, "M"},
	} {
		_, err := pool.Exec(ctx, "INSERT INTO tournament_boards VALUES ($1, 'international', $2, $3, $4, $5, $6, $7)",
			run, b.name, b.gold, b.silver, b.bronze, b.points, b.title)
		if err != nil {
			t.Fatalf("adding %s to the board: %v", b.name, err)
		}
	}

	got, err := NewRatingsReader(asReader(t, pool)).TournamentSeries(ctx, run, "international")
	if err != nil {
		t.Fatalf("TournamentSeries: %v", err)
	}
	want := []TrophyRow{
		{Name: "ann", Title: "WC", Gold: 2}, {Name: "bob", Title: "GM", Gold: 1}, {Name: "cat", Title: "GM", Gold: 1},
		{Name: "dan", Title: "M", Silver: 3, Bronze: 1},
	}
	if !slices.Equal(got.Board, want) {
		t.Errorf("board = %v, want %v", got.Board, want)
	}
}

func TestTournamentSeriesGivesTheTenBestSeasons(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	if _, err := pool.Exec(ctx, "INSERT INTO player_peak_titles VALUES ($1, 'p05', 'WC', 2024)", run); err != nil {
		t.Fatalf("adding a World Champion: %v", err)
	}
	// Eleven seasons: p00 has most points; p01 and p02 tie on points and wins, p02 seen first.
	for i := range 11 {
		points, wins, first := float64(30-i), 3, int64(100+i)
		switch i {
		case 1:
			first = 200
		case 2:
			points = 29
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO tournament_season_records VALUES ($1, 'international', 30, 'Grandmaster', $2, $3, 7, $4, '2025-07-31', $5)`,
			run, fmt.Sprintf("p%02d", i), points, []int{wins, 1, 1, 0}, first)
		if err != nil {
			t.Fatalf("adding season record %d: %v", i, err)
		}
	}

	got, err := NewRatingsReader(asReader(t, pool)).TournamentSeries(ctx, run, "international")
	if err != nil {
		t.Fatalf("TournamentSeries: %v", err)
	}
	var names []string
	for _, s := range got.SeasonRecords {
		names = append(names, s.Name)
	}
	if want := []string{"p00", "p02", "p01", "p03", "p04", "p05", "p06", "p07", "p08", "p09"}; !slices.Equal(names, want) {
		t.Errorf("season records = %v, want %v", names, want)
	}
	s := got.SeasonRecords[5]
	want := SeasonRecord{Name: "p05", WorldChampion: true, Points: 25, Games: 7, Placings: []int{3, 1, 1, 0},
		Season: 30, Division: "Grandmaster", LastPlayed: time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC)}
	if s.Name != want.Name || s.WorldChampion != want.WorldChampion || s.Points != want.Points || s.Games != want.Games ||
		!slices.Equal(s.Placings, want.Placings) || s.Season != want.Season || s.Division != want.Division || !s.LastPlayed.Equal(want.LastPlayed) {
		t.Errorf("season record 6 = %+v, want %+v", s, want)
	}
}

func TestTournamentSeriesGivesTheTopDivisionsBestGamesByName(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	gm := ptr("Grandmaster")
	named := func(score float64, date string, seats ...Seat) int64 {
		id := addGame(t, pool, 0, date, "International S30 - Grandmaster game 1", append([]Seat{{"win", score}}, seats...)...)
		classifyGame(t, pool, run, id, 30, gm, ptr("name"))
		return id
	}
	for i := range 11 {
		named(float64(200+i), fmt.Sprintf("2025-07-%02d", i+1), Seat{"x", 10}, Seat{"y", 20})
	}
	tied := named(210, "2025-08-01", Seat{"tie", 210}, Seat{"z", 5})
	recorded := addGame(t, pool, 0, "2020-01-01", "International S20", Seat{"old", 999}, Seat{"x", 1})
	classifyGame(t, pool, run, recorded, 20, gm, ptr("recorded"))
	silver := addGame(t, pool, 0, "2025-07-01", "International S30 - Silver 1 game 1", Seat{"low", 998}, Seat{"x", 1})
	classifyGame(t, pool, run, silver, 30, ptr("Silver"), ptr("name"))

	got, err := NewRatingsReader(asReader(t, pool)).TournamentSeries(ctx, run, "international")
	if err != nil {
		t.Fatalf("TournamentSeries: %v", err)
	}
	var scores []float64
	for _, g := range got.GameRecords {
		scores = append(scores, g.Score)
	}
	if want := []float64{210, 210, 209, 208, 207, 206, 205, 204, 203, 202}; !slices.Equal(scores, want) {
		t.Fatalf("game record scores = %v, want %v (named top-division games only)", scores, want)
	}
	first, second := got.GameRecords[0], got.GameRecords[1]
	if first.Name != "win" || first.Tournament != "International S30 - Grandmaster game 1" ||
		!first.Date.Equal(time.Date(2025, 7, 11, 0, 0, 0, 0, time.UTC)) ||
		!slices.Equal(first.Opponents, []Seat{{"y", 20}, {"x", 10}}) {
		t.Errorf("first game record = %+v", first)
	}
	// In the tied game the earlier seat wins, as upstream's stable sort does.
	if second.Name != "win" || !slices.Equal(second.Opponents, []Seat{{"tie", 210}, {"z", 5}}) {
		t.Errorf("tied game record = %+v (match %d)", second, tied)
	}
}

func TestTournamentSeriesNeedsAPublishedRun(t *testing.T) {
	pool := migrated(t)
	run := addRun(t, pool, "complete", time.Time{})
	_, err := NewRatingsReader(asReader(t, pool)).TournamentSeries(context.Background(), run, "international")
	if !errors.Is(err, ErrRunNotFound) {
		t.Errorf("TournamentSeries of an unpublished run: %v, want ErrRunNotFound", err)
	}
}
