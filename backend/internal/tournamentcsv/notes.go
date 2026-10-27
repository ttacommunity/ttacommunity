package tournamentcsv

import "io"

// SeasonNotesHeader is a season notes file's first line, exactly.
var SeasonNotesHeader = []string{"tournament", "season", "stage", "note", "source"}

// SeasonNote is a note shown beside one season, or one of its stages (Stage
// "" is the season's own), such as what a special season was.
type SeasonNote struct {
	Tournament string
	Season     string
	Stage      string
	Note       string
	Source     string
}

// ReadSeasonNotes reads a season notes CSV, refusing a row naming a series not
// in series. A season or stage given twice keeps the first row.
func ReadSeasonNotes(file string, r io.Reader, series []string) (Result[SeasonNote], error) {
	parse := func(f func(int) string) (SeasonNote, Reason) { return parseSeasonNote(series, f) }
	return read(file, r, SeasonNotesHeader, parse, func(x SeasonNote) string {
		return x.Tournament + "\x00" + x.Season + "\x00" + x.Stage
	})
}

func parseSeasonNote(series []string, f func(int) string) (SeasonNote, Reason) {
	x := SeasonNote{Tournament: f(0), Season: f(1), Stage: f(2), Note: f(3), Source: f(4)}
	switch r := badPlace(series, x.Tournament, x.Season, x.Stage); {
	case r != "":
		return x, r
	case !trimmed(x.Note):
		return x, BadReason
	case !trimmed(x.Source):
		return x, BadSource
	}
	return x, ""
}
