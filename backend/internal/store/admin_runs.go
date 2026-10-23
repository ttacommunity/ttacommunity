// Rating runs in admin: list them with their progress, publish or roll back, and compare one with another.

package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrRunNotComplete means the run is still running or failed, so it cannot be published.
var ErrRunNotComplete = errors.New("only a complete run can be published")

// AdminRun is one rating run as admin sees it.
type AdminRun struct {
	ID                      int64
	Status                  string
	StartedAt               time.Time
	FinishedAt, PublishedAt *time.Time
	Current                 bool
	Games, Players          int
	EngineVersion           string
	Progress                string
	ProgressAt              *time.Time
	Protected, Deleting     bool
	Bytes, DiskBytes        int64 // of the rows it wrote, and with its share of their indexes; recorded as it ended, 0 while going
}

// Runs returns the community ranking's runs, newest first, each with its latest progress line.
func (a *AdminStore) Runs(ctx context.Context) ([]AdminRun, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT r.run_id, r.status, r.started_at, r.finished_at, r.published_at,
		       EXISTS (SELECT 1 FROM current_run c WHERE c.run_id = r.run_id),
		       coalesce(r.games, 0), coalesce(r.players, 0), r.engine_version,
		       coalesce(p.line, ''), p.updated_at, r.protected, r.deleting, r.result_bytes, r.disk_bytes
		FROM rating_runs r LEFT JOIN run_progress p USING (run_id)
		WHERE r.ranking_id = community_ranking()
		ORDER BY r.run_id DESC`)
	if err != nil {
		return nil, fmt.Errorf("reading runs for admin: %w", err)
	}
	runs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (AdminRun, error) {
		var r AdminRun
		err := row.Scan(&r.ID, &r.Status, &r.StartedAt, &r.FinishedAt, &r.PublishedAt, &r.Current,
			&r.Games, &r.Players, &r.EngineVersion, &r.Progress, &r.ProgressAt, &r.Protected, &r.Deleting, &r.Bytes, &r.DiskBytes)
		return r, err
	})
	if err != nil {
		return nil, fmt.Errorf("reading runs for admin: %w", err)
	}
	return runs, nil
}

// Publish makes a complete run the community ranking's current one, which also rolls back to an older run.
// It returns ErrRunNotFound or ErrRunNotComplete.
func (a *AdminStore) Publish(ctx context.Context, run int64) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, "SELECT status FROM rating_runs WHERE run_id = $1 AND ranking_id = community_ranking()", run).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRunNotFound
		}
		if err != nil {
			return fmt.Errorf("publishing run %d: %w", run, err)
		}
		if status != "complete" {
			return ErrRunNotComplete
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO current_run (ranking_id, run_id) VALUES (community_ranking(), $1)
			ON CONFLICT (ranking_id) DO UPDATE SET run_id = excluded.run_id`, run); err != nil {
			return fmt.Errorf("publishing run %d: %w", run, err)
		}
		if _, err := tx.Exec(ctx, "UPDATE rating_runs SET published_at = now() WHERE run_id = $1 AND published_at IS NULL", run); err != nil {
			return fmt.Errorf("publishing run %d: %w", run, err)
		}
		return nil
	})
}

// RunMove is one player's rating change between two runs.
type RunMove struct {
	Name          string
	Before, After float64
	Change        float64
}

// RunChanges compares one run's ratings with another's, in the main model and all formats.
type RunChanges struct {
	Ratings, RatingsBefore             int
	New, Gone                          int
	Moved1, Moved10, Moved50, Moved100 int
	Biggest                            []RunMove
}

// changesModel is the model whose conservative rating the comparison uses: the leaderboard's default.
const changesModel = "glicko2_daneo"

// RunChanges compares run's ratings with before's: how many moved by more than 1, 10, 50 and 100 points, who
// is new or gone, and the 20 biggest moves, of players who moved at all. It returns ErrRunNotFound when either
// run does not exist.
func (a *AdminStore) RunChanges(ctx context.Context, run, before int64) (RunChanges, error) {
	var found int
	if err := a.pool.QueryRow(ctx, "SELECT count(*) FROM rating_runs WHERE run_id IN ($1, $2)", run, before).Scan(&found); err != nil {
		return RunChanges{}, fmt.Errorf("comparing runs %d and %d: %w", run, before, err)
	}
	if found != 2 {
		return RunChanges{}, ErrRunNotFound
	}
	var c RunChanges
	err := a.pool.QueryRow(ctx, `
		WITH a AS (SELECT player_count, player_name, c_rating FROM player_ratings WHERE run_id = $1 AND model_type = $3),
		     b AS (SELECT player_count, player_name, c_rating FROM player_ratings WHERE run_id = $2 AND model_type = $3),
		     d AS (SELECT abs(a.c_rating - b.c_rating) AS m FROM a JOIN b USING (player_count, player_name))
		SELECT (SELECT count(*) FROM a), (SELECT count(*) FROM b),
		       (SELECT count(*) FROM a WHERE NOT EXISTS (SELECT 1 FROM b WHERE b.player_count = a.player_count AND b.player_name = a.player_name)),
		       (SELECT count(*) FROM b WHERE NOT EXISTS (SELECT 1 FROM a WHERE a.player_count = b.player_count AND a.player_name = b.player_name)),
		       count(*) FILTER (WHERE m > 1), count(*) FILTER (WHERE m > 10),
		       count(*) FILTER (WHERE m > 50), count(*) FILTER (WHERE m > 100)
		FROM d`, run, before, changesModel).Scan(&c.Ratings, &c.RatingsBefore, &c.New, &c.Gone, &c.Moved1, &c.Moved10, &c.Moved50, &c.Moved100)
	if err != nil {
		return RunChanges{}, fmt.Errorf("comparing runs %d and %d: %w", run, before, err)
	}
	rows, err := a.pool.Query(ctx, `
		SELECT a.player_name, b.c_rating, a.c_rating, a.c_rating - b.c_rating
		FROM player_ratings a JOIN player_ratings b USING (model_type, player_count, player_name)
		WHERE a.run_id = $1 AND b.run_id = $2 AND a.model_type = $3 AND a.player_count = 0 AND a.c_rating <> b.c_rating
		ORDER BY abs(a.c_rating - b.c_rating) DESC, a.player_name LIMIT 20`, run, before, changesModel)
	if err != nil {
		return RunChanges{}, fmt.Errorf("comparing runs %d and %d: %w", run, before, err)
	}
	c.Biggest, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (RunMove, error) {
		var m RunMove
		err := row.Scan(&m.Name, &m.Before, &m.After, &m.Change)
		return m, err
	})
	if err != nil {
		return RunChanges{}, fmt.Errorf("comparing runs %d and %d: %w", run, before, err)
	}
	return c, nil
}
