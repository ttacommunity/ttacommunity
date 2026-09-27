package matchcsv

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
)

// Result is what CheckFiles found.
type Result struct {
	Files   []string
	Rows    int
	Matches []Match
	Refused []Refusal
}

// CheckFiles reads every .csv file at the top of fsys, in name order, checks
// each row, and refuses a game seen in an earlier row or file. It fails if
// there is no .csv file, or if one is not a match CSV.
func CheckFiles(fsys fs.FS) (Result, error) {
	files, err := fs.Glob(fsys, "*.csv")
	if err != nil {
		return Result{}, fmt.Errorf("listing .csv files: %w", err)
	}
	if len(files) == 0 {
		return Result{}, errors.New("no .csv files to load")
	}

	var got Result
	var checked []Match
	for _, name := range files {
		f, err := fsys.Open(name)
		if err != nil {
			return Result{}, fmt.Errorf("opening %s: %w", name, err)
		}
		rows, err := Read(path.Base(name), f)
		f.Close()
		if err != nil {
			return Result{}, err
		}
		got.Files = append(got.Files, name)
		got.Rows += len(rows)
		for _, row := range rows {
			m, reason := Check(row)
			if reason != "" {
				got.Refused = append(got.Refused, Refusal{File: row.File, Line: row.Line, Reason: reason, Fields: row.Fields})
				continue
			}
			checked = append(checked, m)
		}
	}
	matches, dups := Dedupe(checked)
	got.Matches = matches
	got.Refused = append(got.Refused, dups...)
	return got, nil
}
