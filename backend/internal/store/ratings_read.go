package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/runcache"
)

// Errors a public read can end in. Their text is safe to show a visitor.
var (
	ErrNoPublishedRun = errors.New("no rating run has been published yet")
	ErrRunNotFound    = errors.New("no published rating run has this id")
	ErrCanWrite       = errors.New("the ratings reader's database role can write")
	ErrNoHistory      = errors.New("this run has no rating history for this model and format")
)

// PublishedRun is a run visitors can read.
type PublishedRun struct {
	ID            int64
	PublishedAt   time.Time
	EngineVersion string
	FinishedAt    *time.Time
	// What the engine recorded about the run's input; nil for a run completed before it did.
	Games, Players      *int
	FirstGame, LastGame *time.Time
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
	Country    *string
	Title      *Title
	Peak       *Peak
	Badges     []ExtraBadge
}

// ExtraBadge is a badge admin gave a player, shown beside their title.
type ExtraBadge struct {
	Label       string
	Description string
}

// badgesByPlayer returns every player's badges, by label: they show at once, whatever the run.
func (r *RatingsReader) badgesByPlayer(ctx context.Context, names []string) (map[string][]ExtraBadge, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT pb.player_name, b.label, b.description FROM player_badges pb JOIN badges b USING (badge_id)
		WHERE pb.player_name = ANY($1) ORDER BY lower(b.label)`, names)
	if err != nil {
		return nil, fmt.Errorf("reading badges: %w", err)
	}
	got := map[string][]ExtraBadge{}
	var name string
	var b ExtraBadge
	_, err = pgx.ForEachRow(rows, []any{&name, &b.Label, &b.Description}, func() error {
		got[name] = append(got[name], b)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading badges: %w", err)
	}
	return got, nil
}

// Peak is a player's best title over their career, and the year they held it.
type Peak struct {
	Code string
	Year *int
}

// Title is the tournament title the engine derived for a player in a run.
type Title struct {
	Code   string
	Count  int
	Reason string
}

// HistoryRow is one player's rating at the end of a past rating period.
type HistoryRow struct {
	Name    string
	Rating  float64
	RD      float64
	CRating float64
}

// RatingsReader reads published ratings through a read-only pool.
type RatingsReader struct {
	pool  *pgxpool.Pool
	seats *runcache.Cache[struct{}, map[string]seatCounts]
}

// NewRatingsReader returns a reader over pool, which should act as ratings_read.
func NewRatingsReader(pool *pgxpool.Pool) *RatingsReader {
	return &RatingsReader{pool: pool, seats: runcache.New[struct{}, map[string]seatCounts]()}
}

// Ping returns an error if the database does not answer.
func (r *RatingsReader) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
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

// CurrentRun returns the run the site shows now, the community ranking's,
// with what the engine recorded about it, or ErrNoPublishedRun.
func (r *RatingsReader) CurrentRun(ctx context.Context) (PublishedRun, error) {
	var run PublishedRun
	err := r.pool.QueryRow(ctx, `
		SELECT c.run_id, r.published_at, r.engine_version, r.finished_at, r.games, r.players, r.first_game, r.last_game
		FROM current_run c JOIN rating_runs r USING (run_id)
		WHERE c.ranking_id = community_ranking()`).Scan(&run.ID, &run.PublishedAt, &run.EngineVersion, &run.FinishedAt,
		&run.Games, &run.Players, &run.FirstGame, &run.LastGame)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublishedRun{}, ErrNoPublishedRun
	}
	if err != nil {
		return PublishedRun{}, fmt.Errorf("reading the current run: %w", err)
	}
	return run, nil
}

// RunLastGame returns the date of a published run's last game, or nil for a run that
// recorded none. An unknown or unpublished run is ErrRunNotFound.
func (r *RatingsReader) RunLastGame(ctx context.Context, run int64) (*time.Time, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return nil, err
	}
	var last *time.Time
	err := r.pool.QueryRow(ctx, "SELECT last_game FROM rating_runs WHERE run_id = $1", run).Scan(&last)
	if err != nil {
		return nil, fmt.Errorf("reading run %d's last game: %w", run, err)
	}
	return last, nil
}

// Leaderboard returns a published run's ratings for one model and format in
// rank order, unranked players last, each with the player's country now and
// the run's title and career peak. An unknown run is ErrRunNotFound.
func (r *RatingsReader) Leaderboard(ctx context.Context, run int64, model string, format int) ([]LeaderboardRow, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT r.rank, r.rank_delta, r.player_name, r.rating, r.rd, r.c_rating, r.games_played, r.opponents_count,
			r.wins, r.losses, r.draws, r.win_rate, r.last_played, m.country_code, t.title, t.title_count, t.badge_reason,
			k.peak_title, k.peak_year
		FROM player_ratings r
		LEFT JOIN players p ON p.name = r.player_name
		LEFT JOIN members m ON m.player_id = p.player_id
		LEFT JOIN player_titles t ON t.run_id = r.run_id AND t.player_name = r.player_name
		LEFT JOIN player_peak_titles k ON k.run_id = r.run_id AND k.player_name = r.player_name
		WHERE r.run_id = $1 AND r.model_type = $2 AND r.player_count = $3
		  AND `+notHiddenPlayer+`
		ORDER BY r.rank NULLS LAST, r.player_name`, run, model, format)
	if err != nil {
		return nil, fmt.Errorf("reading run %d's %s leaderboard: %w", run, model, err)
	}
	board, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (LeaderboardRow, error) {
		var p LeaderboardRow
		var title, reason, peak *string
		var count, peakYear *int
		err := row.Scan(&p.Rank, &p.RankDelta, &p.Name, &p.Rating, &p.RD, &p.CRating, &p.Games, &p.Opponents,
			&p.Wins, &p.Losses, &p.Draws, &p.WinRate, &p.LastPlayed, &p.Country, &title, &count, &reason, &peak, &peakYear)
		if title != nil {
			p.Title = &Title{Code: *title, Count: *count, Reason: *reason}
		}
		if peak != nil {
			p.Peak = &Peak{Code: *peak, Year: peakYear}
		}
		return p, err
	})
	if err != nil {
		return nil, fmt.Errorf("reading run %d's %s leaderboard: %w", run, model, err)
	}
	names := make([]string, len(board))
	for i, p := range board {
		names[i] = p.Name
	}
	badges, err := r.badgesByPlayer(ctx, names)
	if err != nil {
		return nil, err
	}
	for i := range board {
		board[i].Badges = badges[board[i].Name]
	}
	return board, nil
}

