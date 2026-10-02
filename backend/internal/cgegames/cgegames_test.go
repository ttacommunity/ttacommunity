package cgegames

import (
	"slices"
	"strings"
	"testing"
	"time"
)

const header = "tournament,season,stage,name,cge_tournament,cge_season,group,game,finished,spectate_code,unplayed," +
	"player1,score1,status1,cge_id1,banned1,player2,score2,status2,cge_id2,banned2," +
	"player3,score3,status3,cge_id3,banned3,player4,score4,status4,cge_id4,banned4\n"

func TestReadTakesEachGameWithItsSeats(t *testing.T) {
	in := header +
		"international,16,,International S16 - Bronze 1 game 2,21,7,Bronze 1,2,2021-12-02,TAPEZONA,,rap2,215.0,finished,152158,,elracha,-2.0,resigned,124851,yes,snowcrush,-5.0,timeout,21731,,,,,,\n" +
		"worlds,Age I,Stage 2,Worlds 2 Stage 2 - group 3 game 1,4149,2,group 3,1,,NENICINO,yes,ann,-5.0,timeout,1,,bob,-5.0,timeout,2,,,,,,,,,,\n"

	got, err := Read("g.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got.Games) != 2 || len(got.Refused) != 0 {
		t.Fatalf("games %+v, refused %v", got.Games, got.Refused)
	}
	g := got.Games[0]
	finished := time.Date(2021, 12, 2, 0, 0, 0, 0, time.UTC)
	if g.Season != "16" || g.Stage != "" || g.Name != "International S16 - Bronze 1 game 2" {
		t.Errorf("placing = %q %q %q", g.Season, g.Stage, g.Name)
	}
	if g.Tournament != "international" || g.CGETournament != 21 || g.CGESeason != 7 || g.Group != "Bronze 1" || g.Number != 2 ||
		g.Finished == nil || !g.Finished.Equal(finished) || g.Code != "TAPEZONA" || g.Unplayed {
		t.Errorf("game = %+v", g)
	}
	want := []Seat{
		{Name: "rap2", Score: 215, Outcome: Finished, CGEUser: 152158},
		{Name: "elracha", Score: -2, Outcome: Resigned, CGEUser: 124851, Banned: true},
		{Name: "snowcrush", Score: -5, Outcome: TimedOut, CGEUser: 21731},
	}
	if !slices.Equal(g.Seats, want) {
		t.Errorf("seats = %+v, want %+v", g.Seats, want)
	}
	if u := got.Games[1]; !u.Unplayed || u.Finished != nil || len(u.Seats) != 2 {
		t.Errorf("never-played game = %+v", u)
	}
}

func TestReadRefusesBadRows(t *testing.T) {
	seat := ",ann,10.0,finished,1,,bob,5.0,finished,2,,,,,,,,,,"
	tests := []struct {
		name string
		row  string
		want Reason
	}{
		{"no tournament", ",,,,21,7,g,1,2021-12-02,X," + seat, BadTournament},
		{"a CGE id that is not a number", "international,,,,x,7,g,1,2021-12-02,X," + seat, BadCGEID},
		{"game zero", "international,,,,21,7,g,0,2021-12-02,X," + seat, BadGame},
		{"a finish date that is not a day", "international,,,,21,7,g,1,Dec 02,X," + seat, BadDate},
		{"a seat with a name and no score", "international,,,,21,7,g,1,2021-12-02,X,,ann,,finished,1,,bob,5.0,finished,2,,,,,,,,,,", BadSeat},
		{"an outcome CGE never shows", "international,,,,21,7,g,1,2021-12-02,X,,ann,10.0,won,1,,bob,5.0,finished,2,,,,,,,,,,", BadOutcome},
		{"a name without a season", "international,,,International S16 - g game 1,21,7,g,1,2021-12-02,X," + seat, BadPlace},
		{"one seat only", "international,,,,21,7,g,1,2021-12-02,X,,ann,10.0,finished,1,,,,,,,,,,,,,,,,", BadSeat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Read("g.csv", strings.NewReader(header+tt.row+"\n"))
			if err != nil {
				t.Fatalf("Read(%q): %v", tt.row, err)
			}
			want := []Refusal{{Line: 2, Reason: tt.want}}
			if !slices.Equal(got.Refused, want) || len(got.Games) != 0 {
				t.Errorf("Read(%q): refused = %v; want %v", tt.row, got.Refused, want)
			}
		})
	}
}

func TestReadRefusesAnotherHeader(t *testing.T) {
	if _, err := Read("g.csv", strings.NewReader("tournament,game\nx,1\n")); err == nil {
		t.Error("Read accepted a file with another header")
	}
}
