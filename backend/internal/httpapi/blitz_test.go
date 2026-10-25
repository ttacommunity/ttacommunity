package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

// The fake's shown blitz edition is us_open 2026 with Ann (known to the ratings) and bob (not) still in.

func (f *fakeRatings) BlitzEditions(context.Context) ([]store.BlitzEditionRow, error) {
	return []store.BlitzEditionRow{{Slug: "us_open", Name: "US Open", Edition: "2026", Format: store.DefaultBlitzFormat, Points: blitz.FIDEScoring, Entrants: 2, Round: new(2)}}, nil
}

func (f *fakeRatings) BlitzEdition(ctx context.Context, slug, edition string) (store.BlitzEdition, error) {
	if slug != "us_open" || edition != "2026" {
		return store.BlitzEdition{}, store.ErrNoBlitzEdition
	}
	if f.blitzEdition != nil {
		return *f.blitzEdition, nil
	}
	rows, _ := f.BlitzEditions(ctx)
	bob := int64(8)
	at := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	game := int64(500)
	finished := time.Date(2026, 10, 14, 21, 30, 0, 0, time.FixedZone("CEST", 2*3600))
	return store.BlitzEdition{BlitzEditionRow: rows[0], Entrants: []store.BlitzEntrant{
		{ID: 7, MemberID: 1007, Name: "Ann", Player: "Ann", Zone: "Europe/Berlin", Country: "DE", Status: "active"},
		{ID: 8, MemberID: 1008, Name: "bob", Zone: "America/New_York", Status: "active"},
	}, Rounds: []store.BlitzRound{
		{ID: 101, Number: 1, Kind: "swiss", PublishedAt: &at, Matches: []store.BlitzMatch{{ID: 1, A: 7, B: &bob, Result: "b", GameID: &game, DecidedAt: &at,
			Game:   &store.BlitzGame{ShareCode: "ABCD1234", A: store.BlitzSide{Culture: 200, Outcome: "finished"}, B: store.BlitzSide{Culture: 210, Outcome: "finished"}, Finished: finished},
			Ruling: "Ann played with an alt account"}}},
		{ID: 102, Number: 2, Kind: "swiss", PublishedAt: &at, Matches: []store.BlitzMatch{{ID: 2, A: 7}}},
	}}, nil
}

