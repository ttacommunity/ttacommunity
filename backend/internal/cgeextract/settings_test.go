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

const tournamentSettings = "tournament_id,each_plays,timeout_penalty,ignore_ai_players,scoring_2p,scoring_3p,scoring_4p," +
	"tiebreak_2p,tiebreak_3p_plus,stages_read,stages_in_extract\n"

const stageSettings = "tournament_id,stage_id,stage_number,each_plays,timeout_penalty,ignore_ai_players,scoring_2p," +
	"scoring_3p,scoring_4p,tiebreak_2p,tiebreak_3p_plus,source,fetched\n"

func TestReadSettingsTellsInheritedFromNone(t *testing.T) {
	got, err := ReadTournamentSettings("t.csv", strings.NewReader(tournamentSettings+
		`19,6,-1,yes,-,5-2-0,,"duelPositionsComparison, winnerScore",-,26,26`+"\n"))
	if err != nil {
		t.Fatalf("ReadTournamentSettings: %v", err)
	}
	s := got.Settings[0]
	if s.TournamentID != 19 || s.StageID != 0 || *s.EachPlays != 6 || *s.TimeoutPenalty != -1 || !*s.IgnoreAI {
		t.Errorf("settings = %+v", s)
	}
	if s.Points[2] == nil || len(*s.Points[2]) != 0 || !slices.Equal(*s.Points[3], []float64{5, 2, 0}) || s.Points[4] != nil {
		t.Errorf("points = %v, want 2p none, 3p 5-2-0, 4p inherited", s.Points)
	}
	if *s.Tiebreak2p != "duelPositionsComparison, winnerScore" || *s.Tiebreak3pPlus != "" {
		t.Errorf("tiebreaks = %q, %q; want the 2-way list and none for 3+", *s.Tiebreak2p, *s.Tiebreak3pPlus)
	}
}

func TestReadStageSettingsGivesOnlyWhatDiffers(t *testing.T) {
	got, err := ReadStageSettings("s.csv", strings.NewReader(stageSettings+
		"19,3,1,,-1,no,,,,-,-,/tournaments/detail/19/1,2026-10-01\n"))
	if err != nil {
		t.Fatalf("ReadStageSettings: %v", err)
	}
	s := got.Settings[0]
	if s.StageID != 3 || s.EachPlays != nil || *s.TimeoutPenalty != -1 || *s.IgnoreAI || s.Points[3] != nil ||
		*s.Tiebreak2p != "" || s.Source != "/tournaments/detail/19/1" {
		t.Errorf("settings = %+v, want penalty -1, no AI, no tiebreaks, the rest inherited", s)
	}
}

func TestReadSettingsRefusesBadValues(t *testing.T) {
	tests := []struct{ name, line string }{
		{"a positive penalty", "19,6,2,yes,-,5-2-0,,-,-,1,1"},
		{"points for the wrong number of places", "19,6,0,yes,-,5-2,,-,-,1,1"},
		{"an AI setting that is not yes or no", "19,6,0,maybe,-,5-2-0,,-,-,1,1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadTournamentSettings("t.csv", strings.NewReader(tournamentSettings+tt.line+"\n"))
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Settings) != 0 || len(got.Refused) != 1 {
				t.Errorf("settings %d, refused %+v; want the row refused", len(got.Settings), got.Refused)
			}
		})
	}
}

func FuzzReadSettings(f *testing.F) {
	f.Add("tournament_id,tournament_name,session_id,share_code,rules,card_set\n56,Cup,891270,ABCDEFGH,Tabletop,Random Mix\n")
	f.Add("share_code,game_id,reason\nCODEAAAA,12,r\nCODEBBBB,x,r\n")
	f.Add(tournamentSettings + `19,6,-1,yes,-,5-2-0,,"duelPositionsComparison, winnerScore",-,26,26` + "\n")
	f.Add(stageSettings + "19,3,1,,-1,no,,,,-,-,/tournaments/detail/19/1,2026-10-01\n")
	f.Fuzz(func(t *testing.T, in string) {
		_, _ = ReadSpecialRules("r.csv", strings.NewReader(in))
		_, _ = ReadConfirmedPairs("p.csv", strings.NewReader(in))
		_, _ = ReadTournamentSettings("t.csv", strings.NewReader(in))
		_, _ = ReadStageSettings("s.csv", strings.NewReader(in))
	})
}
