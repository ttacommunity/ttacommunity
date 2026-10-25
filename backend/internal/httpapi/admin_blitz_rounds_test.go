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

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

func matchText(matches []store.BlitzMatch) string {
	var parts []string
	for _, m := range matches {
		if m.B == nil {
			parts = append(parts, fmt.Sprintf("bye:%d", m.A))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", m.A, *m.B))
		}
	}
	return strings.Join(parts, ",")
}

func (f *fakeAdmins) findRound(slug, edition string, id int64) (int, error) {
	if slug != "us_open" || edition != "2026" {
		return -1, store.ErrNoBlitzEdition
	}
	for i, r := range f.rounds {
		if r.ID == id {
			return i, nil
		}
	}
	return -1, store.ErrNoBlitzRound
}

func (f *fakeAdmins) AddBlitzRound(_ context.Context, slug, edition string, r store.BlitzRound) (store.BlitzRound, error) {
	if slug != "us_open" || edition != "2026" {
		return store.BlitzRound{}, store.ErrNoBlitzEdition
	}
	for _, x := range f.rounds {
		if x.PublishedAt == nil {
			return store.BlitzRound{}, store.ErrBlitzDraftExists
		}
		for _, m := range x.Matches {
			if m.B != nil && m.Result == "" {
				return store.BlitzRound{}, fmt.Errorf("%w: entrant %d v entrant %d", store.ErrBlitzRoundUndecided, m.A, *m.B)
			}
		}
	}
	r.ID, r.Number = int64(101+len(f.rounds)), len(f.rounds)+1
	if r.Kind == "" {
		r.Kind = "swiss"
	}
	for i := range r.Matches {
		r.Matches[i].ID = r.ID*10 + int64(i)
	}
	f.rounds = append(f.rounds, r)
	f.notes = append(f.notes, fmt.Sprintf("add round %d", r.Number))
	return r, nil
}

func (f *fakeAdmins) SetBlitzRoundMatches(_ context.Context, slug, edition string, id int64, matches []store.BlitzMatch) error {
	i, err := f.findRound(slug, edition, id)
	if err != nil {
		return err
	}
	if f.rounds[i].Kind == "knockout" {
		return store.ErrBlitzKnockoutFixed
	}
	for _, m := range matches {
		if m.A == 99 || (m.B != nil && *m.B == 99) {
			return fmt.Errorf("%w: entrant 99", store.ErrBadBlitzMatch)
		}
	}
	for _, d := range f.rounds[i].Matches {
		if d.Result != "" && !slices.ContainsFunc(matches, func(m store.BlitzMatch) bool { return m.A == d.A && *m.B == *d.B }) {
			return fmt.Errorf("%w: entrant %d and entrant %d", store.ErrBlitzMatchDecided, d.A, *d.B)
		}
	}
	f.rounds[i].Matches = matches
	f.notes = append(f.notes, fmt.Sprintf("set round %d matches %s", id, matchText(matches)))
	return nil
}

func (f *fakeAdmins) PublishBlitzRound(_ context.Context, slug, edition string, id int64) error {
	i, err := f.findRound(slug, edition, id)
	if err != nil {
		return err
	}
	if len(f.rounds[i].Matches) == 0 {
		return fmt.Errorf("%w: Ann", store.ErrBlitzRoundIncomplete)
	}
	at := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	f.rounds[i].PublishedAt = &at
	f.notes = append(f.notes, fmt.Sprintf("publish round %d", id))
	return nil
}

func (f *fakeAdmins) DeleteBlitzRound(_ context.Context, slug, edition string, id int64) error {
	i, err := f.findRound(slug, edition, id)
	if err != nil {
		return err
	}
	if i != len(f.rounds)-1 {
		return store.ErrBlitzRoundNotLast
	}
	if slices.ContainsFunc(f.rounds[i].Matches, func(m store.BlitzMatch) bool { return m.Result != "" }) {
		return store.ErrBlitzRoundPlayed
	}
	f.rounds = append(f.rounds[:i], f.rounds[i+1:]...)
	f.notes = append(f.notes, fmt.Sprintf("remove round %d", id))
	return nil
}

