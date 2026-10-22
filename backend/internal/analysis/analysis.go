// Package analysis computes upstream's model-analysis figures from a run's
// ratings: the rating distribution and how far the models agree.
package analysis

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"ttacommunity/backend/internal/round"
	"ttacommunity/backend/internal/store"
)

// Engines are the continuous models the distribution shows.
var Engines = []string{"glicko2_daneo", "glicko2_std"}

// binStart, binWidth and bins are upstream's histogram: 100-point bins from 1000 to 2100.
const (
	binStart = 1000
	binWidth = 100
	bins     = 11
)

// Distribution is how ratings spread in each engine, with their means for
// every player and for active ones.
type Distribution struct {
	Labels      []string
	Counts      map[string][]int
	Means       map[string]float64
	ActiveMeans map[string]float64
}

// Distribute counts each engine's ratings into upstream's bins, as numpy's
// histogram does (the last bin includes its top edge; others are left out),
// and averages them, 1500 where there are none.
func Distribute(ratings []store.ModelRating, active map[string]bool) Distribution {
	d := Distribution{Counts: map[string][]int{}, Means: map[string]float64{}, ActiveMeans: map[string]float64{}}
	for i := range bins {
		lo := binStart + i*binWidth
		d.Labels = append(d.Labels, fmt.Sprintf("%d-%d", lo, lo+binWidth-1))
	}
	for _, e := range Engines {
		counts := make([]int, bins)
		var all, act []float64
		for _, r := range ratings {
			if r.Model != e {
				continue
			}
			all = append(all, r.Rating)
			if active[r.Player] {
				act = append(act, r.Rating)
			}
			if b := int(math.Floor((r.Rating - binStart) / binWidth)); r.Rating >= binStart && b < bins {
				counts[b]++
			} else if r.Rating == binStart+bins*binWidth {
				counts[bins-1]++
			}
		}
		d.Counts[e], d.Means[e], d.ActiveMeans[e] = counts, mean(all), mean(act)
	}
	return d
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 1500
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return round.Tenth(sum / float64(len(xs)))
}

// Pair is how far two models agree: Pearson on ratings, Spearman on ranks.
type Pair struct {
	A, B              string
	Pearson, Spearman float64
}

// Agreement is the four closest and four furthest pairs of the page models,
// over the players rated in all of them with 15 or more opponents.
type Agreement struct {
	Sample       int
	Top, Weakest []Pair
}

// Agree compares every pair of the page models, Spearman's formula on ranks
// within the sample.
func Agree(ratings []store.ModelRating) Agreement {
	models := store.PlayerPageModels
	by := map[string]map[string]float64{}
	for _, r := range ratings {
		if r.Opponents < 15 || !slices.Contains(models, r.Model) {
			continue
		}
		if by[r.Player] == nil {
			by[r.Player] = map[string]float64{}
		}
		by[r.Player][r.Model] = r.Rating
	}
	var players []string
	for p, m := range by {
		if len(m) == len(models) {
			players = append(players, p)
		}
	}
	slices.Sort(players)
	a := Agreement{Sample: len(players), Top: []Pair{}, Weakest: []Pair{}}
	if len(players) < 2 {
		return a
	}

	// One slice per model, players in the same order in each.
	values, ranks := make([][]float64, len(models)), make([][]int, len(models))
	for i, m := range models {
		values[i] = make([]float64, len(players))
		for k, p := range players {
			values[i][k] = by[p][m]
		}
		ranks[i] = competitionRanks(values[i])
	}

	var pairs []Pair
	for i, m1 := range models {
		for j := i + 1; j < len(models); j++ {
			var d2 float64
			for k := range players {
				d := float64(ranks[i][k] - ranks[j][k])
				d2 += d * d
			}
			n := float64(len(players))
			pairs = append(pairs, Pair{A: m1, B: models[j], Pearson: round.To(pearson(values[i], values[j]), 4), Spearman: round.To(1-6*d2/(n*(n*n-1)), 4)})
		}
	}
	slices.SortStableFunc(pairs, func(x, y Pair) int {
		switch {
		case x.Spearman > y.Spearman:
			return -1
		case x.Spearman < y.Spearman:
			return 1
		}
		return 0
	})
	a.Top = pairs[:min(4, len(pairs))]
	a.Weakest = pairs[max(0, len(pairs)-4):]
	return a
}

// competitionRanks ranks each rating, highest first: equal ratings share the
// rank of their first, and the next rating skips the places they took.
func competitionRanks(ratings []float64) []int {
	order := make([]int, len(ratings))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(ratings[b], ratings[a]) })
	ranks := make([]int, len(ratings))
	rank := 1
	for pos, i := range order {
		if pos > 0 && ratings[i] != ratings[order[pos-1]] {
			rank = pos + 1
		}
		ranks[i] = rank
	}
	return ranks
}

func pearson(x, y []float64) float64 {
	var mx, my float64
	for i := range x {
		mx += x[i]
		my += y[i]
	}
	mx /= float64(len(x))
	my /= float64(len(y))
	var sxy, sxx, syy float64
	for i := range x {
		sxy += (x[i] - mx) * (y[i] - my)
		sxx += (x[i] - mx) * (x[i] - mx)
		syy += (y[i] - my) * (y[i] - my)
	}
	return sxy / math.Sqrt(sxx*syy)
}
