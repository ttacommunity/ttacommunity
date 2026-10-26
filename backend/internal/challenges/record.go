// Package challenges holds the rules of the game's single-player challenges, and reads the committed challenge list
// and the old site's entries for the load job. It knows nothing of the database or HTTP.
package challenges

import (
	"strconv"
	"strings"
	"time"
)

// Player is whose an entry is: a member, or for an entry not yet linked to one, the name it was entered under.
type Player struct {
	Member *int64
	Name   string
}

// Key is equal for entries of the same player: the same member, or unlinked names that differ only in case.
func (p Player) Key() string {
	if p.Member != nil {
		return "member:" + strconv.FormatInt(*p.Member, 10)
	}
	return "name:" + strings.ToLower(p.Name)
}

// Entry is one domination entered for a challenge: its id, whose it is (a Player's Key), whether a Discord proof
// is linked, whether it came from a since-fixed game bug, and when it was entered.
type Entry struct {
	ID         int64
	Player     string
	Domination int
	Verified   bool
	Bugged     bool
	At         time.Time
}

// Record returns the index of the entry holding the record among one challenge's entries, or -1 when none counts.
// A bugged entry never counts, nor with verifiedOnly an unproven one. Each player's best counts: their highest
// domination, a verified one winning their own tie, then the earlier. Of the players' bests, the highest holds the
// record, and of equal ones the earlier, then the first entered.
func Record(entries []Entry, verifiedOnly bool) int {
	best := map[string]int{}
	for i, e := range entries {
		if e.Bugged || (verifiedOnly && !e.Verified) {
			continue
		}
		if j, ok := best[e.Player]; !ok || betterOwn(e, entries[j]) {
			best[e.Player] = i
		}
	}
	holder := -1
	for _, i := range best {
		if holder < 0 || ahead(entries[i], entries[holder]) {
			holder = i
		}
	}
	return holder
}

// betterOwn reports whether e beats prev as one player's best.
func betterOwn(e, prev Entry) bool {
	if e.Domination != prev.Domination {
		return e.Domination > prev.Domination
	}
	if e.Verified != prev.Verified {
		return e.Verified
	}
	return earlier(e, prev)
}

// ahead reports whether player best e ranks above player best other.
func ahead(e, other Entry) bool {
	if e.Domination != other.Domination {
		return e.Domination > other.Domination
	}
	return earlier(e, other)
}

func earlier(e, other Entry) bool {
	if !e.At.Equal(other.At) {
		return e.At.Before(other.At)
	}
	return e.ID < other.ID
}
