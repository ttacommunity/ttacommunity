//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// classified is a game as the engine classifies it.
type classified struct {
	season, nameSeason int
	division, source   *string
	inRecords          bool
}

func classifyGame(t *testing.T, pool *pgxpool.Pool, run, match int64, c classified) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO tournament_games (run_id, match_id, series, season, name_season, division, group_label, division_source, in_records)
		VALUES ($1, $2, 'international', $3, $4, $5, $5, $6, $7)`, run, match, c.season, c.nameSeason, c.division, c.source, c.inRecords)
	if err != nil {
		t.Fatalf("classifying game %d: %v", match, err)
	}
}

func TestTournamentSeriesGivesTheSeriesCounts(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	if _, err := pool.Exec(ctx, "INSERT INTO tournament_series VALUES ($1, 'grand_slams', 12868, 5222)", run); err != nil {
		t.Fatalf("adding series counts: %v", err)
	}

	got, err := NewRatingsReader(asReader(t, pool)).TournamentSeries(ctx, run, "grand_slams")
	if err != nil {
		t.Fatalf("TournamentSeries: %v", err)
	}
	if got.Games != 12868 || got.Seasons != 5222 {
		t.Errorf("games %d, seasons %d; want the run's counts 12868 and 5222", got.Games, got.Seasons)
	}
	if got.Board == nil || got.SeasonRecords == nil || got.GameRecords == nil {
		t.Errorf("empty lists = %v %v %v, want empty slices", got.Board, got.SeasonRecords, got.GameRecords)
	}
}

func TestTournamentSeriesGivesTheBoardInTheEnginesOrder(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPeakTitle(t, pool, run, "cat", "WC", 2023)
	insert := `INSERT INTO tournament_boards (run_id, series, player_name, gold, silver, bronze, points, title,
		first_season, position, details, games, wins, win_rate) VALUES ($1, 'x', $2, $3, $4, $5, 0, $6, NULL, $7, $8, $9, $10, $11)`
	one, two := 1, 2
	three, rate := 30, 66.7
	for _, b := range []struct {
		name                 string
		gold, silver, bronze *int
		title, details       *string
		position             int
		games, wins          *int
		rate                 *float64
	}{
		{name: "cat", gold: &one, silver: &one, bronze: &two, title: ptr("GM"), position: 2},
		{name: "bob", gold: &two, silver: &one, bronze: &one, details: ptr("2025 Wimbledon TTA Champion"), position: 1},
		{name: "ann", games: &three, wins: &two, rate: &rate, position: 3},
	} {
		if _, err := pool.Exec(ctx, insert, run, b.name, b.gold, b.silver, b.bronze, b.title, b.position, b.details, b.games, b.wins, b.rate); err != nil {
			t.Fatalf("adding %s to the board: %v", b.name, err)
		}
	}

	got, err := NewRatingsReader(asReader(t, pool)).TournamentSeries(ctx, run, "x")
	if err != nil {
		t.Fatalf("TournamentSeries: %v", err)
	}
	var lines []string
	for _, b := range got.Board {
		lines = append(lines, fmt.Sprintf("%s wc=%t title=%s medals=%s details=%s ladder=%s",
			b.Name, b.WorldChampion, orDash(b.Title), medals(b), orDash(b.Details), ladder(b)))
	}
	want := []string{
		"bob wc=false title=- medals=2/1/1 details=2025 Wimbledon TTA Champion ladder=-",
		"cat wc=true title=GM medals=1/1/2 details=- ladder=-",
		"ann wc=false title=- medals=- details=- ladder=2/30 66.7",
	}
	if !slices.Equal(lines, want) {
		t.Errorf("board =\n%q\nwant\n%q", lines, want)
	}
}

func orDash(s *string) string {
	if s == nil {
		return "-"
	}
	return *s
}

func medals(b TrophyRow) string {
	if b.Gold == nil {
		return "-"
	}
	return fmt.Sprintf("%d/%d/%d", *b.Gold, *b.Silver, *b.Bronze)
}

func ladder(b TrophyRow) string {
	if b.Games == nil {
		return "-"
	}
	return fmt.Sprintf("%d/%d %.1f", *b.Wins, *b.Games, *b.WinRate)
}

func TestTournamentSeriesGivesTheTenBestSeasons(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPeakTitle(t, pool, run, "p05", "WC", 2024)
	// Eleven seasons: p00 has most points; p01 and p02 tie on points and wins, p02 seen first.
	for i := range 11 {
		points, wins, first := float64(30-i), 3, int64(100+i)
		switch i {
		case 1:
			first = 200
		case 2:
			points = 29
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO tournament_season_records VALUES ($1, 'international', 30, 'Grandmaster', $2, $3, 7, $4, '2025-07-31', $5)`,
			run, fmt.Sprintf("p%02d", i), points, []int{wins, 1, 1, 0}, first)
		if err != nil {
			t.Fatalf("adding season record %d: %v", i, err)
		}
	}

	got, err := NewRatingsReader(asReader(t, pool)).TournamentSeries(ctx, run, "international")
	if err != nil {
		t.Fatalf("TournamentSeries: %v", err)
	}
	var names []string
	for _, s := range got.SeasonRecords {
		names = append(names, s.Name)
	}
	if want := []string{"p00", "p02", "p01", "p03", "p04", "p05", "p06", "p07", "p08", "p09"}; !slices.Equal(names, want) {
		t.Errorf("season records = %v, want %v", names, want)
	}
	s := got.SeasonRecords[5]
	want := SeasonRecord{Name: "p05", WorldChampion: true, Points: 25, Games: 7, Placings: []int{3, 1, 1, 0},
		Season: 30, Division: "Grandmaster", LastPlayed: time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC)}
	if s.Name != want.Name || s.WorldChampion != want.WorldChampion || s.Points != want.Points || s.Games != want.Games ||
		!slices.Equal(s.Placings, want.Placings) || s.Season != want.Season || s.Division != want.Division || !s.LastPlayed.Equal(want.LastPlayed) {
		t.Errorf("season record 6 = %+v, want %+v", s, want)
	}
}

