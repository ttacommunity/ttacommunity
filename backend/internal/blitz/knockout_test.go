package blitz_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"ttacommunity/backend/internal/blitz"
)

// pairText lists pairs as "a-b" in order.
func pairText(pairs [][2]int64) []string {
	out := make([]string, len(pairs))
	for i, p := range pairs {
		out[i] = fmt.Sprintf("%d-%d", p[0], p[1])
	}
	return out
}

func TestFirstKnockoutPairsTheBestWithTheWorstInBracketOrder(t *testing.T) {
	tests := []struct {
		name   string
		seeded []int64
		want   []string
	}{
		{"a final", []int64{11, 12}, []string{"11-12"}},
		{"semi-finals", []int64{11, 12, 13, 14}, []string{"11-14", "12-13"}},
		{"quarter-finals", []int64{11, 12, 13, 14, 15, 16, 17, 18}, []string{"11-18", "14-15", "12-17", "13-16"}},
		{"a round of sixteen", []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			[]string{"1-16", "8-9", "4-13", "5-12", "2-15", "7-10", "3-14", "6-11"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := blitz.FirstKnockout(tt.seeded)
			if err != nil {
				t.Fatalf("FirstKnockout(%v): %v", tt.seeded, err)
			}
			if !slices.Equal(pairText(got), tt.want) {
				t.Errorf("FirstKnockout(%v) = %v, want %v", tt.seeded, pairText(got), tt.want)
			}
		})
	}
}

func TestNextKnockoutPairsNeighbouringWinners(t *testing.T) {
	tests := []struct {
		name    string
		winners []int64
		want    []string
	}{
		{"the semi-finals' winners", []int64{14, 12}, []string{"14-12"}},
		{"the quarter-finals' winners, upsets and all", []int64{18, 14, 12, 16}, []string{"18-14", "12-16"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := blitz.NextKnockout(tt.winners)
			if err != nil {
				t.Fatalf("NextKnockout(%v): %v", tt.winners, err)
			}
			if !slices.Equal(pairText(got), tt.want) {
				t.Errorf("NextKnockout(%v) = %v, want %v", tt.winners, pairText(got), tt.want)
			}
		})
	}
}

func TestAKnockoutIsRefusedWithoutAPowerOfTwo(t *testing.T) {
	tests := []struct {
		name    string
		pair    func([]int64) ([][2]int64, error)
		players []int64
	}{
		{"a first round of nobody", blitz.FirstKnockout, nil},
		{"a first round of one", blitz.FirstKnockout, []int64{1}},
		{"a first round of six", blitz.FirstKnockout, []int64{1, 2, 3, 4, 5, 6}},
		{"a next round from the champion alone", blitz.NextKnockout, []int64{1}},
		{"a next round from three winners", blitz.NextKnockout, []int64{1, 2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := tt.pair(tt.players); !errors.Is(err, blitz.ErrBracket) {
				t.Errorf("pairing %v = %v, %v; want ErrBracket", tt.players, got, err)
			}
		})
	}
}

func TestWinnerNamesWhoGoesThrough(t *testing.T) {
	tests := []struct {
		name   string
		result blitz.Result
		want   int64
		ok     bool
	}{
		{"a win for a", blitz.WinA, 1, true},
		{"a win for b", blitz.WinB, 2, true},
		{"a draw", blitz.Draw, 0, false},
		{"a double loss", blitz.DoubleLoss, 0, false},
		{"no result", blitz.Undecided, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := blitz.Winner(blitz.Played{A: 1, B: 2, Result: tt.result})
			if got != tt.want || ok != tt.ok {
				t.Errorf("Winner(%s) = %d, %v; want %d, %v", tt.result, got, ok, tt.want, tt.ok)
			}
		})
	}
}
