package tournamentcsv

import (
	"io"
	"slices"

	"ttacommunity/backend/internal/gamename"
)

// SeasonNotesHeader is a season notes file's first line, exactly.
var SeasonNotesHeader = []string{"tournament", "season", "note", "source"}

// SeasonNote is a note shown beside one season, such as what a special season was.
type SeasonNote struct {
	Tournament string
	Season     string
	Note       string
	Source     string
}

// ReadSeasonNotes reads a season notes CSV. A season given twice keeps the first row.
func ReadSeasonNotes(file string, r io.Reader) (Result[SeasonNote], error) {
	return read(file, r, SeasonNotesHeader, parseSeasonNote, func(x SeasonNote) string { return x.Tournament + "\x00" + x.Season })
}

func parseSeasonNote(f func(int) string) (SeasonNote, Reason) {
	x := SeasonNote{Tournament: f(0), Season: f(1), Note: f(2), Source: f(3)}
	switch {
	case !slices.ContainsFunc(gamename.Tournaments, func(g gamename.Tournament) bool { return g.Slug == x.Tournament }):
		return x, BadSeries
	case !trimmed(x.Season):
		return x, BadSeason
	case !trimmed(x.Note):
		return x, BadReason
	case !trimmed(x.Source):
		return x, BadSource
	}
	return x, ""
}