func TestTournamentSeriesGivesTheBestGamesMarkedForRecords(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	gm := ptr("Grandmaster")
	record := classified{season: 30, nameSeason: 30, division: gm, source: ptr("name"), inRecords: true}
	named := func(score float64, date string, seats ...Seat) int64 {
		id := addGame(t, pool, 0, date, "International S30 - Grandmaster game 1", append([]Seat{{"win", score}}, seats...)...)
		classifyGame(t, pool, run, id, record)
		return id
	}
	for i := range 11 {
		named(float64(200+i), fmt.Sprintf("2025-07-%02d", i+1), Seat{"x", 10}, Seat{"y", 20})
	}
	tied := named(210, "2025-08-01", Seat{"tie", 210}, Seat{"z", 5})
	recorded := addGame(t, pool, 0, "2020-01-01", "International S20", Seat{"old", 999}, Seat{"x", 1})
	classifyGame(t, pool, run, recorded, classified{season: 20, nameSeason: 20, division: gm, source: ptr("recorded")})
	silver := addGame(t, pool, 0, "2025-07-01", "International S30 - Silver 1 game 1", Seat{"low", 998}, Seat{"x", 1})
	classifyGame(t, pool, run, silver, classified{season: 30, nameSeason: 30, division: ptr("Silver"), source: ptr("name")})
	// A named top-division game the engine leaves out of the records, as upstream's "Grandmaster 1".
	unmarked := addGame(t, pool, 0, "2025-07-02", "International S30 - Grandmaster 1 game 1", Seat{"skip", 997}, Seat{"x", 1})
	classifyGame(t, pool, run, unmarked, classified{season: 30, nameSeason: 30, division: gm, source: ptr("name")})

	got, err := NewRatingsReader(asReader(t, pool)).TournamentSeries(ctx, run, "international")
	if err != nil {
		t.Fatalf("TournamentSeries: %v", err)
	}
	var scores []float64
	for _, g := range got.GameRecords {
		scores = append(scores, g.Score)
	}
	if want := []float64{210, 210, 209, 208, 207, 206, 205, 204, 203, 202}; !slices.Equal(scores, want) {
		t.Fatalf("game record scores = %v, want %v (named top-division games only)", scores, want)
	}
	first, second := got.GameRecords[0], got.GameRecords[1]
	if first.Name != "win" || first.Tournament != "International S30 - Grandmaster game 1" ||
		!first.Date.Equal(time.Date(2025, 7, 11, 0, 0, 0, 0, time.UTC)) ||
		!slices.Equal(first.Opponents, []Seat{{"y", 20}, {"x", 10}}) {
		t.Errorf("first game record = %+v", first)
	}
	// In the tied game the earlier seat wins, as upstream's stable sort does.
	if second.Name != "win" || !slices.Equal(second.Opponents, []Seat{{"tie", 210}, {"z", 5}}) {
		t.Errorf("tied game record = %+v (match %d)", second, tied)
	}
}