func TestTheSiteListsBlitzEditionsAndTheirEntrants(t *testing.T) {
	ratings := &fakeRatings{run: store.PublishedRun{ID: 7}, rated: map[string]store.Rated{"Ann": {Rating: 1700, RD: 60}}}
	rec, _ := serve(t, ratings, "/v1/blitz")
	var list BlitzEditions
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/blitz = %d %s", rec.Code, rec.Body)
	}
	if len(list.Editions) != 1 || list.Editions[0].Tournament != "us_open" || list.Editions[0].Entrants != 2 || list.Editions[0].Round == nil || *list.Editions[0].Round != 2 {
		t.Errorf("editions = %+v, want us_open 2026 with 2 entrants at round 2", list.Editions)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheCurrent {
		t.Errorf("Cache-Control = %q, want %q", got, cacheCurrent)
	}

	rec, _ = serve(t, ratings, "/v1/blitz/us_open/2026")
	var ed BlitzEdition
	if err := json.Unmarshal(rec.Body.Bytes(), &ed); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/blitz/us_open/2026 = %d %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNone {
		t.Errorf("the edition's Cache-Control = %q, want %q: its results change as they are recorded", got, cacheNone)
	}
	ann := 1700.0
	de := "DE"
	want := []BlitzEntrant{
		{Name: "Ann", Country: &de, Zone: "Europe/Berlin", UtcOffset: "+02:00", Rating: &ann},
		{Name: "bob", Country: nil, Zone: "America/New_York", UtcOffset: "-04:00", Rating: nil},
	}
	if diff := cmp.Diff(want, ed.Entrants); diff != "" {
		t.Errorf("entrants (-want +got):\n%s", diff)
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "discord") {
		t.Errorf("the site's edition mentions Discord: %s", rec.Body)
	}
	bob := "bob"
	won := BlitzResult("b")
	game := &BlitzPlayedGame{ShareCode: "ABCD1234", ACulture: 200, AOutcome: "finished", BCulture: 210, BOutcome: "finished", FinishedAt: time.Date(2026, 10, 14, 19, 30, 0, 0, time.UTC)}
	wantRounds := []BlitzPublicMatch{{A: "Ann", ARating: &ann, B: &bob, BRating: nil, Result: &won, Game: game, Ruled: true}, {A: "Ann", ARating: &ann, B: nil, BRating: nil}}
	if len(ed.Rounds) != 2 || ed.Rounds[0].Number != 1 || ed.Rounds[1].Number != 2 {
		t.Fatalf("rounds = %+v, want rounds 1 and 2", ed.Rounds)
	}
	if diff := cmp.Diff(wantRounds, []BlitzPublicMatch{ed.Rounds[0].Matches[0], ed.Rounds[1].Matches[0]}); diff != "" {
		t.Errorf("the rounds' matches (-want +got):\n%s", diff)
	}
	if strings.Contains(rec.Body.String(), "alt account") {
		t.Errorf("the site's edition carries the ruling's reason: %s", rec.Body)
	}
	// bob beat Ann in a game, so he goes first despite the lower Buchholz; round 2 is unplayed for both.
	wantStandings := []BlitzStanding{
		{Rank: 1, PlaceFrom: 1, PlaceTo: 1, Name: "bob", Country: nil, Points: 1, Games: 1, Byes: 0, Buchholz: 2, Status: "active"},
		{Rank: 2, PlaceFrom: 2, PlaceTo: 2, Name: "Ann", Country: &de, Points: 1, Games: 1, Byes: 1, Buchholz: 2.5, Status: "active"},
	}
	if diff := cmp.Diff(wantStandings, ed.Standings); diff != "" {
		t.Errorf("standings (-want +got):\n%s", diff)
	}

	rec, _ = serve(t, &fakeRatings{runErr: store.ErrNoPublishedRun}, "/v1/blitz/us_open/2026")
	if err := json.Unmarshal(rec.Body.Bytes(), &ed); err != nil || rec.Code != http.StatusOK || ed.Entrants[0].Rating != nil {
		t.Errorf("GET edition with no published run = %d %s, want 200 with no ratings", rec.Code, rec.Body)
	}
}

func TestTheSiteBlitzRoutesRefuseWhatTheyShould(t *testing.T) {
	tests := []struct {
		name, path string
		want       int
	}{
		{"a bad slug", "/v1/blitz/Bad!/2026", http.StatusBadRequest},
		{"an unknown edition", "/v1/blitz/us_open/2027", http.StatusNotFound},
		{"an unknown tournament", "/v1/blitz/worlds/2026", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec, _ := serve(t, &fakeRatings{}, tt.path); rec.Code != tt.want {
				t.Errorf("GET %s = %d %s, want %d", tt.path, rec.Code, rec.Body, tt.want)
			}
		})
	}
}

