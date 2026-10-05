package tournamentcsv

import (
	"slices"
	"strings"
	"testing"
)

const ladderWinsHeader = "ladder,player,wins,source\n"

func TestReadLadderWinsTakesEachPlayer(t *testing.T) {
	in := ladderWinsHeader + "mercurial_ladder,wolvs,10,organisers' Hall of Fame\nmercurial_ladder,Grozz,3.5,organisers' Hall of Fame\n"
	got, err := ReadLadderWins("w.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadLadderWins: %v", err)
	}
	want := []LadderWins{
		{Ladder: "mercurial_ladder", Player: "wolvs", Wins: 10, Source: "organisers' Hall of Fame"},
		{Ladder: "mercurial_ladder", Player: "Grozz", Wins: 3.5, Source: "organisers' Hall of Fame"},
	}
	if !slices.Equal(got.Facts, want) || len(got.Refused) != 0 {
		t.Errorf("wins = %+v, refused %v; want %+v", got.Facts, got.Refused, want)
	}
}

func TestReadLadderWinsRefusesBadRows(t *testing.T) {
	for name, row := range map[string]string{
		"not a tournament name": "Mercurial Ladder,wolvs,10,s",
		"no player":             "mercurial_ladder,,10,s",
		"no wins":               "mercurial_ladder,wolvs,0,s",
		"not a number":          "mercurial_ladder,wolvs,ten,s",
		"a third of a win":      "mercurial_ladder,wolvs,1.33,s",
		"no source":             "mercurial_ladder,wolvs,10,",
	} {
		got, err := ReadLadderWins("w.csv", strings.NewReader(ladderWinsHeader+row+"\n"))
		if err != nil {
			t.Fatalf("%s: ReadLadderWins: %v", name, err)
		}
		if len(got.Facts) != 0 || len(got.Refused) != 1 {
			t.Errorf("%s: %+v accepted, want it refused", name, got.Facts)
		}
	}
}
