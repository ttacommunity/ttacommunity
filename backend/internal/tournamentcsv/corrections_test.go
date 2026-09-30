package tournamentcsv

import (
	"slices"
	"strings"
	"testing"
)

const correctionsHeader = "series,name_season,season,reason,source\n"

func TestReadCorrectionsTakesASeasonOrNone(t *testing.T) {
	in := correctionsHeader +
		"intermezzo,8,7,labelled one high,Intermezzo Season 7.xlsx\n" +
		"intermezzo,7,,a copy of season 6,Intermezzo Season 6.xlsx\n"

	got, err := ReadCorrections("c.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadCorrections: %v", err)
	}
	if len(got.Facts) != 2 || len(got.Refused) != 0 {
		t.Fatalf("corrections = %+v, refused %v", got.Facts, got.Refused)
	}
	first, second := got.Facts[0], got.Facts[1]
	if first.Series != "intermezzo" || first.NameSeason != 8 || first.Season == nil || *first.Season != 7 ||
		first.Reason != "labelled one high" || first.Source != "Intermezzo Season 7.xlsx" {
		t.Errorf("correction 1 = %+v", first)
	}
	if second.NameSeason != 7 || second.Season != nil {
		t.Errorf("correction 2 = %+v, want season 7 left out", second)
	}
}

func TestReadCorrectionsRefusesBadRows(t *testing.T) {
	tests := []struct {
		name string
		row  string
		want Reason
	}{
		{"unknown series", "worlds,1,2,r,s", BadSeries},
		{"name season zero", "intermezzo,0,2,r,s", BadSeason},
		{"season not a number", "intermezzo,8,seven,r,s", BadSeason},
		{"season zero", "intermezzo,8,0,r,s", BadSeason},
		{"no reason", "intermezzo,8,7,,s", BadReason},
		{"no source", "intermezzo,8,7,r,", BadSource},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadCorrections("c.csv", strings.NewReader(correctionsHeader+tt.row+"\n"))
			if err != nil {
				t.Fatalf("ReadCorrections(%q): %v", tt.row, err)
			}
			want := []Refusal{{File: "c.csv", Line: 2, Reason: tt.want}}
			if !slices.Equal(got.Refused, want) || len(got.Facts) != 0 {
				t.Errorf("ReadCorrections(%q): refused = %v; want %v", tt.row, got.Refused, want)
			}
		})
	}
}

func TestReadCorrectionsRefusesANameSeasonTwice(t *testing.T) {
	in := correctionsHeader + "intermezzo,8,7,r,s\nintermezzo,8,6,r,s\n"
	got, err := ReadCorrections("c.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadCorrections: %v", err)
	}
	if want := []Refusal{{File: "c.csv", Line: 3, Reason: Duplicate}}; !slices.Equal(got.Refused, want) || len(got.Facts) != 1 {
		t.Errorf("refused = %v, facts %+v; want %v and one fact", got.Refused, got.Facts, want)
	}
}
