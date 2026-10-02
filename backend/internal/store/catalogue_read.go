package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrTournamentNotFound is returned for a tournament with no placed games.
var ErrTournamentNotFound = errors.New("no tournament with games has this name")

// GameSpan is how many games and players a tournament, season or stage has,
// and the days of its first and last game.
type GameSpan struct {
	Games, Players int
	First, Last    time.Time
}

// CatalogueEntry is one tournament with placed games.
type CatalogueEntry struct {
	Slug, Name string
	Seasons    int
	Span       GameSpan
}

// CatalogueStage is one stage of a season; Label "" is its only or main stage.
type CatalogueStage struct {
	Label string
	Span  GameSpan
}

// PodiumPlace is one place on a cup year's recorded podium.
type PodiumPlace struct {
	Place       int
	Name, Final string
	Finished    time.Time
	Source      string
}

// CatalogueSeason is one season with its stages and recorded podium.
type CatalogueSeason struct {
	Label  string
	Span   GameSpan
	Stages []CatalogueStage
	Podium []PodiumPlace
}

// CatalogueTournament is one tournament's seasons, in order of their first game.
type CatalogueTournament struct {
	Slug, Name string
	Seasons    []CatalogueSeason
}

// placedSeats is every placed game's seats, one row per player, with its
// tournament, season and stage.
const placedSeats = `
	SELECT t.slug, t.name, s.season_id, s.label AS season, st.stage_id, st.label AS stage,
	       m.match_id, m.date, p.name AS player
	FROM matches m
	JOIN tournament_stages st USING (stage_id)
	JOIN tournament_seasons s USING (season_id)
	JOIN tournaments t USING (tournament_id)
	CROSS JOIN LATERAL unnest(ARRAY[m.player1, m.player2, m.player3, m.player4]) AS p(name)
	WHERE p.name IS NOT NULL`

// Catalogue returns every tournament with placed games, by name, for a
// published run. Unplaced games count nowhere.
func (r *RatingsReader) Catalogue(ctx context.Context, run int64) ([]CatalogueEntry, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		WITH g AS (`+placedSeats+`)
		SELECT slug, name, count(DISTINCT season_id), count(DISTINCT match_id), count(DISTINCT player), min(date), max(date)
		FROM g GROUP BY slug, name ORDER BY name, slug`)
	if err != nil {
		return nil, fmt.Errorf("reading the catalogue: %w", err)
	}
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CatalogueEntry, error) {
		var e CatalogueEntry
		return e, row.Scan(&e.Slug, &e.Name, &e.Seasons, &e.Span.Games, &e.Span.Players, &e.Span.First, &e.Span.Last)
	})
	if err != nil {
		return nil, fmt.Errorf("reading the catalogue: %w", err)
	}
	return got, nil
}

// CatalogueTournament returns a tournament's seasons in order of their first
// game, each with its stages (in the same order) and its recorded podium,
// for a published run. A tournament without placed games is ErrTournamentNotFound.
func (r *RatingsReader) CatalogueTournament(ctx context.Context, run int64, slug string) (CatalogueTournament, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return CatalogueTournament{}, err
	}
	fail := func(what string, err error) (CatalogueTournament, error) {
		return CatalogueTournament{}, fmt.Errorf("reading %s's %s: %w", slug, what, err)
	}
	got := CatalogueTournament{Slug: slug, Seasons: []CatalogueSeason{}}
	rows, err := r.pool.Query(ctx, `
		WITH g AS (`+placedSeats+` AND t.slug = $1)
		SELECT name, season_id, season, count(DISTINCT match_id), count(DISTINCT player), min(date), max(date)
		FROM g GROUP BY name, season_id, season ORDER BY min(date), season`, slug)
	if err != nil {
		return fail("seasons", err)
	}
	var ids []int64
	got.Seasons, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (CatalogueSeason, error) {
		s := CatalogueSeason{Stages: []CatalogueStage{}, Podium: []PodiumPlace{}}
		var id int64
		err := row.Scan(&got.Name, &id, &s.Label, &s.Span.Games, &s.Span.Players, &s.Span.First, &s.Span.Last)
		ids = append(ids, id)
		return s, err
	})
	if err != nil {
		return fail("seasons", err)
	}
	if len(got.Seasons) == 0 {
		return CatalogueTournament{}, ErrTournamentNotFound
	}
	bySeason := make(map[int64]int, len(ids))
	byLabel := make(map[string]int, len(ids))
	for i, id := range ids {
		bySeason[id], byLabel[got.Seasons[i].Label] = i, i
	}

	rows, err = r.pool.Query(ctx, `
		WITH g AS (`+placedSeats+` AND t.slug = $1)
		SELECT season_id, stage, count(DISTINCT match_id), count(DISTINCT player), min(date), max(date)
		FROM g GROUP BY season_id, stage_id, stage ORDER BY min(date), stage`, slug)
	if err != nil {
		return fail("stages", err)
	}
	var season int64
	var st CatalogueStage
	_, err = pgx.ForEachRow(rows, []any{&season, &st.Label, &st.Span.Games, &st.Span.Players, &st.Span.First, &st.Span.Last}, func() error {
		i := bySeason[season]
		got.Seasons[i].Stages = append(got.Seasons[i].Stages, st)
		return nil
	})
	if err != nil {
		return fail("stages", err)
	}

	rows, err = r.pool.Query(ctx, `
		SELECT season, place, player_name, final, finished, source
		FROM tournament_cup_podiums WHERE cup = $1 ORDER BY season, final, place`, slug)
	if err != nil {
		return fail("podiums", err)
	}
	var label string
	var p PodiumPlace
	_, err = pgx.ForEachRow(rows, []any{&label, &p.Place, &p.Name, &p.Final, &p.Finished, &p.Source}, func() error {
		if i, ok := byLabel[label]; ok {
			got.Seasons[i].Podium = append(got.Seasons[i].Podium, p)
		}
		return nil
	})
	if err != nil {
		return fail("podiums", err)
	}
	return got, nil
}
