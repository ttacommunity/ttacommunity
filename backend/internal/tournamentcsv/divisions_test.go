package tournamentcsv

import (
	"slices"
	"strings"
	"testing"
)

const header = "series,season,division,player,source\n"

func TestReadTakesEveryColumn(t *testing.T) {
	in := header +
		"international,20,Grandmaster,ann,Hall of Fame.xlsx\r\n" +
		"intermezzo,3,Grandmaster,bob,Hall of Fame.xlsx\r\n"

	got, err := ReadDivisions("d.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want := []Division{
		{Series: "international", Season: 20, Division: "Grandmaster", Player: "ann", Source: "Hall of Fame.xlsx"},
		{Series: "intermezzo", Season: 3, Division: "Grandmaster", Player: "bob", Source: "Hall of Fame.xlsx"},
	}
	if !slices.Equal(got.Facts, want) {
		t.Errorf("divisions = %v, want %v", got.Facts, want)
	}
	if got.Rows != 2 || len(got.Refused) != 0 {
		t.Errorf("rows %d, refused %v; want 2, none", got.Rows, got.Refused)
	}
}

func TestReadRefusesBadRows(t *testing.T) {
	tests := []struct {
		name string
		row  string
		want Reason
	}{
		{"unknown series", "snail_cup,1,Grandmaster,ann,s", BadSeries},
		{"series in capitals", "International,1,Grandmaster,ann,s", BadSeries},
		{"season zero", "international,0,Grandmaster,ann,s", BadSeason},
		{"season not a number", "international,S20,Grandmaster,ann,s", BadSeason},
		{"no division", "international,1,,ann,s", BadDivision},
		{"division with spaces round it", "international,1, Grandmaster,ann,s", BadDivision},
		{"no player", "international,1,Grandmaster,,s", BadPlayer},
		{"player with spaces round it", "international,1,Grandmaster, ann ,s", BadPlayer},
		{"no source", "international,1,Grandmaster,ann,", BadSource},
		{"too few fields", "international,1,Grandmaster", BadPlayer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadDivisions("d.csv", strings.NewReader(header+tt.row+"\n"))
			if err != nil {
				t.Fatalf("Read(%q): %v", tt.row, err)
			}
			want := []Refusal{{File: "d.csv", Line: 2, Reason: tt.want}}
			if !slices.Equal(got.Refused, want) || len(got.Facts) != 0 {
				t.Errorf("Read(%q): refused = %v, divisions = %v; want %v, none", tt.row, got.Refused, got.Facts, want)
			}
		})
	}
}

func TestReadRefusesAPlayerTwiceInOneSeason(t *testing.T) {
	in := header +
		"international,20,Grandmaster,ann,s\n" +
		"international,20,Master,ann,s\n" +
		"international,21,Grandmaster,ann,s\n"

	got, err := ReadDivisions("d.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got.Facts) != 2 {
		t.Errorf("divisions = %v, want seasons 20 and 21 once each", got.Facts)
	}
	if want := []Refusal{{File: "d.csv", Line: 3, Reason: Duplicate}}; !slices.Equal(got.Refused, want) {
		t.Errorf("refused = %v, want %v", got.Refused, want)
	}
}

func TestReadNeedsTheHeader(t *testing.T) {
	for _, in := range []string{
		"",
		"series,season,division,player\n",
		"season,series,division,player,source\n",
	} {
		if _, err := ReadDivisions("d.csv", strings.NewReader(in)); err == nil {
			t.Errorf("Read(%q): no error, want one", in)
		}
	}
}
