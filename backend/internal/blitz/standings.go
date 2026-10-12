// The standings of a Swiss tournament: points from results and byes, and CGE's tiebreaks (the games among the
// tied when they have all met, then opponents' points, then a draw from a seed).
package blitz

import (
	"math/rand/v2"
	"slices"
)

// Result is how a match ended: a win for either side, a draw, a double loss, or nothing yet.
type Result string

const (
	Undecided  Result = ""
	WinA       Result = "a"
	WinB       Result = "b"
	Draw       Result = "draw"
	DoubleLoss Result = "double_loss"
)

// Results are the results a match can have.
var Results = []Result{WinA, WinB, Draw, DoubleLoss}

// Entrant is who the standings rank, and whether they have dropped out.
type Entrant struct {
	ID      int64
	Dropped bool
}

// Played is a match as the standings see it: who met whom and how it ended; B is 0 for a bye.
type Played struct {
	A, B   int64
	Result Result
}

// Standing is one row: the rank, points, games played, byes, the sum of the opponents' points, and dropped.
type Standing struct {
	ID              int64
	Rank            int
	Points          float64
	Games, Byes     int
	OpponentsPoints float64
	Dropped         bool
}

// points is what each side of a result takes: win 1, draw a half, loss or double loss 0.
func points(r Result) (a, b float64) {
	switch r {
	case WinA:
		return 1, 0
	case WinB:
		return 0, 1
	case Draw:
		return 0.5, 0.5
	}
	return 0, 0
}

// Standings ranks the entrants: by points (a win 1, a draw a half, a bye 1), then, among players on equal points
// who have all met, by the points scored among them; then by their opponents' points; then by a draw from the
// seed, the same every time. Dropped players come last, their points kept. Ranks run 1, 2, 3…: a tie the draw
// broke reads like any other.
func Standings(entrants []Entrant, matches []Played, seed uint64) []Standing {
	rows := map[int64]*Standing{}
	for _, e := range entrants {
		rows[e.ID] = &Standing{ID: e.ID, Dropped: e.Dropped}
	}
	against := map[int64][]int64{}
	for _, m := range matches {
		a, b := rows[m.A], rows[m.B]
		if m.B == 0 && a != nil {
			a.Points++
			a.Byes++
			continue
		}
		if m.Result == Undecided || a == nil || b == nil {
			continue
		}
		pa, pb := points(m.Result)
		a.Points += pa
		b.Points += pb
		a.Games++
		b.Games++
		against[m.A] = append(against[m.A], m.B)
		against[m.B] = append(against[m.B], m.A)
	}
	for id, row := range rows {
		for _, o := range against[id] {
			row.OpponentsPoints += rows[o].Points
		}
	}

	// A player's share of the points among those tied with them, when the tied players have all met.
	among := map[int64]float64{}
	byPoints := map[[2]any][]int64{}
	for id, row := range rows {
		key := [2]any{row.Dropped, row.Points}
		byPoints[key] = append(byPoints[key], id)
	}
	for _, group := range byPoints {
		if len(group) < 2 || !allMet(group, against) {
			continue
		}
		in := map[int64]bool{}
		for _, id := range group {
			in[id] = true
		}
		for _, m := range matches {
			if m.Result == Undecided || !in[m.A] || !in[m.B] {
				continue
			}
			pa, pb := points(m.Result)
			among[m.A] += pa
			among[m.B] += pb
		}
	}

	draw := func(id int64) float64 {
		return rand.New(rand.NewPCG(seed, uint64(id))).Float64()
	}
	out := make([]Standing, 0, len(rows))
	for _, row := range rows {
		out = append(out, *row)
	}
	slices.SortFunc(out, func(a, b Standing) int {
		if a.Dropped != b.Dropped {
			if a.Dropped {
				return 1
			}
			return -1
		}
		for _, d := range []float64{b.Points - a.Points, among[b.ID] - among[a.ID], b.OpponentsPoints - a.OpponentsPoints, draw(a.ID) - draw(b.ID)} {
			if d < 0 {
				return -1
			}
			if d > 0 {
				return 1
			}
		}
		return 0
	})
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}

// allMet reports whether every pair in the group has a decided match between them.
func allMet(group []int64, against map[int64][]int64) bool {
	for i, a := range group {
		for _, b := range group[i+1:] {
			if !slices.Contains(against[a], b) {
				return false
			}
		}
	}
	return true
}
