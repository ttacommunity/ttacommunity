// The knockout after the Swiss rounds: the best seed meets the worst, in bracket order, and each later round pairs
// the winners of neighbouring matches until the final.
package blitz

import "errors"

// ErrBracket means a knockout round of a player count that is not a power of two, of at least two.
var ErrBracket = errors.New("a knockout round needs 2, 4, 8, 16 or 32 players")

// FirstKnockout pairs the knockout's first round from its players, best seed first: 1 v n, 2 v n-1 and so on,
// listed in bracket order so that neighbouring matches' winners meet next (for eight: 1-8, 4-5, 2-7, 3-6).
func FirstKnockout(seeded []int64) ([][2]int64, error) {
	if !powerOfTwo(len(seeded)) {
		return nil, ErrBracket
	}
	order := []int{1}
	for len(order) < len(seeded) {
		n := 2 * len(order)
		next := make([]int, 0, n)
		for _, s := range order {
			next = append(next, s, n+1-s)
		}
		order = next
	}
	pairs := make([][2]int64, 0, len(order)/2)
	for i := 0; i < len(order); i += 2 {
		pairs = append(pairs, [2]int64{seeded[order[i]-1], seeded[order[i+1]-1]})
	}
	return pairs, nil
}

// NextKnockout pairs the next knockout round from the winners of the one before, in its bracket order.
func NextKnockout(winners []int64) ([][2]int64, error) {
	if !powerOfTwo(len(winners)) {
		return nil, ErrBracket
	}
	pairs := make([][2]int64, 0, len(winners)/2)
	for i := 0; i < len(winners); i += 2 {
		pairs = append(pairs, [2]int64{winners[i], winners[i+1]})
	}
	return pairs, nil
}

// Winner returns who goes through from a match, and false when it has no winner: undecided, a draw or a double loss.
func Winner(m Played) (int64, bool) {
	switch m.Result {
	case WinA:
		return m.A, true
	case WinB:
		return m.B, true
	}
	return 0, false
}

func powerOfTwo(n int) bool {
	return n >= 2 && n&(n-1) == 0
}
