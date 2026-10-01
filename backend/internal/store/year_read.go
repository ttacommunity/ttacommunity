package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Record is a player's opponents, wins, losses and draws.
type Record struct {
	Opponents, Wins, Losses, Draws int
}

// YearRow is one player on a year's board.
type YearRow struct {
	Name                string
	Rating, RD, CRating float64
	Country             *string
	Title               *Title
	Peak                *Peak
	Season, Career      Record
	LastPlayed          *time.Time
	PreviousRating      *float64
	PreviousRD          *float64
	PreviousCRating     *float64
}

// Champion is a year's World Champion; Reigning marks the current one.
type Champion struct {
	Name     string
	Reigning bool
}

// YearBoard is a run's ratings at the end of a year, as upstream's archive shows them.
type YearBoard struct {
	Year      int
	Snapshot  time.Time
	Previous  *time.Time
	Games     int
	Champions []Champion
	Rows      []YearRow
}

// YearBoard returns a published run's board for a year in a model and format:
// everyone who played that year, at their last rating of the year, best
// conservative rating first, with each player's record that year and up to
// it, and their last rating up to the previous year's last period. Snapshot is
// the model's last period that year (another model's period if this one has
// none that year). A year with no history is ErrNoHistory.
func (r *RatingsReader) YearBoard(ctx context.Context, run int64, year int, model string, format int) (YearBoard, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return YearBoard{}, err
	}
	b := YearBoard{Year: year, Champions: []Champion{}}

	var snapshot *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT coalesce(
			(SELECT max(period_date) FROM rating_history
			 WHERE run_id = $1 AND model_type = $2 AND player_count = $3 AND extract(year FROM period_date) = $4),
			(SELECT max(period_date) FROM rating_history WHERE run_id = $1 AND extract(year FROM period_date) = $4)),
			(SELECT max(period_date) FROM rating_history
			 WHERE run_id = $1 AND model_type = $2 AND player_count = $3 AND extract(year FROM period_date) = $4 - 1),
			(SELECT count(DISTINCT match_id) FROM pairwise_matches
			 WHERE run_id = $1 AND extract(year FROM date) = $4)`,
		run, model, format, year).Scan(&snapshot, &b.Previous, &b.Games)
	if err != nil {
		return YearBoard{}, fmt.Errorf("finding run %d's %d periods: %w", run, year, err)
	}
	if snapshot == nil {
		return YearBoard{}, ErrNoHistory
	}
	b.Snapshot = *snapshot

	rows, err := r.pool.Query(ctx, `
		SELECT y.player_name, false FROM player_yearly_titles y
		WHERE y.run_id = $1 AND y.year = $2 AND y.title = 'WC'
		UNION ALL
		SELECT t.player_name, true FROM player_titles t
		WHERE t.run_id = $1 AND t.title = 'WC'
			AND $2 = (SELECT extract(year FROM max(date))::int FROM pairwise_matches WHERE run_id = $1)
		ORDER BY 2, 1`, run, year)
	if err != nil {
		return YearBoard{}, fmt.Errorf("reading run %d's %d champions: %w", run, year, err)
	}
	b.Champions, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Champion, error) {
		var c Champion
		err := row.Scan(&c.Name, &c.Reigning)
		return c, err
	})
	if err != nil {
		return YearBoard{}, fmt.Errorf("reading run %d's %d champions: %w", run, year, err)
	}

	rows, err = r.pool.Query(ctx, `
		WITH mine AS (
			SELECT player_a AS name, outcome_a AS outcome, date FROM pairwise_matches
			WHERE run_id = $1 AND ($3 = 0 OR player_count = $3) AND extract(year FROM date) <= $5
			UNION ALL
			SELECT player_b, 1 - outcome_a, date FROM pairwise_matches
			WHERE run_id = $1 AND ($3 = 0 OR player_count = $3) AND extract(year FROM date) <= $5),
		rec AS (
			SELECT name,
				count(*) FILTER (WHERE extract(year FROM date) = $5) AS s_opps,
				count(*) FILTER (WHERE extract(year FROM date) = $5 AND outcome = 1) AS s_wins,
				count(*) FILTER (WHERE extract(year FROM date) = $5 AND outcome = 0) AS s_losses,
				count(*) FILTER (WHERE extract(year FROM date) = $5 AND outcome = 0.5) AS s_draws,
				max(date) FILTER (WHERE extract(year FROM date) = $5) AS last_played,
				count(*) AS c_opps, count(*) FILTER (WHERE outcome = 1) AS c_wins,
				count(*) FILTER (WHERE outcome = 0) AS c_losses, count(*) FILTER (WHERE outcome = 0.5) AS c_draws
			FROM mine GROUP BY name)
		SELECT h.player_name, h.rating, h.rd, h.c_rating, p.country_code, t.title, t.title_count, t.badge_reason,
			k.peak_title, k.peak_year,
			coalesce(rec.s_opps, 0), coalesce(rec.s_wins, 0), coalesce(rec.s_losses, 0), coalesce(rec.s_draws, 0),
			rec.last_played,
			coalesce(rec.c_opps, 0), coalesce(rec.c_wins, 0), coalesce(rec.c_losses, 0), coalesce(rec.c_draws, 0),
			prev.rating, prev.rd, prev.c_rating
		FROM (SELECT DISTINCT ON (player_name) * FROM rating_history
			WHERE run_id = $1 AND model_type = $2 AND player_count = $3 AND period_date <= $4 AND extract(year FROM period_date) = $5
			ORDER BY player_name, period_date DESC) h
		LEFT JOIN players p ON p.name = h.player_name
		LEFT JOIN player_titles t ON t.run_id = $1 AND t.player_name = h.player_name
		LEFT JOIN player_peak_titles k ON k.run_id = $1 AND k.player_name = h.player_name
		LEFT JOIN rec ON rec.name = h.player_name
		LEFT JOIN LATERAL (SELECT rating, rd, c_rating FROM rating_history
			WHERE run_id = $1 AND model_type = $2 AND player_count = $3 AND player_name = h.player_name AND period_date <= $6
			ORDER BY period_date DESC LIMIT 1) prev ON true
		ORDER BY h.c_rating DESC, h.player_name`, run, model, format, b.Snapshot, year, b.Previous)
	if err != nil {
		return YearBoard{}, fmt.Errorf("reading run %d's %d board: %w", run, year, err)
	}
	b.Rows, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (YearRow, error) {
		var y YearRow
		var title, reason, peak *string
		var count, peakYear *int
		err := row.Scan(&y.Name, &y.Rating, &y.RD, &y.CRating, &y.Country, &title, &count, &reason, &peak, &peakYear,
			&y.Season.Opponents, &y.Season.Wins, &y.Season.Losses, &y.Season.Draws, &y.LastPlayed,
			&y.Career.Opponents, &y.Career.Wins, &y.Career.Losses, &y.Career.Draws,
			&y.PreviousRating, &y.PreviousRD, &y.PreviousCRating)
		if title != nil {
			y.Title = &Title{Code: *title, Count: *count, Reason: *reason}
		}
		if peak != nil {
			y.Peak = &Peak{Code: *peak, Year: peakYear}
		}
		return y, err
	})
	if err != nil {
		return YearBoard{}, fmt.Errorf("reading run %d's %d board: %w", run, year, err)
	}
	return b, nil
}

// Year is a year a run's ratings cover, and whether it has a World Champion.
type Year struct {
	Year     int
	Champion bool
}

// Years returns the years a published run's GlickoD history covers, newest
// first, each marked if the run derived a World Champion for it.
func (r *RatingsReader) Years(ctx context.Context, run int64) ([]Year, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT y.year, EXISTS (SELECT 1 FROM player_yearly_titles t WHERE t.run_id = $1 AND t.year = y.year AND t.title = 'WC')
		FROM (SELECT DISTINCT extract(year FROM period_date)::int AS year FROM rating_history
			WHERE run_id = $1 AND model_type = 'glicko2_daneo' AND player_count = 0) y
		ORDER BY 1 DESC`, run)
	if err != nil {
		return nil, fmt.Errorf("reading run %d's years: %w", run, err)
	}
	years, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Year, error) {
		var y Year
		err := row.Scan(&y.Year, &y.Champion)
		return y, err
	})
	if err != nil {
		return nil, fmt.Errorf("reading run %d's years: %w", run, err)
	}
	return years, nil
}
