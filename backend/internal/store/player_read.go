package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"ttacommunity/backend/internal/round"
)

// ErrPlayerNotFound is a name no player has, or one only a case-insensitive
// match would find and more than one player does.
var ErrPlayerNotFound = errors.New("no player has this name")

// PlayerPageModels are the models the player page shows: each engine,
// continuous and with a season reset.
var PlayerPageModels = []string{
	"glicko2_daneo", "glicko2_daneo_softer", "glicko2_std", "glicko2_std_softer", "glicko2_mp",
	"glicko2_mp_softer", "glicko2_adapt", "glicko2_adapt_softer", "whr", "whr_softer",
}

// gamesPerPage is how many games one page of a player's games holds.
const gamesPerPage = 30

// Player is one player's details and ratings in a run.
type Player struct {
	Name       string
	Country    *string
	Title      *Title
	Peak       *Peak
	LastPlayed *time.Time
	NewestGame *time.Time
	Games      map[int]int
	Ratings    []PlayerRating
}

// PlayerRating is a player's rating in one model and format.
type PlayerRating struct {
	Model     string
	Format    int
	Rank      *int
	Rating    float64
	RD        float64
	CRating   float64
	Opponents int
	WinRate   float64
}

// HistoryPoint is a player's rating at the end of one period in one model.
type HistoryPoint struct {
	Model   string
	Date    time.Time
	Rating  float64
	CRating float64
}

// Seat is one player's score in a game.
type Seat struct {
	Name  string
	Score float64
}

// PlayerMatch is one game, its seats in the order they were recorded.
type PlayerMatch struct {
	ID          int64
	Tournament  string
	Date        time.Time
	PlayerCount int
	ReplayCode  *string
	Seats       []Seat
}

// MatchPage is one page of a player's games.
type MatchPage struct {
	Page, Pages, Total int
	Matches            []PlayerMatch
}

// Rated is a rating and its deviation.
type Rated struct {
	Rating, RD float64
}

// Rival is a player's record against one opponent.
type Rival struct {
	Name                       string
	Games, Wins, Losses, Draws int
}

// playerGames selects the ids of the games run rated for the player named $2.
const playerGames = `
	SELECT match_id FROM pairwise_matches WHERE run_id = $1 AND player_a = $2
	UNION
	SELECT match_id FROM pairwise_matches WHERE run_id = $1 AND player_b = $2`

// resolve returns the stored name for asked in a published run: the exact
// name, else the only one equal to it ignoring case.
func (r *RatingsReader) resolve(ctx context.Context, run int64, asked string) (string, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return "", err
	}
	rows, err := r.pool.Query(ctx, "SELECT name FROM players WHERE lower(name) = lower($1)", asked)
	if err != nil {
		return "", fmt.Errorf("looking up a player: %w", err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return "", fmt.Errorf("looking up a player: %w", err)
	}
	for _, n := range names {
		if n == asked {
			return n, nil
		}
	}
	if len(names) == 1 {
		return names[0], nil
	}
	return "", ErrPlayerNotFound
}

