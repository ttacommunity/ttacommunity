package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"ttacommunity/backend/internal/tournamentcsv"
)

// ErrTournamentNotFound is returned for a tournament with no placed games.
var ErrTournamentNotFound = errors.New("no tournament with games has this name")

// GameSpan is how many games and players a tournament, season or stage has,
// and the days of its first and last game.
type GameSpan struct {
	Games, Players int
	First, Last    time.Time
}

// CatalogueEntry is one tournament with placed games, and the rankings it is rated in.
type CatalogueEntry struct {
	Slug, Name string
	Seasons    int
	Rankings   []string
	Span       GameSpan
}

// CGEEntry is a tournament on CGE's site, with which of ours it is (Tournament,
// "" for none) and whether that one is rated. Started falls back to when it was created.
type CGEEntry struct {
	ID                      int
	Name, Status, Organiser string
	Started, Finished       *time.Time
	Tournament              string
	Rated                   bool
}

// CatalogueView is what the Tournaments page lists: our tournaments with games,
// and CGE's catalogue, newest first.
type CatalogueView struct {
	Tournaments []CatalogueEntry
	CGE         []CGEEntry
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

// CGELink is a CGE tournament page for a season: the whole tournament
// (Season 0), or one of its seasons when it holds several of ours.
type CGELink struct {
	Tournament, Season int
}

// CatalogueSeason is one season with its stages, podium and CGE pages.
type CatalogueSeason struct {
	Label  string
	Span   GameSpan
	Stages []CatalogueStage
	Podium []PodiumPlace
	CGE    []CGELink
	Note   string
}

// leadingNumber is the number a season label starts with.
var leadingNumber = regexp.MustCompile(`^\d+`)

// leagueTops names the division whose standings give a league season's podium.
var leagueTops = map[string][]string{
	"intermezzo":    {"Grandmaster", "Diamond"},
	"international": {"Grandmaster", "Diamond"},
	"royal_league":  {"Emperor"},
	"worlds":        {"Final"},
}

// LadderWin is one player's top-tier wins in a ladder, as its organisers count them.
type LadderWin struct {
	Name   string
	Wins   float64
	Source string
}

// CatalogueTournament is one tournament's seasons, in order of their first
// game. A ladder's champions are its players' top-tier wins, most first.
type CatalogueTournament struct {
	Slug, Name  string
	Seasons     []CatalogueSeason
	Ladder      bool
	TopTierWins []LadderWin
	Players     []string
}

// placedSeats is every placed game's seats, one row per player, with its
// tournament, season and stage, for tournaments the site shows: rated (in a
// ranking) or made visible. Never-played games are not counted.
const placedSeats = `
	SELECT t.slug, t.name, s.season_id, s.label AS season, st.stage_id, st.label AS stage,
	       m.match_id, m.date, p.name AS player
	FROM matches m
	JOIN tournament_stages st USING (stage_id)
	JOIN tournament_seasons s USING (season_id)
	JOIN tournaments t USING (tournament_id)
	CROSS JOIN LATERAL unnest(ARRAY[m.player1, m.player2, m.player3, m.player4]) AS p(name)
	WHERE p.name IS NOT NULL AND m.state = 'played'
	  AND (t.visible OR EXISTS (SELECT 1 FROM ranking_tournaments rt WHERE rt.tournament_id = t.tournament_id))`

// Catalogue returns every shown tournament with placed games, by name, with
// the rankings it is in, and CGE's shown tournaments, newest first, for a
// published run. Shown is rated or made visible; unplaced games count nowhere.
func (r *RatingsReader) Catalogue(ctx context.Context, run int64) (CatalogueView, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return CatalogueView{}, err
	}
	rows, err := r.pool.Query(ctx, `
		WITH g AS (`+placedSeats+`)
		SELECT slug, name, count(DISTINCT season_id), count(DISTINCT match_id), count(DISTINCT player), min(date), max(date),
		       array(SELECT k.slug FROM ranking_tournaments rt JOIN rankings k USING (ranking_id)
		             JOIN tournaments t USING (tournament_id) WHERE t.slug = g.slug ORDER BY k.slug)
		FROM g GROUP BY slug, name ORDER BY name, slug`)
	if err != nil {
		return CatalogueView{}, fmt.Errorf("reading the catalogue: %w", err)
	}
	var got CatalogueView
	got.Tournaments, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (CatalogueEntry, error) {
		var e CatalogueEntry
		return e, row.Scan(&e.Slug, &e.Name, &e.Seasons, &e.Span.Games, &e.Span.Players, &e.Span.First, &e.Span.Last, &e.Rankings)
	})
	if err != nil {
		return CatalogueView{}, fmt.Errorf("reading the catalogue: %w", err)
	}

	rows, err = r.pool.Query(ctx, `
		SELECT c.cge_tournament, c.name, c.status, c.organiser, coalesce(c.started, c.created), c.finished,
		       coalesce(t.slug, ''), EXISTS (SELECT 1 FROM ranking_tournaments rt WHERE rt.tournament_id = c.tournament_id)
		FROM cge_tournaments c LEFT JOIN tournaments t USING (tournament_id)
		WHERE c.visible OR EXISTS (SELECT 1 FROM ranking_tournaments rt WHERE rt.tournament_id = c.tournament_id)
		ORDER BY c.finished DESC NULLS FIRST, coalesce(c.started, c.created) DESC NULLS LAST, c.cge_tournament DESC`)
	if err != nil {
		return CatalogueView{}, fmt.Errorf("reading CGE's catalogue: %w", err)
	}
	got.CGE, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (CGEEntry, error) {
		var c CGEEntry
		return c, row.Scan(&c.ID, &c.Name, &c.Status, &c.Organiser, &c.Started, &c.Finished, &c.Tournament, &c.Rated)
	})
	if err != nil {
		return CatalogueView{}, fmt.Errorf("reading CGE's catalogue: %w", err)
	}
	return got, nil
}

