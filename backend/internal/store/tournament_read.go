package store

import (
	"context"
	"errors"
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
// engine marked for records (winner's score, then load order). A player with
// a deleted or banned CGE account is left off the board, and named
// DeletedPlayer, without a crown, in the records.
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

	got.Board, err = r.board(ctx, run, series, nil)
	if err != nil {
		return fail("board", err)
	}

	rows, err := r.pool.Query(ctx, `
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
	names := make([]*string, len(got.SeasonRecords))
	for i := range got.SeasonRecords {
		names[i] = &got.SeasonRecords[i].Name
	}
	if err := r.maskNames(ctx, names); err != nil {
		return fail("season records", err)
	}
	for i := range got.SeasonRecords {
		got.SeasonRecords[i].WorldChampion = got.SeasonRecords[i].WorldChampion && got.SeasonRecords[i].Name != DeletedPlayer
	}

	rows, err = r.pool.Query(ctx, `
		SELECT l.name, l.date, l.players, l.cultures, l.places
		FROM tournament_games g JOIN game_lines l ON l.game_id = g.match_id
		WHERE g.run_id = $1 AND g.series = $2 AND g.in_records
		ORDER BY (SELECT max(c) FROM unnest(l.cultures) c) DESC, l.game_id
		LIMIT $3`, run, series, recordsShown)
	if err != nil {
		return fail("game records", err)
	}
	got.GameRecords, err = pgx.CollectRows(rows, scanGameRecord)
	if err != nil {
		return fail("game records", err)
	}
	if err := r.maskGameRecords(ctx, got.GameRecords); err != nil {
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

// maskGameRecords replaces the names of winners and opponents whose players are deleted or banned with DeletedPlayer.
func (r *RatingsReader) maskGameRecords(ctx context.Context, games []GameRecord) error {
	var names []string
	seen := map[string]bool{}
	for _, g := range games {
		if !seen[g.Name] {
			seen[g.Name] = true
			names = append(names, g.Name)
		}
		for _, o := range g.Opponents {
			if !seen[o.Name] {
				seen[o.Name] = true
				names = append(names, o.Name)
			}
		}
	}
	hidden, err := r.HiddenNames(ctx, names)
	if err != nil {
		return err
	}
	if len(hidden) == 0 {
		return nil
	}
	for i := range games {
		if hidden[games[i].Name] {
			games[i].Name = DeletedPlayer
		}
		for j := range games[i].Opponents {
			if hidden[games[i].Opponents[j].Name] {
				games[i].Opponents[j].Name = DeletedPlayer
			}
		}
	}
	return nil
}

// board returns a series' board in the engine's order, its first limit rows
// if limit is not nil, without players who have a deleted or banned CGE account.
func (r *RatingsReader) board(ctx context.Context, run int64, series string, limit *int) ([]TrophyRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT b.player_name, p.player_name IS NOT NULL, b.title, b.gold, b.silver, b.bronze,
		       b.details, b.games, b.wins, b.win_rate
		FROM tournament_boards b
		LEFT JOIN player_peak_titles p ON p.run_id = b.run_id AND p.player_name = b.player_name AND p.peak_title = 'WC'
		WHERE b.run_id = $1 AND b.series = $2 AND NOT `+hiddenName("b.player_name")+`
		ORDER BY b.position
		LIMIT $3`, run, series, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (TrophyRow, error) {
		var b TrophyRow
		return b, row.Scan(&b.Name, &b.WorldChampion, &b.Title, &b.Gold, &b.Silver, &b.Bronze,
			&b.Details, &b.Games, &b.Wins, &b.WinRate)
	})
}

// SeriesCount is a series' game and edition counts, as its page states them.
type SeriesCount struct {
	Series         string
	Games, Seasons int
}

// Legend is one player in the Hall of Fame's all-time major circuits table.
type Legend struct {
	Name                                                                  string
	WorldChampion                                                         bool
	WorldTitles, InternationalTitles, IntermezzoTitles, RoyalLeagueTitles int
	Gold, Silver, Bronze                                                  int
}

