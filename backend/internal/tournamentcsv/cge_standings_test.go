package tournamentcsv

import (
	"slices"
	"strings"
	"testing"
)

const cgeStandingsHeader = "cge_tournament,cge_season,group,row,place,player,cge_user,banned,games,points,places_2p,places_3p,places_4p,source\n"

func TestReadCGEStandingsTakesEachRow(t *testing.T) {
	in := cgeStandingsHeader +
		"4760,2,2024 group 1,1,1,Magpie,101239,,5,21,1 1,1 0 0,2 0 0 0,CGE p\n" +
		"4760,2,2024 group 1,4,4,swift (BAN),65877,true,2,-1.5,,,0 0 0 2,CGE p\n"
	got, err := ReadCGEStandings("s.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadCGEStandings: %v", err)
	}
	if len(got.Facts) != 2 || len(got.Refused) != 0 {
		t.Fatalf("got %+v, refused %v; want 2 rows", got.Facts, got.Refused)
	}
	first, last := got.Facts[0], got.Facts[1]
	if first.CGETournament != 4760 || first.CGESeason != 2 || first.Group != "2024 group 1" || first.Row != 1 || first.Place != 1 ||
		first.Player != "Magpie" || first.CGEUser != 101239 || first.Banned || first.Games != 5 || first.Points != 21 {
		t.Errorf("first row = %+v", first)
	}
	if !slices.Equal(first.Places2p, []int{1, 1}) || !slices.Equal(first.Places3p, []int{1, 0, 0}) || !slices.Equal(first.Places4p, []int{2, 0, 0, 0}) {
		t.Errorf("first row's places = %v %v %v", first.Places2p, first.Places3p, first.Places4p)
	}
	if !last.Banned || last.Points != -1.5 || last.Places2p != nil || !slices.Equal(last.Places4p, []int{0, 0, 0, 2}) {
		t.Errorf("last row = %+v", last)
	}
}

func TestReadCGEStandingsTakesARowWithoutAPlace(t *testing.T) {
	got, err := ReadCGEStandings("s.csv", strings.NewReader(cgeStandingsHeader+"19,1,Gold 1,3,,a,7,,6,9,,2 2 2,,CGE p\n"))
	if err != nil {
		t.Fatalf("ReadCGEStandings: %v", err)
	}
	if len(got.Facts) != 1 || got.Facts[0].Place != 0 {
		t.Errorf("got %+v, refused %v; want one row without a place", got.Facts, got.Refused)
	}
}

func TestReadCGEStandingsRefusesBadRows(t *testing.T) {
	for name, row := range map[string]string{
		"no tournament":              ",2,g,1,1,a,7,,5,21,,1 0 0,,s",
		"no season":                  "4760,x,g,1,1,a,7,,5,21,,1 0 0,,s",
		"no group":                   "4760,2,,1,1,a,7,,5,21,,1 0 0,,s",
		"no row":                     "4760,2,g,0,1,a,7,,5,21,,1 0 0,,s",
		"place not a number":         "4760,2,g,1,first,a,7,,5,21,,1 0 0,,s",
		"no player":                  "4760,2,g,1,1,,7,,5,21,,1 0 0,,s",
		"no CGE user":                "4760,2,g,1,1,a,,,5,21,,1 0 0,,s",
		"banned not true":            "4760,2,g,1,1,a,7,yes,5,21,,1 0 0,,s",
		"negative games":             "4760,2,g,1,1,a,7,,-1,21,,1 0 0,,s",
		"points not a number":        "4760,2,g,1,1,a,7,,5,many,,1 0 0,,s",
		"3-player places of 2":       "4760,2,g,1,1,a,7,,5,21,,1 0,,s",
		"a negative place count":     "4760,2,g,1,1,a,7,,5,21,,1 -1 0,,s",
		"places not numbers":         "4760,2,g,1,1,a,7,,5,21,,one 0 0,,s",
		"places more than its games": "4760,2,g,1,1,a,7,,1,21,,1 1 0,,s",
		"no source":                  "4760,2,g,1,1,a,7,,5,21,,1 0 0,,",
	} {
		got, err := ReadCGEStandings("s.csv", strings.NewReader(cgeStandingsHeader+row+"\n"))
		if err != nil {
			t.Fatalf("%s: ReadCGEStandings: %v", name, err)
		}
		if len(got.Facts) != 0 || len(got.Refused) != 1 {
			t.Errorf("%s: %+v accepted, want it refused", name, got.Facts)
		}
	}
}

func TestReadCGEStandingsRefusesARowGivenTwice(t *testing.T) {
	row := "4760,2,g,1,1,a,7,,5,21,,1 0 0,,s\n"
	got, err := ReadCGEStandings("s.csv", strings.NewReader(cgeStandingsHeader+row+row))
	if err != nil {
		t.Fatalf("ReadCGEStandings: %v", err)
	}
	if len(got.Facts) != 1 || len(got.Refused) != 1 || got.Refused[0].Reason != Duplicate {
		t.Errorf("got %d rows, refused %v; want the second refused as a duplicate", len(got.Facts), got.Refused)
	}
}
