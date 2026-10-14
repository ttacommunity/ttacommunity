package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
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

// CatalogueEntry is one tournament with placed games, and the rankings it is rated in.
type CatalogueEntry struct {
	Slug, Name string
	Seasons    int
	Rankings   []string
	Span       GameSpan
	Card       Card
}

// Card is how a tournament presents itself on its card and its page's header, as admin writes it; empty fields
// are unset.
type Card struct {
	Icon, Tagline, Summary string
	// Subtitle and Cadence are the page header's line under the name and its cadence figure.
	Subtitle, Cadence string
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
	Note  string
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
	Label string
	// InChampions is false for a season whose winner admin leaves out of the champions, such as a play-off.
	InChampions bool
	Span        GameSpan
	Stages      []CatalogueStage
	Podium      []PodiumPlace
	CGE         []CGELink
	Note        string
}

// leadingNumber is the number a season label starts with.
var leadingNumber = regexp.MustCompile(`^\d+`)

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
	Rules       []EditionRules
	// Regulations is the format and regulations written in admin, shown instead of the rules; empty for none.
	Regulations string
	Card        Card
	// Rated is whether the tournament is in a ranking, so its games move ratings.
	Rated bool
	// GameRecords are its ten highest winning scores in played games.
	GameRecords []GameRecord
	Stories     []Story
	// SeasonName and StageName are what it calls its seasons and stages, empty for the defaults.
	SeasonName, StageName string
}

// Story is one of a tournament's stories, as DANeo's series pages tell them: a player, a badge, a title and the
// story itself, all as admin writes them.
type Story struct {
	Player, Badge, Title, Text string
}

// stories returns a tournament's stories in their order.
func stories(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, slug string) ([]Story, error) {
	rows, err := q.Query(ctx, "SELECT player_name, badge, title, text FROM tournament_stories WHERE tournament = $1 ORDER BY position", slug)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Story])
}

// EditionRules is one CGE tournament's description of a tournament edition
// (or a league's whole run): usually its organiser's format and rules.
type EditionRules struct {
	CGETournament int
	Name          string
	Description   string
}

