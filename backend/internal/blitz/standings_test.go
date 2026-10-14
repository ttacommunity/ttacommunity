package blitz_test

import (
	"fmt"
	"maps"
	"slices"
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
			for _, s := range blitz.Standings(in(1, 2, 3, 4, 5), tt.matches, 8, 1) {
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
			for _, s := range blitz.Standings(tt.entrants, tt.matches, tt.rounds, 1) {
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
			if got := order(blitz.Standings(entrants, tt.matches, 8, 1), tt.tied...); !slices.Equal(got, tt.want) {
				t.Errorf("order of %v = %v, want %v", tt.tied, got, tt.want)
			}
		})
	}
}

func TestStandingsPutDroppedPlayersLastWithTheirPoints(t *testing.T) {
	entrants := []blitz.Entrant{{ID: 1}, {ID: 2, Dropped: true}, {ID: 3}}
	got := blitz.Standings(entrants, []blitz.Played{game(1, 2, 1, blitz.WinA), bye(1, 3), game(2, 1, 3, blitz.WinA)}, 8, 1)
	if last := got[len(got)-1]; last.ID != 2 || !last.Dropped || last.Points != 1 {
		t.Errorf("last standing = %+v, want 2, dropped, on 1 point", last)
	}
	if len(blitz.Standings(nil, nil, 8, 1)) != 0 {
		t.Error("Standings of nobody is not empty")
	}
}

func TestStandingsBreakWhatIsLeftByTheSeed(t *testing.T) {
	entrants := in(1, 2, 3, 4, 5, 6, 7, 8)
	ids := func(seed uint64) []int64 {
		var out []int64
		for _, s := range blitz.Standings(entrants, nil, 8, seed) {
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
	for i, s := range blitz.Standings(entrants, nil, 8, 1) {
		if s.Rank != i+1 {
			t.Errorf("standing %d has rank %d, want %d: a tie the draw broke reads like any other rank", i, s.Rank, i+1)
		}
	}
}
