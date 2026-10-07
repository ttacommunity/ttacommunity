// Tournament settings in admin: shown, rated, counted in the community leaderboard, and season and stage notes.

package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// ErrNoTournament means no tournament, or no season of it, has that name.
var ErrNoTournament = errors.New("no such tournament or season")

// AdminTournament is one tournament as admin edits it: Regulations is admin's format and regulations text,
// shown instead of CGERegulations, the latest edition's CGE description; either is empty for none.
type AdminTournament struct {
	Slug, Name        string
	Visible           bool
	Rated             bool
	Community         bool
	Regulations       string
	CGERegulations    string
	Card              Card
	Stories           []Story
	RatedPlayerCounts []int
	Kind              string
	TopDivisions      []string
	// Points are the points per place, by player count (2, 3 and 4), empty for a count it does not score.
	Points map[int][]float64
	// OriginalName is the name the tournament came in with, kept when it is renamed.
	OriginalName string
	Seasons      []AdminSeason
}

// AdminSeason is a season with its own note, its stages, and the podium admin set for it, if any.
type AdminSeason struct {
	Label, OriginalName string
	Note                string
	HasGames            bool
	Rated               bool
	// InChampions is whether its winner is listed among the tournament's champions.
	InChampions bool
	// TimeoutPenalty is the points a player loses for each game they time out of, 0 or below.
	TimeoutPenalty float64
	Stages         []AdminStage
	Podium         []PodiumEntry
	PodiumNote     string
}

// PodiumEntry is one place on a podium set in admin.
type PodiumEntry struct {
	Place int
	Name  string
}

// AdminStage is a stage of a season (an empty label is a season without stages) with its note.
type AdminStage struct {
	Label, OriginalName string
	Note                string
}

// TournamentSettings are the settings to change; a nil field is left as it is. RatedPlayerCounts are the player
// counts whose games count towards ratings (2, 3, 4).
type TournamentSettings struct {
	Visible, Rated, Community *bool
	RatedPlayerCounts         []int
	// Kind is league, cup or ladder ("" unchanged); TopDivisions give a league's podium (nil unchanged).
	Kind         string
	TopDivisions []string
	// Points replace the points per place for each player count given; an empty list clears one.
	Points map[int][]float64
}

// ErrBadScoring means points for a player count other than 2, 3 or 4, not one per place, negative,
// or higher for a worse place.
var ErrBadScoring = errors.New("points are one per place, never negative, and never higher for a worse place")

// ErrBadKind means a kind other than league, cup or ladder.
var ErrBadKind = errors.New("a tournament is a league, a cup or a ladder")

// ErrBadPlayerCounts means rated player counts that are empty or not among 2, 3 and 4.
var ErrBadPlayerCounts = errors.New("rated player counts are one or more of 2, 3 and 4")

