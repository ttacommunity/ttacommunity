package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

func (f *fakeAdmins) findMatch(slug, edition string, id int64) (*store.BlitzMatch, error) {
	if slug != "us_open" || edition != "2026" {
		return nil, store.ErrNoBlitzEdition
	}
	for i := range f.rounds {
		for j := range f.rounds[i].Matches {
			if f.rounds[i].Matches[j].ID == id {
				return &f.rounds[i].Matches[j], nil
			}
		}
	}
	return nil, store.ErrNoBlitzMatch
}

func (f *fakeAdmins) RecordBlitzGame(_ context.Context, slug, edition string, id int64, g store.BlitzGame) error {
	m, err := f.findMatch(slug, edition, id)
	if err != nil {
		return err
	}
	if m.B == nil {
		return store.ErrBlitzBye
	}
	if g.ShareCode == "TAKEN" {
		return &store.BlitzShareCodeError{GameID: 55}
	}
	for _, o := range []string{g.A.Outcome, g.B.Outcome} {
		if o != "finished" && o != "resigned" && o != "timed_out" {
			return store.ErrBadBlitzGame
		}
	}
	if g.A.Outcome != "finished" && g.B.Outcome != "finished" {
		return store.ErrBadBlitzGame
	}
	result := "draw"
	if g.A.Culture > g.B.Culture || g.B.Outcome != "finished" {
		result = "a"
	} else if g.B.Culture > g.A.Culture || g.A.Outcome != "finished" {
		result = "b"
	}
	at := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	gameID := int64(500 + id)
	m.Result, m.GameID, m.Game, m.Ruling, m.DecidedAt = result, &gameID, &g, "", &at
	f.notes = append(f.notes, fmt.Sprintf("game on %d: %s %g/%s %g/%s", id, g.ShareCode, g.A.Culture, g.A.Outcome, g.B.Culture, g.B.Outcome))
	return nil
}

func (f *fakeAdmins) RecordBlitzRuling(_ context.Context, slug, edition string, id int64, result, reason string) error {
	if strings.TrimSpace(reason) == "" || result == "c" {
		return store.ErrBadBlitzRuling
	}
	m, err := f.findMatch(slug, edition, id)
	if err != nil {
		return err
	}
	if m.B == nil {
		return store.ErrBlitzBye
	}
	for _, r := range f.rounds {
		if r.Kind == "knockout" && slices.ContainsFunc(r.Matches, func(x store.BlitzMatch) bool { return x.ID == id }) && result != "a" && result != "b" {
			return store.ErrBlitzKnockoutWinner
		}
	}
	at := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	m.Result, m.Ruling, m.DecidedAt = result, reason, &at
	f.notes = append(f.notes, fmt.Sprintf("ruling on %d: %s %s", id, result, reason))
	return nil
}

// roundOnePublished gives the fake four entrants and round 1 published: Ann (7) v Cy (8), bob (9) v Dee (10).
func roundOnePublished(f *fakeAdmins) {
	fourEntered(f)
	cy, dee := int64(8), int64(10)
	at := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	f.rounds = []store.BlitzRound{{ID: 101, Number: 1, Kind: "swiss", Seed: 1, PublishedAt: &at,
		Matches: []store.BlitzMatch{{ID: 1010, A: 7, B: &cy}, {ID: 1011, A: 9, B: &dee}}}}
}

