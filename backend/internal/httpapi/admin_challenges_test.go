package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

const challengeAdminSite = "https://ttac-admin.example"

// ChallengeEntriesForAdmin answers four entries, newest first: Ann's hidden duplicate, Bob's unproven 300, Cy's
// verified and checked 120, and Ann's verified 100 not yet checked.
func (f *fakeAdmins) ChallengeEntriesForAdmin(context.Context) ([]store.AdminChallengeEntry, error) {
	f.notes = append(f.notes, "challenge entries")
	at := time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC)
	member := int64(7)
	entry := func(id int64, name string, d int, proof string, hours int) store.AdminChallengeEntry {
		return store.AdminChallengeEntry{ChallengeEntry: store.ChallengeEntry{ID: id, Challenge: "1ai", Player: name, Domination: d, ProofURL: proof,
			At: at.Add(time.Duration(hours) * time.Hour)}, ChallengeName: "First", EnteredAs: strings.ToLower(name), Source: "web"}
	}
	hidden := entry(4, "Ann", 90, "", 4)
	hidden.Member, hidden.Hidden, hidden.HiddenReason = &member, true, "a duplicate"
	checked := entry(2, "Cy", 120, "https://discord.com/channels/1/2/3", 2)
	checked.Checked = true
	return []store.AdminChallengeEntry{hidden, entry(3, "Bob", 300, "", 3), checked, entry(1, "Ann", 100, "https://discord.com/channels/1/2/4", 1)}, nil
}

func (f *fakeAdmins) SetChallengeEntry(_ context.Context, id int64, c store.ChallengeEntryChange) error {
	if id == 99 {
		return store.ErrNoChallengeEntry
	}
	var set []string
	if c.Domination != nil {
		set = append(set, fmt.Sprint("domination ", *c.Domination))
	}
	if c.ProofURL != nil {
		set = append(set, "proof "+*c.ProofURL)
	}
	if c.Hidden != nil {
		set = append(set, fmt.Sprint("hidden ", *c.Hidden))
	}
	f.notes = append(f.notes, fmt.Sprintf("set entry %d: %s", id, strings.Join(set, ", ")))
	return nil
}

func (f *fakeAdmins) MergeChallengeName(_ context.Context, from, to string) (store.ChallengeNameMerged, error) {
	if from == "nobody" {
		return store.ChallengeNameMerged{}, store.ErrNoChallengeEntry
	}
	f.notes = append(f.notes, "merge "+from+" into "+to)
	return store.ChallengeNameMerged{Moved: 2, Member: true}, nil
}

