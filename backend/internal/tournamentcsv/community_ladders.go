package tournamentcsv

import (
	"io"
	"strconv"
	"strings"
)

// CommunityLaddersHeader is a community ladders file's first line, exactly.
var CommunityLaddersHeader = []string{"tournament", "from_edition", "ratio", "half_steps_from", "shares", "rounds", "source"}

// CommunityLadder is how the community leaderboard scores a ladder's rounds:
// a rung's winner earns 1000 × Ratio^steps (a step a rung, or half a step a
// rung after HalfStepsFrom; 0 for never), each place its share of that.
// Rounds is how many latest rounds a rolling edition sums.
type CommunityLadder struct {
	Tournament    string
	FromEdition   int
	Ratio         float64
	HalfStepsFrom int
	Shares        []float64
	Rounds        int
	Source        string
}

// ReadCommunityLadders reads a community ladders CSV. A ladder and edition given twice keeps the first row.
func ReadCommunityLadders(file string, r io.Reader) (Result[CommunityLadder], error) {
	return read(file, r, CommunityLaddersHeader, parseCommunityLadder, func(x CommunityLadder) string {
		return x.Tournament + "\x00" + strconv.Itoa(x.FromEdition)
	})
}

func parseCommunityLadder(f func(int) string) (CommunityLadder, Reason) {
	edition, editionErr := strconv.Atoi(f(1))
	ratio, ratioErr := strconv.ParseFloat(f(2), 64)
	rounds, roundsErr := strconv.Atoi(f(5))
	x := CommunityLadder{Tournament: f(0), FromEdition: edition, Ratio: ratio, Rounds: rounds, Source: f(6)}
	halfOK := true
	if f(3) != "" {
		x.HalfStepsFrom, halfOK = positive(f(3))
		halfOK = halfOK && x.HalfStepsFrom > 1
	}
	sharesOK := f(4) != ""
	for _, p := range strings.Split(f(4), " ") {
		v, err := strconv.ParseFloat(p, 64)
		sharesOK = sharesOK && err == nil && v >= 0 && v <= 1
		x.Shares = append(x.Shares, v)
	}
	switch {
	case !slugShape.MatchString(x.Tournament):
		return x, BadSeries
	case editionErr != nil || edition < 2000 || edition > 2999:
		return x, BadEdition
	case ratioErr != nil || ratio <= 0 || ratio >= 1, !halfOK, !sharesOK:
		return x, BadPoints
	case roundsErr != nil || rounds < 1:
		return x, BadRow
	case !trimmed(x.Source):
		return x, BadSource
	}
	return x, ""
}
