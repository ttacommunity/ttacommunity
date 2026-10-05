package main

import (
	"encoding/csv"
	"strings"
	"testing"

	"ttacommunity/backend/internal/store"
)

var aCheck = store.StandingsCheck{
	StagesOnlyOnCGE: 3,
	Stages: []store.StageCheck{
		{CGETournament: 19, CGESeason: 4, Name: "International Championship", Players: 10, Agree: 7, Differences: []store.StandingDifference{
			{CGEUser: 1, Player: "secretname", Kind: store.DiffGames, CGEGames: 6, HeldGames: 5, CGEPoints: 12,
				CGEPlaces: store.Places{P3: []int{2, 2, 2}}, HeldPlaces: store.Places{P3: []int{2, 2, 1}}},
			{CGEUser: 2, Player: "other", Kind: store.DiffPlacesTimeout, CGEGames: 6, HeldGames: 6},
			{CGEUser: 3, Player: "third", Kind: store.DiffPlacesTimeout, CGEGames: 6, HeldGames: 6},
		}},
		{CGETournament: 21, CGESeason: 7, Name: "Intermezzo", Players: 4, Agree: 4},
	},
}

func TestReportStandingsCountsAndNamesNoPlayer(t *testing.T) {
	var b strings.Builder
	reportStandings(&b, aCheck, "d.csv")
	got := b.String()
	for _, want := range []string{
		"stages checked: 2 (3 only on CGE)",
		"players: 14, agree: 11 (78.6%)",
		"  games                     1\n  places_after_a_timeout    2\n",
		"  19/4 International Championship: 7 of 10 agree",
		"differences written to d.csv",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "21/7") {
		t.Errorf("report lists a stage where everyone agrees:\n%s", got)
	}
	if strings.Contains(got, "secretname") {
		t.Errorf("report names a player:\n%s", got)
	}
}

func TestWriteDifferencesGivesOneRowEach(t *testing.T) {
	var b strings.Builder
	if err := writeDifferences(&b, aCheck); err != nil {
		t.Fatalf("writeDifferences: %v", err)
	}
	rows, err := csv.NewReader(strings.NewReader(b.String())).ReadAll()
	if err != nil {
		t.Fatalf("reading what was written: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want a header and 3", len(rows))
	}
	header, first := strings.Join(rows[0], ","), strings.Join(rows[1], ",")
	if header != "cge_tournament,cge_season,name,cge_user,player,kind,cge_games,held_games,cge_points,cge_places,held_places" {
		t.Errorf("header = %s", header)
	}
	if first != "19,4,International Championship,1,secretname,games,6,5,12,3p 2 2 2,3p 2 2 1" {
		t.Errorf("first row = %s", first)
	}
}
