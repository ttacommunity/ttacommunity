package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

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
	replaced := store.FactsReplaced{Written: 2, Unknown: 1, Kept: 1}

	var b strings.Builder
	reportFacts(&b, "placements", "d.csv", 5, refused, replaced)
	got := b.String()

	for _, want := range []string{
		"file: d.csv",
		"rows read: 5",
		"placements written: 2",
		"not a player: 1",
		"kept as set in admin: 1",
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

func TestLoadFactsWritesNothingWhenARowIsRefused(t *testing.T) {
	file := filepath.Join(t.TempDir(), "p.csv")
	if err := os.WriteFile(file, []byte("cup\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RATINGS_DATABASE_URL", "postgres://127.0.0.1:1/never")
	read := func(string, io.Reader) (tournamentcsv.Result[tournamentcsv.Podium], error) {
		return tournamentcsv.Result[tournamentcsv.Podium]{
			Rows:    2,
			Facts:   []tournamentcsv.Podium{{Cup: "wimbledon"}},
			Refused: []tournamentcsv.Refusal{{File: file, Line: 3, Reason: tournamentcsv.BadSeries}},
		}, nil
	}
	replace := func(context.Context, *pgxpool.Pool, []tournamentcsv.Podium) (store.FactsReplaced, error) {
		t.Error("replace was called for a file with a refused row")
		return store.FactsReplaced{}, nil
	}
	err := loadFacts("podiums", file, read, replace)
	if err == nil || !strings.Contains(err.Error(), "1 of 2 rows refused") {
		t.Errorf("loadFacts = %v, want an error naming the 1 of 2 rows refused", err)
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
	set := store.CountriesSet{Set: 1, Kept: 2, Unknown: 1}

	var b strings.Builder
	reportCountries(&b, []string{"r.csv", "o.csv"}, res, set)
	got := b.String()

	for _, want := range []string{
		"files: r.csv, o.csv",
		"rows read: 9",
		"countries set: 1",
		"signed in, kept their own: 2",
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

func TestWriteInputTablesListsTheStoresInputTables(t *testing.T) {
	var b strings.Builder
	writeInputTables(&b)
	got := strings.Fields(b.String())
	want := append(slices.Clone(store.ExportTables), store.NotExported...)
	if !slices.Equal(got, want) {
		t.Errorf("got = %q, want %q", got, want)
	}
}

func TestRunRefusesAMissingOrWrongCommandLine(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want int
	}{
		{"no command", nil, 64},
		{"unknown command", []string{"frobnicate"}, 64},
		{"old flag form", []string{"-migrate"}, 64},
		{"argument where none is taken", []string{"migrate", "extra"}, 64},
		{"matches without a directory", []string{"matches"}, 64},
		{"matches with two directories", []string{"matches", "a", "b"}, 64},
		{"facts without a file", []string{"facts", "divisions"}, 64},
		{"countries without a file", []string{"countries"}, 64},
		{"add-admin without a name and role", []string{"add-admin", "100000000000000001"}, 64},
		{"cge-extract without settings", []string{"cge-extract", "extract"}, 64},
		{"input-tables with an argument", []string{"input-tables", "x"}, 64},
		{"unknown flag", []string{"migrate", "-nope"}, 2},
		{"help", []string{"migrate", "-h"}, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stderr strings.Builder
			if got := run(tt.args, &stderr); got != tt.want {
				t.Errorf("run(%q) = %d, want %d", tt.args, got, tt.want)
			}
			if !strings.Contains(stderr.String(), "usage: load") {
				t.Errorf("run(%q) printed no usage:\n%s", tt.args, stderr.String())
			}
		})
	}
}

func TestRunListsEveryCommand(t *testing.T) {
	var stderr strings.Builder
	run(nil, &stderr)
	for _, name := range []string{"migrate", "matches", "facts", "countries", "classify", "export", "backup", "backup-to", "backup-config-to", "restore", "export-upstream", "check-standings", "add-admin", "cge-extract", "input-tables"} {
		if !strings.Contains(stderr.String(), "load "+name+" ") && !strings.Contains(stderr.String(), "load "+name+"\n") {
			t.Errorf("usage does not list %q:\n%s", name, stderr.String())
		}
	}
}

func TestRunReachesEachCommand(t *testing.T) {
	t.Setenv("RATINGS_DATABASE_URL", "")
	t.Setenv("BACKUP_AGE_RECIPIENT", "")
	missing := filepath.Join(t.TempDir(), "missing")
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"migrate", []string{"migrate"}, "RATINGS_DATABASE_URL is not set"},
		{"classify", []string{"classify"}, "RATINGS_DATABASE_URL is not set"},
		{"matches", []string{"matches", missing}, "checking " + missing},
		{"matches with reset", []string{"matches", "-reset", missing}, "checking " + missing},
		{"countries", []string{"countries", missing}, "missing"},
		{"facts of an unknown kind", []string{"facts", "nonsense", missing}, `unknown kind of fact "nonsense"`},
		{"facts", []string{"facts", "divisions", missing}, "opening " + missing},
		{"export", []string{"export", missing}, "RATINGS_DATABASE_URL is not set"},
		{"backup", []string{"backup", missing}, "RATINGS_DATABASE_URL is not set"},
		{"backup-to", []string{"backup-to", missing}, "BACKUP_AGE_RECIPIENT is not set"},
		{"backup-config-to", []string{"backup-config-to", missing}, "BACKUP_AGE_RECIPIENT is not set"},
		{"restore", []string{"restore", missing}, "RATINGS_DATABASE_URL is not set"},
		{"export-upstream", []string{"export-upstream", missing}, "RATINGS_DATABASE_URL is not set"},
		{"check-standings", []string{"check-standings", missing}, "RATINGS_DATABASE_URL is not set"},
		{"add-admin", []string{"add-admin", "-name", "Local admin", "-role", "admin", "100000000000000001"}, "RATINGS_DATABASE_URL is not set"},
		{"cge-extract", []string{"cge-extract", "-settings", missing, missing}, "missing"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stderr strings.Builder
			if got := run(tt.args, &stderr); got != 1 {
				t.Errorf("run(%q) = %d, want 1", tt.args, got)
			}
			if got := stderr.String(); !strings.HasPrefix(got, "load: ") || !strings.Contains(got, tt.want) {
				t.Errorf("run(%q) printed %q, want a load error containing %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestUnknownFactKindNamesTheKnownOnes(t *testing.T) {
	err := replaceFacts("nonsense", "x.csv")
	for _, want := range []string{"divisions", "podiums", "community_ladders"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("replaceFacts(nonsense) = %v, want an error naming %q", err, want)
		}
	}
}
