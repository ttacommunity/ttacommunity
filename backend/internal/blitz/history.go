// What pairing the next Swiss round needs to know of each entrant's rounds so far: their floats, whether they had a
// bye, and the rounds they did not play.
package blitz

import (
	"maps"
	"slices"

	"ttacommunity/backend/internal/pairing"
)

// History is an entrant's Swiss rounds as the pairing sees them: their floats in the latest round (Floats[0]) and
// the one before (FIDE's C.04.3, 1.4), whether they had the bye or won as much as a win without playing (C.04.1,
// 4), and how many rounds they did not play.
type History struct {
	Floats   [2]pairing.Float
	HadBye   bool
	Unplayed int
}

// Histories reads each entrant's History from the matches of the Swiss rounds numbered in rounds and their skips,
// as the scoring values them. Matches of other rounds are left out.
func Histories(entrants []Entrant, matches []Played, rounds []int, s Scoring) map[int64]History {
	out := map[int64]History{}
	points := map[int64]float64{}
	for _, n := range slices.Sorted(slices.Values(rounds)) {
		before := maps.Clone(points)
		floats := map[int64]pairing.Float{}
		unplayed := func(id int64, got float64) {
			h := out[id]
			h.Unplayed++
			h.HadBye = h.HadBye || got >= s.Win
			out[id] = h
			points[id] += got
			if got > s.Loss {
				floats[id] = pairing.Down
			}
		}
		for _, m := range matches {
			switch {
			case m.Round != n:
			case m.B == 0:
				unplayed(m.A, s.Bye)
				h := out[m.A]
				h.HadBye, floats[m.A] = true, pairing.Down
				out[m.A] = h
			case m.Forfeit:
				pa, pb, _, _ := s.points(m)
				unplayed(m.A, pa)
				unplayed(m.B, pb)
			default:
				pa, pb, _, _ := s.points(m)
				points[m.A] += pa
				points[m.B] += pb
				if before[m.A] > before[m.B] {
					floats[m.A], floats[m.B] = pairing.Down, pairing.Up
				} else if before[m.A] < before[m.B] {
					floats[m.A], floats[m.B] = pairing.Up, pairing.Down
				}
			}
		}
		for _, e := range entrants {
			if slices.Contains(e.Skipped, n) {
				unplayed(e.ID, s.Skip)
			}
		}
		for _, e := range entrants {
			h := out[e.ID]
			h.Floats = [2]pairing.Float{floats[e.ID], h.Floats[0]}
			out[e.ID] = h
		}
	}
	return out
}
