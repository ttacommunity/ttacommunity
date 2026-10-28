// Package pairing pairs a round of a Swiss tournament by a chosen method: FIDE's Dutch system, with or without
// time zones as a tiebreak, or the closest time zones, the closest ratings or chance within each score group; and
// by an order (ratings, a lot, or chance) where the method leaves a choice.
package pairing

import (
	"errors"
	"fmt"
	"slices"
)

// Method is how a round is paired.
type Method string

// The methods. Methods lists them all.
const (
	Dutch         Method = "dutch"
	DutchTimezone Method = "dutch-timezone"
	Timezone      Method = "timezone"
	Rating        Method = "rating"
	Random        Method = "random"
)

// Methods are the methods Pair knows, in the order they are offered.
var Methods = []Method{Dutch, DutchTimezone, Timezone, Rating, Random}

// Order is what decides where a method leaves a choice: the Dutch system's starting ranks (set in Player.TPN by
// the caller), and the tiebreak among equally close time zones.
type Order string

// The orders. Orders lists them all.
const (
	ByRating Order = "rating"
	ByLot    Order = "lot"
	ByChance Order = "random"
)

// Orders are the orders Pair knows.
var Orders = []Order{ByRating, ByLot, ByChance}

// Float is a float a player received in a round: down when they met a lower score or had a bye, up when they met
// a higher score.
type Float int8

// The floats.
const (
	NoFloat Float = iota
	Down
	Up
)

// Player is a player as the pairing sees them: their points so far, starting rank (TPN, 1 the top), place in the
// standings (1 the top, 0 when not known), rating (Rated false when they have none), the zone's offset from UTC in
// minutes, who they have met, whether they had a bye (or a win without playing), how many rounds they have not
// played, and their floats in the previous round (Floats[0]) and the one before (Floats[1]).
type Player struct {
	ID        int64
	Points    float64
	TPN       int
	Place     int
	Rating    float64
	Rated     bool
	Offset    int
	Opponents []int64
	HadBye    bool
	Unplayed  int
	Floats    [2]Float
}

// Match is one pairing, with its zone gap in minutes and rating gap.
type Match struct {
	A, B      int64
	ZoneGap   int
	RatingGap float64
}

// Round is a pairing of everyone: the matches, and the bye when the count is odd.
type Round struct {
	Matches []Match
	Bye     *int64
}

// ErrTooFew means fewer than two players to pair.
var ErrTooFew = errors.New("a round needs at least two players")

// ErrNoPairing means no round pairs every player without someone meeting an opponent again.
var ErrNoPairing = errors.New("no round pairs everyone without someone meeting an opponent again")

// ErrUnknownMethod means a method Pair does not know.
var ErrUnknownMethod = errors.New("unknown pairing method")

// ErrUnknownOrder means an order Pair does not know.
var ErrUnknownOrder = errors.New("unknown pairing order")

// Pair pairs the players for a round by the method, never two who have met before. The order decides among equally
// close time zones (Timezone), and ranks the bye there: by rating, by the players' TPN (a lot), or by chance; the
// Dutch methods follow Player.TPN, and Rating and Random have their own. The seed decides what is left to chance, so
// the same players give the same round. Only ByRating ever reads a rating. It returns ErrNoPairing when no round
// pairs everyone without a repeat.
func Pair(players []Player, m Method, o Order, seed uint64) (Round, error) {
	if !slices.Contains(Methods, m) {
		return Round{}, fmt.Errorf("%w: %q", ErrUnknownMethod, m)
	}
	if !slices.Contains(Orders, o) {
		return Round{}, fmt.Errorf("%w: %q", ErrUnknownOrder, o)
	}
	if len(players) < 2 {
		return Round{}, ErrTooFew
	}
	switch m {
	case Dutch:
		return pairDutch(players, false)
	case DutchTimezone:
		return pairDutch(players, true)
	case Timezone:
		return closest(players, criteria{timezone: true, order: o}, seed)
	case Rating:
		return closest(players, criteria{order: ByRating}, seed)
	default:
		return closest(players, criteria{order: ByChance}, seed)
	}
}

// ZoneGap is the distance in minutes between two UTC offsets on the 24-hour circle.
func ZoneGap(a, b int) int {
	d := a - b
	if d < 0 {
		d = -d
	}
	d %= 1440
	return min(d, 1440-d)
}

// scoreGroups splits ranked players into runs of equal points, highest first.
func scoreGroups(ranked []Player) [][]Player {
	var groups [][]Player
	for i, p := range ranked {
		if i == 0 || p.Points != ranked[i-1].Points {
			groups = append(groups, nil)
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], p)
	}
	return groups
}

func met(a, b Player) bool {
	return slices.Contains(a.Opponents, b.ID) || slices.Contains(b.Opponents, a.ID)
}

func ratingOf(p Player) float64 {
	if p.Rated {
		return p.Rating
	}
	return unratedAs
}

func ratingGap(a, b Player) float64 {
	d := ratingOf(a) - ratingOf(b)
	if d < 0 {
		return -d
	}
	return d
}

func match(a, b Player) Match {
	return Match{A: a.ID, B: b.ID, ZoneGap: ZoneGap(a.Offset, b.Offset), RatingGap: ratingGap(a, b)}
}
