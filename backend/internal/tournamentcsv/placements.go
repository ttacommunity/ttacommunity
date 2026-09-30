package tournamentcsv

import (
	"fmt"
	"io"
	"strconv"
)

// PlacementsHeader is a placements file's first line, exactly.
var PlacementsHeader = []string{"series", "season", "division", "player", "place", "points", "reason", "source"}

// Placement is a place the organisers recorded in a division's final
// standings, with the points if known, why it is recorded, and where from.
type Placement struct {
	Series   string
	Season   int
	Division string
	Player   string
	Place    int
	Points   *float64
	Reason   string
	Source   string
}

// ReadPlacements reads a placements CSV. Players may share a place; a player
// named twice in one division and season keeps the first row.
func ReadPlacements(file string, r io.Reader) (Result[Placement], error) {
	return read(file, r, PlacementsHeader, parsePlacement, func(p Placement) string {
		return fmt.Sprint(p.Series, "\x00", p.Season, "\x00", p.Division, "\x00", p.Player)
	})
}

func parsePlacement(f func(int) string) (Placement, Reason) {
	s, okSeries := series(f(0))
	season, okSeason := positive(f(1))
	place, okPlace := positive(f(4))
	p := Placement{Series: s, Season: season, Division: f(2), Player: f(3), Place: place, Reason: f(6), Source: f(7)}
	okPoints := true
	if f(5) != "" {
		v, err := strconv.ParseFloat(f(5), 64)
		p.Points, okPoints = &v, err == nil
	}
	switch {
	case !okSeries:
		return p, BadSeries
	case !okSeason:
		return p, BadSeason
	case !trimmed(p.Division):
		return p, BadDivision
	case !trimmed(p.Player):
		return p, BadPlayer
	case !okPlace:
		return p, BadPlace
	case !okPoints:
		return p, BadPoints
	case !trimmed(p.Reason):
		return p, BadReason
	case !trimmed(p.Source):
		return p, BadSource
	}
	return p, ""
}
