//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/google/go-cmp/cmp"
)

// addGame stores one game and, if run is not zero, the run's 1v1 pairs for it.
func addGame(t *testing.T, pool *pgxpool.Pool, run int64, date, tournament string, seats ...Seat) int64 {
	t.Helper()
	ctx := context.Background()
	var batch int64
	if err := pool.QueryRow(ctx, "INSERT INTO import_batches (source) VALUES ('load') RETURNING batch_id").Scan(&batch); err != nil {
		t.Fatalf("adding a batch: %v", err)
	}
	var id int64
	err := pool.QueryRow(ctx, `
		INSERT INTO games (batch_id, source_file, source_line, name, date)
		VALUES ($1, 'f.csv', 1, $2, $3) RETURNING game_id`, batch, tournament, date).Scan(&id)
	if err != nil {
		t.Fatalf("adding a game on %s: %v", date, err)
	}
	var names []string
	var scores []float64
	for _, s := range seats {
		names, scores = append(names, s.Name), append(scores, s.Score)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO players (name) SELECT unnest($1::text[]) ON CONFLICT (name) DO NOTHING", names); err != nil {
		t.Fatalf("adding game %d's players: %v", id, err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO seats (game_id, seat, player_id, player_name, culture, recorded_score, place)
		SELECT $1, s.seat, p.player_id, s.name, greatest(s.score, 0), s.score, rank() OVER (ORDER BY s.score DESC)
		FROM unnest($2::text[], $3::float8[]) WITH ORDINALITY AS s(name, score, seat) JOIN players p USING (name)`,
		id, names, scores)
	if err != nil {
		t.Fatalf("adding game %d's seats: %v", id, err)
	}
	if run == 0 {
		return id
	}
	for i, a := range seats {
		for _, b := range seats[i+1:] {
			outcome := 0.5
			if a.Score > b.Score {
				outcome = 1
			} else if a.Score < b.Score {
				outcome = 0
			}
			_, err := pool.Exec(ctx, `
				INSERT INTO pairwise_matches (run_id, match_id, date, tournament, player_count, player_a, player_b,
					score_a, score_b, outcome_a, weight, glicko_eligible)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 1, true)`,
				run, id, date, tournament, len(seats), a.Name, b.Name, a.Score, b.Score, outcome)
			if err != nil {
				t.Fatalf("adding a pair for game %d: %v", id, err)
			}
		}
	}
	return id
}

func TestPlayerFindsANameExactlyOrByAUniqueCase(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "Ann", "Nimbusnim", "nimbusnim")

	tests := []struct {
		name, asked string
		run         int64
		want        string
		err         error
	}{
		{name: "exact name", asked: "Ann", run: run, want: "Ann"},
		{name: "another case, one match", asked: "aNN", run: run, want: "Ann"},
		{name: "exact name among two cases", asked: "nimbusnim", run: run, want: "nimbusnim"},
		{name: "another case, two matches", asked: "NIMBUSNIM", run: run, err: ErrPlayerNotFound},
		{name: "unknown name", asked: "zed", run: run, err: ErrPlayerNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.Player(ctx, tt.run, tt.asked)
			if !errors.Is(err, tt.err) || (tt.err == nil && got.Name != tt.want) {
				t.Errorf("Player(%q) = %q, %v; want %q, %v", tt.asked, got.Name, err, tt.want, tt.err)
			}
		})
	}
}

func TestPlayerHidesDeletedAndBannedAccounts(t *testing.T) {
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
			t.Fatalf("setting up player account: %v", err)
		}
	}

	for _, name := range []string{"ann", "bob"} {
		if _, err := r.Player(ctx, run, name); !errors.Is(err, ErrPlayerNotFound) {
			t.Errorf("Player(%q) = %v, want ErrPlayerNotFound", name, err)
		}
	}
	if got, err := r.Player(ctx, run, "dan"); err != nil || got.Name != "dan" {
		t.Errorf("Player(dan) = %q, %v; want dan", got.Name, err)
	}
}

func TestPlayerGivesRatingsGamesAndDetails(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	other := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob")
	if _, err := SetCountries(ctx, pool, map[string]string{"ann": "PL"}); err != nil {
		t.Fatalf("SetCountries: %v", err)
	}
	addTitle(t, pool, run, "ann", "GM", 2, "Played in Royal League Grandmaster")
	addPeakTitle(t, pool, run, "ann", "WC", 2024)
	for _, model := range PlayerPageModels {
		addRating(t, pool, run, model, 0, "ann", ptr(4), 1800)
	}
	addRating(t, pool, run, "glicko2_daneo", 3, "ann", nil, 1700)
	addRating(t, pool, run, "glicko2_std_retro", 0, "ann", ptr(1), 1900)
	addRating(t, pool, run, "glicko2_daneo", 0, "bob", ptr(1), 1900)
	addRating(t, pool, other, "glicko2_daneo", 0, "ann", ptr(9), 1000)

	addGame(t, pool, run, "2025-01-05", "Cup", Seat{"ann", 90}, Seat{"bob", 80})
	addGame(t, pool, run, "2025-03-07", "Cup", Seat{"ann", 70}, Seat{"bob", 60}, Seat{"cy", 50})
	addGame(t, pool, run, "2025-02-01", "Cup", Seat{"bob", 70}, Seat{"ann", 60}, Seat{"cy", 50})
	addGame(t, pool, run, "2025-06-30", "Cup", Seat{"bob", 70}, Seat{"cy", 60})
	addGame(t, pool, other, "2025-12-01", "Cup", Seat{"ann", 70}, Seat{"bob", 60})
	addGame(t, pool, 0, "2025-11-01", "Cup", Seat{"ann", 70}, Seat{"bob", 60})

	got, err := r.Player(ctx, run, "ann")
	if err != nil {
		t.Fatalf("Player: %v", err)
	}
	if got.Country == nil || *got.Country != "PL" {
		t.Errorf("country = %v, want PL", got.Country)
	}
	if got.Title == nil || got.Title.Code != "GM" || got.Title.Count != 2 {
		t.Errorf("title = %+v, want GM ×2", got.Title)
	}
	if got.Peak == nil || got.Peak.Code != "WC" || got.Peak.Year == nil || *got.Peak.Year != 2024 {
		t.Errorf("peak = %+v, want WC in 2024", got.Peak)
	}
	wantGames := map[int]int{0: 3, 2: 1, 3: 2}
	if fmt.Sprint(got.Games) != fmt.Sprint(wantGames) {
		t.Errorf("games = %v, want %v (this run's only)", got.Games, wantGames)
	}
	if got.LastPlayed == nil || !got.LastPlayed.Equal(day("2025-03-07")) {
		t.Errorf("last played = %v, want 2025-03-07", got.LastPlayed)
	}
	if got.NewestGame == nil || !got.NewestGame.Equal(day("2025-06-30")) {
		t.Errorf("newest game = %v, want 2025-06-30 (anyone's, this run's)", got.NewestGame)
	}
	if len(got.Ratings) != len(PlayerPageModels)+1 {
		t.Fatalf("%d ratings, want %d (page models only, this run and player only)", len(got.Ratings), len(PlayerPageModels)+1)
	}
	for _, p := range got.Ratings {
		if p.Model == "glicko2_std_retro" || p.CRating == 1000 || p.CRating == 1900 {
			t.Errorf("rating %+v is not ann's in this run's page models", p)
		}
	}
	unranked := slices.IndexFunc(got.Ratings, func(p PlayerRating) bool { return p.Format == 3 })
	if unranked < 0 || got.Ratings[unranked].Rank != nil || got.Ratings[unranked].Opponents != 9 {
		t.Errorf("3P rating = %+v, want unranked with 9 opponents", got.Ratings)
	}
}

func TestPlayerHistoryGivesThePageModelsOldestFirst(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann")
	addHistory(t, pool, run, "glicko2_daneo", 0, "ann", "2025-02-28", 1810)
	addHistory(t, pool, run, "glicko2_daneo", 0, "ann", "2025-01-31", 1800)
	addHistory(t, pool, run, "glicko2_daneo_softer", 0, "ann", "2025-01-31", 1700)
	addHistory(t, pool, run, "glicko2_daneo", 2, "ann", "2025-01-31", 1600)
	addHistory(t, pool, run, "glicko2_std_retro", 0, "ann", "2025-01-31", 1500)
	addHistory(t, pool, run, "glicko2_daneo", 0, "bob", "2025-01-31", 1400)

	name, got, err := r.PlayerHistory(ctx, run, "ANN", 0)
	if err != nil {
		t.Fatalf("PlayerHistory: %v", err)
	}
	want := []HistoryPoint{
		{Model: "glicko2_daneo", Date: day("2025-01-31"), Rating: 1890, CRating: 1800},
		{Model: "glicko2_daneo", Date: day("2025-02-28"), Rating: 1900, CRating: 1810},
		{Model: "glicko2_daneo_softer", Date: day("2025-01-31"), Rating: 1790, CRating: 1700},
	}
	if name != "ann" || fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("PlayerHistory(ANN, 0) = %q, %v; want ann, %v", name, got, want)
	}
	if _, _, err := r.PlayerHistory(ctx, run, "zed", 0); !errors.Is(err, ErrPlayerNotFound) {
		t.Errorf("PlayerHistory(zed): err = %v, want ErrPlayerNotFound", err)
	}
}

func TestPlayerMatchesPagesTheRunsGamesNewestFirst(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	other := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob", "cy")
	start := day("2024-01-01")
	for i := range 31 {
		addGame(t, pool, run, start.AddDate(0, 0, i).Format(time.DateOnly), fmt.Sprintf("Duel %d", i), Seat{"bob", 5}, Seat{"ann", 9})
	}
	three := addGame(t, pool, run, "2024-01-15", "Trio", Seat{"cy", 3}, Seat{"ann", 7}, Seat{"bob", 7})
	addGame(t, pool, run, "2025-01-01", "Without ann", Seat{"bob", 1}, Seat{"cy", 2})
	addGame(t, pool, other, "2025-01-01", "Another run", Seat{"ann", 1}, Seat{"cy", 2})

	name, first, err := r.PlayerMatches(ctx, run, "ann", 0, 1)
	if err != nil {
		t.Fatalf("PlayerMatches: %v", err)
	}
	if name != "ann" || first.Total != 32 || first.Page != 1 || first.Pages != 2 || len(first.Matches) != 30 {
		t.Fatalf("page 1 = %q, total %d, page %d of %d, %d games; want ann, 32, 1 of 2, 30",
			name, first.Total, first.Page, first.Pages, len(first.Matches))
	}
	if first.Matches[0].Tournament != "Duel 30" {
		t.Errorf("first game = %q, want the newest, Duel 30", first.Matches[0].Tournament)
	}
	for i := 1; i < len(first.Matches); i++ {
		if first.Matches[i].Date.After(first.Matches[i-1].Date) {
			t.Errorf("game %d (%v) is newer than game %d (%v)", i, first.Matches[i].Date, i-1, first.Matches[i-1].Date)
		}
	}
	trio := slices.IndexFunc(first.Matches, func(m PlayerMatch) bool { return m.ID == three })
	wantSeats := []Seat{{"cy", 3}, {"ann", 7}, {"bob", 7}}
	if trio < 0 || first.Matches[trio].PlayerCount != 3 || !slices.Equal(first.Matches[trio].Seats, wantSeats) {
		t.Errorf("the 3P game = %+v, want seats %v in seat order", first.Matches, wantSeats)
	}

	_, past, err := r.PlayerMatches(ctx, run, "ann", 0, 9)
	if err != nil || past.Page != 2 || len(past.Matches) != 2 {
		t.Errorf("page 9 = page %d, %d games, %v; want the last page, 2, 2 games", past.Page, len(past.Matches), err)
	}
	_, trios, err := r.PlayerMatches(ctx, run, "ann", 3, 1)
	if err != nil || trios.Total != 1 || trios.Pages != 1 || len(trios.Matches) != 1 || trios.Matches[0].ID != three {
		t.Errorf("3P games = %+v, %v; want only the trio", trios, err)
	}
	_, none, err := r.PlayerMatches(ctx, run, "ann", 4, 1)
	if err != nil || none.Total != 0 || none.Pages != 1 || none.Page != 1 || len(none.Matches) != 0 {
		t.Errorf("4P games = %+v, %v; want none, on page 1 of 1", none, err)
	}
}

func TestPlayerTakesGamesAsTheRunRatedThem(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob", "cy")
	addGame(t, pool, run, "2025-01-05", "Duel", Seat{"ann", 9}, Seat{"bob", 5})
	fixed := addGame(t, pool, run, "2025-01-10", "Trio", Seat{"ann", 9}, Seat{"bob", 5}, Seat{"cy", 1})
	if _, err := pool.Exec(ctx, "UPDATE games SET date = '2025-03-01' WHERE game_id = $1", fixed); err != nil {
		t.Fatalf("moving the game after the run: %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM seats WHERE game_id = $1 AND player_name = 'cy'", fixed); err != nil {
		t.Fatalf("removing a seat after the run: %v", err)
	}

	p, err := r.Player(ctx, run, "ann")
	if err != nil {
		t.Fatalf("Player: %v", err)
	}
	if want := map[int]int{0: 2, 2: 1, 3: 1}; fmt.Sprint(p.Games) != fmt.Sprint(want) {
		t.Errorf("games = %v, want %v (the run's formats)", p.Games, want)
	}
	if p.LastPlayed == nil || !p.LastPlayed.Equal(day("2025-01-10")) {
		t.Errorf("last played = %v, want the run's 2025-01-10", p.LastPlayed)
	}

	_, page, err := r.PlayerMatches(ctx, run, "ann", 3, 1)
	if err != nil {
		t.Fatalf("PlayerMatches: %v", err)
	}
	if page.Total != 1 || len(page.Matches) != 1 || page.Matches[0].ID != fixed {
		t.Fatalf("3P games = %+v, want the trio as the run rated it", page)
	}
	if m := page.Matches[0]; !m.Date.Equal(day("2025-01-10")) || m.PlayerCount != 3 {
		t.Errorf("trio = %v, %dP; want the run's 2025-01-10, 3P", m.Date, m.PlayerCount)
	}
}

func TestVersusTakesGamesAsTheRunRatedThem(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob", "cy")
	duel := addGame(t, pool, run, "2025-01-05", "Duel", Seat{"ann", 9}, Seat{"bob", 5})
	trio := addGame(t, pool, run, "2025-01-10", "Trio", Seat{"ann", 9}, Seat{"bob", 5}, Seat{"cy", 1})
	if _, err := pool.Exec(ctx, "UPDATE games SET date = '2025-03-01' WHERE game_id = $1", duel); err != nil {
		t.Fatalf("moving the game after the run: %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM seats WHERE game_id = $1 AND player_name = 'cy'", trio); err != nil {
		t.Fatalf("removing a seat after the run: %v", err)
	}

	_, _, games, err := r.Versus(ctx, run, "ann", "bob")
	if err != nil {
		t.Fatalf("Versus: %v", err)
	}
	var got []string
	for _, g := range games {
		got = append(got, fmt.Sprintf("%d %s %dP", g.ID, g.Date.Format(time.DateOnly), g.PlayerCount))
	}
	want := []string{fmt.Sprintf("%d 2025-01-10 3P", trio), fmt.Sprintf("%d 2025-01-05 2P", duel)}
	if !slices.Equal(got, want) {
		t.Errorf("shared games = %q, want %q (the run's dates and formats, newest first)", got, want)
	}
}

func TestHiddenNamesFindsDeletedAndBannedAccountsIgnoringCase(t *testing.T) {
	pool := migrated(t)
	addPlayers(t, pool, "ann", "bob", "cy")
	hideAccount(t, pool, "bob", "deleted")
	hideAccount(t, pool, "cy", "banned")

	got, err := NewRatingsReader(asReader(t, pool)).HiddenNames(context.Background(), []string{"ann", "BOB", "cy", "nobody"})
	if err != nil {
		t.Fatalf("HiddenNames: %v", err)
	}
	if want := map[string]bool{"BOB": true, "cy": true}; !maps.Equal(got, want) {
		t.Errorf("HiddenNames = %v, want %v", got, want)
	}
}

func TestRatingsOfGivesTheNamedPlayersInOneModelAndFormat(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	other := publishedRun(t, pool)
	addRating(t, pool, run, "glicko2_std", 2, "ann", ptr(1), 1800)
	addRating(t, pool, run, "glicko2_std", 2, "bob", ptr(2), 1700)
	addRating(t, pool, run, "glicko2_std", 2, "cy", ptr(3), 1600)
	addRating(t, pool, run, "glicko2_std", 0, "ann", ptr(1), 1000)
	addRating(t, pool, run, "glicko2_mp", 2, "ann", ptr(1), 1100)
	addRating(t, pool, other, "glicko2_std", 2, "bob", ptr(1), 1200)

	got, err := r.RatingsOf(ctx, run, "glicko2_std", 2, []string{"ann", "bob", "nobody"})
	if err != nil {
		t.Fatalf("RatingsOf: %v", err)
	}
	want := map[string]Rated{"ann": {Rating: 1900, RD: 50}, "bob": {Rating: 1800, RD: 50}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("RatingsOf = %v, want %v", got, want)
	}
}

func addHistoryWithRD(t *testing.T, pool *pgxpool.Pool, run int64, model string, format int, name string, periodDate string, rating, rd float64) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO rating_history (run_id, model_type, player_count, player_name, period_date, rating, rd, c_rating)
		VALUES ($1, $2, $3, $4, $5, $6::float8, $7::float8, $6::float8 - 3 * $7::float8)
		ON CONFLICT (run_id, model_type, player_count, player_name, period_date) DO UPDATE
		SET rating = EXCLUDED.rating, rd = EXCLUDED.rd, c_rating = EXCLUDED.c_rating`,
		run, model, format, name, periodDate, rating, rd)
	if err != nil {
		t.Fatalf("inserting rating_history: %v", err)
	}
}

