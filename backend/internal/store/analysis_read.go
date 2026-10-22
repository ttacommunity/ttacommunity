package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"ttacommunity/backend/internal/round"
)

// CalibrationRow is how well one model predicted games in one format, over
// every period together: Mode is "standard" or "walk_forward".
type CalibrationRow struct {
	Mode              string
	Model             string
	Format            int
	Matches           int
	Brier, ECE        float64
	LogLoss, Accuracy *float64
	Bins              []byte
}

// ModelRating is one player's rating and rank in one page model.
type ModelRating struct {
	Player    string
	Model     string
	Rating    float64
	Rank      *int
	Opponents int
}

// Calibration returns a published run's aggregated calibration, standard and
// walk-forward, for every model and format.
func (r *RatingsReader) Calibration(ctx context.Context, run int64) ([]CalibrationRow, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT 'standard', model_type, player_count, matches_evaluated, brier_score, ece, log_loss, accuracy, bin_data
		FROM standard_calibration WHERE run_id = $1 AND period_month = 'AGGREGATED'
		UNION ALL
		SELECT 'walk_forward', model_type, player_count, matches_evaluated, brier_score, ece, log_loss, accuracy, bin_data
		FROM walk_forward_calibration WHERE run_id = $1 AND period_month = 'AGGREGATED'
		ORDER BY 1, 2, 3`, run)
	if err != nil {
		return nil, fmt.Errorf("reading run %d's calibration: %w", run, err)
	}
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CalibrationRow, error) {
		var c CalibrationRow
		err := row.Scan(&c.Mode, &c.Model, &c.Format, &c.Matches, &c.Brier, &c.ECE, &c.LogLoss, &c.Accuracy, &c.Bins)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("reading run %d's calibration: %w", run, err)
	}
	return got, nil
}

// AnalysisRatings returns a published run's ratings in PlayerPageModels for a format.
func (r *RatingsReader) AnalysisRatings(ctx context.Context, run int64, format int) ([]ModelRating, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT player_name, model_type, rating, rank, opponents_count FROM player_ratings
		WHERE run_id = $1 AND player_count = $2 AND model_type = ANY($3)
		ORDER BY player_name, model_type`, run, format, PlayerPageModels)
	if err != nil {
		return nil, fmt.Errorf("reading run %d's ratings: %w", run, err)
	}
	ratings, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ModelRating, error) {
		var m ModelRating
		err := row.Scan(&m.Player, &m.Model, &m.Rating, &m.Rank, &m.Opponents)
		return m, err
	})
	if err != nil {
		return nil, fmt.Errorf("reading run %d's ratings: %w", run, err)
	}
	return ratings, nil
}

// ActivePlayers returns the players active in a published run as upstream's
// analysis counts them: more than 10 games in the 365 days up to its newest game.
func (r *RatingsReader) ActivePlayers(ctx context.Context, run int64) (map[string]bool, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return nil, err
	}
	// Only the last year's pairs are read, through the (run_id, date) index; a pair's date is its game's.
	rows, err := r.pool.Query(ctx, `
		WITH recent AS (
			SELECT DISTINCT match_id FROM pairwise_matches
			WHERE run_id = $1 AND date >= (SELECT max(date) FROM pairwise_matches WHERE run_id = $1) - 365)
		SELECT s.player_name FROM recent r JOIN seats s ON s.game_id = r.match_id
		GROUP BY s.player_name HAVING count(*) > 10`, run)
	if err != nil {
		return nil, fmt.Errorf("reading run %d's active players: %w", run, err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("reading run %d's active players: %w", run, err)
	}
	active := make(map[string]bool, len(names))
	for _, n := range names {
		active[n] = true
	}
	return active, nil
}

// CompareModels are the models upstream's player comparison shows, in its order.
var CompareModels = []string{
	"glicko2_daneo", "glicko2_daneo_softer", "glicko2_std", "glicko2_std_softer", "glicko2_mp",
	"glicko2_mp_softer", "glicko2_adapt", "glicko2_adapt_softer", "whr", "whr_softer",
}

// DefaultCompared are the players upstream compares when nothing is searched, in the order its page shows them.
var DefaultCompared = []string{"Genghisip", "Martin_Pecheur", "Weidenbaum", "a440", "pv4", "saru"}

// ComparedPlayer is one player's ratings in CompareModels, for one format.
type ComparedPlayer struct {
	Name           string
	Country, Title *string
	Ratings        []ComparedRating
}

// ComparedRating is a player's standing in one model.
type ComparedRating struct {
	Model               string
	Rank                *int
	Rating, RD, CRating float64
	Opponents           int
	WinRate             float64
}