// CatalogueTournament returns a tournament's seasons in order of their first
// game, each with its stages (in the same order) and its recorded podium,
// for a published run. A tournament without placed games, or hidden, is ErrTournamentNotFound.
func (r *RatingsReader) CatalogueTournament(ctx context.Context, run int64, slug string) (CatalogueTournament, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return CatalogueTournament{}, err
	}
	fail := func(what string, err error) (CatalogueTournament, error) {
		return CatalogueTournament{}, fmt.Errorf("reading %s's %s: %w", slug, what, err)
	}
	got := CatalogueTournament{Slug: slug, Seasons: []CatalogueSeason{}, Ladder: slices.Contains(tournamentcsv.Ladders, slug), TopTierWins: []LadderWin{}}
	rows, err := r.pool.Query(ctx, `
		WITH g AS (`+placedSeats+` AND t.slug = $1)
		SELECT name, season_id, season, count(DISTINCT match_id), count(DISTINCT player), min(date), max(date)
		FROM g GROUP BY name, season_id, season ORDER BY min(date), season`, slug)
	if err != nil {
		return fail("seasons", err)
	}
	var ids []int64
	got.Seasons, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (CatalogueSeason, error) {
		s := CatalogueSeason{Stages: []CatalogueStage{}, Podium: []PodiumPlace{}, CGE: []CGELink{}}
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
	cupPodium := map[int]bool{}
	_, err = pgx.ForEachRow(rows, []any{&label, &p.Place, &p.Name, &p.Final, &p.Finished, &p.Source}, func() error {
		if i, ok := byLabel[label]; ok {
			got.Seasons[i].Podium = append(got.Seasons[i].Podium, p)
			cupPodium[i] = true
		}
		return nil
	})
	if err != nil {
		return fail("podiums", err)
	}

	// A league's podium: its recorded placements, else the run's top-division standings.
	rows, err = r.pool.Query(ctx, `
		WITH official AS (
			SELECT season, place, player_name, source FROM tournament_official_placements
			WHERE series = $1 AND division = ANY($3) AND place <= 3),
		derived AS (
			SELECT season, place, player_name, division || ' standings' AS source FROM tournament_standings
			WHERE run_id = $2 AND series = $1 AND division = ANY($3) AND place <= 3
			  AND season NOT IN (SELECT season FROM official))
		SELECT season::text, place, player_name, source FROM (SELECT * FROM official UNION ALL SELECT * FROM derived) p
		ORDER BY season, place, player_name`, slug, run, leagueTops[slug])
	if err != nil {
		return fail("league podiums", err)
	}
	// A league's seasons are numbered; one labelled "2022 (Age A)" is found by its number, a "2023 Play Off" is not.
	byNumber := make(map[string]int, len(got.Seasons))
	for i, s := range got.Seasons {
		if n := leadingNumber.FindString(s.Label); n != "" && (s.Label == n || strings.HasPrefix(s.Label, n+" (")) {
			byNumber[n] = i
		}
	}
	_, err = pgx.ForEachRow(rows, []any{&label, &p.Place, &p.Name, &p.Source}, func() error {
		if i, ok := byNumber[label]; ok && !cupPodium[i] {
			p.Final, p.Finished = "", got.Seasons[i].Span.Last
			got.Seasons[i].Podium = append(got.Seasons[i].Podium, p)
		}
		return nil
	})
	if err != nil {
		return fail("league podiums", err)
	}

	rows, err = r.pool.Query(ctx, `
		WITH g AS (
			SELECT s.label, m.cge_tournament AS ct, m.cge_season AS cs
			FROM matches m
			JOIN tournament_stages st USING (stage_id)
			JOIN tournament_seasons s USING (season_id)
			JOIN tournaments t USING (tournament_id)
			WHERE t.slug = $1 AND m.cge_tournament IS NOT NULL
			UNION
			SELECT s.label, st.cge_tournament, st.cge_season
			FROM tournament_stages st
			JOIN tournament_seasons s USING (season_id)
			JOIN tournaments t USING (tournament_id)
			WHERE t.slug = $1 AND st.cge_tournament IS NOT NULL),
		spans AS (SELECT ct, count(DISTINCT label) AS seasons FROM g GROUP BY ct)
		SELECT DISTINCT g.label, g.ct, CASE WHEN spans.seasons > 1 THEN g.cs ELSE 0 END AS cs
		FROM g JOIN spans USING (ct) ORDER BY 2, 3`, slug)
	if err != nil {
		return fail("CGE pages", err)
	}
	var link CGELink
	_, err = pgx.ForEachRow(rows, []any{&label, &link.Tournament, &link.Season}, func() error {
		if i, ok := byLabel[label]; ok {
			got.Seasons[i].CGE = append(got.Seasons[i].CGE, link)
		}
		return nil
	})
	if err != nil {
		return fail("CGE pages", err)
	}

	rows, err = r.pool.Query(ctx, "SELECT season, note FROM tournament_season_notes WHERE tournament = $1", slug)
	if err != nil {
		return fail("season notes", err)
	}
	var note string
	_, err = pgx.ForEachRow(rows, []any{&label, &note}, func() error {
		if i, ok := byLabel[label]; ok {
			got.Seasons[i].Note = note
		}
		return nil
	})
	if err != nil {
		return fail("season notes", err)
	}

	rows, err = r.pool.Query(ctx, `
		SELECT player_name, wins::float8, source FROM tournament_ladder_wins
		WHERE ladder = $1 ORDER BY wins DESC, player_name`, slug)
	if err != nil {
		return fail("top-tier wins", err)
	}
	if got.TopTierWins, err = pgx.CollectRows(rows, pgx.RowToStructByPos[LadderWin]); err != nil {
		return fail("top-tier wins", err)
	}

	// Every player of the shown tournament, for a search box's suggestions.
	rows, err = r.pool.Query(ctx, `
		SELECT DISTINCT p.name FROM (`+shownGames+`) g JOIN matches m USING (match_id)
		CROSS JOIN LATERAL unnest(ARRAY[m.player1, m.player2, m.player3, m.player4]) AS p(name)
		WHERE p.name IS NOT NULL ORDER BY p.name`, slug, "", "", "")
	if err != nil {
		return fail("players", err)
	}
	if got.Players, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
		return fail("players", err)
	}
	return got, nil
}
