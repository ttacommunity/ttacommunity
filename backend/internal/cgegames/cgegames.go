// Package cgegames reads games as CGE's tournament pages show them: each
// game's CGE identity and finish date, and each seat's CGE user, score and
// how it ended. The load job attaches them to the games already loaded.
package cgegames

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Header is a CGE games file's first line, exactly.
var Header = []string{
	"tournament", "cge_tournament", "cge_season", "group", "game", "finished", "spectate_code", "unplayed",
	"player1", "score1", "status1", "cge_id1", "banned1", "player2", "score2", "status2", "cge_id2", "banned2",
	"player3", "score3", "status3", "cge_id3", "banned3", "player4", "score4", "status4", "cge_id4", "banned4",
}

// Outcome is how a seat ended.
type Outcome string

const (
	Finished Outcome = "finished"
	Resigned Outcome = "resigned"
	TimedOut Outcome = "timed_out"
)

// Reason says why a row was refused.
type Reason string

const (
	BadTournament Reason = "bad_tournament"
	BadCGEID      Reason = "bad_cge_id"
	BadGame       Reason = "bad_game"
	BadDate       Reason = "bad_date"
	BadSeat       Reason = "bad_seat"
	BadOutcome    Reason = "bad_outcome"
)

// Seat is one player of a game. Score follows the match files' convention:
// a resignation scores minus its place, a timeout -5.
type Seat struct {
	Name    string
	Score   float64
	Outcome Outcome
	CGEUser int
	Banned  bool
}

// Game is one game on CGE. Tournament is our tournament it belongs to;
// Finished is nil for a game never played.
type Game struct {
	Tournament    string
	CGETournament int
	CGESeason     int
	Group         string
	Number        int
	Finished      *time.Time
	Code          string
	Unplayed      bool
	Seats         []Seat
	Line          int
}

// Refusal is a row that failed a check, by its line.
type Refusal struct {
	Line   int
	Reason Reason
}

// Result is what Read found: the games, and the rows refused.
type Result struct {
	Games   []Game
	Refused []Refusal
}

// statuses maps CGE's status words to outcomes.
var statuses = map[string]Outcome{"finished": Finished, "resigned": Resigned, "timeout": TimedOut}

// Read reads a CGE games CSV. It fails if the file is not one.
func Read(file string, r io.Reader) (Result, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	head, err := cr.Read()
	if err != nil && !errors.Is(err, io.EOF) {
		return Result{}, fmt.Errorf("%s: %w", file, err)
	}
	if !slices.Equal(head, Header) {
		return Result{}, fmt.Errorf("%s: want the header %s", file, strings.Join(Header, ","))
	}
	var got Result
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return got, nil
		}
		if err != nil {
			return Result{}, fmt.Errorf("%s: %w", file, err)
		}
		g, reason := parse(rec)
		if reason != "" {
			got.Refused = append(got.Refused, Refusal{Line: line, Reason: reason})
			continue
		}
		g.Line = line
		got.Games = append(got.Games, g)
	}
}

func parse(rec []string) (Game, Reason) {
	f := func(i int) string {
		if i < len(rec) {
			return rec[i]
		}
		return ""
	}
	g := Game{Tournament: f(0), Group: f(3), Code: f(6), Unplayed: f(7) != ""}
	var err1, err2, err3 error
	g.CGETournament, err1 = strconv.Atoi(f(1))
	g.CGESeason, err2 = strconv.Atoi(f(2))
	g.Number, err3 = strconv.Atoi(f(4))
	switch {
	case strings.TrimSpace(g.Tournament) == "":
		return g, BadTournament
	case err1 != nil || err2 != nil || g.CGETournament < 1 || g.CGESeason < 0:
		return g, BadCGEID
	case err3 != nil || g.Number < 1:
		return g, BadGame
	}
	if f(5) != "" {
		d, err := time.Parse(time.DateOnly, f(5))
		if err != nil {
			return g, BadDate
		}
		g.Finished = &d
	}
	for s := range 4 {
		name, score, status, id := f(8+5*s), f(9+5*s), f(10+5*s), f(11+5*s)
		if name == "" {
			continue
		}
		points, err := strconv.ParseFloat(score, 64)
		if err != nil {
			return g, BadSeat
		}
		outcome, ok := statuses[status]
		if !ok {
			return g, BadOutcome
		}
		user, _ := strconv.Atoi(id)
		g.Seats = append(g.Seats, Seat{Name: name, Score: points, Outcome: outcome, CGEUser: user, Banned: f(12+5*s) != ""})
	}
	if len(g.Seats) < 2 {
		return g, BadSeat
	}
	return g, ""
}
