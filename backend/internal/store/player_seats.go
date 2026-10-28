// A player's results by starting turn position, across formats.
package store

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"ttacommunity/backend/internal/round"
)

// PlayerSeatStats is a player's performance broken down by starting turn position across formats.
type PlayerSeatStats struct {
	Name    string
	Unrated bool
	Groups  []PlayerSeatGroup
	Overall PlayerSeatOverview
}

// PlayerSeatOverview summarizes a player's overall performance across all seats.
type PlayerSeatOverview struct {
	TotalGames    int
	TotalWins     int
	TotalDraws    int
	WinRate       float64
	AvgCulture    float64
	StrongestSeat *PlayerSeatDelta
	WeakestSeat   *PlayerSeatDelta
}

// PlayerSeatDelta represents the seat with the highest/lowest win rate relative to overall.
type PlayerSeatDelta struct {
	PlayerCount  int
	Seat         int
	WinRate      float64
	DeltaWinRate float64
}

// PlayerSeatGroup holds seat statistics for one player count (2, 3, or 4).
type PlayerSeatGroup struct {
	PlayerCount int
	TotalGames  int
	TotalWins   int
	TotalDraws  int
	WinRate     float64
	AvgCulture  float64
	Seats       []PlayerSeatRow
}

// PlayerSeatRow holds performance in one turn position.
type PlayerSeatRow struct {
	Seat         int
	Games        int
	Wins         int
	Draws        int
	WinRate      float64
	DeltaWinRate float64
	AvgCulture   float64
	Places       []SeatPlaceRate
}

