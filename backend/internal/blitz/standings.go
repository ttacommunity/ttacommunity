// The standings of a Swiss tournament: points from results and byes, then the tie-breaks of FIDE's Swiss rules
// (Direct Encounter, then Buchholz with its handling of unplayed rounds), then a draw from a seed.
package blitz

import (
	"cmp"
	"maps"
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

// Played is a match as the standings see it: its round, who met whom, how it ended, and whether it was a forfeit
// (a result with no game played); B is 0 for a bye.
type Played struct {
	Round   int
	A, B    int64
	Result  Result
	Forfeit bool
}

// Standing is one row: the rank, points, games played (forfeits and byes aside), byes, Buchholz, and dropped.
type Standing struct {
	ID          int64
	Rank        int
	Points      float64
	Games, Byes int
	Buchholz    float64
	Dropped     bool
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

// Standings ranks the entrants by points (a win 1, a draw a half, a bye 1), then by the tie-breaks of FIDE's
// regulations for Swiss tournaments: Direct Encounter, then Buchholz, with unplayed rounds as its Article 16 has
// them (rounds is the tournament's number of Swiss rounds); then by a draw from the seed, the same every time.
// Dropped players come last, their points kept. Ranks run 1, 2, 3…: a tie the draw broke reads like any other.
func Standings(entrants []Entrant, matches []Played, rounds int, seed uint64) []Standing {
	rows := map[int64]*Standing{}
	for _, e := range entrants {
		rows[e.ID] = &Standing{ID: e.ID, Dropped: e.Dropped}
	}
	paired := map[int64]map[int]bool{}
	lostByForfeit := map[int64]map[int]bool{}
	for id := range rows {
		paired[id], lostByForfeit[id] = map[int]bool{}, map[int]bool{}
	}
	var played []int
	encounters := map[[2]int64][]float64{}
	for _, m := range matches {
		if !slices.Contains(played, m.Round) {
			played = append(played, m.Round)
		}
		a, b := rows[m.A], rows[m.B]
		if m.B == 0 && a != nil {
			a.Points++
			a.Byes++
			paired[m.A][m.Round] = true
			continue
		}
		if a == nil || b == nil {
			continue
		}
		paired[m.A][m.Round], paired[m.B][m.Round] = true, true
		if m.Result == Undecided {
			continue
		}
		pa, pb := points(m.Result)
		a.Points += pa
		b.Points += pb
		if m.Forfeit {
			lostByForfeit[m.A][m.Round], lostByForfeit[m.B][m.Round] = pa == 0, pb == 0
			continue
		}
		a.Games++
		b.Games++
		encounters[[2]int64{m.A, m.B}] = append(encounters[[2]int64{m.A, m.B}], pa)
		encounters[[2]int64{m.B, m.A}] = append(encounters[[2]int64{m.B, m.A}], pb)
	}
	slices.Sort(played)

	// An unpaired round is a zero-point bye. For the opponents' Buchholz it counts as a draw when only more such
	// rounds or forfeit losses follow it (as after dropping out), else as the zero it scored.
	missed, adjusted := map[int64]int{}, map[int64]float64{}
	for id, row := range rows {
		adjusted[id] = row.Points
		onlyUnplayedAfter := true
		for _, r := range slices.Backward(played) {
			switch {
			case !paired[id][r]:
				missed[id]++
				if onlyUnplayedAfter {
					adjusted[id] += 0.5
				}
			case !lostByForfeit[id][r]:
				onlyUnplayedAfter = false
			}
		}
	}

	// Buchholz: each opponent's adjusted score; an unplayed round counts as a dummy with the player's own score, at
	// most the opponent's adjusted score for a forfeit, and a draw in every round for a bye or missed round.
	most := 0.5 * float64(rounds)
	for _, m := range matches {
		a, b := rows[m.A], rows[m.B]
		switch {
		case a == nil:
		case m.B == 0:
			a.Buchholz += min(a.Points, most)
		case b == nil || m.Result == Undecided:
		case m.Forfeit:
			a.Buchholz += min(a.Points, adjusted[m.B])
			b.Buchholz += min(b.Points, adjusted[m.A])
		default:
			a.Buchholz += adjusted[m.B]
			b.Buchholz += adjusted[m.A]
		}
	}
	for id, row := range rows {
		row.Buchholz += float64(missed[id]) * min(row.Points, most)
	}

	among := func(a, b int64) (float64, bool) {
		scores := encounters[[2]int64{a, b}]
		if len(scores) == 0 {
			return 0, false
		}
		var sum float64
		for _, s := range scores {
			sum += s
		}
		return sum / float64(len(scores)), true
	}
	draw := func(id int64) float64 {
		return rand.New(rand.NewPCG(seed, uint64(id))).Float64()
	}
	type tie struct {
		dropped bool
		points  float64
	}
	groups := map[tie][]int64{}
	for id, row := range rows {
		key := tie{row.Dropped, row.Points}
		groups[key] = append(groups[key], id)
	}
	keys := slices.SortedFunc(maps.Keys(groups), func(x, y tie) int {
		if x.dropped != y.dropped {
			if x.dropped {
				return 1
			}
			return -1
		}
		return cmp.Compare(y.points, x.points)
	})
	out := make([]Standing, 0, len(rows))
	for _, k := range keys {
		group := groups[k]
		slices.Sort(group)
		for _, tied := range directEncounter(group, among) {
			slices.SortFunc(tied, func(x, y int64) int {
				if c := cmp.Compare(rows[y].Buchholz, rows[x].Buchholz); c != 0 {
					return c
				}
				return cmp.Compare(draw(x), draw(y))
			})
			for _, id := range tied {
				out = append(out, *rows[id])
			}
		}
	}
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}

// directEncounter orders a group tied on points by FIDE's Direct Encounter (Article 6), from among(a, b): a's
// average score in the games a played against b, and whether they played one. It returns the group as subgroups
// in order, each one still tied.
func directEncounter(group []int64, among func(a, b int64) (float64, bool)) [][]int64 {
	if len(group) < 2 {
		return [][]int64{group}
	}
	score, missing := map[int64]float64{}, map[int64]float64{}
	for i, a := range group {
		for _, b := range group[i+1:] {
			sa, met := among(a, b)
			if !met {
				missing[a]++
				missing[b]++
				continue
			}
			sb, _ := among(b, a)
			score[a] += sa
			score[b] += sb
		}
	}
	var out [][]int64
	if len(missing) == 0 {
		// All have met: the scores rank them, and each set still tied is ranked again by the games within it.
		sorted := slices.Clone(group)
		slices.SortStableFunc(sorted, func(a, b int64) int { return cmp.Compare(score[b], score[a]) })
		for start := 0; start < len(sorted); {
			end := start + 1
			for end < len(sorted) && score[sorted[end]] == score[sorted[start]] {
				end++
			}
			if end-start == len(group) {
				return [][]int64{group}
			}
			out = append(out, directEncounter(sorted[start:end], among)...)
			start = end
		}
		return out
	}
	// Not all have met: whoever is alone at the top whatever the missing games end as goes first, then the next;
	// the rest are ranked again by the games among them alone.
	rest := slices.Clone(group)
	for {
		i := slices.IndexFunc(rest, func(a int64) bool {
			return !slices.ContainsFunc(rest, func(b int64) bool { return b != a && score[a] <= score[b]+missing[b] })
		})
		if i < 0 {
			break
		}
		out = append(out, []int64{rest[i]})
		rest = slices.Delete(rest, i, i+1)
	}
	if len(rest) == len(group) {
		return [][]int64{group}
	}
	if len(rest) > 0 {
		out = append(out, directEncounter(rest, among)...)
	}
	return out
}
