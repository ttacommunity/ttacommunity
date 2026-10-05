package store

import (
	"slices"
	"testing"
)

func TestGroupOrderFollowsDivisionRanks(t *testing.T) {
	tests := []struct {
		name     string
		in, want []string
	}{
		{"league divisions, top first, numbers in order",
			[]string{"Wood 3", "Bronze 10", "Gold 1", "Grandmaster", "Bronze 2", "Silver 1", "Master 1", "Platinum 1"},
			[]string{"Grandmaster", "Master 1", "Platinum 1", "Gold 1", "Silver 1", "Bronze 2", "Bronze 10", "Wood 3"}},
		{"Royal League ranks, a tiebreak group beside its rank",
			[]string{"Knight 1", "Baron 2", "Best Duke", "Emperor", "Count 1", "King 1", "Duke 1", "Viscount 1", "Marquess 1", "Prince 1"},
			[]string{"Emperor", "King 1", "Prince 1", "Duke 1", "Best Duke", "Marquess 1", "Count 1", "Viscount 1", "Baron 2", "Knight 1"}},
		{"upper bracket before lower", []string{"Lower 2", "Upper 10", "Lower 10", "Upper 2"}, []string{"Upper 2", "Upper 10", "Lower 2", "Lower 10"}},
		{"other names in natural order after", []string{"group 10", "Finals", "group 2"}, []string{"Finals", "group 2", "group 10"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slices.Clone(tt.in)
			slices.SortFunc(got, groupOrder)
			if !slices.Equal(got, tt.want) {
				t.Errorf("sorted %q = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
