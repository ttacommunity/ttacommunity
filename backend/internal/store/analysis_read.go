package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
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

// AnalysisRatings returns a published run's ratings in PlayerPageModels for a
// format, and the players active as upstream's analysis counts them: more than
// 10 games in the 365 days up to the run's newest game.
func (r *RatingsReader) AnalysisRatings(ctx context.Context, run int64, format int) ([]ModelRating, map[string]bool, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return nil, nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT player_name, model_type, rating, rank, opponents_count FROM player_ratings
		WHERE run_id = $1 AND player_count = $2 AND model_type = ANY($3)
		ORDER BY player_name, model_type`, run, format, PlayerPageModels)
	if err != nil {
		return nil, nil, fmt.Errorf("reading run %d's ratings: %w", run, err)
	}
	ratings, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ModelRating, error) {
		var m ModelRating
		err := row.Scan(&m.Player, &m.Model, &m.Rating, &m.Rank, &m.Opponents)
		return m, err
	})
	if err != nil {
		return nil, nil, fmt.Errorf("reading run %d's ratings: %w", run, err)
	}

	rows, err = r.pool.Query(ctx, `
		WITH run_games AS (
			SELECT DISTINCT m.match_id, m.date, m.player1, m.player2, m.player3, m.player4
			FROM matches m JOIN pairwise_matches p ON p.match_id = m.match_id AND p.run_id = $1),
		recent AS (
			SELECT * FROM run_games WHERE date >= (SELECT max(date) FROM run_games) - 365)
		SELECT name FROM (
			SELECT player1 AS name FROM recent UNION ALL SELECT player2 FROM recent
			UNION ALL SELECT player3 FROM recent WHERE player3 IS NOT NULL
			UNION ALL SELECT player4 FROM recent WHERE player4 IS NOT NULL) seats
		GROUP BY name HAVING count(*) > 10`, run)
	if err != nil {
		return nil, nil, fmt.Errorf("reading run %d's active players: %w", run, err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, nil, fmt.Errorf("reading run %d's active players: %w", run, err)
	}
	active := make(map[string]bool, len(names))
	for _, n := range names {
		active[n] = true
	}
	return ratings, active, nil
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
func (r *RatingsReader) ComparedPlayers(ctx context.Context, run int64, format int, terms []string) ([]ComparedPlayer, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return nil, err
	}
	query := `
		SELECT p.name, p.country_code, t.title FROM players p
		LEFT JOIN player_titles t ON t.run_id = $1 AND t.player_name = p.name
		LEFT JOIN player_ratings g ON g.run_id = $1 AND g.player_name = p.name AND g.model_type = 'glicko2_daneo' AND g.player_count = 0
		WHERE EXISTS (SELECT 1 FROM unnest($2::text[]) term
			WHERE strpos(lower(p.name), lower(term)) > 0 OR lower(p.country_code) = lower(term))
		ORDER BY g.c_rating DESC NULLS LAST, p.name LIMIT 15`
	args := []any{run, terms}
	if len(terms) == 0 {
		query = `
			SELECT p.name, p.country_code, t.title FROM players p
			LEFT JOIN player_titles t ON t.run_id = $1 AND t.player_name = p.name
			WHERE p.name = ANY($2) ORDER BY array_position($2, p.name)`
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

// FormatCount is how many of a run's games had Format players.
type FormatCount struct{ Format, Games int }

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
// format, players per title, and the 15 countries with most rated players.
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
		SELECT player_count, count(*) FROM matches
		WHERE match_id IN (SELECT match_id FROM pairwise_matches WHERE run_id = $1)
		GROUP BY player_count ORDER BY player_count`, run)
	if err == nil {
		a.Formats, err = pgx.CollectRows(rows, pgx.RowToStructByPos[FormatCount])
	}
	if err != nil {
		return a, fmt.Errorf("counting run %d's games per format: %w", run, err)
	}

	rows, err = r.pool.Query(ctx, `
		SELECT title, count(*) FROM player_titles WHERE run_id = $1 GROUP BY title ORDER BY 2 DESC, title`, run)
	if err == nil {
		a.Titles, err = pgx.CollectRows(rows, pgx.RowToStructByPos[TitleCount])
	}
	if err != nil {
		return a, fmt.Errorf("counting run %d's titles: %w", run, err)
	}

	rows, err = r.pool.Query(ctx, `
		SELECT p.country_code, count(DISTINCT p.name),
			avg(s.rating) FILTER (WHERE t.title = 'GM'),
			avg(s.rating) FILTER (WHERE t.title IN ('GM', 'M')),
			avg(s.rating) FILTER (WHERE t.title IN ('GM', 'M', 'P')),
			avg(s.rating)
		FROM players p
		JOIN player_ratings s ON s.run_id = $1 AND s.player_name = p.name AND s.model_type = 'glicko2_std' AND s.player_count = 0
		LEFT JOIN player_titles t ON t.run_id = $1 AND t.player_name = p.name
		WHERE p.country_code IS NOT NULL
		GROUP BY p.country_code ORDER BY 2 DESC, 1 LIMIT 15`, run)
	if err == nil {
		a.Countries, err = pgx.CollectRows(rows, pgx.RowToStructByPos[CountryRatings])
	}
	if err != nil {
		return a, fmt.Errorf("reading run %d's countries: %w", run, err)
	}
	return a, nil
}
