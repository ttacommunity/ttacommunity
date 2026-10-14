// Pairing a Swiss round: players on the same points meet, never the same opponent twice (a round that cannot avoid
// it is refused), and within a score group the pairs are the best time-zone fit (then the closest ratings, when
// asked), with chance deciding the rest.
package blitz

import (
	"cmp"
	"errors"
	"math"
	"math/bits"
	"math/rand/v2"
	"slices"
)

// Player is an entrant as the pairing sees them: their points so far, their place in the standings before the
// round (1 the top, 0 when not known), rating (Rated false when they have none), the zone's offset from UTC in
// minutes at pairing time, who they have met, and whether they had a bye.
type Player struct {
	ID        int64
	Points    float64
	Place     int
	Rating    float64
	Rated     bool
	Offset    int
	Opponents []int64
	HadBye    bool
}

// Criteria are what the pairing minimises within a score group: the zone gap when Timezone, then the rating gap
// when Rating. What is left is decided by the seed.
type Criteria struct {
	Timezone, Rating bool
}

// Match is one pairing, with its zone gap in minutes and rating gap.
type Match struct {
	A, B      int64
	ZoneGap   int
	RatingGap float64
}

// Round is a pairing of everyone: the matches, and the bye when the count is odd.
type Round struct {
	Matches []Match
	Bye     *int64
}

// ErrTooFew means fewer than two players to pair.
var ErrTooFew = errors.New("a round needs at least two players")

// ErrNoPairing means no round pairs every player without someone meeting an opponent again.
var ErrNoPairing = errors.New("no round pairs everyone without someone meeting an opponent again: lower the Swiss rounds in Format to go on to the knockout")

// unratedAs is the rating an unrated player is given for a rating gap.
const unratedAs = 1500

// dpPlayers is the largest group paired by the exact search; a larger one is paired greedily from the top.
const dpPlayers = 20

// ZoneGap is the distance in minutes between two UTC offsets on the 24-hour circle.
func ZoneGap(a, b int) int {
	d := a - b
	if d < 0 {
		d = -d
	}
	d %= 1440
	return min(d, 1440-d)
}

// Pair pairs the players for a round, never two who have met before. The seed makes the same players give the
// same round. It returns ErrNoPairing when no round pairs everyone without a repeat.
func Pair(players []Player, c Criteria, seed uint64) (Round, error) {
	if len(players) < 2 {
		return Round{}, ErrTooFew
	}
	rng := rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15))
	ranked := rank(players, rng)
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
		if c.Timezone {
			v += float64(ZoneGap(a.Offset, b.Offset)) * 1000
		}
		if c.Rating {
			v += ratingGap(a, b)
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

// rank orders players by points, then by their place in the standings where both have one, then by rating with
// unrated players last, then by chance.
func rank(players []Player, rng *rand.Rand) []Player {
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
		if a.Rated != b.Rated {
			if a.Rated {
				return -1
			}
			return 1
		}
		if a.Rated && a.Rating != b.Rating {
			if a.Rating > b.Rating {
				return -1
			}
			return 1
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

// scoreGroups splits ranked players into runs of equal points, highest first.
func scoreGroups(ranked []Player) [][]Player {
	var groups [][]Player
	for i, p := range ranked {
		if i == 0 || p.Points != ranked[i-1].Points {
			groups = append(groups, nil)
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], p)
	}
	return groups
}

func met(a, b Player) bool {
	return slices.Contains(a.Opponents, b.ID) || slices.Contains(b.Opponents, a.ID)
}

func ratingOf(p Player) float64 {
	if p.Rated {
		return p.Rating
	}
	return unratedAs
}

func ratingGap(a, b Player) float64 {
	return math.Abs(ratingOf(a) - ratingOf(b))
}

func match(a, b Player) Match {
	return Match{A: a.ID, B: b.ID, ZoneGap: ZoneGap(a.Offset, b.Offset), RatingGap: ratingGap(a, b)}
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