// ComparedPlayers returns up to 15 players whose name contains a term or
// whose country is one, ignoring case, strongest first by GlickoD's all-format
// C rating; with no terms, the DefaultCompared players who exist, in order.
// Players with a deleted or banned CGE account are left out.
func (r *RatingsReader) ComparedPlayers(ctx context.Context, run int64, format int, terms []string) ([]ComparedPlayer, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return nil, err
	}
	query := `
		SELECT p.name, m.country_code, t.title FROM players p
		LEFT JOIN members m ON m.player_id = p.player_id
		LEFT JOIN player_titles t ON t.run_id = $1 AND t.player_name = p.name
		LEFT JOIN player_ratings g ON g.run_id = $1 AND g.player_name = p.name AND g.model_type = 'glicko2_daneo' AND g.player_count = 0
		WHERE EXISTS (SELECT 1 FROM unnest($2::text[]) term
			WHERE strpos(lower(p.name), lower(term)) > 0 OR lower(m.country_code) = lower(term))
		  AND ` + notHiddenPlayer + `
		ORDER BY g.c_rating DESC NULLS LAST, p.name LIMIT 15`
	args := []any{run, terms}
	if len(terms) == 0 {
		query = `
			SELECT p.name, m.country_code, t.title FROM players p
			LEFT JOIN members m ON m.player_id = p.player_id
			LEFT JOIN player_titles t ON t.run_id = $1 AND t.player_name = p.name
			WHERE p.name = ANY($2) AND ` + notHiddenPlayer + ` ORDER BY array_position($2, p.name)`
		args = []any{run, DefaultCompared}
	}
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("finding players to compare in run %d: %w", run, err)
	}
	players, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ComparedPlayer, error) {
		var p ComparedPlayer
		err := row.Scan(&p.Name, &p.Country, &p.Title)
		return p, err
	})
	if err != nil {
		return nil, fmt.Errorf("finding players to compare in run %d: %w", run, err)
	}
	if len(players) == 0 {
		return players, nil
	}

	names := make([]string, len(players))
	for i, p := range players {
		names[i] = p.Name
	}
	rows, err = r.pool.Query(ctx, `
		SELECT player_name, model_type, rank, rating, rd, c_rating, opponents_count, win_rate FROM player_ratings
		WHERE run_id = $1 AND player_count = $2 AND player_name = ANY($3) AND model_type = ANY($4)
		ORDER BY array_position($4, model_type)`, run, format, names, CompareModels)
	if err != nil {
		return nil, fmt.Errorf("reading compared ratings in run %d: %w", run, err)
	}
	byName := map[string][]ComparedRating{}
	var name string
	var c ComparedRating
	_, err = pgx.ForEachRow(rows, []any{&name, &c.Model, &c.Rank, &c.Rating, &c.RD, &c.CRating, &c.Opponents, &c.WinRate}, func() error {
		byName[name] = append(byName[name], c)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading compared ratings in run %d: %w", run, err)
	}
	for i := range players {
		players[i].Ratings = byName[players[i].Name]
	}
	return players, nil
}

// Activity is who plays and where, as upstream's activity page counts them.
type Activity struct {
	Active, Inactive int
	Formats          []FormatCount
	Titles           []TitleCount
	Countries        []CountryRatings
}

// FormatCount is how many of a run's games had Format players, and the
// one-to-one pairs the engine rated from them.
type FormatCount struct{ Format, Games, Pairs int }

// TitleCount is how many players hold Title.
type TitleCount struct {
	Title   string
	Players int
}

// CountryRatings is a country's rated players and their mean Glicko-2
// Standard all-format rating: by title (nil with none) and overall.
type CountryRatings struct {
	Country          string
	Players          int
	GM, MPlus, PPlus *float64
	All              float64
}

