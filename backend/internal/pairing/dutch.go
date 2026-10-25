// The FIDE (Dutch) system, C.04.3 as applied from 1 February 2026, without colours: brackets from the top score
// down, S1 against S2, altered by transpositions and exchanges in FIDE's order until a candidate meets every
// criterion, else the best candidate. Article numbers and criteria ([C1]–[C21]) are FIDE's.
package pairing

import (
	"cmp"
	"math"
	"slices"
)

// levelSteps bounds the search at one level of a bracket, so a large bracket with no perfect candidate is paired
// in good time with the best candidate found.
const levelSteps = 200_000

// pairableSteps bounds each check that the players left can still be paired.
const pairableSteps = 100_000

// dutch is one round's pairing: who has met, who may take the bye, and the search's budget.
type dutch struct {
	met    map[[2]int64]bool
	byeMax float64 // the lowest score the bye can go to with the round still complete ([C5])
	anyBye bool    // everyone has had a bye, so anyone may take it
	zones  bool
	steps  int
}

// candidate is a bracket's pairing: the MDP-pairings first (MDP first), then the remainder's pairs, and the
// downfloaters, the MDPs left in the Limbo first.
type candidate struct {
	pairs    [][2]Player
	mdpPairs int
	down     []Player
	limbo    int
}

// quality is how well a candidate meets the quality criteria, for comparing two.
type quality struct {
	next   bool         // [C8]
	unplay int          // [C9]
	counts [4]int       // [C14]–[C17]
	diffs  [4][]float64 // [C18]–[C21], each in descending order
	zone   int          // the total zone gap, compared only with zones
}

// pairDutch pairs the players by the Dutch system; with zones, the closest time zones choose among the opponents
// the Dutch rules rank equally within S1 and S2.
func pairDutch(players []Player, zones bool) (Round, error) {
	ranked := slices.Clone(players)
	slices.SortStableFunc(ranked, byRank)
	d := &dutch{met: map[[2]int64]bool{}, zones: zones}
	for _, p := range ranked {
		for _, o := range p.Opponents {
			d.met[[2]int64{min(p.ID, o), max(p.ID, o)}] = true
		}
	}
	if !d.chooseByeScore(ranked) {
		return Round{}, ErrNoPairing
	}
	groups := scoreGroups(ranked)
	var out Round
	var mdps []Player
	for i, residents := range groups {
		var next, below []Player
		if i+1 < len(groups) {
			next = groups[i+1]
			for _, g := range groups[i+2:] {
				below = append(below, g...)
			}
		}
		c, ok := d.bracket(mdps, residents, next, below)
		if !ok {
			return Round{}, ErrNoPairing
		}
		for _, p := range c.pairs {
			out.Matches = append(out.Matches, match(p[0], p[1]))
		}
		mdps = slices.SortedStableFunc(slices.Values(c.down), byRank)
	}
	switch len(mdps) {
	case 0:
	case 1:
		out.Bye = new(mdps[0].ID)
	default:
		return Round{}, ErrNoPairing
	}
	return out, nil
}

// byRank is FIDE's order (Article 1.2): score, then starting rank, then id for players given the same rank.
func byRank(a, b Player) int {
	return cmp.Or(cmp.Compare(b.Points, a.Points), cmp.Compare(a.TPN, b.TPN), cmp.Compare(a.ID, b.ID))
}

// chooseByeScore sets the lowest score the bye can go to with every other player paired ([C5]), letting anyone
// take it when all have had one. It reports whether the round can be paired at all.
func (d *dutch) chooseByeScore(ranked []Player) bool {
	if len(ranked)%2 == 0 {
		return d.completable(ranked)
	}
	d.anyBye = !slices.ContainsFunc(ranked, func(p Player) bool { return !p.HadBye })
	var scores []float64
	for _, p := range ranked {
		if d.anyBye || !p.HadBye {
			scores = append(scores, p.Points)
		}
	}
	slices.Sort(scores)
	for _, s := range slices.Compact(scores) {
		if d.byeMax = s; d.completable(ranked) {
			return true
		}
	}
	return false
}

func (d *dutch) hasMet(a, b Player) bool {
	return d.met[[2]int64{min(a.ID, b.ID), max(a.ID, b.ID)}]
}

func (d *dutch) byeOK(p Player) bool {
	return (d.anyBye || !p.HadBye) && p.Points <= d.byeMax
}

