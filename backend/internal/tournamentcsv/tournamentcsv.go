// Package tournamentcsv reads recorded tournament facts, the things match
// names cannot say: divisions, official placements, season corrections,
// finish dates, career honours and the community leaderboard.
// Each is one CSV with a fixed header and a source column.
package tournamentcsv

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// Series are the series a fact may be recorded for.
var Series = []string{"international", "intermezzo", "royal_league", "worlds"}

// Reason says why a row was refused.
type Reason string

// The reasons a row is refused.
const (
	BadSeries   Reason = "bad_series"
	BadSeason   Reason = "bad_season"
	BadDivision Reason = "bad_division"
	BadPlayer   Reason = "bad_player"
	BadPlace    Reason = "bad_place"
	BadPoints   Reason = "bad_points"
	BadReason   Reason = "bad_reason"
	BadSource   Reason = "bad_source"
	BadDate     Reason = "bad_date"
	BadMedals   Reason = "bad_medals"
	BadEdition  Reason = "bad_edition"
	BadEvent    Reason = "bad_event"
	BadEvents   Reason = "bad_events"
	BadRow      Reason = "bad_row"
	Duplicate   Reason = "duplicate"
)

// Refusal is a row that was not used. It names the line, never the player.
type Refusal struct {
	File   string
	Line   int
	Reason Reason
}

// Result is what one file gave: its rows, and the rows refused.
type Result[T any] struct {
	Rows    int
	Facts   []T
	Refused []Refusal
}

// read reads a CSV whose first line must be header. parse turns a record
// into a fact or the reason it is refused; a fact whose key was seen on an
// earlier line is refused as a duplicate.
func read[T any](file string, r io.Reader, header []string, parse func(field func(int) string) (T, Reason), key func(T) string) (Result[T], error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	head, err := cr.Read()
	if errors.Is(err, io.EOF) || (err == nil && !slices.Equal(head, header)) {
		return Result[T]{}, fmt.Errorf("%s: want the header %s", file, strings.Join(header, ","))
	}
	if err != nil {
		return Result[T]{}, fmt.Errorf("%s: %w", file, err)
	}

	var got Result[T]
	seen := map[string]bool{}
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return got, nil
		}
		if err != nil {
			return Result[T]{}, fmt.Errorf("%s: %w", file, err)
		}
		got.Rows++
		fact, reason := parse(func(i int) string {
			if i < len(rec) {
				return rec[i]
			}
			return ""
		})
		if reason == "" && seen[key(fact)] {
			reason = Duplicate
		}
		if reason != "" {
			got.Refused = append(got.Refused, Refusal{File: file, Line: line, Reason: reason})
			continue
		}
		seen[key(fact)] = true
		got.Facts = append(got.Facts, fact)
	}
}

// series returns the field if it names a known series.
func series(s string) (string, bool) {
	return s, slices.Contains(Series, s)
}

// positive returns the field as a positive integer.
func positive(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	return n, err == nil && n > 0
}

// trimmed reports whether s is not empty and has no space at either end.
func trimmed(s string) bool {
	return s != "" && s == strings.TrimSpace(s)
}
