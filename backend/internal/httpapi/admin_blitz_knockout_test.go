package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

// seedText shows a seed for a failure message, or "none".
func seedText(seed *int) string {
	if seed == nil {
		return "none"
	}
	return fmt.Sprint(*seed)
}

// swissDone gives the fake a one-round Swiss with a knockout of four, its round published and decided: Ann (7)
// beat Cy (8) and bob (9) beat Dee (10).
func swissDone(f *fakeAdmins) {
	roundOnePublished(f)
	f.format = store.BlitzFormat{SwissRounds: 1, KnockoutSize: 4}
	at := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	for i := range f.rounds[0].Matches {
		f.rounds[0].Matches[i].Result, f.rounds[0].Matches[i].DecidedAt, f.rounds[0].Matches[i].Ruling = "a", &at, "ruled"
	}
}

func TestTheStandingsCountTheSwissRoundsOnly(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	swissDone(f)
	at := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	cy, dee := int64(8), int64(10)
	f.rounds = append(f.rounds, store.BlitzRound{ID: 102, Number: 2, Kind: "knockout", Seed: 2, PublishedAt: &at, Matches: []store.BlitzMatch{
		{ID: 1020, A: 7, B: &dee, Result: "b", Ruling: "ruled", DecidedAt: &at}, {ID: 1021, A: 9, B: &cy, Result: "b", Ruling: "ruled", DecidedAt: &at},
	}})
	rec := adminDoWith(t, &fakeRatings{run: store.PublishedRun{ID: 7}}, f, org, http.MethodGet, "/v1/admin/blitz/us_open/2026", site, "")
	var ed AdminBlitzEdition
	if err := json.Unmarshal(rec.Body.Bytes(), &ed); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET edition = %d %s", rec.Code, rec.Body)
	}
	points := map[string]float64{}
	for _, s := range ed.Standings {
		points[s.Name] = s.Points
	}
	if points["Ann"] != 1 || points["Dee"] != 0 {
		t.Errorf("points = %v, want Ann 1 and Dee 0: a knockout result does not count", points)
	}
}

