package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

// nextFakeRound is the round a skip is for: the draft, else the one after the latest.
func (f *fakeAdmins) nextFakeRound() int {
	for _, r := range f.rounds {
		if r.PublishedAt == nil {
			return r.Number
		}
	}
	return len(f.rounds) + 1
}

func (f *fakeAdmins) SkipBlitzRound(_ context.Context, slug, edition string, id int64) (int, error) {
	if slug != "us_open" || edition != "2026" {
		return 0, store.ErrNoBlitzEdition
	}
	for i := range f.blitzEntrants() {
		if f.entrants[i].ID != id {
			continue
		}
		next := f.nextFakeRound()
		switch {
		case f.entrants[i].Status == "dropped":
			return 0, store.ErrBlitzEntrantDropped
		case next > f.blitzRow().Format.SwissRounds:
			return 0, store.ErrBlitzSkipNotSwiss
		}
		f.entrants[i].Skips = append(f.entrants[i].Skips, next)
		f.notes = append(f.notes, fmt.Sprintf("skip entrant %d round %d", id, next))
		return next, nil
	}
	return 0, store.ErrNoBlitzEntrant
}

func (f *fakeAdmins) UnskipBlitzRound(_ context.Context, slug, edition string, id int64) error {
	if slug != "us_open" || edition != "2026" {
		return store.ErrNoBlitzEdition
	}
	for i := range f.blitzEntrants() {
		if f.entrants[i].ID != id {
			continue
		}
		next := f.nextFakeRound()
		if !slices.Contains(f.entrants[i].Skips, next) {
			return store.ErrBlitzNotSkipping
		}
		f.entrants[i].Skips = slices.DeleteFunc(f.entrants[i].Skips, func(n int) bool { return n == next })
		f.notes = append(f.notes, fmt.Sprintf("unskip entrant %d", id))
		return nil
	}
	return store.ErrNoBlitzEntrant
}

// roundOneDecided is roundOnePublished with both results in: Ann beat Cy, bob beat Dee.
func roundOneDecided(f *fakeAdmins) {
	roundOnePublished(f)
	at := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	for i := range f.rounds[0].Matches {
		f.rounds[0].Matches[i].Result, f.rounds[0].Matches[i].DecidedAt, f.rounds[0].Matches[i].Ruling = "a", &at, "ruled"
	}
}

