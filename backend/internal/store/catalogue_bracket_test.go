//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// bracketLines writes each group of a bracket as "stage|group number|state" and its seats as "entrant name place
// points →next".
func bracketLines(d SeasonBracket) []string {
	var out []string
	for _, r := range d.Rounds {
		for _, g := range r.Groups {
			line := fmt.Sprintf("%s|%s %d|%s:", r.Stage, g.Name, g.Number, g.State)
			for _, s := range g.Seats {
				points := "-"
				if s.Points != nil {
					points = fmt.Sprint(*s.Points)
				}
				line += fmt.Sprintf(" %d %s %d %s →%d;", s.Entrant, s.Name, s.Place, points, s.Next)
			}
			out = append(out, line)
		}
	}
	return out
}

// bracketFixture places Wimbledon 2024 as a bracket: two groups of Stage 1, played, and Stage 2's group, still running,
// with 6-3-1-0 points per place.
func bracketFixture(t *testing.T) (int64, *RatingsReader) {
	t.Helper()
	pool := migrated(t)
	ctx := context.Background()
	run := publishedRun(t, pool)
	placedOn(t, pool, map[string]time.Time{
		"Wimbledon 2024 Stage 1 - group 2 game 1": day("2024-07-01"),
		"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01"),
		"Wimbledon 2024 Stage 2 - group 1 game 1": day("2024-07-10"),
	})
	setSeats(t, pool, "%Stage 1 - group 1 %", Seat{"dan", 10}, Seat{"ann", 40}, Seat{"bob", 30}, Seat{"cat", 20})
	setSeats(t, pool, "%Stage 1 - group 2 %", Seat{"eve", 40}, Seat{"fay", 30}, Seat{"gus", 20}, Seat{"hal", 10})
	setSeats(t, pool, "%Stage 2 - group 1 %", Seat{"fay", 50}, Seat{"ann", 40}, Seat{"eve", 30}, Seat{"bob", 20})
	setGameState(t, pool, "%Stage 2 - group 1 %", "running")
	for _, q := range []string{
		"INSERT INTO settings (series_id, points_4p) SELECT series_id, '{6,3,1,0}' FROM series WHERE slug = 'wimbledon'",
		"UPDATE series SET bracket = true WHERE slug = 'wimbledon'",
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("setting up the bracket: %v", err)
		}
	}
	hideAccount(t, pool, "hal", "deleted")
	withSpans(t, pool, run)
	return run, NewRatingsReader(asReader(t, pool))
}

func TestSeasonBracketGivesEachRoundsGroupsAndWhereEachPlayerWentNext(t *testing.T) {
	run, r := bracketFixture(t)
	got, err := r.SeasonBracket(context.Background(), run, "wimbledon", "2024")
	if err != nil {
		t.Fatalf("SeasonBracket: %v", err)
	}
	want := []string{
		"Stage 1|group 1 1|played: 1 ann 1 6 →1; 2 bob 2 3 →1; 3 cat 3 1 →0; 4 dan 4 0 →0;",
		"Stage 1|group 2 2|played: 5 eve 1 6 →1; 6 fay 2 3 →1; 7 gus 3 1 →0; 8 Deleted player 4 0 →0;",
		"Stage 2|group 1 1|running: 6 fay 0 - →0; 1 ann 0 - →0; 5 eve 0 - →0; 2 bob 0 - →0;",
	}
	if diff := cmp.Diff(want, bracketLines(got)); diff != "" {
		t.Errorf("the bracket (-want +got):\n%s", diff)
	}
}

func TestSeasonBracketRefusesWhatIsNotABracketSeason(t *testing.T) {
	run, r := bracketFixture(t)
	for _, tt := range []struct{ name, slug, season string }{
		{"a season it does not have", "wimbledon", "2023"},
		{"a tournament it does not know", "nope", "2024"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := r.SeasonBracket(context.Background(), run, tt.slug, tt.season); !errors.Is(err, ErrTournamentNotFound) {
				t.Errorf("SeasonBracket(%s, %s) = %v, want ErrTournamentNotFound", tt.slug, tt.season, err)
			}
		})
	}
}

func TestSeasonBracketRefusesATournamentNotShownAsABracket(t *testing.T) {
	for _, tt := range []struct{ name, set string }{
		{"not marked as a bracket", "bracket = false"},
		{"a blitz tournament, whose knockout is not read here", "bracket = true, blitz = true"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			ctx := context.Background()
			run := publishedRun(t, pool)
			placedOn(t, pool, map[string]time.Time{"Wimbledon 2024 Stage 1 - group 1 game 1": day("2024-07-01")})
			withSpans(t, pool, run)
			if _, err := pool.Exec(ctx, "UPDATE series SET "+tt.set+" WHERE slug = 'wimbledon'"); err != nil {
				t.Fatal(err)
			}
			r := NewRatingsReader(asReader(t, pool))
			if _, err := r.SeasonBracket(ctx, run, "wimbledon", "2024"); !errors.Is(err, ErrTournamentNotFound) {
				t.Errorf("SeasonBracket = %v, want ErrTournamentNotFound", err)
			}
			got, err := r.CatalogueTournament(ctx, run, "wimbledon")
			if err != nil || got.Bracket {
				t.Errorf("CatalogueTournament(wimbledon).Bracket = %v, %v; want false", got.Bracket, err)
			}
			view, err := r.Catalogue(ctx, run)
			if err != nil || len(view.Tournaments) != 1 || view.Tournaments[0].Bracket {
				t.Errorf("Catalogue's wimbledon = %+v, %v; want it, not a bracket", view.Tournaments, err)
			}
		})
	}
}

func TestCatalogueTournamentSaysItIsShownAsABracket(t *testing.T) {
	run, r := bracketFixture(t)
	got, err := r.CatalogueTournament(context.Background(), run, "wimbledon")
	if err != nil || !got.Bracket {
		t.Errorf("CatalogueTournament(wimbledon).Bracket = %v, %v; want true", got.Bracket, err)
	}
}
