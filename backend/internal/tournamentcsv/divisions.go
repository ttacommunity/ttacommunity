package tournamentcsv

import (
	"fmt"
	"io"
)

// DivisionsHeader is a divisions file's first line, exactly.
var DivisionsHeader = []string{"series", "season", "division", "player", "source"}

// Division is one player's division in one season, and where that is recorded.
type Division struct {
	Series   string
	Season   int
	Division string
	Player   string
	Source   string
}

// ReadDivisions reads a divisions CSV. A player named twice in one series and
// season keeps the first row.
func ReadDivisions(file string, r io.Reader) (Result[Division], error) {
	return read(file, r, DivisionsHeader, parseDivision, func(d Division) string {
		return fmt.Sprint(d.Series, "\x00", d.Season, "\x00", d.Player)
	})
}

func parseDivision(f func(int) string) (Division, Reason) {
	s, okSeries := series(f(0))
	season, okSeason := positive(f(1))
	d := Division{Series: s, Season: season, Division: f(2), Player: f(3), Source: f(4)}
	switch {
	case !okSeries:
		return d, BadSeries
	case !okSeason:
		return d, BadSeason
	case !trimmed(d.Division):
		return d, BadDivision
	case !trimmed(d.Player):
		return d, BadPlayer
	case !trimmed(d.Source):
		return d, BadSource
	}
	return d, ""
}