func TestTheSiteShowsTheKnockoutWithSeedsAndTheChampion(t *testing.T) {
	bob := int64(8)
	at := time.Date(2026, 12, 21, 12, 0, 0, 0, time.UTC)
	ed := store.BlitzEdition{
		BlitzEditionRow: store.BlitzEditionRow{Slug: "us_open", Name: "US Open", Edition: "2026", Format: store.BlitzFormat{SwissRounds: 1, KnockoutSize: 2}, Points: blitz.FIDEScoring,
			Entrants: 2, Round: new(2), Champion: new("bob")},
		Entrants: []store.BlitzEntrant{{ID: 7, Name: "Ann", Zone: "UTC", Status: "active"}, {ID: 8, Name: "bob", Zone: "UTC", Status: "active"}},
		Rounds: []store.BlitzRound{
			{ID: 101, Number: 1, Kind: "swiss", PublishedAt: &at, Matches: []store.BlitzMatch{{ID: 1, A: 7, B: &bob, Result: "a", Ruling: "ruled", DecidedAt: &at}}},
			{ID: 102, Number: 2, Kind: "knockout", PublishedAt: &at, Matches: []store.BlitzMatch{{ID: 2, A: 7, B: &bob, Slot: new(0), SeedA: new(1), SeedB: new(2), Result: "b", Ruling: "ruled", DecidedAt: &at}}},
		},
	}
	rec, _ := serve(t, &fakeRatings{run: store.PublishedRun{ID: 7}, blitzEdition: &ed}, "/v1/blitz/us_open/2026")
	var got BlitzEdition
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK || len(got.Rounds) != 2 {
		t.Fatalf("GET /v1/blitz/us_open/2026 = %d %s", rec.Code, rec.Body)
	}
	if got.Champion == nil || *got.Champion != "bob" {
		t.Errorf("champion = %v, want bob", got.Champion)
	}
	if swiss := got.Rounds[0]; swiss.Kind != "swiss" || swiss.Matches[0].ASeed != nil {
		t.Errorf("the Swiss round = %+v, want kind swiss and no seeds", swiss)
	}
	final := got.Rounds[1]
	if m := final.Matches[0]; final.Kind != "knockout" || m.ASeed == nil || *m.ASeed != 1 || m.BSeed == nil || *m.BSeed != 2 {
		t.Errorf("the final = %+v, want kind knockout, Ann seeded 1 and bob 2", final)
	}
}

func TestTheSiteStandingsTellAForfeitFromAGame(t *testing.T) {
	tests := []struct {
		name string
		game *int64
		want float64
	}{
		{"a ruling with no game is a forfeit: the loser's dummy has the loser's own score", nil, 0},
		{"a ruling over a played game counts as that game", new(int64(500)), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bob := int64(8)
			at := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
			ed := store.BlitzEdition{
				BlitzEditionRow: store.BlitzEditionRow{Slug: "us_open", Name: "US Open", Edition: "2026", Format: store.DefaultBlitzFormat, Points: blitz.FIDEScoring, Entrants: 2, Round: new(1)},
				Entrants:        []store.BlitzEntrant{{ID: 7, Name: "Ann", Zone: "UTC", Status: "active"}, {ID: 8, Name: "bob", Zone: "UTC", Status: "active"}},
				Rounds: []store.BlitzRound{{ID: 101, Number: 1, Kind: "swiss", PublishedAt: &at,
					Matches: []store.BlitzMatch{{ID: 1, A: 7, B: &bob, Result: "b", GameID: tt.game, Ruling: "ruled", DecidedAt: &at}}}},
			}
			rec, _ := serve(t, &fakeRatings{run: store.PublishedRun{ID: 7}, blitzEdition: &ed}, "/v1/blitz/us_open/2026")
			var got BlitzEdition
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
				t.Fatalf("GET /v1/blitz/us_open/2026 = %d %s", rec.Code, rec.Body)
			}
			for _, s := range got.Standings {
				if s.Name == "Ann" && s.Buchholz != tt.want {
					t.Errorf("Ann's Buchholz = %g, want %g", s.Buchholz, tt.want)
				}
			}
		})
	}
}

func TestTheSiteListsWhoSkippedEachRound(t *testing.T) {
	ratings := &fakeRatings{run: store.PublishedRun{ID: 7}}
	ed, _ := ratings.BlitzEdition(context.Background(), "us_open", "2026")
	ed.Points.Skip = 0.5
	ed.Entrants = append(ed.Entrants, store.BlitzEntrant{ID: 9, MemberID: 1009, Name: "Cy", Zone: "UTC", Status: "active", Skips: []int{1, 2}})
	ratings.blitzEdition = &ed
	rec, _ := serve(t, ratings, "/v1/blitz/us_open/2026")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/blitz/us_open/2026 = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "rounds.0.skipped", `["Cy"]`)
	wantJSON(t, rec.Body.Bytes(), "rounds.1.skipped", `["Cy"]`)
	var got BlitzEdition
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, s := range got.Standings {
		if s.Name == "Cy" && s.Points != 1 {
			t.Errorf("Cy, who skipped both rounds = %g points, want the edition's 0.5 for each", s.Points)
		}
	}
}

