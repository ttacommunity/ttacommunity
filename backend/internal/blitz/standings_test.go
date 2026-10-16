package blitz_test

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"ttacommunity/backend/internal/blitz"
)

func in(ids ...int64) []blitz.Entrant {
	var out []blitz.Entrant
	for _, id := range ids {
		out = append(out, blitz.Entrant{ID: id})
	}
	return out
}

func game(round int, a, b int64, r blitz.Result) blitz.Played {
	return blitz.Played{Round: round, A: a, B: b, Result: r}
}

func forfeit(round int, a, b int64, r blitz.Result) blitz.Played {
	return blitz.Played{Round: round, A: a, B: b, Result: r, Forfeit: true}
}

func bye(round int, a int64) blitz.Played {
	return blitz.Played{Round: round, A: a}
}

// order is the ids of the standings in ranked order, only those in ids.
func order(s []blitz.Standing, ids ...int64) []int64 {
	var out []int64
	for _, x := range s {
		if slices.Contains(ids, x.ID) {
			out = append(out, x.ID)
		}
	}
	return out
}

func TestStandingsScore(t *testing.T) {
	tests := []struct {
		name    string
		matches []blitz.Played
		want    map[int64]string
	}{
		{"a win is 1, a draw a half, a bye 1 and no game, an undecided match nothing",
			[]blitz.Played{game(1, 1, 2, blitz.WinA), game(1, 3, 4, blitz.Draw), bye(1, 5), game(2, 2, 4, blitz.Draw), game(2, 1, 3, blitz.Undecided)},
			map[int64]string{1: "1 1 0", 2: "0.5 2 0", 3: "0.5 1 0", 4: "1 2 0", 5: "1 0 1"}},
		{"a double loss gives neither a point",
			[]blitz.Played{game(1, 1, 2, blitz.DoubleLoss), game(2, 3, 1, blitz.WinA)},
			map[int64]string{1: "0 2 0", 2: "0 1 0", 3: "1 1 0", 4: "0 0 0", 5: "0 0 0"}},
		{"a forfeit scores as ruled but is no game played",
			[]blitz.Played{forfeit(1, 1, 2, blitz.WinA), forfeit(1, 3, 4, blitz.DoubleLoss), game(2, 1, 3, blitz.Draw)},
			map[int64]string{1: "1.5 1 0", 2: "0 0 0", 3: "0.5 1 0", 4: "0 0 0", 5: "0 0 0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := map[int64]string{}
			for _, s := range blitz.Standings(in(1, 2, 3, 4, 5), tt.matches, 8, 1, blitz.FIDEScoring) {
				got[s.ID] = fmt.Sprintf("%g %d %d", s.Points, s.Games, s.Byes)
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("points, games and byes = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuchholz(t *testing.T) {
	tests := []struct {
		name     string
		entrants []blitz.Entrant
		matches  []blitz.Played
		rounds   int
		of       int64
		want     float64
	}{
		{"a game adds the opponent's score",
			in(1, 2, 3, 4),
			[]blitz.Played{game(1, 1, 2, blitz.WinA), game(1, 3, 4, blitz.WinA), game(2, 1, 3, blitz.WinA), game(2, 2, 4, blitz.WinA)},
			4, 1, 2},
		{"a bye counts as a game against a dummy with the player's own score",
			in(1, 2, 3),
			[]blitz.Played{game(1, 1, 2, blitz.WinA), bye(1, 3), game(2, 3, 1, blitz.WinA), bye(2, 2)},
			4, 3, 3},
		{"the dummy scores no more than a draw in every round of the tournament",
			in(1, 2, 3),
			[]blitz.Played{game(1, 1, 2, blitz.WinA), bye(1, 3), game(2, 3, 1, blitz.WinA), bye(2, 2)},
			2, 3, 2},
		{"a forfeit counts as a dummy with the player's own score, no more than the opponent's",
			in(1, 2, 3, 4),
			[]blitz.Played{
				forfeit(1, 1, 2, blitz.WinA), game(1, 3, 4, blitz.WinA),
				game(2, 2, 3, blitz.WinA), game(2, 4, 1, blitz.WinA),
				game(3, 2, 4, blitz.WinA), game(3, 3, 1, blitz.WinA),
			},
			4, 1, 4},
		{"an opponent's rounds after they drop count as draws",
			[]blitz.Entrant{{ID: 1}, {ID: 2, Dropped: true}, {ID: 3}, {ID: 4}},
			[]blitz.Played{game(1, 1, 2, blitz.WinA), game(1, 3, 4, blitz.WinA), game(2, 1, 3, blitz.WinA), bye(2, 4)},
			4, 1, 1.5},
		{"an opponent's missed round counts as the zero it scored once they play again",
			in(1, 2, 3),
			[]blitz.Played{game(1, 1, 2, blitz.WinA), game(2, 3, 1, blitz.WinA), bye(2, 2)},
			4, 1, 2},
		{"an opponent's missed round followed only by forfeit losses counts as a draw",
			in(1, 2, 3, 4),
			[]blitz.Played{
				game(1, 1, 2, blitz.WinA), game(1, 3, 4, blitz.WinA),
				game(2, 1, 3, blitz.WinA), bye(2, 4),
				forfeit(3, 3, 2, blitz.WinA), game(3, 1, 4, blitz.WinA),
			},
			4, 1, 3.5},
		{"a missed round counts as a dummy with the player's own score",
			in(1, 2, 3),
			[]blitz.Played{game(1, 1, 2, blitz.WinA), game(2, 3, 1, blitz.WinA), bye(2, 2)},
			4, 3, 2},
		{"an undecided match adds nothing",
			in(1, 2, 3),
			[]blitz.Played{game(1, 1, 2, blitz.WinA), bye(1, 3), game(2, 1, 3, blitz.Undecided), bye(2, 2)},
			4, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, s := range blitz.Standings(tt.entrants, tt.matches, tt.rounds, 1, blitz.FIDEScoring) {
				if s.ID == tt.of && s.Buchholz != tt.want {
					t.Errorf("Buchholz of %d = %g, want %g", tt.of, s.Buchholz, tt.want)
				}
			}
		})
	}
}

func TestDirectEncounter(t *testing.T) {
	tests := []struct {
		name    string
		players int64
		matches []blitz.Played
		tied    []int64
		want    []int64
	}{
		{"of two tied players who met, the winner goes first, whatever their Buchholz",
			6,
			[]blitz.Played{
				game(1, 1, 2, blitz.WinA), game(1, 3, 4, blitz.Draw), game(1, 5, 6, blitz.WinA),
				game(2, 3, 1, blitz.WinA), game(2, 2, 5, blitz.WinA), game(2, 6, 4, blitz.WinA),
				game(3, 4, 1, blitz.WinA), game(3, 6, 2, blitz.WinA), game(3, 5, 3, blitz.WinA),
			},
			[]int64{1, 2}, []int64{1, 2}},
		{"a forfeit is no encounter, so Buchholz decides",
			6,
			[]blitz.Played{
				forfeit(1, 1, 2, blitz.WinA), game(1, 3, 4, blitz.Draw), game(1, 5, 6, blitz.WinA),
				game(2, 3, 1, blitz.WinA), game(2, 2, 5, blitz.WinA), game(2, 6, 4, blitz.WinA),
				game(3, 4, 1, blitz.WinA), game(3, 6, 2, blitz.WinA), game(3, 5, 3, blitz.WinA),
			},
			[]int64{1, 2}, []int64{2, 1}},
		{"players who have all met are ranked by the games among them, then again among those still tied",
			8,
			[]blitz.Played{
				game(1, 1, 2, blitz.WinA), game(1, 4, 3, blitz.WinA), game(1, 5, 6, blitz.WinA), game(1, 7, 8, blitz.WinA),
				game(2, 3, 1, blitz.WinA), game(2, 2, 4, blitz.WinA), game(2, 5, 7, blitz.WinA), game(2, 6, 8, blitz.WinA),
				game(3, 1, 4, blitz.WinA), game(3, 2, 3, blitz.WinA), game(3, 5, 8, blitz.WinA), game(3, 6, 7, blitz.WinA),
				game(4, 5, 1, blitz.WinA), game(4, 6, 2, blitz.WinA), game(4, 3, 7, blitz.WinA), game(4, 4, 8, blitz.WinA),
			},
			[]int64{1, 2, 3, 4}, []int64{1, 2, 4, 3}},
		{"when not all have met, one sure to top the games among them whatever the missing ones goes first",
			8,
			[]blitz.Played{
				game(1, 1, 2, blitz.WinA), game(1, 3, 4, blitz.WinA), game(1, 6, 5, blitz.WinA), game(1, 7, 8, blitz.WinA),
				game(2, 1, 3, blitz.WinA), game(2, 2, 5, blitz.WinA), game(2, 7, 4, blitz.WinA), game(2, 6, 8, blitz.WinA),
				game(3, 4, 1, blitz.WinA), game(3, 2, 6, blitz.WinA), game(3, 3, 8, blitz.WinA), game(3, 7, 5, blitz.WinA),
				game(4, 5, 1, blitz.WinA), game(4, 7, 2, blitz.WinA), game(4, 6, 3, blitz.WinA), game(4, 8, 4, blitz.WinA),
			},
			[]int64{1, 2, 3}, []int64{1, 2, 3}},
		{"when not all have met and nobody is sure of the top, Buchholz decides",
			6,
			[]blitz.Played{
				game(1, 1, 3, blitz.WinA), game(1, 2, 4, blitz.WinA), game(1, 5, 6, blitz.WinA),
				game(2, 3, 4, blitz.WinA), game(2, 5, 6, blitz.Draw), game(2, 1, 2, blitz.Draw),
			},
			[]int64{1, 2, 5}, []int64{1, 2, 5}},
		{"a pair who met more than once count the average of their games",
			8,
			[]blitz.Played{
				game(1, 1, 2, blitz.WinA), game(1, 3, 4, blitz.WinA), game(1, 5, 6, blitz.WinA), game(1, 7, 8, blitz.WinA),
				game(2, 3, 1, blitz.WinA), game(2, 2, 5, blitz.WinA), game(2, 4, 6, blitz.Draw), game(2, 7, 8, blitz.WinA),
				game(3, 2, 3, blitz.WinA), game(3, 6, 1, blitz.WinA), game(3, 7, 4, blitz.WinA), game(3, 5, 8, blitz.WinA),
				game(4, 1, 2, blitz.WinA), game(4, 7, 3, blitz.WinA), game(4, 5, 4, blitz.WinA), game(4, 8, 6, blitz.WinA),
			},
			[]int64{1, 2, 3}, []int64{2, 3, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entrants []blitz.Entrant
			for id := range tt.players {
				entrants = append(entrants, blitz.Entrant{ID: id + 1})
			}
			if got := order(blitz.Standings(entrants, tt.matches, 8, 1, blitz.FIDEScoring), tt.tied...); !slices.Equal(got, tt.want) {
				t.Errorf("order of %v = %v, want %v", tt.tied, got, tt.want)
			}
		})
	}
}

func TestStandingsPutDroppedPlayersLastWithTheirPoints(t *testing.T) {
	entrants := []blitz.Entrant{{ID: 1}, {ID: 2, Dropped: true}, {ID: 3}}
	got := blitz.Standings(entrants, []blitz.Played{game(1, 2, 1, blitz.WinA), bye(1, 3), game(2, 1, 3, blitz.WinA)}, 8, 1, blitz.FIDEScoring)
	if last := got[len(got)-1]; last.ID != 2 || !last.Dropped || last.Points != 1 {
		t.Errorf("last standing = %+v, want 2, dropped, on 1 point", last)
	}
	if len(blitz.Standings(nil, nil, 8, 1, blitz.FIDEScoring)) != 0 {
		t.Error("Standings of nobody is not empty")
	}
}

func TestStandingsBreakWhatIsLeftByTheSeed(t *testing.T) {
	entrants := in(1, 2, 3, 4, 5, 6, 7, 8)
	ids := func(seed uint64) []int64 {
		var out []int64
		for _, s := range blitz.Standings(entrants, nil, 8, seed, blitz.FIDEScoring) {
			out = append(out, s.ID)
		}
		return out
	}
	first := ids(1)
	if !slices.Equal(first, ids(1)) {
		t.Errorf("the same seed twice gave %v then %v", first, ids(1))
	}
	if slices.IsSorted(first) {
		t.Errorf("seed 1 left the ids in their given order %v, want a draw", first)
	}
	if other := ids(2); slices.Equal(first, other) {
		t.Errorf("seeds 1 and 2 gave the same order %v", first)
	}
	for i, s := range blitz.Standings(entrants, nil, 8, 1, blitz.FIDEScoring) {
		if s.Rank != i+1 {
			t.Errorf("standing %d has rank %d, want %d: a tie the draw broke reads like any other rank", i, s.Rank, i+1)
		}
	}
}

// edition is an edition's own scoring, every value unlike FIDE's.
var edition = blitz.Scoring{Win: 3, Draw: 1, Loss: 0.5, ForfeitWin: 2, ForfeitLoss: 0, Bye: 1.5, Skip: 0}

func TestStandingsScoreByTheEditionsPoints(t *testing.T) {
	tests := []struct {
		name    string
		matches []blitz.Played
		want    map[int64]float64
	}{
		{"a win, a loss and a draw", []blitz.Played{game(1, 1, 2, blitz.WinA), game(1, 3, 4, blitz.Draw)}, map[int64]float64{1: 3, 2: 0.5, 3: 1, 4: 1}},
		{"a bye", []blitz.Played{bye(1, 5)}, map[int64]float64{5: 1.5}},
		{"a forfeit, and a ruled draw", []blitz.Played{forfeit(1, 1, 2, blitz.WinB), forfeit(1, 3, 4, blitz.Draw)}, map[int64]float64{1: 0, 2: 2, 3: 1, 4: 1}},
		{"a double loss, played or ruled, a forfeit loss each", []blitz.Played{game(1, 1, 2, blitz.DoubleLoss), forfeit(1, 3, 4, blitz.DoubleLoss)},
			map[int64]float64{1: 0, 2: 0, 3: 0, 4: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, s := range blitz.Standings(in(1, 2, 3, 4, 5), tt.matches, 8, 1, edition) {
				if want, ok := tt.want[s.ID]; ok && s.Points != want {
					t.Errorf("player %d: points = %v, want %v", s.ID, s.Points, want)
				}
			}
		})
	}
}

func TestBuchholzUsesTheEditionsPoints(t *testing.T) {
	forfeitHalf := blitz.FIDEScoring
	forfeitHalf.ForfeitLoss = 0.5
	tests := []struct {
		name    string
		scoring blitz.Scoring
		rounds  int
		matches []blitz.Played
		want    map[int64]float64
	}{
		// 2 drops after losing to 1: 2's missed round 2 counts as the edition's draw for 1, and 1's bye as a draw
		// in every round at most.
		{"an unplayed round is the edition's draw", edition, 2, []blitz.Played{game(1, 1, 2, blitz.WinA), bye(2, 1)}, map[int64]float64{1: 3.5, 2: 5}},
		// 2 misses round 2, then loses round 3 by forfeit: the missed round still counts as a draw for 3.
		{"a forfeit loss worth points is still a forfeit loss", forfeitHalf, 3,
			[]blitz.Played{game(1, 2, 3, blitz.WinA), game(2, 1, 3, blitz.Draw), forfeit(3, 3, 2, blitz.WinA)}, map[int64]float64{3: 4.5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, s := range blitz.Standings(in(1, 2, 3), tt.matches, tt.rounds, 1, tt.scoring) {
				if want, ok := tt.want[s.ID]; ok && s.Buchholz != want {
					t.Errorf("player %d: Buchholz = %v, want %v", s.ID, s.Buchholz, want)
				}
			}
		})
	}
}

func TestCheckScoringRefusesPointsOutOfRangeOrOrder(t *testing.T) {
	with := func(change func(*blitz.Scoring)) blitz.Scoring {
		s := blitz.FIDEScoring
		change(&s)
		return s
	}
	tests := []struct {
		name      string
		scoring   blitz.Scoring
		wantError string
	}{
		{"FIDE's", blitz.FIDEScoring, ""},
		{"nothing worth anything", blitz.Scoring{}, ""},
		{"ten for everything", blitz.Scoring{Win: 10, Draw: 10, Loss: 10, ForfeitWin: 10, ForfeitLoss: 10, Bye: 10, Skip: 10}, ""},
		{"halves", with(func(s *blitz.Scoring) { s.Win, s.Draw, s.Bye = 2.5, 1.5, 2.5 }), ""},
		{"a forfeit loss above a forfeit win", with(func(s *blitz.Scoring) { s.ForfeitLoss = 1.5 }), ""},
		{"a win over 10", with(func(s *blitz.Scoring) { s.Win = 10.5 }), "0 to 10"},
		{"a skip over 10", with(func(s *blitz.Scoring) { s.Skip = 11 }), "0 to 10"},
		{"a loss below 0", with(func(s *blitz.Scoring) { s.Loss = -0.5 }), "0 to 10"},
		{"a quarter", with(func(s *blitz.Scoring) { s.Bye = 0.25 }), "halves"},
		{"a draw above a win", with(func(s *blitz.Scoring) { s.Draw = 1.5 }), "at least"},
		{"a loss above a draw", with(func(s *blitz.Scoring) { s.Loss = 1 }), "at least"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := blitz.CheckScoring(tt.scoring)
			switch {
			case tt.wantError == "" && err != nil:
				t.Errorf("CheckScoring(%+v) = %v, want nil", tt.scoring, err)
			case tt.wantError != "" && (err == nil || !strings.Contains(err.Error(), tt.wantError)):
				t.Errorf("CheckScoring(%+v) = %v, want an error naming %q", tt.scoring, err, tt.wantError)
			}
		})
	}
}
