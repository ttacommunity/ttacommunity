//go:build integration

package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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

	got, err := NewRatingsReader(asReader(t, pool)).Catalogue(context.Background(), run)
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	// Each game here has ann and one other player of its own; the friendly game is placed nowhere.
	want := []CatalogueEntry{
		{Slug: "intermezzo", Name: "Intermezzo", Seasons: 2, Span: GameSpan{Games: 3, Players: 4, First: day("2024-01-02"), Last: day("2024-05-01")}},
		{Slug: "wimbledon", Name: "Wimbledon", Seasons: 1, Span: GameSpan{Games: 1, Players: 2, First: day("2024-07-01"), Last: day("2024-07-01")}},
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
			Stages: []CatalogueStage{stage("Stage 1", day("2023-07-15"))}, Podium: []PodiumPlace{}},
		{Label: "2024", Span: GameSpan{Games: 2, Players: 3, First: day("2024-07-01"), Last: day("2025-02-24")},
			Stages: []CatalogueStage{stage("Stage 1", day("2024-07-01")), stage("Stage 7", day("2025-02-24"))},
			Podium: []PodiumPlace{{Place: 1, Name: "ann", Final: "", Finished: day("2025-02-24"), Source: "CGE"}}},
	}}
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