func TestTheSiteScoresByTheEditionsPoints(t *testing.T) {
	tests := []struct {
		name     string
		points   blitz.Scoring
		wantFIDE bool
		wantBob  float64
	}{
		{"FIDE's", blitz.FIDEScoring, true, 1},
		{"the edition's own", blitz.Scoring{Win: 3, Draw: 1, ForfeitWin: 3, Bye: 3}, false, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratings := &fakeRatings{run: store.PublishedRun{ID: 7}}
			ed, _ := ratings.BlitzEdition(context.Background(), "us_open", "2026")
			ed.Points = tt.points
			ratings.blitzEdition = &ed
			rec, _ := serve(t, ratings, "/v1/blitz/us_open/2026")
			if rec.Code != http.StatusOK {
				t.Fatalf("GET /v1/blitz/us_open/2026 = %d %s", rec.Code, rec.Body)
			}
			p := tt.points
			wantJSON(t, rec.Body.Bytes(), "points", fmt.Sprintf(`{"win": %g, "draw": %g, "loss": %g, "forfeit_win": %g, "forfeit_loss": %g, "bye": %g, "skip": %g}`,
				p.Win, p.Draw, p.Loss, p.ForfeitWin, p.ForfeitLoss, p.Bye, p.Skip))
			wantJSON(t, rec.Body.Bytes(), "fide_points", fmt.Sprint(tt.wantFIDE))
			var got BlitzEdition
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			for _, s := range got.Standings {
				if s.Name == "bob" && s.Points != tt.wantBob {
					t.Errorf("bob, who won round 1's game = %g points, want %g", s.Points, tt.wantBob)
				}
			}
		})
	}
}

func TestTheSiteStandingsSharePlacesOnlyTheDrawSeparates(t *testing.T) {
	bob, at := int64(8), time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		matches []store.BlitzMatch
		want    map[string][2]int
	}{
		{"before any result, both share the first place", []store.BlitzMatch{{ID: 1, A: 7, B: &bob}}, map[string][2]int{"Ann": {1, 2}, "bob": {1, 2}}},
		{"a result separates them", []store.BlitzMatch{{ID: 1, A: 7, B: &bob, Result: "a", GameID: new(int64(500)), DecidedAt: &at}}, map[string][2]int{"Ann": {1, 1}, "bob": {2, 2}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ed := store.BlitzEdition{
				BlitzEditionRow: store.BlitzEditionRow{Slug: "us_open", Name: "US Open", Edition: "2026", Format: store.DefaultBlitzFormat, Points: blitz.FIDEScoring, Entrants: 2, Round: new(1)},
				Entrants:        []store.BlitzEntrant{{ID: 7, Name: "Ann", Zone: "UTC", Status: "active"}, {ID: 8, Name: "bob", Zone: "UTC", Status: "active"}},
				Rounds:          []store.BlitzRound{{ID: 101, Number: 1, Kind: "swiss", PublishedAt: &at, Matches: tt.matches}},
			}
			rec, _ := serve(t, &fakeRatings{run: store.PublishedRun{ID: 7}, blitzEdition: &ed}, "/v1/blitz/us_open/2026")
			var got BlitzEdition
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
				t.Fatalf("GET /v1/blitz/us_open/2026 = %d %s", rec.Code, rec.Body)
			}
			for _, s := range got.Standings {
				if want := tt.want[s.Name]; s.PlaceFrom != want[0] || s.PlaceTo != want[1] {
					t.Errorf("%s's place = %d–%d, want %d–%d", s.Name, s.PlaceFrom, s.PlaceTo, want[0], want[1])
				}
			}
		})
	}
}
