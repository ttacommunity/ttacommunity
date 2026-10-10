package cgeextract

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

const header = "\xef\xbb\xbftournament_id,tournament_name,organiser_id,organiser_name,moderators,tournament_created_at," +
	"tournament_started_at,tournament_finished_at,stage_id,stage_number,stage_started_at,stage_finished_at,group_id," +
	"group_name,session_id,share_code,session_state,started_at,finished_at,speed,timer,player_id,player_name," +
	"starting_position,players_in_game,standing,score,finish_status\n"

// row is one extract line of tournament 19, stage 3, group 7, for the seat given.
func row(session, player int, name, standing, score, status string) string {
	return "19,Cup,101,orga,\"mod one, mod two\",2020-04-07 20:42:28.000,2020-04-25 21:19:48.000,,3,1," +
		"2020-04-25 21:19:48.000,2020-06-08 13:09:01.000,7,Gold 1," + strconv.Itoa(session) + ",ABCDEFGH,finished," +
		"2020-04-25 21:19:16.992,2020-05-12 07:34:01.739,Medium Async,48 h," + strconv.Itoa(player) + "," + name + ",0,2," +
		standing + "," + score + "," + status + "\n"
}

func TestReadParsesASeat(t *testing.T) {
	got, err := Read("x.csv", strings.NewReader(header+row(5, 1, "ann", "1", "181.0", "finished")))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got.Seats) != 1 || len(got.Refused) != 0 {
		t.Fatalf("seats %d, refused %v; want one seat", len(got.Seats), got.Refused)
	}
	s := got.Seats[0]
	finished := time.Date(2020, 5, 12, 7, 34, 1, 739_000_000, time.UTC)
	if s.TournamentID != 19 || s.StageID != 3 || s.StageNumber != 1 || s.GroupID != 7 || s.GroupName != "Gold 1" ||
		s.SessionID != 5 || s.ShareCode != "ABCDEFGH" || s.State != "finished" || !s.FinishedAt.Equal(finished) ||
		s.Speed != "Medium Async" || s.Timer != "48 h" || s.PlayerID != 1 || s.PlayerName != "ann" ||
		*s.Standing != 1 || *s.Score != 181 || s.FinishStatus != Finished || s.PlayersInGame != 2 {
		t.Errorf("seat = %+v", s)
	}
	tn := got.Tournaments[19]
	if tn.OrganiserID != 101 || !slices.Equal(tn.Moderators, []string{"mod one", "mod two"}) || tn.Finished != nil {
		t.Errorf("tournament = %+v, want organiser 101, two moderators, not finished", tn)
	}
}

func TestReadTakesABanFromTheName(t *testing.T) {
	got, err := Read("x.csv", strings.NewReader(header+row(5, 1, "ann(BAN)", "1", "181.0", "finished")))
	if err != nil {
		t.Fatal(err)
	}
	if s := got.Seats[0]; s.PlayerName != "ann" || !s.Banned {
		t.Errorf("name %q banned %v, want ann, true", s.PlayerName, s.Banned)
	}
}

func TestReadKeepsATimedOutSeatWithoutScoreOrStanding(t *testing.T) {
	got, err := Read("x.csv", strings.NewReader(header+row(5, 1, "ann", "", "", "timed out")))
	if err != nil {
		t.Fatal(err)
	}
	if s := got.Seats[0]; s.FinishStatus != TimedOut || s.Score != nil || s.Standing != nil {
		t.Errorf("seat = %+v, want timed out, no score, no standing", s)
	}
}

func TestReadRefusesRowsThatDoNotFit(t *testing.T) {
	tests := []struct {
		name, line string
		want       Reason
	}{
		{"a session that is not a number", strings.Replace(row(5, 1, "ann", "1", "181.0", "finished"), ",5,ABCDEFGH,", ",x,ABCDEFGH,", 1), BadID},
		{"an unknown state", strings.Replace(row(5, 1, "ann", "1", "181.0", "finished"), ",finished,2020-04-25 21:19:16", ",paused,2020-04-25 21:19:16", 1), BadState},
		{"an unknown finish status", row(5, 1, "ann", "1", "181.0", "won"), BadStatus},
		{"a finished seat without a score", row(5, 1, "ann", "1", "", "finished"), BadScore},
		{"a resigned seat with a positive score", row(5, 1, "ann", "2", "12.0", "resigned"), BadScore},
		{"a timed-out seat with a standing", row(5, 1, "ann", "2", "", "timed out"), BadStanding},
		{"a seat with no player", row(5, 0, "", "", "", "timed out"), NoPlayer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Read("x.csv", strings.NewReader(header+tt.line))
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if len(got.Seats) != 0 || len(got.Refused) != 1 || got.Refused[0].Reason != tt.want || got.Refused[0].Line != 2 {
				t.Errorf("seats %d, refused %+v; want line 2 refused as %s", len(got.Seats), got.Refused, tt.want)
			}
		})
	}
}

func TestReadRefusesASeatTwice(t *testing.T) {
	line := row(5, 1, "ann", "1", "181.0", "finished")
	got, err := Read("x.csv", strings.NewReader(header+line+line))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Seats) != 1 || len(got.Refused) != 1 || got.Refused[0].Reason != Duplicate {
		t.Errorf("seats %d, refused %+v; want the second line refused as a duplicate", len(got.Seats), got.Refused)
	}
}

func TestReadRefusesAnotherHeader(t *testing.T) {
	if _, err := Read("x.csv", strings.NewReader("tournament_id,name\n")); err == nil {
		t.Error("Read with another header: no error, want one")
	}
}

func FuzzRead(f *testing.F) {
	f.Add(header)
	f.Add(header + row(5, 1, "ann", "1", "181.0", "finished") + row(5, 2, "bob(BAN)", "2", "120.5", "finished"))
	f.Add(header + row(6, 1, "ann", "", "-9", "resigned") + row(6, 2, "bob", "", "-8", "resigned") + row(6, 3, "cy", "", "", "timed out"))
	f.Add(header + row(7, 1, "ann", "", "", "") + row(7, 1, "ann", "", "", ""))
	f.Fuzz(func(t *testing.T, in string) {
		got, err := Read("x.csv", strings.NewReader(in))
		if err != nil {
			return
		}
		if n := len(got.Seats) + len(got.Refused); n != got.Rows {
			t.Errorf("seats + refused = %d, want rows %d", n, got.Rows)
		}
		for _, g := range Games(got.Seats) {
			if places := Places(g.Seats); len(places) != len(g.Seats) {
				t.Errorf("session %d: %d places for %d seats", g.SessionID, len(places), len(g.Seats))
			}
		}
	})
}
