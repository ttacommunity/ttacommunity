package challenges

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

// Listed is a challenge's id and set, as the list gives them.
type Listed struct {
	ID, Set string
}

// Best is a player's best entry on a challenge: its rank among the players' bests (equal dominations share one)
// and its share of the record in percent, both counted as Counted counts.
type Best struct {
	Entry
	Rank  int
	Share float64
}

// Summary is one player's results: their best on each challenge played, how many they played, how many of those
// bests are verified and hold the record, their total domination and rating, each also by set, and when they last
// entered anything.
type Summary struct {
	Key, Name                        string
	Bests                            map[string]Best
	Played, Verified, Records, Total int
	Totals                           map[string]int
	Rating                           float64
	Ratings                          map[string]float64
	LastEntry                        time.Time
}

// Stats are every challenge's board, best first, and every player with a counted entry, by Key.
type Stats struct {
	Boards  map[string][]Best
	Players map[string]*Summary
}

// Counted is a domination as sums and ratings count it: a negative one (a lost game) counts as 0.
func Counted(domination int) int {
	return max(0, domination)
}

// Compute ranks entries, given in the order they were entered, on the listed challenges. A bugged entry never
// counts, nor with verifiedOnly an unproven one. A player's rating is their bests' shares averaged over every
// challenge, unplayed ones counting 0. With verifiedOnly and a trust Key, that player's unproven entries count too,
// for them alone: the boards and everyone else stay as they are, so a best above the record shares over 100%.
func Compute(list []Listed, entries []Entry, verifiedOnly bool, trust string) Stats {
	sizes := map[string]int{}
	setOf := map[string]string{}
	for _, c := range list {
		sizes[c.Set]++
		setOf[c.ID] = c.Set
	}
	names := map[string]string{}
	for _, e := range entries {
		names[e.Player] = e.Name
	}

	boards := map[string][]Best{}
	for _, c := range list {
		boards[c.ID] = []Best{}
	}
	for _, b := range bests(entries, func(e Entry) bool { return !e.Bugged && (!verifiedOnly || e.Verified) }) {
		if _, ok := boards[b.Challenge]; ok {
			boards[b.Challenge] = append(boards[b.Challenge], Best{Entry: b})
		}
	}
	for _, board := range boards {
		slices.SortFunc(board, func(a, b Best) int { return order(a.Entry, b.Entry) })
		for i := range board {
			board[i].Name = names[board[i].Player]
			board[i].Rank = i + 1
			if i > 0 && board[i-1].Domination == board[i].Domination {
				board[i].Rank = board[i-1].Rank
			}
			board[i].Share = share(board[i].Domination, Counted(board[0].Domination), 0)
		}
	}

	players := map[string]*Summary{}
	add := func(b Best) {
		p := players[b.Player]
		if p == nil {
			p = &Summary{Key: b.Player, Name: names[b.Player], Bests: map[string]Best{}, Totals: map[string]int{}, Ratings: map[string]float64{}}
			players[b.Player] = p
		}
		set := setOf[b.Challenge]
		p.Bests[b.Challenge] = b
		p.Played++
		if b.Verified {
			p.Verified++
		}
		if b.Rank == 1 {
			p.Records++
		}
		p.Total += Counted(b.Domination)
		p.Totals[set] += Counted(b.Domination)
		p.Rating += b.Share / float64(len(list))
		p.Ratings[set] += b.Share / float64(sizes[set])
	}
	trusted := verifiedOnly && trust != ""
	for _, board := range boards {
		for _, b := range board {
			if !trusted || b.Player != trust {
				add(b)
			}
		}
	}
	if trusted {
		for _, e := range bests(entries, func(e Entry) bool { return !e.Bugged && e.Player == trust }) {
			if board, ok := boards[e.Challenge]; ok {
				add(trustedBest(e, board, names[trust]))
			}
		}
	}
	for _, e := range entries {
		if p := players[e.Player]; p != nil && e.At.After(p.LastEntry) {
			p.LastEntry = e.At
		}
	}
	return Stats{Boards: boards, Players: players}
}

