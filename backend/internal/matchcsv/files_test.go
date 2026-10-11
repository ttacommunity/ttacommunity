package matchcsv

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
)

func TestCheckFilesReadsEveryCSVInNameOrder(t *testing.T) {
	fsys := fstest.MapFS{
		"b.csv":       {Data: []byte(header + "Cup,2024-01-03,ann,1,bob,2,,,,,2\r\nCup,2024-01-02,ann,1,bob,2,,,,,2\r\n")},
		"a.csv":       {Data: []byte(header + "Cup,2024-01-02,ann,1,bob,2,,,,,2\r\nCup,2024-01-04,ann,0,bob,0,,,,,2\r\n")},
		"README.md":   {Data: []byte("not a match file")},
		"old/c.csv":   {Data: []byte("not even a header")},
		"notes.CSV~":  {Data: []byte("backup")},
		"z-empty.csv": {Data: []byte(header)},
	}

	got, err := CheckFiles(fsys)
	if err != nil {
		t.Fatalf("CheckFiles: %v", err)
	}
	if want := []string{"a.csv", "b.csv", "z-empty.csv"}; strings.Join(got.Files, " ") != strings.Join(want, " ") {
		t.Errorf("files = %q, want %q", got.Files, want)
	}
	if got.Rows != 4 {
		t.Errorf("rows = %d, want 4", got.Rows)
	}
	if len(got.Matches) != 3 || got.Matches[0].File != "a.csv" || got.Matches[1].Line != 3 || got.Matches[2].File != "b.csv" {
		t.Errorf("matches = %+v, want a.csv:2, a.csv:3 and b.csv:2", got.Matches)
	}
	wantRefused := map[string]Reason{"b.csv:3": Duplicate}
	if len(got.Refused) != len(wantRefused) {
		t.Fatalf("refused = %+v, want %v", got.Refused, wantRefused)
	}
	for _, r := range got.Refused {
		at := fmt.Sprintf("%s:%d", r.File, r.Line)
		if wantRefused[at] != r.Reason || len(r.Fields) != len(Header) {
			t.Errorf("refused %s as %q with %d fields, want %q with %d", at, r.Reason, len(r.Fields), wantRefused[at], len(Header))
		}
	}
}

func TestCheckFilesFails(t *testing.T) {
	tests := []struct {
		name string
		fsys fstest.MapFS
		want error
	}{
		{name: "no CSV files", fsys: fstest.MapFS{"README.md": {Data: []byte("x")}}, want: ErrNoFiles},
		{name: "a file that is not a match CSV", fsys: fstest.MapFS{
			"a.csv": {Data: []byte(header)},
			"b.csv": {Data: []byte("Player,Rating\r\n")},
		}, want: ErrBadHeader},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := CheckFiles(tt.fsys); !errors.Is(err, tt.want) {
				t.Errorf("CheckFiles = %v, want %v", err, tt.want)
			}
		})
	}
}

func FuzzCheckFiles(f *testing.F) {
	f.Add([]byte(header))
	f.Add([]byte(header + "Cup,2024-01-02,ann,1,bob,2,,,,,2\r\nCup,2024-01-03,\"multi\nline\",1,bob,2,,,,,2\r\n"))
	f.Add([]byte(header + "Intermezzo S7,2024-01-02,ann,331,bob,212,cat,-5.0,,,3\r\nCup,2024-01-02,ann,1\r\n"))
	f.Add([]byte(header + "Cup,2024-01-02,ann,1,bob,2,,,,,2\r\nCup,2024-01-02,ann,1,bob,2,,,,,2\r\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		got, err := CheckFiles(fstest.MapFS{"s1.csv": {Data: data}})
		if err != nil {
			return
		}
		if n := len(got.Matches) + len(got.Refused); n != got.Rows {
			t.Errorf("matches + refused = %d, want rows %d", n, got.Rows)
		}
	})
}
