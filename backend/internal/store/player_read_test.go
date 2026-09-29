//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// addGame stores one game and, if run is not zero, the run's 1v1 pairs for it.
func addGame(t *testing.T, pool *pgxpool.Pool, run int64, date, tournament string, seats ...Seat) int64 {
	t.Helper()
	ctx := context.Background()
	var batch int64
	if err := pool.QueryRow(ctx, "INSERT INTO import_batches (source) VALUES ('load') RETURNING batch_id").Scan(&batch); err != nil {
		t.Fatalf("adding a batch: %v", err)
	}
	cols := make([]any, 8)
	for i, s := range seats {
		cols[2*i], cols[2*i+1] = s.Name, s.Score
	}
	var id int64
	err := pool.QueryRow(ctx, `
		INSERT INTO matches (batch_id, source_file, source_line, tournament, date, player_count,
			player1, score1, player2, score2, player3, score3, player4, score4)
		VALUES ($1, 'f.csv', 1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING match_id`,
		append([]any{batch, tournament, date, len(seats)}, cols...)...).Scan(&id)
	if err != nil {
		t.Fatalf("adding a game on %s: %v", date, err)
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

func publishedRun(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	return addRun(t, pool, "complete", time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC))
}

func TestPlayerFindsANameExactlyOrByAUniqueCase(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	unpublished := addRun(t, pool, "complete", time.Time{})
	addPlayers(t, pool, "Ann", "Nyalala", "nyalala")

	tests := []struct {
		name, asked string
		run         int64
		want        string
		err         error
	}{
		{name: "exact name", asked: "Ann", run: run, want: "Ann"},
		{name: "another case, one match", asked: "aNN", run: run, want: "Ann"},
		{name: "exact name among two cases", asked: "nyalala", run: run, want: "nyalala"},
		{name: "another case, two matches", asked: "NYALALA", run: run, err: ErrPlayerNotFound},
		{name: "unknown name", asked: "zed", run: run, err: ErrPlayerNotFound},
		{name: "unpublished run", asked: "Ann", run: unpublished, err: ErrRunNotFound},
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
	if _, err := pool.Exec(ctx, "INSERT INTO player_peak_titles (run_id, player_name, peak_title, peak_year) VALUES ($1, 'ann', 'WC', 2024)", run); err != nil {
		t.Fatalf("adding ann's peak: %v", err)
	}
	for _, model := range PlayerPageModels {
		addRating(t, pool, run, model, 0, "ann", intp(4), 1800)
	}
	addRating(t, pool, run, "glicko2_daneo", 3, "ann", nil, 1700)
	addRating(t, pool, run, "glicko2_std_retro", 0, "ann", intp(1), 1900)
	addRating(t, pool, run, "glicko2_daneo", 0, "bob", intp(1), 1900)
	addRating(t, pool, other, "glicko2_daneo", 0, "ann", intp(9), 1000)

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
	addHistory(t, pool, run, "whr_softer", 0, "ann", "2025-01-31", 1700)
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
		{Model: "whr_softer", Date: day("2025-01-31"), Rating: 1790, CRating: 1700},
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

func TestRatingsOfGivesTheNamedPlayersInOneModelAndFormat(t *testing.T) {
	pool := migrated(t)
	r := NewRatingsReader(asReader(t, pool))
	ctx := context.Background()
	run := publishedRun(t, pool)
	other := publishedRun(t, pool)
	addRating(t, pool, run, "glicko2_std", 2, "ann", intp(1), 1800)
	addRating(t, pool, run, "glicko2_std", 2, "bob", intp(2), 1700)
	addRating(t, pool, run, "glicko2_std", 2, "cy", intp(3), 1600)
	addRating(t, pool, run, "glicko2_std", 0, "ann", intp(1), 1000)
	addRating(t, pool, run, "glicko2_mp", 2, "ann", intp(1), 1100)
	addRating(t, pool, other, "glicko2_std", 2, "bob", intp(1), 1200)

	got, err := r.RatingsOf(ctx, run, "glicko2_std", 2, []string{"ann", "bob", "nobody"})
	if err != nil {
		t.Fatalf("RatingsOf: %v", err)
	}
	want := map[string]Rated{"ann": {Rating: 1900, RD: 50}, "bob": {Rating: 1800, RD: 50}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("RatingsOf = %v, want %v", got, want)
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