func TestHallOfFameGivesCountsLegendsAndTheTopOfTwoBoards(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	for _, q := range []string{
		"INSERT INTO tournament_series VALUES ($1, 'worlds', 31121, 5), ($1, 'international', 31674, 34)",
		"INSERT INTO tournament_legends VALUES ($1, 2, 'ann', 0, 2, 0, 0, 2, 1, 0), ($1, 1, 'wc', 1, 1, 0, 0, 2, 0, 1)",
	} {
		if _, err := pool.Exec(ctx, q, run); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	addPeakTitle(t, pool, run, "wc", "WC", 2025)
	for i := range 12 {
		_, err := pool.Exec(ctx, `INSERT INTO tournament_boards (run_id, series, player_name, gold, silver, bronze, points, position, details)
			VALUES ($1, 'grand_slams', $2, 1, 0, 0, 0, $3, 'cup')`, run, fmt.Sprintf("g%02d", i), i+1)
		if err != nil {
			t.Fatalf("adding a cup row: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tournament_boards (run_id, series, player_name, points, position, games, wins, win_rate)
		VALUES ($1, 'ladders', 'lad', 0, 1, 91, 54, 59.3)`, run); err != nil {
		t.Fatalf("adding a ladder row: %v", err)
	}

	got, err := NewRatingsReader(asReader(t, pool)).HallOfFame(ctx, run)
	if err != nil {
		t.Fatalf("HallOfFame: %v", err)
	}
	if want := []SeriesCount{{Series: "international", Games: 31674, Seasons: 34}, {Series: "worlds", Games: 31121, Seasons: 5}}; !slices.Equal(got.Series, want) {
		t.Errorf("series = %v, want %v", got.Series, want)
	}
	if want := []Legend{
		{Name: "wc", WorldChampion: true, WorldTitles: 1, InternationalTitles: 1, Gold: 2, Bronze: 1},
		{Name: "ann", InternationalTitles: 2, Gold: 2, Silver: 1},
	}; !slices.Equal(got.Legends, want) {
		t.Errorf("legends = %+v, want %+v", got.Legends, want)
	}
	if len(got.GrandSlams) != 10 || got.GrandSlams[0].Name != "g00" || got.GrandSlams[9].Name != "g09" {
		t.Errorf("grand slams = %d rows from %v, want the first 10 by position", len(got.GrandSlams), got.GrandSlams)
	}
	if len(got.Ladders) != 1 || *got.Ladders[0].Wins != 54 {
		t.Errorf("ladders = %+v, want the one ladder row", got.Ladders)
	}
}

func addRecord(t *testing.T, pool *pgxpool.Pool, run int64, player string, position int, finish string, career bool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO tournament_records VALUES ($1, $2, $3, 'International Championship', 'Season 30', 'Grandmaster',
		  '1 / 7', '20 pts', CASE WHEN $3 = 1 THEN 'gold' END, $4, $5, $6)`,
		run, player, position, fmt.Sprintf("%s #%d", player, position), finish, career)
	if err != nil {
		t.Fatalf("adding record %d: %v", position, err)
	}
}

func recordIDs(recs []TournamentRecord) []string {
	var got []string
	for _, r := range recs {
		got = append(got, r.Details)
	}
	return got
}

func TestPlayerAchievementsGiveTheSummaryAndRecordsLatestFirst(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "Ann", "Bob")
	if _, err := pool.Exec(ctx, `INSERT INTO tournament_achievements VALUES ($1, 'Ann', 1, 2, 0, 0, 3, 1, 0,
		ARRAY['2024 World Champion', '2x International Champion'])`, run); err != nil {
		t.Fatal(err)
	}
	addRecord(t, pool, run, "Ann", 1, "2024-12-15", false)
	addRecord(t, pool, run, "Ann", 2, "Career / All-Time", true)
	addRecord(t, pool, run, "Ann", 3, "2025-08-14", false)
	addRecord(t, pool, run, "Ann", 4, "2025-08-14", false)
	addRecord(t, pool, run, "Bob", 5, "2025-08-14", false)

	name, summary, recs, err := r.PlayerAchievements(ctx, run, "ann")
	if err != nil {
		t.Fatalf("PlayerAchievements: %v", err)
	}
	want := AchievementSummary{WorldTitles: 1, InternationalTitles: 2, Gold: 3, Silver: 1,
		Accolades: []string{"2024 World Champion", "2x International Champion"}}
	if name != "Ann" || summary == nil || fmt.Sprint(*summary) != fmt.Sprint(want) {
		t.Errorf("PlayerAchievements = %q, %+v; want Ann, %+v", name, summary, want)
	}
	// A career total's label sorts after every date, as on the original site.
	if got, want := recordIDs(recs), []string{"Ann #2", "Ann #4", "Ann #3", "Ann #1"}; !slices.Equal(got, want) {
		t.Errorf("records = %q, want %q", got, want)
	}
	first := recs[3]
	if first.Medal == nil || *first.Medal != "gold" || first.Tournament != "International Championship" ||
		first.Season != "Season 30" || first.Placement != "1 / 7" || first.Points != "20 pts" ||
		first.FinishDate != "2024-12-15" || first.Career || !recs[0].Career || recs[1].Medal != nil {
		t.Errorf("records = %+v, want each column as written", recs)
	}
}

func TestPlayerAchievementsMatchRecordsIgnoringCase(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "RowandeSable", "RowanDeSable")
	addRecord(t, pool, run, "RowandeSable", 1, "2020-07-26", false)
	addRecord(t, pool, run, "RowanDeSable", 2, "2023-09-25", false)

	name, summary, recs, err := r.PlayerAchievements(ctx, run, "RowandeSable")
	if err != nil {
		t.Fatalf("PlayerAchievements: %v", err)
	}
	if got, want := recordIDs(recs), []string{"RowanDeSable #2", "RowandeSable #1"}; name != "RowandeSable" || summary != nil || !slices.Equal(got, want) {
		t.Errorf("PlayerAchievements = %q, %+v, %q; want RowandeSable, no summary, %q", name, summary, got, want)
	}
}

func TestPlayerAchievementsRefuseAnUnknownPlayer(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "Ann")

	if _, _, _, err := r.PlayerAchievements(ctx, run, "Nobody"); !errors.Is(err, ErrPlayerNotFound) {
		t.Errorf("PlayerAchievements of an unknown player: %v, want ErrPlayerNotFound", err)
	}
	_, summary, recs, err := r.PlayerAchievements(ctx, run, "Ann")
	if err != nil || summary != nil || len(recs) != 0 {
		t.Errorf("PlayerAchievements of a player with none = %+v, %v, %v; want nothing, no error", summary, recs, err)
	}
}

