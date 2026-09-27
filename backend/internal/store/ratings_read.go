package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors a public read can end in. Their text is safe to show a visitor.
var (
	ErrNoPublishedRun = errors.New("no rating run has been published yet")
	ErrRunNotFound    = errors.New("no published rating run has this id")
	ErrCanWrite       = errors.New("the ratings reader's database role can write")
)

// PublishedRun is a run visitors can read.
type PublishedRun struct {
	ID          int64
	PublishedAt time.Time
}

// LeaderboardRow is one player's rating in one model and format.
type LeaderboardRow struct {
	Rank       *int
	RankDelta  int
	Name       string
	Rating     float64
	RD         float64
	CRating    float64
	Games      int
	Opponents  int
	Wins       int
	Losses     int
	Draws      int
	WinRate    float64
	LastPlayed *time.Time
}

// RatingsReader reads published ratings through a read-only pool.
type RatingsReader struct {
	pool *pgxpool.Pool
}

// NewRatingsReader returns a reader over pool, which should act as ratings_read.
func NewRatingsReader(pool *pgxpool.Pool) *RatingsReader {
	return &RatingsReader{pool: pool}
}

// CheckReadOnly returns ErrCanWrite if the pool's role is a superuser, can
// create tables, or can change any table. It works before the schema exists.
// The API calls it at start, so a misconfigured role refuses to serve.
func (r *RatingsReader) CheckReadOnly(ctx context.Context) error {
	var canWrite bool
	err := r.pool.QueryRow(ctx, `
		SELECT (SELECT rolsuper FROM pg_roles WHERE rolname = current_user)
			OR has_schema_privilege('public', 'CREATE')
			OR EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
				WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p')
				AND has_table_privilege(c.oid, 'INSERT, UPDATE, DELETE, TRUNCATE'))`).Scan(&canWrite)
	if err != nil {
		return fmt.Errorf("checking the ratings reader's privileges: %w", err)
	}
	if canWrite {
		return ErrCanWrite
	}
	return nil
}

// CurrentRun returns the run the site shows now, or ErrNoPublishedRun.
func (r *RatingsReader) CurrentRun(ctx context.Context) (PublishedRun, error) {
	var run PublishedRun
	err := r.pool.QueryRow(ctx, `
		SELECT c.run_id, r.published_at
		FROM current_run c JOIN rating_runs r USING (run_id)`).Scan(&run.ID, &run.PublishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublishedRun{}, ErrNoPublishedRun
	}
	if err != nil {
		return PublishedRun{}, fmt.Errorf("reading the current run: %w", err)
	}
	return run, nil
}

// Leaderboard returns a published run's ratings for one model and format in
// rank order, unranked players last. An unknown run is ErrRunNotFound.
func (r *RatingsReader) Leaderboard(ctx context.Context, run int64, model string, format int) ([]LeaderboardRow, error) {
	var published bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM rating_runs
			WHERE run_id = $1 AND status = 'complete' AND published_at IS NOT NULL)`, run).Scan(&published)
	if err != nil {
		return nil, fmt.Errorf("looking up run %d: %w", run, err)
	}
	if !published {
		return nil, ErrRunNotFound
	}

	rows, err := r.pool.Query(ctx, `
		SELECT rank, rank_delta, player_name, rating, rd, c_rating, games_played, opponents_count,
			wins, losses, draws, win_rate, last_played
		FROM player_ratings
		WHERE run_id = $1 AND model_type = $2 AND player_count = $3
		ORDER BY rank NULLS LAST, player_name`, run, model, format)
	if err != nil {
		return nil, fmt.Errorf("reading run %d's %s leaderboard: %w", run, model, err)
	}
	board, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (LeaderboardRow, error) {
		var p LeaderboardRow
		err := row.Scan(&p.Rank, &p.RankDelta, &p.Name, &p.Rating, &p.RD, &p.CRating, &p.Games, &p.Opponents,
			&p.Wins, &p.Losses, &p.Draws, &p.WinRate, &p.LastPlayed)
		return p, err
	})
	if err != nil {
		return nil, fmt.Errorf("reading run %d's %s leaderboard: %w", run, model, err)
	}
	return board, nil
}
