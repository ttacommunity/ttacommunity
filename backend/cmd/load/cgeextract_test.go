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
	refused := []cgeextract.Refusal{{Line: 7, Reason: cgeextract.BadID}}
	var b strings.Builder

	reportCGEExtract(&b, plan, refused, nil)

	got := b.String()
	for _, want := range []string{"CGE games: 3", "matched to ours: 1", "only CGE has: 1 (running 1)",
		"held back as possible copies: 1", "  CODECCCC", "ours only: 2", "refused: 1", "  line 7 bad_id",
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
	applied := store.CGEApplied{BatchID: 9, Added: 2, SeatsAdded: 3, CopiesRemoved: 1, UnknownSeatsRated: 2}
	var b strings.Builder

	reportCGEExtract(&b, store.CGEPlan{}, nil, &applied)

	got := b.String()
	for _, want := range []string{"batch 9: 2 games added", "3 seats added", "1 copies an earlier load added removed",
		"seats with no player in rated tournaments: 2"} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q:\n%s", want, got)
		}
	}
}
