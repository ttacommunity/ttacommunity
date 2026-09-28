// Package leaderboardview selects the rows the site's leaderboard shows: the
// same filters, ranks, search and sort as the browser applies, for exports.
package leaderboardview

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"ttacommunity/backend/internal/store"
)

// InactiveOnOrBefore is upstream's fixed cutoff: a player whose last game is on
// or before it is inactive. The site uses the same date.
var InactiveOnOrBefore = time.Date(2025, 5, 31, 0, 0, 0, 0, time.UTC)

// Query is the table's filters and sort, as the site's parameters carry them.
type Query struct {
	Status  string // active, all or inactive
	MinOpps int
	Title   string // a title code, or "" for any
	Search  string // names or country codes, comma-separated
	Sort    string // rank, name, c_rating, rating, opps, win_rate or last_played
	Order   string // asc or desc
}

// Row is one shown player, with their rank among the filtered players.
type Row struct {
	store.LeaderboardRow
	DisplayRank int
	Active      bool
}

// Select returns the rows the site shows for q, in its order.
func Select(rows []store.LeaderboardRow, q Query) []Row {
	var pool []Row
	for _, p := range rows {
		active := p.LastPlayed != nil && p.LastPlayed.After(InactiveOnOrBefore)
		switch {
		case p.Opponents < q.MinOpps,
			q.Status == "active" && !active,
			q.Status == "inactive" && active,
			q.Title != "" && (p.Title == nil || p.Title.Code != q.Title):
			continue
		}
		pool = append(pool, Row{LeaderboardRow: p, Active: active})
	}
	slices.SortStableFunc(pool, func(a, b Row) int {
		return cmp.Or(cmp.Compare(b.CRating, a.CRating), strings.Compare(a.Name, b.Name))
	})
	for i := range pool {
		pool[i].DisplayRank = i + 1
	}

	if terms := searchTerms(q.Search); len(terms) > 0 {
		pool = slices.DeleteFunc(pool, func(p Row) bool { return !slices.ContainsFunc(terms, p.matches) })
	}
	dir := 1
	if q.Order == "desc" {
		dir = -1
	}
	slices.SortStableFunc(pool, func(a, b Row) int {
		return cmp.Or(dir*compareBy(q.Sort, a, b), cmp.Compare(a.DisplayRank, b.DisplayRank))
	})
	return pool
}

func searchTerms(search string) []string {
	var terms []string
	for _, t := range strings.Split(search, ",") {
		if t = strings.TrimSpace(t); t != "" {
			terms = append(terms, strings.ToLower(t))
		}
	}
	return terms
}

func (p Row) matches(term string) bool {
	return strings.Contains(strings.ToLower(p.Name), term) || (p.Country != nil && strings.ToLower(*p.Country) == term)
}

func compareBy(key string, a, b Row) int {
	switch key {
	case "name":
		return strings.Compare(a.Name, b.Name)
	case "c_rating":
		return cmp.Compare(a.CRating, b.CRating)
	case "rating":
		return cmp.Compare(a.Rating, b.Rating)
	case "opps":
		return cmp.Compare(a.Opponents, b.Opponents)
	case "win_rate":
		return cmp.Compare(a.WinRate, b.WinRate)
	case "last_played":
		return cmp.Compare(dateKey(a), dateKey(b))
	default:
		return cmp.Compare(a.DisplayRank, b.DisplayRank)
	}
}

func dateKey(p Row) string {
	if p.LastPlayed == nil {
		return ""
	}
	return p.LastPlayed.Format(time.DateOnly)
}
