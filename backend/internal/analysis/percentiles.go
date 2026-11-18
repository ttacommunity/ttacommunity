// The percentiles page's figures: how ratings spread over bins, and where
// one rating stands among them.
package analysis

import (
	"math"
	"slices"
	"sort"

	"ttacommunity/backend/internal/round"
)

// MarkedPercentiles are the percentiles a spread marks, lowest first.
var MarkedPercentiles = []int{10, 25, 50, 75, 90, 99}

// Bin is the count of ratings from From to below the next bin's From.
type Bin struct {
	From, Count int
}

// Mark is the rating at a percentile.
type Mark struct {
	Percentile int
	Value      float64
}

// Spread is how a set of ratings spreads: every bin from the lowest
// rating's to the highest's, and the ratings at MarkedPercentiles.
type Spread struct {
	Players int
	Mean    float64
	Bins    []Bin
	Marks   []Mark
}

// SpreadOf bins ratings into bins width points wide and marks their
// percentiles by nearest rank: the lowest rating at or below which that
// share of the ratings stand.
func SpreadOf(ratings []float64, width int) Spread {
	sorted := slices.Sorted(slices.Values(ratings))
	s := Spread{Players: len(sorted), Bins: []Bin{}, Marks: []Mark{}}
	if len(sorted) == 0 {
		return s
	}
	binOf := func(r float64) int { return int(math.Floor(r/float64(width))) * width }
	lo := binOf(sorted[0])
	for from := lo; from <= binOf(sorted[len(sorted)-1]); from += width {
		s.Bins = append(s.Bins, Bin{From: from})
	}
	var sum float64
	for _, r := range sorted {
		sum += r
		s.Bins[(binOf(r)-lo)/width].Count++
	}
	s.Mean = round.Tenth(sum / float64(len(sorted)))
	for _, p := range MarkedPercentiles {
		i := max(0, int(math.Ceil(float64(p)*float64(len(sorted))/100))-1)
		s.Marks = append(s.Marks, Mark{Percentile: p, Value: sorted[i]})
	}
	return s
}

// Standing is where a rating stands among others: one more than those rated
// higher, and the share rated lower in per cent to one place.
type Standing struct {
	Rank       int
	Percentile float64
}

// Place returns where rating stands among ratings; among none, first and 0%.
func Place(ratings []float64, rating float64) Standing {
	sorted := slices.Sorted(slices.Values(ratings))
	below := sort.SearchFloat64s(sorted, rating)
	higher := len(sorted) - sort.Search(len(sorted), func(i int) bool { return sorted[i] > rating })
	st := Standing{Rank: higher + 1}
	if len(sorted) > 0 {
		st.Percentile = round.Tenth(100 * float64(below) / float64(len(sorted)))
	}
	return st
}