// Activity returns a published run's active and inactive players (active: a
// last game after inactiveOnOrBefore, in any model or format), its games per
// format and their pairs, players per title, and the 15 countries with most rated players.
// Ties are in reverse code order, as upstream's SQLite returns them.
func (r *RatingsReader) Activity(ctx context.Context, run int64, inactiveOnOrBefore time.Time) (Activity, error) {
	var a Activity
	if err := r.requirePublished(ctx, run); err != nil {
		return a, err
	}
	err := r.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE last > $2), count(*) FILTER (WHERE last IS NULL OR last <= $2) FROM players p
		LEFT JOIN (SELECT player_name, max(last_played) AS last FROM player_ratings WHERE run_id = $1 GROUP BY player_name) r
			ON r.player_name = p.name`, run, inactiveOnOrBefore).Scan(&a.Active, &a.Inactive)
	if err != nil {
		return a, fmt.Errorf("counting run %d's active players: %w", run, err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT player_count, count(DISTINCT match_id), count(*) FROM pairwise_matches WHERE run_id = $1
		GROUP BY player_count ORDER BY player_count`, run)
	if err == nil {
		a.Formats, err = pgx.CollectRows(rows, pgx.RowToStructByPos[FormatCount])
	}
	if err != nil {
		return a, fmt.Errorf("counting run %d's games per format: %w", run, err)
	}

	rows, err = r.pool.Query(ctx, `
		SELECT title, count(*) FROM player_titles WHERE run_id = $1 GROUP BY title ORDER BY 2 DESC, title DESC`, run)
	if err == nil {
		a.Titles, err = pgx.CollectRows(rows, pgx.RowToStructByPos[TitleCount])
	}
	if err != nil {
		return a, fmt.Errorf("counting run %d's titles: %w", run, err)
	}

	rows, err = r.pool.Query(ctx, `
		SELECT m.country_code, count(DISTINCT p.name),
			avg(s.rating) FILTER (WHERE t.title = 'GM'),
			avg(s.rating) FILTER (WHERE t.title IN ('GM', 'M')),
			avg(s.rating) FILTER (WHERE t.title IN ('GM', 'M', 'P')),
			avg(s.rating)
		FROM members m
		JOIN players p ON p.player_id = m.player_id
		JOIN player_ratings s ON s.run_id = $1 AND s.player_name = p.name AND s.model_type = 'glicko2_std' AND s.player_count = 0
		LEFT JOIN player_titles t ON t.run_id = $1 AND t.player_name = p.name
		WHERE m.country_code IS NOT NULL
		GROUP BY m.country_code ORDER BY 2 DESC, 1 DESC LIMIT 15`, run)
	if err == nil {
		a.Countries, err = pgx.CollectRows(rows, pgx.RowToStructByPos[CountryRatings])
	}
	if err != nil {
		return a, fmt.Errorf("reading run %d's countries: %w", run, err)
	}
	return a, nil
}

// SeatStatsGroup is seat statistics for one player count (2, 3, 4).
type SeatStatsGroup struct {
	PlayerCount int
	TotalGames  int
	Seats       []SeatWinrateRow
}

// SeatWinrateRow is performance for one starting seat.
type SeatWinrateRow struct {
	Seat    int
	Games   int
	Wins    int
	Draws   int
	WinRate float64
	Places  []SeatPlaceRate
}

// SeatPlaceRate is placement rate for one finish position.
type SeatPlaceRate struct {
	Place int
	Count int
	Rate  float64
}

// seatTiers are the skill tiers SeatAnalysis filters by, "all" first.
var seatTiers = []string{"all", "grandmaster", "master", "advanced", "intermediate", "beginner"}

// seatCounts is one tier's games by player count, seat and place, its outright wins and draws, and its fractional win shares.
type seatCounts struct {
	places      map[[3]int]int
	wins, draws map[[2]int]int
	winShares   map[[2]int]float64
}

// SeatAnalysis returns seat statistics grouped by player count (2P, 3P, 4P) and filtered by skill tier
// (an unknown tier counts as "all").
func (r *RatingsReader) SeatAnalysis(ctx context.Context, run int64, format int, tier string) ([]SeatStatsGroup, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return nil, err
	}
	tiers, err := r.seatCountsOf(ctx, run)
	if err != nil {
		return nil, err
	}
	counts, ok := tiers[tier]
	if !ok {
		counts = tiers["all"]
	}
	return seatGroups(counts, format), nil
}

// seatCountsOf returns run's seat counts for every tier in seatTiers, reading them on the run's first request
// and from memory after, while it is one of the latest runs asked for: every tier and format of the page then
// answers without the database.
func (r *RatingsReader) seatCountsOf(ctx context.Context, run int64) (map[string]seatCounts, error) {
	return r.seats.Get(run, struct{}{}, func() (map[string]seatCounts, error) {
		return r.readSeatCounts(ctx, run)
	})
}

