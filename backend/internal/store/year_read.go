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
// none that year). A year with no history is ErrNoHistory. Players with a
// deleted or banned CGE account are left out, and named DeletedPlayer among
// the champions.
func (r *RatingsReader) YearBoard(ctx context.Context, run int64, year int, model string, format int) (YearBoard, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return YearBoard{}, err
	}
	b := YearBoard{Year: year, Champions: []Champion{}}

	var snapshot *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT coalesce(
			(SELECT max(period_date) FROM rating_history
			 WHERE run_id = $1 AND model_type = $2 AND player_count = $3
				AND period_date >= make_date($4::int, 1, 1) AND period_date < make_date($4::int + 1, 1, 1)),
			(SELECT max(period_date) FROM rating_history WHERE run_id = $1 AND extract(year FROM period_date) = $4
				AND EXISTS (SELECT 1 FROM rating_runs WHERE run_id = $1 -- no scan for a year outside the run's games
					AND $4 BETWEEN coalesce(extract(year FROM first_game), $4) AND coalesce(extract(year FROM last_game), $4)))),
			(SELECT max(period_date) FROM rating_history
			 WHERE run_id = $1 AND model_type = $2 AND player_count = $3
				AND period_date >= make_date($4::int - 1, 1, 1) AND period_date < make_date($4::int, 1, 1)),
			(SELECT count(DISTINCT match_id) FROM pairwise_matches
			 WHERE run_id = $1 AND date >= make_date($4::int, 1, 1) AND date < make_date($4::int + 1, 1, 1))`,
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
	champions := make([]*string, len(b.Champions))
	for i := range b.Champions {
		champions[i] = &b.Champions[i].Name
	}
	if err := r.maskNames(ctx, champions); err != nil {
		return YearBoard{}, fmt.Errorf("reading run %d's %d champions: %w", run, year, err)
	}

	rows, err = r.pool.Query(ctx, `
		WITH rec AS (
			SELECT player_name AS name,
				sum(opponents) FILTER (WHERE year = $5) AS s_opps, sum(wins) FILTER (WHERE year = $5) AS s_wins,
				sum(losses) FILTER (WHERE year = $5) AS s_losses, sum(draws) FILTER (WHERE year = $5) AS s_draws,
				max(last_played) FILTER (WHERE year = $5) AS last_played,
				sum(opponents) AS c_opps, sum(wins) AS c_wins, sum(losses) AS c_losses, sum(draws) AS c_draws
			FROM player_year_records
			WHERE run_id = $1 AND ($3 = 0 OR player_count = $3) AND year <= $5
			GROUP BY player_name)
		SELECT h.player_name, h.rating, h.rd, h.c_rating, m.country_code, t.title, t.title_count, t.badge_reason,
			k.peak_title, k.peak_year,
			coalesce(rec.s_opps, 0), coalesce(rec.s_wins, 0), coalesce(rec.s_losses, 0), coalesce(rec.s_draws, 0),
			rec.last_played,
			coalesce(rec.c_opps, 0), coalesce(rec.c_wins, 0), coalesce(rec.c_losses, 0), coalesce(rec.c_draws, 0),
			prev.rating, prev.rd, prev.c_rating
		FROM (SELECT DISTINCT ON (player_name) * FROM rating_history
			WHERE run_id = $1 AND model_type = $2 AND player_count = $3 AND period_date <= $4 AND extract(year FROM period_date) = $5
			ORDER BY player_name, period_date DESC) h
		LEFT JOIN players p ON p.name = h.player_name
		LEFT JOIN members m ON m.player_id = p.player_id
		LEFT JOIN player_titles t ON t.run_id = $1 AND t.player_name = h.player_name
		LEFT JOIN player_peak_titles k ON k.run_id = $1 AND k.player_name = h.player_name
		LEFT JOIN rec ON rec.name = h.player_name
		LEFT JOIN LATERAL (SELECT rating, rd, c_rating FROM rating_history
			WHERE run_id = $1 AND model_type = $2 AND player_count = $3 AND player_name = h.player_name AND period_date <= $6
			ORDER BY period_date DESC LIMIT 1) prev ON true
		WHERE `+notHiddenPlayer+`
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
