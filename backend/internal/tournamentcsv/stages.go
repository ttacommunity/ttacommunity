package tournamentcsv

import (
	"fmt"
	"io"
	"slices"
	"strconv"
)

// CGEStagesHeader is a CGE stages file's first line, exactly.
var CGEStagesHeader = []string{"tournament", "season", "stage", "cge_tournament", "cge_season"}

// CGEStage links one of our stages to the CGE tournament season it was
// played as. Season and Stage are the catalogue's labels; Stage "" is a
// season's only stage.
type CGEStage struct {
	Tournament    string
	Season        string
	Stage         string
	CGETournament int
	CGESeason     int
}

// ReadCGEStages reads a CGE stages CSV, refusing a row naming a series not in
// series. A stage given twice keeps the first row.
func ReadCGEStages(file string, r io.Reader, series []string) (Result[CGEStage], error) {
	parse := func(f func(int) string) (CGEStage, Reason) { return parseCGEStage(series, f) }
	return read(file, r, CGEStagesHeader, parse, func(x CGEStage) string {
		return fmt.Sprint(x.Tournament, "\x00", x.Season, "\x00", x.Stage)
	})
}

func parseCGEStage(series []string, f func(int) string) (CGEStage, Reason) {
	t, errT := strconv.Atoi(f(3))
	s, errS := strconv.Atoi(f(4))
	x := CGEStage{Tournament: f(0), Season: f(1), Stage: f(2), CGETournament: t, CGESeason: s}
	switch {
	case !slices.Contains(series, x.Tournament):
		return x, BadSeries
	case !trimmed(x.Season):
		return x, BadSeason
	case x.Stage != "" && !trimmed(x.Stage):
		return x, BadDivision
	case errT != nil || errS != nil || t < 1 || s < 1:
		return x, BadEvent
	}
	return x, ""
}
