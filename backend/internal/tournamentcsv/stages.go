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

// badPlace is the reason a row's tournament, season or stage is refused: a tournament not in series, or a season or
// stage with spaces round it (a stage may be empty); "" when all three are good.
func badPlace(series []string, tournament, season, stage string) Reason {
	switch {
	case !slices.Contains(series, tournament):
		return BadSeries
	case !trimmed(season):
		return BadSeason
	case stage != "" && !trimmed(stage):
		return BadDivision
	}
	return ""
}

func parseCGEStage(series []string, f func(int) string) (CGEStage, Reason) {
	t, errT := strconv.Atoi(f(3))
	s, errS := strconv.Atoi(f(4))
	x := CGEStage{Tournament: f(0), Season: f(1), Stage: f(2), CGETournament: t, CGESeason: s}
	switch r := badPlace(series, x.Tournament, x.Season, x.Stage); {
	case r != "":
		return x, r
	case errT != nil || errS != nil || t < 1 || s < 1:
		return x, BadEvent
	}
	return x, ""
}
