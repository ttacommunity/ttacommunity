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

// Entry is one domination entered for a challenge: its id, the challenge, whose it is (a Player's Key) and the name
// shown for them, its Discord proof link ("" for none) and whether one is linked, whether it came from a since-fixed
// game bug, and when it was entered.
type Entry struct {
	ID         int64
	Challenge  string
	Player     string
	Name       string
	Domination int
	ProofURL   string
	Verified   bool
	Bugged     bool
	At         time.Time
}

// betterOwn reports whether e beats prev as one player's best: the higher domination, a verified one winning their
// own tie, then the earlier.
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
