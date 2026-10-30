package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/challenges"
	"ttacommunity/backend/internal/store"
)

func (f *fakeSignIns) SubmitChallengeEntries(_ context.Context, hash []byte, entries []challenges.Submission, _ time.Time) (int, error) {
	if f.reportErr != nil {
		return 0, f.reportErr
	}
	for _, e := range entries {
		f.notes = append(f.notes, fmt.Sprintf("%s by %s: %d %s", e.Challenge, f.sessions[string(hash)].Username, e.Domination, e.ProofURL))
	}
	return len(entries), nil
}

func TestAPlayerSubmitsEntries(t *testing.T) {
	f := newFakeSignIns()
	cookie := sessionFor(f, "token", member(1007, "ann"))
	body := `{"entries": [{"challenge": "1ai", "domination": 90, "proof_url": " https://discord.com/channels/358698681672925195/1/2 "},
		{"challenge": "2ai", "domination": -5}]}`
	rec := playerDo(t, f, http.MethodPost, "/v1/me/challenges/entries", siteOrigin, body, cookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST = %d %s, want 201", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "saved", `2`)
	want := "1ai by ann: 90 https://discord.com/channels/358698681672925195/1/2|2ai by ann: -5 "
	if got := strings.Join(f.notes, "|"); got != want {
		t.Errorf("what reached the store = %q, want %q", got, want)
	}
}

func TestSubmittingEntriesIsRefusedWithTheStatusItNeeds(t *testing.T) {
	good := `{"entries": [{"challenge": "1ai", "domination": 90}]}`
	tests := []struct {
		name, body, origin string
		err                error
		want               int
	}{
		{"a proof from elsewhere", `{"entries": [{"challenge": "1ai", "domination": 90, "proof_url": "https://example.com/x"}]}`, siteOrigin, store.ErrBadProofURL, http.StatusBadRequest},
		{"one challenge twice", `{"entries": [{"challenge": "1ai", "domination": 1}, {"challenge": "1ai", "domination": 2}]}`, siteOrigin, nil, http.StatusBadRequest},
		{"no entries", `{"entries": []}`, siteOrigin, nil, http.StatusBadRequest},
		{"a challenge the database does not know", good, siteOrigin, store.ErrBadChallengeEntries, http.StatusBadRequest},
		{"an ended session", good, siteOrigin, store.ErrNoSession, http.StatusUnauthorized},
		{"not linked", good, siteOrigin, store.ErrNotLinked, http.StatusForbidden},
		{"banned", good, siteOrigin, store.ErrMemberBanned, http.StatusForbidden},
		{"not from the site", good, "https://elsewhere.example", nil, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSignIns()
			f.reportErr = tt.err
			cookie := sessionFor(f, "token", member(1007, "ann"))
			rec := playerDo(t, f, http.MethodPost, "/v1/me/challenges/entries", tt.origin, tt.body, cookie)
			if rec.Code != tt.want {
				t.Errorf("POST = %d %s, want %d", rec.Code, rec.Body, tt.want)
			}
			if tt.err == nil && len(f.notes) > 0 {
				t.Errorf("a refused submission reached the store: %v", f.notes)
			}
		})
	}
}
