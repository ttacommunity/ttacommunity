package httpapi

import (
	"context"
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

func (f *fakeAdmins) AddBlitzOrganiser(_ context.Context, slug, edition string, member int64) error {
	if slug != "us_open" || edition != "2026" {
		return store.ErrNoBlitzEdition
	}
	m, err := f.findMember(member)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(f.organisers, func(o store.BlitzOrganiser) bool { return o.MemberID == member }) {
		f.organisers = append(f.organisers, store.BlitzOrganiser{MemberID: m.ID, Name: m.Name, DiscordName: m.DiscordName})
	}
	f.notes = append(f.notes, fmt.Sprintf("organiser %d", member))
	return nil
}

func (f *fakeAdmins) RemoveBlitzOrganiser(_ context.Context, slug, edition string, member int64) error {
	if slug != "us_open" || edition != "2026" {
		return store.ErrNoBlitzEdition
	}
	i := slices.IndexFunc(f.organisers, func(o store.BlitzOrganiser) bool { return o.MemberID == member })
	if i < 0 {
		return store.ErrNoBlitzOrganiser
	}
	f.organisers = slices.Delete(f.organisers, i, i+1)
	f.notes = append(f.notes, fmt.Sprintf("not organiser %d", member))
	return nil
}

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

// threadsFixture is us_open 2026 with Ann (ann_tta), Cy (a display name), bob (BobPlays), Dee (none) and Eve, round
// 1 a draft pairing Ann with Cy, Eve with the bye and bob with Dee, and the member Carol (carol_tta).
func threadsFixture() *fakeAdmins {
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
		rounds: []store.BlitzRound{{ID: 101, Number: 1, Kind: "swiss", Seed: 1,
			Matches: []store.BlitzMatch{{ID: 1010, A: 7, B: &cy}, {ID: 1011, A: 11}, {ID: 1012, A: 9, B: &dee}}}},
		members: []store.Member{{ID: 21, Name: "Carol", DiscordName: "carol_tta", Zone: "UTC"}},
	}
}

func TestAnOrganiserDownloadsADraftRoundsThreadsOnceItHasADeadlineAndOrganisers(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := threadsFixture()
	do := func(method, path, body string) *httptest.ResponseRecorder {
		return adminDoAs(t, f, org, method, path, site, body)
	}
	for _, step := range []struct{ method, path, body string }{
		{http.MethodPost, "/v1/admin/blitz/us_open/2026/organisers", `{"member": 21}`},
		{http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/deadline", `{"day": "2026-10-19", "time": "13:00", "zone": "Europe/London"}`},
	} {
		if rec := do(step.method, step.path, step.body); rec.Code != http.StatusNoContent {
			t.Fatalf("%s %s = %d %s, want 204", step.method, step.path, rec.Code, rec.Body)
		}
	}
	rec := do(http.MethodGet, "/v1/admin/blitz/us_open/2026/rounds/101/threads", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET threads = %d %s, want 200", rec.Code, rec.Body)
	}
	if got, want := rec.Header().Get("Content-Disposition"), `attachment; filename="us_open-2026-round-1-threads.txt"`; got != want {
		t.Errorf("Content-Disposition = %q, want %q", got, want)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Round 1 · Game 1 · Ann vs Cy\nWelcome @ann_tta, @??? (Cy). ",
		"Round 1 · Game 3 · bob vs Dee\nWelcome @bobplays, @??? (Dee). ",
		"by <t:1792411200:F>, then",
		"please ping the moderator Carol (@carol_tta).\n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("threads =\n%s\nwant them to hold %q", body, want)
		}
	}
	if strings.Contains(body, "Eve") {
		t.Errorf("threads =\n%s\nwant no thread for Eve's bye", body)
	}
	want := []string{org + " blitz.organiser.add us_open/2026", org + " blitz.round.deadline 101", org + " blitz.round.threads 101"}
	if !slices.Equal(f.audits, want) {
		t.Errorf("audits = %q, want %q", f.audits, want)
	}
}