// readSeatCounts reads run's seat counts for every tier in seatTiers, in one query.
func (r *RatingsReader) readSeatCounts(ctx context.Context, run int64) (map[string]seatCounts, error) {
	rows, err := r.pool.Query(ctx, `
		WITH games_in_run AS (
			SELECT match_id AS game_id, date, player_count
			FROM (
				SELECT match_id, date, player_count,
					row_number() OVER (PARTITION BY match_id) AS rn
				FROM pairwise_matches
				WHERE run_id = $1
			) x
			WHERE rn = 1
		),
		seat_rows AS (
			SELECT g.player_count, coalesce(s.starting_position + 1, s.seat) AS seat, s.place,
				avg(coalesce(rh.rating, 1500.0)) OVER game AS avg_rating,
				count(*) FILTER (WHERE s.place = 1) OVER game AS winners
			FROM games_in_run g
			JOIN seats s ON s.game_id = g.game_id
			LEFT JOIN LATERAL (
				SELECT rating FROM rating_history h
				WHERE h.run_id = $1 AND h.model_type = 'glicko2_daneo' AND h.player_count = 0
				  AND h.player_name = s.player_name AND h.period_date <= g.date
				ORDER BY h.period_date DESC LIMIT 1
			) rh ON true
			WINDOW game AS (PARTITION BY s.game_id)
		)
		SELECT
			CASE
				WHEN avg_rating >= 1750 THEN 'grandmaster'
				WHEN avg_rating >= 1625 THEN 'master'
				WHEN avg_rating >= 1500 THEN 'advanced'
				WHEN avg_rating >= 1375 THEN 'intermediate'
				ELSE 'beginner'
			END,
			player_count, seat, place, count(*),
			count(*) FILTER (WHERE place = 1 AND winners = 1),
			count(*) FILTER (WHERE place = 1 AND winners > 1),
			coalesce(sum(CASE WHEN place = 1 THEN 1.0::float8 / winners ELSE 0 END), 0)::float8
		FROM seat_rows
		GROUP BY 1, 2, 3, 4`, run)
	if err != nil {
		return nil, fmt.Errorf("reading run %d's seat analysis: %w", run, err)
	}

	tiers := make(map[string]seatCounts, len(seatTiers))
	for _, t := range seatTiers {
		tiers[t] = seatCounts{
			places:    map[[3]int]int{},
			wins:      map[[2]int]int{},
			draws:     map[[2]int]int{},
			winShares: map[[2]int]float64{},
		}
	}
	var tier string
	var pc, seat, place, count, outrightWins, draws int
	var winShares float64
	_, err = pgx.ForEachRow(rows, []any{&tier, &pc, &seat, &place, &count, &outrightWins, &draws, &winShares}, func() error {
		for _, c := range []seatCounts{tiers[tier], tiers["all"]} {
			c.places[[3]int{pc, seat, place}] += count
			c.wins[[2]int{pc, seat}] += outrightWins
			c.draws[[2]int{pc, seat}] += draws
			c.winShares[[2]int{pc, seat}] += winShares
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading run %d's seat analysis: %w", run, err)
	}
	return tiers, nil
}

// seatGroups returns counts' groups for format: one per player count, or that format's alone.
func seatGroups(c seatCounts, format int) []SeatStatsGroup {
	counts, seatOutrightWins, seatDraws, seatWinShares := c.places, c.wins, c.draws, c.winShares
	playerCounts := []int{2, 3, 4}
	if format == 2 || format == 3 || format == 4 {
		playerCounts = []int{format}
	}

	groups := make([]SeatStatsGroup, 0, len(playerCounts))
	for _, pc := range playerCounts {
		totalGames := 0
		for p := 1; p <= pc; p++ {
			totalGames += counts[[3]int{pc, 1, p}]
		}
		group := SeatStatsGroup{
			PlayerCount: pc,
			TotalGames:  totalGames,
			Seats:       make([]SeatWinrateRow, 0, pc),
		}
		for s := 1; s <= pc; s++ {
			seatGames := 0
			for p := 1; p <= pc; p++ {
				seatGames += counts[[3]int{pc, s, p}]
			}
			wins := seatOutrightWins[[2]int{pc, s}]
			draws := seatDraws[[2]int{pc, s}]
			winShares := seatWinShares[[2]int{pc, s}]
			winRate := 0.0
			if seatGames > 0 {
				winRate = round.To(winShares/float64(seatGames)*100.0, 2)
			}
			places := make([]SeatPlaceRate, 0, pc)
			for p := 1; p <= pc; p++ {
				placeCount := counts[[3]int{pc, s, p}]
				rate := 0.0
				if seatGames > 0 {
					rate = round.To(float64(placeCount)/float64(seatGames)*100.0, 2)
				}
				places = append(places, SeatPlaceRate{
					Place: p,
					Count: placeCount,
					Rate:  rate,
				})
			}
			group.Seats = append(group.Seats, SeatWinrateRow{
				Seat:    s,
				Games:   seatGames,
				Wins:    wins,
				Draws:   draws,
				WinRate: winRate,
				Places:  places,
			})
		}
		groups = append(groups, group)
	}
	return groups
}
