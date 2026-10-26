package main

import (
	"strings"
	"testing"

	"ttacommunity/backend/internal/challenges"
	"ttacommunity/backend/internal/store"
)

func TestReportChallengeEntriesCountsAndNamesNoPlayer(t *testing.T) {
	read := challenges.EntriesRead{Rows: 5, Refused: []challenges.Refusal{{OldID: 7, Reason: "no domination"}}}
	imported := store.ChallengeEntriesImported{Replaced: 3, Imported: 3, Linked: 2,
		Refused: []challenges.Refusal{{OldID: 9, Reason: `no challenge "9ai"`}}}

	var b strings.Builder
	reportChallengeEntries(&b, "entries.json", read, imported)
	got := b.String()

	for _, want := range []string{
		"file: entries.json",
		"entries read: 5",
		"imported entries replaced: 3",
		"imported: 3, linked to a member: 2",
		"refused: 2",
		"  entry 7: no domination\n  entry 9: no challenge \"9ai\"\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}
}

func TestReportChallengesCounts(t *testing.T) {
	var b strings.Builder
	reportChallenges(&b, "challenges.json", 97, store.ChallengesReplaced{Kept: 95, Added: 2, Removed: 1})
	if want := "file: challenges.json\nchallenges read: 97\nkept: 95, added: 2, removed: 1\n"; b.String() != want {
		t.Errorf("report = %q, want %q", b.String(), want)
	}
}
