package tournamentcsv

import (
	"fmt"
	"io"
	"time"
)

// FinishDatesHeader is a finish dates file's first line, exactly.
var FinishDatesHeader = []string{"series", "season", "finish_date", "source"}

// FinishDate is the day a season of a series officially finished, and where
// that is recorded. A World Championship's season is its year.
type FinishDate struct {
	Series string
	Season int
	Date   time.Time
	Source string
}

// ReadFinishDates reads a finish dates CSV, dates as YYYY-MM-DD. A season
// dated twice keeps the first row.
func ReadFinishDates(file string, r io.Reader) (Result[FinishDate], error) {
	return read(file, r, FinishDatesHeader, parseFinishDate, func(d FinishDate) string {
		return fmt.Sprint(d.Series, "\x00", d.Season)
	})
}

func parseFinishDate(f func(int) string) (FinishDate, Reason) {
	s, okSeries := series(f(0))
	season, okSeason := positive(f(1))
	date, err := time.Parse(time.DateOnly, f(2))
	d := FinishDate{Series: s, Season: season, Date: date, Source: f(3)}
	switch {
	case !okSeries:
		return d, BadSeries
	case !okSeason:
		return d, BadSeason
	case err != nil:
		return d, BadDate
	case !trimmed(d.Source):
		return d, BadSource
	}
	return d, ""
}
