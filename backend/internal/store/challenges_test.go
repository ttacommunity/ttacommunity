//go:build integration

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/challenges"
)

// challenge is a valid challenge of the original set with this id and name.
func challenge(id, name string) challenges.Challenge {
	return challenges.Challenge{ID: id, Set: "original", Group: "classic", Difficulty: 1, Players: 2, Name: name,
		Rules: []string{"Win."}, Opponents: json.RawMessage(`[{"level": "easy", "name": null, "flags": {}}]`)}
}

// challengeLines writes the site's challenges as "id name".
func challengeLines(t *testing.T, r *RatingsReader) string {
	t.Helper()
	list, err := r.Challenges(context.Background())
	if err != nil {
		t.Fatalf("Challenges: %v", err)
	}
	var out []string
	for _, c := range list {
		out = append(out, c.ID+" "+c.Name)
	}
	return strings.Join(out, ", ")
}

// withChallengeEntries loads challenges 1ai and 2ai and imports entries, returning the report.
func withChallengeEntries(t *testing.T, pool *pgxpool.Pool, entries ...challenges.ImportedEntry) ChallengeEntriesImported {
	t.Helper()
	ctx := context.Background()
	if _, err := ReplaceChallenges(ctx, pool, []challenges.Challenge{challenge("1ai", "First"), challenge("2ai", "Second")}); err != nil {
		t.Fatalf("ReplaceChallenges: %v", err)
	}
	got, err := ImportChallengeEntries(ctx, pool, entries)
	if err != nil {
		t.Fatalf("ImportChallengeEntries: %v", err)
	}
	return got
}

// imported is an entry from the old site for challenge 1ai, by old id, name and domination, entered on day d of 2025.
func imported(id int64, name string, domination, d int) challenges.ImportedEntry {
	return challenges.ImportedEntry{OldID: id, Challenge: "1ai", Name: name, Domination: domination,
		At: time.Date(2025, 1, d, 12, 0, 0, 0, time.UTC), Source: "web"}
}

func TestReplaceChallengesKeepsTheListAndItsOrder(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	if _, err := ReplaceChallenges(ctx, pool, []challenges.Challenge{challenge("2ai", "Second"), challenge("1ai", "First")}); err != nil {
		t.Fatalf("ReplaceChallenges: %v", err)
	}
	got, err := ReplaceChallenges(ctx, pool, []challenges.Challenge{challenge("3ai", "Third"), challenge("2ai", "Second, renamed")})
	if err != nil {
		t.Fatalf("ReplaceChallenges again: %v", err)
	}
	if got != (ChallengesReplaced{Kept: 1, Added: 1, Removed: 1}) {
		t.Errorf("ReplaceChallenges = %+v, want 1 kept, 1 added, 1 removed", got)
	}
	if got, want := challengeLines(t, NewRatingsReader(asReader(t, pool))), "3ai Third, 2ai Second, renamed"; got != want {
		t.Errorf("challenges = %q, want %q", got, want)
	}
}

func TestReplaceChallengesRefusesToRemoveAChallengeWithEntries(t *testing.T) {
	pool := migrated(t)
	withChallengeEntries(t, pool, imported(1, "Ann", 50, 1))
	_, err := ReplaceChallenges(context.Background(), pool, []challenges.Challenge{challenge("2ai", "Second")})
	if !errors.Is(err, ErrChallengeInUse) {
		t.Fatalf("ReplaceChallenges without 1ai = %v, want ErrChallengeInUse", err)
	}
	if got, want := challengeLines(t, NewRatingsReader(pool)), "1ai First, 2ai Second"; got != want {
		t.Errorf("challenges after the refusal = %q, want %q", got, want)
	}
}

func TestImportChallengeEntriesLinksMembersByNameIgnoringCase(t *testing.T) {
	pool := migrated(t)
	if _, err := pool.Exec(context.Background(), "INSERT INTO members (tta_name) VALUES ('Ann')"); err != nil {
		t.Fatalf("adding Ann: %v", err)
	}
	unknown := imported(3, "Cy", 10, 3)
	unknown.Challenge = "9ai"
	got := withChallengeEntries(t, pool, imported(1, "ann", 50, 1), imported(2, "Zed", 40, 2), unknown)
	if got.Imported != 2 || got.Linked != 1 || len(got.Refused) != 1 || got.Refused[0].OldID != 3 {
		t.Errorf("ImportChallengeEntries = %+v, want 2 imported, 1 linked, entry 3 refused", got)
	}
	entries, err := NewRatingsReader(asReader(t, pool)).ChallengeEntries(context.Background())
	if err != nil {
		t.Fatalf("ChallengeEntries: %v", err)
	}
	var lines []string
	for _, e := range entries {
		lines = append(lines, fmt.Sprintf("%s linked=%v", e.Player, e.Member != nil))
	}
	if got, want := strings.Join(lines, ", "), "Ann linked=true, Zed linked=false"; got != want {
		t.Errorf("entries = %q, want %q (a linked entry shows the member's name)", got, want)
	}
}

