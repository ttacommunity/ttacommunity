package tournamentcsv

import (
	"slices"
	"strings"
	"testing"
)

const stagesHeader = "tournament,season,stage,cge_tournament,cge_season\n"

var knownSeries = []string{"french_open", "intermezzo", "lantern_cup", "wimbledon", "worlds"}

func TestReadCGEStagesTakesEachLink(t *testing.T) {
	in := stagesHeader + "intermezzo,26,,19,21\nwimbledon,2024,Stage 7,4991,7\nlantern_cup,2031,Stage 1,7001,1\n"
	got, err := ReadCGEStages("s.csv", strings.NewReader(in), knownSeries)
	if err != nil {
		t.Fatalf("ReadCGEStages: %v", err)
	}
	want := []CGEStage{
		{Tournament: "intermezzo", Season: "26", Stage: "", CGETournament: 19, CGESeason: 21},
		{Tournament: "wimbledon", Season: "2024", Stage: "Stage 7", CGETournament: 4991, CGESeason: 7},
		{Tournament: "lantern_cup", Season: "2031", Stage: "Stage 1", CGETournament: 7001, CGESeason: 1},
	}
	if !slices.Equal(got.Facts, want) || len(got.Refused) != 0 {
		t.Errorf("stages = %+v, refused %v; want %+v", got.Facts, got.Refused, want)
	}
}

func TestReadCGEStagesRefusesBadRows(t *testing.T) {
	for name, row := range map[string]string{
		"unknown tournament": "snail_cup,1,,19,1",
		"no season":          "intermezzo,,,19,1",
		"no CGE tournament":  "intermezzo,26,,,21",
		"CGE season 0":       "intermezzo,26,,19,0",
		"not a number":       "intermezzo,26,,19,x",
	} {
		got, err := ReadCGEStages("s.csv", strings.NewReader(stagesHeader+row+"\n"), knownSeries)
		if err != nil {
			t.Fatalf("%s: ReadCGEStages: %v", name, err)
		}
		if len(got.Facts) != 0 || len(got.Refused) != 1 {
			t.Errorf("%s: %+v accepted, want it refused", name, got.Facts)
		}
	}
}