func TestHistoricalRatingsOfReturnsRatingsBeforeRequestedMonth(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	other := publishedRun(t, pool)

	addHistoryWithRD(t, pool, run, "glicko2_daneo", 0, "ann", "2024-01-20", 1600, 80)
	addHistoryWithRD(t, pool, run, "glicko2_daneo", 0, "ann", "2024-03-25", 1700, 60)
	addHistoryWithRD(t, pool, run, "glicko2_daneo", 0, "ann", "2024-05-20", 1800, 50)
	addHistoryWithRD(t, pool, run, "glicko2_daneo", 0, "bob", "2024-04-15", 1900, 40)
	addHistoryWithRD(t, pool, run, "glicko2_daneo", 2, "ann", "2024-03-25", 1200, 30)
	addHistoryWithRD(t, pool, other, "glicko2_daneo", 0, "ann", "2024-03-25", 1300, 30)

	queries := []PlayerAt{
		{Name: "ann", Month: "2024-02"},
		{Name: "ann", Month: "2024-04"},
		{Name: "ann", Month: "2024-06"},
		{Name: "ann", Month: "2024-01"},
		{Name: "bob", Month: "2024-03"},
		{Name: "bob", Month: "2024-05"},
		{Name: "nobody", Month: "2024-05"},
	}

	got, err := r.HistoricalRatingsOf(ctx, run, "glicko2_daneo", 0, queries)
	if err != nil {
		t.Fatalf("HistoricalRatingsOf: %v", err)
	}
	want := map[PlayerAt]Rated{
		{Name: "ann", Month: "2024-02"}: {Rating: 1600, RD: 80},
		{Name: "ann", Month: "2024-04"}: {Rating: 1700, RD: 60},
		{Name: "ann", Month: "2024-06"}: {Rating: 1800, RD: 50},
		{Name: "bob", Month: "2024-05"}: {Rating: 1900, RD: 40},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("HistoricalRatingsOf = %v, want %v", got, want)
	}
}

