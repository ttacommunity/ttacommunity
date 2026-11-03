package main

import (
	"strings"
	"testing"

	"ttacommunity/backend/internal/cgeextract"
	"ttacommunity/backend/internal/store"
)

func TestReportCGEExtractCountsAndNamesNoPlayer(t *testing.T) {
	plan := store.CGEPlan{
		Games: []cgeextract.Game{
			{SessionID: 1, ShareCode: "CODEAAAA", State: "finished", Seats: []cgeextract.Seat{{PlayerName: "secretname"}}},
			{SessionID: 2, ShareCode: "CODEBBBB", State: "running"},
			{SessionID: 3, ShareCode: "CODECCCC", State: "finished"},
		},
		Matching:      cgeextract.Matching{Matched: map[int64]int{10: 1}, Added: []int{2}, HeldBack: []int{3}, OursOnly: []int64{11, 12}},
		RunHere:       4,
		Removed:       []int64{20, 21},
		RemovedPlayed: 1,
	}
	stages := []store.CGEStageWrite{{StageID: 30, Settings: map[string]any{"points_2p": []float64{1, 0}}},
		{StageID: 31, Disagree: []string{"each_plays_2p", "timeout_penalty"}}}
	refused := []cgeextract.Refusal{{Line: 7, Reason: cgeextract.BadID}}
	var b strings.Builder

	reportCGEExtract(&b, plan, stages, refused, nil)

	got := b.String()
	for _, want := range []string{"CGE games: 3", "left out, in seasons run on this site: 4", "matched to ours: 1", "only CGE has: 1 (running 1)",
		"held back as possible copies: 1", "  CODECCCC", "ours only: 2", "removed by CGE: 2 (1 played)", "  game 20", "  game 21",
		"stages of ours with CGE stages now: 2", "settings their CGE stages disagree on, kept: 1", "  stage 31: each_plays_2p, timeout_penalty",
		"refused: 1", "  line 7 bad_id",
		"nothing written: add -apply"} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "secretname") {
		t.Errorf("report names a player:\n%s", got)
	}
}

func TestReportCGEExtractSaysWhatWasWritten(t *testing.T) {
	applied := store.CGEApplied{BatchID: 9, Added: 2, SeatsAdded: 3, CopiesRemoved: 1, SeatsRemoved: 2, GamesMoved: 4, PlayersRenamed: 5,
		GamesRemoved: 6}
	var b strings.Builder

	reportCGEExtract(&b, store.CGEPlan{}, nil, nil, &applied)

	got := b.String()
	for _, want := range []string{"batch 9: 2 games added", "3 seats added", "1 copies an earlier load added removed",
		"2 seats of players CGE removed dropped", "4 games moved into their CGE group", "5 players renamed after their account",
		"6 games CGE removed deleted"} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q:\n%s", want, got)
		}
	}
}
