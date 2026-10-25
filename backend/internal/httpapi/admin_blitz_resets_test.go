package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"ttacommunity/backend/internal/store"
)

// fakeLatest returns ErrBlitzNotLatest unless round number is the fake's latest.
func (f *fakeAdmins) fakeLatest(number int) error {
	if number != len(f.rounds) {
		return store.ErrBlitzNotLatest
	}
	return nil
}

func (f *fakeAdmins) ResetBlitzResult(_ context.Context, slug, edition string, id int64) error {
	m, err := f.findMatch(slug, edition, id)
	if err != nil {
		return err
	}
	if m.B == nil {
		return store.ErrBlitzBye
	}
	for _, r := range f.rounds {
		if slices.ContainsFunc(r.Matches, func(x store.BlitzMatch) bool { return x.ID == id }) {
			if err := f.fakeLatest(r.Number); err != nil {
				return err
			}
		}
	}
	m.Result, m.GameID, m.Game, m.Ruling, m.DecidedAt = "", nil, nil, "", nil
	f.notes = append(f.notes, fmt.Sprintf("reset match %d", id))
	return nil
}

func (f *fakeAdmins) ResetBlitzRound(_ context.Context, slug, edition string, id int64) error {
	i, err := f.findRound(slug, edition, id)
	if err != nil {
		return err
	}
	if err := f.fakeLatest(f.rounds[i].Number); err != nil {
		return err
	}
	for j := range f.rounds[i].Matches {
		m := &f.rounds[i].Matches[j]
		m.Result, m.GameID, m.Game, m.Ruling, m.DecidedAt = "", nil, nil, "", nil
	}
	f.notes = append(f.notes, fmt.Sprintf("reset round %d", id))
	return nil
}

func (f *fakeAdmins) ResetBlitzEdition(_ context.Context, slug, edition string) error {
	if slug != "us_open" || edition != "2026" {
		return store.ErrNoBlitzEdition
	}
	f.rounds = nil
	for i := range f.blitzEntrants() {
		f.entrants[i].Status, f.entrants[i].Skips = "active", nil
	}
	f.notes = append(f.notes, "reset edition")
	return nil
}

func TestAnOrganiserResetsAResultARoundAndTheEdition(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	roundOneDecided(f)
	f.entrants[3].Status = "dropped"
	for _, step := range []struct{ method, path string }{
		{http.MethodDelete, "/v1/admin/blitz/us_open/2026/rounds/101/matches/1010/result"},
		{http.MethodDelete, "/v1/admin/blitz/us_open/2026/rounds/101/results"},
		{http.MethodPost, "/v1/admin/blitz/us_open/2026/reset"},
	} {
		if rec := adminDoAs(t, f, org, step.method, step.path, site, ""); rec.Code != http.StatusNoContent {
			t.Fatalf("%s %s = %d %s, want 204", step.method, step.path, rec.Code, rec.Body)
		}
	}
	if want := "reset match 1010|reset round 101|reset edition"; strings.Join(f.notes, "|") != want {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	want := []string{org + " blitz.match.reset 1010", org + " blitz.round.reset 101", org + " blitz.reset us_open/2026"}
	if !slices.Equal(f.audits, want) {
		t.Errorf("audits = %q, want %q", f.audits, want)
	}
	rec := adminDoAs(t, f, org, http.MethodGet, "/v1/admin/blitz/us_open/2026", "", "")
	wantJSON(t, rec.Body.Bytes(), "rounds", `[]`)
	wantJSON(t, rec.Body.Bytes(), "entrants.3", `{"name": "Dee", "status": "active"}`)
}

func TestResetsAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site = "https://ttac-admin.example"
	twoRounds := func(f *fakeAdmins) {
		roundOneDecided(f)
		f.rounds = append(f.rounds, store.BlitzRound{ID: 102, Number: 2, Kind: "swiss", Method: "timezone", Matches: []store.BlitzMatch{{ID: 1020, A: 7, B: new(int64(9))}, {ID: 1021, A: 8}}})
	}
	tests := []struct {
		name, method, path, origin string
		setup                      func(*fakeAdmins)
		want                       int
	}{
		{"another site", http.MethodPost, "/v1/admin/blitz/us_open/2026/reset", "https://evil.example", roundOneDecided, http.StatusForbidden},
		{"a result of an earlier round", http.MethodDelete, "/v1/admin/blitz/us_open/2026/rounds/101/matches/1010/result", site, twoRounds, http.StatusConflict},
		{"an earlier round", http.MethodDelete, "/v1/admin/blitz/us_open/2026/rounds/101/results", site, twoRounds, http.StatusConflict},
		{"a bye", http.MethodDelete, "/v1/admin/blitz/us_open/2026/rounds/102/matches/1021/result", site, twoRounds, http.StatusConflict},
		{"an unknown match", http.MethodDelete, "/v1/admin/blitz/us_open/2026/rounds/101/matches/99/result", site, roundOneDecided, http.StatusNotFound},
		{"an unknown round", http.MethodDelete, "/v1/admin/blitz/us_open/2026/rounds/99/results", site, roundOneDecided, http.StatusNotFound},
		{"an unknown edition", http.MethodPost, "/v1/admin/blitz/us_open/2027/reset", site, roundOneDecided, http.StatusNotFound},
		{"a bad slug", http.MethodPost, "/v1/admin/blitz/Bad!/2026/reset", site, roundOneDecided, http.StatusNotFound},
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
				t.Errorf("a refused reset still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
}
