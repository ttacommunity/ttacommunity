// Tournament settings in admin: shown, rated, counted in the community leaderboard, and season and stage notes.

package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrNoTournament means no tournament, or no season of it, has that name.
var ErrNoTournament = errors.New("no such tournament or season")

// AdminTournament is one tournament as admin edits it.
type AdminTournament struct {
	Slug, Name string
	Visible    bool
	Rated      bool
	Community  bool
	Seasons    []AdminSeason
}

// AdminSeason is a season with its own note and its stages.
type AdminSeason struct {
	Label  string
	Note   string
	Stages []AdminStage
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

// Tournaments returns every tournament with its settings, seasons, stages and notes, in name order.
func (a *AdminStore) Tournaments(ctx context.Context) ([]AdminTournament, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT t.slug, t.name, t.visible, t.in_community_leaderboard,
		       EXISTS (SELECT 1 FROM ranking_tournaments rt JOIN rankings r USING (ranking_id)
		               WHERE r.slug = 'community' AND rt.tournament_id = t.tournament_id),
		       se.label, coalesce(st.label, ''),
		       coalesce((SELECT note FROM tournament_season_notes n WHERE n.tournament = t.slug AND n.season = se.label AND n.stage = ''), ''),
		       coalesce((SELECT note FROM tournament_season_notes n WHERE n.tournament = t.slug AND n.season = se.label AND n.stage = st.label AND st.label <> ''), '')
		FROM tournaments t
		LEFT JOIN tournament_seasons se USING (tournament_id)
		LEFT JOIN tournament_stages st USING (season_id)
		ORDER BY t.name, se.season_id, st.stage_id`)
	if err != nil {
		return nil, fmt.Errorf("reading tournaments for admin: %w", err)
	}
	var out []AdminTournament
	var slug, name string
	var visible, community, rated bool
	var season, stage, seasonNote, stageNote *string
	_, err = pgx.ForEachRow(rows, []any{&slug, &name, &visible, &community, &rated, &season, &stage, &seasonNote, &stageNote}, func() error {
		if len(out) == 0 || out[len(out)-1].Slug != slug {
			out = append(out, AdminTournament{Slug: slug, Name: name, Visible: visible, Rated: rated, Community: community, Seasons: []AdminSeason{}})
		}
		t := &out[len(out)-1]
		if season == nil {
			return nil
		}
		if len(t.Seasons) == 0 || t.Seasons[len(t.Seasons)-1].Label != *season {
			t.Seasons = append(t.Seasons, AdminSeason{Label: *season, Note: *seasonNote, Stages: []AdminStage{}})
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
	return out, nil
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