// completable reports whether the players can all be paired without a rematch, one taking the bye when they are
// odd ([C4]).
func (d *dutch) completable(ps []Player) bool {
	bye := d.byeOK
	if len(ps)%2 == 0 {
		bye = nil
	}
	return d.pairable(ps, func(i, j int) bool { return !d.hasMet(ps[i], ps[j]) }, func(i int) bool { return bye != nil && bye(ps[i]) })
}

// pairable reports whether ps can all be paired by can, one of them taking the bye if bye allows it. It backs up
// from the player with the fewest choices, and says no when it runs out of steps.
func (d *dutch) pairable(ps []Player, can func(i, j int) bool, bye func(i int) bool) bool {
	used := make([]bool, len(ps))
	byeLeft := len(ps)%2 == 1
	steps := 0
	var place func(left int) bool
	place = func(left int) bool {
		if left == 0 {
			return true
		}
		if steps++; steps > pairableSteps {
			return false
		}
		pick, fewest := -1, math.MaxInt
		for i := range ps {
			if used[i] {
				continue
			}
			n := 0
			if byeLeft && bye(i) {
				n++
			}
			for j := range ps {
				if j != i && !used[j] && can(i, j) {
					n++
				}
			}
			if n == 0 {
				return false
			}
			if n < fewest {
				pick, fewest = i, n
			}
		}
		used[pick] = true
		if byeLeft && bye(pick) {
			byeLeft = false
			if place(left - 1) {
				return true
			}
			byeLeft = true
		}
		for j := range ps {
			if j != pick && !used[j] && can(pick, j) {
				used[j] = true
				if place(left - 2) {
					return true
				}
				used[j] = false
			}
		}
		used[pick] = false
		return false
	}
	return place(len(ps))
}

// bracket pairs one bracket, its MDPs and residents, knowing the next scoregroup and the players below that. It
// tries the most pairs first, and the most MDPs paired among those ([C6], [C7]). ok is false when no candidate
// meets [C1]–[C4].
func (d *dutch) bracket(mdps, residents, next, below []Player) (candidate, bool) {
	last := len(next) == 0
	r := len(residents)
	top := min(len(mdps), r) + (r-min(len(mdps), r))/2
	for total := top; total >= 0; total-- {
		if last && len(mdps)+r-2*total > 1 {
			break
		}
		for m1 := min(len(mdps), r, total); m1 >= 0; m1-- {
			if 2*(total-m1) > r-m1 {
				continue
			}
			if c, ok := d.level(mdps, residents, m1, total-m1, next, below); ok {
				return c, true
			}
		}
	}
	return candidate{}, false
}

// search keeps the best candidate seen at a level, and whether to stop.
type search struct {
	d           *dutch
	next, below []Player
	last        bool
	valid       map[string]bool // [C4], by the downfloaters' ids
	nextOK      map[string]bool // [C8], by the same
	best        candidate
	bestQ       quality
	found, stop bool
}

// level searches the candidates that pair m1 MDPs and rp pairs of the remainder, in FIDE's order (Articles 3.6,
// 3.7 and 4): the sets of MDPs, then the transpositions of S2, then the remainder's transpositions and exchanges.
func (d *dutch) level(mdps, residents []Player, m1, rp int, next, below []Player) (candidate, bool) {
	d.steps = 0
	s := &search{d: d, next: next, below: below, last: len(next) == 0, valid: map[string]bool{}, nextOK: map[string]bool{}}
	for _, set := range mdpSets(mdps, m1) {
		var s1, limbo []Player
		for i, p := range mdps {
			if slices.Contains(set, i) {
				s1 = append(s1, p)
			} else {
				limbo = append(limbo, p)
			}
		}
		d.transpositions(s1, residents, func(mp [][2]Player, rem []Player) bool {
			return d.homogeneous(rem, rp, func(pairs [][2]Player, down []Player) bool {
				c := candidate{pairs: append(slices.Clone(mp), pairs...), mdpPairs: len(mp), down: append(slices.Clone(limbo), down...), limbo: len(limbo)}
				return s.consider(c)
			})
		})
		if s.stop {
			break
		}
	}
	return s.best, s.found
}

