package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

// ChallengeEntryPage records each page asked for and answers one hidden entry of 101.
func (f *fakeAdmins) ChallengeEntryPage(_ context.Context, page, size int) (store.AdminChallengeEntries, error) {
	f.notes = append(f.notes, "challenge entries page "+strconv.Itoa(page))
	f.pageSize = size
	member := int64(7)
	return store.AdminChallengeEntries{Total: 101, Entries: []store.AdminChallengeEntry{{
		ChallengeEntry: store.ChallengeEntry{ID: 3, Challenge: "1ai", Member: &member, Player: "Ann", Domination: 120,
			At: time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC)},
		ChallengeName: "First", EnteredAs: "ann", Source: "web", Hidden: true, HiddenReason: "a duplicate", Checked: true,
	}}}, nil
}

func TestAnOrganiserPagesThroughChallengeEntries(t *testing.T) {
	f := &fakeAdmins{}
	rec := adminDo(t, f, http.MethodGet, "/v1/admin/challenges/entries?page=2", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET challenge entries as an organiser = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "", `{"total": 101, "page": 2, "page_size": 100}`)
	wantJSON(t, rec.Body.Bytes(), "entries.0", `{"id": 3, "challenge_name": "First", "player": "Ann", "member": 7, "entered_as": "ann",
		"account": null, "proof_url": null, "source": "web", "hidden": true, "hidden_reason": "a duplicate", "checked": true}`)
	if len(f.notes) != 1 || f.notes[0] != "challenge entries page 2" || f.pageSize != 100 {
		t.Errorf("store asked for %q at %d a page, want page 2 at 100", f.notes, f.pageSize)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
}

func TestChallengeEntriesAreRefusedWhenTheyShouldBe(t *testing.T) {
	tests := []struct {
		name, who, path string
		want            int
	}{
		{"someone not in admin", "333333333333333333", "/v1/admin/challenges/entries", http.StatusForbidden},
		{"page 0", "222222222222222222", "/v1/admin/challenges/entries?page=0", http.StatusBadRequest},
		{"a page that is not a number", "222222222222222222", "/v1/admin/challenges/entries?page=two", http.StatusBadRequest},
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