// bests returns each player's best counting entry on each challenge, as betterOwn chooses it.
func bests(entries []Entry, counts func(Entry) bool) []Entry {
	type pair struct{ challenge, player string }
	best := map[pair]int{}
	for i, e := range entries {
		if !counts(e) {
			continue
		}
		k := pair{e.Challenge, e.Player}
		if j, ok := best[k]; !ok || betterOwn(e, entries[j]) {
			best[k] = i
		}
	}
	got := make([]Entry, 0, len(best))
	for _, i := range best {
		got = append(got, entries[i])
	}
	return got
}

// trustedBest ranks e against the official board, the trusted player's own entry on it left out.
func trustedBest(e Entry, board []Best, name string) Best {
	b := Best{Entry: e, Rank: 1}
	b.Name = name
	record := 0
	if len(board) > 0 {
		record = Counted(board[0].Domination)
	}
	for _, o := range board {
		if o.Player != e.Player && o.Domination > e.Domination {
			b.Rank++
		}
	}
	b.Share = share(e.Domination, record, 100)
	return b
}

// share is domination's counted value as a percentage of record, or ifNoRecord for a positive one when record is 0.
func share(domination, record int, ifNoRecord float64) float64 {
	counted := Counted(domination)
	switch {
	case record > 0:
		return 100 * float64(counted) / float64(record)
	case counted > 0:
		return ifNoRecord
	}
	return 0
}

// order sorts player bests best first: the higher domination, then the earlier, then the first entered.
func order(a, b Entry) int {
	if ahead(a, b) {
		return -1
	}
	if ahead(b, a) {
		return 1
	}
	return 0
}

// ScopePlayer is one player's results over a scope's challenges: their total, how many they played, and their rating.
type ScopePlayer struct {
	Name   string
	Total  int
	Played int
	Rating float64
}

// Scope sums a set of challenges: the pooled record (every record added up), how many have one, how many there are,
// and the players who played any of them, by total and by rating.
type Scope struct {
	Pooled, Covered, Size int
	ByTotal, ByRating     []ScopePlayer
}

// Scope sums the challenges ids. Players are ranked by total, then by challenges played, then by name; by rating
// they are ranked by rating, then in that order.
func (s Stats) Scope(ids []string) Scope {
	got := Scope{Size: len(ids)}
	for _, id := range ids {
		if board := s.Boards[id]; len(board) > 0 {
			got.Pooled += Counted(board[0].Domination)
			got.Covered++
		}
	}
	for _, p := range s.Players {
		sp := ScopePlayer{Name: p.Name}
		for _, id := range ids {
			if b, ok := p.Bests[id]; ok {
				sp.Total += Counted(b.Domination)
				sp.Played++
				sp.Rating += b.Share / float64(len(ids))
			}
		}
		if sp.Played > 0 {
			got.ByTotal = append(got.ByTotal, sp)
		}
	}
	slices.SortFunc(got.ByTotal, func(a, b ScopePlayer) int {
		return cmp.Or(cmp.Compare(b.Total, a.Total), cmp.Compare(b.Played, a.Played), strings.Compare(a.Name, b.Name))
	})
	got.ByRating = slices.Clone(got.ByTotal)
	slices.SortStableFunc(got.ByRating, func(a, b ScopePlayer) int { return cmp.Compare(b.Rating, a.Rating) })
	return got
}

// Step is a time a challenge's record went up: the entry that set it, and its gain on the record before (nil for
// the first).
type Step struct {
	Entry
	Gain *int
}

// Steps returns each time the record of one challenge's entries went up, in the order they were entered. A bugged
// entry never counts, nor with verifiedOnly an unproven one.
func Steps(entries []Entry, verifiedOnly bool) []Step {
	counted := slices.DeleteFunc(slices.Clone(entries), func(e Entry) bool { return e.Bugged || (verifiedOnly && !e.Verified) })
	slices.SortFunc(counted, func(a, b Entry) int {
		if earlier(a, b) {
			return -1
		}
		if earlier(b, a) {
			return 1
		}
		return 0
	})
	var steps []Step
	for _, e := range counted {
		if len(steps) == 0 {
			steps = append(steps, Step{Entry: e})
			continue
		}
		prev := steps[len(steps)-1].Domination
		if e.Domination > prev {
			gain := e.Domination - prev
			steps = append(steps, Step{Entry: e, Gain: &gain})
		}
	}
	return steps
}
