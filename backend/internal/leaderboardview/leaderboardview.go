// Package leaderboardview selects the rows the site's leaderboard shows: the
// same filters, ranks, search and sort as the browser applies, for exports.
package leaderboardview

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"ttacommunity/backend/internal/round"
	"ttacommunity/backend/internal/store"
)

// upstreamCutoff is upstream's fixed date, kept for a run that recorded no last game.
var upstreamCutoff = time.Date(2025, 5, 31, 0, 0, 0, 0, time.UTC)

// InactiveCutoff returns the date on or before which a player's last game makes
// them inactive: 12 months before the run's last game. The site uses the same rule.
func InactiveCutoff(lastGame *time.Time) time.Time {
	if lastGame == nil {
		return upstreamCutoff
	}
	return lastGame.AddDate(-1, 0, 0)
}

// Query is the table's filters and sort, as the site's parameters carry them.
type Query struct {
	Status  string // active, all or inactive
	MinOpps int
	Title   string // a title code, or "" for any
	Search  string // names or country codes, comma-separated
	Sort    string // rank, name, c_rating, rating, opps, win_rate or last_played
	Order   string // asc or desc
	// Played, for a year's archive, names who played that year: they are its active players.
	Played map[string]bool
	// InactiveOnOrBefore is the live board's cutoff (InactiveCutoff).
	InactiveOnOrBefore time.Time
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
		active := p.LastPlayed != nil && p.LastPlayed.After(q.InactiveOnOrBefore)
		if q.Played != nil {
			active = q.Played[p.Name]
		}
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

	if terms := SearchTerms(q.Search); len(terms) > 0 {
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

// SearchTerms splits a search into its comma-separated terms, trimmed and lower-cased, dropping empty ones.
func SearchTerms(search string) []string {
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

// FromYear turns a year's board into leaderboard rows carrying the season or
// career record (stats "career"), and names who played that year, as the
// site's archive does.
func FromYear(b store.YearBoard, stats string) ([]store.LeaderboardRow, map[string]bool) {
	rows := make([]store.LeaderboardRow, 0, len(b.Rows))
	played := map[string]bool{}
	for _, y := range b.Rows {
		r := y.Season
		if stats == "career" {
			r = y.Career
		}
		rate := (float64(r.Wins) + 0.5*float64(r.Draws)) / float64(max(1, r.Opponents)) * 100
		rows = append(rows, store.LeaderboardRow{
			Name: y.Name, Rating: y.Rating, RD: y.RD, CRating: y.CRating, Games: r.Opponents, Opponents: r.Opponents,
			Wins: r.Wins, Losses: r.Losses, Draws: r.Draws, WinRate: round.TenthAway(rate), LastPlayed: y.LastPlayed,
			Country: y.Country, Title: y.Title, Peak: y.Peak,
		})
		if y.Season.Opponents > 0 {
			played[y.Name] = true
		}
	}
	return rows, played
}
