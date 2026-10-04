// Tournament settings in admin: shown, rated, counted in the community leaderboard, and season and stage notes.

package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// ErrNoTournament means no tournament, or no season of it, has that name.
var ErrNoTournament = errors.New("no such tournament or season")

// AdminTournament is one tournament as admin edits it: Regulations is admin's format and regulations text,
// shown instead of CGERegulations, the latest edition's CGE description; either is empty for none.
type AdminTournament struct {
	Slug, Name     string
	Visible        bool
	Rated          bool
	Community      bool
	Regulations    string
	CGERegulations string
	Card           Card
	Seasons        []AdminSeason
}

// AdminSeason is a season with its own note, its stages, and the podium admin set for it, if any.
type AdminSeason struct {
	Label      string
	Note       string
	HasGames   bool
	Stages     []AdminStage
	Podium     []PodiumEntry
	PodiumNote string
}

// PodiumEntry is one place on a podium set in admin.
type PodiumEntry struct {
	Place int
	Name  string
}

// AdminStage is a stage of a season (an empty label is a season without stages) with its note.
type AdminStage struct {
	Label string
	Note  string
}

// TournamentSettings are the settings to change; a nil field is left as it is.
type TournamentSettings struct {
	Visible, Rated, Community *bool
}

// Tournaments returns every tournament with its settings, seasons (latest first), stages and notes, in name order.
func (a *AdminStore) Tournaments(ctx context.Context) ([]AdminTournament, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT t.slug, t.name, t.visible, t.in_community_leaderboard,
		       EXISTS (SELECT 1 FROM ranking_tournaments rt JOIN rankings r USING (ranking_id)
		               WHERE r.slug = 'community' AND rt.tournament_id = t.tournament_id),
		       coalesce((SELECT text FROM tournament_regulations g WHERE g.tournament = t.slug), ''),
		       coalesce(k.icon, ''), coalesce(k.tagline, ''), coalesce(k.summary, ''),
		       coalesce((SELECT c.description FROM cge_tournaments c WHERE c.tournament_id = t.tournament_id AND c.description <> ''
		                 ORDER BY coalesce(c.started, c.created, c.finished) DESC NULLS LAST, c.cge_tournament DESC LIMIT 1), ''),
		       se.label, coalesce(st.label, ''),
		       coalesce((SELECT note FROM tournament_season_notes n WHERE n.tournament = t.slug AND n.season = se.label AND n.stage = ''), ''),
		       coalesce((SELECT note FROM tournament_season_notes n WHERE n.tournament = t.slug AND n.season = se.label AND n.stage = st.label AND st.label <> ''), ''),
		       d.last IS NOT NULL
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
	var hasGames bool
	_, err = pgx.ForEachRow(rows, []any{&slug, &name, &visible, &community, &rated, &regulations,
		&card.Icon, &card.Tagline, &card.Summary, &cgeRegulations, &season, &stage, &seasonNote, &stageNote, &hasGames}, func() error {
		if len(out) == 0 || out[len(out)-1].Slug != slug {
			out = append(out, AdminTournament{Slug: slug, Name: name, Visible: visible, Rated: rated, Community: community,
				Regulations: regulations, CGERegulations: cgeRegulations, Card: card, Seasons: []AdminSeason{}})
		}
		t := &out[len(out)-1]
		if season == nil {
			return nil
		}
		if len(t.Seasons) == 0 || t.Seasons[len(t.Seasons)-1].Label != *season {
			t.Seasons = append(t.Seasons, AdminSeason{Label: *season, Note: *seasonNote, HasGames: hasGames, Stages: []AdminStage{}})
		}
		s := &t.Seasons[len(t.Seasons)-1]
		if *stage != "" {
			s.Stages = append(s.Stages, AdminStage{Label: *stage, Note: *stageNote})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading tournaments for admin: %w", err)
	}
	if err := a.addPodiums(ctx, out); err != nil {
		return nil, err
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

// SetTournament changes a tournament's settings in one transaction, or returns ErrNoTournament.
func (a *AdminStore) SetTournament(ctx context.Context, slug string, s TournamentSettings) error {
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
	label = seasonPrefix.ReplaceAllString(strings.TrimSpace(label), "")
	tag, err := a.pool.Exec(ctx, `
		INSERT INTO tournament_seasons (tournament_id, label)
		SELECT tournament_id, $2 FROM tournaments WHERE slug = $1
		ON CONFLICT (tournament_id, label) DO NOTHING`, slug, label)
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
var ErrBadCard = errors.New("a card's icon is at most 8 characters, its description 60 and its information box 1,000")

// SetCard saves a tournament's card, trimmed; an empty card removes it. It returns ErrNoTournament or ErrBadCard.
func (a *AdminStore) SetCard(ctx context.Context, slug string, c Card) error {
	c = Card{Icon: strings.TrimSpace(c.Icon), Tagline: strings.TrimSpace(c.Tagline), Summary: strings.TrimSpace(c.Summary)}
	if utf8.RuneCountInString(c.Icon) > 8 || utf8.RuneCountInString(c.Tagline) > 60 || utf8.RuneCountInString(c.Summary) > 1000 {
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
		q := `INSERT INTO tournament_cards (tournament, icon, tagline, summary) VALUES ($1, $2, $3, $4)
			ON CONFLICT (tournament) DO UPDATE SET icon = excluded.icon, tagline = excluded.tagline, summary = excluded.summary`
		args := []any{slug, c.Icon, c.Tagline, c.Summary}
		if c == (Card{}) {
			q, args = "DELETE FROM tournament_cards WHERE tournament = $1", []any{slug}
		}
		if _, err := tx.Exec(ctx, q, args...); err != nil {
			return fmt.Errorf("setting the card of %s: %w", slug, err)
		}
		return nil
	})
}