func TestPlayerRivalsCountsEachOpponentsRecord(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	other := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob", "cy", "dee")
	addGame(t, pool, run, "2025-01-01", "Cup", Seat{"ann", 9}, Seat{"bob", 5})
	addGame(t, pool, run, "2025-01-02", "Cup", Seat{"bob", 9}, Seat{"ann", 5})
	addGame(t, pool, run, "2025-01-03", "Cup", Seat{"ann", 5}, Seat{"bob", 5})
	addGame(t, pool, run, "2025-01-04", "Cup", Seat{"cy", 9}, Seat{"ann", 7}, Seat{"dee", 5})
	addGame(t, pool, run, "2025-01-05", "Cup", Seat{"bob", 9}, Seat{"cy", 5})
	addGame(t, pool, other, "2025-01-06", "Cup", Seat{"ann", 9}, Seat{"dee", 5})

	name, got, err := r.PlayerRivals(ctx, run, "ann", 0)
	if err != nil {
		t.Fatalf("PlayerRivals: %v", err)
	}
	want := []Rival{
		{Name: "bob", Games: 3, Wins: 1, Losses: 1, Draws: 1},
		{Name: "dee", Games: 1, Wins: 1},
		{Name: "cy", Games: 1, Losses: 1},
	}
	if name != "ann" || !slices.Equal(got, want) {
		t.Errorf("PlayerRivals(ann, 0) = %q, %v; want ann, %v", name, got, want)
	}
	_, duels, err := r.PlayerRivals(ctx, run, "ann", 2)
	if err != nil || len(duels) != 1 || duels[0].Name != "bob" {
		t.Errorf("PlayerRivals(ann, 2) = %v, %v; want bob only", duels, err)
	}
}