// Catalogue returns every shown tournament with placed games, by name, with
// the rankings it is in, and CGE's shown tournaments, newest first, for a
// published run. Shown is rated or made visible; the games are counted as the
// run counted them (its catalogue spans), and unplaced games count nowhere.
func (r *RatingsReader) Catalogue(ctx context.Context, run int64) (CatalogueView, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return CatalogueView{}, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT t.slug, t.name,
		       (SELECT count(*) FROM catalogue_spans se WHERE se.run_id = $1 AND se.series_id = t.series_id
		          AND se.tournament_id IS NOT NULL AND se.stage_id IS NULL),
		       s.games, s.players, s.first_game, s.last_game,
		       array(SELECT k.slug FROM ranking_series rt JOIN rankings k USING (ranking_id)
		             WHERE rt.series_id = t.series_id ORDER BY k.slug),
		       coalesce(c.icon, ''), coalesce(c.tagline, ''), coalesce(c.summary, ''),
		       coalesce(c.subtitle, ''), coalesce(c.cadence, '')
		FROM catalogue_spans s JOIN series t USING (series_id) LEFT JOIN tournament_cards c ON c.tournament = t.slug
		WHERE s.run_id = $1 AND s.tournament_id IS NULL
		  AND (t.visible OR EXISTS (SELECT 1 FROM ranking_series rt WHERE rt.series_id = t.series_id))
		ORDER BY t.name, t.slug`, run)
	if err != nil {
		return CatalogueView{}, fmt.Errorf("reading the catalogue: %w", err)
	}
	var got CatalogueView
	got.Tournaments, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (CatalogueEntry, error) {
		var e CatalogueEntry
		return e, row.Scan(&e.Slug, &e.Name, &e.Seasons, &e.Span.Games, &e.Span.Players, &e.Span.First, &e.Span.Last, &e.Rankings,
			&e.Card.Icon, &e.Card.Tagline, &e.Card.Summary, &e.Card.Subtitle, &e.Card.Cadence)
	})
	if err != nil {
		return CatalogueView{}, fmt.Errorf("reading the catalogue: %w", err)
	}

	rows, err = r.pool.Query(ctx, `
		SELECT c.cge_tournament, c.name, c.status, c.organiser, coalesce(c.started, c.created), c.finished,
		       coalesce(t.slug, ''), EXISTS (SELECT 1 FROM ranking_series rt WHERE rt.series_id = c.series_id)
		FROM cge_tournaments c LEFT JOIN series t ON t.series_id = c.series_id
		WHERE c.visible OR EXISTS (SELECT 1 FROM ranking_series rt WHERE rt.series_id = c.series_id)
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
// for a published run, counted as the run counted them. A tournament without
// placed games, or hidden, is ErrTournamentNotFound.
func (r *RatingsReader) CatalogueTournament(ctx context.Context, run int64, slug string) (CatalogueTournament, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return CatalogueTournament{}, err
	}
	fail := func(what string, err error) (CatalogueTournament, error) {
		return CatalogueTournament{}, fmt.Errorf("reading %s's %s: %w", slug, what, err)
	}
	got := CatalogueTournament{Slug: slug, Seasons: []CatalogueSeason{}, TopTierWins: []LadderWin{}}
	var top []string
	err := r.pool.QueryRow(ctx, "SELECT kind = 'ladder', top_divisions, tournament_noun, stage_noun FROM series WHERE slug = $1", slug).
		Scan(&got.Ladder, &top, &got.SeasonName, &got.StageName)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fail("kind", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT t.name, s.tournament_id, se.label, s.games, s.players, s.first_game, s.last_game
		FROM catalogue_spans s JOIN series t USING (series_id) JOIN tournaments se ON se.tournament_id = s.tournament_id
		WHERE s.run_id = $2 AND t.slug = $1 AND s.stage_id IS NULL
		  AND (t.visible OR EXISTS (SELECT 1 FROM ranking_series rt WHERE rt.series_id = t.series_id))
		ORDER BY s.first_game, se.label`, slug, run)
	if err != nil {
		return fail("seasons", err)
	}
	var ids []int64
	got.Seasons, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (CatalogueSeason, error) {
		s := CatalogueSeason{InChampions: true, Stages: []CatalogueStage{}, Podium: []PodiumPlace{}, CGE: []CGELink{}}
		var id int64
		err := row.Scan(&got.Name, &id, &s.Label, &s.Span.Games, &s.Span.Players, &s.Span.First, &s.Span.Last)
		ids = append(ids, id)
		return s, err
	})
	if err != nil {
		return fail("seasons", err)
	}
	if err := r.addSeasonsWithoutGames(ctx, slug, &got, &ids); err != nil {
		return fail("seasons without games", err)
	}
	if len(got.Seasons) == 0 {
		return CatalogueTournament{}, ErrTournamentNotFound
	}
	bySeason := make(map[int64]int, len(ids))
	byLabel := make(map[string]int, len(ids))
	for i, id := range ids {
		bySeason[id], byLabel[got.Seasons[i].Label] = i, i
	}

	rows, err = r.pool.Query(ctx, `SELECT se.label FROM tournaments se JOIN series t USING (series_id)
		WHERE t.slug = $1 AND NOT se.in_champions`, slug)
	if err != nil {
		return fail("champion seasons", err)
	}
	var left string
	_, err = pgx.ForEachRow(rows, []any{&left}, func() error {
		if i, ok := byLabel[left]; ok {
			got.Seasons[i].InChampions = false
		}
		return nil
	})
	if err != nil {
		return fail("champion seasons", err)
	}

	rows, err = r.pool.Query(ctx, `
		SELECT s.tournament_id, st.label, s.games, s.players, s.first_game, s.last_game
		FROM catalogue_spans s JOIN series t USING (series_id) JOIN stages st ON st.stage_id = s.stage_id
		WHERE s.run_id = $2 AND t.slug = $1
		  AND (t.visible OR EXISTS (SELECT 1 FROM ranking_series rt WHERE rt.series_id = t.series_id))
		ORDER BY s.first_game, st.label`, slug, run)
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
		ORDER BY season, place, player_name`, slug, run, top)
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

	// A podium set in admin replaces the season's recorded or derived one.
	rows, err = r.pool.Query(ctx, `
		SELECT season, place, player_name, CASE WHEN note = '' THEN 'Set in admin' ELSE 'Set in admin: ' || note END
		FROM tournament_podium_overrides WHERE tournament = $1 ORDER BY season, place`, slug)
	if err != nil {
		return fail("admin podiums", err)
	}
	set := map[int]bool{}
	_, err = pgx.ForEachRow(rows, []any{&label, &p.Place, &p.Name, &p.Source}, func() error {
		i, ok := byLabel[label]
		if !ok {
			return nil
		}
		if !set[i] {
			got.Seasons[i].Podium, set[i] = nil, true
		}
		p.Final, p.Finished = "", got.Seasons[i].Span.Last
		got.Seasons[i].Podium = append(got.Seasons[i].Podium, p)
		return nil
	})
	if err != nil {
		return fail("admin podiums", err)
	}

	rows, err = r.pool.Query(ctx, `
		WITH g AS (
			SELECT s.label, gr.cge_tournament AS ct, gr.cge_stage_number AS cs
			FROM groups gr
			JOIN stages st USING (stage_id)
			JOIN tournaments s USING (tournament_id)
			JOIN series t USING (series_id)
			WHERE t.slug = $1 AND gr.cge_tournament IS NOT NULL
			  AND EXISTS (SELECT 1 FROM games m WHERE m.group_id = gr.group_id)
			UNION
			SELECT s.label, st.cge_tournament, st.cge_stage_number
			FROM stages st
			JOIN tournaments s USING (tournament_id)
			JOIN series t USING (series_id)
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

	rows, err = r.pool.Query(ctx, "SELECT season, stage, note FROM tournament_season_notes WHERE tournament = $1", slug)
	if err != nil {
		return fail("season notes", err)
	}
	var stage, note string
	_, err = pgx.ForEachRow(rows, []any{&label, &stage, &note}, func() error {
		i, ok := byLabel[label]
		if !ok {
			return nil
		}
		if stage == "" {
			got.Seasons[i].Note = note
			return nil
		}
		for j := range got.Seasons[i].Stages {
			if got.Seasons[i].Stages[j].Label == stage {
				got.Seasons[i].Stages[j].Note = note
			}
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

	rows, err = r.pool.Query(ctx, `
		SELECT c.cge_tournament, c.name, c.description FROM cge_tournaments c JOIN series t USING (series_id)
		WHERE t.slug = $1 AND c.description <> ''
		ORDER BY coalesce(c.started, c.created, c.finished) DESC NULLS LAST, c.cge_tournament DESC`, slug)
	if err != nil {
		return fail("rules", err)
	}
	if got.Rules, err = pgx.CollectRows(rows, pgx.RowToStructByPos[EditionRules]); err != nil {
		return fail("rules", err)
	}
	err = r.pool.QueryRow(ctx, "SELECT coalesce((SELECT text FROM tournament_regulations WHERE tournament = $1), '')", slug).Scan(&got.Regulations)
	if err != nil {
		return fail("regulations", err)
	}
	err = r.pool.QueryRow(ctx, `SELECT coalesce(max(icon), ''), coalesce(max(tagline), ''), coalesce(max(summary), ''),
		coalesce(max(subtitle), ''), coalesce(max(cadence), '')
		FROM tournament_cards WHERE tournament = $1`, slug).Scan(&got.Card.Icon, &got.Card.Tagline, &got.Card.Summary,
		&got.Card.Subtitle, &got.Card.Cadence)
	if err != nil {
		return fail("card", err)
	}
	err = r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM ranking_series rt JOIN series t USING (series_id)
		WHERE t.slug = $1)`, slug).Scan(&got.Rated)
	if err != nil {
		return fail("rated", err)
	}
	// The best scores come from the seats; game_lines builds its arrays only for the games shown.
	rows, err = r.pool.Query(ctx, `
		WITH top AS (
		    SELECT g.game_id, max(p.culture) AS best
		    FROM games g JOIN seats p USING (game_id) JOIN groups gr USING (group_id) JOIN stages st USING (stage_id)
		    JOIN tournaments s USING (tournament_id) JOIN series t USING (series_id)
		    WHERE t.slug = $1 AND g.state = 'played'
		    GROUP BY g.game_id ORDER BY best DESC, g.game_id LIMIT $2)
		SELECT m.name, m.date, m.players, m.cultures, m.places
		FROM top JOIN game_lines m USING (game_id) ORDER BY top.best DESC, top.game_id`, slug, recordsShown)
	if err != nil {
		return fail("game records", err)
	}
	if got.GameRecords, err = pgx.CollectRows(rows, scanGameRecord); err != nil {
		return fail("game records", err)
	}
	if err := r.maskGameRecords(ctx, got.GameRecords); err != nil {
		return fail("game records", err)
	}
	if err := r.markWorldChampions(ctx, run, got.GameRecords); err != nil {
		return fail("game records", err)
	}
	if got.Stories, err = stories(ctx, r.pool, slug); err != nil {
		return fail("stories", err)
	}

	// Every player of the shown tournament, for a search box's suggestions.
	rows, err = r.pool.Query(ctx, `
		SELECT DISTINCT p.name
		FROM games g JOIN seats s USING (game_id) JOIN players p USING (player_id)
		JOIN groups gr USING (group_id) JOIN stages st USING (stage_id)
		JOIN tournaments se USING (tournament_id) JOIN series t USING (series_id)
		WHERE t.slug = $1 AND (t.visible OR EXISTS (SELECT 1 FROM ranking_series rt WHERE rt.series_id = t.series_id))
		  AND `+notHiddenPlayer+`
		ORDER BY p.name`, slug)
	if err != nil {
		return fail("players", err)
	}
	if got.Players, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
		return fail("players", err)
	}
	return got, nil
}