// Player returns a player's details, games per format (0 is all) and ratings
// in PlayerPageModels, in a published run.
func (r *RatingsReader) Player(ctx context.Context, run int64, asked string) (Player, error) {
	name, err := r.resolve(ctx, run, asked)
	if err != nil {
		return Player{}, err
	}
	p := Player{Name: name, Games: map[int]int{0: 0}}

	var title, reason, peak *string
	var count, peakYear *int
	err = r.pool.QueryRow(ctx, `
		SELECT p.country_code, t.title, t.title_count, t.badge_reason, k.peak_title, k.peak_year,
			(SELECT max(date) FROM pairwise_matches WHERE run_id = $1)
		FROM players p
		LEFT JOIN player_titles t ON t.run_id = $1 AND t.player_name = p.name
		LEFT JOIN player_peak_titles k ON k.run_id = $1 AND k.player_name = p.name
		WHERE p.name = $2`, run, name).Scan(&p.Country, &title, &count, &reason, &peak, &peakYear, &p.NewestGame)
	if err != nil {
		return Player{}, fmt.Errorf("reading a player in run %d: %w", run, err)
	}
	if title != nil {
		p.Title = &Title{Code: *title, Count: *count, Reason: *reason}
	}
	if peak != nil {
		p.Peak = &Peak{Code: *peak, Year: peakYear}
	}

	rows, err := r.pool.Query(ctx, `
		SELECT m.player_count, count(*), max(m.date)
		FROM matches m JOIN (`+playerGames+`) mine USING (match_id)
		GROUP BY m.player_count`, run, name)
	if err != nil {
		return Player{}, fmt.Errorf("counting a player's games in run %d: %w", run, err)
	}
	for rows.Next() {
		var format, games int
		var last time.Time
		if err := rows.Scan(&format, &games, &last); err != nil {
			return Player{}, fmt.Errorf("counting a player's games in run %d: %w", run, err)
		}
		p.Games[format] = games
		p.Games[0] += games
		if p.LastPlayed == nil || last.After(*p.LastPlayed) {
			p.LastPlayed = &last
		}
	}
	if err := rows.Err(); err != nil {
		return Player{}, fmt.Errorf("counting a player's games in run %d: %w", run, err)
	}

	rows, err = r.pool.Query(ctx, `
		SELECT model_type, player_count, rank, rating, rd, c_rating, opponents_count, win_rate
		FROM player_ratings
		WHERE run_id = $1 AND player_name = $2 AND model_type = ANY($3)
		ORDER BY model_type, player_count`, run, name, PlayerPageModels)
	if err != nil {
		return Player{}, fmt.Errorf("reading a player's ratings in run %d: %w", run, err)
	}
	p.Ratings, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (PlayerRating, error) {
		var pr PlayerRating
		err := row.Scan(&pr.Model, &pr.Format, &pr.Rank, &pr.Rating, &pr.RD, &pr.CRating, &pr.Opponents, &pr.WinRate)
		return pr, err
	})
	if err != nil {
		return Player{}, fmt.Errorf("reading a player's ratings in run %d: %w", run, err)
	}
	return p, nil
}