func addYearlyTitle(t *testing.T, pool *pgxpool.Pool, run int64, name string, year int, title string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		"INSERT INTO player_yearly_titles (run_id, player_name, year, title) VALUES ($1, $2, $3, $4)", run, name, year, title)
	if err != nil {
		t.Fatalf("adding %s's %d title: %v", name, year, err)
	}
}

func TestPlayerCareerGivesEachYearsRecordPeakAndTitle(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	other := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob", "cy")
	addGame(t, pool, run, "2024-03-01", "Cup", Seat{"ann", 9}, Seat{"bob", 5})
	addGame(t, pool, run, "2024-04-01", "Cup", Seat{"ann", 5}, Seat{"bob", 5}, Seat{"cy", 7})
	addGame(t, pool, run, "2025-02-01", "Cup", Seat{"bob", 9}, Seat{"ann", 5})
	addGame(t, pool, other, "2025-03-01", "Cup", Seat{"ann", 9}, Seat{"bob", 5})
	addYearlyTitle(t, pool, run, "ann", 2024, "GM")
	addYearlyTitle(t, pool, other, "ann", 2025, "M")
	addHistory(t, pool, run, "glicko2_daneo", 0, "ann", "2024-03-31", 1600)
	addHistory(t, pool, run, "glicko2_daneo_softer", 0, "ann", "2024-04-30", 1700)
	addHistory(t, pool, run, "glicko2_std", 2, "ann", "2024-04-30", 1900)

	name, years, _, err := r.PlayerCareer(ctx, run, "ANN", 0, "glicko2_daneo")
	if err != nil {
		t.Fatalf("PlayerCareer: %v", err)
	}
	peak2024 := 1690.0
	want := []CareerYear{
		{Year: 2024, Opponents: 3, Wins: 1, Losses: 1, Draws: 1, PeakRating: &peak2024, Title: ptr("GM")},
		{Year: 2025, Opponents: 1, Losses: 1},
	}
	if name != "ann" || fmt.Sprint(deref(years)) != fmt.Sprint(deref(want)) {
		t.Errorf("PlayerCareer(ANN, 0, glicko2_daneo) = %q, %v; want ann, %v (GlickoD's peak, not WHR's)", name, deref(years), deref(want))
	}

	_, duels, _, err := r.PlayerCareer(ctx, run, "ann", 2, "glicko2_std")
	if err != nil || len(duels) != 2 || duels[0].Opponents != 1 || duels[0].PeakRating == nil || *duels[0].PeakRating != 1990 {
		t.Errorf("PlayerCareer(ann, 2) = %v, %v; want 2P years, peak from 2P history", deref(duels), err)
	}
	_, fours, _, err := r.PlayerCareer(ctx, run, "ann", 4, "glicko2_std")
	if err != nil || len(fours) != 2 || fours[0].Opponents != 3 || fours[0].PeakRating != nil {
		t.Errorf("PlayerCareer(ann, 4) = %v, %v; want all formats' years, no 4P peak", deref(fours), err)
	}
}

