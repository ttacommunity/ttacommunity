// Package divisioncsv reads recorded tournament divisions: which division a
// player played in one season, for seasons whose match names do not say.
package divisioncsv

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// Header is the file's first line, exactly.
var Header = []string{"series", "season", "division", "player", "source"}

// Series are the series a division may be recorded for.
var Series = []string{"international", "intermezzo", "royal_league"}

// Reason says why a row was refused.
type Reason string

// The reasons a row is refused.
const (
	BadSeries   Reason = "bad_series"
	BadSeason   Reason = "bad_season"
	BadDivision Reason = "bad_division"
	BadPlayer   Reason = "bad_player"
	BadSource   Reason = "bad_source"
	Duplicate   Reason = "duplicate"
)

// Division is one player's division in one season, and where that is recorded.
type Division struct {
	Series   string
	Season   int
	Division string
	Player   string
	Source   string
}

// Refusal is a row that was not used. It names the line, never the player.
type Refusal struct {
	File   string
	Line   int
	Reason Reason
}

// Result is what was read: the divisions, and the rows refused.
type Result struct {
	Rows      int
	Divisions []Division
	Refused   []Refusal
}

// Read reads one CSV. It fails if the header is not Header; a player named
// twice in one series and season keeps the first row.
func Read(file string, r io.Reader) (Result, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	head, err := cr.Read()
	if errors.Is(err, io.EOF) || (err == nil && !slices.Equal(head, Header)) {
		return Result{}, fmt.Errorf("%s: want the header %s", file, strings.Join(Header, ","))
	}
	if err != nil {
		return Result{}, fmt.Errorf("%s: %w", file, err)
	}

	var got Result
	type key struct {
		series string
		season int
		player string
	}
	seen := map[key]bool{}
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return got, nil
		}
		if err != nil {
			return Result{}, fmt.Errorf("%s: %w", file, err)
		}
		got.Rows++
		d, reason := parse(rec)
		k := key{d.Series, d.Season, d.Player}
		if reason == "" && seen[k] {
			reason = Duplicate
		}
		if reason != "" {
			got.Refused = append(got.Refused, Refusal{File: file, Line: line, Reason: reason})
			continue
		}
		seen[k] = true
		got.Divisions = append(got.Divisions, d)
	}
}

func parse(rec []string) (Division, Reason) {
	f := func(i int) string {
		if i < len(rec) {
			return rec[i]
		}
		return ""
	}
	season, err := strconv.Atoi(f(1))
	d := Division{Series: f(0), Season: season, Division: f(2), Player: f(3), Source: f(4)}
	switch {
	case !slices.Contains(Series, d.Series):
		return d, BadSeries
	case err != nil || season < 1:
		return d, BadSeason
	case !trimmed(d.Division):
		return d, BadDivision
	case !trimmed(d.Player):
		return d, BadPlayer
	case !trimmed(d.Source):
		return d, BadSource
	}
	return d, ""
}

// trimmed reports whether s is not empty and has no space at either end.
func trimmed(s string) bool {
	return s != "" && s == strings.TrimSpace(s)
}
