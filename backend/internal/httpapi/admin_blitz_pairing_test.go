package httpapi

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

func TestRoundsArePairedByTheEditionsMethods(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	tests := []struct {
		name       string
		setup      func(*fakeAdmins)
		pairing    store.BlitzPairing
		wantMethod BlitzPairingMethod
		want       []string
	}{
		// Ann (1700) and Cy (1650) are rated, bob and Dee not: starting ranks Ann, Cy, bob, Dee. The time zones
		// would pair Ann with Cy (both Berlin).
		{"round 1 by its own method: the Dutch, top half v bottom half", fourEntered, store.BlitzPairing{First: "dutch", Later: "timezone"}, "dutch", []string{"7-9", "8-10"}},
		{"round 1 by time zone", fourEntered, store.BlitzPairing{First: "timezone", Later: "dutch"}, "timezone", []string{"7-8", "9-10"}},
		// Ann and bob won round 1: the Dutch pairs the winners and the losers.
		{"later rounds by the later method", roundOneDecided, store.BlitzPairing{First: "random", Later: "dutch"}, "dutch", []string{"7-9", "8-10"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			tt.setup(f)
			f.pairing = tt.pairing
			ratings := &fakeRatings{run: store.PublishedRun{ID: 7}, rated: map[string]store.Rated{"Ann": {Rating: 1700}, "Cy": {Rating: 1650}}}
			rec := adminDoWith(t, ratings, f, org, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds", site, "")
			var round AdminBlitzRound
			if err := json.Unmarshal(rec.Body.Bytes(), &round); err != nil || rec.Code != http.StatusCreated {
				t.Fatalf("POST rounds = %d %s", rec.Code, rec.Body)
			}
			if round.Method == nil || *round.Method != tt.wantMethod {
				t.Errorf("round method = %v, want %s", round.Method, tt.wantMethod)
			}
			if got := pairsOf(round); !slices.Equal(got, tt.want) {
				t.Errorf("pairs = %v, want %v", got, tt.want)
			}
			if got := f.rounds[len(f.rounds)-1].Method; got != string(tt.wantMethod) {
				t.Errorf("stored method = %q, want %q", got, tt.wantMethod)
			}
		})
	}
}

func TestAProposalPairsByTheRoundsOwnMethod(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	roundOnePublished(f)
	f.rounds[0].Method = "dutch"
	ratings := &fakeRatings{run: store.PublishedRun{ID: 7}, rated: map[string]store.Rated{"Ann": {Rating: 1700}, "Cy": {Rating: 1650}}}
	rec := adminDoWith(t, ratings, f, org, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds/101/pairing", site, `{}`)
	var round AdminBlitzRound
	if err := json.Unmarshal(rec.Body.Bytes(), &round); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("POST pairing = %d %s", rec.Code, rec.Body)
	}
	if got, want := pairsOf(round), []string{"7-9", "8-10"}; !slices.Equal(got, want) {
		t.Errorf("pairs = %v, want %v (the Dutch, not the edition's time zones)", got, want)
	}
	if round.Method == nil || *round.Method != "dutch" {
		t.Errorf("method = %v, want dutch", round.Method)
	}
}

func TestAnOrganiserSetsHowRoundsArePaired(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	fourEntered(f)
	rec := adminDoAs(t, f, org, http.MethodPut, "/v1/admin/blitz/us_open/2026/pairing", site, `{"first": "dutch-timezone", "later": "rating"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT pairing = %d %s, want 204", rec.Code, rec.Body)
	}
	if want := "pairing dutch-timezone/rating"; strings.Join(f.notes, "|") != want {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	if len(f.audits) != 1 || f.audits[0] != org+" blitz.pairing us_open/2026" {
		t.Errorf("audits = %q, want the pairing set", f.audits)
	}
	rec = adminDoAs(t, f, org, http.MethodGet, "/v1/admin/blitz/us_open/2026", site, "")
	wantJSON(t, rec.Body.Bytes(), "pairing", `{"first": "dutch-timezone", "later": "rating"}`)
}

func TestSettingThePairingIsRefusedWhenItShouldBe(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	tests := []struct {
		name, path, origin, body string
		want                     int
	}{
		{"round 1 by rating", "/v1/admin/blitz/us_open/2026/pairing", site, `{"first": "rating", "later": "dutch"}`, http.StatusBadRequest},
		{"later rounds at random", "/v1/admin/blitz/us_open/2026/pairing", site, `{"first": "dutch", "later": "random"}`, http.StatusBadRequest},
		{"a method nobody knows", "/v1/admin/blitz/us_open/2026/pairing", site, `{"first": "swiss-ish", "later": "dutch"}`, http.StatusBadRequest},
		{"a method missing", "/v1/admin/blitz/us_open/2026/pairing", site, `{"first": "dutch"}`, http.StatusBadRequest},
		{"an unknown edition", "/v1/admin/blitz/us_open/2027/pairing", site, `{"first": "dutch", "later": "dutch"}`, http.StatusNotFound},
		{"from another site", "/v1/admin/blitz/us_open/2026/pairing", "https://evil.example", `{"first": "dutch", "later": "dutch"}`, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			fourEntered(f)
			if rec := adminDoAs(t, f, org, http.MethodPut, tt.path, tt.origin, tt.body); rec.Code != tt.want {
				t.Errorf("PUT %s %s = %d %s, want %d", tt.path, tt.body, rec.Code, rec.Body, tt.want)
			}
			if len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
}

func TestTheSiteShowsHowAnEditionIsPaired(t *testing.T) {
	bob := int64(8)
	at := time.Date(2026, 12, 21, 12, 0, 0, 0, time.UTC)
	ed := store.BlitzEdition{
		BlitzEditionRow: store.BlitzEditionRow{Slug: "us_open", Name: "US Open", Edition: "2026", Format: store.BlitzFormat{SwissRounds: 1, KnockoutSize: 2}, Points: blitz.FIDEScoring,
			Pairing: store.BlitzPairing{First: "random", Later: "dutch"}, Entrants: 2, Round: new(2)},
		Entrants: []store.BlitzEntrant{{ID: 7, Name: "Ann", Zone: "UTC", Status: "active"}, {ID: 8, Name: "bob", Zone: "UTC", Status: "active"}},
		Rounds: []store.BlitzRound{
			{ID: 101, Number: 1, Kind: "swiss", Method: "random", PublishedAt: &at, Matches: []store.BlitzMatch{{ID: 1, A: 7, B: &bob}}},
			{ID: 102, Number: 2, Kind: "knockout", PublishedAt: &at, Matches: []store.BlitzMatch{{ID: 2, A: 7, B: &bob, Slot: new(0), SeedA: new(1), SeedB: new(2)}}},
		},
	}
	rec, _ := serve(t, &fakeRatings{run: store.PublishedRun{ID: 7}, blitzEdition: &ed}, "/v1/blitz/us_open/2026")
	wantJSON(t, rec.Body.Bytes(), "pairing", `{"first": "random", "later": "dutch"}`)
	wantJSON(t, rec.Body.Bytes(), "rounds.0.method", `"random"`)
	wantJSON(t, rec.Body.Bytes(), "rounds.1.method", `null`)
}
