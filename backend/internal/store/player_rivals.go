// A player's rivals, career by year, head-to-head games with another player, and matrix of results.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"ttacommunity/backend/internal/round"
)

// Rival is a player's record against one opponent.
type Rival struct {
	Name                       string
	Games, Wins, Losses, Draws int
}

// PlayerRivals returns the stored name and a player's record against the
// 200 opponents they met most in a format (0 is all), most games first, then
// most wins. Opponents with a deleted or banned CGE account are left out.
func (r *RatingsReader) PlayerRivals(ctx context.Context, run int64, asked string, format int) (string, []Rival, error) {
	name, err := r.resolve(ctx, run, asked)
	if err != nil {
		return "", nil, err
	}
	rows, err := r.pool.Query(ctx, `
		WITH mine AS (
			SELECT player_b AS opponent, outcome_a AS outcome FROM pairwise_matches
			WHERE run_id = $1 AND player_a = $2 AND ($3 = 0 OR player_count = $3)
			UNION ALL
			SELECT player_a, 1 - outcome_a FROM pairwise_matches
			WHERE run_id = $1 AND player_b = $2 AND ($3 = 0 OR player_count = $3))
		SELECT opponent, count(*), count(*) FILTER (WHERE outcome = 1),
			count(*) FILTER (WHERE outcome = 0), count(*) FILTER (WHERE outcome = 0.5)
		FROM mine
		GROUP BY opponent
		HAVING NOT `+hiddenName("opponent")+`
		ORDER BY 2 DESC, 3 DESC, opponent
		LIMIT 200`, run, name, format)
	if err != nil {
		return "", nil, fmt.Errorf("reading a player's rivals in run %d: %w", run, err)
	}
	rivals, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Rival, error) {
		var v Rival
		err := row.Scan(&v.Name, &v.Games, &v.Wins, &v.Losses, &v.Draws)
		return v, err
	})
	if err != nil {
		return "", nil, fmt.Errorf("reading a player's rivals in run %d: %w", run, err)
	}
	return name, rivals, nil
}

// sharedGames is how many of two players' shared games Versus returns.
const sharedGames = 100

// CareerYear is a player's record in one calendar year and up to its end.
type CareerYear struct {
	Year                                                   int
	Opponents, Wins, Losses, Draws                         int
	CareerOpponents, CareerWins, CareerLosses, CareerDraws int
	PeakRating                                             *float64
	YearEnd                                                *float64
	Title                                                  *string
}

// MatrixCell is a player's rating in one page model, and their rank.
type MatrixCell struct {
	Model               string
	Rating, RD, CRating float64
	Rank                *int
}

// CareerPeak is a player's highest rating in one model, when it was, and
// their rank that month.
type CareerPeak struct {
	Rating float64
	Rank   int
	Date   time.Time
}

// PlayerCareer returns the stored name, a player's record in each year of a
// format (all formats when the format has none), and their peak in model, or
// nil without history there.
func (r *RatingsReader) PlayerCareer(ctx context.Context, run int64, asked string, format int, model string) (string, []CareerYear, *CareerPeak, error) {
	name, err := r.resolve(ctx, run, asked)
	if err != nil {
		return "", nil, nil, err
	}
	years, err := r.careerYears(ctx, run, name, format)
	if err == nil && len(years) == 0 && format != 0 {
		years, err = r.careerYears(ctx, run, name, 0)
	}
	if err != nil {
		return "", nil, nil, err
	}

	yearPeak, yearEnd, peak, err := r.modelPeaks(ctx, run, name, format, model)
	if err != nil {
		return "", nil, nil, err
	}
	for i := range years {
		if p, ok := yearPeak[years[i].Year]; ok {
			years[i].PeakRating = &p
		}
		if e, ok := yearEnd[years[i].Year]; ok {
			years[i].YearEnd = &e
		}
		prev := CareerYear{}
		if i > 0 {
			prev = years[i-1]
		}
		years[i].CareerOpponents = prev.CareerOpponents + years[i].Opponents
		years[i].CareerWins = prev.CareerWins + years[i].Wins
		years[i].CareerLosses = prev.CareerLosses + years[i].Losses
		years[i].CareerDraws = prev.CareerDraws + years[i].Draws
	}
	return name, years, peak, nil
}

