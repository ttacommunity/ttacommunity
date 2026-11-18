package analysis

import (
	"slices"
	"testing"
)

func TestSpreadOfBinsEveryRatingFromTheLowestToTheHighest(t *testing.T) {
	tests := []struct {
		name    string
		ratings []float64
		width   int
		want    []Bin
	}{
		{"no players", nil, 50, []Bin{}},
		{"a bin's top edge is the next bin's", []float64{1660, 1510, 1549.9, 1550}, 50, []Bin{{1500, 2}, {1550, 1}, {1600, 0}, {1650, 1}}},
		{"below zero rounds down", []float64{10, -20.5}, 50, []Bin{{-50, 1}, {0, 1}}},
		{"wider bins", []float64{1660, 1510, 1549.9, 1550}, 100, []Bin{{1500, 3}, {1600, 1}}},
		{"narrower bins", []float64{1510, 1549.9}, 25, []Bin{{1500, 1}, {1525, 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SpreadOf(tt.ratings, tt.width)
			if !slices.Equal(got.Bins, tt.want) {
				t.Errorf("SpreadOf(%v, %d).Bins = %v, want %v", tt.ratings, tt.width, got.Bins, tt.want)
			}
			if got.Players != len(tt.ratings) {
				t.Errorf("SpreadOf(%v).Players = %d, want %d", tt.ratings, got.Players, len(tt.ratings))
			}
		})
	}
}

func TestSpreadOfMarksTheNearestRankPercentiles(t *testing.T) {
	ratings := []float64{1000, 900, 800, 700, 600, 500, 400, 300, 200, 100}
	want := []Mark{{10, 100}, {25, 300}, {50, 500}, {75, 800}, {90, 900}, {99, 1000}}
	if got := SpreadOf(ratings, 50).Marks; !slices.Equal(got, want) {
		t.Errorf("SpreadOf(%v).Marks = %v, want %v", ratings, got, want)
	}
	if got := SpreadOf(nil, 50).Marks; got == nil || len(got) != 0 {
		t.Errorf("SpreadOf(nil).Marks = %#v, want empty", got)
	}
}

func TestSpreadOfLeavesItsInputAlone(t *testing.T) {
	ratings := []float64{3, 1, 2}
	SpreadOf(ratings, 50)
	if !slices.Equal(ratings, []float64{3, 1, 2}) {
		t.Errorf("SpreadOf sorted its input: %v", ratings)
	}
}

func TestPlaceCountsWhoIsRatedLowerAndHigher(t *testing.T) {
	pool := []float64{1600, 1500, 1400, 1500}
	tests := []struct {
		name   string
		pool   []float64
		rating float64
		want   Standing
	}{
		{"ties are neither lower nor higher", pool, 1500, Standing{Rank: 2, Percentile: 25}},
		{"above everyone", pool, 1700, Standing{Rank: 1, Percentile: 100}},
		{"the best of them", pool, 1600, Standing{Rank: 1, Percentile: 75}},
		{"below everyone", pool, 1300, Standing{Rank: 5, Percentile: 0}},
		{"to one place", []float64{1, 2, 3}, 3, Standing{Rank: 1, Percentile: 66.7}},
		{"no players", nil, 1500, Standing{Rank: 1, Percentile: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Place(tt.pool, tt.rating); got != tt.want {
				t.Errorf("Place(%v, %v) = %+v, want %+v", tt.pool, tt.rating, got, tt.want)
			}
		})
	}
}

func TestSpreadOfComputesArithmeticMean(t *testing.T) {
	if got := SpreadOf(nil, 50).Mean; got != 0 {
		t.Errorf("SpreadOf(nil).Mean = %v, want 0", got)
	}
	if got := SpreadOf([]float64{1500, 1600.25}, 50).Mean; got != 1550.1 {
		t.Errorf("SpreadOf([1500, 1600.25]).Mean = %v, want 1550.1", got)
	}
}