func TestAnOrganiserListsChallengeEntriesByView(t *testing.T) {
	tests := []struct {
		name, query string
		want        string
	}{
		{"every entry, newest first", "", `[{"id": 4, "hidden": true, "member": 7, "entered_as": "ann", "hidden_reason": "a duplicate"}, {"id": 3}, {"id": 2}, {"id": 1}]`},
		{"verified, not yet checked", "?view=unchecked", `[{"id": 1}]`},
		{"flagged", "?view=flagged", `[{"id": 3, "flags": ["no proof, over 2× everyone else's best (120 by Cy)"]}]`},
		{"hidden", "?view=hidden", `[{"id": 4}]`},
		{"a search, ignoring case", "?q=BOB", `[{"id": 3}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/challenges/entries"+tt.query, "", "")
			if rec.Code != http.StatusOK {
				t.Fatalf("GET = %d %s", rec.Code, rec.Body)
			}
			wantJSON(t, rec.Body.Bytes(), "entries", tt.want)
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
			}
		})
	}
	rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/challenges/entries?q=bob", "", "")
	wantJSON(t, rec.Body.Bytes(), "", `{"total": 1, "page": 1, "page_size": 100, "counts": [{"view": "all", "entries": 4}, {"view": "unchecked", "entries": 1},
		{"view": "checked", "entries": 1}, {"view": "unverified", "entries": 1}, {"view": "flagged", "entries": 1}, {"view": "bugged", "entries": 0},
		{"view": "hidden", "entries": 1}]}`)
}

func TestChallengeEntriesAreRefusedWhenTheyShouldBe(t *testing.T) {
	tests := []struct {
		name, who, path string
		want            int
	}{
		{"someone not in admin", "333333333333333333", "/v1/admin/challenges/entries", http.StatusForbidden},
		{"page 0", "222222222222222222", "/v1/admin/challenges/entries?page=0", http.StatusBadRequest},
		{"a page that is not a number", "222222222222222222", "/v1/admin/challenges/entries?page=two", http.StatusBadRequest},
		{"a view there is not", "222222222222222222", "/v1/admin/challenges/entries?view=odd", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			if rec := adminDoAs(t, f, tt.who, http.MethodGet, tt.path, "", ""); rec.Code != tt.want {
				t.Errorf("GET %s by %s = %d %s, want %d", tt.path, tt.name, rec.Code, rec.Body, tt.want)
			}
			if len(f.notes) != 0 {
				t.Errorf("a refused request reached the store: %q", f.notes)
			}
		})
	}
}

func TestAnOrganiserChangesAnEntryAndMergesANameAudited(t *testing.T) {
	f := &fakeAdmins{}
	rec := adminDo(t, f, http.MethodPatch, "/v1/admin/challenges/entries/3", challengeAdminSite,
		`{"domination": 30, "proof_url": " https://discordapp.com/channels/358698681672925195/5/6 ", "hidden": true, "hidden_reason": "a typo"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH = %d %s, want 204", rec.Code, rec.Body)
	}
	rec = adminDo(t, f, http.MethodPost, "/v1/admin/challenges/names", challengeAdminSite, `{"from": "anne", "to": "Ann"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST names = %d %s, want 200", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "", `{"moved": 2, "member": true}`)
	want := "set entry 3: domination 30, proof https://discord.com/channels/358698681672925195/5/6, hidden true|merge anne into Ann"
	if got := strings.Join(f.notes, "|"); got != want {
		t.Errorf("what reached the store = %q, want %q", got, want)
	}
	if got := strings.Join(f.audits, "|"); got != "222222222222222222 challenge.entry.set 3|222222222222222222 challenge.name.merge anne" {
		t.Errorf("audits = %q", got)
	}
}

func TestChallengeChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	tests := []struct {
		name, method, path, origin, body string
		want                             int
	}{
		{"nothing to change", http.MethodPatch, "/v1/admin/challenges/entries/3", challengeAdminSite, `{}`, http.StatusBadRequest},
		{"a proof from elsewhere", http.MethodPatch, "/v1/admin/challenges/entries/3", challengeAdminSite, `{"proof_url": "https://example.com"}`, http.StatusBadRequest},
		{"hiding without a reason", http.MethodPatch, "/v1/admin/challenges/entries/3", challengeAdminSite, `{"hidden": true}`, http.StatusBadRequest},
		{"no such entry", http.MethodPatch, "/v1/admin/challenges/entries/99", challengeAdminSite, `{"checked": true}`, http.StatusNotFound},
		{"not from the admin site", http.MethodPatch, "/v1/admin/challenges/entries/3", "https://elsewhere.example", `{"checked": true}`, http.StatusForbidden},
		{"a name with no entries", http.MethodPost, "/v1/admin/challenges/names", challengeAdminSite, `{"from": "nobody", "to": "Ann"}`, http.StatusNotFound},
		{"merging into nothing", http.MethodPost, "/v1/admin/challenges/names", challengeAdminSite, `{"from": "anne", "to": "  "}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			if rec := adminDo(t, f, tt.method, tt.path, tt.origin, tt.body); rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.audits) != 0 {
				t.Errorf("a refused change was audited: %q", f.audits)
			}
		})
	}
}
