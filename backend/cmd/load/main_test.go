package main

import (
	"strings"
	"testing"

	"ttacommunity/backend/internal/cgegames"
	"ttacommunity/backend/internal/countrycsv"
	"ttacommunity/backend/internal/matchcsv"
	"ttacommunity/backend/internal/store"
	"ttacommunity/backend/internal/tournamentcsv"
)

func TestReportCountsByReasonAndNamesNoPlayer(t *testing.T) {
	res := matchcsv.Result{
		Files: []string{"a.csv", "b.csv"},
		Rows:  6,
		Refused: []matchcsv.Refusal{
			{File: "a.csv", Line: 3, Reason: matchcsv.StandingsRow, Fields: []string{"Cup", "2024-01-02", "1.45secretname"}},
			{File: "a.csv", Line: 4, Reason: matchcsv.Duplicate, Fields: []string{"Cup", "2024-01-02", "secretname"}},
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
		"  duplicate        1\n  standings_row    2\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "secretname") {
		t.Errorf("report names a player:\n%s", got)
	}
}

func TestReportFactsCountsAndNamesNoPlayer(t *testing.T) {
	refused := []tournamentcsv.Refusal{
		{File: "d.csv", Line: 3, Reason: tournamentcsv.BadSeason},
		{File: "d.csv", Line: 5, Reason: tournamentcsv.Duplicate},
	}
	replaced := store.FactsReplaced{Written: 2, Unknown: 1}

	var b strings.Builder
	reportFacts(&b, "placements", "d.csv", 5, refused, replaced)
	got := b.String()

	for _, want := range []string{
		"file: d.csv",
		"rows read: 5",
		"placements written: 2",
		"not a player: 1",
		"refused: 2",
		"  d.csv:3 bad_season\n  d.csv:5 duplicate\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "secretname") {
		t.Errorf("report names a player:\n%s", got)
	}
}

func TestReportCountriesCountsAndNamesNoPlayer(t *testing.T) {
	res := countrycsv.Result{
		Rows:      9,
		Countries: map[string]string{"secretname": "PL", "other": "DE"},
		None:      4,
		Refused: []countrycsv.Refusal{
			{File: "r.csv", Line: 3, Reason: countrycsv.BadCode},
			{File: "o.csv", Line: 2, Reason: countrycsv.BadPlayer},
			{File: "r.csv", Line: 7, Reason: countrycsv.BadCode},
		},
	}
	set := store.CountriesSet{Set: 1, Unknown: 1}

	var b strings.Builder
	reportCountries(&b, []string{"r.csv", "o.csv"}, res, set)
	got := b.String()

	for _, want := range []string{
		"files: r.csv, o.csv",
		"rows read: 9",
		"countries set: 1",
		"not a player: 1",
		"no country: 4",
		"refused: 3",
		"  r.csv:3 bad_code\n  o.csv:2 bad_player\n  r.csv:7 bad_code\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "secretname") {
		t.Errorf("report names a player:\n%s", got)
	}
}

func TestReportClassifiedCountsAndNamesUnreadTournaments(t *testing.T) {
	var b strings.Builder
	reportClassified(&b, store.Classified{Placed: 5, Unplaced: []string{"Friendly game", "Odd Cup"}})
	got := b.String()

	for _, want := range []string{
		"placed in the catalogue: 5",
		"names no rule reads: 2 (their games stay unplaced)",
		"  Friendly game\n  Odd Cup\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}
}

func TestReportClassifiedSaysNothingIsUnplaced(t *testing.T) {
	var b strings.Builder
	reportClassified(&b, store.Classified{Placed: 3})
	if got := b.String(); strings.Contains(got, "no rule reads") {
		t.Errorf("report lists unread names when there are none:\n%s", got)
	}
}

func TestReportAttachedCountsAndNamesNoPlayer(t *testing.T) {
	var b strings.Builder
	reportAttached(&b, "g.csv", cgegames.Result{Refused: []cgegames.Refusal{{Line: 4, Reason: cgegames.BadSeat}}}, store.Added{
		Added: 5, NeverPlayed: 2, AlreadyHeld: 7, Unsettled: 1,
		Conflicts: []cgegames.Game{{CGETournament: 21, CGESeason: 9, Seats: []cgegames.Seat{{Name: "secretname"}}}},
	}, store.Attached{
		Matched:       3,
		Renamed:       2,
		LacksTimedOut: 4,
		NeverPlayed:   1,
		Unmatched: []cgegames.Game{
			{CGETournament: 21, CGESeason: 7, Seats: []cgegames.Seat{{Name: "secretname"}}},
			{CGETournament: 21, CGESeason: 7}, {CGETournament: 19, CGESeason: 2},
		},
		Seasons: []store.SeasonLink{{CGETournament: 21, CGESeason: 7, Season: "16", Games: 3}},
	})
	got := b.String()
	for _, want := range []string{
		"matched to our games: 3 (2 with a player renamed since, 4 our file records without a timed-out player)",
		"never played, set apart: 1",
		"no game of ours: 3 (CGE 19/2: 1, 21/7: 2)",
		"  CGE 21/7 -> season 16: 3 games\n",
		"refused: 1\n  g.csv:4 bad_seat\n",
		"added: 5 (2 of them never played)",
		"already held: 7",
		"placing not settled, not added: 1",
		"may be a copy of ours, not added: 1 (CGE 21/9: 1)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "secretname") {
		t.Errorf("report names a player:\n%s", got)
	}
}