// consider weighs a candidate, keeping it when it is valid and better than the best so far. It reports whether
// to stop: a perfect candidate, or the level's steps spent.
func (s *search) consider(c candidate) bool {
	if s.d.steps > levelSteps {
		s.stop = true
		return true
	}
	key := idsKey(c.down)
	ok, seen := s.valid[key]
	if !seen {
		ok = s.d.validDown(c.down, s.next, s.below, s.last)
		s.valid[key] = ok
	}
	if !ok {
		return false
	}
	q := s.d.quality(c, s, key)
	if !s.found || compareQuality(q, s.bestQ, s.d.zones) < 0 {
		s.best, s.bestQ, s.found = c, q, true
	}
	if q.perfect() {
		s.stop = true
	}
	return s.stop
}

// validDown reports whether a bracket's downfloaters leave a round that can be completed ([C4]); from the last
// bracket, only the bye's taker may be left.
func (d *dutch) validDown(down, next, below []Player, last bool) bool {
	if last {
		return len(down) == 0 || len(down) == 1 && d.byeOK(down[0])
	}
	return d.completable(slices.Concat(down, next, below))
}

// nextBracketOK reports whether the next bracket, the downfloaters with the next scoregroup, can pair every
// downfloater and all its residents but at most one, with the round still complete ([C8]).
func (d *dutch) nextBracketOK(down, next, below []Player) bool {
	if len(next) == 0 {
		return true
	}
	fits := func(residents []Player) bool {
		ps := slices.Concat(down, residents)
		can := func(i, j int) bool { return (i >= len(down) || j >= len(down)) && !d.hasMet(ps[i], ps[j]) }
		return d.pairable(ps, can, func(int) bool { return false })
	}
	if (len(down)+len(next))%2 == 0 {
		return fits(next) && d.completable(below)
	}
	for i := range next {
		if fits(slices.Delete(slices.Clone(next), i, i+1)) && d.completable(append(slices.Clone(below), next[i])) {
			return true
		}
	}
	return false
}

// quality measures a candidate against [C8], [C9] and [C14]–[C21], and its total zone gap.
func (d *dutch) quality(c candidate, s *search, key string) quality {
	var q quality
	ok, seen := s.nextOK[key]
	if !seen {
		ok = d.nextBracketOK(c.down, s.next, s.below)
		s.nextOK[key] = ok
	}
	q.next = ok
	if s.last && len(c.down) == 1 {
		q.unplay = c.down[0].Unplayed
	}
	for _, p := range c.down[c.limbo:] {
		for k := range 2 {
			if p.Floats[k] == Down {
				q.counts[2*k]++
			}
		}
	}
	for _, pr := range c.pairs[:c.mdpPairs] {
		mdp, opp := pr[0], pr[1]
		for k := range 2 {
			if opp.Floats[k] == Up {
				q.counts[2*k+1]++
				q.diffs[2*k+1] = append(q.diffs[2*k+1], mdp.Points-opp.Points)
			}
			if mdp.Floats[k] == Down {
				q.diffs[2*k] = append(q.diffs[2*k], mdp.Points-opp.Points)
			}
		}
	}
	for i := range q.diffs {
		slices.SortFunc(q.diffs[i], func(a, b float64) int { return cmp.Compare(b, a) })
	}
	for _, pr := range c.pairs {
		q.zone += ZoneGap(pr[0].Offset, pr[1].Offset)
	}
	return q
}

// perfect reports whether a candidate meets every quality criterion, so the search can stop (Article 3.4). An
// MDP's score difference ([C18], [C20]) is the same against every resident, so it never stops the search.
func (q quality) perfect() bool {
	return q.next && q.unplay == 0 && q.counts == [4]int{}
}

// compareQuality orders two candidates' quality, the better first, in the criteria's priority; with zones, the
// smaller total zone gap breaks a tie.
func compareQuality(a, b quality, zones bool) int {
	if a.next != b.next {
		if a.next {
			return -1
		}
		return 1
	}
	if c := cmp.Compare(a.unplay, b.unplay); c != 0 {
		return c
	}
	if c := slices.Compare(a.counts[:], b.counts[:]); c != 0 {
		return c
	}
	for i := range a.diffs {
		if c := compareDiffs(a.diffs[i], b.diffs[i]); c != 0 {
			return c
		}
	}
	if zones {
		return cmp.Compare(a.zone, b.zone)
	}
	return 0
}