func TestAnOrganiserRecordsGamesAndRulings(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	roundOnePublished(f)
	f.format = store.BlitzFormat{SwissRounds: 8, KnockoutSize: 8}
	ratings := &fakeRatings{run: store.PublishedRun{ID: 7}}
	do := func(method, path, body string) *httptest.ResponseRecorder {
		return adminDoWith(t, ratings, f, org, method, path, site, body)
	}

	rec := do(http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/matches/1010/game",
		`{"share_code": "ABCD1234", "a": {"culture": 250, "outcome": "finished"}, "b": {"culture": 200, "outcome": "resigned"}, "finished_on": "2026-10-14", "finished_time": "21:30", "zone": "Europe Central"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT game = %d %s, want 204", rec.Code, rec.Body)
	}
	rec = do(http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/matches/1011/ruling", `{"result": "draw", "reason": "Neither could find a time"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT ruling = %d %s, want 204", rec.Code, rec.Body)
	}
	if want := "game on 1010: ABCD1234 250/finished 200/resigned|ruling on 1011: draw Neither could find a time"; strings.Join(f.notes, "|") != want {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	for i, w := range []string{"blitz.match.game 1010", "blitz.match.ruling 1011"} {
		if i >= len(f.audits) || f.audits[i] != org+" "+w {
			t.Errorf("audit %d = %q, want %q", i, f.audits, org+" "+w)
		}
	}

	rec = do(http.MethodGet, "/v1/admin/blitz/us_open/2026", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET edition = %d %s", rec.Code, rec.Body)
	}
	body := rec.Body.Bytes()
	wantJSON(t, body, "rounds.0.matches.0.result", `"a"`)
	wantJSON(t, body, "rounds.0.matches.0.game", `{"share_code": "ABCD1234", "a_culture": 250, "a_outcome": "finished", "b_culture": 200, "b_outcome": "resigned", "finished_at": "2026-10-14T19:30:00Z"}`)
	wantJSON(t, body, "rounds.0.matches.0.ruling", `""`)
	wantJSON(t, body, "rounds.0.matches.0.decided_at", `"2026-10-15T09:00:00Z"`)
	wantJSON(t, body, "rounds.0.matches.1.result", `"draw"`)
	wantJSON(t, body, "rounds.0.matches.1.game", `null`)
	wantJSON(t, body, "rounds.0.matches.1.ruling", `"Neither could find a time"`)
	var ed AdminBlitzEdition
	if err := json.Unmarshal(body, &ed); err != nil {
		t.Fatal(err)
	}
	var standings []string
	for _, s := range ed.Standings {
		standings = append(standings, fmt.Sprintf("%d:%s %g %d %d %g %s", s.Rank, s.Name, s.Points, s.Games, s.Byes, s.Buchholz, s.Status))
	}
	// Ann won; bob and Dee drew by ruling, no game played, and are tied on every count but the draw from the seed; Cy lost.
	if len(standings) != 4 || standings[0] != "1:Ann 1 1 0 0 active" || standings[3] != "4:Cy 0 1 0 1 active" ||
		!strings.HasSuffix(standings[1], " 0.5 0 0 0.5 active") || !strings.HasSuffix(standings[2], " 0.5 0 0 0.5 active") {
		t.Errorf("standings = %q, want Ann first, bob and Dee on a half, Cy last", standings)
	}
}

func TestBlitzResultsAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	game := `{"share_code": "ABCD1234", "a": {"culture": 250, "outcome": "finished"}, "b": {"culture": 200, "outcome": "finished"}, "finished_on": "2026-10-14", "finished_time": "21:30", "zone": "Europe Central"}`
	withBye := func(f *fakeAdmins) {
		roundOnePublished(f)
		f.rounds[0].Matches = append(f.rounds[0].Matches, store.BlitzMatch{ID: 1012, A: 7})
	}
	tests := []struct {
		name                       string
		setup                      func(*fakeAdmins)
		method, path, origin, body string
		want                       int
		wantSaid                   string
	}{
		{"a game from another site", roundOnePublished, http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/matches/1010/game", "https://evil.example", game, http.StatusForbidden, ""},
		{"a game on an unknown edition", roundOnePublished, http.MethodPut, "/v1/admin/blitz/us_open/2027/rounds/101/matches/1010/game", site, game, http.StatusNotFound, ""},
		{"a game on an unknown match", roundOnePublished, http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/matches/99/game", site, game, http.StatusNotFound, ""},
		{"a game on a bye", withBye, http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/matches/1012/game", site, game, http.StatusBadRequest, "bye"},
		{"a share code on another game", roundOnePublished, http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/matches/1010/game", site,
			`{"share_code": "TAKEN", "a": {"culture": 250, "outcome": "finished"}, "b": {"culture": 200, "outcome": "finished"}, "finished_on": "2026-10-14", "finished_time": "21:30", "zone": "Europe Central"}`, http.StatusConflict, "55"},
		{"a game nobody finished", roundOnePublished, http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/matches/1010/game", site,
			`{"share_code": "ABCD1234", "a": {"culture": 0, "outcome": "resigned"}, "b": {"culture": 0, "outcome": "timed_out"}, "finished_on": "2026-10-14", "finished_time": "21:30", "zone": "Europe Central"}`, http.StatusBadRequest, ""},
		{"an outcome that is not finished, resigned or timed_out", roundOnePublished, http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/matches/1010/game", site,
			`{"share_code": "ABCD1234", "a": {"culture": 250, "outcome": "won"}, "b": {"culture": 200, "outcome": "finished"}, "finished_on": "2026-10-14", "finished_time": "21:30", "zone": "Europe Central"}`, http.StatusBadRequest, "outcome"},
		{"a game finishing in a zone that is not one", roundOnePublished, http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/matches/1010/game", site,
			`{"share_code": "ABCD1234", "a": {"culture": 250, "outcome": "finished"}, "b": {"culture": 200, "outcome": "finished"}, "finished_on": "2026-10-14", "finished_time": "21:30", "zone": "Atlantis"}`, http.StatusBadRequest, "Atlantis"},
		{"a game finishing at a time that is not one", roundOnePublished, http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/matches/1010/game", site,
			`{"share_code": "ABCD1234", "a": {"culture": 250, "outcome": "finished"}, "b": {"culture": 200, "outcome": "finished"}, "finished_on": "2026-10-14", "finished_time": "9pm", "zone": "UTC"}`, http.StatusBadRequest, "9pm"},
		{"a game with no body", roundOnePublished, http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/matches/1010/game", site, "", http.StatusBadRequest, ""},
		{"a ruling with no reason", roundOnePublished, http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/matches/1010/ruling", site, `{"result": "a", "reason": " "}`, http.StatusBadRequest, ""},
		{"a ruling that is not a, b, draw or double_loss", roundOnePublished, http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/matches/1010/ruling", site, `{"result": "c", "reason": "x"}`, http.StatusBadRequest, ""},
		{"a ruling on a bye", withBye, http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/matches/1012/ruling", site, `{"result": "a", "reason": "x"}`, http.StatusBadRequest, "bye"},
		{"a ruling on an unknown match", roundOnePublished, http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/matches/99/ruling", site, `{"result": "a", "reason": "x"}`, http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			tt.setup(f)
			rec := adminDoAs(t, f, owner, tt.method, tt.path, tt.origin, tt.body)
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if tt.wantSaid != "" && !strings.Contains(rec.Body.String(), tt.wantSaid) {
				t.Errorf("%s %s said %s, want it to mention %q", tt.method, tt.path, rec.Body, tt.wantSaid)
			}
			if len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused result still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
}
