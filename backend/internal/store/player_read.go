package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"ttacommunity/backend/internal/round"
)

// ErrPlayerNotFound is a name no player has, or one only a case-insensitive
// match would find and more than one player does.
var ErrPlayerNotFound = errors.New("no player has this name")

// DeletedPlayer is what a player whose CGE accounts are deleted or banned is masked as.
const DeletedPlayer = "Deleted player"

// notHiddenPlayer excludes players who have any deleted or banned CGE account.
const notHiddenPlayer = `NOT EXISTS (
	SELECT 1 FROM player_accounts pa
	WHERE pa.player_id = p.player_id AND (pa.deleted OR pa.banned)
)`

// notUnknownPlayer excludes UnknownPlayer: an empty seat, nobody to look up. Its seats still show by that name.
const notUnknownPlayer = `p.name <> '` + UnknownPlayer + `'`

// hiddenName is true when the name in column is, ignoring case, a player with any deleted or banned CGE account.
func hiddenName(column string) string {
	return `EXISTS (
	SELECT 1 FROM players hp JOIN player_accounts pa USING (player_id)
	WHERE lower(hp.name) = lower(` + column + `) AND (pa.deleted OR pa.banned)
)`
}

// PlayerPageModels are the models the player page shows: the ones the engine computes, Glicko-D continuous and
// with a season reset, and standard Glicko-2.
var PlayerPageModels = []string{"glicko2_daneo", "glicko2_daneo_softer", "glicko2_std"}

// gamesPerPage is how many games one page of a player's games holds.
const gamesPerPage = 30

// Player is one player's details and ratings in a run.
type Player struct {
	Name       string
	Country    *string
	Title      *Title
	Peak       *Peak
	Badges     []ExtraBadge
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

// Seat is one player's culture in a game.
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

// PlayerAt is a player's name and month (in "YYYY-MM" format).
type PlayerAt struct {
	Name  string
	Month string
}

// Rival is a player's record against one opponent.
type Rival struct {
	Name                       string
	Games, Wins, Losses, Draws int
}

// playerRunGames selects each game run $1 rated for the player named $2, once, with its date and format as the
// run took them. A run's own rows are read, not the games: they are few pages, and one game edited after the run
// keeps what the run rated until the next.
const playerRunGames = `
	SELECT match_id, date, player_count FROM pairwise_matches WHERE run_id = $1 AND player_a = $2
	UNION
	SELECT match_id, date, player_count FROM pairwise_matches WHERE run_id = $1 AND player_b = $2`

// resolve returns the stored name for asked in a published run: the exact
// name, else the only one equal to it ignoring case. Players with deleted or
// banned CGE accounts, and UnknownPlayer, are not resolved.
func (r *RatingsReader) resolve(ctx context.Context, run int64, asked string) (string, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return "", err
	}
	rows, err := r.pool.Query(ctx, "SELECT p.name FROM players p WHERE lower(p.name) = lower($1) AND "+notUnknownPlayer+" AND "+notHiddenPlayer, asked)
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
		SELECT m.country_code, t.title, t.title_count, t.badge_reason, k.peak_title, k.peak_year,
			(SELECT max(date) FROM pairwise_matches WHERE run_id = $1)
		FROM players p
		LEFT JOIN members m ON m.player_id = p.player_id
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
	badges, err := r.badgesByPlayer(ctx, []string{name})
	if err != nil {
		return Player{}, err
	}
	p.Badges = badges[name]

	rows, err := r.pool.Query(ctx, `
		SELECT player_count, count(*), max(date) FROM (`+playerRunGames+`) mine
		GROUP BY player_count`, run, name)
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
// for a player in a format (0 is all), newest first, each with the date and
// format the run took. A page past the last gives the last. Seats keep their
// stored names, for rating estimates: the caller masks the HiddenNames before
// answering.
func (r *RatingsReader) PlayerMatches(ctx context.Context, run int64, asked string, format, page int) (string, MatchPage, error) {
	name, err := r.resolve(ctx, run, asked)
	if err != nil {
		return "", MatchPage{}, err
	}
	const mine = `(` + playerRunGames + `) mine WHERE $3 = 0 OR player_count = $3`
	var mp MatchPage
	err = r.pool.QueryRow(ctx, `SELECT count(*) FROM `+mine, run, name, format).Scan(&mp.Total)
	if err != nil {
		return "", MatchPage{}, fmt.Errorf("counting a player's games in run %d: %w", run, err)
	}
	mp.Pages = max(1, (mp.Total+gamesPerPage-1)/gamesPerPage)
	mp.Page = min(max(page, 1), mp.Pages)

	rows, err := r.pool.Query(ctx, `
		SELECT match_id, date, player_count FROM `+mine+`
		ORDER BY date DESC, match_id DESC
		LIMIT $4 OFFSET $5`, run, name, format, gamesPerPage, (mp.Page-1)*gamesPerPage)
	if err != nil {
		return "", MatchPage{}, fmt.Errorf("paging a player's games in run %d: %w", run, err)
	}
	mp.Matches, err = r.gamesAsRated(ctx, rows)
	if err != nil {
		return "", MatchPage{}, fmt.Errorf("reading a player's games in run %d: %w", run, err)
	}
	return name, mp, nil
}

// gamesAsRated reads rows of a run's games (id, date, format) and returns those games in their order, with
// their tournament, replay code and seats, and the run's date and format.
func (r *RatingsReader) gamesAsRated(ctx context.Context, rows pgx.Rows) ([]PlayerMatch, error) {
	rated, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (PlayerMatch, error) {
		var m PlayerMatch
		return m, row.Scan(&m.ID, &m.Date, &m.PlayerCount)
	})
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(rated))
	for i, m := range rated {
		ids[i] = m.ID
	}
	lines, err := r.pool.Query(ctx, `
		SELECT m.game_id, m.name, m.date, m.player_count, m.share_code, m.players, m.cultures
		FROM game_lines m WHERE m.game_id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	games, err := pgx.CollectRows(lines, scanMatch)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]PlayerMatch, len(games))
	for _, g := range games {
		byID[g.ID] = g
	}
	out := make([]PlayerMatch, 0, len(rated))
	for _, m := range rated {
		g := byID[m.ID]
		g.ID, g.Date, g.PlayerCount = m.ID, m.Date, m.PlayerCount
		out = append(out, g)
	}
	return out, nil
}

// scanMatch reads a game and its seats, in the order they were recorded.
func scanMatch(row pgx.CollectableRow) (PlayerMatch, error) {
	var m PlayerMatch
	var names []string
	var cultures []float64
	err := row.Scan(&m.ID, &m.Tournament, &m.Date, &m.PlayerCount, &m.ReplayCode, &names, &cultures)
	for i := range names {
		m.Seats = append(m.Seats, Seat{Name: names[i], Score: cultures[i]})
	}
	return m, err
}

// RatingsOf returns the ratings of the named players who have one in a published run's
// model and format. An unknown or unpublished run is ErrRunNotFound.
func (r *RatingsReader) RatingsOf(ctx context.Context, run int64, model string, format int, names []string) (map[string]Rated, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return nil, err
	}
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

// HistoricalRatingsOf returns the ratings of the named players at the start of each
// requested month (strictly before period_date in that month) in a published run's
// model and format. If a player had no rating before that month, they are absent from the map.
func (r *RatingsReader) HistoricalRatingsOf(ctx context.Context, run int64, model string, format int, at []PlayerAt) (map[PlayerAt]Rated, error) {
	if len(at) == 0 {
		return map[PlayerAt]Rated{}, nil
	}
	if err := r.requirePublished(ctx, run); err != nil {
		return nil, err
	}
	seen := make(map[PlayerAt]bool, len(at))
	var names []string
	var monthStarts []string
	for _, p := range at {
		if seen[p] || p.Name == "" || p.Month == "" {
			continue
		}
		seen[p] = true
		names = append(names, p.Name)
		monthStarts = append(monthStarts, p.Month+"-01")
	}
	if len(names) == 0 {
		return map[PlayerAt]Rated{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT u.name, to_char(u.month_start, 'YYYY-MM'), h.rating, h.rd
		FROM (
			SELECT DISTINCT unnest($4::text[]) AS name, unnest($5::date[]) AS month_start
		) u
		JOIN LATERAL (
			SELECT rating, rd
			FROM rating_history
			WHERE run_id = $1 AND model_type = $2 AND player_count = $3
			  AND player_name = u.name AND period_date < u.month_start
			ORDER BY period_date DESC
			LIMIT 1
		) h ON true`, run, model, format, names, monthStarts)
	if err != nil {
		return nil, fmt.Errorf("reading historical ratings in run %d: %w", run, err)
	}
	got := make(map[PlayerAt]Rated, len(seen))
	for rows.Next() {
		var name, month string
		var p Rated
		if err := rows.Scan(&name, &month, &p.Rating, &p.RD); err != nil {
			return nil, fmt.Errorf("reading historical ratings in run %d: %w", run, err)
		}
		got[PlayerAt{Name: name, Month: month}] = p
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading historical ratings in run %d: %w", run, err)
	}
	return got, nil
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

