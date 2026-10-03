package tournamentcsv

import (
	"io"
	"slices"

	"ttacommunity/backend/internal/gamename"
)

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

// ReadSeasonNotes reads a season notes CSV. A season or stage given twice keeps the first row.
func ReadSeasonNotes(file string, r io.Reader) (Result[SeasonNote], error) {
	return read(file, r, SeasonNotesHeader, parseSeasonNote, func(x SeasonNote) string {
		return x.Tournament + "\x00" + x.Season + "\x00" + x.Stage
	})
}

func parseSeasonNote(f func(int) string) (SeasonNote, Reason) {
	x := SeasonNote{Tournament: f(0), Season: f(1), Stage: f(2), Note: f(3), Source: f(4)}
	switch {
	case !slices.ContainsFunc(gamename.Tournaments, func(g gamename.Tournament) bool { return g.Slug == x.Tournament }):
		return x, BadSeries
	case !trimmed(x.Season):
		return x, BadSeason
	case x.Stage != "" && !trimmed(x.Stage):
		return x, BadDivision
	case !trimmed(x.Note):
		return x, BadReason
	case !trimmed(x.Source):
		return x, BadSource
	}
	return x, ""
}