func TestTheEditionShowsItsOrganisersAndEachRoundsDeadline(t *testing.T) {
	f := threadsFixture()
	f.organisers = []store.BlitzOrganiser{{MemberID: 21, Name: "Carol", DiscordName: "carol_tta"}}
	f.rounds[0].Deadline, f.rounds[0].DeadlineZone = at("2026-10-19T12:00:00Z"), "Europe/London"
	rec := adminDo(t, f, http.MethodGet, "/v1/admin/blitz/us_open/2026", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET edition = %d %s, want 200", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "organisers", `[{"member": 21, "name": "Carol", "discord_name": "carol_tta"}]`)
	wantJSON(t, rec.Body.Bytes(), "rounds.0.deadline", `{"at": "2026-10-19T12:00:00Z", "zone": "Europe/London"}`)
}

func TestThreadsAreRefusedUntilTheRoundCanHaveThem(t *testing.T) {
	ready := func(f *fakeAdmins) {
		f.organisers = []store.BlitzOrganiser{{MemberID: 21, Name: "Carol", DiscordName: "carol_tta"}}
		f.rounds[0].Deadline, f.rounds[0].DeadlineZone = at("2026-10-19T12:00:00Z"), "UTC"
	}
	tests := []struct {
		name, path string
		change     func(*fakeAdmins)
		want       int
		says       string
	}{
		{"no deadline", "/v1/admin/blitz/us_open/2026/rounds/101/threads", func(f *fakeAdmins) { ready(f); f.rounds[0].Deadline = nil }, http.StatusConflict, "deadline"},
		{"no organiser", "/v1/admin/blitz/us_open/2026/rounds/101/threads", func(f *fakeAdmins) { ready(f); f.organisers = nil }, http.StatusConflict, "organiser"},
		{"only a bye", "/v1/admin/blitz/us_open/2026/rounds/101/threads", func(f *fakeAdmins) {
			ready(f)
			f.rounds[0].Matches = []store.BlitzMatch{{ID: 1011, A: 11}}
		}, http.StatusConflict, "no game"},
		{"an unknown round", "/v1/admin/blitz/us_open/2026/rounds/999/threads", ready, http.StatusNotFound, "no such round"},
		{"an unknown edition", "/v1/admin/blitz/us_open/2027/rounds/101/threads", ready, http.StatusNotFound, "no such blitz edition"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := threadsFixture()
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

func TestThreadsNeedASignedInAdmin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/blitz/us_open/2026/rounds/101/threads", nil)
	rec := httptest.NewRecorder()
	newMux(&fakeRatings{}, threadsFixture(), slog.New(slog.DiscardHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("GET threads without a token = %d %s, want 401", rec.Code, rec.Body)
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
			f := threadsFixture()
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

func TestOrganisersAreAddedAndRemoved(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := threadsFixture()
	steps := []struct {
		method, path, body string
		want               int
	}{
		{http.MethodPost, "/v1/admin/blitz/us_open/2026/organisers", `{"member": 99}`, http.StatusNotFound},
		{http.MethodPost, "/v1/admin/blitz/us_open/2026/organisers", `{"member": 21}`, http.StatusNoContent},
		{http.MethodDelete, "/v1/admin/blitz/us_open/2026/organisers/21", "", http.StatusNoContent},
		{http.MethodDelete, "/v1/admin/blitz/us_open/2026/organisers/21", "", http.StatusNotFound},
	}
	for _, s := range steps {
		if rec := adminDoAs(t, f, org, s.method, s.path, site, s.body); rec.Code != s.want {
			t.Errorf("%s %s %s = %d %s, want %d", s.method, s.path, s.body, rec.Code, rec.Body, s.want)
		}
	}
	if want := "organiser 21|not organiser 21"; strings.Join(f.notes, "|") != want {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	want := []string{org + " blitz.organiser.add us_open/2026", org + " blitz.organiser.remove us_open/2026"}
	if !slices.Equal(f.audits, want) {
		t.Errorf("audits = %q, want %q", f.audits, want)
	}
}

func TestAFinishedRoundsDeadlineIsReadOnly(t *testing.T) {
	f := threadsFixture()
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
