package tournamentcsv

import (
	"slices"
	"strings"
	"testing"
)

const communityPointsHeader = "tournament,from_edition,group,place,points,source\n"

func TestReadCommunityPointsTakesEachRow(t *testing.T) {
	in := communityPointsHeader + "international,2025,Grandmaster,1,1000,sheet\nworlds,2025,Stage 3,0,130,sheet\nintermezzo,2024,Wood,2,15,sheet\n"
	got, err := ReadCommunityPoints("p.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadCommunityPoints: %v", err)
	}
	want := []CommunityPoints{
		{Tournament: "international", FromEdition: 2025, Group: "Grandmaster", Place: 1, Points: 1000, Source: "sheet"},
		{Tournament: "worlds", FromEdition: 2025, Group: "Stage 3", Place: 0, Points: 130, Source: "sheet"},
		{Tournament: "intermezzo", FromEdition: 2024, Group: "Wood", Place: 2, Points: 15, Source: "sheet"},
	}
	if !slices.Equal(got.Facts, want) || len(got.Refused) != 0 {
		t.Errorf("points = %+v, refused %v; want %+v", got.Facts, got.Refused, want)
	}
}

func TestReadCommunityPointsRefusesBadRows(t *testing.T) {
	for name, row := range map[string]string{
		"not a tournament name": "International,2025,Grandmaster,1,1000,s",
		"edition not a year":    "international,25,Grandmaster,1,1000,s",
		"no group":              "international,2025,,1,1000,s",
		"negative place":        "international,2025,Grandmaster,-1,1000,s",
		"points not a number":   "international,2025,Grandmaster,1,lots,s",
		"negative points":       "international,2025,Grandmaster,1,-5,s",
		"no source":             "international,2025,Grandmaster,1,1000,",
	} {
		got, err := ReadCommunityPoints("p.csv", strings.NewReader(communityPointsHeader+row+"\n"))
		if err != nil {
			t.Fatalf("%s: ReadCommunityPoints: %v", name, err)
		}
		if len(got.Facts) != 0 || len(got.Refused) != 1 {
			t.Errorf("%s: %+v accepted, want it refused", name, got.Facts)
		}
	}
}

func TestReadCommunityPointsRefusesAPlaceGivenTwice(t *testing.T) {
	row := "international,2025,Grandmaster,1,1000,s\n"
	got, err := ReadCommunityPoints("p.csv", strings.NewReader(communityPointsHeader+row+row))
	if err != nil {
		t.Fatalf("ReadCommunityPoints: %v", err)
	}
	if len(got.Facts) != 1 || len(got.Refused) != 1 || got.Refused[0].Reason != Duplicate {
		t.Errorf("got %d rows, refused %v; want the second refused as a duplicate", len(got.Facts), got.Refused)
	}
}
