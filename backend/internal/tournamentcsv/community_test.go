package tournamentcsv

import (
	"slices"
	"strings"
	"testing"
)

const (
	standingsHeader = "edition,sheet_row,rank,player,points,events,source\n"
	scoresHeader    = "edition,player,event,score,source\n"
)

func TestReadCommunityStandingsTakesEveryColumn(t *testing.T) {
	in := standingsHeader + "2024,1,1,Willow_Tree,7657,11,TTA_Leaderboard_2024.csv\nrace26,3,2,wren,3970,11,s\n"
	got, err := ReadCommunityStandings("c.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadCommunityStandings: %v", err)
	}
	want := []Standing{
		{Edition: "2024", SheetRow: 1, Rank: 1, Player: "Willow_Tree", Points: 7657, Events: 11, Source: "TTA_Leaderboard_2024.csv"},
		{Edition: "race26", SheetRow: 3, Rank: 2, Player: "wren", Points: 3970, Events: 11, Source: "s"},
	}
	if !slices.Equal(got.Facts, want) || len(got.Refused) != 0 {
		t.Errorf("standings = %+v, refused %v; want %+v", got.Facts, got.Refused, want)
	}
}

func TestReadCommunityStandingsRefusesBadRows(t *testing.T) {
	tests := []struct {
		name string
		row  string
		want Reason
	}{
		{"unknown edition", "2019,1,1,ann,10,1,s", BadEdition},
		{"rank zero", "2024,1,0,ann,10,1,s", BadPlace},
		{"sheet row zero", "2024,0,1,ann,10,1,s", BadRow},
		{"no player", "2024,1,1,,10,1,s", BadPlayer},
		{"points not a number", "2024,1,1,ann,7 657,1,s", BadPoints},
		{"negative events", "2024,1,1,ann,10,-1,s", BadEvents},
		{"no source", "2024,1,1,ann,10,1,", BadSource},
		{"a player twice in an edition", "2024,1,1,ann,10,1,s\n2024,2,2,ann,9,1,s", Duplicate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadCommunityStandings("c.csv", strings.NewReader(standingsHeader+tt.row+"\n"))
			if err != nil {
				t.Fatalf("ReadCommunityStandings(%q): %v", tt.row, err)
			}
			if len(got.Refused) != 1 || got.Refused[0].Reason != tt.want {
				t.Errorf("ReadCommunityStandings(%q): refused = %v; want one %s", tt.row, got.Refused, tt.want)
			}
		})
	}
}

func TestReadCommunityScoresTakesEveryColumn(t *testing.T) {
	in := scoresHeader + "2024,Willow_Tree,wrld,1600,TTA_Leaderboard_2024.csv\n2025,pipit4,aus,1,s\n"
	got, err := ReadCommunityScores("s.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadCommunityScores: %v", err)
	}
	want := []Score{
		{Edition: "2024", Player: "Willow_Tree", Event: "wrld", Score: 1600, Source: "TTA_Leaderboard_2024.csv"},
		{Edition: "2025", Player: "pipit4", Event: "aus", Score: 1, Source: "s"},
	}
	if !slices.Equal(got.Facts, want) || len(got.Refused) != 0 {
		t.Errorf("scores = %+v, refused %v; want %+v", got.Facts, got.Refused, want)
	}
}

func TestReadCommunityScoresRefusesBadRows(t *testing.T) {
	tests := []struct {
		name string
		row  string
		want Reason
	}{
		{"unknown edition", "2019,ann,wrld,10,s", BadEdition},
		{"no event", "2024,ann,,10,s", BadEvent},
		{"score not a number", "2024,ann,wrld,lots,s", BadPoints},
		{"an event twice for a player", "2024,ann,wrld,10,s\n2024,ann,wrld,9,s", Duplicate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadCommunityScores("s.csv", strings.NewReader(scoresHeader+tt.row+"\n"))
			if err != nil {
				t.Fatalf("ReadCommunityScores(%q): %v", tt.row, err)
			}
			if len(got.Refused) != 1 || got.Refused[0].Reason != tt.want {
				t.Errorf("ReadCommunityScores(%q): refused = %v; want one %s", tt.row, got.Refused, tt.want)
			}
		})
	}
}
