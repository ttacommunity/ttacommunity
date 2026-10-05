package tournamentcsv

import (
	"io"
	"regexp"
	"strconv"
	"time"
)

// CGETournamentsHeader is a CGE catalogue file's first line, exactly.
var CGETournamentsHeader = []string{
	"cge_tournament", "name", "status", "organiser", "created", "started", "finished",
	"description", "mentions_glicko", "links", "tournament", "season",
}

// CGETournament is one tournament as its CGE page shows it. Tournament and
// Season name which of ours it is, or are "" for one we do not rate or collect.
type CGETournament struct {
	ID                                 int
	Name, Status, Organiser            string
	Created, Started, Finished         *time.Time
	Description, MentionsGlicko, Links string
	Tournament, Season                 string
}

var slugShape = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ReadCGETournaments reads a CGE catalogue CSV. A tournament given twice keeps the first row.
func ReadCGETournaments(file string, r io.Reader) (Result[CGETournament], error) {
	return read(file, r, CGETournamentsHeader, parseCGETournament, func(x CGETournament) string {
		return strconv.Itoa(x.ID)
	})
}

func parseCGETournament(f func(int) string) (CGETournament, Reason) {
	id, err := strconv.Atoi(f(0))
	x := CGETournament{
		ID: id, Name: f(1), Status: f(2), Organiser: f(3), Description: f(7), MentionsGlicko: f(8), Links: f(9),
		Tournament: f(10), Season: f(11),
	}
	var bad bool
	day := func(s string) *time.Time {
		if s == "" {
			return nil
		}
		d, err := time.Parse(time.DateOnly, s)
		bad = bad || err != nil
		return &d
	}
	x.Created, x.Started, x.Finished = day(f(4)), day(f(5)), day(f(6))
	switch {
	case err != nil || id < 1:
		return x, BadEvent
	case !trimmed(x.Name):
		return x, BadRow
	case bad:
		return x, BadDate
	case x.Tournament != "" && !slugShape.MatchString(x.Tournament):
		return x, BadSeries
	case x.Tournament == "" && x.Season != "":
		return x, BadSeason
	}
	return x, ""
}
