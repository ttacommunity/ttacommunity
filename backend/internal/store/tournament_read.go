package store

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

// TrophyRow is one player on a series' board: medals and a title for leagues
// and the World Championship, medals and honours for the cups, or ladder
// games, wins and win rate. Fields a board does not show are nil.
type TrophyRow struct {
	Name                 string
	WorldChampion        bool
	Title                *string
	Gold, Silver, Bronze *int
	Details              *string
	Games, Wins          *int
	WinRate              *float64
}

// SeasonRecord is one player's season in a series' top division.
type SeasonRecord struct {
	Name          string
	WorldChampion bool
	Points        float64
	Games         int
	Placings      []int
	Season        int
	Division      string
	LastPlayed    time.Time
}

// GameRecord is a top-division game, by its winner's score.
type GameRecord struct {
	Name          string
	WorldChampion bool
	Score         float64
	Opponents     []Seat
	Tournament    string
	Date          time.Time
}

// TournamentSeries is what a series' Hall of Fame page shows.
type TournamentSeries struct {
	Games, Seasons int
	Board          []TrophyRow
	SeasonRecords  []SeasonRecord
	GameRecords    []GameRecord
}

// recordsShown is how many seasons and games the page lists, as upstream's.
const recordsShown = 10

// TournamentSeries returns a published run's results for a series: its game
// and edition counts, its board in the engine's order, and the ten best
// top-division seasons (points, then wins, then first seen) and games the
// engine marked for records (winner's score, then load order).
func (r *RatingsReader) TournamentSeries(ctx context.Context, run int64, series string) (TournamentSeries, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return TournamentSeries{}, err
	}
	got := TournamentSeries{Board: []TrophyRow{}, SeasonRecords: []SeasonRecord{}, GameRecords: []GameRecord{}}
	fail := func(what string, err error) (TournamentSeries, error) {
		return TournamentSeries{}, fmt.Errorf("reading run %d's %s %s: %w", run, series, what, err)
	}

	err := r.pool.QueryRow(ctx, `
		SELECT coalesce(max(games), 0), coalesce(max(editions), 0) FROM tournament_series WHERE run_id = $1 AND series = $2`,
		run, series).Scan(&got.Games, &got.Seasons)
	if err != nil {
		return fail("counts", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT b.player_name, p.player_name IS NOT NULL, b.title, b.gold, b.silver, b.bronze,
		       b.details, b.games, b.wins, b.win_rate
		FROM tournament_boards b
		LEFT JOIN player_peak_titles p ON p.run_id = b.run_id AND p.player_name = b.player_name AND p.peak_title = 'WC'
		WHERE b.run_id = $1 AND b.series = $2
		ORDER BY b.position`, run, series)
	if err != nil {
		return fail("board", err)
	}
	got.Board, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (TrophyRow, error) {
		var b TrophyRow
		return b, row.Scan(&b.Name, &b.WorldChampion, &b.Title, &b.Gold, &b.Silver, &b.Bronze,
			&b.Details, &b.Games, &b.Wins, &b.WinRate)
	})
	if err != nil {
		return fail("board", err)
	}

	rows, err = r.pool.Query(ctx, `
		SELECT s.player_name, p.player_name IS NOT NULL, s.points, s.games, s.placings, s.season, s.division, s.last_date
		FROM tournament_season_records s
		LEFT JOIN player_peak_titles p ON p.run_id = s.run_id AND p.player_name = s.player_name AND p.peak_title = 'WC'
		WHERE s.run_id = $1 AND s.series = $2
		ORDER BY s.points DESC, s.placings[1] DESC, s.first_match_id
		LIMIT $3`, run, series, recordsShown)
	if err != nil {
		return fail("season records", err)
	}
	got.SeasonRecords, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (SeasonRecord, error) {
		var s SeasonRecord
		return s, row.Scan(&s.Name, &s.WorldChampion, &s.Points, &s.Games, &s.Placings, &s.Season, &s.Division, &s.LastPlayed)
	})
	if err != nil {
		return fail("season records", err)
	}

	rows, err = r.pool.Query(ctx, `
		SELECT m.tournament, m.date, m.player1, m.score1, m.player2, m.score2,
		       m.player3, m.score3, m.player4, m.score4
		FROM tournament_games g JOIN matches m USING (match_id)
		WHERE g.run_id = $1 AND g.series = $2 AND g.in_records
		ORDER BY greatest(m.score1, m.score2, m.score3, m.score4) DESC, m.match_id
		LIMIT $3`, run, series, recordsShown)
	if err != nil {
		return fail("game records", err)
	}
	got.GameRecords, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (GameRecord, error) {
		var g GameRecord
		var names [4]*string
		var scores [4]*float64
		err := row.Scan(&g.Tournament, &g.Date, &names[0], &scores[0], &names[1], &scores[1],
			&names[2], &scores[2], &names[3], &scores[3])
		var seats []Seat
		for i := range names {
			if names[i] != nil && scores[i] != nil {
				seats = append(seats, Seat{Name: *names[i], Score: *scores[i]})
			}
		}
		// Stable, so a tied top score goes to the earlier seat, as upstream's sort.
		slices.SortStableFunc(seats, func(a, b Seat) int { return cmp.Compare(b.Score, a.Score) })
		if len(seats) > 0 {
			g.Name, g.Score, g.Opponents = seats[0].Name, seats[0].Score, seats[1:]
		}
		return g, err
	})
	if err != nil {
		return fail("game records", err)
	}
	return got, r.markWorldChampions(ctx, run, got.GameRecords)
}

// markWorldChampions sets WorldChampion on each game record whose winner the
// run holds as a World Champion.
func (r *RatingsReader) markWorldChampions(ctx context.Context, run int64, games []GameRecord) error {
	rows, err := r.pool.Query(ctx, "SELECT player_name FROM player_peak_titles WHERE run_id = $1 AND peak_title = 'WC'", run)
	if err != nil {
		return fmt.Errorf("reading run %d's World Champions: %w", run, err)
	}
	champions, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("reading run %d's World Champions: %w", run, err)
	}
	for i := range games {
		games[i].WorldChampion = slices.Contains(champions, games[i].Name)
	}
	return nil
}
