package tournamentcsv

import (
	"slices"
	"strings"
	"testing"
)

const placementsHeader = "series,season,division,player,place,points,reason,source\n"

func TestReadPlacementsTakesEveryColumn(t *testing.T) {
	in := placementsHeader +
		"intermezzo,24,Grandmaster,ann,1,25,no games in the match data,Hall of Fame.xlsx\n" +
		"royal_league,1,Emperor,bob,2,,upstream's recorded podium,upstream hall_of_fame.py\n"

	got, err := ReadPlacements("p.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadPlacements: %v", err)
	}
	pts := 25.0
	want := []Placement{
		{Series: "intermezzo", Season: 24, Division: "Grandmaster", Player: "ann", Place: 1, Points: &pts,
			Reason: "no games in the match data", Source: "Hall of Fame.xlsx"},
		{Series: "royal_league", Season: 1, Division: "Emperor", Player: "bob", Place: 2,
			Reason: "upstream's recorded podium", Source: "upstream hall_of_fame.py"},
	}
	if len(got.Facts) != len(want) || len(got.Refused) != 0 {
		t.Fatalf("placements = %+v, refused %v; want %+v", got.Facts, got.Refused, want)
	}
	for i := range want {
		g, w := got.Facts[i], want[i]
		if g.Series != w.Series || g.Season != w.Season || g.Division != w.Division || g.Player != w.Player ||
			g.Place != w.Place || (g.Points == nil) != (w.Points == nil) || (g.Points != nil && *g.Points != *w.Points) ||
			g.Reason != w.Reason || g.Source != w.Source {
			t.Errorf("placement %d = %+v, want %+v", i, g, w)
		}
	}
}

func TestReadPlacementsTakesTheWorldChampionship(t *testing.T) {
	got, err := ReadPlacements("p.csv", strings.NewReader(placementsHeader+"worlds,2025,Final,ann,1,,r,s\n"))
	if err != nil {
		t.Fatalf("ReadPlacements: %v", err)
	}
	if len(got.Facts) != 1 || got.Facts[0].Series != "worlds" || got.Facts[0].Season != 2025 {
		t.Errorf("placements = %+v, refused %v; want the 2025 World Championship", got.Facts, got.Refused)
	}
}

func TestReadPlacementsTakesATeamOfTheNationsLeague(t *testing.T) {
	got, err := ReadPlacements("p.csv", strings.NewReader(placementsHeader+
		"nations_league,10,Apple,DANeo,1,36,r,s\nnations_league,10,Apple,DrRerNate,1,36,r,s\n"))
	if err != nil {
		t.Fatalf("ReadPlacements: %v", err)
	}
	if len(got.Facts) != 2 || len(got.Refused) != 0 {
		t.Errorf("placements = %+v, refused %v; want both players of the champion team", got.Facts, got.Refused)
	}
}

func TestReadPlacementsRefusesBadRows(t *testing.T) {
	tests := []struct {
		name string
		row  string
		want Reason
	}{
		{"unknown series", "snail_cup,1,Final,ann,1,,r,s", BadSeries},
		{"no series", ",1,Final,ann,1,,r,s", BadSeries},
		{"season zero", "intermezzo,0,Grandmaster,ann,1,,r,s", BadSeason},
		{"no division", "intermezzo,1,,ann,1,,r,s", BadDivision},
		{"no player", "intermezzo,1,Grandmaster,,1,,r,s", BadPlayer},
		{"place zero", "intermezzo,1,Grandmaster,ann,0,,r,s", BadPlace},
		{"place not a number", "intermezzo,1,Grandmaster,ann,first,,r,s", BadPlace},
		{"points not a number", "intermezzo,1,Grandmaster,ann,1,lots,r,s", BadPoints},
		{"no reason", "intermezzo,1,Grandmaster,ann,1,,,s", BadReason},
		{"no source", "intermezzo,1,Grandmaster,ann,1,,r,", BadSource},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadPlacements("p.csv", strings.NewReader(placementsHeader+tt.row+"\n"))
			if err != nil {
				t.Fatalf("ReadPlacements(%q): %v", tt.row, err)
			}
			want := []Refusal{{File: "p.csv", Line: 2, Reason: tt.want}}
			if !slices.Equal(got.Refused, want) || len(got.Facts) != 0 {
				t.Errorf("ReadPlacements(%q): refused = %v; want %v", tt.row, got.Refused, want)
			}
		})
	}
}

func TestReadPlacementsRefusesAPlayerTwiceInADivision(t *testing.T) {
	in := placementsHeader +
		"royal_league,1,Emperor,ann,2,,tie,s\n" +
		"royal_league,1,Emperor,bob,2,,tie,s\n" +
		"royal_league,1,Emperor,ann,3,,tie,s\n"

	got, err := ReadPlacements("p.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadPlacements: %v", err)
	}
	if len(got.Facts) != 2 {
		t.Errorf("placements = %+v, want two players sharing 2nd", got.Facts)
	}
	if want := []Refusal{{File: "p.csv", Line: 4, Reason: Duplicate}}; !slices.Equal(got.Refused, want) {
		t.Errorf("refused = %v, want %v", got.Refused, want)
	}
}
