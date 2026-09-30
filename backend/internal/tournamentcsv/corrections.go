package tournamentcsv

import (
	"fmt"
	"io"
)

// CorrectionsHeader is a corrections file's first line, exactly.
var CorrectionsHeader = []string{"series", "name_season", "season", "reason", "source"}

// Correction gives the real season of games whose name gives the wrong one.
// A nil Season leaves those games out of tournament results.
type Correction struct {
	Series     string
	NameSeason int
	Season     *int
	Reason     string
	Source     string
}

// ReadCorrections reads a corrections CSV. A name season corrected twice in a
// series keeps the first row.
func ReadCorrections(file string, r io.Reader) (Result[Correction], error) {
	return read(file, r, CorrectionsHeader, parseCorrection, func(c Correction) string {
		return fmt.Sprint(c.Series, "\x00", c.NameSeason)
	})
}

func parseCorrection(f func(int) string) (Correction, Reason) {
	s, okSeries := series(f(0))
	named, okNamed := positive(f(1))
	c := Correction{Series: s, NameSeason: named, Reason: f(3), Source: f(4)}
	okSeason := true
	if f(2) != "" {
		var season int
		season, okSeason = positive(f(2))
		c.Season = &season
	}
	switch {
	case !okSeries:
		return c, BadSeries
	case !okNamed || !okSeason:
		return c, BadSeason
	case !trimmed(c.Reason):
		return c, BadReason
	case !trimmed(c.Source):
		return c, BadSource
	}
	return c, ""
}
