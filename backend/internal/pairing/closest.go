// The closest-fit methods: players on the same points meet, and within a score group the pairs are the best
// time-zone fit when asked, then the closest by the order (ratings, or the players' TPN for a lot), with chance
// deciding the rest.
package pairing

import (
	"cmp"
	"math"
	"math/bits"
	"math/rand/v2"
	"slices"
)

// criteria are what the closest-fit pairing minimises within a score group: the zone gap when timezone, then the
// rating gap (ByRating) or the TPN gap (ByLot). What is left is decided by the seed.
type criteria struct {
	timezone bool
	order    Order
}

// unratedAs is the rating an unrated player is given for a rating gap.
const unratedAs = 1500

// dpPlayers is the largest group paired by the exact search; a larger one is paired greedily from the top.
const dpPlayers = 20

// closest pairs the players by the criteria, never two who have met before. The seed makes the same players give
// the same round. It returns ErrNoPairing when no round pairs everyone without a repeat.
func closest(players []Player, c criteria, seed uint64) (Round, error) {
	rng := rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15))
	ranked := rank(players, rng, c.order)
	jitter := map[[2]int64]float64{}
	cost := func(a, b Player) float64 {
		if met(a, b) {
			return math.Inf(1)
		}
		key := [2]int64{min(a.ID, b.ID), max(a.ID, b.ID)}
		if _, ok := jitter[key]; !ok {
			jitter[key] = rng.Float64()
		}
		v := math.Abs(a.Points-b.Points) * 1e6
		if c.timezone {
			v += float64(ZoneGap(a.Offset, b.Offset)) * 1000
		}
		switch c.order {
		case ByRating:
			v += ratingGap(a, b)
		case ByLot:
			v += math.Abs(float64(a.TPN - b.TPN))
		}
		return v + jitter[key]
	}
	if len(ranked)%2 == 0 {
		if m, ok := pairField(ranked, cost); ok {
			return Round{Matches: m}, nil
		}
		return Round{}, ErrNoPairing
	}
	// The bye goes to the lowest-ranked player without one, unless only another choice lets everyone else be paired.
	for _, i := range byeOrder(ranked) {
		if m, ok := pairField(slices.Delete(slices.Clone(ranked), i, i+1), cost); ok {
			return Round{Matches: m, Bye: new(ranked[i].ID)}, nil
		}
	}
	return Round{}, ErrNoPairing
}

// pairField pairs an even field without a repeat: score group by score group, the odd one or anyone a group cannot
// place floating down to the next, and, when the last of them cannot be paired, by a search across the whole field.
func pairField(field []Player, cost func(a, b Player) float64) ([]Match, bool) {
	var out []Match
	var carry []Player
	for _, group := range scoreGroups(field) {
		pool := append(carry, group...)
		carry = nil
		if len(pool)%2 == 1 {
			carry, pool = []Player{pool[len(pool)-1]}, pool[:len(pool)-1]
		}
		if m, ok := matching(pool, cost); ok {
			out = append(out, m...)
		} else {
			carry = append(pool, carry...)
		}
	}
	if len(carry) == 0 {
		return out, true
	}
	if m, ok := matching(carry, cost); ok {
		return append(out, m...), true
	}
	return withoutRepeats(field, cost)
}

// searchSteps bounds the search across a whole field, so a field that cannot be paired is refused in good time.
const searchSteps = 1_000_000

// withoutRepeats pairs a ranked even field so nobody meets an opponent again: the best-ranked player left takes the
// cheapest partner they have not met, backing up at a dead end. ok is false when no such round exists, or none is
// found within searchSteps.
func withoutRepeats(field []Player, cost func(a, b Player) float64) ([]Match, bool) {
	used := make([]bool, len(field))
	var out []Match
	steps := 0
	var place func() bool
	place = func() bool {
		i := slices.Index(used, false)
		if i < 0 {
			return true
		}
		if steps++; steps > searchSteps {
			return false
		}
		used[i] = true
		var partners []int
		for j := i + 1; j < len(field); j++ {
			if !used[j] && !math.IsInf(cost(field[i], field[j]), 1) {
				partners = append(partners, j)
			}
		}
		slices.SortStableFunc(partners, func(x, y int) int { return cmp.Compare(cost(field[i], field[x]), cost(field[i], field[y])) })
		for _, j := range partners {
			used[j] = true
			out = append(out, match(field[i], field[j]))
			if place() {
				return true
			}
			out = out[:len(out)-1]
			used[j] = false
		}
		used[i] = false
		return false
	}
	if !place() {
		return nil, false
	}
	return out, true
}

