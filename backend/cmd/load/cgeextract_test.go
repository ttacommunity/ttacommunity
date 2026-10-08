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
		Matching: cgeextract.Matching{Matched: map[int64]int{10: 1}, Added: []int{2}, HeldBack: []int{3}, OursOnly: []int64{11, 12}},
	}
	refused := []cgeextract.Refusal{{Line: 7, Reason: cgeextract.NoPlayer}}
	var b strings.Builder

	reportCGEExtract(&b, plan, refused, nil)

	got := b.String()
	for _, want := range []string{"CGE games: 3", "matched to ours: 1", "only CGE has: 1 (running 1)",
		"held back as possible copies: 1", "  CODECCCC", "ours only: 2", "refused: 1", "  line 7 no_player",
		"nothing written: add -apply"} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "secretname") {
		t.Errorf("report names a player:\n%s", got)
	}
}
