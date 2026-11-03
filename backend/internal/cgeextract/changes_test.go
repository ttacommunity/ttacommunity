package cgeextract

import (
	"slices"
	"strings"
	"testing"
)

// changesHeader is the changes file's first line: the extract's columns between change and changed_columns.
var changesHeader = "change," + strings.TrimSuffix(strings.TrimPrefix(header, "\xef\xbb\xbf"), "\n") + ",changed_columns\n"

// change is one changes file line: kind, the extract row, and the columns it names.
func change(kind, line, columns string) string {
	return kind + "," + strings.TrimSuffix(line, "\n") + "," + columns + "\n"
}

func TestReadChangesSplitsPresentRowsFromRemovedOnes(t *testing.T) {
	in := changesHeader +
		change("new", row(5, 1, "ann", "1", "181.0", "finished"), "") +
		change("changed", row(6, 2, "bob", "1", "90.0", "finished"), "standing score") +
		change("removed", row(7, 3, "cy", "1", "70.0", "finished"), "")
	got, err := ReadChanges("changes.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadChanges: %v", err)
	}
	var present, removed []int
	for _, s := range got.Seats {
		present = append(present, s.SessionID)
	}
	for _, s := range got.Removed {
		removed = append(removed, s.SessionID)
	}
	if !slices.Equal(present, []int{5, 6}) || !slices.Equal(removed, []int{7}) || got.Full || len(got.Refused) != 0 {
		t.Errorf("present %v, removed %v, full %t, refused %v; want 5 and 6, 7, not full, none", present, removed, got.Full, got.Refused)
	}
	if _, ok := got.Tournaments[19]; !ok || len(got.Tournaments) != 1 {
		t.Errorf("tournaments = %v, want 19's, from its present rows", got.Tournaments)
	}
}

func TestReadChangesRefusesAnUnknownChange(t *testing.T) {
	got, err := ReadChanges("changes.csv", strings.NewReader(changesHeader+change("moved", row(5, 1, "ann", "1", "181.0", "finished"), "")))
	if err != nil {
		t.Fatalf("ReadChanges: %v", err)
	}
	if len(got.Seats) != 0 || len(got.Removed) != 0 || len(got.Refused) != 1 || got.Refused[0].Line != 2 || got.Refused[0].File != "changes.csv" {
		t.Errorf("seats %v, removed %v, refused %+v; want line 2 of changes.csv refused", got.Seats, got.Removed, got.Refused)
	}
}

func TestReadChangesRefusesAnotherHeader(t *testing.T) {
	if _, err := ReadChanges("changes.csv", strings.NewReader(header+row(5, 1, "ann", "1", "181.0", "finished"))); err == nil {
		t.Error("ReadChanges read the full file's header; want an error naming the header it wants")
	}
}

func TestReadMarksTheFullFile(t *testing.T) {
	got, err := Read("x.csv", strings.NewReader(header+row(5, 1, "ann", "1", "181.0", "finished")))
	if err != nil || !got.Full {
		t.Errorf("Read: full %t, %v; want the full file marked so", got.Full, err)
	}
}

func FuzzReadChanges(f *testing.F) {
	f.Add(changesHeader)
	f.Add(changesHeader + change("new", row(5, 1, "ann", "1", "181.0", "finished"), "") +
		change("removed", row(7, 3, "cy", "1", "70.0", "finished"), ""))
	f.Add(changesHeader + change("changed", row(6, 2, "bob", "", "-9", "resigned"), "standing score finish_status"))
	f.Fuzz(func(t *testing.T, in string) {
		got, err := ReadChanges("changes.csv", strings.NewReader(in))
		if err != nil {
			return
		}
		if n := len(got.Seats) + len(got.Removed) + len(got.Refused); n != got.Rows {
			t.Errorf("present + removed + refused = %d, want rows %d", n, got.Rows)
		}
	})
}

func TestCombineTakesEachRowOnce(t *testing.T) {
	full, err := Read("x.csv", strings.NewReader(header+row(5, 1, "ann", "1", "181.0", "finished")+row(5, 2, "bob", "2", "90.0", "finished")))
	if err != nil {
		t.Fatal(err)
	}
	changes, err := ReadChanges("changes.csv", strings.NewReader(changesHeader+
		change("changed", row(5, 2, "bob", "2", "90.0", "finished"), "score")+
		change("new", row(8, 4, "dee", "1", "50.0", "finished"), "")+
		change("removed", row(7, 3, "cy", "1", "70.0", "finished"), "")))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		got         Extract
		wantPresent []int
		wantFull    bool
	}{
		{"both files", Combine(&full, &changes), []int{5, 5, 8}, true},
		{"the full file alone", Combine(&full, nil), []int{5, 5}, true},
		{"the changes file alone", Combine(nil, &changes), []int{5, 8}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var present []int
			for _, s := range tt.got.Seats {
				present = append(present, s.SessionID)
			}
			if !slices.Equal(present, tt.wantPresent) || tt.got.Full != tt.wantFull {
				t.Errorf("present %v, full %t; want %v, %t", present, tt.got.Full, tt.wantPresent, tt.wantFull)
			}
		})
	}
	if got := Combine(&full, &changes); len(got.Removed) != 1 || got.Removed[0].SessionID != 7 {
		t.Errorf("removed = %v, want the changes file's session 7", got.Removed)
	}
}