// Tournaments returns every tournament with its settings, seasons (latest first), stages and notes, in name order.
func (a *AdminStore) Tournaments(ctx context.Context) ([]AdminTournament, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT t.slug, t.name, t.visible, t.in_community_leaderboard,
		       EXISTS (SELECT 1 FROM ranking_tournaments rt JOIN rankings r USING (ranking_id)
		               WHERE r.slug = 'community' AND rt.tournament_id = t.tournament_id),
		       coalesce((SELECT text FROM tournament_regulations g WHERE g.tournament = t.slug), ''),
		       coalesce(k.icon, ''), coalesce(k.tagline, ''), coalesce(k.summary, ''), coalesce(k.subtitle, ''), coalesce(k.cadence, ''),
		       coalesce((SELECT c.description FROM cge_tournaments c WHERE c.tournament_id = t.tournament_id AND c.description <> ''
		                 ORDER BY coalesce(c.started, c.created, c.finished) DESC NULLS LAST, c.cge_tournament DESC LIMIT 1), ''),
		       se.label, coalesce(st.label, ''), coalesce(se.original_name, ''), coalesce(st.original_name, ''),
		       coalesce((SELECT note FROM tournament_season_notes n WHERE n.tournament = t.slug AND n.season = se.label AND n.stage = ''), ''),
		       coalesce((SELECT note FROM tournament_season_notes n WHERE n.tournament = t.slug AND n.season = se.label AND n.stage = st.label AND st.label <> ''), ''),
		       d.last IS NOT NULL, coalesce(se.rated, true), coalesce(se.in_champions, true), t.rated_player_counts, t.kind, t.top_divisions, t.original_name,
		       t.points_2p, t.points_3p, t.points_4p,
		       coalesce((SELECT r.timeout_penalty FROM tournament_season_rules r WHERE r.series = t.slug AND r.season::text = se.label), 0)
		FROM tournaments t
		LEFT JOIN tournament_cards k ON k.tournament = t.slug
		LEFT JOIN tournament_seasons se USING (tournament_id)
		LEFT JOIN tournament_stages st USING (season_id)
		LEFT JOIN LATERAL (SELECT max(m.date) AS last FROM tournament_stages s2 JOIN matches m ON m.stage_id = s2.stage_id
		                   WHERE s2.season_id = se.season_id) d ON true
		ORDER BY t.name, coalesce(substring(se.label FROM '^[0-9]+')::int, -1) DESC, d.last DESC NULLS LAST,
		         se.label DESC, st.stage_id`)
	if err != nil {
		return nil, fmt.Errorf("reading tournaments for admin: %w", err)
	}
	var out []AdminTournament
	var slug, name string
	var visible, community, rated bool
	var regulations, cgeRegulations string
	var card Card
	var season, stage, seasonNote, stageNote *string
	var seasonOriginal, stageOriginal string
	var hasGames, seasonRated, inChampions bool
	var counts []int
	var kind, original string
	var top []string
	var p2, p3, p4 []float64
	var penalty float64
	_, err = pgx.ForEachRow(rows, []any{&slug, &name, &visible, &community, &rated, &regulations,
		&card.Icon, &card.Tagline, &card.Summary, &card.Subtitle, &card.Cadence, &cgeRegulations, &season, &stage, &seasonOriginal, &stageOriginal, &seasonNote, &stageNote, &hasGames,
		&seasonRated, &inChampions, &counts, &kind, &top, &original, &p2, &p3, &p4, &penalty}, func() error {
		if len(out) == 0 || out[len(out)-1].Slug != slug {
			out = append(out, AdminTournament{Slug: slug, Name: name, Visible: visible, Rated: rated, Community: community,
				Regulations: regulations, CGERegulations: cgeRegulations, Card: card, RatedPlayerCounts: counts,
				Kind: kind, TopDivisions: top, Points: map[int][]float64{2: p2, 3: p3, 4: p4}, OriginalName: original, Seasons: []AdminSeason{}})
		}
		t := &out[len(out)-1]
		if season == nil {
			return nil
		}
		if len(t.Seasons) == 0 || t.Seasons[len(t.Seasons)-1].Label != *season {
			t.Seasons = append(t.Seasons, AdminSeason{Label: *season, OriginalName: seasonOriginal, Note: *seasonNote, HasGames: hasGames, Rated: seasonRated, InChampions: inChampions, TimeoutPenalty: penalty, Stages: []AdminStage{}})
		}
		s := &t.Seasons[len(t.Seasons)-1]
		if *stage != "" {
			s.Stages = append(s.Stages, AdminStage{Label: *stage, OriginalName: stageOriginal, Note: *stageNote})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading tournaments for admin: %w", err)
	}
	if err := a.addPodiums(ctx, out); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Stories, err = stories(ctx, a.pool, out[i].Slug); err != nil {
			return nil, fmt.Errorf("reading %s's stories for admin: %w", out[i].Slug, err)
		}
	}
	return out, nil
}

// addPodiums puts each podium set in admin on its season.
func (a *AdminStore) addPodiums(ctx context.Context, ts []AdminTournament) error {
	rows, err := a.pool.Query(ctx, "SELECT tournament, season, place, player_name, note FROM tournament_podium_overrides ORDER BY place")
	if err != nil {
		return fmt.Errorf("reading podiums for admin: %w", err)
	}
	seasons := map[[2]string]*AdminSeason{}
	for i := range ts {
		for j := range ts[i].Seasons {
			seasons[[2]string{ts[i].Slug, ts[i].Seasons[j].Label}] = &ts[i].Seasons[j]
		}
	}
	var slug, season, name, note string
	var place int
	_, err = pgx.ForEachRow(rows, []any{&slug, &season, &place, &name, &note}, func() error {
		if se, ok := seasons[[2]string{slug, season}]; ok {
			se.Podium = append(se.Podium, PodiumEntry{Place: place, Name: name})
			if note != "" {
				se.PodiumNote = note
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("reading podiums for admin: %w", err)
	}
	return nil
}

// ErrBadPodium means a podium has nobody, a place outside 1-3 or twice, a player twice, or a blank name.
var ErrBadPodium = errors.New("a podium is 1st to 3rd, each place and player once")

// SetPodium replaces a season's podium with the one given, which the site shows over a recorded or derived
// one, with the note on its first place. It returns ErrNoTournament or ErrBadPodium.
func (a *AdminStore) SetPodium(ctx context.Context, slug, season string, podium []PodiumEntry, note string) error {
	places, names := map[int]bool{}, map[string]bool{}
	for _, e := range podium {
		name := strings.TrimSpace(e.Name)
		if e.Place < 1 || e.Place > 3 || places[e.Place] || name == "" || names[name] {
			return ErrBadPodium
		}
		places[e.Place], names[name] = true, true
	}
	if len(podium) == 0 {
		return ErrBadPodium
	}
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		if err := seasonExists(ctx, tx, slug, season); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM tournament_podium_overrides WHERE tournament = $1 AND season = $2", slug, season); err != nil {
			return fmt.Errorf("setting the podium of %s %s: %w", slug, season, err)
		}
		for _, e := range podium {
			n := ""
			if e.Place == podium[0].Place {
				n = note
			}
			if _, err := tx.Exec(ctx, `INSERT INTO tournament_podium_overrides (tournament, season, place, player_name, note)
				VALUES ($1, $2, $3, $4, $5)`, slug, season, e.Place, strings.TrimSpace(e.Name), n); err != nil {
				return fmt.Errorf("setting the podium of %s %s: %w", slug, season, err)
			}
		}
		return nil
	})
}

// ClearPodium removes the podium admin set for a season, so the recorded or derived one shows again.
// It returns ErrNoTournament for a season that does not exist.
func (a *AdminStore) ClearPodium(ctx context.Context, slug, season string) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		if err := seasonExists(ctx, tx, slug, season); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM tournament_podium_overrides WHERE tournament = $1 AND season = $2", slug, season); err != nil {
			return fmt.Errorf("clearing the podium of %s %s: %w", slug, season, err)
		}
		return nil
	})
}

// seasonExists returns ErrNoTournament unless the tournament has that season.
func seasonExists(ctx context.Context, tx pgx.Tx, slug, season string) error {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tournaments t JOIN tournament_seasons se USING (tournament_id)
		WHERE t.slug = $1 AND se.label = $2)`, slug, season).Scan(&exists)
	if err != nil {
		return fmt.Errorf("finding %s %s: %w", slug, season, err)
	}
	if !exists {
		return ErrNoTournament
	}
	return nil
}

