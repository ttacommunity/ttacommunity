//go:build integration

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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

func TestImportChallengeEntriesLinksMembersByNameAddingTheNewOnes(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "INSERT INTO members (tta_name) VALUES ('Ann'); INSERT INTO players (name) VALUES ('Pat')"); err != nil {
		t.Fatalf("adding Ann and the player Pat: %v", err)
	}
	unknown := imported(5, "Cy", 10, 5)
	unknown.Challenge = "9ai"
	got := withChallengeEntries(t, pool, imported(1, "ann", 50, 1), imported(2, "Zed", 40, 2), imported(3, "zed", 45, 3), imported(4, "pat", 30, 4), unknown)
	if got.Imported != 4 || got.Linked != 4 || got.Members != 2 || len(got.Refused) != 1 || got.Refused[0].OldID != 5 {
		t.Errorf("ImportChallengeEntries = %+v, want 4 imported, all linked, 2 members added, entry 5 refused", got)
	}
	entries, err := NewRatingsReader(asReader(t, pool)).ChallengeEntries(ctx)
	if err != nil {
		t.Fatalf("ChallengeEntries: %v", err)
	}
	var lines []string
	for _, e := range entries {
		lines = append(lines, fmt.Sprintf("%s linked=%v", e.Player, e.Member != nil))
	}
	if got, want := strings.Join(lines, ", "), "Ann linked=true, Zed linked=true, Zed linked=true, Pat linked=true"; got != want {
		t.Errorf("entries = %q, want %q (a linked entry shows the member's name)", got, want)
	}
	members := rowsOf(t, pool, "SELECT tta_name || ':' || coalesce(player_id::text, '-') FROM members ORDER BY tta_name")
	if len(members) != 3 || members[0] != "Ann:-" || !strings.HasPrefix(members[1], "Pat:") || members[1] == "Pat:-" || members[2] != "Zed:-" {
		t.Errorf("members = %q, want Ann as before, Pat added with the player of that name, Zed added without one", members)
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

// adminLines lists every entry as admin sees it, newest first, as "player domination hidden=reason checked bugged".
func adminLines(t *testing.T, a *AdminStore) string {
	t.Helper()
	entries, err := a.ChallengeEntriesForAdmin(context.Background())
	if err != nil {
		t.Fatalf("ChallengeEntriesForAdmin: %v", err)
	}
	var lines []string
	for _, e := range entries {
		lines = append(lines, fmt.Sprintf("%s %s %d hidden=%q checked=%v bugged=%v", e.Player, e.ChallengeName, e.Domination, e.HiddenReason, e.Checked, e.Bugged))
	}
	return strings.Join(lines, ", ")
}

func TestAdminListsEveryEntryNewestFirst(t *testing.T) {
	pool := migrated(t)
	hidden := imported(2, "Bob", 90, 2)
	hidden.Hidden, hidden.HiddenReason, hidden.Checked = true, "a duplicate", true
	withChallengeEntries(t, pool, imported(1, "Ann", 50, 1), hidden, imported(3, "Cy", 70, 3))
	want := `Cy First 70 hidden="" checked=false bugged=false, Bob First 90 hidden="a duplicate" checked=true bugged=false, Ann First 50 hidden="" checked=false bugged=false`
	if got := adminLines(t, NewAdminStore(asAdminLogin(t, pool))); got != want {
		t.Errorf("entries = %q, want %q", got, want)
	}
}

func TestAdminChangesAnEntry(t *testing.T) {
	pool := migrated(t)
	withChallengeEntries(t, pool, imported(1, "Ann", 50, 1))
	a := NewAdminStore(asAdminLogin(t, pool))
	ctx := context.Background()
	entries, err := a.ChallengeEntriesForAdmin(ctx)
	if err != nil {
		t.Fatalf("ChallengeEntriesForAdmin: %v", err)
	}
	id := entries[0].ID
	d, yes, reason := 60, true, "a typo"
	if err := a.SetChallengeEntry(ctx, id, ChallengeEntryChange{Domination: &d, Checked: &yes, Bugged: &yes, Hidden: &yes, HiddenReason: &reason}); err != nil {
		t.Fatalf("SetChallengeEntry: %v", err)
	}
	if got, want := adminLines(t, a), `Ann First 60 hidden="a typo" checked=true bugged=true`; got != want {
		t.Errorf("entry = %q, want %q", got, want)
	}
	no := false
	if err := a.SetChallengeEntry(ctx, id, ChallengeEntryChange{Hidden: &no}); err != nil {
		t.Fatalf("SetChallengeEntry (shown again): %v", err)
	}
	if got, want := adminLines(t, a), `Ann First 60 hidden="" checked=true bugged=true`; got != want {
		t.Errorf("shown again = %q, want %q: showing clears the reason", got, want)
	}
	if err := a.SetChallengeEntry(ctx, id, ChallengeEntryChange{ProofURL: new(" https://canary.discordapp.com/channels/358698681672925195/5/6 ")}); err != nil {
		t.Fatalf("SetChallengeEntry (proof): %v", err)
	}
	if got, want := rowsOf(t, pool, "SELECT proof_url FROM challenge_entries"), []string{"https://discord.com/channels/358698681672925195/5/6"}; !slices.Equal(got, want) {
		t.Errorf("proof = %q, want %q: written the one way", got, want)
	}
	tests := []struct {
		name string
		id   int64
		c    ChallengeEntryChange
		want error
	}{
		{"no such entry", id + 99, ChallengeEntryChange{Checked: &yes}, ErrNoChallengeEntry},
		{"hidden without a reason", id, ChallengeEntryChange{Hidden: &yes}, ErrBadChallengeEntry},
		{"a domination out of bounds", id, ChallengeEntryChange{Domination: new(10000)}, ErrBadChallengeEntry},
		{"a proof that is not a link", id, ChallengeEntryChange{ProofURL: new("not a link")}, ErrBadChallengeEntry},
		{"a proof from elsewhere", id, ChallengeEntryChange{ProofURL: new("https://example.com/channels/358698681672925195/5/6")}, ErrBadChallengeEntry},
		{"a channel, not a message", id, ChallengeEntryChange{ProofURL: new("https://discord.com/channels/358698681672925195/5")}, ErrBadChallengeEntry},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := a.SetChallengeEntry(ctx, tt.id, tt.c); !errors.Is(err, tt.want) {
				t.Errorf("SetChallengeEntry = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestAdminMergesANameIntoAMember(t *testing.T) {
	pool := migrated(t)
	withChallengeEntries(t, pool, imported(1, "Anne", 50, 1), imported(2, "anne", 55, 2), imported(3, "Cy", 70, 3))
	a := NewAdminStore(asAdminLogin(t, pool))
	ctx := context.Background()
	if _, err := a.AddMember(ctx, Member{Name: "cyrus", Zone: "UTC"}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	week := []challenges.Week{{Number: 1, Of: time.Date(2024, 9, 22, 0, 0, 0, 0, time.UTC), Challenge: "1ai", Winners: []challenges.Placing{{Name: "CY", Domination: 70}}}}
	if _, err := ReplaceChallengeWeeks(ctx, pool, week); err != nil {
		t.Fatalf("ReplaceChallengeWeeks: %v", err)
	}
	got, err := a.MergeChallengeName(ctx, " cy ", "Cyrus")
	if err != nil || got.Moved != 2 {
		t.Fatalf("MergeChallengeName = %+v, %v, want 2 moved: the entry and the week's placing, both under the member the import made", got, err)
	}
	if lines, want := adminLines(t, a), "cyrus First 70"; !strings.HasPrefix(lines, want) || !strings.Contains(lines, "Anne First 55") {
		t.Errorf("entries = %q, want Cy's entry under cyrus and both Anne entries under Anne", lines)
	}
	if weeks, err := NewRatingsReader(asReader(t, pool)).ChallengeWeeks(ctx); err != nil || len(weeks) != 1 || weeks[0].Winners[0].Name != "cyrus" {
		t.Errorf("weeks = %+v, %v, want the placing shown as the member cyrus", weeks, err)
	}
	for _, tt := range []struct {
		name, from, to string
		want           error
	}{
		{"no member of the name to", "anne", "Ann", ErrNoMember},
		{"nothing under from", "nobody", "Cyrus", ErrNoChallengeEntry},
		{"already theirs", "cyrus", "Cyrus", ErrNoChallengeEntry},
		{"an empty name", "", "Cyrus", ErrBadChallengeEntry},
	} {
		if _, err := a.MergeChallengeName(ctx, tt.from, tt.to); !errors.Is(err, tt.want) {
			t.Errorf("%s: MergeChallengeName(%q, %q) = %v, want %v", tt.name, tt.from, tt.to, err, tt.want)
		}
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

func TestReplaceChallengeWeeksLinksPlacingsToMembers(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	withChallengeEntries(t, pool)
	if _, err := NewAdminStore(asAdminLogin(t, pool)).AddMember(ctx, Member{Name: "ann", Zone: "UTC"}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	week := []challenges.Week{{Number: 1, Of: time.Date(2024, 9, 22, 0, 0, 0, 0, time.UTC), Challenge: "1ai",
		Winners: []challenges.Placing{{Name: "ANN", Domination: 90}}, RunnersUp: []challenges.Placing{{Name: "Zed", Domination: 80}}}}
	if _, err := ReplaceChallengeWeeks(ctx, pool, week); err != nil {
		t.Fatalf("ReplaceChallengeWeeks: %v", err)
	}
	weeks, err := NewRatingsReader(asReader(t, pool)).ChallengeWeeks(ctx)
	if err != nil {
		t.Fatalf("ChallengeWeeks: %v", err)
	}
	if len(weeks) != 1 || weeks[0].Winners[0].Name != "ann" || weeks[0].RunnersUp[0].Name != "Zed" {
		t.Errorf("weeks = %+v, want the winner shown as the member ann and the runner-up as entered", weeks)
	}
	if linked := rowsOf(t, pool, "SELECT name || ':' || coalesce(member_id::text, '-') FROM challenge_week_places ORDER BY place DESC"); !slices.Equal(linked, []string{"ANN:" + linked[0][4:], "Zed:-"}) || linked[0] == "ANN:-" {
		t.Errorf("placings = %q, want ANN linked to a member and Zed to none", linked)
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