// fourEntered gives the fake Ann (7, Berlin, rated), Cy (8, Berlin, rated), bob (9, New York) and Dee (10, Chicago),
// in an edition paired by time zone throughout.
func fourEntered(f *fakeAdmins) {
	f.blitzEntrants()
	f.pairing = store.BlitzPairing{First: "timezone", Later: "timezone"}
	_, _ = f.AddBlitzEntrants(context.Background(), "us_open", "2026", []store.BlitzEntrant{
		{Name: "Cy", Player: "Cy", Zone: "Europe/Berlin"}, {Name: "bob", Zone: "America/New_York"}, {Name: "Dee", Zone: "America/Chicago"},
	})
	f.notes = nil
}

func TestAnOrganiserPairsChangesAndPublishesARound(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	fourEntered(f)
	ratings := &fakeRatings{run: store.PublishedRun{ID: 7}, rated: map[string]store.Rated{"Ann": {Rating: 1700}, "Cy": {Rating: 1650}}}
	do := func(method, path, body string) *httptest.ResponseRecorder {
		return adminDoWith(t, ratings, f, org, method, path, site, body)
	}

	rec := do(http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds", `{"timezone": true}`)
	var round AdminBlitzRound
	if err := json.Unmarshal(rec.Body.Bytes(), &round); err != nil || rec.Code != http.StatusCreated {
		t.Fatalf("POST rounds = %d %s", rec.Code, rec.Body)
	}
	if round.Number != 1 || round.PublishedAt != nil || len(round.Matches) != 2 {
		t.Fatalf("round = %+v, want a draft round 1 with two matches", round)
	}
	for _, m := range round.Matches {
		switch {
		case m.B == nil:
			t.Errorf("match %+v is a bye, want none with four players", m)
		case m.A == 7 || *m.B == 7:
			if other := m.A + *m.B - 7; other != 8 || m.ZoneGapMinutes != 0 || m.RatingGap == nil || *m.RatingGap != 50 {
				t.Errorf("Ann's match = %+v, want Cy (8) in the same zone, 50 rating points apart", m)
			}
		default:
			if m.ZoneGapMinutes != 60 || m.RatingGap != nil || m.Repeat {
				t.Errorf("the other match = %+v, want bob and Dee an hour apart, no rating gap, no repeat", m)
			}
		}
	}

	rec = do(http.MethodGet, "/v1/admin/blitz/us_open/2026", "")
	var ed AdminBlitzEdition
	if err := json.Unmarshal(rec.Body.Bytes(), &ed); err != nil || rec.Code != http.StatusOK || len(ed.Rounds) != 1 || len(ed.Rounds[0].Matches) != 2 {
		t.Fatalf("GET edition = %d %s, want the draft round in it", rec.Code, rec.Body)
	}
	for _, step := range []struct{ method, path, body string }{
		{http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/matches", `{"matches": [{"a": 7, "b": 9}, {"a": 8, "b": 10}]}`},
		{http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds/101/publish", ""},
	} {
		if rec := do(step.method, step.path, step.body); rec.Code != http.StatusNoContent {
			t.Errorf("%s %s = %d %s, want 204", step.method, step.path, rec.Code, rec.Body)
		}
	}
	if rec := do(http.MethodDelete, "/v1/admin/blitz/us_open/2026/rounds/101", ""); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE the latest round, published but unplayed = %d %s, want 204", rec.Code, rec.Body)
	}
	if want := "add round 1|set round 101 matches 7-9,8-10|publish round 101|remove round 101"; strings.Join(f.notes, "|") != want {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	want := []string{"blitz.round.add us_open/2026", "blitz.round.matches 101", "blitz.round.publish 101", "blitz.round.remove 101"}
	for i, w := range want {
		if i >= len(f.audits) || f.audits[i] != org+" "+w {
			t.Errorf("audit %d = %q, want %q", i, f.audits, org+" "+w)
		}
	}
}

func TestAProposedPairingKeepsDecidedMatchesAndWritesNothing(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	roundOnePublished(f)
	at := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	f.rounds[0].Matches[0].Result, f.rounds[0].Matches[0].DecidedAt, f.rounds[0].Matches[0].Ruling = "a", &at, "ruled"
	ratings := &fakeRatings{run: store.PublishedRun{ID: 7}}
	rec := adminDoWith(t, ratings, f, org, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds/101/pairing", site, `{"timezone": true}`)
	var round AdminBlitzRound
	if err := json.Unmarshal(rec.Body.Bytes(), &round); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("POST pairing = %d %s, want 200", rec.Code, rec.Body)
	}
	if round.Id != 101 || round.Number != 1 || round.PublishedAt == nil || len(round.Matches) != 2 {
		t.Fatalf("proposal = %+v, want round 101 with two matches", round)
	}
	kept, fresh := round.Matches[0], round.Matches[1]
	if kept.Id != 1010 || kept.Result == nil || *kept.Result != "a" {
		t.Errorf("the decided match = %+v, want Ann v Cy kept with its id and result", kept)
	}
	if fresh.Id != 0 || fresh.Result != nil || fresh.B == nil || fresh.A+*fresh.B != 19 {
		t.Errorf("the fresh match = %+v, want bob and Dee paired again with no id", fresh)
	}
	if len(f.notes)+len(f.audits) != 0 {
		t.Errorf("a proposal changed something: %q %q", f.notes, f.audits)
	}
	if rec := adminDoWith(t, ratings, f, org, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds/5/pairing", site, `{}`); rec.Code != http.StatusNotFound {
		t.Errorf("a proposal for an unknown round = %d, want 404", rec.Code)
	}
}

func TestAProposedPairingKeepsTheMatchesNamedAndPairsTheRest(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	deeDropped := func(f *fakeAdmins) {
		roundOnePublished(f)
		f.entrants[3].Status = "dropped"
	}
	annBeatCy := func(f *fakeAdmins) {
		roundOnePublished(f)
		at := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
		f.rounds[0].Matches[0].Result, f.rounds[0].Matches[0].DecidedAt, f.rounds[0].Matches[0].Ruling = "a", &at, "ruled"
	}
	tests := []struct {
		name  string
		setup func(*fakeAdmins)
		body  string
		want  string
	}{
		{"nothing kept pairs everyone again", roundOnePublished, `{}`, "7-8,9-10"},
		{"a kept pair stays and the rest are paired", roundOnePublished, `{"keep": [{"a": 7, "b": 9}]}`, "7-9,8-10"},
		{"everyone kept changes nothing", roundOnePublished, `{"keep": [{"a": 7, "b": 9}, {"a": 8, "b": 10}]}`, "7-9,8-10"},
		{"a kept bye is paired when one is left over", roundOnePublished, `{"keep": [{"a": 7, "b": 9}, {"a": 8}]}`, "7-9,8-10"},
		{"the one left over gets the bye", deeDropped, `{"keep": [{"a": 7, "b": 9}]}`, "7-9,bye:8"},
		{"a decided match named in keep stays once", annBeatCy, `{"keep": [{"a": 7, "b": 8}]}`, "7-8,9-10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			tt.setup(f)
			rec := adminDoWith(t, &fakeRatings{run: store.PublishedRun{ID: 7}}, f, org, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds/101/pairing", site, tt.body)
			var round AdminBlitzRound
			if err := json.Unmarshal(rec.Body.Bytes(), &round); err != nil || rec.Code != http.StatusOK {
				t.Fatalf("POST pairing %s = %d %s, want 200", tt.body, rec.Code, rec.Body)
			}
			var pairs []string
			for _, m := range round.Matches {
				if m.B == nil {
					pairs = append(pairs, fmt.Sprintf("bye:%d", m.A))
				} else {
					pairs = append(pairs, fmt.Sprintf("%d-%d", min(m.A, *m.B), max(m.A, *m.B)))
				}
			}
			slices.Sort(pairs)
			if got := strings.Join(pairs, ","); got != tt.want {
				t.Errorf("POST pairing %s = %s, want %s", tt.body, got, tt.want)
			}
		})
	}
}

func TestTheNextRoundIsPairedOnPoints(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	roundOnePublished(f)
	at := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	for i := range f.rounds[0].Matches { // Ann beat Cy, bob beat Dee: the winners meet in round 2 despite the zones
		f.rounds[0].Matches[i].Result, f.rounds[0].Matches[i].DecidedAt, f.rounds[0].Matches[i].Ruling = "a", &at, "ruled"
	}
	rec := adminDoWith(t, &fakeRatings{run: store.PublishedRun{ID: 7}}, f, org, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds", site, `{"timezone": true}`)
	var round AdminBlitzRound
	if err := json.Unmarshal(rec.Body.Bytes(), &round); err != nil || rec.Code != http.StatusCreated || len(round.Matches) != 2 {
		t.Fatalf("POST rounds = %d %s, want round 2 with two matches", rec.Code, rec.Body)
	}
	var pairs []string
	for _, m := range round.Matches {
		pairs = append(pairs, fmt.Sprintf("%d-%d", min(m.A, *m.B), max(m.A, *m.B)))
	}
	slices.Sort(pairs)
	if want := []string{"7-9", "8-10"}; !slices.Equal(pairs, want) {
		t.Errorf("round 2's pairs = %v, want %v (the winners together, the losers together)", pairs, want)
	}
}

func TestBlitzRoundChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	draft := func(f *fakeAdmins) {
		fourEntered(f)
		f.rounds = []store.BlitzRound{{ID: 101, Number: 1, Kind: "swiss", Method: "timezone", Seed: 1}}
	}
	twoRounds := func(f *fakeAdmins) {
		draft(f)
		at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
		f.rounds[0].PublishedAt = &at
		f.rounds = append(f.rounds, store.BlitzRound{ID: 102, Number: 2, Kind: "swiss", Method: "timezone", Seed: 2})
	}
	oneDecided := func(f *fakeAdmins) {
		roundOnePublished(f)
		at := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
		f.rounds[0].Matches[0].Result, f.rounds[0].Matches[0].DecidedAt, f.rounds[0].Matches[0].Ruling = "a", &at, "ruled"
	}
	tests := []struct {
		name                       string
		setup                      func(*fakeAdmins)
		method, path, origin, body string
		want                       int
	}{
		{"pairing one entrant", nil, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds", site, `{}`, http.StatusBadRequest},
		{"pairing while a draft exists", draft, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds", site, `{}`, http.StatusConflict},
		{"pairing an unknown edition", fourEntered, http.MethodPost, "/v1/admin/blitz/us_open/2027/rounds", site, `{}`, http.StatusNotFound},
		{"pairing from another site", fourEntered, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds", "https://evil.example", `{}`, http.StatusForbidden},
		{"a match naming nobody", draft, http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/matches", site, `{"matches": [{"a": 99, "b": 7}]}`, http.StatusBadRequest},
		{"matches for an unknown round", draft, http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/5/matches", site, `{"matches": []}`, http.StatusNotFound},
		{"publishing an empty round", draft, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds/101/publish", site, "", http.StatusConflict},
		{"publishing an unknown round", draft, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds/5/publish", site, "", http.StatusNotFound},
		{"deleting a round behind a later one", twoRounds, http.MethodDelete, "/v1/admin/blitz/us_open/2026/rounds/101", site, "", http.StatusConflict},
		{"deleting an unknown round", draft, http.MethodDelete, "/v1/admin/blitz/us_open/2026/rounds/5", site, "", http.StatusNotFound},
		{"pairing while a result is missing", oneDecided, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds", site, `{}`, http.StatusConflict},
		{"leaving out a decided match", oneDecided, http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/matches", site, `{"matches": [{"a": 9, "b": 10}]}`, http.StatusBadRequest},
		{"removing an entrant who has played", swissDone, http.MethodDelete, "/v1/admin/blitz/us_open/2026/entrants/7", site, "", http.StatusConflict},
		{"removing a wildcard in a knockout match", wildcardInFinal, http.MethodDelete, "/v1/admin/blitz/us_open/2026/entrants/11", site, "", http.StatusConflict},
		{"deleting a round with a result", oneDecided, http.MethodDelete, "/v1/admin/blitz/us_open/2026/rounds/101", site, "", http.StatusConflict},
		{"keeping an unknown entrant", roundOnePublished, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds/101/pairing", site, `{"keep": [{"a": 99, "b": 7}]}`, http.StatusBadRequest},
		{"keeping a dropped entrant", func(f *fakeAdmins) { roundOnePublished(f); f.entrants[3].Status = "dropped" }, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds/101/pairing", site, `{"keep": [{"a": 9, "b": 10}]}`, http.StatusBadRequest},
		{"keeping someone twice", roundOnePublished, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds/101/pairing", site, `{"keep": [{"a": 7, "b": 8}, {"a": 8}]}`, http.StatusBadRequest},
		{"keeping a player of a decided match elsewhere", oneDecided, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds/101/pairing", site, `{"keep": [{"a": 7, "b": 9}]}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			if tt.setup != nil {
				tt.setup(f)
			}
			rec := adminDoAs(t, f, owner, tt.method, tt.path, tt.origin, tt.body)
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
}

func TestTheByeGoesByTheStandingsNotTheRatings(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	roundOnePublished(f)
	at := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	// Ann and Cy both forfeited; bob beat Dee in a game and then dropped. Dee's opponent scored, so Dee's Buchholz
	// places Dee above Ann and Cy, though Dee is the one without a rating.
	f.rounds[0].Matches[0].Result, f.rounds[0].Matches[0].DecidedAt, f.rounds[0].Matches[0].Ruling = "double_loss", &at, "no-show"
	f.rounds[0].Matches[1].Result, f.rounds[0].Matches[1].DecidedAt, f.rounds[0].Matches[1].GameID = "a", &at, new(int64(500))
	f.entrants[2].Status = "dropped"
	ratings := &fakeRatings{run: store.PublishedRun{ID: 7}, rated: map[string]store.Rated{"Ann": {Rating: 1700}, "Cy": {Rating: 1650}}}
	rec := adminDoWith(t, ratings, f, org, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds", site, `{"timezone": true}`)
	var round AdminBlitzRound
	if err := json.Unmarshal(rec.Body.Bytes(), &round); err != nil || rec.Code != http.StatusCreated {
		t.Fatalf("POST rounds = %d %s, want round 2", rec.Code, rec.Body)
	}
	for _, m := range round.Matches {
		if m.B == nil && m.A != 7 && m.A != 8 {
			t.Errorf("the bye went to entrant %d, want Ann (7) or Cy (8), the lowest in the standings", m.A)
		}
	}
}

func TestTheByeGoesByTheEditionsPoints(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	forfeitHalf := blitz.FIDEScoring
	forfeitHalf.ForfeitLoss = 0.5
	tests := []struct {
		name   string
		points blitz.Scoring
		want   int64
	}{
		// Cy lost by forfeit and Dee a game: tied on 0, Cy's Buchholz is the lower.
		{"FIDE's", blitz.FIDEScoring, 8},
		{"a forfeit loss worth a half", forfeitHalf, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			fourEntered(f)
			ids, _ := f.AddBlitzEntrants(context.Background(), "us_open", "2026", []store.BlitzEntrant{{Name: "Eve", Zone: "Europe/London"}})
			f.notes = nil
			cy, dee := int64(8), int64(10)
			at := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
			f.rounds = []store.BlitzRound{{ID: 101, Number: 1, Kind: "swiss", Method: "timezone", Seed: 1, PublishedAt: &at, Matches: []store.BlitzMatch{
				{ID: 1010, A: 7, B: &cy, Result: "a", Ruling: "no-show", DecidedAt: &at},
				{ID: 1011, A: 9, B: &dee, Result: "a", GameID: new(int64(500)), DecidedAt: &at},
				{ID: 1012, A: ids[0]}}}}
			f.points = tt.points
			rec := adminDoWith(t, &fakeRatings{run: store.PublishedRun{ID: 7}}, f, org, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds", site, `{"timezone": true}`)
			var round AdminBlitzRound
			if err := json.Unmarshal(rec.Body.Bytes(), &round); err != nil || rec.Code != http.StatusCreated {
				t.Fatalf("POST rounds = %d %s, want round 2", rec.Code, rec.Body)
			}
			for _, m := range round.Matches {
				if m.B == nil && m.A != tt.want {
					t.Errorf("the bye went to entrant %d, want %d, the lowest by the edition's points", m.A, tt.want)
				}
			}
		})
	}
}

func TestAdminStandingsCountOnlyPublishedRounds(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	roundOnePublished(f)
	cy := int64(8)
	f.rounds = append(f.rounds, store.BlitzRound{ID: 102, Number: 2, Kind: "swiss", Method: "timezone", Matches: []store.BlitzMatch{{ID: 1020, A: 7, B: &cy}, {ID: 1021, A: 9}}})
	rec := adminDoWith(t, &fakeRatings{run: store.PublishedRun{ID: 7}}, f, org, http.MethodGet, "/v1/admin/blitz/us_open/2026", site, "")
	var ed AdminBlitzEdition
	if err := json.Unmarshal(rec.Body.Bytes(), &ed); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET edition = %d %s", rec.Code, rec.Body)
	}
	for _, s := range ed.Standings {
		if s.Name == "bob" && (s.Points != 0 || s.Byes != 0) {
			t.Errorf("bob = %g points and %d byes, want none from the draft round's bye", s.Points, s.Byes)
		}
	}
}