// PlayerSeats calculates seat statistics for a player: by default rated games only,
// or all games when unrated is true.
func (r *RatingsReader) PlayerSeats(ctx context.Context, run int64, asked string, unrated bool) (PlayerSeatStats, error) {
	name, err := r.resolve(ctx, run, asked)
	if err != nil {
		return PlayerSeatStats{}, err
	}

	// The games rated are counted by the run as it is made; every played game is counted here, from the player's seats.
	query := `
		SELECT player_count, seat, games, wins, draws, avg_culture, p1, p2, p3, p4
		FROM player_seat_stats WHERE run_id = $1 AND player_name = $2
		ORDER BY player_count, seat`
	args := []any{run, name}
	if unrated {
		query = `
		WITH mine AS (
			SELECT (SELECT count(*) FROM seats o WHERE o.game_id = s.game_id) AS player_count,
				s.game_id, s.starting_position, s.seat, s.place, s.culture
			FROM seats s JOIN games g ON g.game_id = s.game_id AND g.state = 'played'
			WHERE s.player_name = $1),
		player_seats AS (
			SELECT
				m.player_count,
				coalesce(m.starting_position + 1, m.seat) AS seat,
				m.place,
				m.culture,
				(SELECT count(*) FROM seats o WHERE o.game_id = m.game_id AND o.place = 1) AS p1_count
			FROM mine m
		)
		SELECT 
			player_count,
			seat,
			count(*) AS games,
			count(*) FILTER (WHERE place = 1 AND p1_count = 1) AS wins,
			count(*) FILTER (WHERE place = 1 AND p1_count > 1) AS draws,
			round(avg(culture)::numeric, 1) AS avg_culture,
			count(*) FILTER (WHERE place = 1) AS p1,
			count(*) FILTER (WHERE place = 2) AS p2,
			count(*) FILTER (WHERE place = 3) AS p3,
			count(*) FILTER (WHERE place = 4) AS p4
		FROM player_seats
		GROUP BY player_count, seat
		ORDER BY player_count, seat`
		args = []any{name}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return PlayerSeatStats{}, fmt.Errorf("reading player %s seat stats in run %d: %w", name, run, err)
	}

	type rawRow struct {
		playerCount    int
		seat           int
		games          int
		wins           int
		draws          int
		avgCulture     float64
		p1, p2, p3, p4 int
	}

	rawList, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (rawRow, error) {
		var rr rawRow
		err := row.Scan(&rr.playerCount, &rr.seat, &rr.games, &rr.wins, &rr.draws, &rr.avgCulture, &rr.p1, &rr.p2, &rr.p3, &rr.p4)
		return rr, err
	})
	if err != nil {
		return PlayerSeatStats{}, fmt.Errorf("reading player %s seat stats in run %d: %w", name, run, err)
	}

	stats := PlayerSeatStats{
		Name:    name,
		Unrated: unrated,
		Groups:  make([]PlayerSeatGroup, 0, 3),
	}

	var totalOverallGames, totalOverallWins, totalOverallDraws int
	var weightedOverallCultureSum float64

	var allSeatDeltas []PlayerSeatDelta

	for _, pc := range []int{2, 3, 4} {
		group := PlayerSeatGroup{
			PlayerCount: pc,
			Seats:       make([]PlayerSeatRow, 0, pc),
		}

		var groupCultureSum float64
		for _, rr := range rawList {
			if rr.playerCount == pc {
				group.TotalGames += rr.games
				group.TotalWins += rr.wins
				group.TotalDraws += rr.draws
				groupCultureSum += rr.avgCulture * float64(rr.games)
			}
		}

		if group.TotalGames > 0 {
			group.WinRate = round.To(float64(group.TotalWins)/float64(group.TotalGames)*100, 2)
			group.AvgCulture = round.Tenth(groupCultureSum / float64(group.TotalGames))
		}

		for s := 1; s <= pc; s++ {
			var match *rawRow
			for i := range rawList {
				if rawList[i].playerCount == pc && rawList[i].seat == s {
					match = &rawList[i]
					break
				}
			}

			row := PlayerSeatRow{
				Seat:   s,
				Places: make([]SeatPlaceRate, 0, pc),
			}

			if match != nil && match.games > 0 {
				row.Games = match.games
				row.Wins = match.wins
				row.Draws = match.draws
				row.WinRate = round.To(float64(match.wins)/float64(match.games)*100, 2)
				row.DeltaWinRate = round.To(row.WinRate-group.WinRate, 2)
				row.AvgCulture = round.Tenth(match.avgCulture)

				counts := []int{match.p1, match.p2, match.p3, match.p4}
				for p := 1; p <= pc; p++ {
					c := counts[p-1]
					rate := round.To(float64(c)/float64(match.games)*100, 2)
					row.Places = append(row.Places, SeatPlaceRate{Place: p, Count: c, Rate: rate})
				}

				allSeatDeltas = append(allSeatDeltas, PlayerSeatDelta{
					PlayerCount:  pc,
					Seat:         s,
					WinRate:      row.WinRate,
					DeltaWinRate: row.DeltaWinRate,
				})
			} else {
				row.DeltaWinRate = 0
				for p := 1; p <= pc; p++ {
					row.Places = append(row.Places, SeatPlaceRate{Place: p, Count: 0, Rate: 0})
				}
			}

			group.Seats = append(group.Seats, row)
		}

		totalOverallGames += group.TotalGames
		totalOverallWins += group.TotalWins
		totalOverallDraws += group.TotalDraws
		weightedOverallCultureSum += groupCultureSum

		stats.Groups = append(stats.Groups, group)
	}

	stats.Overall.TotalGames = totalOverallGames
	stats.Overall.TotalWins = totalOverallWins
	stats.Overall.TotalDraws = totalOverallDraws
	if totalOverallGames > 0 {
		stats.Overall.WinRate = round.To(float64(totalOverallWins)/float64(totalOverallGames)*100, 2)
		stats.Overall.AvgCulture = round.Tenth(weightedOverallCultureSum / float64(totalOverallGames))
	}

	if len(allSeatDeltas) > 0 {
		slices.SortFunc(allSeatDeltas, func(a, b PlayerSeatDelta) int {
			switch {
			case a.DeltaWinRate > b.DeltaWinRate:
				return -1
			case a.DeltaWinRate < b.DeltaWinRate:
				return 1
			}
			return 0
		})
		best := allSeatDeltas[0]
		worst := allSeatDeltas[len(allSeatDeltas)-1]
		stats.Overall.StrongestSeat = &best
		stats.Overall.WeakestSeat = &worst
	}

	return stats, nil
}