func addHistoryRow(t *testing.T, pool *pgxpool.Pool, run int64, model string, format int, name, period string, rating, cRating float64) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO rating_history (run_id, model_type, player_count, player_name, period_date, rating, rd, c_rating)
		VALUES ($1, $2, $3, $4, $5, $6, 30, $7)`, run, model, format, name, period, rating, cRating)
	if err != nil {
		t.Fatalf("adding %s's history at %s: %v", name, period, err)
	}
}

func TestPlayerCareerGivesTheModelsPeakRankedByConservativeRating(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann")
	addHistoryRow(t, pool, run, "glicko2_std", 0, "ann", "2024-01-31", 1790, 1700)
	addHistoryRow(t, pool, run, "glicko2_std", 0, "ann", "2024-02-29", 1890.04, 1800)
	addHistoryRow(t, pool, run, "glicko2_std", 0, "ann", "2024-03-31", 1890, 1810)
	addHistoryRow(t, pool, run, "glicko2_std", 0, "bob", "2024-02-29", 1940, 1850)
	addHistoryRow(t, pool, run, "glicko2_std", 0, "cy", "2024-02-29", 1950, 1700)
	addHistoryRow(t, pool, run, "glicko2_std", 0, "dee", "2024-02-29", 1850, 1820)
	addHistoryRow(t, pool, run, "glicko2_std", 0, "eve", "2024-03-31", 2100, 2000)
	addHistoryRow(t, pool, run, "glicko2_std", 2, "ann", "2024-02-29", 3000, 2900)
	addHistoryRow(t, pool, run, "glicko2_daneo_softer", 0, "ann", "2024-01-31", 2500, 2400)

	tests := []struct {
		name  string
		model string
		want  *CareerPeak
	}{
		// The peak is compared rounded, so February's 1890.04 ties March's 1890 and comes first.
		{"bob and dee rank above by conservative rating, cy does not", "glicko2_std", &CareerPeak{Rating: 1890, Rank: 3, Date: day("2024-02-29")}},
		{"another model has its own peak", "glicko2_daneo_softer", &CareerPeak{Rating: 2500, Rank: 1, Date: day("2024-01-31")}},
		{"no history in the model", "glicko2_adapt", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, got, err := r.PlayerCareer(ctx, run, "ann", 0, tt.model)
			if err != nil {
				t.Fatalf("PlayerCareer(ann, 0, %s): %v", tt.model, err)
			}
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("PlayerCareer(ann, 0, %s) peak = %+v, want %+v", tt.model, got, tt.want)
			}
		})
	}
}

func TestVersusGivesTwoPlayersSharedGamesNewestFirst(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	other := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob", "cy")
	first := addGame(t, pool, run, "2024-01-01", "Cup", Seat{"ann", 9}, Seat{"bob", 5})
	trio := addGame(t, pool, run, "2024-02-01", "Cup", Seat{"cy", 9}, Seat{"bob", 7}, Seat{"ann", 5})
	addGame(t, pool, run, "2024-03-01", "Cup", Seat{"ann", 9}, Seat{"cy", 5})
	addGame(t, pool, other, "2024-04-01", "Cup", Seat{"ann", 9}, Seat{"bob", 5})
	for i := range 101 {
		addGame(t, pool, run, day("2020-01-01").AddDate(0, 0, i).Format(time.DateOnly), "Old", Seat{"bob", 1}, Seat{"ann", 2})
	}

	a, b, got, err := r.Versus(ctx, run, "ANN", "Bob")
	if err != nil {
		t.Fatalf("Versus: %v", err)
	}
	if a != "ann" || b != "bob" || len(got) != 100 || got[0].ID != trio || got[1].ID != first {
		t.Fatalf("Versus(ANN, Bob) = %q, %q, %d games starting %v; want ann, bob, 100 starting the trio then the duel", a, b, len(got), got[:2])
	}
	if !slices.Equal(got[0].Seats, []Seat{{"cy", 9}, {"bob", 7}, {"ann", 5}}) {
		t.Errorf("trio seats = %v, want seat order", got[0].Seats)
	}
	if _, _, _, err := r.Versus(ctx, run, "ann", "zed"); !errors.Is(err, ErrPlayerNotFound) {
		t.Errorf("Versus(ann, zed): err = %v, want ErrPlayerNotFound", err)
	}
}

// deref prints a career's optional fields by value, so two careers compare by content.
func deref(years []CareerYear) []string {
	out := make([]string, 0, len(years))
	for _, y := range years {
		peak, title := "-", "-"
		if y.PeakRating != nil {
			peak = fmt.Sprint(*y.PeakRating)
		}
		if y.Title != nil {
			title = *y.Title
		}
		out = append(out, fmt.Sprintf("%d %d/%d/%d/%d peak %s title %s", y.Year, y.Opponents, y.Wins, y.Losses, y.Draws, peak, title))
	}
	return out
}

func TestPlayerCareerAddsTheRunningRecord(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob", "cy")
	addGame(t, pool, run, "2024-03-01", "Cup", Seat{"ann", 9}, Seat{"bob", 5})
	addGame(t, pool, run, "2024-04-01", "Cup", Seat{"ann", 5}, Seat{"bob", 5}, Seat{"cy", 7})
	addGame(t, pool, run, "2025-02-01", "Cup", Seat{"bob", 9}, Seat{"ann", 5})

	_, years, _, err := r.PlayerCareer(ctx, run, "ann", 0, "glicko2_daneo")
	if err != nil {
		t.Fatalf("PlayerCareer: %v", err)
	}
	if len(years) != 2 {
		t.Fatalf("PlayerCareer = %d years, want 2", len(years))
	}
	y24, y25 := years[0], years[1]
	if y24.CareerOpponents != 3 || y24.CareerWins != 1 || y24.CareerLosses != 1 || y24.CareerDraws != 1 {
		t.Errorf("2024 career = %d/%d/%d/%d, want 3/1/1/1", y24.CareerOpponents, y24.CareerWins, y24.CareerLosses, y24.CareerDraws)
	}
	if y25.CareerOpponents != 4 || y25.CareerWins != 1 || y25.CareerLosses != 2 || y25.CareerDraws != 1 {
		t.Errorf("2025 career = %d/%d/%d/%d, want 4/1/2/1", y25.CareerOpponents, y25.CareerWins, y25.CareerLosses, y25.CareerDraws)
	}
}

func TestPlayerCareerGivesTheModelsYearEnd(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob")
	addGame(t, pool, run, "2024-03-01", "Cup", Seat{"ann", 9}, Seat{"bob", 5})
	addGame(t, pool, run, "2025-02-01", "Cup", Seat{"bob", 9}, Seat{"ann", 5})
	addHistory(t, pool, run, "glicko2_std", 0, "ann", "2024-04-30", 1600)
	addHistory(t, pool, run, "glicko2_std", 0, "ann", "2024-12-31", 1610.04)
	addHistory(t, pool, run, "glicko2_daneo_softer", 0, "ann", "2025-12-31", 1700)

	tests := []struct {
		model          string
		want24, want25 string
	}{
		{"glicko2_std", "1700", "-"},
		{"glicko2_daneo_softer", "-", "1790"},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			_, years, _, err := r.PlayerCareer(ctx, run, "ann", 0, tt.model)
			if err != nil || len(years) != 2 {
				t.Fatalf("PlayerCareer(ann, 0, %s) = %d years, %v; want 2", tt.model, len(years), err)
			}
			got24, got25 := yearEnd(years[0]), yearEnd(years[1])
			if got24 != tt.want24 || got25 != tt.want25 {
				t.Errorf("PlayerCareer(ann, 0, %s) year ends = %s, %s; want %s, %s", tt.model, got24, got25, tt.want24, tt.want25)
			}
		})
	}
}

// yearEnd prints a year's last rating, or - without one.
func yearEnd(y CareerYear) string {
	if y.YearEnd == nil {
		return "-"
	}
	return fmt.Sprint(*y.YearEnd)
}

func TestPlayerMatrixGivesTheLiveRatings(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann")
	addRating(t, pool, run, "glicko2_daneo", 0, "ann", ptr(4), 1800.04)
	addRating(t, pool, run, "glicko2_daneo_softer", 0, "ann", nil, 1700)
	addRating(t, pool, run, "glicko2_std_retro", 0, "ann", ptr(1), 1900)
	addRating(t, pool, run, "glicko2_daneo", 2, "ann", ptr(2), 1500)

	name, cells, err := r.PlayerMatrix(ctx, run, "ANN", 0, nil)
	if err != nil {
		t.Fatalf("PlayerMatrix: %v", err)
	}
	// addRating stores rating = c + 100 and RD 50.
	want := []MatrixCell{
		{Model: "glicko2_daneo", Rating: 1900, RD: 50, CRating: 1800, Rank: ptr(4)},
		{Model: "glicko2_daneo_softer", Rating: 1800, RD: 50, CRating: 1700},
	}
	if name != "ann" || fmt.Sprint(cellText(cells)) != fmt.Sprint(cellText(want)) {
		t.Errorf("PlayerMatrix(ANN, 0) = %q, %v; want ann, %v", name, cellText(cells), cellText(want))
	}
}

func TestPlayerMatrixTakesEachModelsLastPeriodOfTheYear(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob")
	addHistory(t, pool, run, "glicko2_std", 0, "ann", "2024-11-30", 1500)
	addHistory(t, pool, run, "glicko2_std", 0, "ann", "2024-12-31", 1600)
	addHistory(t, pool, run, "glicko2_std", 0, "bob", "2024-12-31", 1650)
	addHistory(t, pool, run, "glicko2_std", 0, "bob", "2025-01-31", 1700)
	addHistory(t, pool, run, "glicko2_daneo_softer", 0, "ann", "2024-06-30", 1400)
	addHistory(t, pool, run, "glicko2_daneo_softer", 0, "bob", "2024-12-31", 1450)

	year := 2024
	_, cells, err := r.PlayerMatrix(ctx, run, "ann", 0, &year)
	if err != nil {
		t.Fatalf("PlayerMatrix: %v", err)
	}
	// ann is second by conservative rating on Standard's last 2024 period; she has no WHR row on WHR's.
	want := []MatrixCell{{Model: "glicko2_std", Rating: 1690, RD: 30, CRating: 1600, Rank: ptr(2)}}
	if fmt.Sprint(cellText(cells)) != fmt.Sprint(cellText(want)) {
		t.Errorf("PlayerMatrix(ann, 2024) = %v, want %v", cellText(cells), cellText(want))
	}
}

func cellText(cells []MatrixCell) []string {
	out := []string{}
	for _, c := range cells {
		rank := "-"
		if c.Rank != nil {
			rank = fmt.Sprint(*c.Rank)
		}
		out = append(out, fmt.Sprintf("%s %v±%v C%v #%s", c.Model, c.Rating, c.RD, c.CRating, rank))
	}
	return out
}

func TestYearBoardTakesEveryPlayersLastRatingOfTheYear(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob", "cy")
	if _, err := SetCountries(ctx, pool, map[string]string{"ann": "PL"}); err != nil {
		t.Fatalf("SetCountries: %v", err)
	}
	addTitle(t, pool, run, "ann", "GM", 1, "Royal League")
	addGame(t, pool, run, "2023-05-01", "Cup", Seat{"ann", 9}, Seat{"bob", 5})
	addGame(t, pool, run, "2024-03-01", "Cup", Seat{"ann", 5}, Seat{"bob", 5}, Seat{"cy", 7})
	addGame(t, pool, run, "2024-12-20", "Cup", Seat{"bob", 9}, Seat{"ann", 5})
	addGame(t, pool, run, "2025-01-10", "Cup", Seat{"ann", 9}, Seat{"cy", 5})
	addHistory(t, pool, run, "glicko2_std", 0, "ann", "2023-12-31", 1500)
	addHistory(t, pool, run, "glicko2_std", 0, "ann", "2024-11-30", 1550)
	addHistory(t, pool, run, "glicko2_std", 0, "ann", "2024-12-31", 1600)
	addHistory(t, pool, run, "glicko2_std", 0, "bob", "2024-12-31", 1650)
	addHistory(t, pool, run, "glicko2_std", 0, "cy", "2024-03-31", 1700)
	addHistory(t, pool, run, "glicko2_std", 0, "cy", "2025-01-31", 1800)
	addHistory(t, pool, run, "glicko2_daneo_softer", 0, "cy", "2024-12-31", 1900)
	addYearlyTitle(t, pool, run, "bob", 2024, "WC")
	withYearRecords(t, pool, run)

	got, err := r.YearBoard(ctx, run, 2024, "glicko2_std", 0)
	if err != nil {
		t.Fatalf("YearBoard: %v", err)
	}
	if !got.Snapshot.Equal(day("2024-12-31")) || got.Previous == nil || !got.Previous.Equal(day("2023-12-31")) {
		t.Errorf("snapshot %v, previous %v; want 2024-12-31 and 2023-12-31", got.Snapshot, got.Previous)
	}
	if got.Games != 2 || fmt.Sprint(got.Champions) != "[{bob false}]" {
		t.Errorf("games %d, champions %v; want 2 games in 2024, bob", got.Games, got.Champions)
	}
	// cy played in 2024 only in March: on the board with that rating, not 2025's; ann's previous is her 2023 year end.
	if len(got.Rows) != 3 {
		t.Fatalf("%d rows, want cy, bob and ann: %+v", len(got.Rows), got.Rows)
	}
	if cy := got.Rows[0]; cy.Name != "cy" || cy.CRating != 1700 {
		t.Errorf("first row = %s at %v, want cy at 1700 (his last 2024 rating, the highest)", cy.Name, cy.CRating)
	}
	if got.Rows[1].Name != "bob" {
		t.Errorf("second row = %s, want bob", got.Rows[1].Name)
	}
	ann := got.Rows[2]
	if ann.Name != "ann" || ann.Rating != 1690 || ann.CRating != 1600 || ann.Country == nil || *ann.Country != "PL" ||
		ann.Title == nil || ann.Title.Code != "GM" {
		t.Errorf("ann = %+v", ann)
	}
	if ann.Season != (Record{Opponents: 3, Wins: 0, Losses: 2, Draws: 1}) || ann.Career != (Record{Opponents: 4, Wins: 1, Losses: 2, Draws: 1}) {
		t.Errorf("ann's season %+v, career %+v; want 3/0/2/1 and 4/1/2/1", ann.Season, ann.Career)
	}
	if ann.LastPlayed == nil || !ann.LastPlayed.Equal(day("2024-12-20")) {
		t.Errorf("ann's last game of 2024 = %v, want 2024-12-20", ann.LastPlayed)
	}
	if ann.PreviousCRating == nil || *ann.PreviousCRating != 1500 {
		t.Errorf("ann's previous C = %v, want 1500", ann.PreviousCRating)
	}
	if bob := got.Rows[1]; bob.PreviousCRating != nil {
		t.Errorf("bob = %+v, want no previous rating", bob)
	}
}

func TestYearBoardCountsTheRecordsTheRunRecorded(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob")
	addGame(t, pool, run, "2023-05-01", "Cup", Seat{"ann", 9}, Seat{"bob", 5})
	addGame(t, pool, run, "2024-03-01", "Cup", Seat{"ann", 9}, Seat{"bob", 5})
	addHistory(t, pool, run, "glicko2_std", 0, "ann", "2024-12-31", 1600)
	withYearRecords(t, pool, run)
	if _, err := pool.Exec(ctx, "UPDATE pairwise_matches SET outcome_a = 0 WHERE run_id = $1", run); err != nil {
		t.Fatalf("turning the run's pairs into losses: %v", err)
	}

	got, err := r.YearBoard(ctx, run, 2024, "glicko2_std", 0)
	if err != nil {
		t.Fatalf("YearBoard: %v", err)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("%d rows, want ann's: %+v", len(got.Rows), got.Rows)
	}
	if ann := got.Rows[0]; ann.Season != (Record{Opponents: 1, Wins: 1}) || ann.Career != (Record{Opponents: 2, Wins: 2}) {
		t.Errorf("ann's season %+v, career %+v; want 1 win and 2 wins, as the run recorded them", ann.Season, ann.Career)
	}
}

func TestYearBoardCountsEachFormatsGames(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob", "cy")
	addGame(t, pool, run, "2024-03-01", "Cup", Seat{"ann", 9}, Seat{"bob", 5})
	addGame(t, pool, run, "2024-04-01", "Cup", Seat{"ann", 9}, Seat{"bob", 5}, Seat{"cy", 7})
	for _, format := range []int{0, 2, 3} {
		addHistory(t, pool, run, "glicko2_std", format, "ann", "2024-12-31", 1600)
	}
	withYearRecords(t, pool, run)

	tests := []struct {
		name      string
		format    int
		opponents int
	}{
		{"every format", 0, 3},
		{"two players", 2, 1},
		{"three players", 3, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.YearBoard(ctx, run, 2024, "glicko2_std", tt.format)
			if err != nil || len(got.Rows) != 1 {
				t.Fatalf("YearBoard(format %d) = %+v, %v; want ann's row", tt.format, got.Rows, err)
			}
			if ann := got.Rows[0]; ann.Season.Opponents != tt.opponents || ann.Career.Opponents != tt.opponents {
				t.Errorf("format %d: ann's season %+v, career %+v; want %d opponents in both",
					tt.format, ann.Season, ann.Career, tt.opponents)
			}
		})
	}
}

func TestYearBoardRefusesAYearWithoutHistory(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	run := publishedRun(t, pool)
	addHistory(t, pool, run, "glicko2_std", 0, "ann", "2024-12-31", 1600)
	if _, err := r.YearBoard(context.Background(), run, 2019, "glicko2_std", 0); !errors.Is(err, ErrNoHistory) {
		t.Errorf("YearBoard(2019): err = %v, want ErrNoHistory", err)
	}
}

func TestYearBoardRefusesAYearOutsideTheRunsGames(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	if _, err := pool.Exec(ctx, "UPDATE rating_runs SET first_game = '2024-01-05', last_game = '2024-12-20' WHERE run_id = $1", run); err != nil {
		t.Fatalf("recording run %d's games: %v", run, err)
	}
	addHistory(t, pool, run, "glicko2_std", 0, "ann", "2024-12-31", 1600)
	addHistory(t, pool, run, "glicko2_daneo_softer", 0, "ann", "2019-12-31", 1500)

	if _, err := r.YearBoard(ctx, run, 2019, "glicko2_std", 0); !errors.Is(err, ErrNoHistory) {
		t.Errorf("YearBoard(2019) of a run whose games are all in 2024: err = %v, want ErrNoHistory", err)
	}
	if _, err := r.YearBoard(ctx, run, 2024, "glicko2_std", 0); err != nil {
		t.Errorf("YearBoard(2024): %v", err)
	}
}

func TestYearBoardNamesTheReigningChampionInTheRunsLatestYear(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann", "bob")
	addGame(t, pool, run, "2025-06-01", "Cup", Seat{"ann", 9}, Seat{"bob", 5})
	addGame(t, pool, run, "2026-03-01", "Cup", Seat{"ann", 9}, Seat{"bob", 5})
	addHistory(t, pool, run, "glicko2_std", 0, "ann", "2025-06-30", 1600)
	addHistory(t, pool, run, "glicko2_std", 0, "ann", "2026-03-31", 1650)
	addTitle(t, pool, run, "ann", "WC", 1, "Reigning World Champion")
	addYearlyTitle(t, pool, run, "ann", 2025, "WC")

	for year, want := range map[int]string{2026: "[{ann true}]", 2025: "[{ann false}]"} {
		got, err := r.YearBoard(ctx, run, year, "glicko2_std", 0)
		if err != nil || fmt.Sprint(got.Champions) != want {
			t.Errorf("YearBoard(%d) champions = %v, %v; want %s", year, got.Champions, err, want)
		}
	}
}

func TestYearsListsTheRunsYearsNewestFirst(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	run := publishedRun(t, pool)
	other := publishedRun(t, pool)
	addHistory(t, pool, run, "glicko2_daneo", 0, "ann", "2019-03-31", 1600)
	addHistory(t, pool, run, "glicko2_daneo", 0, "ann", "2021-01-31", 1600)
	addHistory(t, pool, run, "glicko2_daneo", 0, "bob", "2021-02-28", 1600)
	addHistory(t, pool, other, "glicko2_daneo", 0, "ann", "2023-01-31", 1600)
	addYearlyTitle(t, pool, run, "ann", 2019, "WC")
	addYearlyTitle(t, pool, other, "ann", 2021, "WC")

	got, err := r.Years(context.Background(), run)
	if err != nil || fmt.Sprint(got) != "[{2021 false} {2019 true}]" {
		t.Errorf("Years = %v, %v; want 2021 without a champion, 2019 with one", got, err)
	}
}

func TestPlayerGivesTheirBadges(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	addPlayers(t, pool, "ann")
	addRating(t, pool, run, "glicko2_daneo", 0, "ann", ptr(1), 1900)
	giveBadges(t, pool, "ann", "Organiser")

	got, err := r.Player(ctx, run, "ann")
	if err != nil {
		t.Fatalf("Player: %v", err)
	}
	if diff := cmp.Diff([]ExtraBadge{{Label: "Organiser", Description: "Organiser description"}}, got.Badges); diff != "" {
		t.Errorf("badges (-want +got):\n%s", diff)
	}
}

func TestPlayerSeats(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)

	addPlayers(t, pool, "ann", "bob", "cyd")
	// 2P game (rated): ann seat 1 wins outright
	addGame(t, pool, run, "2026-01-01", "Dual 1", Seat{"ann", 150}, Seat{"bob", 100})
	// 2P game (rated): ann seat 2 loses outright
	addGame(t, pool, run, "2026-01-02", "Dual 2", Seat{"bob", 120}, Seat{"ann", 110})
	// 2P game (rated): ann seat 1 draws
	addGame(t, pool, run, "2026-01-03", "Dual 3", Seat{"ann", 130}, Seat{"bob", 130})

	// 3P unrated game (run=0): ann seat 1 wins outright
	addGame(t, pool, 0, "2026-01-04", "Exhibition", Seat{"ann", 200}, Seat{"bob", 150}, Seat{"cyd", 100})

	// 1. Rated only
	ratedStats, err := r.PlayerSeats(ctx, run, "ann", false)
	if err != nil {
		t.Fatalf("PlayerSeats(rated): %v", err)
	}
	if ratedStats.Overall.TotalGames != 3 || ratedStats.Overall.TotalWins != 1 || ratedStats.Overall.TotalDraws != 1 {
		t.Errorf("rated total: games %d, wins %d, draws %d; want 3, 1, 1", ratedStats.Overall.TotalGames, ratedStats.Overall.TotalWins, ratedStats.Overall.TotalDraws)
	}
	if ratedStats.Overall.WinRate != 33.33 {
		t.Errorf("rated winrate: %f; want 33.33", ratedStats.Overall.WinRate)
	}
	if ratedStats.Overall.StrongestSeat == nil || ratedStats.Overall.StrongestSeat.Seat != 1 {
		t.Errorf("strongest seat: %v; want seat 1", ratedStats.Overall.StrongestSeat)
	}
	if ratedStats.Overall.WeakestSeat == nil || ratedStats.Overall.WeakestSeat.Seat != 2 {
		t.Errorf("weakest seat: %v; want seat 2", ratedStats.Overall.WeakestSeat)
	}

	// 2. Unrated included
	allStats, err := r.PlayerSeats(ctx, run, "ann", true)
	if err != nil {
		t.Fatalf("PlayerSeats(unrated): %v", err)
	}
	if allStats.Overall.TotalGames != 4 || allStats.Overall.TotalWins != 2 || allStats.Overall.TotalDraws != 1 {
		t.Errorf("all games total: games %d, wins %d, draws %d; want 4, 2, 1", allStats.Overall.TotalGames, allStats.Overall.TotalWins, allStats.Overall.TotalDraws)
	}
}

func TestPlayerRivalsLeavesOutHiddenPlayers(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	addGame(t, pool, run, "2025-01-01", "Cup", Seat{"ann", 9}, Seat{"bob", 5}, Seat{"cat", 3})
	addGame(t, pool, run, "2025-01-02", "Cup", Seat{"ann", 9}, Seat{"bob", 5})
	hideAccount(t, pool, "bob", "banned")

	_, got, err := NewRatingsReader(asReader(t, pool)).PlayerRivals(ctx, run, "ann", 0)
	if err != nil {
		t.Fatalf("PlayerRivals: %v", err)
	}
	if len(got) != 1 || got[0].Name != "cat" {
		t.Errorf("PlayerRivals(ann) = %+v, want cat only: bob's account is banned", got)
	}
}

func TestYearBoardMasksHiddenChampions(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	addGame(t, pool, run, "2025-06-01", "Cup", Seat{"ann", 9}, Seat{"cat", 5})
	addHistory(t, pool, run, "glicko2_std", 0, "cat", "2025-06-30", 1600)
	addYearlyTitle(t, pool, run, "ann", 2025, "WC")
	addYearlyTitle(t, pool, run, "cat", 2025, "WC")
	hideAccount(t, pool, "ann", "deleted")

	got, err := NewRatingsReader(asReader(t, pool)).YearBoard(ctx, run, 2025, "glicko2_std", 0)
	if err != nil {
		t.Fatalf("YearBoard: %v", err)
	}
	if want := "[{Deleted player false} {cat false}]"; fmt.Sprint(got.Champions) != want {
		t.Errorf("2025 champions = %v, want %s: ann's account is deleted", got.Champions, want)
	}
}
