// The pairs of players in a run's last month of rated games, with their ratings going into each game, for /upsets.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// UpsetPair is one player against one opponent in a rated game: both players' Glicko-D ratings in the game's
// format from the last period before it, Name's score against Other (1, 0.5 or 0), and whether Name timed out.
type UpsetPair struct {
	GameID                           int64
	Date                             time.Time
	Tournament                       string
	PlayerCount                      int
	Name, Other                      string
	Rating, RD, OtherRating, OtherRD float64
	Score                            float64
	TimedOut                         bool
}

// UpsetPairs returns the month up to a published run's last game, and every rated pair in it in format (0 is all),
// each way round. A pair is left out when either player had no rating before the game, or is hidden.
func (r *RatingsReader) UpsetPairs(ctx context.Context, run int64, format int) (time.Time, time.Time, []UpsetPair, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return time.Time{}, time.Time{}, nil, err
	}
	var from, to time.Time
	err := r.pool.QueryRow(ctx, `SELECT (last_game - interval '1 month' + interval '1 day')::date, last_game
		FROM rating_runs WHERE run_id = $1 AND last_game IS NOT NULL`, run).Scan(&from, &to)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, time.Time{}, nil, nil
	}
	if err != nil {
		return time.Time{}, time.Time{}, nil, fmt.Errorf("reading run %d's last month: %w", run, err)
	}
	rows, err := r.pool.Query(ctx, `
		WITH shown AS (SELECT p.player_id, p.name FROM players p WHERE `+notHiddenPlayer+`),
		pairs AS (
			SELECT match_id, date, tournament, player_count, player_a AS me, player_b AS opp, outcome_a AS score
			FROM pairwise_matches WHERE run_id = $1 AND glicko_eligible AND date BETWEEN $2 AND $3 AND ($4 = 0 OR player_count = $4)
			UNION ALL
			SELECT match_id, date, tournament, player_count, player_b, player_a, 1 - outcome_a
			FROM pairwise_matches WHERE run_id = $1 AND glicko_eligible AND date BETWEEN $2 AND $3 AND ($4 = 0 OR player_count = $4)
		)
		SELECT x.match_id, x.date, x.tournament, x.player_count, a.name, b.name, ha.rating, ha.rd, hb.rating, hb.rd,
			x.score, coalesce(s.outcome = 'timed_out', false)
		FROM pairs x
		JOIN shown a ON a.player_id = x.me
		JOIN shown b ON b.player_id = x.opp
		JOIN LATERAL (SELECT rating, rd FROM rating_history h WHERE h.run_id = $1 AND h.model_type = 'glicko2_daneo'
			AND h.player_count = x.player_count AND h.player_id = x.me AND h.period_date < x.date
			ORDER BY h.period_date DESC LIMIT 1) ha ON true
		JOIN LATERAL (SELECT rating, rd FROM rating_history h WHERE h.run_id = $1 AND h.model_type = 'glicko2_daneo'
			AND h.player_count = x.player_count AND h.player_id = x.opp AND h.period_date < x.date
			ORDER BY h.period_date DESC LIMIT 1) hb ON true
		LEFT JOIN seats s ON s.game_id = x.match_id AND s.player_id = x.me`, run, from, to, format)
	if err != nil {
		return time.Time{}, time.Time{}, nil, fmt.Errorf("reading run %d's last month of pairs: %w", run, err)
	}
	pairs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (UpsetPair, error) {
		var p UpsetPair
		err := row.Scan(&p.GameID, &p.Date, &p.Tournament, &p.PlayerCount, &p.Name, &p.Other, &p.Rating, &p.RD,
			&p.OtherRating, &p.OtherRD, &p.Score, &p.TimedOut)
		return p, err
	})
	if err != nil {
		return time.Time{}, time.Time{}, nil, fmt.Errorf("reading run %d's last month of pairs: %w", run, err)
	}
	return from, to, pairs, nil
}
