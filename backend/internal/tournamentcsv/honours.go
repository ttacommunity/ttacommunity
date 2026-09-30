package tournamentcsv

import (
	"fmt"
	"io"
	"slices"
	"strconv"
)

// HonoursHeader is a career honours file's first line, exactly.
var HonoursHeader = []string{"series", "player", "gold", "silver", "bronze", "points", "source"}

// HonourSeries are the series whose career honours are recorded, not derived.
var HonourSeries = []string{"sodium_ladder", "mercurial_ladder", "q_and_d"}

// Honour is a player's career medals in a series, from its organisers' Hall
// of Fame, and its points as they wrote them, if any.
type Honour struct {
	Series string
	Player string
	Gold   int
	Silver int
	Bronze int
	Points *string
	Source string
}

// ReadHonours reads a career honours CSV. A player named twice in one series
// keeps the first row.
func ReadHonours(file string, r io.Reader) (Result[Honour], error) {
	return read(file, r, HonoursHeader, parseHonour, func(h Honour) string {
		return fmt.Sprint(h.Series, "\x00", h.Player)
	})
}

func parseHonour(f func(int) string) (Honour, Reason) {
	h := Honour{Series: f(0), Player: f(1), Source: f(6)}
	var medals [3]int
	okMedals := true
	for i := range medals {
		n, err := strconv.Atoi(f(2 + i))
		medals[i], okMedals = n, okMedals && err == nil && n >= 0
	}
	h.Gold, h.Silver, h.Bronze = medals[0], medals[1], medals[2]
	if p := f(5); p != "" {
		h.Points = &p
	}
	switch {
	case !slices.Contains(HonourSeries, h.Series):
		return h, BadSeries
	case !trimmed(h.Player):
		return h, BadPlayer
	case !okMedals:
		return h, BadMedals
	case !trimmed(h.Source):
		return h, BadSource
	}
	return h, ""
}
