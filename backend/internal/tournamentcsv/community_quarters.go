package tournamentcsv

import (
	"io"
	"regexp"
	"strconv"
)

// CommunityQuartersHeader is a community quarters file's first line, exactly.
var CommunityQuartersHeader = []string{"tournament", "season", "quarter", "source"}

// CommunityQuarter is the quarter the community leaderboard's organisers count
// a tournament's season in, which decides the editions it scores in.
type CommunityQuarter struct {
	Tournament, Season string
	Year, Quarter      int
	Source             string
}

var quarterShape = regexp.MustCompile(`^(2\d{3}) Q([1-4])$`)

// ReadCommunityQuarters reads a community quarters CSV. A season given twice keeps the first row.
func ReadCommunityQuarters(file string, r io.Reader) (Result[CommunityQuarter], error) {
	return read(file, r, CommunityQuartersHeader, parseCommunityQuarter, func(x CommunityQuarter) string {
		return x.Tournament + "\x00" + x.Season
	})
}

func parseCommunityQuarter(f func(int) string) (CommunityQuarter, Reason) {
	x := CommunityQuarter{Tournament: f(0), Season: f(1), Source: f(3)}
	m := quarterShape.FindStringSubmatch(f(2))
	if m != nil {
		x.Year, _ = strconv.Atoi(m[1])
		x.Quarter, _ = strconv.Atoi(m[2])
	}
	switch {
	case !slugShape.MatchString(x.Tournament):
		return x, BadSeries
	case !trimmed(x.Season):
		return x, BadSeason
	case m == nil:
		return x, BadDate
	case !trimmed(x.Source):
		return x, BadSource
	}
	return x, ""
}
