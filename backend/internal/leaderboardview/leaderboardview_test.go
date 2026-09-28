package leaderboardview

import (
	"slices"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

func row(name string, cRating float64, opponents int, lastPlayed string, country, title string) store.LeaderboardRow {
	r := store.LeaderboardRow{Name: name, CRating: cRating, Rating: cRating + 100, Opponents: opponents, WinRate: 50}
	if lastPlayed != "" {
		d := day(lastPlayed)
		r.LastPlayed = &d
	}
	if country != "" {
		r.Country = &country
	}
	if title != "" {
		r.Title = &store.Title{Code: title}
	}
	return r
}

func names(rows []Row) []string {
	var got []string
	for _, r := range rows {
		got = append(got, r.Name)
	}
	return got
}

var defaults = Query{Status: "active", MinOpps: 30, Sort: "rank", Order: "asc"}

func TestSelectKeepsTheSamePoolAsTheSite(t *testing.T) {
	rows := []store.LeaderboardRow{
		row("active", 1800, 40, "2026-01-01", "", ""),
		row("few", 1990, 29, "2026-01-01", "", ""),
		row("retired", 1900, 50, "2025-05-31", "", ""),
		row("never", 1950, 50, "", "", ""),
		row("gm", 1700, 40, "2026-01-01", "", "GM"),
	}
	tests := []struct {
		name  string
		query Query
		want  []string
		ranks []int
	}{
		{name: "defaults", query: defaults, want: []string{"active", "gm"}, ranks: []int{1, 2}},
		{name: "inactive", query: Query{Status: "inactive", MinOpps: 30, Sort: "rank", Order: "asc"}, want: []string{"never", "retired"}},
		{name: "all", query: Query{Status: "all", Sort: "rank", Order: "asc"}, want: []string{"few", "never", "retired", "active", "gm"}},
		{name: "title", query: Query{Status: "active", MinOpps: 30, Title: "GM", Sort: "rank", Order: "asc"}, want: []string{"gm"}, ranks: []int{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Select(rows, tt.query)
			if !slices.Equal(names(got), tt.want) {
				t.Errorf("Select = %q, want %q", names(got), tt.want)
			}
			for i, want := range tt.ranks {
				if got[i].DisplayRank != want {
					t.Errorf("%s's rank = %d, want %d", got[i].Name, got[i].DisplayRank, want)
				}
			}
		})
	}
}

func TestSelectMarksActivePlayers(t *testing.T) {
	got := Select([]store.LeaderboardRow{row("new", 1500, 5, "2026-01-01", "", ""), row("old", 1400, 5, "2020-01-01", "", "")},
		Query{Status: "all", Sort: "rank", Order: "asc"})
	if !got[0].Active || got[1].Active {
		t.Errorf("active = %t, %t; want true, false", got[0].Active, got[1].Active)
	}
}

func TestSelectSearchesNamesAndCountries(t *testing.T) {
	rows := []store.LeaderboardRow{
		row("Annika", 1900, 40, "2026-01-01", "PL", ""),
		row("ann", 1800, 40, "2026-01-01", "", ""),
		row("Bob", 1700, 40, "2026-01-01", "DE", ""),
		row("Hannah", 1600, 40, "2026-01-01", "", ""),
	}
	tests := []struct {
		search string
		want   []string
		ranks  []int
	}{
		{search: "ANN", want: []string{"Annika", "ann", "Hannah"}, ranks: []int{1, 2, 4}},
		{search: "de", want: []string{"Bob"}},
		{search: "bob, hann ,", want: []string{"Bob", "Hannah"}},
		{search: "zzz"},
	}
	for _, tt := range tests {
		t.Run(tt.search, func(t *testing.T) {
			q := defaults
			q.Search = tt.search
			got := Select(rows, q)
			if !slices.Equal(names(got), tt.want) {
				t.Errorf("Select(search %q) = %q, want %q", tt.search, names(got), tt.want)
			}
			for i, want := range tt.ranks {
				if got[i].DisplayRank != want {
					t.Errorf("%s's rank = %d, want %d", got[i].Name, got[i].DisplayRank, want)
				}
			}
		})
	}
}

func TestSelectSortsEitherWay(t *testing.T) {
	rows := []store.LeaderboardRow{
		row("b", 1800, 60, "2026-01-01", "", ""),
		row("a", 1900, 40, "2026-01-01", "", ""),
		row("c", 1700, 50, "2026-02-01", "", ""),
	}
	tests := []struct {
		sort, order string
		want        []string
	}{
		{"rank", "asc", []string{"a", "b", "c"}},
		{"rank", "desc", []string{"c", "b", "a"}},
		{"name", "desc", []string{"c", "b", "a"}},
		{"opps", "desc", []string{"b", "c", "a"}},
		{"last_played", "desc", []string{"c", "a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.sort+" "+tt.order, func(t *testing.T) {
			q := defaults
			q.Sort, q.Order = tt.sort, tt.order
			if got := names(Select(rows, q)); !slices.Equal(got, tt.want) {
				t.Errorf("Select(sort %s %s) = %q, want %q", tt.sort, tt.order, got, tt.want)
			}
		})
	}
}

func day(s string) time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return d
}