func TestTheKnockoutIsPairedFromTheStandingsThenFromTheWinners(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	swissDone(f)
	ratings := &fakeRatings{run: store.PublishedRun{ID: 7}}
	do := func(method, path, body string) *httptest.ResponseRecorder {
		return adminDoWith(t, ratings, f, org, method, path, site, body)
	}
	pair := func(want int) AdminBlitzRound {
		t.Helper()
		rec := do(http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds", `{}`)
		var round AdminBlitzRound
		if err := json.Unmarshal(rec.Body.Bytes(), &round); err != nil || rec.Code != http.StatusCreated {
			t.Fatalf("POST rounds = %d %s, want 201", rec.Code, rec.Body)
		}
		if round.Kind != "knockout" || len(round.Matches) != want {
			t.Fatalf("round = %+v, want a knockout round of %d matches", round, want)
		}
		if rec := do(http.MethodPost, fmt.Sprintf("/v1/admin/blitz/us_open/2026/rounds/%d/publish", round.Id), ""); rec.Code != http.StatusNoContent {
			t.Fatalf("publish = %d %s", rec.Code, rec.Body)
		}
		return round
	}
	decide := func(id int64, result string) {
		t.Helper()
		if rec := do(http.MethodPut, fmt.Sprintf("/v1/admin/blitz/us_open/2026/rounds/1/matches/%d/ruling", id), fmt.Sprintf(`{"result": %q, "reason": "ruled"}`, result)); rec.Code != http.StatusNoContent {
			t.Fatalf("ruling on %d = %d %s", id, rec.Code, rec.Body)
		}
	}

	semis := pair(2)
	for i, m := range semis.Matches {
		if m.Slot == nil || *m.Slot != i || m.B == nil || m.ASeed == nil || m.BSeed == nil {
			t.Fatalf("semi-final %d = %+v, want slot %d with both seeds", i, m, i)
		}
		if *m.ASeed+*m.BSeed != 5 || *m.ASeed > 2 || (m.A != 7 && m.A != 9) {
			t.Errorf("semi-final %d = seeds %d v %d (%d v %d), want a Swiss winner seeded 1 or 2 against 4 or 3", i, *m.ASeed, *m.BSeed, m.A, *m.B)
		}
	}
	decide(semis.Matches[0].Id, "b")
	decide(semis.Matches[1].Id, "a")

	final := pair(1)
	if m := final.Matches[0]; m.A != *semis.Matches[0].B || m.B == nil || *m.B != semis.Matches[1].A || m.Slot == nil || *m.Slot != 0 {
		t.Errorf("final = %+v, want slot 0's winner (%d) against slot 1's (%d)", m, *semis.Matches[0].B, semis.Matches[1].A)
	}
	if m := final.Matches[0]; m.ASeed == nil || *m.ASeed != *semis.Matches[0].BSeed || m.BSeed == nil || *m.BSeed != *semis.Matches[1].ASeed {
		t.Errorf("final's seeds = %s v %s, want the winners' seeds %d v %d", seedText(m.ASeed), seedText(m.BSeed), *semis.Matches[0].BSeed, *semis.Matches[1].ASeed)
	}
	decide(final.Matches[0].Id, "a")

	rec := do(http.MethodGet, "/v1/admin/blitz/us_open/2026", "")
	var ed AdminBlitzEdition
	if err := json.Unmarshal(rec.Body.Bytes(), &ed); err != nil || ed.Champion == nil {
		t.Fatalf("GET edition = %d %s, want a champion", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds", `{}`); rec.Code != http.StatusConflict {
		t.Errorf("a round after the final = %d %s, want 409", rec.Code, rec.Body)
	}
}

// semisPublished gives the fake swissDone's edition with its semi-finals published: Ann (7, seed 1) v Dee (10,
// seed 4) in slot 0 and bob (9, seed 2) v Cy (8, seed 3) in slot 1, with the results given.
func semisPublished(first, second string) func(*fakeAdmins) {
	return func(f *fakeAdmins) {
		swissDone(f)
		at := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
		cy, dee := int64(8), int64(10)
		semis := store.BlitzRound{ID: 102, Number: 2, Kind: "knockout", Seed: 2, PublishedAt: &at,
			Matches: []store.BlitzMatch{{ID: 1020, A: 7, B: &dee, Slot: new(0), SeedA: new(1), SeedB: new(4)}, {ID: 1021, A: 9, B: &cy, Slot: new(1), SeedA: new(2), SeedB: new(3)}}}
		for i, r := range []string{first, second} {
			if r != "" {
				semis.Matches[i].Result, semis.Matches[i].Ruling, semis.Matches[i].DecidedAt = r, "ruled", &at
			}
		}
		f.rounds = append(f.rounds, semis)
	}
}

func TestKnockoutChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	shortOfPlayers := func(f *fakeAdmins) {
		swissDone(f)
		f.entrants[2].Status, f.entrants[3].Status = "dropped", "dropped"
	}
	tests := []struct {
		name         string
		setup        func(*fakeAdmins)
		method, path string
		body         string
		want         int
	}{
		{"a knockout short of players", shortOfPlayers, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds", `{}`, http.StatusConflict},
		{"the next round while a knockout match is drawn", semisPublished("a", "draw"), http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds", `{}`, http.StatusConflict},
		{"a fresh pairing of a knockout round", semisPublished("", ""), http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds/102/pairing", `{}`, http.StatusConflict},
		{"changing a knockout round by hand", semisPublished("", ""), http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/102/matches", `{"matches": [{"a": 7, "b": 9}, {"a": 8, "b": 10}]}`, http.StatusConflict},
		{"a knockout ruling with no winner", semisPublished("", ""), http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/102/matches/1020/ruling", `{"result": "draw", "reason": "agreed"}`, http.StatusBadRequest},
		{"the format once the knockout started", semisPublished("", ""), http.MethodPatch, "/v1/admin/blitz/us_open/2026", `{"swiss_rounds": 2, "knockout_size": 4}`, http.StatusConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			tt.setup(f)
			rec := adminDoWith(t, &fakeRatings{run: store.PublishedRun{ID: 7}}, f, org, tt.method, tt.path, site, tt.body)
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
}

func TestAKnockoutPairingIsNeverFlaggedAsARepeat(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	swissDone(f)
	at := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	cy, dee := int64(8), int64(10)
	// Ann v Cy and bob v Dee meet again in the semi-finals, as they did in the Swiss round.
	f.rounds = append(f.rounds, store.BlitzRound{ID: 102, Number: 2, Kind: "knockout", Seed: 2, PublishedAt: &at,
		Matches: []store.BlitzMatch{{ID: 1020, A: 7, B: &cy, Slot: new(0)}, {ID: 1021, A: 9, B: &dee, Slot: new(1)}}})
	rec := adminDoWith(t, &fakeRatings{run: store.PublishedRun{ID: 7}}, f, org, http.MethodGet, "/v1/admin/blitz/us_open/2026", site, "")
	var ed AdminBlitzEdition
	if err := json.Unmarshal(rec.Body.Bytes(), &ed); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET edition = %d %s", rec.Code, rec.Body)
	}
	for _, r := range ed.Rounds {
		for _, m := range r.Matches {
			if m.Repeat {
				t.Errorf("round %d's match %d-%d is flagged as a repeat, want none: the knockout may repeat a Swiss pairing", r.Number, m.A, *m.B)
			}
		}
	}
}

func TestTheKnockoutsSeedsAreFixedWhenItBegins(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	semisPublished("", "")(f)
	f.rounds[0].Matches[0].Result = "b" // a Swiss result corrected after the knockout began: Cy beat Ann after all
	rec := adminDoWith(t, &fakeRatings{run: store.PublishedRun{ID: 7}}, f, org, http.MethodGet, "/v1/admin/blitz/us_open/2026", site, "")
	var ed AdminBlitzEdition
	if err := json.Unmarshal(rec.Body.Bytes(), &ed); err != nil || rec.Code != http.StatusOK || len(ed.Rounds) != 2 {
		t.Fatalf("GET edition = %d %s", rec.Code, rec.Body)
	}
	if m := ed.Rounds[1].Matches[0]; m.ASeed == nil || *m.ASeed != 1 || m.BSeed == nil || *m.BSeed != 4 {
		t.Errorf("Ann v Dee's seeds = %s v %s, want 1 v 4 as the knockout began", seedText(m.ASeed), seedText(m.BSeed))
	}
}

// finalPaired gives the fake a one-round Swiss with a knockout of two: Ann (7) drew with Cy (8), bob (9) beat Dee
// (10), so the Swiss places are bob, Ann, Cy, Dee; the final, bob v Ann, is published and not yet played.
func finalPaired(f *fakeAdmins) {
	roundOnePublished(f)
	f.format = store.BlitzFormat{SwissRounds: 1, KnockoutSize: 2}
	at := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	for i, r := range []string{"draw", "a"} {
		f.rounds[0].Matches[i].Result, f.rounds[0].Matches[i].DecidedAt, f.rounds[0].Matches[i].Ruling = r, &at, "ruled"
	}
	ann := int64(7)
	f.rounds = append(f.rounds, store.BlitzRound{ID: 102, Number: 2, Kind: "knockout", Seed: 2, PublishedAt: &at,
		Matches: []store.BlitzMatch{{ID: 1020, A: 9, B: &ann, Slot: new(0), SeedA: new(1), SeedB: new(2)}}})
}

// wildcardInFinal is finalPaired with Eve (11), entered after the Swiss round, in bob's place in the final.
func wildcardInFinal(f *fakeAdmins) {
	finalPaired(f)
	f.entrants = append(f.entrants, store.BlitzEntrant{ID: 11, MemberID: 1011, Name: "Eve", Zone: "UTC", Status: "active"})
	f.rounds[1].Matches[0].A = 11
}

func TestAKnockoutRoundNamesTheBestSwissSeedNotInIt(t *testing.T) {
	cy, dee := int64(8), int64(10)
	tests := []struct {
		name    string
		dropped []int
		want    *int64
	}{
		{"the best Swiss place outside the final", nil, &cy},
		{"past one who dropped", []int{1}, &dee},
		{"nobody when everyone outside dropped", []int{1, 3}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			finalPaired(f)
			for _, i := range tt.dropped {
				f.entrants[i].Status = "dropped"
			}
			rec := adminDoWith(t, &fakeRatings{run: store.PublishedRun{ID: 7}}, f, "222222222222222222", http.MethodGet, "/v1/admin/blitz/us_open/2026", "", "")
			var ed AdminBlitzEdition
			if err := json.Unmarshal(rec.Body.Bytes(), &ed); err != nil || rec.Code != http.StatusOK {
				t.Fatalf("GET edition = %d %s", rec.Code, rec.Body)
			}
			for _, r := range ed.Rounds {
				got := r.Reserve
				switch {
				case r.Kind == "swiss" && got != nil:
					t.Errorf("round 1, Swiss: reserve %d, want none", *got)
				case r.Kind == "knockout" && (got == nil) != (tt.want == nil), r.Kind == "knockout" && got != nil && *got != *tt.want:
					t.Errorf("the final's reserve = %s, want %s", seedText64(got), seedText64(tt.want))
				}
			}
		})
	}
}

// seedText64 shows an entrant id for a failure message, or "none".
func seedText64(id *int64) string {
	if id == nil {
		return "none"
	}
	return fmt.Sprint(*id)
}

// ReplaceBlitzPlayer swaps out for in in a match of the fake's latest knockout round, refusing as the store does
// for the cases the tests use: a Swiss round, an active player out, and someone in the round in.
func (f *fakeAdmins) ReplaceBlitzPlayer(_ context.Context, slug, edition string, roundID, matchID, out, in int64) error {
	if slug != "us_open" || edition != "2026" {
		return store.ErrNoBlitzEdition
	}
	for ri, r := range f.rounds {
		if r.ID != roundID {
			continue
		}
		if r.Kind != "knockout" {
			return store.ErrBlitzNotKnockout
		}
		for mi, m := range r.Matches {
			if m.ID != matchID {
				continue
			}
			for _, e := range f.blitzEntrants() {
				if e.ID == out && e.Status != "dropped" {
					return store.ErrBlitzNotDropped
				}
			}
			for _, x := range r.Matches {
				if x.A == in || (x.B != nil && *x.B == in) {
					return fmt.Errorf("%w: entrant %d", store.ErrBadBlitzMatch, in)
				}
			}
			if m.A == out {
				f.rounds[ri].Matches[mi].A = in
			} else {
				f.rounds[ri].Matches[mi].B = new(in)
			}
			f.notes = append(f.notes, fmt.Sprintf("replace %d: %d for %d", matchID, in, out))
			return nil
		}
		return store.ErrNoBlitzMatch
	}
	return store.ErrNoBlitzRound
}

func TestAKnockoutPlayerWhoDroppedIsReplaced(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	path := "/v1/admin/blitz/us_open/2026/rounds/102/matches/1020/replacement"
	tests := []struct {
		name, path, body string
		want             int
		wantSaid         string
	}{
		{"Ann, who dropped, by Cy", path, `{"out": 7, "in": 8}`, http.StatusNoContent, ""},
		{"a player who has not dropped", path, `{"out": 9, "in": 8}`, http.StatusConflict, "drop them first"},
		{"with someone in the round", path, `{"out": 7, "in": 9}`, http.StatusBadRequest, ""},
		{"in a Swiss round", "/v1/admin/blitz/us_open/2026/rounds/101/matches/1010/replacement", `{"out": 7, "in": 10}`, http.StatusConflict, "Swiss"},
		{"in an unknown match", "/v1/admin/blitz/us_open/2026/rounds/102/matches/99/replacement", `{"out": 7, "in": 8}`, http.StatusNotFound, ""},
		{"with no one named", path, `{"out": 7}`, http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			finalPaired(f)
			f.entrants[0].Status = "dropped"
			rec := adminDoAs(t, f, org, http.MethodPut, tt.path, site, tt.body)
			if rec.Code != tt.want || !strings.Contains(rec.Body.String(), tt.wantSaid) {
				t.Errorf("PUT %s %s = %d %s, want %d naming %q", tt.path, tt.body, rec.Code, rec.Body, tt.want, tt.wantSaid)
			}
			wantNotes, wantAudits := "", 0
			if tt.want == http.StatusNoContent {
				wantNotes, wantAudits = "replace 1020: 8 for 7", 1
			}
			if strings.Join(f.notes, "|") != wantNotes || len(f.audits) != wantAudits {
				t.Errorf("changes = %q, audits %q; want %q and %d", f.notes, f.audits, wantNotes, wantAudits)
			}
		})
	}
}