func TestCommunityLeaderboardGivesAnEditionWithPlayersCountriesAndTitles(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "RowandeSable", "RowanDeSable")
	setCountries(t, pool, map[string]string{"ann": "PL", "RowandeSable": "FR"})
	for _, q := range []string{
		"INSERT INTO player_titles (run_id, player_name, title, title_count, badge_reason) VALUES ($1, 'ann', 'SGM', 1, 'r')",
		`INSERT INTO community_standings VALUES ('2024', 2, 2, 'Marten Fisher', 6687, 13, 's'), ('2024', 1, 1, 'ANN', 7657, 11, 's'),
		  ('2024', 4, 3, 'RowandeSable', 10, 1, 's'), ('2024', 3, 3, 'zed', 10, 1, 's'), ('2025', 1, 1, 'ann', 6600, 17, 's')`,
		"INSERT INTO community_scores VALUES ('2024', 'ANN', 'wrld', 1600, 's'), ('2024', 'ANN', 'ic_24', 1000, 's'), ('2025', 'ann', 'wrld', 5, 's')",
	} {
		args := []any{}
		if strings.Contains(q, "$1") {
			args = append(args, run)
		}
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	got, err := r.CommunityLeaderboard(ctx, run, "2024")
	if err != nil {
		t.Fatalf("CommunityLeaderboard: %v", err)
	}
	var lines []string
	for _, row := range got {
		lines = append(lines, fmt.Sprintf("%d %s %s %s %g %d %v", row.Rank, row.Name, orDash(row.Country), orDash(row.Title), row.Points, row.Events, row.Scores))
	}
	want := []string{
		"1 ANN PL SGM 7657 11 map[ic_24:1000 wrld:1600]",
		"2 Marten Fisher - - 6687 13 map[]",
		"3 zed - - 10 1 map[]", // equal ranks and points in the sheet's order
		"3 RowandeSable FR - 10 1 map[]",
	}
	if !slices.Equal(lines, want) {
		t.Errorf("CommunityLeaderboard(2024) =\n%s\nwant\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}

func TestCommunityLeaderboardExcludesDeletedAndBannedPlayers(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob", "dan")
	for _, q := range []string{
		"INSERT INTO player_accounts (cge_user, player_id, name, deleted) SELECT 1, player_id, 'ann', true FROM players WHERE name = 'ann'",
		"INSERT INTO player_accounts (cge_user, player_id, name, banned) SELECT 2, player_id, 'bob', true FROM players WHERE name = 'bob'",
		"INSERT INTO player_accounts (cge_user, player_id, name) SELECT 3, player_id, 'dan' FROM players WHERE name = 'dan'",
		"INSERT INTO community_standings VALUES ('2024', 1, 1, 'ann', 7000, 10, 's'), ('2024', 2, 2, 'bob', 6000, 10, 's'), ('2024', 3, 3, 'dan', 5000, 10, 's')",
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("setting up community standings or accounts: %v", err)
		}
	}

	got, err := r.CommunityLeaderboard(ctx, run, "2024")
	if err != nil {
		t.Fatalf("CommunityLeaderboard: %v", err)
	}
	if len(got) != 1 || got[0].Name != "dan" {
		t.Errorf("CommunityLeaderboard = %+v, want only dan (ann and bob excluded)", got)
	}
}