// HallOfFame is what the Hall of Fame overview shows.
type HallOfFame struct {
	Series              []SeriesCount
	Legends             []Legend
	GrandSlams, Ladders []TrophyRow
}

// hubBoardRows is how many rows of the cups and ladders boards the overview lists, as upstream's.
const hubBoardRows = 10

// HallOfFame returns a published run's Hall of Fame overview: every series'
// counts, the all-time major circuits table in the engine's order, and the
// first ten rows of the Grand Slams and Ladders boards, all without players
// who have a deleted or banned CGE account.
func (r *RatingsReader) HallOfFame(ctx context.Context, run int64) (HallOfFame, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return HallOfFame{}, err
	}
	fail := func(what string, err error) (HallOfFame, error) {
		return HallOfFame{}, fmt.Errorf("reading run %d's Hall of Fame %s: %w", run, what, err)
	}
	rows, err := r.pool.Query(ctx, "SELECT series, games, editions FROM tournament_series WHERE run_id = $1 ORDER BY series", run)
	if err != nil {
		return fail("counts", err)
	}
	got := HallOfFame{}
	got.Series, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (SeriesCount, error) {
		var s SeriesCount
		return s, row.Scan(&s.Series, &s.Games, &s.Seasons)
	})
	if err != nil {
		return fail("counts", err)
	}
	rows, err = r.pool.Query(ctx, `
		SELECT l.player_name, p.player_name IS NOT NULL, l.world_titles, l.international_titles,
		       l.intermezzo_titles, l.royal_league_titles, l.gold, l.silver, l.bronze
		FROM tournament_legends l
		LEFT JOIN player_peak_titles p ON p.run_id = l.run_id AND p.player_name = l.player_name AND p.peak_title = 'WC'
		WHERE l.run_id = $1 AND NOT `+hiddenName("l.player_name")+`
		ORDER BY l.position`, run)
	if err != nil {
		return fail("legends", err)
	}
	got.Legends, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Legend, error) {
		var l Legend
		return l, row.Scan(&l.Name, &l.WorldChampion, &l.WorldTitles, &l.InternationalTitles,
			&l.IntermezzoTitles, &l.RoyalLeagueTitles, &l.Gold, &l.Silver, &l.Bronze)
	})
	if err != nil {
		return fail("legends", err)
	}
	shown := hubBoardRows
	if got.GrandSlams, err = r.board(ctx, run, "grand_slams", &shown); err != nil {
		return fail("grand slams", err)
	}
	if got.Ladders, err = r.board(ctx, run, "ladders", &shown); err != nil {
		return fail("ladders", err)
	}
	return got, nil
}

// AchievementSummary is a player's titles and medals across the four major
// circuits, and the accolades their page features.
type AchievementSummary struct {
	WorldTitles         int
	InternationalTitles int
	IntermezzoTitles    int
	RoyalLeagueTitles   int
	Gold                int
	Silver              int
	Bronze              int
	Accolades           []string
}

// TournamentRecord is one row of a player's tournament records: a season, a
// campaign or a career total, as the engine wrote it for the page.
type TournamentRecord struct {
	Tournament string
	Season     string
	Division   string
	Placement  string
	Points     string
	Medal      *string
	Details    string
	FinishDate string
	Career     bool
}

