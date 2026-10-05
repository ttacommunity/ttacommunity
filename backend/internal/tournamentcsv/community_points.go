package tournamentcsv

import (
	"io"
	"strconv"
)

// CommunityPointsHeader is a community points table file's first line, exactly.
var CommunityPointsHeader = []string{"tournament", "from_edition", "group", "place", "points", "source"}

// CommunityPoints is one cell of the community leaderboard's points tables:
// what a place in a group (a division, stage, round or rung) of a tournament
// earns, from an edition on until a later table replaces it. Place 0 is the
// organisers' "0" row.
type CommunityPoints struct {
	Tournament  string
	FromEdition int
	Group       string
	Place       int
	Points      int
	Source      string
}

// ReadCommunityPoints reads a community points CSV. A cell given twice keeps the first row.
func ReadCommunityPoints(file string, r io.Reader) (Result[CommunityPoints], error) {
	return read(file, r, CommunityPointsHeader, parseCommunityPoints, func(x CommunityPoints) string {
		return x.Tournament + "\x00" + strconv.Itoa(x.FromEdition) + "\x00" + x.Group + "\x00" + strconv.Itoa(x.Place)
	})
}

func parseCommunityPoints(f func(int) string) (CommunityPoints, Reason) {
	edition, editionErr := strconv.Atoi(f(1))
	place, placeErr := strconv.Atoi(f(3))
	points, pointsErr := strconv.Atoi(f(4))
	x := CommunityPoints{Tournament: f(0), FromEdition: edition, Group: f(2), Place: place, Points: points, Source: f(5)}
	switch {
	case !slugShape.MatchString(x.Tournament):
		return x, BadSeries
	case editionErr != nil || edition < 2000 || edition > 2999:
		return x, BadEdition
	case !trimmed(x.Group):
		return x, BadDivision
	case placeErr != nil || place < 0:
		return x, BadPlace
	case pointsErr != nil || points < 0:
		return x, BadPoints
	case !trimmed(x.Source):
		return x, BadSource
	}
	return x, ""
}
