package leaderboardview

import (
	"fmt"
	"slices"
	"testing"

	"ttacommunity/backend/internal/store"
)

func TestTopTenIsTheSitesFirstView(t *testing.T) {
	last := day("2026-06-01")
	var rows []store.LeaderboardRow
	for i := range 12 {
		rows = append(rows, row(fmt.Sprintf("p%02d", i), 2000-float64(i), 40, "2026-05-01", "", ""))
	}
	rows = append(rows,
		row("few opponents", 2500, 29, "2026-05-01", "", ""),
		row("inactive", 2400, 40, "2025-05-01", "", ""),
	)
	want := []string{"p00", "p01", "p02", "p03", "p04", "p05", "p06", "p07", "p08", "p09"}
	if got := TopTen(rows, &last); !slices.Equal(got, want) {
		t.Errorf("TopTen = %q, want %q: the ten best active players with 30 opponents or more", got, want)
	}
	if got := TopTen(rows[:3], &last); len(got) != 3 {
		t.Errorf("TopTen of three = %q, want all three", got)
	}
}

func TestTopTenChangeOf(t *testing.T) {
	tests := []struct {
		name          string
		before, after []string
		want          store.TopTenChange
		changed       bool
	}{
		{name: "the same", before: []string{"a", "b", "c"}, after: []string{"a", "b", "c"}},
		{name: "reordered below the top", before: []string{"a", "b", "c"}, after: []string{"a", "c", "b"}},
		{name: "a new number one", before: []string{"a", "b", "c"}, after: []string{"b", "a", "c"},
			want: store.TopTenChange{NumberOne: "b"}, changed: true},
		{name: "in and out", before: []string{"a", "b", "c", "d"}, after: []string{"a", "e", "b", "f"},
			want: store.TopTenChange{In: []string{"e", "f"}, Out: []string{"c", "d"}}, changed: true},
		{name: "a newcomer at the top", before: []string{"a", "b"}, after: []string{"z", "a"},
			want: store.TopTenChange{NumberOne: "z", In: []string{"z"}, Out: []string{"b"}}, changed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := TopTenChangeOf(tt.before, tt.after)
			if changed != tt.changed || got.NumberOne != tt.want.NumberOne || !slices.Equal(got.In, tt.want.In) || !slices.Equal(got.Out, tt.want.Out) {
				t.Errorf("TopTenChangeOf(%q, %q) = %+v, %v; want %+v, %v", tt.before, tt.after, got, changed, tt.want, tt.changed)
			}
		})
	}
}