// History returns the snapshot of the latest period on or before at (the first
// period when at is earlier), for a published run, model and format, and its date.
// Without at, it uses the run's previous period, the one before its last. Only the
// players in that snapshot are in it, less those with a deleted or banned CGE account.
func (r *RatingsReader) History(ctx context.Context, run int64, model string, format int, at *time.Time) (time.Time, []HistoryRow, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return time.Time{}, nil, err
	}
	var on time.Time
	if at != nil {
		// The snapshot on or before the date, else the first one, as upstream picks it (7cd7220).
		err := r.pool.QueryRow(ctx, `
			SELECT coalesce(max(period_date) FILTER (WHERE period_date <= $4), min(period_date))
			FROM rating_history WHERE run_id = $1 AND model_type = $2 AND player_count = $3`, run, model, format, *at).Scan(&on)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && on.IsZero()) {
			return time.Time{}, nil, ErrNoHistory
		}
		if err != nil {
			return time.Time{}, nil, fmt.Errorf("finding run %d's snapshot for %s: %w", run, at.Format(time.DateOnly), err)
		}
	} else {
		err := r.pool.QueryRow(ctx, `
			SELECT period_date FROM (
				SELECT DISTINCT period_date FROM rating_history
				WHERE run_id = $1 AND model_type = $2 AND player_count = $3
				ORDER BY period_date DESC LIMIT 2) last
			ORDER BY period_date LIMIT 1`, run, model, format).Scan(&on)
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, nil, ErrNoHistory
		}
		if err != nil {
			return time.Time{}, nil, fmt.Errorf("finding run %d's previous period: %w", run, err)
		}
	}

	rows, err := r.pool.Query(ctx, `
		SELECT player_name, rating, rd, c_rating
		FROM rating_history
		WHERE run_id = $1 AND model_type = $2 AND player_count = $3 AND period_date = $4
		  AND NOT `+hiddenName("player_name")+`
		ORDER BY player_name`, run, model, format, on)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("reading run %d's %s history: %w", run, model, err)
	}
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (HistoryRow, error) {
		var h HistoryRow
		err := row.Scan(&h.Name, &h.Rating, &h.RD, &h.CRating)
		return h, err
	})
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("reading run %d's %s history: %w", run, model, err)
	}
	return on, got, nil
}

// requirePublished returns ErrRunNotFound unless run is complete and published.
// CheckPublished returns ErrRunNotFound unless run is complete and published, for answers kept outside the store.
func (r *RatingsReader) CheckPublished(ctx context.Context, run int64) error {
	return r.requirePublished(ctx, run)
}

func (r *RatingsReader) requirePublished(ctx context.Context, run int64) error {
	var published bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM rating_runs
			WHERE run_id = $1 AND status = 'complete' AND published_at IS NOT NULL)`, run).Scan(&published)
	if err != nil {
		return fmt.Errorf("looking up run %d: %w", run, err)
	}
	if !published {
		return ErrRunNotFound
	}
	return nil
}
