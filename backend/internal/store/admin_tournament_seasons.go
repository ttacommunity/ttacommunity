// Changes to a tournament's seasons in admin: adding and removing one, and its podium, notes, time-out penalty and
// what it counts in, each alone or all in one transaction.

package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

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
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM series t JOIN tournaments se USING (series_id)
		WHERE t.slug = $1 AND se.label = $2)`, slug, season).Scan(&exists)
	if err != nil {
		return fmt.Errorf("finding %s %s: %w", slug, season, err)
	}
	if !exists {
		return ErrNoTournament
	}
	return nil
}

// SetNote saves the note shown beside a season (stage "") or one of its stages; an empty note removes it.
// It returns ErrNoTournament when the tournament has no such season or stage.
func (a *AdminStore) SetNote(ctx context.Context, slug, season, stage, note string) error {
	var exists bool
	if err := a.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM series t JOIN tournaments se USING (series_id)
		               LEFT JOIN stages st USING (tournament_id)
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
		INSERT INTO tournaments (series_id, label, original_name)
		SELECT series_id, $2, $3 FROM series WHERE slug = $1
		ON CONFLICT (series_id, label) DO NOTHING`, slug, label, typed)
	if err != nil {
		return fmt.Errorf("adding season %s of %s: %w", label, slug, err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var exists bool
	if err := a.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM series WHERE slug = $1)", slug).Scan(&exists); err != nil {
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
			SELECT se.tournament_id, EXISTS (SELECT 1 FROM stages st WHERE st.tournament_id = se.tournament_id)
			FROM series t JOIN tournaments se USING (series_id) WHERE t.slug = $1 AND se.label = $2`,
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
		if _, err := tx.Exec(ctx, "DELETE FROM tournaments WHERE tournament_id = $1", id); err != nil {
			return fmt.Errorf("removing season %s of %s: %w", label, slug, err)
		}
		return nil
	})
}

// SeasonChange is a change to one season: each part that is not nil, true or empty is set.
type SeasonChange struct {
	Podium           *[]PodiumEntry
	PodiumNote       string
	ClearPodium      bool
	Rated            *bool
	InChampions      *bool
	InCommunity      *bool
	RunHere          *bool
	TimeoutPenalty   *float64
	Note             *string
	StageNotes       map[string]string
	MoveToTournament *string
	NewSeason        *string
}

// ChangeSeason sets every part of c on a tournament's season in one transaction, so a refused part changes
// nothing. It returns what the refused part's Set method would.
func (a *AdminStore) ChangeSeason(ctx context.Context, slug, season string, c SeasonChange) error {
	return a.InTx(ctx, func(tx *AdminStore) error {
		steps := []struct {
			asked bool
			do    func() error
		}{
			{c.Podium != nil, func() error { return tx.SetPodium(ctx, slug, season, *c.Podium, c.PodiumNote) }},
			{c.ClearPodium, func() error { return tx.ClearPodium(ctx, slug, season) }},
			{c.Rated != nil, func() error { return tx.SetSeasonRated(ctx, slug, season, *c.Rated) }},
			{c.InChampions != nil, func() error { return tx.SetSeasonInChampions(ctx, slug, season, *c.InChampions) }},
			{c.InCommunity != nil, func() error { return tx.SetSeasonInCommunity(ctx, slug, season, *c.InCommunity) }},
			{c.RunHere != nil, func() error { return tx.SetSeasonRunHere(ctx, slug, season, *c.RunHere) }},
			{c.TimeoutPenalty != nil, func() error { return tx.SetSeasonPenalty(ctx, slug, season, *c.TimeoutPenalty) }},
			{c.Note != nil, func() error { return tx.SetNote(ctx, slug, season, "", *c.Note) }},
			{c.MoveToTournament != nil, func() error {
				newLabel := season
				if c.NewSeason != nil && strings.TrimSpace(*c.NewSeason) != "" {
					newLabel = strings.TrimSpace(*c.NewSeason)
				}
				return tx.MoveSeason(ctx, slug, season, *c.MoveToTournament, newLabel)
			}},
		}
		for _, st := range steps {
			if st.asked {
				if err := st.do(); err != nil {
					return err
				}
			}
		}
		for stage, note := range c.StageNotes {
			if err := tx.SetNote(ctx, slug, season, stage, note); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetSeasonInChampions sets whether a season's winner is listed among the tournament's champions, or returns
// ErrNoTournament for a season that does not exist.
func (a *AdminStore) SetSeasonInChampions(ctx context.Context, slug, season string, in bool) error {
	tag, err := a.pool.Exec(ctx, `UPDATE tournaments se SET in_champions = $3 FROM series t
		WHERE t.series_id = se.series_id AND t.slug = $1 AND se.label = $2`, slug, season, in)
	if err != nil {
		return fmt.Errorf("setting whether %s %s is among the champions: %w", slug, season, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoTournament
	}
	return nil
}

// SetSeasonInCommunity sets whether a season counts in the community leaderboard, from the next run, or
// returns ErrNoTournament for a season that does not exist.
func (a *AdminStore) SetSeasonInCommunity(ctx context.Context, slug, season string, in bool) error {
	tag, err := a.pool.Exec(ctx, `UPDATE tournaments se SET in_community = $3 FROM series t
		WHERE t.series_id = se.series_id AND t.slug = $1 AND se.label = $2`, slug, season, in)
	if err != nil {
		return fmt.Errorf("setting whether %s %s counts in the community leaderboard: %w", slug, season, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoTournament
	}
	return nil
}

// SetSeasonRunHere sets whether a season is run on this site, so CGE's loads take its CGE tournament's page and
// never its stages or games, or returns ErrNoTournament for a season that does not exist.
func (a *AdminStore) SetSeasonRunHere(ctx context.Context, slug, season string, here bool) error {
	tag, err := a.pool.Exec(ctx, `UPDATE tournaments se SET run_here = $3 FROM series t
		WHERE t.series_id = se.series_id AND t.slug = $1 AND se.label = $2`, slug, season, here)
	if err != nil {
		return fmt.Errorf("setting whether %s %s is run on this site: %w", slug, season, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoTournament
	}
	return nil
}

// ErrBadPenalty means a time-out penalty above 0, or for a season not numbered.
var ErrBadPenalty = errors.New("a time-out penalty is 0 or below, for a numbered season")

// SetSeasonPenalty sets the points a player loses in a season's standings for each game they time out of.
func (a *AdminStore) SetSeasonPenalty(ctx context.Context, slug, season string, penalty float64) error {
	number, err := strconv.Atoi(season)
	if penalty > 0 || err != nil || number < 1 {
		return ErrBadPenalty
	}
	tag, err := a.pool.Exec(ctx, `
		INSERT INTO settings (tournament_id, timeout_penalty, source)
		SELECT se.tournament_id, $3, 'admin' FROM series t JOIN tournaments se USING (series_id)
		WHERE t.slug = $1 AND se.label = $2::int::text
		ON CONFLICT (tournament_id) DO UPDATE SET timeout_penalty = excluded.timeout_penalty, source = excluded.source`,
		slug, number, penalty)
	if err != nil {
		return fmt.Errorf("setting %s %s's time-out penalty: %w", slug, season, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoTournament
	}
	return nil
}

// SetSeasonRated sets whether a season's games count towards ratings, from the next run, or returns
// ErrNoTournament for a season that does not exist.
func (a *AdminStore) SetSeasonRated(ctx context.Context, slug, season string, rated bool) error {
	tag, err := a.pool.Exec(ctx, `UPDATE tournaments se SET rated = $3 FROM series t
		WHERE t.series_id = se.series_id AND t.slug = $1 AND se.label = $2`, slug, season, rated)
	if err != nil {
		return fmt.Errorf("setting whether %s %s is rated: %w", slug, season, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoTournament
	}
	return nil
}

// MoveSeason moves a season from one tournament to another, optionally renaming its label.
// It returns ErrNoTournament when the source season or destination tournament does not exist,
// or ErrSeasonExists when the destination tournament already has that season label.
func (a *AdminStore) MoveSeason(ctx context.Context, fromTournament, season, toTournament, newLabel string) error {
	newLabel = strings.TrimSpace(newLabel)
	if newLabel == "" {
		newLabel = season
	}
	newLabel = seasonPrefix.ReplaceAllString(newLabel, "")
	if newLabel == "" || len(newLabel) > 64 {
		return errors.New("a season's label is 1 to 64 characters")
	}

	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var tournamentID int64
		err := tx.QueryRow(ctx, `SELECT se.tournament_id FROM series t JOIN tournaments se USING (series_id)
			WHERE t.slug = $1 AND se.label = $2`, fromTournament, season).Scan(&tournamentID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoTournament
		} else if err != nil {
			return fmt.Errorf("finding %s season %s: %w", fromTournament, season, err)
		}

		var destSeriesID int64
		err = tx.QueryRow(ctx, `SELECT series_id FROM series WHERE slug = $1`, toTournament).Scan(&destSeriesID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoTournament
		} else if err != nil {
			return fmt.Errorf("finding destination tournament %s: %w", toTournament, err)
		}

		var exists bool
		err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tournaments WHERE series_id = $1 AND label = $2)`,
			destSeriesID, newLabel).Scan(&exists)
		if err != nil {
			return fmt.Errorf("checking destination season %s %s: %w", toTournament, newLabel, err)
		}
		if exists {
			return ErrSeasonExists
		}

		if _, err := tx.Exec(ctx, `UPDATE tournaments SET series_id = $1, label = $2 WHERE tournament_id = $3`,
			destSeriesID, newLabel, tournamentID); err != nil {
			return fmt.Errorf("moving tournament season: %w", err)
		}

		if _, err := tx.Exec(ctx, `UPDATE cge_tournaments SET series_id = $1 WHERE tournament_id = $2`,
			destSeriesID, tournamentID); err != nil {
			return fmt.Errorf("moving cge_tournaments: %w", err)
		}

		if _, err := tx.Exec(ctx, `UPDATE tournament_season_notes SET tournament = $1, season = $2
			WHERE tournament = $3 AND season = $4`, toTournament, newLabel, fromTournament, season); err != nil {
			return fmt.Errorf("moving tournament_season_notes: %w", err)
		}

		if _, err := tx.Exec(ctx, `UPDATE tournament_podium_overrides SET tournament = $1, season = $2
			WHERE tournament = $3 AND season = $4`, toTournament, newLabel, fromTournament, season); err != nil {
			return fmt.Errorf("moving tournament_podium_overrides: %w", err)
		}

		return nil
	})
}
