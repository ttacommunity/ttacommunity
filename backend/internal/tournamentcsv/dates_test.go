package tournamentcsv

import (
	"slices"
	"strings"
	"testing"
	"time"
)

const datesHeader = "series,season,finish_date,source\n"

func TestReadFinishDatesTakesEveryColumn(t *testing.T) {
	in := datesHeader + "international,34,2026-08-14,upstream parse_tournaments.py\nworlds,2025,2025-12-15,s\n"

	got, err := ReadFinishDates("d.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadFinishDates: %v", err)
	}
	want := []FinishDate{
		{Series: "international", Season: 34, Date: time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC), Source: "upstream parse_tournaments.py"},
		{Series: "worlds", Season: 2025, Date: time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC), Source: "s"},
	}
	if !slices.Equal(got.Facts, want) || len(got.Refused) != 0 {
		t.Errorf("dates = %+v, refused %v; want %+v", got.Facts, got.Refused, want)
	}
}

func TestReadFinishDatesRefusesBadRows(t *testing.T) {
	tests := []struct {
		name string
		row  string
		want Reason
	}{
		{"unknown series", "snail_cup,1,2026-01-01,s", BadSeries},
		{"season zero", "intermezzo,0,2026-01-01,s", BadSeason},
		{"no date", "intermezzo,1,,s", BadDate},
		{"day first", "intermezzo,1,14-08-2026,s", BadDate},
		{"no such day", "intermezzo,1,2026-02-30,s", BadDate},
		{"no source", "intermezzo,1,2026-01-01,", BadSource},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadFinishDates("d.csv", strings.NewReader(datesHeader+tt.row+"\n"))
			if err != nil {
				t.Fatalf("ReadFinishDates(%q): %v", tt.row, err)
			}
			want := []Refusal{{File: "d.csv", Line: 2, Reason: tt.want}}
			if !slices.Equal(got.Refused, want) || len(got.Facts) != 0 {
				t.Errorf("ReadFinishDates(%q): refused = %v; want %v", tt.row, got.Refused, want)
			}
		})
	}
}

func TestReadFinishDatesRefusesASecondDateForASeason(t *testing.T) {
	in := datesHeader + "intermezzo,3,2019-08-04,s\nintermezzo,3,2019-08-05,s\n"
	got, err := ReadFinishDates("d.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadFinishDates: %v", err)
	}
	if want := []Refusal{{File: "d.csv", Line: 3, Reason: Duplicate}}; !slices.Equal(got.Refused, want) || len(got.Facts) != 1 {
		t.Errorf("refused = %v, facts %d; want %v and 1 fact", got.Refused, len(got.Facts), want)
	}
}