// addSeasonsWithoutGames adds the tournament's seasons that have no games yet (admin can add one, to carry
// its note), each before the first season with a higher leading number, or last.
func (r *RatingsReader) addSeasonsWithoutGames(ctx context.Context, slug string, got *CatalogueTournament, ids *[]int64) error {
	rows, err := r.pool.Query(ctx, `
		SELECT t.name, se.tournament_id, se.label FROM series t JOIN tournaments se USING (series_id)
		WHERE t.slug = $1 AND NOT EXISTS (
			SELECT 1 FROM stages st JOIN groups gr USING (stage_id) JOIN games m USING (group_id)
			WHERE st.tournament_id = se.tournament_id)
		ORDER BY se.tournament_id`, slug)
	if err != nil {
		return err
	}
	var name, label string
	var id int64
	_, err = pgx.ForEachRow(rows, []any{&name, &id, &label}, func() error {
		got.Name = name
		at := len(got.Seasons)
		if n, err := strconv.Atoi(leadingNumber.FindString(label)); err == nil {
			for i, s := range got.Seasons {
				if m, err := strconv.Atoi(leadingNumber.FindString(s.Label)); err == nil && m > n {
					at = i
					break
				}
			}
		}
		s := CatalogueSeason{Label: label, InChampions: true, Stages: []CatalogueStage{}, Podium: []PodiumPlace{}, CGE: []CGELink{}}
		got.Seasons = slices.Insert(got.Seasons, at, s)
		*ids = slices.Insert(*ids, at, id)
		return nil
	})
	return err
}