// PlayerHistory returns the stored name and a player's rating after each
// period in PlayerPageModels for one format, by model, oldest first.
func (r *RatingsReader) PlayerHistory(ctx context.Context, run int64, asked string, format int) (string, []HistoryPoint, error) {
	name, err := r.resolve(ctx, run, asked)
	if err != nil {
		return "", nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT model_type, period_date, rating, c_rating
		FROM rating_history
		WHERE run_id = $1 AND player_name = $2 AND player_count = $3 AND model_type = ANY($4)
		ORDER BY model_type, period_date`, run, name, format, PlayerPageModels)
	if err != nil {
		return "", nil, fmt.Errorf("reading a player's history in run %d: %w", run, err)
	}
	points, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (HistoryPoint, error) {
		var h HistoryPoint
		err := row.Scan(&h.Model, &h.Date, &h.Rating, &h.CRating)
		return h, err
	})
	if err != nil {
		return "", nil, fmt.Errorf("reading a player's history in run %d: %w", run, err)
	}
	return name, points, nil
}

// PlayerMatches returns the stored name and one page of the games run rated
// for a player in a format (0 is all), newest first. A page past the last
// gives the last.
func (r *RatingsReader) PlayerMatches(ctx context.Context, run int64, asked string, format, page int) (string, MatchPage, error) {
	name, err := r.resolve(ctx, run, asked)
	if err != nil {
		return "", MatchPage{}, err
	}
	const inFormat = ` WHERE $3 = 0 OR m.player_count = $3`
	var mp MatchPage
	err = r.pool.QueryRow(ctx, `
		SELECT count(*) FROM matches m JOIN (`+playerGames+`) mine USING (match_id)`+inFormat,
		run, name, format).Scan(&mp.Total)
	if err != nil {
		return "", MatchPage{}, fmt.Errorf("counting a player's games in run %d: %w", run, err)
	}
	mp.Pages = max(1, (mp.Total+gamesPerPage-1)/gamesPerPage)
	mp.Page = min(max(page, 1), mp.Pages)

	rows, err := r.pool.Query(ctx, `
		SELECT m.match_id, m.tournament, m.date, m.player_count, m.replay_code,
			m.player1, m.score1, m.player2, m.score2, m.player3, m.score3, m.player4, m.score4
		FROM matches m JOIN (`+playerGames+`) mine USING (match_id)`+inFormat+`
		ORDER BY m.date DESC, m.match_id DESC
		LIMIT $4 OFFSET $5`, run, name, format, gamesPerPage, (mp.Page-1)*gamesPerPage)
	if err != nil {
		return "", MatchPage{}, fmt.Errorf("reading a player's games in run %d: %w", run, err)
	}
	mp.Matches, err = pgx.CollectRows(rows, scanMatch)
	if err != nil {
		return "", MatchPage{}, fmt.Errorf("reading a player's games in run %d: %w", run, err)
	}
	return name, mp, nil
}

// scanMatch reads a game and its seats, in the order they were recorded.
func scanMatch(row pgx.CollectableRow) (PlayerMatch, error) {
	var m PlayerMatch
	var names [4]*string
	var scores [4]*float64
	err := row.Scan(&m.ID, &m.Tournament, &m.Date, &m.PlayerCount, &m.ReplayCode,
		&names[0], &scores[0], &names[1], &scores[1], &names[2], &scores[2], &names[3], &scores[3])
	for i := range names {
		if names[i] != nil && scores[i] != nil {
			m.Seats = append(m.Seats, Seat{Name: *names[i], Score: *scores[i]})
		}
	}
	return m, err
}

// RatingsOf returns the ratings of the named players who have one in a run's
// model and format.
func (r *RatingsReader) RatingsOf(ctx context.Context, run int64, model string, format int, names []string) (map[string]Rated, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT player_name, rating, rd FROM player_ratings
		WHERE run_id = $1 AND model_type = $2 AND player_count = $3 AND player_name = ANY($4)`,
		run, model, format, names)
	if err != nil {
		return nil, fmt.Errorf("reading ratings in run %d: %w", run, err)
	}
	got := map[string]Rated{}
	for rows.Next() {
		var n string
		var p Rated
		if err := rows.Scan(&n, &p.Rating, &p.RD); err != nil {
			return nil, fmt.Errorf("reading ratings in run %d: %w", run, err)
		}
		got[n] = p
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading ratings in run %d: %w", run, err)
	}
	return got, nil
}

// PlayerRivals returns the stored name and a player's record against the
// 200 opponents they met most in a format (0 is all), most games first, then
// most wins.
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

// peakEngines are the continuous models whose peaks the career box shows, in its order.
var peakEngines = []string{"glicko2_daneo", "glicko2_std", "glicko2_mp", "glicko2_adapt", "whr"}

// sharedGames is how many of two players' shared games Versus returns.
const sharedGames = 100

// CareerYear is a player's record in one calendar year.
type CareerYear struct {
	Year                           int
	Opponents, Wins, Losses, Draws int
	PeakRating                     *float64
	Title                          *string
}

// ModelPeak is a model's highest rating for a player, when it was, and the
// player's rank that month.
type ModelPeak struct {
	Model  string
	Rating float64
	Rank   int
	Date   time.Time
}