func TestImportChallengeEntriesAgainReplacesTheImportedEntries(t *testing.T) {
	pool := migrated(t)
	withChallengeEntries(t, pool, imported(1, "Ann", 50, 1), imported(2, "Bob", 40, 2))
	got, err := ImportChallengeEntries(context.Background(), pool, []challenges.ImportedEntry{imported(1, "Ann", 55, 1)})
	if err != nil {
		t.Fatalf("ImportChallengeEntries again: %v", err)
	}
	if got.Replaced != 2 || got.Imported != 1 {
		t.Errorf("ImportChallengeEntries again = %+v, want 2 replaced, 1 imported", got)
	}
}

func TestTheSiteSeesNoHiddenEntry(t *testing.T) {
	pool := migrated(t)
	hidden := imported(2, "Bob", 90, 2)
	hidden.Hidden, hidden.HiddenReason = true, "a duplicate"
	withChallengeEntries(t, pool, imported(1, "Ann", 50, 1), hidden)
	entries, err := NewRatingsReader(asReader(t, pool)).ChallengeEntries(context.Background())
	if err != nil {
		t.Fatalf("ChallengeEntries: %v", err)
	}
	if len(entries) != 1 || entries[0].Player != "Ann" {
		t.Errorf("entries = %+v, want only Ann's", entries)
	}
}

func TestTheSiteLoginCannotReadAnEntrysAdminOnlyColumns(t *testing.T) {
	reader := asReader(t, migrated(t))
	for _, column := range []string{"account_id", "hidden_reason", "checked", "source"} {
		_, err := reader.Exec(context.Background(), "SELECT "+column+" FROM challenge_entries")
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Errorf("ratings_read reading %s = %v, want permission denied", column, err)
		}
	}
}

func TestAdminPagesThroughEveryEntryNewestFirst(t *testing.T) {
	pool := migrated(t)
	hidden := imported(2, "Bob", 90, 2)
	hidden.Hidden, hidden.HiddenReason, hidden.Checked = true, "a duplicate", true
	withChallengeEntries(t, pool, imported(1, "Ann", 50, 1), hidden, imported(3, "Cy", 70, 3))
	a := NewAdminStore(asAdminLogin(t, pool))
	ctx := context.Background()
	first, err := a.ChallengeEntryPage(ctx, 1, 2)
	if err != nil {
		t.Fatalf("ChallengeEntryPage(1): %v", err)
	}
	second, err := a.ChallengeEntryPage(ctx, 2, 2)
	if err != nil {
		t.Fatalf("ChallengeEntryPage(2): %v", err)
	}
	var lines []string
	for _, e := range append(first.Entries, second.Entries...) {
		lines = append(lines, fmt.Sprintf("%s %s %d hidden=%q checked=%v", e.Player, e.ChallengeName, e.Domination, e.HiddenReason, e.Checked))
	}
	want := `Cy First 70 hidden="" checked=false, Bob First 90 hidden="a duplicate" checked=true, Ann First 50 hidden="" checked=false`
	if got := strings.Join(lines, ", "); got != want {
		t.Errorf("pages = %q, want %q", got, want)
	}
	if first.Total != 3 || second.Total != 3 {
		t.Errorf("totals = %d and %d, want 3", first.Total, second.Total)
	}
}

func TestReplaceChallengeWeeksKeepsTheLatestFile(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	withChallengeEntries(t, pool)
	of := time.Date(2024, 9, 22, 0, 0, 0, 0, time.UTC)
	first := []challenges.Week{
		{Number: 1, Of: of, Challenge: "1ai", Winners: []challenges.Placing{{Name: "Ann", Domination: 90}}},
		{Number: 2, Of: of.AddDate(0, 0, 7), Challenge: "2ai"},
	}
	if _, err := ReplaceChallengeWeeks(ctx, pool, first); err != nil {
		t.Fatalf("ReplaceChallengeWeeks: %v", err)
	}
	second := []challenges.Week{{Number: 1, Of: of, Challenge: "2ai", Winners: []challenges.Placing{{Name: "Bob", Domination: 80}, {Name: "Cy", Domination: 80}},
		RunnersUp: []challenges.Placing{{Name: "Ann", Domination: 70}}}}
	got, err := ReplaceChallengeWeeks(ctx, pool, second)
	if err != nil {
		t.Fatalf("ReplaceChallengeWeeks again: %v", err)
	}
	if got != 1 {
		t.Errorf("ReplaceChallengeWeeks = %d weeks, want 1", got)
	}
	weeks, err := NewRatingsReader(asReader(t, pool)).ChallengeWeeks(ctx)
	if err != nil {
		t.Fatalf("ChallengeWeeks: %v", err)
	}
	if len(weeks) != 1 || weeks[0].Challenge != "2ai" || len(weeks[0].Winners) != 2 || weeks[0].Winners[1].Name != "Cy" || weeks[0].RunnersUp[0].Name != "Ann" {
		t.Errorf("weeks = %+v, want week 1 on 2ai won by Bob and Cy, Ann second", weeks)
	}
}

func TestReplaceChallengeWeeksRefusesAnUnknownChallenge(t *testing.T) {
	pool := migrated(t)
	withChallengeEntries(t, pool)
	_, err := ReplaceChallengeWeeks(context.Background(), pool, []challenges.Week{{Number: 1, Of: time.Now(), Challenge: "nope"}})
	if err == nil {
		t.Error("ReplaceChallengeWeeks took a week of an unknown challenge")
	}
}
