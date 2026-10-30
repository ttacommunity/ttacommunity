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

	"ttacommunity/backend/internal/tournamentcsv"
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
	insert := `INSERT INTO tournament_boards (run_id, series, player_id, gold, silver, bronze, points, title,
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
		if _, err := pool.Exec(ctx, insert, run, playerID(t, pool, b.name), b.gold, b.silver, b.bronze, b.title, b.position, b.details, b.games, b.wins, b.rate); err != nil {
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
			INSERT INTO tournament_season_records (run_id, series, season, division, player_id, points, games, placings, last_date, first_match_id)
			VALUES ($1, 'international', 30, 'Grandmaster', $2, $3, 7, $4, '2025-07-31', $5)`,
			run, playerID(t, pool, fmt.Sprintf("p%02d", i)), points, []int{wins, 1, 1, 0}, first)
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
	addPlayers(t, pool, "ann", "wc")
	for _, q := range []string{
		"INSERT INTO tournament_series VALUES ($1, 'worlds', 31121, 5), ($1, 'international', 31674, 34)",
		`INSERT INTO tournament_legends (run_id, position, player_id, world_titles, international_titles, intermezzo_titles,
		     royal_league_titles, gold, silver, bronze)
		 SELECT $1, l.position, p.player_id, l.w, l.i, 0, 0, l.g, l.s, l.b
		 FROM (VALUES (2, 'ann', 0, 2, 2, 1, 0), (1, 'wc', 1, 1, 2, 0, 1)) AS l(position, name, w, i, g, s, b)
		 JOIN players p USING (name)`,
	} {
		if _, err := pool.Exec(ctx, q, run); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	addPeakTitle(t, pool, run, "wc", "WC", 2025)
	for i := range 12 {
		_, err := pool.Exec(ctx, `INSERT INTO tournament_boards (run_id, series, player_id, gold, silver, bronze, points, position, details)
			VALUES ($1, 'grand_slams', $2, 1, 0, 0, 0, $3, 'cup')`, run, playerID(t, pool, fmt.Sprintf("g%02d", i)), i+1)
		if err != nil {
			t.Fatalf("adding a cup row: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tournament_boards (run_id, series, player_id, points, position, games, wins, win_rate)
		VALUES ($1, 'ladders', $2, 0, 1, 91, 54, 59.3)`, run, playerID(t, pool, "lad")); err != nil {
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
		INSERT INTO tournament_records (run_id, player_id, position, tournament, season, division, placement, points, medal,
		  details, finish_date, career)
		VALUES ($1, $2, $3, 'International Championship', 'Season 30', 'Grandmaster',
		  '1 / 7', '20 pts', CASE WHEN $3 = 1 THEN 'gold' END, $4, $5, $6)`,
		run, playerID(t, pool, player), position, fmt.Sprintf("%s #%d", player, position), finish, career)
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
	if _, err := pool.Exec(ctx, `INSERT INTO tournament_achievements (run_id, player_id, world_titles, international_titles,
		intermezzo_titles, royal_league_titles, gold, silver, bronze, accolades)
		VALUES ($1, $2, 1, 2, 0, 0, 3, 1, 0, ARRAY['2024 World Champion', '2x International Champion'])`, run, playerID(t, pool, "Ann")); err != nil {
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

func TestPlayerAchievementsAreThePlayersOwnUnderTheirNameNow(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "RowandeSable", "RowanDeSable")
	addRecord(t, pool, run, "RowandeSable", 1, "2020-07-26", false)
	addRecord(t, pool, run, "RowanDeSable", 2, "2023-09-25", false)
	if _, err := pool.Exec(ctx, "UPDATE players SET name = 'Rowan' WHERE name = 'RowandeSable'"); err != nil {
		t.Fatalf("renaming: %v", err)
	}

	name, summary, recs, err := r.PlayerAchievements(ctx, run, "rowan")
	if err != nil {
		t.Fatalf("PlayerAchievements: %v", err)
	}
	if got, want := recordIDs(recs), []string{"RowandeSable #1"}; name != "Rowan" || summary != nil || !slices.Equal(got, want) {
		t.Errorf("PlayerAchievements = %q, %+v, %q; want Rowan, no summary, %q", name, summary, got, want)
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
	addPlayers(t, pool, "ann", "RowandeSable", "Marten Fisher", "zed")
	setCountries(t, pool, map[string]string{"ann": "PL", "RowandeSable": "FR"})
	addTitle(t, pool, run, "ann", "SGM", 1, "r")
	if _, err := ReplaceCommunityStandings(ctx, pool, []tournamentcsv.Standing{
		{Edition: "2024", SheetRow: 2, Rank: 2, Player: "Marten Fisher", Points: 6687, Events: 13, Source: "s"},
		{Edition: "2024", SheetRow: 1, Rank: 1, Player: "ANN", Points: 7657, Events: 11, Source: "s"},
		{Edition: "2024", SheetRow: 4, Rank: 3, Player: "RowandeSable", Points: 10, Events: 1, Source: "s"},
		{Edition: "2024", SheetRow: 3, Rank: 3, Player: "zed", Points: 10, Events: 1, Source: "s"},
		{Edition: "2025", SheetRow: 1, Rank: 1, Player: "ann", Points: 6600, Events: 17, Source: "s"},
	}); err != nil {
		t.Fatalf("ReplaceCommunityStandings: %v", err)
	}
	if _, err := ReplaceCommunityScores(ctx, pool, []tournamentcsv.Score{
		{Edition: "2024", Player: "ANN", Event: "wrld", Score: 1600, Source: "s"},
		{Edition: "2024", Player: "ANN", Event: "ic_24", Score: 1000, Source: "s"},
		{Edition: "2025", Player: "ann", Event: "wrld", Score: 5, Source: "s"},
	}); err != nil {
		t.Fatalf("ReplaceCommunityScores: %v", err)
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
		"1 ann PL SGM 7657 11 map[ic_24:1000 wrld:1600]",
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
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("setting up accounts: %v", err)
		}
	}
	var standings []tournamentcsv.Standing
	for i, name := range []string{"ann", "bob", "dan"} {
		standings = append(standings, tournamentcsv.Standing{Edition: "2024", SheetRow: i + 1, Rank: i + 1, Player: name, Points: float64(7000 - 1000*i), Events: 10, Source: "s"})
	}
	if _, err := ReplaceCommunityStandings(ctx, pool, standings); err != nil {
		t.Fatalf("ReplaceCommunityStandings: %v", err)
	}

	got, err := r.CommunityLeaderboard(ctx, run, "2024")
	if err != nil {
		t.Fatalf("CommunityLeaderboard: %v", err)
	}
	if len(got) != 1 || got[0].Name != "dan" {
		t.Errorf("CommunityLeaderboard = %+v, want only dan (ann and bob excluded)", got)
	}
}

func TestHallOfFameLeavesOutHiddenPlayers(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob", "cat")
	hideAccount(t, pool, "ann", "deleted")
	hideAccount(t, pool, "bob", "banned")
	if _, err := pool.Exec(ctx, `INSERT INTO tournament_legends (run_id, position, player_id, world_titles, international_titles,
		    intermezzo_titles, royal_league_titles, gold, silver, bronze)
		SELECT $1, l.position, p.player_id, l.w, l.i, 0, 0, 1, 0, 0
		FROM (VALUES (1, 'ann', 1, 0), (2, 'bob', 1, 0), (3, 'cat', 0, 1)) AS l(position, name, w, i) JOIN players p USING (name)`, run); err != nil {
		t.Fatalf("adding legends: %v", err)
	}
	board := []string{"ann", "bob", "g01", "g02", "g03", "g04", "g05", "g06", "g07", "g08", "g09", "g10"}
	for i, name := range board {
		_, err := pool.Exec(ctx, `INSERT INTO tournament_boards (run_id, series, player_id, points, position)
			VALUES ($1, 'grand_slams', $2, 0, $3), ($1, 'ladders', $2, 0, $3)`, run, playerID(t, pool, name), i+1)
		if err != nil {
			t.Fatalf("adding board rows: %v", err)
		}
	}

	got, err := NewRatingsReader(asReader(t, pool)).HallOfFame(ctx, run)
	if err != nil {
		t.Fatalf("HallOfFame: %v", err)
	}
	if len(got.Legends) != 1 || got.Legends[0].Name != "cat" {
		t.Errorf("legends = %+v, want cat only: ann's account is deleted, bob's banned", got.Legends)
	}
	for what, rows := range map[string][]TrophyRow{"grand slams": got.GrandSlams, "ladders": got.Ladders} {
		if len(rows) != 10 || rows[0].Name != "g01" || rows[9].Name != "g10" {
			t.Errorf("%s = %+v, want g01 to g10: ten rows without ann and bob", what, rows)
		}
	}
}

func TestTournamentSeriesHidesHiddenPlayers(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "cat")
	hideAccount(t, pool, "ann", "deleted")
	addPeakTitle(t, pool, run, "ann", "WC", 2024)
	for i, name := range []string{"ann", "cat"} {
		if _, err := pool.Exec(ctx, `INSERT INTO tournament_boards (run_id, series, player_id, points, position)
			VALUES ($1, 'international', $2, 0, $3)`, run, playerID(t, pool, name), i+1); err != nil {
			t.Fatalf("adding %s's board row: %v", name, err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO tournament_season_records (run_id, series, season, division, player_id, points,
			games, placings, last_date, first_match_id)
			VALUES ($1, 'international', 30, 'Grandmaster', $2, $3, 7, '{3,1,1,0}', '2025-07-31', $4)`,
			run, playerID(t, pool, name), float64(30-i), i+1); err != nil {
			t.Fatalf("adding %s's season: %v", name, err)
		}
	}

	got, err := NewRatingsReader(asReader(t, pool)).TournamentSeries(ctx, run, "international")
	if err != nil {
		t.Fatalf("TournamentSeries: %v", err)
	}
	if len(got.Board) != 1 || got.Board[0].Name != "cat" {
		t.Errorf("board = %+v, want cat only: ann's account is deleted", got.Board)
	}
	var seasons []string
	for _, s := range got.SeasonRecords {
		seasons = append(seasons, fmt.Sprintf("%s %v %g", s.Name, s.WorldChampion, s.Points))
	}
	if want := []string{"Deleted player false 30", "cat false 29"}; !slices.Equal(seasons, want) {
		t.Errorf("season records = %q, want %q: ann's season kept, its name and crown masked", seasons, want)
	}
}

func TestTournamentSeriesListsTiedSeasonRecordsByName(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "zed", "amy")
	for _, name := range []string{"zed", "amy"} {
		if _, err := pool.Exec(ctx, `INSERT INTO tournament_season_records (run_id, series, season, division, player_id, points,
			games, placings, last_date, first_match_id)
			VALUES ($1, 'international', 30, 'Grandmaster', $2, 20, 7, '{3,1,1,0}', '2025-07-31', 1)`,
			run, playerID(t, pool, name)); err != nil {
			t.Fatalf("adding %s's season: %v", name, err)
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
	if want := []string{"amy", "zed"}; !slices.Equal(names, want) {
		t.Errorf("season records = %q, want %q: equal points, wins and first game, so by name", names, want)
	}
}

// circuitGroup makes slug a group named name, with a card, holding members.
func circuitGroup(t *testing.T, pool *pgxpool.Pool, slug, name string, members ...string) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO series (slug, name, kind, is_group) VALUES ($1, $2, 'group', true)`,
		`INSERT INTO tournament_cards (tournament, icon, tagline, summary, subtitle, cadence) VALUES ($1, '🎾', $2, 'All the cups.', 'Open format', 'Yearly')
		 ON CONFLICT (tournament) DO UPDATE SET icon = excluded.icon, tagline = excluded.tagline, summary = excluded.summary,
		     subtitle = excluded.subtitle, cadence = excluded.cadence`,
	} {
		if _, err := pool.Exec(ctx, q, slug, name); err != nil {
			t.Fatalf("making the group %s: %v", slug, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE series SET parent_id = (SELECT series_id FROM series WHERE slug = $1) WHERE slug = ANY($2)`,
		slug, members); err != nil {
		t.Fatalf("putting %v in %s: %v", members, slug, err)
	}
}

func TestTournamentSeriesGivesTheGroupsPageAsAdminWritesIt(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
		"Intermezzo S24 - Bronze 1 game 1":        day("2024-01-02"),
	})
	for _, q := range []string{
		`INSERT INTO series (slug, name) VALUES ('quiet_cup', 'Quiet Cup')`,
		`DELETE FROM ranking_series WHERE series_id = (SELECT series_id FROM series WHERE slug = 'intermezzo')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	circuitGroup(t, pool, "grand_slams", "Grand Slams & Cups", "wimbledon", "intermezzo", "quiet_cup")
	if _, err := pool.Exec(ctx, "UPDATE series SET format_summary = 'Knockout duels.' WHERE slug = 'wimbledon'"); err != nil {
		t.Fatalf("giving Wimbledon a format in brief: %v", err)
	}
	if err := NewAdminStore(pool).SetStories(ctx, "grand_slams", []Story{{Player: "ann", Title: "Champion", Text: "Won it."}}); err != nil {
		t.Fatalf("SetStories: %v", err)
	}
	withSpans(t, pool, run)

	got, err := NewRatingsReader(asReader(t, pool)).TournamentSeries(ctx, run, "grand_slams")
	if err != nil {
		t.Fatalf("TournamentSeries: %v", err)
	}
	if got.Name != "Grand Slams & Cups" || got.Card.Tagline != "Grand Slams & Cups" || got.Card.Cadence != "Yearly" {
		t.Errorf("name %q, card %+v; want the group's name and card", got.Name, got.Card)
	}
	if want := []Story{{Player: "ann", Title: "Champion", Text: "Won it."}}; !slices.Equal(got.Stories, want) {
		t.Errorf("stories = %+v, want %+v", got.Stories, want)
	}
	// Intermezzo is neither rated nor visible, and the quiet cup has no games: neither has a page to link to.
	if want := []SeriesMember{{Slug: "wimbledon", Name: "Wimbledon", Format: "Knockout duels."}}; !slices.Equal(got.Members, want) {
		t.Errorf("members = %+v, want %+v", got.Members, want)
	}
	if got.FirstGame == nil || !got.FirstGame.Equal(day("2024-07-01")) {
		t.Errorf("first game = %v, want Wimbledon's 2024-07-01, not the hidden Intermezzo's", got.FirstGame)
	}
}

func TestTournamentSeriesWithoutAGroupHasNoPageText(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	if _, err := pool.Exec(ctx, "DELETE FROM tournament_cards WHERE tournament = 'ladders'"); err != nil {
		t.Fatalf("removing the ladders' card: %v", err)
	}

	got, err := NewRatingsReader(asReader(t, pool)).TournamentSeries(ctx, run, "ladders")
	if err != nil {
		t.Fatalf("TournamentSeries: %v", err)
	}
	if got.Name != "" || got.Card != (Card{}) || got.FirstGame != nil || got.Members == nil || len(got.Members) != 0 || got.Stories == nil {
		t.Errorf("got name %q, card %+v, first game %v, members %v, stories %v; want none, as empty lists",
			got.Name, got.Card, got.FirstGame, got.Members, got.Stories)
	}
}

func TestHallOfFameGivesEachCircuitsCard(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
	circuitGroup(t, pool, "grand_slams", "Grand Slams & Cups", "wimbledon")
	circuitGroup(t, pool, "ladders", "Ladders")
	withSpans(t, pool, run)

	got, err := NewRatingsReader(asReader(t, pool)).HallOfFame(ctx, run)
	if err != nil {
		t.Fatalf("HallOfFame: %v", err)
	}
	var slugs, names []string
	for _, c := range got.Circuits {
		slugs, names = append(slugs, c.Slug), append(names, c.Name)
	}
	if !slices.Equal(slugs, []string{"grand_slams", "ladders"}) || !slices.Equal(names, []string{"Grand Slams & Cups", "Ladders"}) {
		t.Fatalf("circuits %q named %q, want grand_slams then ladders, by their groups' names", slugs, names)
	}
	if c := got.Circuits[0]; c.Card.Summary != "All the cups." || c.FirstGame == nil || !c.FirstGame.Equal(day("2024-07-01")) {
		t.Errorf("grand slams card %+v, first game %v; want its card and Wimbledon's first game", c.Card, c.FirstGame)
	}
	if got.Circuits[1].FirstGame != nil {
		t.Errorf("ladders' first game = %v, want none: it holds no tournament", got.Circuits[1].FirstGame)
	}
}
