package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"ttacommunity/backend/internal/challenges"
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
		{ID: 2, Challenge: "1ai", Player: "Bob", Domination: 150, At: at.Add(time.Hour)},
		{ID: 3, Challenge: "1ai", Player: "Cy", Domination: 900, ProofURL: "https://discord.com/channels/1/2/4", Bugged: true, At: at},
		{ID: 4, Challenge: "epic1_exp", Player: "Ann", Domination: 80, At: at},
	}, nil
}

// ChallengeWeeks are two weeks, the first won by two tied names.
func (f *fakeRatings) ChallengeWeeks(context.Context) ([]challenges.Week, error) {
	of := time.Date(2024, 9, 22, 0, 0, 0, 0, time.UTC)
	return []challenges.Week{
		{Number: 1, Of: of, Challenge: "1ai", Winners: []challenges.Placing{{Name: "Ann", Domination: 90}, {Name: "Bob", Domination: 90}}},
		{Number: 2, Of: of.AddDate(0, 0, 7), Challenge: "epic1_exp", RunnersUp: []challenges.Placing{{Name: "Cy", Domination: 40}}},
	}, nil
}

func TestChallengeWeeks(t *testing.T) {
	rec, _ := serve(t, &fakeRatings{}, "/v1/challenges/weekly")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "weeks", `[
		{"week": 1, "week_of": "2024-09-22", "challenge": "1ai", "winners": [{"player": "Ann", "domination": 90}, {"player": "Bob"}], "runners_up": []},
		{"week": 2, "winners": [], "runners_up": [{"player": "Cy", "domination": 40}]}]`)
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=86400" {
		t.Errorf("Cache-Control = %q, want a day", got)
	}
}

func TestChallengeHighscores(t *testing.T) {
	tests := []struct {
		name, path, check, want string
	}{
		{"verified only: the verified record", "/v1/challenges", "challenges.0",
			`{"id": "1ai", "set": "original", "group": "classic", "players": 2, "entrants": 1,
			  "record": {"player": "Ann", "domination": 120, "proof_url": "https://discord.com/channels/1/2/3", "entered_at": "2025-03-01T10:00:00Z", "rank": 1}}`},
		{"verified only: no record without a verified entry", "/v1/challenges", "challenges.1", `{"id": "epic1_exp", "record": null, "entrants": 0}`},
		{"all entries: the unproven record", "/v1/challenges?verified=false", "challenges.0", `{"entrants": 2, "record": {"player": "Bob", "proof_url": null}}`},
		{"a set, then its groups, then everything", "/v1/challenges", "scopes",
			`[{"set": "original", "group": ""}, {"set": "original", "group": "classic"}, {"set": "expansion", "group": ""},
			  {"set": "expansion", "group": "epic"}, {"set": "all", "group": "", "pooled": 120, "covered": 1, "size": 2,
			   "by_total": [{"player": "Ann", "total": 120, "played": 1}]}]`},
		{"most records held", "/v1/challenges?verified=false", "record_holders", `[{"player": "Ann", "records": 1}, {"player": "Bob", "records": 1}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, _ := serve(t, &fakeRatings{}, tt.path)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s = %d %s", tt.path, rec.Code, rec.Body)
			}
			wantJSON(t, rec.Body.Bytes(), tt.check, tt.want)
			if got := rec.Header().Get("Cache-Control"); got != "public, max-age=60" {
				t.Errorf("Cache-Control = %q, want a minute", got)
			}
		})
	}
}

func TestChallengeBoard(t *testing.T) {
	tests := []struct {
		name, path  string
		status      int
		check, want string
	}{
		{"the ranking", "/v1/challenges/1ai?verified=false", http.StatusOK, "ranking",
			`[{"player": "Bob", "domination": 150, "rank": 1}, {"player": "Ann", "domination": 120, "rank": 2}]`},
		{"the record history", "/v1/challenges/1ai?verified=false", http.StatusOK, "history",
			`[{"player": "Ann", "gain": null}, {"player": "Bob", "gain": 30}]`},
		{"every entry, newest first, bugged ones too", "/v1/challenges/1ai", http.StatusOK, "entries",
			`[{"player": "Bob", "bugged": false}, {"player": "Cy", "bugged": true}, {"player": "Ann"}]`},
		{"an unknown challenge", "/v1/challenges/nope", http.StatusNotFound, "", `{}`},
		{"an id no challenge can have", "/v1/challenges/Bad-Id", http.StatusBadRequest, "", `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, _ := serve(t, &fakeRatings{}, tt.path)
			if rec.Code != tt.status {
				t.Fatalf("GET %s = %d %s, want %d", tt.path, rec.Code, rec.Body, tt.status)
			}
			wantJSON(t, rec.Body.Bytes(), tt.check, tt.want)
		})
	}
}

func TestChallengePlayers(t *testing.T) {
	rec, _ := serve(t, &fakeRatings{}, "/v1/challenges/players?verified=false")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "players", `[
		{"player": "Ann", "played": 2, "verified": 1, "records": 1, "total": 200, "totals": {"original": 120, "expansion": 80, "mix": 0}},
		{"player": "Bob", "played": 1, "records": 1, "total": 150}]`)
}

func TestChallengePlayer(t *testing.T) {
	tests := []struct {
		name, path  string
		status      int
		check, want string
	}{
		{"matched ignoring case", "/v1/challenges/players/ann", http.StatusOK, "",
			`{"player": "Ann", "unproven": 1, "summary": {"played": 1, "records": 1},
			  "bests": [{"challenge": "1ai", "rank": 1, "entrants": 1, "share": 100, "record": 120, "record_holder": "Ann"}]}`},
		{"newest entries first", "/v1/challenges/players/Ann", http.StatusOK, "recent", `[{"challenge": "epic1_exp"}, {"challenge": "1ai"}]`},
		{"no counted entry: no summary", "/v1/challenges/players/Bob", http.StatusOK, "", `{"player": "Bob", "summary": null, "bests": []}`},
		{"as if verified counts their own unproven entries", "/v1/challenges/players/Bob?as_if_verified=true", http.StatusOK, "bests",
			`[{"challenge": "1ai", "domination": 150, "rank": 1, "entrants": 2, "share": 125}]`},
		{"only bugged entries: no summary", "/v1/challenges/players/Cy", http.StatusOK, "summary", `null`},
		{"a name with no entries", "/v1/challenges/players/Nobody", http.StatusNotFound, "", `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, _ := serve(t, &fakeRatings{}, tt.path)
			if rec.Code != tt.status {
				t.Fatalf("GET %s = %d %s, want %d", tt.path, rec.Code, rec.Body, tt.status)
			}
			wantJSON(t, rec.Body.Bytes(), tt.check, tt.want)
		})
	}
}
