package tournamentcsv

import (
	"slices"
	"strings"
	"testing"
)

const cgeHeader = "cge_tournament,name,status,organiser,created,started,finished,description,mentions_glicko,links,tournament,season\n"

func TestReadCGETournamentsTakesEachTournament(t *testing.T) {
	in := cgeHeader +
		"5731,Survivors Cup 2026,finished,wolvs,2025-12-21,,2026-09-03,\"Welcome!\nOne game per round.\",Survivors Cup contributes to Glicko,https://example.org,survivors_cup,2026\n" +
		"4335,The banana tournament,finished,,,,2023-05-23,,,,,\n"

	got, err := ReadCGETournaments("c.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadCGETournaments: %v", err)
	}
	if len(got.Facts) != 2 || len(got.Refused) != 0 {
		t.Fatalf("tournaments = %+v, refused %v", got.Facts, got.Refused)
	}
	s := got.Facts[0]
	created, finished := day(2025, 12, 21), day(2026, 9, 3)
	if s.ID != 5731 || s.Name != "Survivors Cup 2026" || s.Organiser != "wolvs" || *s.Created != created || s.Started != nil ||
		*s.Finished != finished || s.Description != "Welcome!\nOne game per round." || s.Tournament != "survivors_cup" || s.Season != "2026" {
		t.Errorf("first = %+v", s)
	}
	if b := got.Facts[1]; b.Tournament != "" || b.Organiser != "" || b.Created != nil || *b.Finished != day(2023, 5, 23) {
		t.Errorf("second = %+v", b)
	}
}

func TestReadCGETournamentsRefusesBadRows(t *testing.T) {
	tests := []struct {
		name string
		row  string
		want Reason
	}{
		{"id not a number", "x,Cup,finished,,,,,,,,,", BadEvent},
		{"id zero", "0,Cup,finished,,,,,,,,,", BadEvent},
		{"no name", "5,,finished,,,,,,,,,", BadRow},
		{"a date that is not a day", "5,Cup,finished,,,,Game setting:,,,,,", BadDate},
		{"an unknown shape of tournament", "5,Cup,finished,,,,,,,,World Cup,", BadSeries},
		{"a season without a tournament", "5,Cup,finished,,,,,,,,,2025", BadSeason},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadCGETournaments("c.csv", strings.NewReader(cgeHeader+tt.row+"\n"))
			if err != nil {
				t.Fatalf("ReadCGETournaments(%q): %v", tt.row, err)
			}
			want := []Refusal{{File: "c.csv", Line: 2, Reason: tt.want}}
			if !slices.Equal(got.Refused, want) || len(got.Facts) != 0 {
				t.Errorf("ReadCGETournaments(%q): refused = %v; want %v", tt.row, got.Refused, want)
			}
		})
	}
}

func TestReadCGETournamentsRefusesATournamentTwice(t *testing.T) {
	got, err := ReadCGETournaments("c.csv", strings.NewReader(cgeHeader+"5,Cup,finished,,,,,,,,,\n5,Cup again,finished,,,,,,,,,\n"))
	if err != nil {
		t.Fatalf("ReadCGETournaments: %v", err)
	}
	if len(got.Facts) != 1 || !slices.Equal(got.Refused, []Refusal{{File: "c.csv", Line: 3, Reason: Duplicate}}) {
		t.Errorf("tournaments %+v, refused %v; want the first and line 3 a duplicate", got.Facts, got.Refused)
	}
}
