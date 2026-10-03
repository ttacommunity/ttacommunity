package tournamentcsv

import (
	"io"
	"math"
	"slices"
	"strconv"
)

// LadderWinsHeader is a ladder wins file's first line, exactly.
var LadderWinsHeader = []string{"ladder", "player", "wins", "source"}

// Ladders are the tournaments whose champions are counted as top-tier wins.
var Ladders = []string{"mercurial_ladder", "sodium_ladder", "transcontinental_ladder"}

// LadderWins is one player's wins in a ladder's top tier, in halves for a tie.
type LadderWins struct {
	Ladder string
	Player string
	Wins   float64
	Source string
}

// ReadLadderWins reads a ladder wins CSV. A player given twice in a ladder keeps the first row.
func ReadLadderWins(file string, r io.Reader) (Result[LadderWins], error) {
	return read(file, r, LadderWinsHeader, parseLadderWins, func(x LadderWins) string { return x.Ladder + "\x00" + x.Player })
}

func parseLadderWins(f func(int) string) (LadderWins, Reason) {
	wins, err := strconv.ParseFloat(f(2), 64)
	x := LadderWins{Ladder: f(0), Player: f(1), Wins: wins, Source: f(3)}
	switch {
	case !slices.Contains(Ladders, x.Ladder):
		return x, BadSeries
	case !trimmed(x.Player):
		return x, BadPlayer
	case err != nil || wins <= 0 || math.Mod(wins*2, 1) != 0:
		return x, BadPoints
	case !trimmed(x.Source):
		return x, BadSource
	}
	return x, ""
}
