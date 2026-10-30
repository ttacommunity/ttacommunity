// A player's page: their details and ratings in a run, their rating history, and their games a page at a time.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrPlayerNotFound is a name no player has, or one only a case-insensitive
// match would find and more than one player does.
var ErrPlayerNotFound = errors.New("no player has this name")

// notUnknownPlayer excludes UnknownPlayer: an empty seat, nobody to look up. Its seats still show by that name.
const notUnknownPlayer = `p.name <> '` + UnknownPlayer + `'`

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

// playerRunGames selects each game run $1 rated for the player with id $2, once, with its date and format as the
// run took them. A run's own rows are read, not the games: they are few pages, and one game edited after the run
// keeps what the run rated until the next.
const playerRunGames = `
	SELECT match_id, date, player_count FROM pairwise_matches WHERE run_id = $1 AND player_a = $2
	UNION
	SELECT match_id, date, player_count FROM pairwise_matches WHERE run_id = $1 AND player_b = $2`

// namedPlayer is a player a page asks for: their id, and their name now.
type namedPlayer struct {
	ID   int64
	Name string
}

// resolve returns the player asked names in a published run: the exact
// name, else the only one equal to it ignoring case; with neither, the player
// who went by it most recently (player_names), ignoring case. Players with deleted or
// banned CGE accounts, and UnknownPlayer, are not resolved.
func (r *RatingsReader) resolve(ctx context.Context, run int64, asked string) (namedPlayer, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return namedPlayer{}, err
	}
	rows, err := r.pool.Query(ctx, "SELECT p.player_id, p.name FROM players p WHERE lower(p.name) = lower($1) AND "+notUnknownPlayer+" AND "+notHiddenPlayer, asked)
	if err != nil {
		return namedPlayer{}, fmt.Errorf("looking up a player: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowToStructByPos[namedPlayer])
	if err != nil {
		return namedPlayer{}, fmt.Errorf("looking up a player: %w", err)
	}
	for _, p := range found {
		if p.Name == asked {
			return p, nil
		}
	}
	if len(found) == 1 {
		return found[0], nil
	}
	if len(found) > 1 {
		return namedPlayer{}, ErrPlayerNotFound
	}
	var p namedPlayer
	err = r.pool.QueryRow(ctx, `
		SELECT p.player_id, p.name FROM player_names n JOIN players p USING (player_id)
		WHERE lower(n.name) = lower($1) AND `+notUnknownPlayer+` AND `+notHiddenPlayer+`
		ORDER BY n.last_seen DESC, p.player_id LIMIT 1`, asked).Scan(&p.ID, &p.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return namedPlayer{}, ErrPlayerNotFound
	}
	if err != nil {
		return namedPlayer{}, fmt.Errorf("looking up a player's old name: %w", err)
	}
	return p, nil
}

// Player returns a player's details, games per format (0 is all) and ratings
// in PlayerPageModels, in a published run.
func (r *RatingsReader) Player(ctx context.Context, run int64, asked string) (Player, error) {
	who, err := r.resolve(ctx, run, asked)
	if err != nil {
		return Player{}, err
	}
	name := who.Name
	p := Player{Name: name, Games: map[int]int{0: 0}}

	var title, reason, peak *string
	var count, peakYear *int
	err = r.pool.QueryRow(ctx, `
		SELECT m.country_code, t.title, t.title_count, t.badge_reason, k.peak_title, k.peak_year,
			(SELECT max(date) FROM pairwise_matches WHERE run_id = $1)
		FROM players p
		LEFT JOIN members m ON m.player_id = p.player_id
		LEFT JOIN player_titles t ON t.run_id = $1 AND t.player_id = p.player_id
		LEFT JOIN player_peak_titles k ON k.run_id = $1 AND k.player_id = p.player_id
		WHERE p.player_id = $2`, run, who.ID).Scan(&p.Country, &title, &count, &reason, &peak, &peakYear, &p.NewestGame)
	if err != nil {
		return Player{}, fmt.Errorf("reading a player in run %d: %w", run, err)
	}
	p.Title, p.Peak = titleAndPeak(title, count, reason, peak, peakYear)
	badges, err := r.badgesByPlayer(ctx, []string{name})
	if err != nil {
		return Player{}, err
	}
	p.Badges = badges[name]

	rows, err := r.pool.Query(ctx, `
		SELECT player_count, count(*), max(date) FROM (`+playerRunGames+`) mine
		GROUP BY player_count`, run, who.ID)
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
		WHERE run_id = $1 AND player_id = $2 AND model_type = ANY($3)
		ORDER BY model_type, player_count`, run, who.ID, PlayerPageModels)
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
	who, err := r.resolve(ctx, run, asked)
	if err != nil {
		return "", nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT model_type, period_date, rating, c_rating
		FROM rating_history
		WHERE run_id = $1 AND player_id = $2 AND player_count = $3 AND model_type = ANY($4)
		ORDER BY model_type, period_date`, run, who.ID, format, PlayerPageModels)
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
	return who.Name, points, nil
}

// PlayerMatches returns the stored name and one page of the games run rated
// for a player in a format (0 is all), newest first, each with the date and
// format the run took. A page past the last gives the last. Seats keep their
// stored names, for rating estimates: the caller masks the HiddenNames before
// answering.
func (r *RatingsReader) PlayerMatches(ctx context.Context, run int64, asked string, format, page int) (string, MatchPage, error) {
	who, err := r.resolve(ctx, run, asked)
	if err != nil {
		return "", MatchPage{}, err
	}
	const mine = `(` + playerRunGames + `) mine WHERE $3 = 0 OR player_count = $3`
	var mp MatchPage
	err = r.pool.QueryRow(ctx, `SELECT count(*) FROM `+mine, run, who.ID, format).Scan(&mp.Total)
	if err != nil {
		return "", MatchPage{}, fmt.Errorf("counting a player's games in run %d: %w", run, err)
	}
	mp.Pages = max(1, (mp.Total+gamesPerPage-1)/gamesPerPage)
	mp.Page = min(max(page, 1), mp.Pages)

	rows, err := r.pool.Query(ctx, `
		SELECT match_id, date, player_count FROM `+mine+`
		ORDER BY date DESC, match_id DESC
		LIMIT $4 OFFSET $5`, run, who.ID, format, gamesPerPage, (mp.Page-1)*gamesPerPage)
	if err != nil {
		return "", MatchPage{}, fmt.Errorf("paging a player's games in run %d: %w", run, err)
	}
	mp.Matches, err = r.gamesAsRated(ctx, rows)
	if err != nil {
		return "", MatchPage{}, fmt.Errorf("reading a player's games in run %d: %w", run, err)
	}
	return who.Name, mp, nil
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
		SELECT p.name, r.rating, r.rd FROM player_ratings r JOIN players p USING (player_id)
		WHERE r.run_id = $1 AND r.model_type = $2 AND r.player_count = $3 AND p.name = ANY($4)`,
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
		JOIN players p ON p.name = u.name
		JOIN LATERAL (
			SELECT rating, rd
			FROM rating_history
			WHERE run_id = $1 AND model_type = $2 AND player_count = $3
			  AND player_id = p.player_id AND period_date < u.month_start
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
