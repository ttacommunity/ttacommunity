// The top 10 as the site's leaderboard first shows it, and how one run's top 10 changed from another's, for the
// ratings run news posted to Discord.
package leaderboardview

import (
	"slices"
	"time"

	"ttacommunity/backend/internal/store"
)

// DefaultMinOpps is the fewest opponents a player needs on the live leaderboard as it first shows, the site's
// MIN_OPPONENTS.
const DefaultMinOpps = 30

// TopTen returns the names of the ten best players on rows as the site first shows them, for a run whose last game
// was lastGame: active players with DefaultMinOpps opponents or more, best first.
func TopTen(rows []store.LeaderboardRow, lastGame *time.Time) []string {
	shown := Select(rows, Query{Status: "active", MinOpps: DefaultMinOpps, Sort: "rank", Order: "asc", InactiveOnOrBefore: InactiveCutoff(lastGame)})
	var names []string
	for _, r := range shown[:min(10, len(shown))] {
		names = append(names, r.Name)
	}
	return names
}

// TopTenChangeOf says how the top 10 after differs from before: a new number one, and who came in and went out,
// each in its board's order. It reports false when neither changed; a reorder below the top changes nothing.
func TopTenChangeOf(before, after []string) (store.TopTenChange, bool) {
	var c store.TopTenChange
	if len(after) > 0 && (len(before) == 0 || after[0] != before[0]) {
		c.NumberOne = after[0]
	}
	for _, n := range after {
		if !slices.Contains(before, n) {
			c.In = append(c.In, n)
		}
	}
	for _, n := range before {
		if !slices.Contains(after, n) {
			c.Out = append(c.Out, n)
		}
	}
	return c, c.NumberOne != "" || len(c.In) > 0 || len(c.Out) > 0
}
