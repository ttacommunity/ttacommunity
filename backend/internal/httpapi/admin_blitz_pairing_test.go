package httpapi

import (
	"encoding/json"
	"fmt"
	"maps"
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
	rec := adminDoAs(t, f, org, http.MethodPut, "/v1/admin/blitz/us_open/2026/pairing", site, `{"first": "random", "later": "random", "ranks": "lot"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT pairing = %d %s, want 204", rec.Code, rec.Body)
	}
	if want := "pairing random/random/lot"; strings.Join(f.notes, "|") != want {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	if len(f.audits) != 1 || f.audits[0] != org+" blitz.pairing us_open/2026" {
		t.Errorf("audits = %q, want the pairing set", f.audits)
	}
	rec = adminDoAs(t, f, org, http.MethodGet, "/v1/admin/blitz/us_open/2026", site, "")
	wantJSON(t, rec.Body.Bytes(), "pairing", `{"first": "random", "later": "random", "ranks": "lot"}`)
}

func TestSettingThePairingIsRefusedWhenItShouldBe(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	tests := []struct {
		name, path, origin, body string
		want                     int
	}{
		{"round 1 by rating", "/v1/admin/blitz/us_open/2026/pairing", site, `{"first": "rating", "later": "dutch", "ranks": "rating"}`, http.StatusBadRequest},
		{"a method nobody knows", "/v1/admin/blitz/us_open/2026/pairing", site, `{"first": "swiss-ish", "later": "dutch", "ranks": "rating"}`, http.StatusBadRequest},
		{"a method missing", "/v1/admin/blitz/us_open/2026/pairing", site, `{"first": "dutch", "ranks": "rating"}`, http.StatusBadRequest},
		{"starting ranks by something else", "/v1/admin/blitz/us_open/2026/pairing", site, `{"first": "dutch", "later": "dutch", "ranks": "random"}`, http.StatusBadRequest},
		{"the rank order missing", "/v1/admin/blitz/us_open/2026/pairing", site, `{"first": "dutch", "later": "dutch"}`, http.StatusBadRequest},
		{"an unknown edition", "/v1/admin/blitz/us_open/2027/pairing", site, `{"first": "dutch", "later": "dutch", "ranks": "rating"}`, http.StatusNotFound},
		{"from another site", "/v1/admin/blitz/us_open/2026/pairing", "https://evil.example", `{"first": "dutch", "later": "dutch", "ranks": "rating"}`, http.StatusForbidden},
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
			Pairing: store.BlitzPairing{First: "random", Later: "dutch", Ranks: "lot"}, Entrants: 2, Round: new(2)},
		Entrants: []store.BlitzEntrant{{ID: 7, Name: "Ann", Zone: "UTC", Status: "active"}, {ID: 8, Name: "bob", Zone: "UTC", Status: "active"}},
		Rounds: []store.BlitzRound{
			{ID: 101, Number: 1, Kind: "swiss", Method: "random", PublishedAt: &at, Matches: []store.BlitzMatch{{ID: 1, A: 7, B: &bob}}},
			{ID: 102, Number: 2, Kind: "knockout", PublishedAt: &at, Matches: []store.BlitzMatch{{ID: 2, A: 7, B: &bob, Slot: new(0), SeedA: new(1), SeedB: new(2)}}},
		},
	}
	rec, _ := serve(t, &fakeRatings{run: store.PublishedRun{ID: 7}, blitzEdition: &ed}, "/v1/blitz/us_open/2026")
	wantJSON(t, rec.Body.Bytes(), "pairing", `{"first": "random", "later": "dutch", "ranks": "lot"}`)
	wantJSON(t, rec.Body.Bytes(), "rounds.0.method", `"random"`)
	wantJSON(t, rec.Body.Bytes(), "rounds.1.method", `null`)
}

func TestStartingRanksAreGivenOnceAndLateEntrantsSlottedIn(t *testing.T) {
	entrant := func(id int64, name string, rank int, status string) store.BlitzEntrant {
		return store.BlitzEntrant{ID: id, Name: name, Player: name, Status: status, StartingRank: rank}
	}
	rated := map[string]store.Rated{"Ann": {Rating: 1700}, "Cy": {Rating: 1650}, "Dee": {Rating: 1660}}
	tests := []struct {
		name     string
		entrants []store.BlitzEntrant
		lot      bool
		want     map[int64]int
		changed  bool
	}{
		{"by rating from the start: rated first, highest first, the unrated by name",
			[]store.BlitzEntrant{entrant(1, "bob", 0, "active"), entrant(2, "Ann", 0, "active"), entrant(3, "Cy", 0, "active"), entrant(4, "al", 0, "active")},
			false, map[int64]int{2: 1, 3: 2, 4: 3, 1: 4}, true},
		{"ranks once given are kept, whatever the ratings now",
			[]store.BlitzEntrant{entrant(1, "bob", 1, "active"), entrant(2, "Ann", 3, "active"), entrant(3, "Cy", 2, "active")},
			false, map[int64]int{1: 1, 2: 3, 3: 2}, false},
		{"a late entrant is slotted in by rating, the ranks below renumbered",
			[]store.BlitzEntrant{entrant(2, "Ann", 1, "active"), entrant(3, "Cy", 2, "active"), entrant(1, "bob", 3, "active"), entrant(5, "Dee", 0, "active")},
			false, map[int64]int{2: 1, 5: 2, 3: 3, 1: 4}, true},
		{"a late unrated entrant goes after the rated, by name among the unrated",
			[]store.BlitzEntrant{entrant(2, "Ann", 1, "active"), entrant(3, "Cy", 2, "active"), entrant(1, "bob", 3, "active"), entrant(4, "al", 0, "active")},
			false, map[int64]int{2: 1, 3: 2, 4: 3, 1: 4}, true},
		{"by lot, a late entrant takes the next rank",
			[]store.BlitzEntrant{entrant(2, "Ann", 2, "active"), entrant(3, "Cy", 1, "active"), entrant(1, "bob", 3, "active"), entrant(5, "Dee", 0, "active")},
			true, map[int64]int{3: 1, 2: 2, 1: 3, 5: 4}, true},
		{"a dropped entrant keeps its rank; one dropped before round 1 gets none",
			[]store.BlitzEntrant{entrant(2, "Ann", 1, "dropped"), entrant(3, "Cy", 2, "active"), entrant(1, "bob", 0, "dropped"), entrant(5, "Dee", 0, "active")},
			false, map[int64]int{2: 1, 5: 2, 3: 3}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := startingRanks(tt.entrants, rated, tt.lot, 1)
			if !maps.Equal(got, tt.want) {
				t.Errorf("starting ranks = %v, want %v", got, tt.want)
			}
			if changed != tt.changed {
				t.Errorf("changed = %v, want %v", changed, tt.changed)
			}
		})
	}
}

func TestStartingRanksByLotAreADrawTheSeedFixes(t *testing.T) {
	var entrants []store.BlitzEntrant
	for i := range int64(8) {
		entrants = append(entrants, store.BlitzEntrant{ID: i + 1, Name: fmt.Sprintf("p%d", i+1), Player: fmt.Sprintf("p%d", i+1), Status: "active"})
	}
	rated := map[string]store.Rated{"p1": {Rating: 1800}, "p2": {Rating: 1700}, "p3": {Rating: 1600}}
	byRating, _ := startingRanks(entrants, rated, false, 1)
	first, _ := startingRanks(entrants, rated, true, 1)
	again, _ := startingRanks(entrants, rated, true, 1)
	other, _ := startingRanks(entrants, rated, true, 2)
	if !maps.Equal(first, again) {
		t.Errorf("seed 1 twice gave %v then %v", first, again)
	}
	if maps.Equal(first, byRating) {
		t.Errorf("the lot with seed 1 gave the rating order %v", first)
	}
	if maps.Equal(first, other) {
		t.Errorf("seeds 1 and 2 gave the same lot %v", first)
	}
	ranks := slices.Sorted(maps.Values(first))
	if !slices.Equal(ranks, []int{1, 2, 3, 4, 5, 6, 7, 8}) {
		t.Errorf("the lot's ranks = %v, want 1 to 8 once each", ranks)
	}
}

func TestRoundOneFixesTheStartingRanksByTheEditionsOrder(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	tests := []struct {
		name  string
		ranks string
		want  map[int64]int
	}{
		{"by rating: Ann 1700, Cy 1650, then bob and Dee by name", "rating", map[int64]int{7: 1, 8: 2, 9: 3, 10: 4}},
		{"by lot: a draw", "lot", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			fourEntered(f)
			f.pairing = store.BlitzPairing{First: "dutch", Later: "dutch", Ranks: tt.ranks}
			ratings := &fakeRatings{run: store.PublishedRun{ID: 7}, rated: map[string]store.Rated{"Ann": {Rating: 1700}, "Cy": {Rating: 1650}}}
			rec := adminDoWith(t, ratings, f, org, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds", site, "")
			var round AdminBlitzRound
			if err := json.Unmarshal(rec.Body.Bytes(), &round); err != nil || rec.Code != http.StatusCreated {
				t.Fatalf("POST rounds = %d %s", rec.Code, rec.Body)
			}
			got := map[int64]int{}
			for _, e := range f.entrants {
				got[e.ID] = e.StartingRank
			}
			if tt.want != nil && !maps.Equal(got, tt.want) {
				t.Errorf("stored starting ranks = %v, want %v", got, tt.want)
			}
			if ranks := slices.Sorted(maps.Values(got)); !slices.Equal(ranks, []int{1, 2, 3, 4}) {
				t.Fatalf("stored starting ranks = %v, want 1 to 4 once each", got)
			}
			// The Dutch pairs the top half against the bottom half by starting rank: 1 v 3 and 2 v 4.
			byRank := map[int]int64{}
			for id, r := range got {
				byRank[r] = id
			}
			want := []string{fmt.Sprintf("%d-%d", min(byRank[1], byRank[3]), max(byRank[1], byRank[3])), fmt.Sprintf("%d-%d", min(byRank[2], byRank[4]), max(byRank[2], byRank[4]))}
			slices.Sort(want)
			if pairs := pairsOf(round); !slices.Equal(pairs, want) {
				t.Errorf("pairs = %v, want %v by the starting ranks %v", pairs, want, got)
			}
			rec = adminDoWith(t, ratings, f, org, http.MethodGet, "/v1/admin/blitz/us_open/2026", site, "")
			wantJSON(t, rec.Body.Bytes(), "entrants.0.starting_rank", fmt.Sprint(got[7]))
		})
	}
}
