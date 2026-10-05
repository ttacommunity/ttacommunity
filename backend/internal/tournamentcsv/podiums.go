package tournamentcsv

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"time"

	"ttacommunity/backend/internal/gamename"
)

// PodiumsHeader is a podiums file's first line, exactly.
var PodiumsHeader = []string{"cup", "season", "final", "finished", "place", "player", "source"}

// Podium is one place on a cup year's podium. Season is the catalogue's label
// ("2024"); Final names one of several finals in a year, or is "". Finished is
// the day that final finished.
type Podium struct {
	Cup      string
	Season   string
	Final    string
	Finished time.Time
	Place    int
	Player   string
	Source   string
}

// ReadPodiums reads a podiums CSV. A place given twice in a final keeps the first row.
func ReadPodiums(file string, r io.Reader) (Result[Podium], error) {
	return read(file, r, PodiumsHeader, parsePodium, func(x Podium) string {
		return fmt.Sprint(x.Cup, "\x00", x.Season, "\x00", x.Final, "\x00", x.Place)
	})
}

func parsePodium(f func(int) string) (Podium, Reason) {
	finished, dateErr := time.Parse(time.DateOnly, f(3))
	place, err := strconv.Atoi(f(4))
	x := Podium{Cup: f(0), Season: f(1), Final: f(2), Finished: finished, Place: place, Player: f(5), Source: f(6)}
	switch {
	case !slices.ContainsFunc(gamename.Tournaments, func(t gamename.Tournament) bool { return t.Slug == x.Cup }):
		return x, BadSeries
	case !trimmed(x.Season):
		return x, BadSeason
	case x.Final != "" && !trimmed(x.Final):
		return x, BadDivision
	case dateErr != nil:
		return x, BadDate
	case err != nil || place < 1 || place > 3:
		return x, BadPlace
	case !trimmed(x.Player):
		return x, BadPlayer
	case !trimmed(x.Source):
		return x, BadSource
	}
	return x, ""
}