// rank orders players by points, then by their place in the standings where both have one, then by the order (rating
// with unrated players last, or TPN for a lot), then by chance. The order decides the bye and the search's start, so
// a method without ratings leaves them out here too.
func rank(players []Player, rng *rand.Rand, order Order) []Player {
	ranked := slices.Clone(players)
	draw := make(map[int64]float64, len(ranked))
	for _, p := range ranked {
		draw[p.ID] = rng.Float64()
	}
	slices.SortStableFunc(ranked, func(a, b Player) int {
		if a.Points != b.Points {
			if a.Points > b.Points {
				return -1
			}
			return 1
		}
		if a.Place > 0 && b.Place > 0 && a.Place != b.Place {
			return a.Place - b.Place
		}
		if order == ByRating && a.Rated != b.Rated {
			if a.Rated {
				return -1
			}
			return 1
		}
		if order == ByRating && a.Rated && a.Rating != b.Rating {
			if a.Rating > b.Rating {
				return -1
			}
			return 1
		}
		if order == ByLot && a.TPN != b.TPN {
			return a.TPN - b.TPN
		}
		if draw[a.ID] != draw[b.ID] {
			if draw[a.ID] < draw[b.ID] {
				return -1
			}
			return 1
		}
		return 0
	})
	return ranked
}

// byeOrder lists who may take the bye, the first choice first: the players without one from the lowest-ranked up,
// or everyone from the lowest up when all have had one.
func byeOrder(ranked []Player) []int {
	var fresh, all []int
	for i := len(ranked) - 1; i >= 0; i-- {
		all = append(all, i)
		if !ranked[i].HadBye {
			fresh = append(fresh, i)
		}
	}
	if len(fresh) > 0 {
		return fresh
	}
	return all
}

// matching pairs an even pool at the least total cost: exactly for a small pool, else greedily from the top.
// ok is false when some pair would cost infinitely (a repeat) whichever way.
func matching(pool []Player, cost func(a, b Player) float64) ([]Match, bool) {
	if len(pool) == 0 {
		return nil, true
	}
	if len(pool) <= dpPlayers {
		return exactMatching(pool, cost)
	}
	return greedyMatching(pool, cost)
}

// exactMatching tries every way to pair the pool, over subsets: best[mask] is the least cost of pairing the
// players in mask, found by pairing its lowest member with each other member.
func exactMatching(pool []Player, cost func(a, b Player) float64) ([]Match, bool) {
	n := len(pool)
	full := 1<<n - 1
	best := make([]float64, full+1)
	partner := make([]int8, full+1)
	for mask := 1; mask <= full; mask++ {
		best[mask] = math.Inf(1)
		if bits.OnesCount(uint(mask))%2 == 1 {
			continue
		}
		i := bits.TrailingZeros(uint(mask))
		rest := mask &^ (1 << i)
		for j := i + 1; j < n; j++ {
			if rest&(1<<j) == 0 {
				continue
			}
			c := cost(pool[i], pool[j])
			if math.IsInf(c, 1) {
				continue
			}
			if sub := rest &^ (1 << j); best[sub]+c < best[mask] {
				best[mask], partner[mask] = best[sub]+c, int8(j)
			}
		}
	}
	if math.IsInf(best[full], 1) {
		return nil, false
	}
	var out []Match
	for mask := full; mask != 0; {
		i, j := bits.TrailingZeros(uint(mask)), int(partner[mask])
		out = append(out, match(pool[i], pool[j]))
		mask &^= 1<<i | 1<<j
	}
	return out, true
}

// greedyMatching pairs from the top: each unpaired player takes the cheapest partner left.
func greedyMatching(pool []Player, cost func(a, b Player) float64) ([]Match, bool) {
	left := slices.Clone(pool)
	var out []Match
	for len(left) > 0 {
		a, bestJ, bestC := left[0], -1, math.Inf(1)
		for j := 1; j < len(left); j++ {
			if c := cost(a, left[j]); c < bestC {
				bestJ, bestC = j, c
			}
		}
		if bestJ < 0 {
			return nil, false
		}
		out = append(out, match(a, left[bestJ]))
		left = slices.Delete(left, bestJ, bestJ+1)
		left = left[1:]
	}
	return out, true
}
