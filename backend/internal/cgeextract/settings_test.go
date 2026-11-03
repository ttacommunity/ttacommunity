package cgeextract

import (
	"slices"
	"strings"
	"testing"
)

func TestReadSpecialRulesGivesEachGamesRulesAndCards(t *testing.T) {
	in := "tournament_id,tournament_name,session_id,share_code,rules,card_set\n" +
		"56,Cup,891270,ABCDEFGH,Tabletop,Random Mix\n57,Cup,889139,HGFEDCBA,Digital,Base Game\n57,Cup,x,ABCD,Digital,Base Game\n"
	got, err := ReadSpecialRules("rules.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadSpecialRules: %v", err)
	}
	want := map[int]Special{891270: {Rules: "Tabletop", CardSet: "Random Mix"}, 889139: {Rules: "Digital", CardSet: "Base Game"}}
	if len(got.Games) != 2 || got.Games[891270] != want[891270] || got.Games[889139] != want[889139] {
		t.Errorf("games = %+v, want %+v", got.Games, want)
	}
	if len(got.Refused) != 1 || got.Refused[0].Line != 4 || got.Refused[0].Reason != BadID {
		t.Errorf("refused = %+v, want line 4 as bad_id", got.Refused)
	}
}

const stagesFile = "tournament_id,tournament_name,stage_id,stage_number,stage_name,timeout_penalty,each_player_plays_2p," +
	"scoring_2p,each_player_plays_3p,scoring_3p,each_player_plays_4p,scoring_4p,ignore_ai_in_scoring," +
	"tiebreakers_2_players_tie,tiebreakers_3plus_players_tie,winner_score_minimum\n"

func TestReadStagesGivesEachStagesSettings(t *testing.T) {
	got, err := ReadStages("stages.csv", strings.NewReader(stagesFile+
		`5086,Quick and Dirty,3,1,Season 1,-1,0,,1,5-2-0,4,6-3-1-0,yes,"duelPositionsComparison, winnerScore",winnerScore,50`+"\n"))
	if err != nil {
		t.Fatalf("ReadStages: %v", err)
	}
	if len(got.Settings) != 1 || len(got.Refused) != 0 {
		t.Fatalf("settings %+v, refused %+v; want one stage", got.Settings, got.Refused)
	}
	s := got.Settings[0]
	if s.TournamentID != 5086 || s.StageID != 3 || s.StageNumber != 1 || s.TimeoutPenalty != -1 || !s.IgnoreAI ||
		s.WinnerScoreMinimum == nil || *s.WinnerScoreMinimum != 50 {
		t.Errorf("settings = %+v, want tournament 5086, stage 3, penalty -1, AI ignored, winner score at least 50", s)
	}
	if len(s.Points[2]) != 0 || !slices.Equal(s.Points[3], []float64{5, 2, 0}) || !slices.Equal(s.Points[4], []float64{6, 3, 1, 0}) {
		t.Errorf("points = %v, want 2p none, 3p 5-2-0, 4p 6-3-1-0", s.Points)
	}
	if s.EachPlays[2] != 0 || s.EachPlays[3] != 1 || s.EachPlays[4] != 4 {
		t.Errorf("each plays = %v, want 0, 1 and 4", s.EachPlays)
	}
	if s.Tiebreak2p != "duelPositionsComparison, winnerScore" || s.Tiebreak3pPlus != "winnerScore" {
		t.Errorf("tiebreaks = %q, %q; want CGE's, in order", s.Tiebreak2p, s.Tiebreak3pPlus)
	}
}

func TestReadStagesGivesNoneForEmptySettings(t *testing.T) {
	got, err := ReadStages("stages.csv", strings.NewReader(stagesFile+"19,Intermezzo,3,1,Season 1,0,1,1-0,0,,0,,no,,,\n"))
	if err != nil || len(got.Settings) != 1 {
		t.Fatalf("ReadStages: %+v, %v; want one stage", got, err)
	}
	s := got.Settings[0]
	if s.Tiebreak2p != "" || s.Tiebreak3pPlus != "" || s.WinnerScoreMinimum != nil {
		t.Errorf("tiebreaks %q, %q and winner score minimum %v; want none of each", s.Tiebreak2p, s.Tiebreak3pPlus, s.WinnerScoreMinimum)
	}
}

func TestReadStagesRefusesBadValues(t *testing.T) {
	tests := []struct{ name, line string }{
		{"a positive penalty", "5086,Q,3,1,,2,1,1-0,0,,0,,no,,,"},
		{"no penalty", "5086,Q,3,1,,,1,1-0,0,,0,,no,,,"},
		{"a negative count of games", "5086,Q,3,1,,0,-1,1-0,0,,0,,no,,,"},
		{"no count of games", "5086,Q,3,1,,0,,1-0,0,,0,,no,,,"},
		{"points for the wrong number of places", "5086,Q,3,1,,0,1,1-0-0,0,,0,,no,,,"},
		{"negative points", "5086,Q,3,1,,0,1,1--1,0,,0,,no,,,"},
		{"an AI setting that is not yes or no", "5086,Q,3,1,,0,1,1-0,0,,0,,,,,"},
		{"no stage", "5086,Q,0,1,,0,1,1-0,0,,0,,no,,,"},
		{"a winner score minimum over 100", "5086,Q,3,1,,0,1,1-0,0,,0,,no,,,101"},
		{"a negative winner score minimum", "5086,Q,3,1,,0,1,1-0,0,,0,,no,,,-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadStages("stages.csv", strings.NewReader(stagesFile+tt.line+"\n"))
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Settings) != 0 || len(got.Refused) != 1 || got.Refused[0].Line != 2 {
				t.Errorf("%s: settings %+v, refused %+v; want line 2 refused", tt.line, got.Settings, got.Refused)
			}
		})
	}
}

func TestReadStagesRefusesAnotherHeader(t *testing.T) {
	old := "tournament_id,tournament_name,stage_id,stage_number,stage_name,timeout_penalty,each_player_plays_2p," +
		"scoring_2p,each_player_plays_3p,scoring_3p,each_player_plays_4p,scoring_4p,ignore_ai_in_scoring\n"
	if _, err := ReadStages("stages.csv", strings.NewReader(old+"19,I,3,1,,0,1,1-0,0,,0,,no\n")); err == nil {
		t.Error("ReadStages read a stages file without tiebreakers; want an error naming the header it wants")
	}
}

func FuzzReadSettings(f *testing.F) {
	f.Add("tournament_id,tournament_name,session_id,share_code,rules,card_set\n56,Cup,891270,ABCDEFGH,Tabletop,Random Mix\n")
	f.Add("share_code,game_id,reason\nCODEAAAA,12,r\nCODEBBBB,x,r\n")
	f.Add(stagesFile + `5086,Quick and Dirty,3,1,Season 1,-1,0,,1,5-2-0,4,6-3-1-0,yes,"duelPositionsComparison, winnerScore",winnerScore,50` + "\n")
	f.Fuzz(func(t *testing.T, in string) {
		_, _ = ReadSpecialRules("r.csv", strings.NewReader(in))
		_, _ = ReadConfirmedPairs("p.csv", strings.NewReader(in))
		_, _ = ReadStages("stages.csv", strings.NewReader(in))
	})
}