// PlayerAchievements returns the stored name, a player's achievement summary
// (nil without a medal) and their records in a published run. Records match
// the name ignoring case, as upstream's page does; latest finish first, by
// byte order, so a career total's label sorts after every date.
func (r *RatingsReader) PlayerAchievements(ctx context.Context, run int64, asked string) (string, *AchievementSummary, []TournamentRecord, error) {
	name, err := r.resolve(ctx, run, asked)
	if err != nil {
		return "", nil, nil, err
	}
	fail := func(what string, err error) (string, *AchievementSummary, []TournamentRecord, error) {
		return "", nil, nil, fmt.Errorf("reading run %d's %s: %w", run, what, err)
	}
	var s AchievementSummary
	err = r.pool.QueryRow(ctx, `
		SELECT world_titles, international_titles, intermezzo_titles, royal_league_titles, gold, silver, bronze, accolades
		FROM tournament_achievements WHERE run_id = $1 AND lower(player_name) = lower($2)
		ORDER BY player_name = $2 DESC LIMIT 1`, run, name).
		Scan(&s.WorldTitles, &s.InternationalTitles, &s.IntermezzoTitles, &s.RoyalLeagueTitles, &s.Gold, &s.Silver, &s.Bronze, &s.Accolades)
	summary := &s
	if errors.Is(err, pgx.ErrNoRows) {
		summary = nil
	} else if err != nil {
		return fail("achievements", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT tournament, season, division, placement, points, medal, details, finish_date, career
		FROM tournament_records WHERE run_id = $1 AND lower(player_name) = lower($2)
		ORDER BY finish_date COLLATE "C" DESC, position DESC`, run, name)
	if err != nil {
		return fail("tournament records", err)
	}
	recs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (TournamentRecord, error) {
		var t TournamentRecord
		return t, row.Scan(&t.Tournament, &t.Season, &t.Division, &t.Placement, &t.Points, &t.Medal, &t.Details, &t.FinishDate, &t.Career)
	})
	if err != nil {
		return fail("tournament records", err)
	}
	return name, summary, recs, nil
}

// CommunityRow is one player in an edition of the community leaderboard, with
// their country and the run's title when the name is a player here.
type CommunityRow struct {
	Rank    int
	Name    string
	Country *string
	Title   *string
	Points  float64
	Events  int
	Scores  map[string]float64
}

// CommunityLeaderboard returns an edition of the community leaderboard in a
// published run, best rank first. A name is matched to a player ignoring
// case, the exact spelling first. Equal ranks keep the sheet's order.
func (r *RatingsReader) CommunityLeaderboard(ctx context.Context, run int64, edition string) ([]CommunityRow, error) {
	if err := r.requirePublished(ctx, run); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT s.rank, s.player_name, m.country_code, t.title, s.points, s.events,
		       coalesce((SELECT jsonb_object_agg(c.event, c.score) FROM community_scores c
		                 WHERE c.edition = s.edition AND c.player_name = s.player_name), '{}')
		FROM community_standings s
		LEFT JOIN LATERAL (
		    SELECT player_id, name FROM players WHERE lower(name) = lower(s.player_name)
		    ORDER BY name = s.player_name DESC, name LIMIT 1
		) p ON true
		LEFT JOIN members m ON m.player_id = p.player_id
		LEFT JOIN player_titles t ON t.run_id = $1 AND t.player_name = p.name
		WHERE s.edition = $2
		  AND `+notHiddenPlayer+`
		ORDER BY s.rank, s.points DESC, s.sheet_row`, run, edition)
	if err != nil {
		return nil, fmt.Errorf("reading the community leaderboard %s: %w", edition, err)
	}
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CommunityRow, error) {
		var c CommunityRow
		return c, row.Scan(&c.Rank, &c.Name, &c.Country, &c.Title, &c.Points, &c.Events, &c.Scores)
	})
	if err != nil {
		return nil, fmt.Errorf("reading the community leaderboard %s: %w", edition, err)
	}
	return got, nil
}

// scanGameRecord reads a game (tournament, date, then its seats' names, cultures and places) as its winner's
// record: the best place, a tie going to the earlier seat as upstream's sort, the others its opponents.
func scanGameRecord(row pgx.CollectableRow) (GameRecord, error) {
	var g GameRecord
	var names []string
	var cultures []float64
	var places []int16
	err := row.Scan(&g.Tournament, &g.Date, &names, &cultures, &places)
	var seats []Seat
	for _, p := range byPlace(names, cultures, places, nil) {
		seats = append(seats, Seat{Name: p.Name, Score: p.Score})
	}
	if len(seats) > 0 {
		g.Name, g.Score, g.Opponents = seats[0].Name, seats[0].Score, seats[1:]
	}
	return g, err
}