// validScoring reports whether points suit a game of that many players: none, or one per place,
// never negative and never higher for a worse place.
func validScoring(players int, points []float64) bool {
	if players < 2 || players > 4 || (len(points) != 0 && len(points) != players) {
		return false
	}
	for i, p := range points {
		if p < 0 || (i > 0 && p > points[i-1]) {
			return false
		}
	}
	return true
}

// SetTournament changes a tournament's settings in one transaction, or returns ErrNoTournament.
func (a *AdminStore) SetTournament(ctx context.Context, slug string, s TournamentSettings) error {
	if s.Kind != "" && s.Kind != "league" && s.Kind != "cup" && s.Kind != "ladder" {
		return ErrBadKind
	}
	if s.TopDivisions != nil {
		top := make([]string, 0, len(s.TopDivisions))
		for _, d := range s.TopDivisions {
			if d = strings.TrimSpace(d); d != "" && !slices.Contains(top, d) {
				top = append(top, d)
			}
		}
		s.TopDivisions = top
	}
	if s.RatedPlayerCounts != nil {
		counts := slices.Sorted(slices.Values(s.RatedPlayerCounts))
		counts = slices.Compact(counts)
		if len(counts) == 0 || counts[0] < 2 || counts[len(counts)-1] > 4 {
			return ErrBadPlayerCounts
		}
		s.RatedPlayerCounts = counts
	}
	for players, points := range s.Points {
		if !validScoring(players, points) {
			return ErrBadScoring
		}
	}
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var id int64
		if err := tx.QueryRow(ctx, "SELECT tournament_id FROM tournaments WHERE slug = $1", slug).Scan(&id); errors.Is(err, pgx.ErrNoRows) {
			return ErrNoTournament
		} else if err != nil {
			return fmt.Errorf("setting %s: %w", slug, err)
		}
		if s.Visible != nil {
			if _, err := tx.Exec(ctx, "UPDATE tournaments SET visible = $2 WHERE tournament_id = $1", id, *s.Visible); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		if s.Kind != "" {
			if _, err := tx.Exec(ctx, "UPDATE tournaments SET kind = $2 WHERE tournament_id = $1", id, s.Kind); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		if s.TopDivisions != nil {
			if _, err := tx.Exec(ctx, "UPDATE tournaments SET top_divisions = $2 WHERE tournament_id = $1", id, s.TopDivisions); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		if s.RatedPlayerCounts != nil {
			if _, err := tx.Exec(ctx, "UPDATE tournaments SET rated_player_counts = $2 WHERE tournament_id = $1", id, s.RatedPlayerCounts); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		for players, points := range s.Points {
			if points == nil {
				points = []float64{}
			}
			column := pgx.Identifier{fmt.Sprintf("points_%dp", players)}.Sanitize()
			if _, err := tx.Exec(ctx, "UPDATE tournaments SET "+column+" = $2 WHERE tournament_id = $1", id, points); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		if s.Community != nil {
			if _, err := tx.Exec(ctx, "UPDATE tournaments SET in_community_leaderboard = $2 WHERE tournament_id = $1", id, *s.Community); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		if s.Rated != nil {
			q := `DELETE FROM ranking_tournaments WHERE tournament_id = $1 AND ranking_id = community_ranking()`
			if *s.Rated {
				q = `INSERT INTO ranking_tournaments (ranking_id, tournament_id) VALUES (community_ranking(), $1) ON CONFLICT DO NOTHING`
			}
			if _, err := tx.Exec(ctx, q, id); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		return nil
	})
}

// SetNote saves the note shown beside a season (stage "") or one of its stages; an empty note removes it.
// It returns ErrNoTournament when the tournament has no such season or stage.
func (a *AdminStore) SetNote(ctx context.Context, slug, season, stage, note string) error {
	var exists bool
	if err := a.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM tournaments t JOIN tournament_seasons se USING (tournament_id)
		               LEFT JOIN tournament_stages st USING (season_id)
		               WHERE t.slug = $1 AND se.label = $2 AND ($3 = '' OR st.label = $3))`, slug, season, stage).Scan(&exists); err != nil {
		return fmt.Errorf("noting %s %s: %w", slug, season, err)
	}
	if !exists {
		return ErrNoTournament
	}
	if note == "" {
		_, err := a.pool.Exec(ctx, "DELETE FROM tournament_season_notes WHERE tournament = $1 AND season = $2 AND stage = $3", slug, season, stage)
		return err
	}
	_, err := a.pool.Exec(ctx, `
		INSERT INTO tournament_season_notes (tournament, season, stage, note, source) VALUES ($1, $2, $3, $4, 'admin')
		ON CONFLICT (tournament, season, stage) DO UPDATE SET note = excluded.note, source = excluded.source`, slug, season, stage, note)
	if err != nil {
		return fmt.Errorf("noting %s %s: %w", slug, season, err)
	}
	return nil
}

// ErrSeasonExists means the tournament already has a season with that label.
var ErrSeasonExists = errors.New("the tournament already has that season")

// ErrSeasonHasGames means the season has stages (and so games), and cannot be removed in admin.
var ErrSeasonHasGames = errors.New("the season has games")

// seasonPrefix is a leading "Season", which the site adds to a label itself.
var seasonPrefix = regexp.MustCompile(`(?i)^season\s+`)

// AddSeason adds a season with no games yet to a tournament, so a note can be shown for it. A leading
// "Season" is dropped from the label ("Season 1" is season "1"), as the site adds it.
func (a *AdminStore) AddSeason(ctx context.Context, slug, label string) error {
	typed := strings.TrimSpace(label)
	label = seasonPrefix.ReplaceAllString(typed, "")
	tag, err := a.pool.Exec(ctx, `
		INSERT INTO tournament_seasons (tournament_id, label, original_name)
		SELECT tournament_id, $2, $3 FROM tournaments WHERE slug = $1
		ON CONFLICT (tournament_id, label) DO NOTHING`, slug, label, typed)
	if err != nil {
		return fmt.Errorf("adding season %s of %s: %w", label, slug, err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var exists bool
	if err := a.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM tournaments WHERE slug = $1)", slug).Scan(&exists); err != nil {
		return fmt.Errorf("adding season %s of %s: %w", label, slug, err)
	}
	if !exists {
		return ErrNoTournament
	}
	return ErrSeasonExists
}

// RemoveSeason removes a season that has no stages (no games), and its notes. It returns ErrNoTournament for a
// season that does not exist and ErrSeasonHasGames for one with stages.
func (a *AdminStore) RemoveSeason(ctx context.Context, slug, label string) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var id int64
		var stages bool
		err := tx.QueryRow(ctx, `
			SELECT se.season_id, EXISTS (SELECT 1 FROM tournament_stages st WHERE st.season_id = se.season_id)
			FROM tournaments t JOIN tournament_seasons se USING (tournament_id) WHERE t.slug = $1 AND se.label = $2`,
			slug, label).Scan(&id, &stages)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoTournament
		}
		if err != nil {
			return fmt.Errorf("removing season %s of %s: %w", label, slug, err)
		}
		if stages {
			return ErrSeasonHasGames
		}
		if _, err := tx.Exec(ctx, "DELETE FROM tournament_season_notes WHERE tournament = $1 AND season = $2", slug, label); err != nil {
			return fmt.Errorf("removing season %s of %s: %w", label, slug, err)
		}
		if _, err := tx.Exec(ctx, "DELETE FROM tournament_seasons WHERE season_id = $1", id); err != nil {
			return fmt.Errorf("removing season %s of %s: %w", label, slug, err)
		}
		return nil
	})
}

// SetRegulations saves a tournament's format and regulations text, shown instead of CGE's; an empty text
// removes it. It returns ErrNoTournament for a tournament that does not exist.
func (a *AdminStore) SetRegulations(ctx context.Context, slug, text string) error {
	text = strings.TrimSpace(text)
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM tournaments WHERE slug = $1)", slug).Scan(&exists); err != nil {
			return fmt.Errorf("setting the regulations of %s: %w", slug, err)
		}
		if !exists {
			return ErrNoTournament
		}
		q := `INSERT INTO tournament_regulations (tournament, text) VALUES ($1, $2)
			ON CONFLICT (tournament) DO UPDATE SET text = excluded.text`
		args := []any{slug, text}
		if text == "" {
			q, args = "DELETE FROM tournament_regulations WHERE tournament = $1", []any{slug}
		}
		if _, err := tx.Exec(ctx, q, args...); err != nil {
			return fmt.Errorf("setting the regulations of %s: %w", slug, err)
		}
		return nil
	})
}

// ErrBadCard means a card's icon is over 8 characters, its description over 60 or its information box over 1,000.
var ErrBadCard = errors.New("a card's icon is at most 8 characters, its description, subtitle and cadence 60 and its information box 1,000")

// SetCard saves a tournament's card, trimmed; an empty card removes it. It returns ErrNoTournament or ErrBadCard.
func (a *AdminStore) SetCard(ctx context.Context, slug string, c Card) error {
	c = Card{Icon: strings.TrimSpace(c.Icon), Tagline: strings.TrimSpace(c.Tagline), Summary: strings.TrimSpace(c.Summary),
		Subtitle: strings.TrimSpace(c.Subtitle), Cadence: strings.TrimSpace(c.Cadence)}
	if utf8.RuneCountInString(c.Icon) > 8 || utf8.RuneCountInString(c.Tagline) > 60 || utf8.RuneCountInString(c.Summary) > 1000 ||
		utf8.RuneCountInString(c.Subtitle) > 60 || utf8.RuneCountInString(c.Cadence) > 60 {
		return ErrBadCard
	}
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM tournaments WHERE slug = $1)", slug).Scan(&exists); err != nil {
			return fmt.Errorf("setting the card of %s: %w", slug, err)
		}
		if !exists {
			return ErrNoTournament
		}
		q := `INSERT INTO tournament_cards (tournament, icon, tagline, summary, subtitle, cadence) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (tournament) DO UPDATE SET icon = excluded.icon, tagline = excluded.tagline, summary = excluded.summary,
				subtitle = excluded.subtitle, cadence = excluded.cadence`
		args := []any{slug, c.Icon, c.Tagline, c.Summary, c.Subtitle, c.Cadence}
		if c == (Card{}) {
			q, args = "DELETE FROM tournament_cards WHERE tournament = $1", []any{slug}
		}
		if _, err := tx.Exec(ctx, q, args...); err != nil {
			return fmt.Errorf("setting the card of %s: %w", slug, err)
		}
		return nil
	})
}

// ErrBadStory means more than 12 stories, or one without a player, title or story, or over its lengths.
var ErrBadStory = errors.New("at most 12 stories, each with a player, a title (120) and the story (1,000), a badge of up to 60")

// SetStories replaces a tournament's stories with these, in this order. It returns ErrNoTournament or ErrBadStory.
func (a *AdminStore) SetStories(ctx context.Context, slug string, list []Story) error {
	if len(list) > 12 {
		return ErrBadStory
	}
	clean := make([]Story, len(list))
	for i, s := range list {
		s = Story{Player: strings.TrimSpace(s.Player), Badge: strings.TrimSpace(s.Badge), Title: strings.TrimSpace(s.Title), Text: strings.TrimSpace(s.Text)}
		if s.Player == "" || s.Title == "" || s.Text == "" || utf8.RuneCountInString(s.Player) > 200 ||
			utf8.RuneCountInString(s.Badge) > 60 || utf8.RuneCountInString(s.Title) > 120 || utf8.RuneCountInString(s.Text) > 1000 {
			return ErrBadStory
		}
		clean[i] = s
	}
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM tournaments WHERE slug = $1)", slug).Scan(&exists); err != nil {
			return fmt.Errorf("setting the stories of %s: %w", slug, err)
		}
		if !exists {
			return ErrNoTournament
		}
		if _, err := tx.Exec(ctx, "DELETE FROM tournament_stories WHERE tournament = $1", slug); err != nil {
			return fmt.Errorf("setting the stories of %s: %w", slug, err)
		}
		for i, s := range clean {
			if _, err := tx.Exec(ctx, `INSERT INTO tournament_stories (tournament, position, player_name, badge, title, text)
				VALUES ($1, $2, $3, $4, $5, $6)`, slug, i+1, s.Player, s.Badge, s.Title, s.Text); err != nil {
				return fmt.Errorf("setting the stories of %s: %w", slug, err)
			}
		}
		return nil
	})
}

// SetSeasonInChampions sets whether a season's winner is listed among the tournament's champions, or returns
// ErrNoTournament for a season that does not exist.
func (a *AdminStore) SetSeasonInChampions(ctx context.Context, slug, season string, in bool) error {
	tag, err := a.pool.Exec(ctx, `UPDATE tournament_seasons se SET in_champions = $3 FROM tournaments t
		WHERE t.tournament_id = se.tournament_id AND t.slug = $1 AND se.label = $2`, slug, season, in)
	if err != nil {
		return fmt.Errorf("setting whether %s %s is among the champions: %w", slug, season, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoTournament
	}
	return nil
}

// SetSeasonRated sets whether a season's games count towards ratings, from the next run, or returns
// ErrNoTournament for a season that does not exist.
// ErrBadPenalty means a time-out penalty above 0, or for a season not numbered.
var ErrBadPenalty = errors.New("a time-out penalty is 0 or below, for a numbered season")

// SetSeasonPenalty sets the points a player loses in a season's standings for each game they time out of.
func (a *AdminStore) SetSeasonPenalty(ctx context.Context, slug, season string, penalty float64) error {
	number, err := strconv.Atoi(season)
	if penalty > 0 || err != nil || number < 1 {
		return ErrBadPenalty
	}
	tag, err := a.pool.Exec(ctx, `
		INSERT INTO tournament_season_rules (series, season, timeout_penalty, source)
		SELECT t.slug, $2::int, $3, 'admin' FROM tournaments t JOIN tournament_seasons se USING (tournament_id)
		WHERE t.slug = $1 AND se.label = $2::int::text
		ON CONFLICT (series, season) DO UPDATE SET timeout_penalty = excluded.timeout_penalty, source = excluded.source`,
		slug, number, penalty)
	if err != nil {
		return fmt.Errorf("setting %s %s's time-out penalty: %w", slug, season, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoTournament
	}
	return nil
}

func (a *AdminStore) SetSeasonRated(ctx context.Context, slug, season string, rated bool) error {
	tag, err := a.pool.Exec(ctx, `UPDATE tournament_seasons se SET rated = $3 FROM tournaments t
		WHERE t.tournament_id = se.tournament_id AND t.slug = $1 AND se.label = $2`, slug, season, rated)
	if err != nil {
		return fmt.Errorf("setting whether %s %s is rated: %w", slug, season, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoTournament
	}
	return nil
}
