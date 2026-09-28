// Package matchcsv reads match CSVs and decides which rows are real games.
// The load job and admin imports both use it, so the rules exist once.
package matchcsv

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Header is the column order every match CSV starts with.
var Header = []string{
	"Tournament", "Date",
	"Player1", "Score1", "Player2", "Score2", "Player3", "Score3", "Player4", "Score4",
	"Player_Count",
}

// Reason says why a row was refused. It is stored with the refused row.
type Reason string

const (
	ColumnCount        Reason = "column_count"
	NoTournament       Reason = "no_tournament"
	BadDate            Reason = "bad_date"
	ScoreWithoutPlayer Reason = "score_without_player"
	PlayerWithoutScore Reason = "player_without_score"
	PlayerGap          Reason = "player_gap"
	BadScore           Reason = "bad_score"
	PlayerCount        Reason = "player_count"
	SamePlayerTwice    Reason = "same_player_twice"
	StandingsRow       Reason = "standings_row"
	BadName            Reason = "bad_name"
	Duplicate          Reason = "duplicate"
)

// Row is one CSV record as read, with the file and the line it starts on.
type Row struct {
	File   string
	Line   int
	Fields []string
}

// Player is one seat in a match and the score it finished with.
type Player struct {
	Name  string
	Score float64
}

// Match is a row that passed every check.
type Match struct {
	File       string
	Line       int
	Tournament string
	Date       time.Time
	Players    []Player
	Fields     []string
}

// Refusal is a row that failed a check, kept with the reason.
type Refusal struct {
	File   string
	Line   int
	Reason Reason
	Fields []string
}

// Read reads every record after the header from r. It fails if the file
// is not a match CSV. A record with the wrong column count is still returned,
// so Check can refuse it with a reason.
func Read(file string, r io.Reader) ([]Row, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	head, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("reading the header of %s: %w", file, err)
	}
	if !slices.Equal(head, Header) {
		return nil, fmt.Errorf("%s: header is %q, want %q", file, head, Header)
	}

	var rows []Row
	for {
		fields, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", file, err)
		}
		line, _ := cr.FieldPos(0)
		rows = append(rows, Row{File: file, Line: line, Fields: fields})
	}
}

var rankPrefix = regexp.MustCompile(`^[0-9]+\.`)

// Check turns a row into a match, or returns the reason it is refused.
func Check(row Row) (Match, Reason) {
	f := row.Fields
	if len(f) != len(Header) {
		return Match{}, ColumnCount
	}
	if strings.TrimSpace(f[0]) == "" {
		return Match{}, NoTournament
	}
	date, err := time.Parse(time.DateOnly, f[1])
	if err != nil {
		return Match{}, BadDate
	}

	var players []Player
	for seat := range 4 {
		name, score := f[2+2*seat], f[3+2*seat]
		switch {
		case name == "" && score != "":
			return Match{}, ScoreWithoutPlayer
		case name == "":
			continue
		case score == "":
			return Match{}, PlayerWithoutScore
		case len(players) < seat:
			return Match{}, PlayerGap
		}
		if rankPrefix.MatchString(name) || strings.Contains(name, "No finished games") {
			return Match{}, StandingsRow
		}
		if strings.TrimSpace(name) != name || strings.ContainsFunc(name, unicode.IsControl) {
			return Match{}, BadName
		}
		s, err := strconv.ParseFloat(score, 64)
		if err != nil || math.IsNaN(s) || math.IsInf(s, 0) {
			return Match{}, BadScore
		}
		players = append(players, Player{Name: name, Score: s})
	}

	count, err := strconv.Atoi(f[10])
	if err != nil || count != len(players) || count < 2 {
		return Match{}, PlayerCount
	}
	for i, p := range players {
		if slices.ContainsFunc(players[:i], func(q Player) bool { return q.Name == p.Name }) {
			return Match{}, SamePlayerTwice
		}
	}
	return Match{File: row.File, Line: row.Line, Tournament: f[0], Date: date, Players: players, Fields: f}, ""
}

// Dedupe keeps the first copy of each game and refuses the rest. Two rows are
// the same game when tournament, date, and every seat's name and score match.
func Dedupe(matches []Match) ([]Match, []Refusal) {
	seen := make(map[string]bool, len(matches))
	var kept []Match
	var dups []Refusal
	for _, m := range matches {
		parts := []string{m.Tournament, m.Date.Format(time.DateOnly)}
		for _, p := range m.Players {
			parts = append(parts, p.Name, strconv.FormatFloat(p.Score, 'g', -1, 64))
		}
		key := fmt.Sprintf("%q", parts)
		if seen[key] {
			dups = append(dups, Refusal{File: m.File, Line: m.Line, Reason: Duplicate, Fields: m.Fields})
			continue
		}
		seen[key] = true
		kept = append(kept, m)
	}
	return kept, dups
}