// HiddenNames returns the subset of names whose players, matched ignoring case, have a deleted or banned
// CGE account: the names a public answer shows as DeletedPlayer or leaves out.
func (r *RatingsReader) HiddenNames(ctx context.Context, names []string) (map[string]bool, error) {
	if len(names) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT n FROM unnest($1::text[]) n WHERE `+hiddenName("n"), names)
	if err != nil {
		return nil, fmt.Errorf("checking hidden players: %w", err)
	}
	hidden, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("checking hidden players: %w", err)
	}
	if len(hidden) == 0 {
		return nil, nil
	}
	set := make(map[string]bool, len(hidden))
	for _, n := range hidden {
		set[n] = true
	}
	return set, nil
}

// maskNames replaces each of names whose player has a deleted or banned CGE account with DeletedPlayer.
func (r *RatingsReader) maskNames(ctx context.Context, names []*string) error {
	var list []string
	seen := map[string]bool{}
	for _, n := range names {
		if !seen[*n] {
			seen[*n] = true
			list = append(list, *n)
		}
	}
	hidden, err := r.HiddenNames(ctx, list)
	if err != nil {
		return err
	}
	for _, n := range names {
		if hidden[*n] {
			*n = DeletedPlayer
		}
	}
	return nil
}

// maskMatchSeats replaces the names of seats whose players are deleted or banned with DeletedPlayer.
func (r *RatingsReader) maskMatchSeats(ctx context.Context, matches []PlayerMatch) error {
	var names []*string
	for i := range matches {
		for j := range matches[i].Seats {
			names = append(names, &matches[i].Seats[j].Name)
		}
	}
	return r.maskNames(ctx, names)
}