// PlayerCareer returns the stored name, a player's record in each year of a
// format (all formats when the format has none), and each continuous model's
// peak, as upstream's career box computes them: a year's peak rating is the
// highest in any model's history that year; a model's peak is its highest
// rating rounded to 0.1, ranked by raw rating among that month's history.
func (r *RatingsReader) PlayerCareer(ctx context.Context, run int64, asked string, format int) (string, []CareerYear, []ModelPeak, error) {
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

	rows, err := r.pool.Query(ctx, `
		SELECT extract(year FROM period_date)::int, max(rating)
		FROM rating_history
		WHERE run_id = $1 AND player_name = $2 AND player_count = $3
		GROUP BY 1`, run, name, format)
	if err != nil {
		return "", nil, nil, fmt.Errorf("reading a player's yearly peaks in run %d: %w", run, err)
	}
	yearPeak := map[int]float64{}
	for rows.Next() {
		var y int
		var peak float64
		if err := rows.Scan(&y, &peak); err != nil {
			return "", nil, nil, fmt.Errorf("reading a player's yearly peaks in run %d: %w", run, err)
		}
		yearPeak[y] = round.Tenth(peak)
	}
	if err := rows.Err(); err != nil {
		return "", nil, nil, fmt.Errorf("reading a player's yearly peaks in run %d: %w", run, err)
	}
	for i := range years {
		if p, ok := yearPeak[years[i].Year]; ok {
			years[i].PeakRating = &p
		}
	}

	peaks, err := r.modelPeaks(ctx, run, name, format)
	if err != nil {
		return "", nil, nil, err
	}
	return name, years, peaks, nil
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

// modelPeaks finds each continuous model's peak: the first period with the
// highest rating rounded to 0.1, and 1 + how many ratings that period beat it.
func (r *RatingsReader) modelPeaks(ctx context.Context, run int64, name string, format int) ([]ModelPeak, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT model_type, period_date, rating FROM rating_history
		WHERE run_id = $1 AND player_name = $2 AND player_count = $3 AND model_type = ANY($4)
		ORDER BY period_date`, run, name, format, peakEngines)
	if err != nil {
		return nil, fmt.Errorf("reading a player's peaks in run %d: %w", run, err)
	}
	best := map[string]ModelPeak{}
	for rows.Next() {
		var p ModelPeak
		if err := rows.Scan(&p.Model, &p.Date, &p.Rating); err != nil {
			return nil, fmt.Errorf("reading a player's peaks in run %d: %w", run, err)
		}
		p.Rating = round.Tenth(p.Rating)
		if b, ok := best[p.Model]; !ok || p.Rating > b.Rating {
			best[p.Model] = p
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading a player's peaks in run %d: %w", run, err)
	}

	peaks := []ModelPeak{}
	for _, model := range peakEngines {
		p, ok := best[model]
		if !ok {
			continue
		}
		err := r.pool.QueryRow(ctx, `
			SELECT count(*) + 1 FROM rating_history
			WHERE run_id = $1 AND model_type = $2 AND player_count = $3 AND period_date = $4 AND rating > $5`,
			run, model, format, p.Date, p.Rating).Scan(&p.Rank)
		if err != nil {
			return nil, fmt.Errorf("ranking a player's %s peak in run %d: %w", model, run, err)
		}
		peaks = append(peaks, p)
	}
	return peaks, nil
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
		SELECT m.match_id, m.tournament, m.date, m.player_count, m.replay_code,
			m.player1, m.score1, m.player2, m.score2, m.player3, m.score3, m.player4, m.score4
		FROM matches m JOIN (
			SELECT match_id FROM pairwise_matches WHERE run_id = $1 AND player_a = $2 AND player_b = $3
			UNION
			SELECT match_id FROM pairwise_matches WHERE run_id = $1 AND player_a = $3 AND player_b = $2) shared USING (match_id)
		ORDER BY m.date DESC, m.match_id DESC
		LIMIT $4`, run, a, b, sharedGames)
	if err != nil {
		return "", "", nil, fmt.Errorf("reading shared games in run %d: %w", run, err)
	}
	games, err := pgx.CollectRows(rows, scanMatch)
	if err != nil {
		return "", "", nil, fmt.Errorf("reading shared games in run %d: %w", run, err)
	}
	return a, b, games, nil
}