// modelPeaks reads a player's history in one model and format, all to 0.1:
// each year's highest and last rating, and the career peak (the first period
// with the highest), ranked by conservative rating among that period's
// ratings. The peak is nil without history.
func (r *RatingsReader) modelPeaks(ctx context.Context, run int64, name string, format int, model string) (map[int]float64, map[int]float64, *CareerPeak, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT period_date, rating, c_rating FROM rating_history
		WHERE run_id = $1 AND model_type = $2 AND player_count = $3 AND player_name = $4
		ORDER BY period_date`, run, model, format, name)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading a player's history in run %d: %w", run, err)
	}
	yearPeak, yearEnd := map[int]float64{}, map[int]float64{}
	var peak *CareerPeak
	var peakC float64
	for rows.Next() {
		var date time.Time
		var rating, c float64
		if err := rows.Scan(&date, &rating, &c); err != nil {
			return nil, nil, nil, fmt.Errorf("reading a player's history in run %d: %w", run, err)
		}
		rating = round.Tenth(rating)
		if p, ok := yearPeak[date.Year()]; !ok || rating > p {
			yearPeak[date.Year()] = rating
		}
		yearEnd[date.Year()] = rating
		if peak == nil || rating > peak.Rating {
			peak, peakC = &CareerPeak{Rating: rating, Date: date}, c
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("reading a player's history in run %d: %w", run, err)
	}
	if peak == nil {
		return yearPeak, yearEnd, nil, nil
	}
	err = r.pool.QueryRow(ctx, `
		SELECT count(*) + 1 FROM rating_history
		WHERE run_id = $1 AND model_type = $2 AND player_count = $3 AND period_date = $4 AND c_rating > $5`,
		run, model, format, peak.Date, peakC).Scan(&peak.Rank)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ranking a player's peak in run %d: %w", run, err)
	}
	return yearPeak, yearEnd, peak, nil
}

func (r *RatingsReader) careerYears(ctx context.Context, run int64, name string, format int) ([]CareerYear, error) {
	rows, err := r.pool.Query(ctx, `
		WITH mine AS (
			SELECT date, outcome_a AS outcome FROM pairwise_matches
			WHERE run_id = $1 AND player_a = $2 AND ($3 = 0 OR player_count = $3)
			UNION ALL
			SELECT date, 1 - outcome_a FROM pairwise_matches
			WHERE run_id = $1 AND player_b = $2 AND ($3 = 0 OR player_count = $3))
		SELECT y.year, y.opponents, y.wins, y.losses, y.draws, t.title
		FROM (
			SELECT extract(year FROM date)::int AS year, count(*) AS opponents,
				count(*) FILTER (WHERE outcome = 1) AS wins, count(*) FILTER (WHERE outcome = 0) AS losses,
				count(*) FILTER (WHERE outcome = 0.5) AS draws
			FROM mine GROUP BY 1) y
		LEFT JOIN player_yearly_titles t ON t.run_id = $1 AND t.player_name = $2 AND t.year = y.year
		ORDER BY y.year`, run, name, format)
	if err != nil {
		return nil, fmt.Errorf("reading a player's years in run %d: %w", run, err)
	}
	years, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CareerYear, error) {
		var y CareerYear
		err := row.Scan(&y.Year, &y.Opponents, &y.Wins, &y.Losses, &y.Draws, &y.Title)
		return y, err
	})
	if err != nil {
		return nil, fmt.Errorf("reading a player's years in run %d: %w", run, err)
	}
	return years, nil
}

// Versus returns both stored names and the latest 100 games run rated with
// both players, newest first.
func (r *RatingsReader) Versus(ctx context.Context, run int64, asked, other string) (string, string, []PlayerMatch, error) {
	a, err := r.resolve(ctx, run, asked)
	if err != nil {
		return "", "", nil, err
	}
	b, err := r.resolve(ctx, run, other)
	if err != nil {
		return "", "", nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT match_id, date, player_count FROM (
			SELECT match_id, date, player_count FROM pairwise_matches WHERE run_id = $1 AND player_a = $2 AND player_b = $3
			UNION
			SELECT match_id, date, player_count FROM pairwise_matches WHERE run_id = $1 AND player_a = $3 AND player_b = $2) shared
		ORDER BY date DESC, match_id DESC
		LIMIT $4`, run, a, b, sharedGames)
	if err != nil {
		return "", "", nil, fmt.Errorf("reading shared games in run %d: %w", run, err)
	}
	games, err := r.gamesAsRated(ctx, rows)
	if err != nil {
		return "", "", nil, fmt.Errorf("reading shared games in run %d: %w", run, err)
	}
	if err := r.maskMatchSeats(ctx, games); err != nil {
		return "", "", nil, err
	}
	return a, b, games, nil
}

// PlayerMatrix returns the stored name and the player's rating in each page
// model for a format, to 0.1: live, with the run's ranks, or at the end of
// year, as upstream's matrix takes it: each model's last period that year,
// ranked by conservative rating that period. A model the player has no
// rating in, or no row in on that period, is left out.
func (r *RatingsReader) PlayerMatrix(ctx context.Context, run int64, asked string, format int, year *int) (string, []MatrixCell, error) {
	name, err := r.resolve(ctx, run, asked)
	if err != nil {
		return "", nil, err
	}
	var rows pgx.Rows
	if year == nil {
		rows, err = r.pool.Query(ctx, `
			SELECT model_type, rating, rd, c_rating, rank FROM player_ratings
			WHERE run_id = $1 AND player_name = $2 AND player_count = $3 AND model_type = ANY($4)
			ORDER BY model_type`, run, name, format, PlayerPageModels)
	} else {
		rows, err = r.pool.Query(ctx, `
			WITH last AS (
				SELECT model_type, (SELECT max(period_date) FROM rating_history
					WHERE run_id = $1 AND model_type = m.model_type AND player_count = $3
						AND period_date >= make_date($5, 1, 1) AND period_date < make_date($5 + 1, 1, 1)) AS period_date
				FROM unnest($4::text[]) m (model_type))
			SELECT h.model_type, h.rating, h.rd, h.c_rating,
				(SELECT count(*) + 1 FROM rating_history o
				 WHERE o.run_id = $1 AND o.model_type = h.model_type AND o.player_count = $3
					AND o.period_date = h.period_date AND o.c_rating > h.c_rating)::int
			FROM rating_history h JOIN last USING (model_type, period_date)
			WHERE h.run_id = $1 AND h.player_name = $2 AND h.player_count = $3
			ORDER BY h.model_type`, run, name, format, PlayerPageModels, *year)
	}
	if err != nil {
		return "", nil, fmt.Errorf("reading a player's matrix in run %d: %w", run, err)
	}
	cells, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (MatrixCell, error) {
		var c MatrixCell
		err := row.Scan(&c.Model, &c.Rating, &c.RD, &c.CRating, &c.Rank)
		c.Rating, c.RD, c.CRating = round.Tenth(c.Rating), round.Tenth(c.RD), round.Tenth(c.CRating)
		return c, err
	})
	if err != nil {
		return "", nil, fmt.Errorf("reading a player's matrix in run %d: %w", run, err)
	}
	return name, cells, nil
}
