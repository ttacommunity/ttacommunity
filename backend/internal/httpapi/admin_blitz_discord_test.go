package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

func (f *fakeAdmins) SetBlitzRoundDeadline(_ context.Context, slug, edition string, id int64, at time.Time, zone string) error {
	i, err := f.findRound(slug, edition, id)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(f.rounds[i].Matches, func(m store.BlitzMatch) bool { return m.B != nil && m.Result == "" }) {
		return store.ErrBlitzRoundFinished
	}
	f.rounds[i].Deadline, f.rounds[i].DeadlineZone = &at, zone
	f.notes = append(f.notes, fmt.Sprintf("deadline round %d %s %s", id, at.Format(time.RFC3339), zone))
	return nil
}

// discordFixture is us_open 2026 with Ann (ann_tta), Cy (a display name), bob (BobPlays), Dee (none) and Eve, round
// 1 a draft pairing Ann with Cy, Eve with the bye and bob with Dee.
func discordFixture() *fakeAdmins {
	cy, dee := int64(8), int64(10)
	return &fakeAdmins{
		entrants: []store.BlitzEntrant{
			{ID: 7, Name: "Ann", DiscordName: "ann_tta", Zone: "UTC", Status: "active"},
			{ID: 8, Name: "Cy", DiscordName: "Cy Smith", Zone: "UTC", Status: "active"},
			{ID: 9, Name: "bob", DiscordName: "BobPlays", Zone: "UTC", Status: "active"},
			{ID: 10, Name: "Dee", Zone: "UTC", Status: "active"},
			{ID: 11, Name: "Eve", DiscordName: "eve", Zone: "UTC", Status: "active"},
		},
		format: store.BlitzFormat{SwissRounds: 8, KnockoutSize: 8},
		rounds: []store.BlitzRound{{ID: 101, Number: 1, Kind: "swiss", Method: "timezone", Seed: 1,
			Matches: []store.BlitzMatch{{ID: 1010, A: 7, B: &cy}, {ID: 1011, A: 11}, {ID: 1012, A: 9, B: &dee}}}},
	}
}

func TestAnOrganiserCopiesADraftRoundsDiscordText(t *testing.T) {
	const org = "222222222222222222"
	f := discordFixture()
	rec := adminDoAs(t, f, org, http.MethodGet, "/v1/admin/blitz/us_open/2026/rounds/101/discord", "", "")
	var got BlitzDiscordText
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET discord = %d %s, want 200", rec.Code, rec.Body)
	}
	if want := "@ann_tta vs Cy,\n@bobplays vs Dee"; got.Text != want {
		t.Errorf("text = %q, want %q (Eve's bye left out)", got.Text, want)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if want := []string{org + " blitz.round.discord 101"}; !slices.Equal(f.audits, want) {
		t.Errorf("audits = %q, want %q", f.audits, want)
	}
}

func TestAnOrganiserSetsARoundsDeadlineAndTheEditionShowsIt(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := discordFixture()
	rec := adminDoAs(t, f, org, http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/deadline", site, `{"day": "2026-10-19", "time": "13:00", "zone": "Europe/London"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT deadline = %d %s, want 204", rec.Code, rec.Body)
	}
	if want := []string{org + " blitz.round.deadline 101"}; !slices.Equal(f.audits, want) {
		t.Errorf("audits = %q, want %q", f.audits, want)
	}
	rec = adminDo(t, f, http.MethodGet, "/v1/admin/blitz/us_open/2026", "", "")
	wantJSON(t, rec.Body.Bytes(), "rounds.0.deadline", `{"at": "2026-10-19T12:00:00Z", "zone": "Europe/London"}`)
}

func TestDiscordTextIsRefusedWhenItShouldBe(t *testing.T) {
	tests := []struct {
		name, path string
		change     func(*fakeAdmins)
		want       int
		says       string
	}{
		{"only a bye", "/v1/admin/blitz/us_open/2026/rounds/101/discord", func(f *fakeAdmins) {
			f.rounds[0].Matches = []store.BlitzMatch{{ID: 1011, A: 11}}
		}, http.StatusConflict, "no game"},
		{"an unknown round", "/v1/admin/blitz/us_open/2026/rounds/999/discord", func(*fakeAdmins) {}, http.StatusNotFound, "no such round"},
		{"an unknown edition", "/v1/admin/blitz/us_open/2027/rounds/101/discord", func(*fakeAdmins) {}, http.StatusNotFound, "no such blitz edition"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := discordFixture()
			tt.change(f)
			rec := adminDo(t, f, http.MethodGet, tt.path, "", "")
			if rec.Code != tt.want || !strings.Contains(rec.Body.String(), tt.says) {
				t.Errorf("GET %s = %d %s, want %d saying %q", tt.path, rec.Code, rec.Body, tt.want, tt.says)
			}
			if len(f.audits) != 0 {
				t.Errorf("audits = %q, want none for a refusal", f.audits)
			}
		})
	}
}

func TestDiscordTextNeedsASignedInAdmin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/blitz/us_open/2026/rounds/101/discord", nil)
	rec := httptest.NewRecorder()
	newMux(&fakeRatings{}, discordFixture(), slog.New(slog.DiscardHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("GET discord without a token = %d %s, want 401", rec.Code, rec.Body)
	}
}

func TestABadDeadlineIsRefused(t *testing.T) {
	tests := []struct {
		name, path, body string
		want             int
	}{
		{"a zone that is none", "/v1/admin/blitz/us_open/2026/rounds/101/deadline", `{"day": "2026-10-19", "time": "12:00", "zone": "Mars/Olympus"}`, http.StatusBadRequest},
		{"a time that is none", "/v1/admin/blitz/us_open/2026/rounds/101/deadline", `{"day": "2026-10-19", "time": "25:00", "zone": "UTC"}`, http.StatusBadRequest},
		{"an unknown round", "/v1/admin/blitz/us_open/2026/rounds/999/deadline", `{"day": "2026-10-19", "time": "12:00", "zone": "UTC"}`, http.StatusNotFound},
		{"an unknown edition", "/v1/admin/blitz/us_open/2027/rounds/101/deadline", `{"day": "2026-10-19", "time": "12:00", "zone": "UTC"}`, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := discordFixture()
			rec := adminDo(t, f, http.MethodPut, tt.path, "https://ttac-admin.example", tt.body)
			if rec.Code != tt.want {
				t.Errorf("PUT %s %s = %d %s, want %d", tt.path, tt.body, rec.Code, rec.Body, tt.want)
			}
			if len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused deadline still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
}

func TestAFinishedRoundsDeadlineIsReadOnly(t *testing.T) {
	f := discordFixture()
	for i := range f.rounds[0].Matches {
		if f.rounds[0].Matches[i].B != nil {
			f.rounds[0].Matches[i].Result = "a"
		}
	}
	rec := adminDo(t, f, http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/deadline", "https://ttac-admin.example", `{"day": "2026-10-19", "time": "12:00", "zone": "UTC"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "finished") {
		t.Errorf("PUT a finished round's deadline = %d %s, want 409 saying it is finished", rec.Code, rec.Body)
	}
	if len(f.notes)+len(f.audits) != 0 {
		t.Errorf("a refused deadline still changed something: %q %q", f.notes, f.audits)
	}
}
