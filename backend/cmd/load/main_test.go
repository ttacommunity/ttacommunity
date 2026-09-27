package main

import (
	"strings"
	"testing"

	"ttacommunity/backend/internal/matchcsv"
	"ttacommunity/backend/internal/store"
)

func TestReportCountsByReasonAndNamesNoPlayer(t *testing.T) {
	res := matchcsv.Result{
		Files: []string{"a.csv", "b.csv"},
		Rows:  6,
		Refused: []matchcsv.Refusal{
			{File: "a.csv", Line: 3, Reason: matchcsv.StandingsRow, Fields: []string{"Cup", "2024-01-02", "1.45secretname"}},
			{File: "a.csv", Line: 4, Reason: matchcsv.AllScoresZero, Fields: []string{"Cup", "2024-01-02", "secretname"}},
			{File: "b.csv", Line: 2, Reason: matchcsv.StandingsRow, Fields: []string{"Cup", "2024-01-02", "2.46secretname"}},
		},
	}
	loaded := store.Loaded{BatchID: 7, Matches: 3, Refused: 3, Players: 4}

	var b strings.Builder
	report(&b, res, loaded)
	got := b.String()

	for _, want := range []string{
		"files: 2 (a.csv, b.csv)",
		"rows read: 6",
		"loaded: 3 matches, 4 players (batch 7)",
		"refused: 3",
		"  all_scores_zero  1\n  standings_row    2\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "secretname") {
		t.Errorf("report names a player:\n%s", got)
	}
}
