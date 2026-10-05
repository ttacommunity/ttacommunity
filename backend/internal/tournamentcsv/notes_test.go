package tournamentcsv

import (
	"slices"
	"strings"
	"testing"
)

const notesHeader = "tournament,season,stage,note,source\n"

func TestReadSeasonNotesTakesEachNote(t *testing.T) {
	got, err := ReadSeasonNotes("n.csv", strings.NewReader(notesHeader+
		"worlds,2023 Play Off,,\"A second chance, for 2023's entrants.\",CGE /tournaments/detail/4338\n"+
		"worlds,2023 Play Off,Stage 6,The final four.,CGE /tournaments/detail/4338/6\n"))
	if err != nil {
		t.Fatalf("ReadSeasonNotes: %v", err)
	}
	want := []SeasonNote{
		{Tournament: "worlds", Season: "2023 Play Off", Note: "A second chance, for 2023's entrants.", Source: "CGE /tournaments/detail/4338"},
		{Tournament: "worlds", Season: "2023 Play Off", Stage: "Stage 6", Note: "The final four.", Source: "CGE /tournaments/detail/4338/6"},
	}
	if !slices.Equal(got.Facts, want) || len(got.Refused) != 0 {
		t.Errorf("notes = %+v, refused %v; want %+v", got.Facts, got.Refused, want)
	}
}

func TestReadSeasonNotesRefusesBadRows(t *testing.T) {
	for name, row := range map[string]string{
		"unknown tournament": "snail_cup,1,,a note,s",
		"no season":          "worlds,,,a note,s",
		"no note":            "worlds,2023,,,s",
		"no source":          "worlds,2023,,a note,",
	} {
		got, err := ReadSeasonNotes("n.csv", strings.NewReader(notesHeader+row+"\n"))
		if err != nil {
			t.Fatalf("%s: ReadSeasonNotes: %v", name, err)
		}
		if len(got.Facts) != 0 || len(got.Refused) != 1 {
			t.Errorf("%s: %+v accepted, want it refused", name, got.Facts)
		}
	}
}
