package blitz_test

import (
	"fmt"
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

func played(a, b int64, r blitz.Result) blitz.Played {
	return blitz.Played{A: a, B: b, Result: r}
}

// lines writes each standing as "rank:id points games byes opp" (dropped ones with a trailing "dropped").
func lines(s []blitz.Standing) []string {
	var out []string
	for _, x := range s {
		line := fmt.Sprintf("%d:%d %g %d %d %g", x.Rank, x.ID, x.Points, x.Games, x.Byes, x.OpponentsPoints)
		if x.Dropped {
			line += " dropped"
		}
		out = append(out, line)
	}
	return out
}

func TestStandingsScoreAndBreakTies(t *testing.T) {
	tests := []struct {
		name     string
		entrants []blitz.Entrant
		matches  []blitz.Played
		want     []string
	}{
		{"a win is 1, a draw a half, a bye 1 and no game, an undecided match nothing",
			in(1, 2, 3, 4, 5),
			[]blitz.Played{played(1, 2, blitz.WinA), played(3, 4, blitz.Draw), played(5, 0, blitz.Undecided), played(2, 4, blitz.Draw), played(1, 3, blitz.Undecided)},
			[]string{"1:4 1 2 0 1", "2:1 1 1 0 0.5", "3:5 1 0 1 0", "4:2 0.5 2 0 2", "5:3 0.5 1 0 1"}},
		{"a double loss gives neither a point",
			in(1, 2, 3),
			[]blitz.Played{played(1, 2, blitz.DoubleLoss), played(3, 1, blitz.WinA)},
			[]string{"1:3 1 1 0 0", "2:1 0 2 0 1", "3:2 0 1 0 0"}},
		{"tied players who have all met are ordered by the points among them",
			in(1, 2, 3, 4),
			[]blitz.Played{
				played(1, 2, blitz.WinA), played(2, 3, blitz.WinA), played(3, 1, blitz.WinA), // 1, 2 and 3 each beat one of the others
				played(1, 4, blitz.WinA), played(2, 4, blitz.WinB), played(3, 4, blitz.WinA), // 4 beat 2 only
				played(1, 2, blitz.WinB), // 2 beats 1 again: 2 leads the three on the games among them
			},
			[]string{"1:2 2 4 0 7", "2:1 2 4 0 7", "3:3 2 3 0 5", "4:4 1 3 0 6"}},
		{"tied players who have not all met are ordered by their opponents' points",
			in(1, 2, 3, 4, 5, 6),
			[]blitz.Played{
				played(1, 3, blitz.WinA), played(2, 4, blitz.WinA), played(5, 6, blitz.WinA), // 1, 2 and 5 win round 1
				played(3, 4, blitz.WinA), played(5, 6, blitz.Draw), played(1, 2, blitz.Draw), // 3 wins round 2, the rest draw
			},
			[]string{"1:1 1.5 2 0 2.5", "2:2 1.5 2 0 1.5", "3:5 1.5 2 0 1", "4:3 1 2 0 1.5", "5:6 0.5 2 0 3", "6:4 0 2 0 2.5"}},
		{"dropped players come last, their points kept",
			[]blitz.Entrant{{ID: 1}, {ID: 2, Dropped: true}, {ID: 3}},
			[]blitz.Played{played(2, 1, blitz.WinA), played(3, 1, blitz.WinB)},
			[]string{"1:1 1 2 0 1", "2:3 0 1 0 1", "3:2 1 1 0 1 dropped"}},
		{"nobody in, nothing out", nil, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lines(blitz.Standings(tt.entrants, tt.matches, 1))
			if !slices.Equal(got, tt.want) {
				t.Errorf("Standings = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStandingsBreakWhatIsLeftByTheSeed(t *testing.T) {
	entrants := in(1, 2, 3, 4, 5, 6, 7, 8)
	order := func(seed uint64) []int64 {
		var ids []int64
		for _, s := range blitz.Standings(entrants, nil, seed) {
			ids = append(ids, s.ID)
		}
		return ids
	}
	first := order(1)
	if !slices.Equal(first, order(1)) {
		t.Errorf("the same seed twice gave %v then %v", first, order(1))
	}
	if slices.IsSorted(first) {
		t.Errorf("seed 1 left the ids in their given order %v, want a draw", first)
	}
	if other := order(2); slices.Equal(first, other) {
		t.Errorf("seeds 1 and 2 gave the same order %v", first)
	}
	for i, s := range blitz.Standings(entrants, nil, 1) {
		if s.Rank != i+1 {
			t.Errorf("standing %d has rank %d, want %d: a tie the draw broke reads like any other rank", i, s.Rank, i+1)
		}
	}
}
