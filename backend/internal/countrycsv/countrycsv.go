// Package countrycsv reads players' countries from a CSV with Player and
// Code columns, such as upstream's ratings export.
package countrycsv

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"slices"
	"strings"
)

// Reason says why a row was refused.
type Reason string

// The reasons a row is refused.
const (
	BadCode   Reason = "bad_code"
	BadPlayer Reason = "bad_player"
)

// Refusal is a row that was not used. It names the line, never the player.
type Refusal struct {
	File   string
	Line   int
	Reason Reason
}

// Result is what was read: each player's country, and the rows not used.
type Result struct {
	Rows      int
	Countries map[string]string
	None      int
	Refused   []Refusal
}

var code = regexp.MustCompile(`^[A-Z]{2}$`)

// Read reads one CSV. "-" or an empty code means no country. It fails if the
// header lacks a Player or a Code column.
func Read(file string, r io.Reader) (Result, error) {
	got := Result{Countries: map[string]string{}}
	return got, read(file, r, &got)
}

// ReadFiles reads the named files from fsys in order; a later file's country
// for a player replaces an earlier one's.
func ReadFiles(fsys fs.FS, names []string) (Result, error) {
	got := Result{Countries: map[string]string{}}
	for _, name := range names {
		f, err := fsys.Open(name)
		if err != nil {
			return Result{}, fmt.Errorf("opening %s: %w", name, err)
		}
		err = read(name, f, &got)
		f.Close()
		if err != nil {
			return Result{}, err
		}
	}
	return got, nil
}

func read(file string, r io.Reader, got *Result) error {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if errors.Is(err, io.EOF) {
		return fmt.Errorf("%s: empty, want a Player and a Code column", file)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	player, country := slices.Index(header, "Player"), slices.Index(header, "Code")
	if player < 0 || country < 0 {
		return fmt.Errorf("%s: want a Player and a Code column", file)
	}

	for line := 2; ; line++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		got.Rows++
		name, c := field(rec, player), field(rec, country)
		switch {
		case name == "" || name != strings.TrimSpace(name):
			got.Refused = append(got.Refused, Refusal{File: file, Line: line, Reason: BadPlayer})
		case c == "" || c == "-":
			got.None++
		case !code.MatchString(c):
			got.Refused = append(got.Refused, Refusal{File: file, Line: line, Reason: BadCode})
		default:
			got.Countries[name] = c
		}
	}
}

func field(rec []string, i int) string {
	if i < len(rec) {
		return rec[i]
	}
	return ""
}
