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

// withMonthOfGames adds a run whose last game is 2026-11-03, ratings before and in the month, and games: a duel in
// the month, a trio in it with cy timing out, a duel before the month, and a duel with a player rated only since.
func withMonthOfGames(t *testing.T) (*RatingsReader, *pgxpool.Pool, int64) {
	t.Helper()
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	if _, err := pool.Exec(ctx, "UPDATE rating_runs SET last_game = '2026-11-03' WHERE run_id = $1", run); err != nil {
		t.Fatalf("setting the run's last game: %v", err)
	}
	addPlayers(t, pool, "ann", "bob", "cy", "dee")
	addGame(t, pool, run, "2026-10-20", "Duel", Seat{"bob", 9}, Seat{"ann", 5})
	addGame(t, pool, run, "2026-11-02", "Trio", Seat{"bob", 9}, Seat{"ann", 5}, Seat{"cy", 1})
	setOutcome(t, pool, "Trio", "cy", "timed_out")
	addGame(t, pool, run, "2026-10-01", "Old duel", Seat{"bob", 9}, Seat{"ann", 5})
	addGame(t, pool, run, "2026-10-25", "Newcomer", Seat{"dee", 9}, Seat{"ann", 5})
	for _, h := range []struct {
		format       int
		name, period string
		rating, rd   float64
	}{
		{2, "ann", "2026-09-30", 1900, 50}, {2, "bob", "2026-09-30", 1500, 80},
		{2, "ann", "2026-10-31", 1850, 49}, {2, "bob", "2026-10-31", 1560, 79}, {2, "dee", "2026-10-31", 1700, 200},
		{3, "ann", "2026-10-31", 1800, 60}, {3, "bob", "2026-10-31", 1600, 70}, {3, "cy", "2026-10-31", 1700, 90},
	} {
		addHistoryWithRD(t, pool, run, "glicko2_daneo", h.format, h.name, h.period, h.rating, h.rd)
	}
	addHistoryWithRD(t, pool, run, "glicko2_std", 2, "ann", "2026-09-30", 1000, 50)
	return NewRatingsReader(asReader(t, pool)), pool, run
}

// pairsOf writes each pair as "game name-other rating/rd vs rating/rd score timed-out".
func pairsOf(pairs []UpsetPair) []string {
	var got []string
	for _, p := range pairs {
		got = append(got, fmt.Sprintf("%s %s-%s %.0f/%.0f vs %.0f/%.0f %.1f %v", p.Tournament, p.Name, p.Other,
			p.Rating, p.RD, p.OtherRating, p.OtherRD, p.Score, p.TimedOut))
	}
	slices.Sort(got)
	return got
}

func TestUpsetPairsAreTheLastMonthsWithRatingsFromBeforeEachGame(t *testing.T) {
	r, _, run := withMonthOfGames(t)
	tests := []struct {
		name   string
		format int
		want   []string
	}{
		{"every format", 0, []string{
			"Duel ann-bob 1900/50 vs 1500/80 0.0 false", "Duel bob-ann 1500/80 vs 1900/50 1.0 false",
			"Trio ann-bob 1800/60 vs 1600/70 0.0 false", "Trio ann-cy 1800/60 vs 1700/90 1.0 false",
			"Trio bob-ann 1600/70 vs 1800/60 1.0 false", "Trio bob-cy 1600/70 vs 1700/90 1.0 false",
			"Trio cy-ann 1700/90 vs 1800/60 0.0 true", "Trio cy-bob 1700/90 vs 1600/70 0.0 true",
		}},
		{"one format", 2, []string{"Duel ann-bob 1900/50 vs 1500/80 0.0 false", "Duel bob-ann 1500/80 vs 1900/50 1.0 false"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from, to, pairs, err := r.UpsetPairs(context.Background(), run, tt.format)
			if err != nil {
				t.Fatalf("UpsetPairs: %v", err)
			}
			if got := pairsOf(pairs); !slices.Equal(got, tt.want) {
				t.Errorf("pairs = %q, want %q", got, tt.want)
			}
			if f, l := from.Format(time.DateOnly), to.Format(time.DateOnly); f != "2026-10-04" || l != "2026-11-03" {
				t.Errorf("window = %s to %s, want 2026-10-04 to 2026-11-03", f, l)
			}
		})
	}
}

func TestUpsetPairsLeaveOutHiddenPlayers(t *testing.T) {
	r, pool, run := withMonthOfGames(t)
	if _, err := pool.Exec(context.Background(), "INSERT INTO player_accounts (cge_user, player_id, name, deleted) SELECT 1, player_id, 'cy', true FROM players WHERE name = 'cy'"); err != nil {
		t.Fatalf("hiding cy: %v", err)
	}
	_, _, pairs, err := r.UpsetPairs(context.Background(), run, 3)
	if err != nil {
		t.Fatalf("UpsetPairs: %v", err)
	}
	if got, want := pairsOf(pairs), []string{"Trio ann-bob 1800/60 vs 1600/70 0.0 false", "Trio bob-ann 1600/70 vs 1800/60 1.0 false"}; !slices.Equal(got, want) {
		t.Errorf("pairs = %q, want %q (none with cy)", got, want)
	}
}

func TestUpsetPairsRefuseAnUnpublishedRun(t *testing.T) {
	pool := migrated(t)
	run := addRun(t, pool, "complete", time.Time{})
	if _, _, _, err := NewRatingsReader(asReader(t, pool)).UpsetPairs(context.Background(), run, 0); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("UpsetPairs(unpublished) err = %v, want ErrRunNotFound", err)
	}
}
