package countrycsv

import (
	"maps"
	"strings"
	"testing"
	"testing/fstest"
)

func TestReadTakesPlayerAndCodeColumns(t *testing.T) {
	in := "Rank,Player,Code,Title\r\n1,ann,PL,GM\r\n2,bob,-,M\r\n3,cat,,\r\n4,dan,US,-\r\n"

	got, err := Read("r.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if want := map[string]string{"ann": "PL", "dan": "US"}; !maps.Equal(got.Countries, want) {
		t.Errorf("countries = %v, want %v", got.Countries, want)
	}
	if got.Rows != 4 || got.None != 2 || len(got.Refused) != 0 {
		t.Errorf("rows %d, none %d, refused %v; want 4, 2, none", got.Rows, got.None, got.Refused)
	}
}

func TestReadRefusesBadRows(t *testing.T) {
	in := "Player,Code\nann,pl\nbob,POL\ncat,P1\n,DE\n eve ,FR\nfay,NL\n"

	got, err := Read("r.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if want := map[string]string{"fay": "NL"}; !maps.Equal(got.Countries, want) {
		t.Errorf("countries = %v, want %v", got.Countries, want)
	}
	want := []Refusal{
		{File: "r.csv", Line: 2, Reason: BadCode},
		{File: "r.csv", Line: 3, Reason: BadCode},
		{File: "r.csv", Line: 4, Reason: BadCode},
		{File: "r.csv", Line: 5, Reason: BadPlayer},
		{File: "r.csv", Line: 6, Reason: BadPlayer},
	}
	if len(got.Refused) != len(want) {
		t.Fatalf("refused = %v, want %v", got.Refused, want)
	}
	for i := range want {
		if got.Refused[i] != want[i] {
			t.Errorf("refused[%d] = %v, want %v", i, got.Refused[i], want[i])
		}
	}
}

func TestReadNeedsBothColumns(t *testing.T) {
	for _, in := range []string{"Player,Country\nann,PL\n", "Name,Code\nann,PL\n", ""} {
		if _, err := Read("r.csv", strings.NewReader(in)); err == nil {
			t.Errorf("Read(%q): no error, want one", in)
		}
	}
}

func TestReadFilesLetsALaterFileWin(t *testing.T) {
	fsys := fstest.MapFS{
		"ratings.csv":   {Data: []byte("Player,Code\nann,US\nbob,DE\n")},
		"overrides.csv": {Data: []byte("Player,Code\nann,CN\n")},
	}

	got, err := ReadFiles(fsys, []string{"ratings.csv", "overrides.csv"})
	if err != nil {
		t.Fatalf("ReadFiles: %v", err)
	}
	if want := map[string]string{"ann": "CN", "bob": "DE"}; !maps.Equal(got.Countries, want) {
		t.Errorf("countries = %v, want %v", got.Countries, want)
	}
	if got.Rows != 3 {
		t.Errorf("rows = %d, want 3", got.Rows)
	}
}

func TestReadFilesFailsOnAMissingFile(t *testing.T) {
	if _, err := ReadFiles(fstest.MapFS{}, []string{"nope.csv"}); err == nil {
		t.Error("ReadFiles of a missing file: no error, want one")
	}
}
