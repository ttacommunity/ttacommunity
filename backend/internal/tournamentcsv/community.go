package tournamentcsv

import (
	"fmt"
	"io"
	"slices"
	"strconv"
)

// CommunityEditions are the community leaderboard's editions the site shows.
var CommunityEditions = []string{"2026", "2025", "2024", "race26"}

// StandingsHeader and ScoresHeader are the community files' first lines, exactly.
var (
	StandingsHeader = []string{"edition", "sheet_row", "rank", "player", "points", "events", "source"}
	ScoresHeader    = []string{"edition", "player", "event", "score", "source"}
)

// Standing is a player's place in an edition of the community leaderboard:
// rank, total points and how many events they played, as its maintainer
// published them, and the row's place in the sheet, which orders equal ranks.
type Standing struct {
	Edition  string
	SheetRow int
	Rank     int
	Player   string
	Points   float64
	Events   int
	Source   string
}

// Score is a player's points in one event of an edition, named by the
// leaderboard's column (wrld, ic_30, aus…).
type Score struct {
	Edition string
	Player  string
	Event   string
	Score   float64
	Source  string
}

// ReadCommunityStandings reads a community standings CSV. A player named
// twice in an edition keeps the first row.
func ReadCommunityStandings(file string, r io.Reader) (Result[Standing], error) {
	return read(file, r, StandingsHeader, parseStanding, func(s Standing) string {
		return fmt.Sprint(s.Edition, "\x00", s.Player)
	})
}

// ReadCommunityScores reads a community scores CSV. An event named twice for
// a player in an edition keeps the first row.
func ReadCommunityScores(file string, r io.Reader) (Result[Score], error) {
	return read(file, r, ScoresHeader, parseScore, func(s Score) string {
		return fmt.Sprint(s.Edition, "\x00", s.Player, "\x00", s.Event)
	})
}

func parseStanding(f func(int) string) (Standing, Reason) {
	row, okRow := positive(f(1))
	rank, okRank := positive(f(2))
	points, errPoints := strconv.ParseFloat(f(4), 64)
	events, errEvents := strconv.Atoi(f(5))
	s := Standing{Edition: f(0), SheetRow: row, Rank: rank, Player: f(3), Points: points, Events: events, Source: f(6)}
	switch {
	case !slices.Contains(CommunityEditions, s.Edition):
		return s, BadEdition
	case !okRow:
		return s, BadRow
	case !okRank:
		return s, BadPlace
	case !trimmed(s.Player):
		return s, BadPlayer
	case errPoints != nil:
		return s, BadPoints
	case errEvents != nil || events < 0:
		return s, BadEvents
	case !trimmed(s.Source):
		return s, BadSource
	}
	return s, ""
}

func parseScore(f func(int) string) (Score, Reason) {
	score, errScore := strconv.ParseFloat(f(3), 64)
	s := Score{Edition: f(0), Player: f(1), Event: f(2), Score: score, Source: f(4)}
	switch {
	case !slices.Contains(CommunityEditions, s.Edition):
		return s, BadEdition
	case !trimmed(s.Player):
		return s, BadPlayer
	case !trimmed(s.Event):
		return s, BadEvent
	case errScore != nil:
		return s, BadPoints
	case !trimmed(s.Source):
		return s, BadSource
	}
	return s, ""
}