func TestAnOrganiserSkipsAPlayersNextRoundThenUndoesIt(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	roundOneDecided(f)
	rec := adminDoAs(t, f, org, http.MethodPost, "/v1/admin/blitz/us_open/2026/entrants/10/skip", site, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST skip = %d %s, want 201", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "round", "2")
	rec = adminDoAs(t, f, org, http.MethodGet, "/v1/admin/blitz/us_open/2026", "", "")
	wantJSON(t, rec.Body.Bytes(), "entrants.3", `{"name": "Dee", "skips": [2]}`)
	wantJSON(t, rec.Body.Bytes(), "entrants.0", `{"name": "Ann", "skips": []}`)
	if rec := adminDoAs(t, f, org, http.MethodDelete, "/v1/admin/blitz/us_open/2026/entrants/10/skip", site, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE skip = %d %s, want 204", rec.Code, rec.Body)
	}
	if want := "skip entrant 10 round 2|unskip entrant 10"; strings.Join(f.notes, "|") != want {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	if want := []string{org + " blitz.entrant.skip 10", org + " blitz.entrant.unskip 10"}; !slices.Equal(f.audits, want) {
		t.Errorf("audits = %q, want %q", f.audits, want)
	}
}

// pairsOf lists a round's pairings as "a-b", the lower id first, and "bye:a", sorted.
func pairsOf(round AdminBlitzRound) []string {
	var pairs []string
	for _, m := range round.Matches {
		if m.B == nil {
			pairs = append(pairs, fmt.Sprintf("bye:%d", m.A))
		} else {
			pairs = append(pairs, fmt.Sprintf("%d-%d", min(m.A, *m.B), max(m.A, *m.B)))
		}
	}
	slices.Sort(pairs)
	return pairs
}

func TestASkippingPlayerIsLeftOutOfThePairing(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	deeSkipsRoundTwo := func(f *fakeAdmins) {
		roundOneDecided(f)
		f.entrants[3].Skips = []int{2}
	}
	deeSkipsRoundOne := func(f *fakeAdmins) {
		roundOnePublished(f)
		f.rounds[0].Matches = f.rounds[0].Matches[:1]
		f.entrants[3].Skips = []int{1}
	}
	tests := []struct {
		name, path, body string
		setup            func(*fakeAdmins)
		want             []string
	}{
		{"the next round", "/v1/admin/blitz/us_open/2026/rounds", `{"timezone": true}`, deeSkipsRoundTwo, []string{"7-9", "bye:8"}},
		{"a round paired again", "/v1/admin/blitz/us_open/2026/rounds/101/pairing", `{}`, deeSkipsRoundOne, []string{"7-8", "bye:9"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			tt.setup(f)
			rec := adminDoWith(t, &fakeRatings{run: store.PublishedRun{ID: 7}}, f, org, http.MethodPost, tt.path, site, tt.body)
			var round AdminBlitzRound
			if err := json.Unmarshal(rec.Body.Bytes(), &round); err != nil || rec.Code/100 != 2 {
				t.Fatalf("POST %s = %d %s, want a round", tt.path, rec.Code, rec.Body)
			}
			if got := pairsOf(round); !slices.Equal(got, tt.want) {
				t.Errorf("pairs = %v, want %v, Dee (10) left out", got, tt.want)
			}
		})
	}
	f := &fakeAdmins{}
	deeSkipsRoundOne(f)
	rec := adminDoWith(t, &fakeRatings{run: store.PublishedRun{ID: 7}}, f, org, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds/101/pairing", site, `{"keep": [{"a": 10, "b": 9}]}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Dee") {
		t.Errorf("keeping Dee in the round she skips = %d %s, want 400 naming her", rec.Code, rec.Body)
	}
}

func TestSkipsAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site = "https://ttac-admin.example"
	knockoutNext := func(f *fakeAdmins) {
		roundOneDecided(f)
		f.format = store.BlitzFormat{SwissRounds: 1, KnockoutSize: 2}
	}
	deeDropped := func(f *fakeAdmins) {
		roundOneDecided(f)
		f.entrants[3].Status = "dropped"
	}
	tests := []struct {
		name, method, path, origin string
		setup                      func(*fakeAdmins)
		want                       int
	}{
		{"another site", http.MethodPost, "/v1/admin/blitz/us_open/2026/entrants/10/skip", "https://evil.example", roundOneDecided, http.StatusForbidden},
		{"a bad slug", http.MethodPost, "/v1/admin/blitz/Bad!/2026/entrants/10/skip", site, roundOneDecided, http.StatusNotFound},
		{"an unknown edition", http.MethodPost, "/v1/admin/blitz/us_open/2027/entrants/10/skip", site, roundOneDecided, http.StatusNotFound},
		{"an unknown entrant", http.MethodPost, "/v1/admin/blitz/us_open/2026/entrants/99/skip", site, roundOneDecided, http.StatusNotFound},
		{"a dropped entrant", http.MethodPost, "/v1/admin/blitz/us_open/2026/entrants/10/skip", site, deeDropped, http.StatusConflict},
		{"the knockout next", http.MethodPost, "/v1/admin/blitz/us_open/2026/entrants/10/skip", site, knockoutNext, http.StatusConflict},
		{"undoing no skip", http.MethodDelete, "/v1/admin/blitz/us_open/2026/entrants/10/skip", site, roundOneDecided, http.StatusConflict},
		{"undoing an unknown entrant's skip", http.MethodDelete, "/v1/admin/blitz/us_open/2026/entrants/99/skip", site, roundOneDecided, http.StatusNotFound},
		{"undoing with a bad slug", http.MethodDelete, "/v1/admin/blitz/Bad!/2026/entrants/10/skip", site, roundOneDecided, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			tt.setup(f)
			rec := adminDoAs(t, f, "111111111111111111", tt.method, tt.path, tt.origin, "")
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused skip still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
}

func TestDroppingAPlayerSkippingTheNextRoundIsRefused(t *testing.T) {
	f := &fakeAdmins{}
	roundOneDecided(f)
	f.entrants[0].Skips = []int{2}
	rec := adminDoAs(t, f, "111111111111111111", http.MethodPatch, "/v1/admin/blitz/us_open/2026/entrants/7", "https://ttac-admin.example", `{"status": "dropped"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "reinstate") {
		t.Errorf("dropping Ann while she skips round 2 = %d %s, want 409 saying to reinstate her first", rec.Code, rec.Body)
	}
}