// compareDiffs compares two descending lists of score differences, a missing one counting as none.
func compareDiffs(a, b []float64) int {
	for i := range max(len(a), len(b)) {
		var x, y float64
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if c := cmp.Compare(x, y); c != 0 {
			return c
		}
	}
	return 0
}

// mdpSets lists the sets of m1 MDPs (by index) to pair, in FIDE's order (Article 4.4): by smallest differing
// index, keeping only those whose Limbo holds the lowest scores ([C7]).
func mdpSets(mdps []Player, m1 int) [][]int {
	var lowest []float64
	for _, p := range mdps {
		lowest = append(lowest, p.Points)
	}
	slices.Sort(lowest)
	lowest = lowest[:len(mdps)-m1]
	var out [][]int
	for _, set := range combinations(len(mdps), m1) {
		var limbo []float64
		for i, p := range mdps {
			if !slices.Contains(set, i) {
				limbo = append(limbo, p.Points)
			}
		}
		slices.Sort(limbo)
		if slices.Equal(limbo, lowest) {
			out = append(out, set)
		}
	}
	return out
}

// combinations lists every k-subset of 0..n-1 in lexicographic order.
func combinations(n, k int) [][]int {
	var out [][]int
	set := make([]int, k)
	var pick func(i, from int)
	pick = func(i, from int) {
		if i == k {
			out = append(out, slices.Clone(set))
			return
		}
		for v := from; v <= n-(k-i); v++ {
			set[i] = v
			pick(i+1, v+1)
		}
	}
	pick(0, 0)
	return out
}

// homogeneous yields a homogeneous bracket's (or remainder's) candidates with p pairs, in FIDE's order (Article
// 3.6): S1 the first p players, S2 the rest, transposed; then each exchange between them. yield returns true to
// stop, and so does homogeneous.
func (d *dutch) homogeneous(players []Player, p int, yield func(pairs [][2]Player, down []Player) bool) bool {
	if p == 0 {
		return yield(nil, players)
	}
	s1, s2 := players[:p], players[p:]
	if d.transpositions(s1, s2, yield) {
		return true
	}
	for k := 1; k <= min(len(s1), len(s2)); k++ {
		for _, x := range exchanges(len(s1), len(s2), k) {
			if d.steps > levelSteps {
				return true
			}
			var n1, n2 []Player
			for i, pl := range players {
				inS1 := i < p
				if slices.Contains(x.out, i) || slices.Contains(x.in, i) {
					inS1 = !inS1
				}
				if inS1 {
					n1 = append(n1, pl)
				} else {
					n2 = append(n2, pl)
				}
			}
			if d.transpositions(n1, n2, yield) {
				return true
			}
		}
	}
	return false
}

// exchange swaps players between S1 and S2, by index in the bracket: out leave S1, in join it.
type exchange struct{ out, in []int }

// maxExchanges bounds how many exchanges of one size are sorted, so a large bracket gives up on them in good time.
const maxExchanges = 100_000

// exchanges lists the exchanges of k players between S1 (indices 0..n1-1) and S2 (n1..n1+n2-1), in FIDE's order
// (Article 4.3.2): the smallest difference of index sums, then the highest index leaving S1, then the lowest joining.
func exchanges(n1, n2, k int) []exchange {
	outs, ins := combinations(n1, k), combinations(n2, k)
	if len(outs)*len(ins) > maxExchanges {
		return nil
	}
	var xs []exchange
	for _, o := range outs {
		for _, in := range ins {
			shifted := make([]int, k)
			for i, v := range in {
				shifted[i] = v + n1
			}
			xs = append(xs, exchange{out: o, in: shifted})
		}
	}
	sum := func(s []int) int {
		t := 0
		for _, v := range s {
			t += v
		}
		return t
	}
	slices.SortStableFunc(xs, func(a, b exchange) int {
		if c := cmp.Compare(sum(a.in)-sum(a.out), sum(b.in)-sum(b.out)); c != 0 {
			return c
		}
		for i := k - 1; i >= 0; i-- {
			if a.out[i] != b.out[i] {
				return cmp.Compare(b.out[i], a.out[i])
			}
		}
		return slices.Compare(a.in, b.in)
	})
	return xs
}

