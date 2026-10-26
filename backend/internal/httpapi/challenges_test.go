package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

func (f *fakeRatings) Challenges(context.Context) ([]store.ChallengeRow, error) {
	return []store.ChallengeRow{
		{ID: "1ai", Set: "original", Group: "classic", Name: "First", Slot: 0, Difficulty: 1, Players: 2},
		{ID: "epic1_exp", Set: "expansion", Group: "epic", Name: "Long Road", Slot: 1, Difficulty: 4, Players: 3},
	}, nil
}

// ChallengeEntries are three entries for 1ai, the verified 120 holding the record, and one unproven for epic1_exp.
func (f *fakeRatings) ChallengeEntries(context.Context) ([]store.ChallengeEntry, error) {
	at := time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC)
	return []store.ChallengeEntry{
		{ID: 1, Challenge: "1ai", Player: "Ann", Domination: 120, ProofURL: "https://discord.com/channels/1/2/3", At: at},
		{ID: 2, Challenge: "1ai", Player: "Bob", Domination: 150, At: at},
		{ID: 3, Challenge: "1ai", Player: "Cy", Domination: 900, ProofURL: "https://discord.com/channels/1/2/4", Bugged: true, At: at},
		{ID: 4, Challenge: "epic1_exp", Player: "Ann", Domination: 80, At: at},
	}, nil
}

func TestChallengesCarryTheirVerifiedRecord(t *testing.T) {
	rec, _ := serve(t, &fakeRatings{}, "/v1/challenges")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/challenges = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "challenges.0", `{"id": "1ai", "set": "original", "group": "classic", "players": 2,
		"record": {"player": "Ann", "domination": 120, "proof_url": "https://discord.com/channels/1/2/3", "entered_at": "2025-03-01T10:00:00Z"}}`)
	wantJSON(t, rec.Body.Bytes(), "challenges.1", `{"id": "epic1_exp", "record": null}`)
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=60" {
		t.Errorf("Cache-Control = %q, want a minute", got)
	}
}