// transpositions yields S1 paired with each ordering of S2 in FIDE's order (Article 4.2), skipping rematches, with
// the S2 players left over; with zones, the closest-zone ordering comes first. yield returns true to stop, and so
// does transpositions.
func (d *dutch) transpositions(s1, s2 []Player, yield func(pairs [][2]Player, left []Player) bool) bool {
	if len(s1) == 0 {
		return yield(nil, s2)
	}
	if d.zones {
		if order, ok := d.closestZones(s1, s2); ok && yield(pairsOf(s1, s2, order)) {
			return true
		}
	}
	used := make([]bool, len(s2))
	order := make([]int, len(s1))
	var place func(i int) bool
	place = func(i int) bool {
		if d.steps++; d.steps > levelSteps {
			return true
		}
		if i == len(s1) {
			return yield(pairsOf(s1, s2, order))
		}
		for j := range s2 {
			if used[j] || d.hasMet(s1[i], s2[j]) {
				continue
			}
			used[j], order[i] = true, j
			if place(i + 1) {
				return true
			}
			used[j] = false
		}
		return false
	}
	return place(0)
}

// pairsOf pairs s1[i] with s2[order[i]], returning the pairs and the S2 players left over.
func pairsOf(s1, s2 []Player, order []int) ([][2]Player, []Player) {
	pairs := make([][2]Player, len(s1))
	taken := make([]bool, len(s2))
	for i, j := range order {
		pairs[i], taken[j] = [2]Player{s1[i], s2[j]}, true
	}
	var left []Player
	for j, p := range s2 {
		if !taken[j] {
			left = append(left, p)
		}
	}
	return pairs, left
}

// closestZones orders S2 against S1 for the least total zone gap without a rematch, nearest to S1[i] against
// S2[i] among equals. ok is false when every ordering has a rematch.
func (d *dutch) closestZones(s1, s2 []Player) ([]int, bool) {
	const forbidden = math.MaxInt32
	weight := len(s1)*len(s2) + 1
	cost := make([][]int, len(s1))
	for i := range s1 {
		cost[i] = make([]int, len(s2))
		for j := range s2 {
			if d.hasMet(s1[i], s2[j]) {
				cost[i][j] = forbidden
			} else {
				cost[i][j] = ZoneGap(s1[i].Offset, s2[j].Offset)*weight + max(i-j, j-i)
			}
		}
	}
	order := assign(cost)
	for i, j := range order {
		if cost[i][j] == forbidden {
			return nil, false
		}
	}
	return order, true
}

// assign solves the assignment problem for n rows and m >= n columns: the column for each row that makes the
// least total cost (the Hungarian algorithm, O(n²m)).
func assign(cost [][]int) []int {
	n, m := len(cost), len(cost[0])
	const inf = math.MaxInt / 4
	u, v := make([]int, n+1), make([]int, m+1)
	row, way := make([]int, m+1), make([]int, m+1)
	for i := 1; i <= n; i++ {
		row[0] = i
		j0 := 0
		minv := make([]int, m+1)
		for j := range minv {
			minv[j] = inf
		}
		used := make([]bool, m+1)
		for {
			used[j0] = true
			i0, delta, j1 := row[j0], inf, 0
			for j := 1; j <= m; j++ {
				if used[j] {
					continue
				}
				if c := cost[i0-1][j-1] - u[i0] - v[j]; c < minv[j] {
					minv[j], way[j] = c, j0
				}
				if minv[j] < delta {
					delta, j1 = minv[j], j
				}
			}
			for j := 0; j <= m; j++ {
				if used[j] {
					u[row[j]] += delta
					v[j] -= delta
				} else {
					minv[j] -= delta
				}
			}
			j0 = j1
			if row[j0] == 0 {
				break
			}
		}
		for j0 != 0 {
			j1 := way[j0]
			row[j0] = row[j1]
			j0 = j1
		}
	}
	out := make([]int, n)
	for j := 1; j <= m; j++ {
		if row[j] != 0 {
			out[row[j]-1] = j - 1
		}
	}
	return out
}

// idsKey names a set of players by their ids, for caching what depends only on who floats.
func idsKey(ps []Player) string {
	ids := make([]int64, len(ps))
	for i, p := range ps {
		ids[i] = p.ID
	}
	slices.Sort(ids)
	b := make([]byte, 0, 8*len(ids))
	for _, id := range ids {
		for s := 0; s < 64; s += 8 {
			b = append(b, byte(id>>s))
		}
	}
	return string(b)
}
