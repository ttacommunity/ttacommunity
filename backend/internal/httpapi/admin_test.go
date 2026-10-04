package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/access"
	"ttacommunity/backend/internal/enginejob"
	"ttacommunity/backend/internal/store"
)

// fakeAdmins accepts the token "good-<id>" for Discord id <id>, and knows one admin and one organiser.
// It records each tournament change and audit, and knows one tournament, worlds.
type fakeAdmins struct {
	settings []store.TournamentSettings
	notes    []string
	audits   []string
	startErr error
	stopErr  error
	// engine is what EngineGoing answers: "" for a run going, "ended" for none, "unset" for not set up,
	// "unreachable" for an error.
	engine string
}

// fakeRuns are runs 4 (complete, published third, after a roll back), 5 (complete, published first),
// 6 (complete, never published), 7 (complete, published second, current), 8 (complete) and 9 (running).
var fakeRuns = []store.AdminRun{
	{ID: 9, Status: "running", Progress: "[3/10] models 2p"},
	{ID: 8, Status: "complete", Games: 100},
	{ID: 7, Status: "complete", Games: 90, Current: true, PublishedAt: at("2026-10-02T10:00:00Z")},
	{ID: 6, Status: "complete"},
	{ID: 5, Status: "complete", PublishedAt: at("2026-10-01T10:00:00Z")},
	{ID: 4, Status: "complete", PublishedAt: at("2026-10-03T10:00:00Z")},
}

func at(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func (*fakeAdmins) Runs(context.Context) ([]store.AdminRun, error) { return fakeRuns, nil }

func (f *fakeAdmins) Publish(_ context.Context, run int64) error {
	switch run {
	case 7, 8:
		f.notes = append(f.notes, fmt.Sprintf("publish %d", run))
		return nil
	case 9:
		return store.ErrRunNotComplete
	}
	return store.ErrRunNotFound
}

func (*fakeAdmins) RunChanges(_ context.Context, run, before int64) (store.RunChanges, error) {
	if run < 4 || run > 9 || before < 4 || before > 9 {
		return store.RunChanges{}, store.ErrRunNotFound
	}
	return store.RunChanges{Ratings: int(run), RatingsBefore: int(before), Biggest: []store.RunMove{{Name: "dan", Before: 1500, After: 1380, Change: -120}}}, nil
}

func (f *fakeAdmins) StopRun(context.Context) ([]string, error) {
	if f.stopErr != nil {
		return nil, f.stopErr
	}
	f.notes = append(f.notes, "stop")
	return []string{"engine-dev-admin-x7k2p"}, nil
}

func (f *fakeAdmins) EngineGoing(context.Context) (bool, error) {
	switch f.engine {
	case "ended":
		return false, nil
	case "unset":
		return false, enginejob.ErrNotSetUp
	case "unreachable":
		return false, errors.New("GET /apis/batch/v1/namespaces/ttac/jobs: dial tcp: i/o timeout")
	}
	return true, nil
}

func (f *fakeAdmins) StartRun(context.Context) (string, error) {
	if f.startErr != nil {
		return "", f.startErr
	}
	f.notes = append(f.notes, "start")
	return "engine-dev-admin-x7k2p", nil
}

func (*fakeAdmins) AllowedOrigin() string { return "https://ttac-admin.example" }

func (*fakeAdmins) Tournaments(context.Context) ([]store.AdminTournament, error) {
	return []store.AdminTournament{{Slug: "worlds", Name: "World Championship", Rated: true, Regulations: "Mine.", CGERegulations: "CGE's.",
		Card:              store.Card{Icon: "🏆", Tagline: "Annual World Championship", Summary: "The pinnacle."},
		Stories:           []store.Story{{Player: "a440", Badge: "Reigning World Champion", Title: "2025", Text: "Won."}},
		RatedPlayerCounts: []int{3, 4},
		Seasons: []store.AdminSeason{{Label: "2025", Rated: true, Stages: []store.AdminStage{{Label: "Stage 1", Note: "n"}},
			Podium: []store.PodiumEntry{{Place: 1, Name: "a440"}}, PodiumNote: "CGE"}}}}, nil
}

func (f *fakeAdmins) SetTournament(_ context.Context, slug string, s store.TournamentSettings) error {
	if slug != "worlds" {
		return store.ErrNoTournament
	}
	if slices.Contains(s.RatedPlayerCounts, 5) {
		return store.ErrBadPlayerCounts
	}
	f.settings = append(f.settings, s)
	return nil
}

func (f *fakeAdmins) SetSeasonRated(_ context.Context, slug, season string, rated bool) error {
	if slug != "worlds" || season == "1999" {
		return store.ErrNoTournament
	}
	f.notes = append(f.notes, fmt.Sprintf("season %s rated %v", season, rated))
	return nil
}

func (f *fakeAdmins) Admins(context.Context) ([]store.Admin, error) {
	return []store.Admin{{DiscordID: "111111111111111111", Name: "Owner", Role: store.RoleAdmin}}, nil
}

func (f *fakeAdmins) SetAdmin(_ context.Context, a store.Admin) error {
	if a.DiscordID == "111111111111111111" && a.Role != store.RoleAdmin {
		return store.ErrLastAdmin
	}
	f.notes = append(f.notes, "set "+a.DiscordID+" "+a.Name+" "+a.Role)
	return nil
}

func (f *fakeAdmins) RemoveAdmin(_ context.Context, id string) error {
	if id == "999999999999999999" {
		return store.ErrNotAdmin
	}
	if id == "111111111111111111" {
		return store.ErrLastAdmin
	}
	f.notes = append(f.notes, "remove "+id)
	return nil
}

func (f *fakeAdmins) AddSeason(_ context.Context, slug, label string) error {
	switch {
	case slug != "worlds":
		return store.ErrNoTournament
	case label == "2025":
		return store.ErrSeasonExists
	}
	f.notes = append(f.notes, "add season "+label)
	return nil
}

func (f *fakeAdmins) RemoveSeason(_ context.Context, slug, label string) error {
	switch {
	case slug != "worlds" || label == "1999":
		return store.ErrNoTournament
	case label == "2025":
		return store.ErrSeasonHasGames
	}
	f.notes = append(f.notes, "remove season "+label)
	return nil
}

func (f *fakeAdmins) SetPodium(_ context.Context, slug, season string, podium []store.PodiumEntry, note string) error {
	switch {
	case slug != "worlds" || season == "1999":
		return store.ErrNoTournament
	case len(podium) > 0 && podium[0].Name == "twice":
		return store.ErrBadPodium
	}
	f.notes = append(f.notes, fmt.Sprintf("podium %s %v %s", season, podium, note))
	return nil
}

func (*fakeAdmins) ExportZip(_ context.Context, w io.Writer) error {
	_, err := io.WriteString(w, "PK zip bytes")
	return err
}

func (*fakeAdmins) DataSummary(context.Context) (store.DataSummary, error) {
	latest := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	return store.DataSummary{Games: 144691, NeverPlayed: 36, Unplaced: 0, Players: 5210, LatestGame: &latest}, nil
}

func (*fakeAdmins) Badges(context.Context) ([]store.Badge, error) {
	return []store.Badge{{ID: 3, Label: "Organiser", Description: "Runs a league", Players: []string{"Ann"}}}, nil
}

func (f *fakeAdmins) AddBadge(_ context.Context, b store.Badge) (int64, error) {
	if b.Label == "Organiser" {
		return 0, store.ErrBadgeExists
	}
	f.notes = append(f.notes, "add badge "+b.Label)
	return 4, nil
}

func (f *fakeAdmins) SetBadge(_ context.Context, b store.Badge) error {
	if b.ID != 3 {
		return store.ErrNoBadge
	}
	f.notes = append(f.notes, fmt.Sprintf("set badge %d %s", b.ID, b.Label))
	return nil
}

func (f *fakeAdmins) RemoveBadge(_ context.Context, id int64) error {
	if id != 3 {
		return store.ErrNoBadge
	}
	f.notes = append(f.notes, fmt.Sprintf("remove badge %d", id))
	return nil
}

func (f *fakeAdmins) GiveBadge(_ context.Context, id int64, name string) error {
	switch {
	case id != 3:
		return store.ErrNoBadge
	case name == "Nobody":
		return store.ErrNoPlayer
	}
	f.notes = append(f.notes, fmt.Sprintf("give %d %s", id, name))
	return nil
}

func (f *fakeAdmins) TakeBadge(_ context.Context, id int64, name string) error {
	if id != 3 || name != "Ann" {
		return store.ErrNoBadge
	}
	f.notes = append(f.notes, fmt.Sprintf("take %d %s", id, name))
	return nil
}

func (*fakeAdmins) TitleOverrides(context.Context) ([]store.TitleOverride, error) {
	return []store.TitleOverride{{Name: "Ann", Title: "SGM", Reason: "won 2019"}}, nil
}

func (f *fakeAdmins) SetTitle(_ context.Context, o store.TitleOverride) error {
	if o.Name == "Nobody" {
		return store.ErrNoPlayer
	}
	if o.Title == "WC" {
		return store.ErrBadTitle
	}
	f.notes = append(f.notes, "title "+o.Name+" "+o.Title+" "+o.Reason)
	return nil
}

func (f *fakeAdmins) ClearTitle(_ context.Context, name string) error {
	if name != "Ann" {
		return store.ErrNoTitle
	}
	f.notes = append(f.notes, "clear title "+name)
	return nil
}

func (f *fakeAdmins) SetStories(_ context.Context, slug string, list []store.Story) error {
	if slug != "worlds" {
		return store.ErrNoTournament
	}
	if len(list) > 0 && list[0].Title == "" {
		return store.ErrBadStory
	}
	for _, st := range list {
		f.notes = append(f.notes, "story "+st.Player+"|"+st.Title)
	}
	return nil
}

func (f *fakeAdmins) SetCard(_ context.Context, slug string, c store.Card) error {
	if slug != "worlds" {
		return store.ErrNoTournament
	}
	if c.Icon == "toolong" {
		return store.ErrBadCard
	}
	f.notes = append(f.notes, "card "+c.Icon+"|"+c.Tagline+"|"+c.Summary)
	return nil
}

func (f *fakeAdmins) SetRegulations(_ context.Context, slug, text string) error {
	if slug != "worlds" {
		return store.ErrNoTournament
	}
	f.notes = append(f.notes, "regulations "+text)
	return nil
}

func (f *fakeAdmins) ClearPodium(_ context.Context, slug, season string) error {
	if slug != "worlds" || season == "1999" {
		return store.ErrNoTournament
	}
	f.notes = append(f.notes, "clear podium "+season)
	return nil
}

func (f *fakeAdmins) SetNote(_ context.Context, slug, season, stage, note string) error {
	if slug != "worlds" {
		return store.ErrNoTournament
	}
	f.notes = append(f.notes, season+"|"+stage+"|"+note)
	return nil
}

func (*fakeAdmins) Verify(_ context.Context, token string) (access.Identity, error) {
	if len(token) > 5 && token[:5] == "good-" {
		return access.Identity{DiscordID: token[5:]}, nil
	}
	return access.Identity{}, errors.New("bad token")
}

func (*fakeAdmins) Admin(_ context.Context, id string) (store.Admin, error) {
	switch id {
	case "111111111111111111":
		return store.Admin{DiscordID: id, Name: "Owner", Role: store.RoleAdmin}, nil
	case "222222222222222222":
		return store.Admin{DiscordID: id, Name: "Org", Role: store.RoleOrganiser}, nil
	}
	return store.Admin{}, store.ErrNotAdmin
}

func (f *fakeAdmins) Audit(_ context.Context, id, action, target string, _ any) error {
	f.audits = append(f.audits, id+" "+action+" "+target)
	return nil
}

func adminGet(t *testing.T, admins Admins, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/me", nil)
	if token != "" {
		req.Header.Set("Cf-Access-Jwt-Assertion", token)
	}
	rec := httptest.NewRecorder()
	NewMux(&fakeRatings{}, admins, slog.New(slog.DiscardHandler)).ServeHTTP(rec, req)
	return rec
}

func TestAdminMeNamesTheSignedInAdminAndTheirRole(t *testing.T) {
	for _, tt := range []struct{ id, name, role string }{
		{"111111111111111111", "Owner", "admin"},
		{"222222222222222222", "Org", "organiser"},
	} {
		rec := adminGet(t, &fakeAdmins{}, "good-"+tt.id)
		var got AdminMe
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("GET /v1/admin/me as %s = %d %s", tt.name, rec.Code, rec.Body)
		}
		if got.Name != tt.name || string(got.Role) != tt.role {
			t.Errorf("me = %+v, want %s as %s", got, tt.name, tt.role)
		}
		if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("headers = %v, want no-store and no CORS", rec.Header())
		}
	}
}

func TestAdminRoutesRefuseWithoutAVerifiedAdmin(t *testing.T) {
	tests := []struct {
		name   string
		admins Admins
		token  string
		want   int
	}{
		{"no token", &fakeAdmins{}, "", http.StatusUnauthorized},
		{"a token that does not verify", &fakeAdmins{}, "forged", http.StatusUnauthorized},
		{"a valid token for someone not in admin", &fakeAdmins{}, "good-333333333333333333", http.StatusForbidden},
		{"admin not configured", nil, "good-111111111111111111", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := adminGet(t, tt.admins, tt.token)
			if rec.Code != tt.want {
				t.Errorf("GET /v1/admin/me with %s = %d, want %d", tt.name, rec.Code, tt.want)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
			}
		})
	}
}

func TestNoPathReachesAnAdminRouteAroundTheGate(t *testing.T) {
	for _, path := range []string{"/v1/./admin/me", "/v1//admin/me", "/v1/x/../admin/me", "/v1/%61dmin/me", "/v1/admin/me/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		NewMux(&fakeRatings{}, &fakeAdmins{}, slog.New(slog.DiscardHandler)).ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			t.Errorf("GET %s without a token = 200 %s, want a refusal", path, rec.Body)
		}
	}
}

func adminDo(t *testing.T, admins Admins, method, path, origin, body string) *httptest.ResponseRecorder {
	t.Helper()
	return adminDoAs(t, admins, "222222222222222222", method, path, origin, body)
}

func adminDoAs(t *testing.T, admins Admins, who, method, path, origin, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Cf-Access-Jwt-Assertion", "good-"+who)
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	NewMux(&fakeRatings{}, admins, slog.New(slog.DiscardHandler)).ServeHTTP(rec, req)
	return rec
}

func TestAdminTournamentsListsSettingsAndNotes(t *testing.T) {
	rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/tournaments", "", "")
	var got []AdminTournament
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/admin/tournaments = %d %s", rec.Code, rec.Body)
	}
	if len(got) != 1 || got[0].Slug != "worlds" || !got[0].Rated || got[0].Seasons[0].Stages[0].Note != "n" {
		t.Errorf("tournaments = %+v", got)
	}
}

func TestAnOrganiserChangesATournamentAndItIsAudited(t *testing.T) {
	f := &fakeAdmins{}
	rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds", "https://ttac-admin.example", `{"community": true}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH = %d %s, want 204", rec.Code, rec.Body)
	}
	if len(f.settings) != 1 || f.settings[0].Community == nil || !*f.settings[0].Community || f.settings[0].Visible != nil {
		t.Errorf("settings = %+v, want only community on", f.settings)
	}
	if len(f.audits) != 1 || f.audits[0] != "222222222222222222 tournament.settings worlds" {
		t.Errorf("audits = %q", f.audits)
	}

	rec = adminDo(t, f, http.MethodPut, "/v1/admin/tournaments/worlds/notes", "https://ttac-admin.example",
		`{"season": "2025", "stage": "Stage 1", "note": "The groups."}`)
	if rec.Code != http.StatusNoContent || len(f.notes) != 1 || f.notes[0] != "2025|Stage 1|The groups." || len(f.audits) != 2 {
		t.Errorf("PUT note = %d, notes %q, audits %q", rec.Code, f.notes, f.audits)
	}
}

func TestAdminChangesRefuseAnotherOriginOrABadRequest(t *testing.T) {
	tests := []struct {
		name, method, path, origin, body string
		want                             int
	}{
		{"no Origin", http.MethodPatch, "/v1/admin/tournaments/worlds", "", `{"visible": true}`, http.StatusForbidden},
		{"another site", http.MethodPatch, "/v1/admin/tournaments/worlds", "https://evil.example", `{"visible": true}`, http.StatusForbidden},
		{"an unknown tournament", http.MethodPatch, "/v1/admin/tournaments/nope", "https://ttac-admin.example", `{"visible": true}`, http.StatusNotFound},
		{"an unknown setting", http.MethodPatch, "/v1/admin/tournaments/worlds", "https://ttac-admin.example", `{"deleted": true}`, http.StatusBadRequest},
		{"a note without a season", http.MethodPut, "/v1/admin/tournaments/worlds/notes", "https://ttac-admin.example", `{"season": "", "stage": "", "note": "x"}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			rec := adminDo(t, f, tt.method, tt.path, tt.origin, tt.body)
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.settings)+len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %+v %q %q", f.settings, f.notes, f.audits)
			}
		})
	}
}

func TestAnAdminManagesOthersInAdmin(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	f := &fakeAdmins{}
	rec := adminDoAs(t, f, owner, http.MethodGet, "/v1/admin/admins", "", "")
	var got []AdminPerson
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK || len(got) != 1 || got[0].Name != "Owner" {
		t.Fatalf("GET admins = %d %s", rec.Code, rec.Body)
	}
	if rec := adminDoAs(t, f, owner, http.MethodPut, "/v1/admin/admins/100000000000000002", site, `{"name": "DANeo", "role": "organiser"}`); rec.Code != http.StatusNoContent {
		t.Errorf("PUT another = %d %s, want 204", rec.Code, rec.Body)
	}
	if rec := adminDoAs(t, f, owner, http.MethodDelete, "/v1/admin/admins/100000000000000002", site, ""); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE another = %d %s, want 204", rec.Code, rec.Body)
	}
	if want := []string{"set 100000000000000002 DANeo organiser", "remove 100000000000000002"}; strings.Join(f.notes, "|") != strings.Join(want, "|") || len(f.audits) != 2 {
		t.Errorf("changes = %q, audits %q", f.notes, f.audits)
	}
}

func TestAdminListChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site, owner, org = "https://ttac-admin.example", "111111111111111111", "222222222222222222"
	tests := []struct {
		name, who, method, path, body string
		want                          int
	}{
		{"an organiser lists", org, http.MethodGet, "/v1/admin/admins", "", http.StatusForbidden},
		{"an organiser promotes themselves", org, http.MethodPut, "/v1/admin/admins/" + org, `{"name": "Org", "role": "admin"}`, http.StatusForbidden},
		{"an organiser adds", org, http.MethodPut, "/v1/admin/admins/333333333333333333", `{"name": "X", "role": "admin"}`, http.StatusForbidden},
		{"the last admin demotes themselves", owner, http.MethodPut, "/v1/admin/admins/" + owner, `{"name": "Owner", "role": "organiser"}`, http.StatusForbidden},
		{"the last admin removes themselves", owner, http.MethodDelete, "/v1/admin/admins/" + owner, "", http.StatusForbidden},
		{"no name", owner, http.MethodPut, "/v1/admin/admins/333333333333333333", `{"name": "", "role": "admin"}`, http.StatusBadRequest},
		{"an unknown role", owner, http.MethodPut, "/v1/admin/admins/333333333333333333", `{"name": "X", "role": "root"}`, http.StatusBadRequest},
		{"someone not in admin", owner, http.MethodDelete, "/v1/admin/admins/999999999999999999", "", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			rec := adminDoAs(t, f, tt.who, tt.method, tt.path, site, tt.body)
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.audits) != 0 || (tt.want != http.StatusNotFound && len(f.notes) != 0) {
				t.Errorf("a refused change still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
}

func TestAnAdminRenamesThemselves(t *testing.T) {
	f := &fakeAdmins{}
	rec := adminDoAs(t, f, "111111111111111111", http.MethodPut, "/v1/admin/admins/111111111111111111", "https://ttac-admin.example",
		`{"name": "Owner, renamed", "role": "admin"}`)
	if rec.Code != http.StatusNoContent || len(f.audits) != 1 {
		t.Errorf("PUT self = %d %s, audits %q; want 204 and audited", rec.Code, rec.Body, f.audits)
	}
}

func TestAdminAddsAndRemovesASeasonWithoutGames(t *testing.T) {
	const site = "https://ttac-admin.example"
	tests := []struct {
		name, method, path, body string
		want                     int
		change                   string
	}{
		{"add", http.MethodPost, "/v1/admin/tournaments/worlds/seasons", `{"label": "2021"}`, http.StatusNoContent, "add season 2021"},
		{"add one that exists", http.MethodPost, "/v1/admin/tournaments/worlds/seasons", `{"label": "2025"}`, http.StatusBadRequest, ""},
		{"add an empty label", http.MethodPost, "/v1/admin/tournaments/worlds/seasons", `{"label": " "}`, http.StatusBadRequest, ""},
		{"add to no tournament", http.MethodPost, "/v1/admin/tournaments/nope/seasons", `{"label": "1"}`, http.StatusNotFound, ""},
		{"remove", http.MethodDelete, "/v1/admin/tournaments/worlds/seasons?label=2023+Play+Off", "", http.StatusNoContent, "remove season 2023 Play Off"},
		{"remove one with games", http.MethodDelete, "/v1/admin/tournaments/worlds/seasons?label=2025", "", http.StatusBadRequest, ""},
		{"remove one that does not exist", http.MethodDelete, "/v1/admin/tournaments/worlds/seasons?label=1999", "", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			rec := adminDo(t, f, tt.method, tt.path, site, tt.body)
			if rec.Code != tt.want {
				t.Fatalf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			changed := strings.Join(f.notes, "|")
			if changed != tt.change || (tt.change != "") != (len(f.audits) == 1) {
				t.Errorf("changes %q, audits %q; want %q, audited when changed", changed, f.audits, tt.change)
			}
		})
	}
}

func TestAnAdminListsRuns(t *testing.T) {
	rec := adminDoAs(t, &fakeAdmins{}, "111111111111111111", http.MethodGet, "/v1/admin/runs", "", "")
	var got []AdminRun
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET runs = %d %s", rec.Code, rec.Body)
	}
	if len(got) != 6 || got[0].Id != 9 || got[0].Progress != "[3/10] models 2p" || !got[2].Current || got[2].PublishedAt == nil || got[1].PublishedAt != nil {
		t.Errorf("runs = %+v", got)
	}
}

func TestAnAdminStartsAndPublishesRunsAndEachIsAudited(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	f := &fakeAdmins{}
	rec := adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/runs", site, "")
	var started StartedRun
	if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil || rec.Code != http.StatusAccepted || started.Job != "engine-dev-admin-x7k2p" {
		t.Fatalf("POST runs = %d %s, want 202 with the job", rec.Code, rec.Body)
	}
	if rec := adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/runs/8/publish", site, ""); rec.Code != http.StatusNoContent {
		t.Errorf("publish 8 = %d %s, want 204", rec.Code, rec.Body)
	}
	if want := "start|publish 8"; strings.Join(f.notes, "|") != want {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	if want := owner + " run.start engine-dev-admin-x7k2p|" + owner + " run.publish 8"; strings.Join(f.audits, "|") != want {
		t.Errorf("audits = %q, want %q", f.audits, want)
	}
}

func TestRunChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site, owner, org = "https://ttac-admin.example", "111111111111111111", "222222222222222222"
	tests := []struct {
		name, who, method, path, origin string
		startErr                        error // also what a stop returns
		want                            int
	}{
		{"an organiser lists runs", org, http.MethodGet, "/v1/admin/runs", "", nil, http.StatusForbidden},
		{"an organiser starts a run", org, http.MethodPost, "/v1/admin/runs", site, nil, http.StatusForbidden},
		{"an organiser publishes", org, http.MethodPost, "/v1/admin/runs/8/publish", site, nil, http.StatusForbidden},
		{"an organiser compares", org, http.MethodGet, "/v1/admin/runs/8/changes", "", nil, http.StatusForbidden},
		{"a start from another site", owner, http.MethodPost, "/v1/admin/runs", "https://evil.example", nil, http.StatusForbidden},
		{"a start while a run is going", owner, http.MethodPost, "/v1/admin/runs", site, enginejob.ErrRunning, http.StatusConflict},
		{"a start where runs are not set up", owner, http.MethodPost, "/v1/admin/runs", site, enginejob.ErrNotSetUp, http.StatusServiceUnavailable},
		{"publishing a running run", owner, http.MethodPost, "/v1/admin/runs/9/publish", site, nil, http.StatusConflict},
		{"publishing no run", owner, http.MethodPost, "/v1/admin/runs/99/publish", site, nil, http.StatusNotFound},
		{"publishing from another site", owner, http.MethodPost, "/v1/admin/runs/8/publish", "https://evil.example", nil, http.StatusForbidden},
		{"an organiser stops a run", org, http.MethodPost, "/v1/admin/runs/stop", site, nil, http.StatusForbidden},
		{"a stop from another site", owner, http.MethodPost, "/v1/admin/runs/stop", "https://evil.example", nil, http.StatusForbidden},
		{"a stop with nothing going", owner, http.MethodPost, "/v1/admin/runs/stop", site, enginejob.ErrNotRunning, http.StatusConflict},
		{"a stop where runs are not set up", owner, http.MethodPost, "/v1/admin/runs/stop", site, enginejob.ErrNotSetUp, http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{startErr: tt.startErr, stopErr: tt.startErr}
			rec := adminDoAs(t, f, tt.who, tt.method, tt.path, tt.origin, "")
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
}

func TestRunChangesCompareWithThePublishedRunUnlessAsked(t *testing.T) {
	const owner = "111111111111111111"
	tests := []struct {
		name, path   string
		want         int
		run, against int64
	}{
		{"against the published run", "/v1/admin/runs/9/changes", http.StatusOK, 9, 7},
		{"the published run against the run published before it", "/v1/admin/runs/7/changes", http.StatusOK, 7, 5},
		{"a run published later against the run published before it", "/v1/admin/runs/4/changes", http.StatusOK, 4, 7},
		{"a run never published against the published run", "/v1/admin/runs/6/changes", http.StatusOK, 6, 7},
		{"the first run published, with nothing before it", "/v1/admin/runs/5/changes", http.StatusNotFound, 0, 0},
		{"against a run asked for", "/v1/admin/runs/9/changes?against=8", http.StatusOK, 9, 8},
		{"a run that does not exist", "/v1/admin/runs/99/changes", http.StatusNotFound, 0, 0},
		{"against a run that does not exist", "/v1/admin/runs/9/changes?against=99", http.StatusNotFound, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := adminDoAs(t, &fakeAdmins{}, owner, http.MethodGet, tt.path, "", "")
			if rec.Code != tt.want {
				t.Fatalf("GET %s = %d %s, want %d", tt.path, rec.Code, rec.Body, tt.want)
			}
			if tt.want != http.StatusOK {
				return
			}
			var got RunChanges
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Run != tt.run || got.Against != tt.against || got.Ratings != int(tt.run) || got.RatingsBefore != int(tt.against) || got.Biggest[0].Change != -120 {
				t.Errorf("changes = %+v, want run %d against %d", got, tt.run, tt.against)
			}
		})
	}
}

func TestAnAdminStopsTheRunThatIsGoing(t *testing.T) {
	const owner = "111111111111111111"
	f := &fakeAdmins{}
	if rec := adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/runs/stop", "https://ttac-admin.example", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("POST stop = %d %s, want 204", rec.Code, rec.Body)
	}
	if strings.Join(f.notes, "|") != "stop" || strings.Join(f.audits, "|") != owner+" run.stop engine-dev-admin-x7k2p" {
		t.Errorf("changes %q, audits %q", f.notes, f.audits)
	}
}

func TestARunWhoseEngineEndedIsShownFailed(t *testing.T) {
	for _, tt := range []struct {
		engine, status, progress string
	}{
		{"", "running", "[3/10] models 2p"},
		{"ended", "failed", "the engine ended without finishing this run"},
		{"unset", "running", "[3/10] models 2p"},
		{"unreachable", "running", "[3/10] models 2p"},
	} {
		t.Run("engine "+tt.engine, func(t *testing.T) {
			rec := adminDoAs(t, &fakeAdmins{engine: tt.engine}, "111111111111111111", http.MethodGet, "/v1/admin/runs", "", "")
			var got []AdminRun
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
				t.Fatalf("GET runs = %d %s", rec.Code, rec.Body)
			}
			if string(got[0].Status) != tt.status || got[0].Progress != tt.progress || got[1].Status != "complete" {
				t.Errorf("runs = %+v, want run 9 %s with %q", got, tt.status, tt.progress)
			}
		})
	}
}

func TestAnOrganiserSetsAndClearsASeasonsPodium(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	rec := adminDo(t, f, http.MethodPut, "/v1/admin/tournaments/worlds/podium", site,
		`{"season": "2025", "podium": [{"place": 1, "name": "a440"}, {"place": 2, "name": "kyshantry"}], "note": "CGE final"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT podium = %d %s, want 204", rec.Code, rec.Body)
	}
	if rec := adminDo(t, f, http.MethodDelete, "/v1/admin/tournaments/worlds/podium?season=2025", site, ""); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE podium = %d %s, want 204", rec.Code, rec.Body)
	}
	if want := "podium 2025 [{1 a440} {2 kyshantry}] CGE final|clear podium 2025"; strings.Join(f.notes, "|") != want {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	if want := org + " tournament.podium worlds|" + org + " tournament.podium.clear worlds"; strings.Join(f.audits, "|") != want {
		t.Errorf("audits = %q, want %q", f.audits, want)
	}
}

func TestPodiumChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site = "https://ttac-admin.example"
	tests := []struct {
		name, method, path, origin, body string
		want                             int
	}{
		{"another site", http.MethodPut, "/v1/admin/tournaments/worlds/podium", "https://evil.example", `{"season": "2025", "podium": [{"place": 1, "name": "a"}], "note": ""}`, http.StatusForbidden},
		{"no such season", http.MethodPut, "/v1/admin/tournaments/worlds/podium", site, `{"season": "1999", "podium": [{"place": 1, "name": "a"}], "note": ""}`, http.StatusNotFound},
		{"not a podium", http.MethodPut, "/v1/admin/tournaments/worlds/podium", site, `{"season": "2025", "podium": [{"place": 1, "name": "twice"}], "note": ""}`, http.StatusBadRequest},
		{"nobody on it", http.MethodPut, "/v1/admin/tournaments/worlds/podium", site, `{"season": "2025", "podium": [], "note": ""}`, http.StatusBadRequest},
		{"a long note", http.MethodPut, "/v1/admin/tournaments/worlds/podium", site, `{"season": "2025", "podium": [{"place": 1, "name": "a"}], "note": "` + strings.Repeat("x", 501) + `"}`, http.StatusBadRequest},
		{"clearing no such season", http.MethodDelete, "/v1/admin/tournaments/worlds/podium?season=1999", site, "", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			rec := adminDo(t, f, tt.method, tt.path, tt.origin, tt.body)
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
}

func TestAdminTournamentsShowASeasonsPodium(t *testing.T) {
	rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/tournaments", "", "")
	var got []AdminTournament
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	if se := got[0].Seasons[0]; len(se.Podium) != 1 || se.Podium[0].Name != "a440" || se.PodiumNote != "CGE" {
		t.Errorf("season = %+v", se)
	}
}

func TestAnOrganiserWritesATournamentsRegulations(t *testing.T) {
	const site = "https://ttac-admin.example"
	f := &fakeAdmins{}
	if rec := adminDo(t, f, http.MethodPut, "/v1/admin/tournaments/worlds/regulations", site, `{"text": "Best of three."}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT = %d %s, want 204", rec.Code, rec.Body)
	}
	if strings.Join(f.notes, "|") != "regulations Best of three." || strings.Join(f.audits, "|") != "222222222222222222 tournament.regulations worlds" {
		t.Errorf("changes %q, audits %q", f.notes, f.audits)
	}
	for _, tt := range []struct {
		name, path, origin, body string
		want                     int
	}{
		{"another site", "/v1/admin/tournaments/worlds/regulations", "https://evil.example", `{"text": "x"}`, http.StatusForbidden},
		{"no such tournament", "/v1/admin/tournaments/nope/regulations", site, `{"text": "x"}`, http.StatusNotFound},
		{"too long", "/v1/admin/tournaments/worlds/regulations", site, `{"text": "` + strings.Repeat("x", 10001) + `"}`, http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			if rec := adminDo(t, f, http.MethodPut, tt.path, tt.origin, tt.body); rec.Code != tt.want {
				t.Errorf("PUT = %d %s, want %d", rec.Code, rec.Body, tt.want)
			}
			if len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
	rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/tournaments", "", "")
	var got []AdminTournament
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got[0].Regulations != "Mine." || got[0].CgeRegulations != "CGE's." {
		t.Errorf("GET = %s, want both regulations", rec.Body)
	}
}

func TestAnAdminSetsAndClearsAPlayersTitle(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	f := &fakeAdmins{}
	rec := adminDoAs(t, f, owner, http.MethodGet, "/v1/admin/titles", "", "")
	var got []TitleOverride
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got) != 1 || got[0].Title != "SGM" {
		t.Fatalf("GET titles = %d %s", rec.Code, rec.Body)
	}
	if rec := adminDoAs(t, f, owner, http.MethodPut, "/v1/admin/titles/Mr%20Ann", site, `{"title": "", "reason": "a false Super GM"}`); rec.Code != http.StatusNoContent {
		t.Errorf("PUT = %d %s, want 204", rec.Code, rec.Body)
	}
	if rec := adminDoAs(t, f, owner, http.MethodDelete, "/v1/admin/titles/Ann", site, ""); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE = %d %s, want 204", rec.Code, rec.Body)
	}
	if want := "title Mr Ann  a false Super GM|clear title Ann"; strings.Join(f.notes, "|") != want {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	if want := owner + " player.title Mr Ann|" + owner + " player.title.clear Ann"; strings.Join(f.audits, "|") != want {
		t.Errorf("audits = %q, want %q", f.audits, want)
	}
}

func TestTitleChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site, owner, org = "https://ttac-admin.example", "111111111111111111", "222222222222222222"
	tests := []struct {
		name, who, method, path, origin, body string
		want                                  int
	}{
		{"an organiser lists", org, http.MethodGet, "/v1/admin/titles", "", "", http.StatusForbidden},
		{"an organiser sets", org, http.MethodPut, "/v1/admin/titles/Ann", site, `{"title": "GM", "reason": "r"}`, http.StatusForbidden},
		{"an organiser clears", org, http.MethodDelete, "/v1/admin/titles/Ann", site, "", http.StatusForbidden},
		{"another site", owner, http.MethodPut, "/v1/admin/titles/Ann", "https://evil.example", `{"title": "GM", "reason": "r"}`, http.StatusForbidden},
		{"WC", owner, http.MethodPut, "/v1/admin/titles/Ann", site, `{"title": "WC", "reason": "r"}`, http.StatusBadRequest},
		{"a made-up title", owner, http.MethodPut, "/v1/admin/titles/Ann", site, `{"title": "Legend", "reason": "r"}`, http.StatusBadRequest},
		{"no reason", owner, http.MethodPut, "/v1/admin/titles/Ann", site, `{"title": "GM", "reason": ""}`, http.StatusBadRequest},
		{"a long reason", owner, http.MethodPut, "/v1/admin/titles/Ann", site, `{"title": "GM", "reason": "` + strings.Repeat("x", 301) + `"}`, http.StatusBadRequest},
		{"no such player", owner, http.MethodPut, "/v1/admin/titles/Nobody", site, `{"title": "GM", "reason": "r"}`, http.StatusNotFound},
		{"clearing a title never set", owner, http.MethodDelete, "/v1/admin/titles/Bob", site, "", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			rec := adminDoAs(t, f, tt.who, tt.method, tt.path, tt.origin, tt.body)
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
}

func TestAnAdminMakesGivesTakesAndRemovesBadges(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	f := &fakeAdmins{}
	rec := adminDoAs(t, f, owner, http.MethodGet, "/v1/admin/badges", "", "")
	var list []AdminBadge
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 || list[0].Players[0] != "Ann" {
		t.Fatalf("GET badges = %d %s", rec.Code, rec.Body)
	}
	rec = adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/badges", site, `{"label": "TD", "description": "Tournament director"}`)
	var made AdminBadge
	if err := json.Unmarshal(rec.Body.Bytes(), &made); err != nil || rec.Code != http.StatusCreated || made.Id != 4 || made.Label != "TD" {
		t.Fatalf("POST badge = %d %s, want 201 with id 4", rec.Code, rec.Body)
	}
	for _, step := range []struct{ method, path, body string }{
		{http.MethodPut, "/v1/admin/badges/3", `{"label": "Org", "description": ""}`},
		{http.MethodPut, "/v1/admin/badges/3/players/Bob", ""},
		{http.MethodDelete, "/v1/admin/badges/3/players/Ann", ""},
		{http.MethodDelete, "/v1/admin/badges/3", ""},
	} {
		if rec := adminDoAs(t, f, owner, step.method, step.path, site, step.body); rec.Code != http.StatusNoContent {
			t.Errorf("%s %s = %d %s, want 204", step.method, step.path, rec.Code, rec.Body)
		}
	}
	if want := "add badge TD|set badge 3 Org|give 3 Bob|take 3 Ann|remove badge 3"; strings.Join(f.notes, "|") != want {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	if want := []string{"badge.add TD", "badge.set 3", "badge.give 3", "badge.take 3", "badge.remove 3"}; len(f.audits) != len(want) {
		t.Errorf("audits = %q, want %d", f.audits, len(want))
	} else {
		for i, w := range want {
			if f.audits[i] != owner+" "+w {
				t.Errorf("audit %d = %q, want %q", i, f.audits[i], owner+" "+w)
			}
		}
	}
}

func TestBadgeChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site, owner, org = "https://ttac-admin.example", "111111111111111111", "222222222222222222"
	tests := []struct {
		name, who, method, path, origin, body string
		want                                  int
	}{
		{"an organiser lists", org, http.MethodGet, "/v1/admin/badges", "", "", http.StatusForbidden},
		{"an organiser makes", org, http.MethodPost, "/v1/admin/badges", site, `{"label": "X", "description": ""}`, http.StatusForbidden},
		{"an organiser gives", org, http.MethodPut, "/v1/admin/badges/3/players/Ann", site, "", http.StatusForbidden},
		{"another site", owner, http.MethodPost, "/v1/admin/badges", "https://evil.example", `{"label": "X", "description": ""}`, http.StatusForbidden},
		{"no label", owner, http.MethodPost, "/v1/admin/badges", site, `{"label": " ", "description": ""}`, http.StatusBadRequest},
		{"a long label", owner, http.MethodPost, "/v1/admin/badges", site, `{"label": "Seventeen letters", "description": ""}`, http.StatusBadRequest},
		{"a long description", owner, http.MethodPost, "/v1/admin/badges", site, `{"label": "X", "description": "` + strings.Repeat("x", 301) + `"}`, http.StatusBadRequest},
		{"a label taken", owner, http.MethodPost, "/v1/admin/badges", site, `{"label": "Organiser", "description": ""}`, http.StatusConflict},
		{"changing no badge", owner, http.MethodPut, "/v1/admin/badges/9", site, `{"label": "X", "description": ""}`, http.StatusNotFound},
		{"giving no badge", owner, http.MethodPut, "/v1/admin/badges/9/players/Ann", site, "", http.StatusNotFound},
		{"giving to nobody", owner, http.MethodPut, "/v1/admin/badges/3/players/Nobody", site, "", http.StatusNotFound},
		{"taking one not held", owner, http.MethodDelete, "/v1/admin/badges/3/players/Bob", site, "", http.StatusNotFound},
		{"removing no badge", owner, http.MethodDelete, "/v1/admin/badges/9", site, "", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			rec := adminDoAs(t, f, tt.who, tt.method, tt.path, tt.origin, tt.body)
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
}

func TestAdminDataSummaryIsForAdminsAndOrganisers(t *testing.T) {
	for _, who := range []string{"111111111111111111", "222222222222222222"} {
		rec := adminDoAs(t, &fakeAdmins{}, who, http.MethodGet, "/v1/admin/data", "", "")
		var got DataSummary
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("GET data as %s = %d %s", who, rec.Code, rec.Body)
		}
		if got.Games != 144691 || got.NeverPlayed != 36 || got.LatestGame == nil || got.LastLoad != nil {
			t.Errorf("data = %+v", got)
		}
	}
}

func TestAnOrganiserSetsATournamentsCard(t *testing.T) {
	const site = "https://ttac-admin.example"
	f := &fakeAdmins{}
	rec := adminDo(t, f, http.MethodPut, "/v1/admin/tournaments/worlds/card", site, `{"icon": "🏆", "tagline": "Annual", "summary": "The pinnacle."}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT card = %d %s, want 204", rec.Code, rec.Body)
	}
	if strings.Join(f.notes, "|") != "card 🏆|Annual|The pinnacle." || strings.Join(f.audits, "|") != "222222222222222222 tournament.card worlds" {
		t.Errorf("changes %q, audits %q", f.notes, f.audits)
	}
	for _, tt := range []struct {
		name, path, origin, body string
		want                     int
	}{
		{"another site", "/v1/admin/tournaments/worlds/card", "https://evil.example", `{"icon": "", "tagline": "", "summary": ""}`, http.StatusForbidden},
		{"no such tournament", "/v1/admin/tournaments/nope/card", site, `{"icon": "", "tagline": "", "summary": ""}`, http.StatusNotFound},
		{"too long for the store", "/v1/admin/tournaments/worlds/card", site, `{"icon": "toolong", "tagline": "", "summary": ""}`, http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			if rec := adminDo(t, f, http.MethodPut, tt.path, tt.origin, tt.body); rec.Code != tt.want {
				t.Errorf("PUT = %d %s, want %d", rec.Code, rec.Body, tt.want)
			}
			if len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
	rec = adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/tournaments", "", "")
	var got []AdminTournament
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got[0].Card.Tagline != "Annual World Championship" {
		t.Errorf("GET = %s, want the card", rec.Body)
	}
}

func TestAnOrganiserReplacesATournamentsStories(t *testing.T) {
	const site = "https://ttac-admin.example"
	f := &fakeAdmins{}
	rec := adminDo(t, f, http.MethodPut, "/v1/admin/tournaments/worlds/stories", site,
		`{"stories": [{"player": "a440", "badge": "", "title": "2025", "text": "Won."}, {"player": "pv4", "badge": "", "title": "2024", "text": "Second."}]}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT stories = %d %s, want 204", rec.Code, rec.Body)
	}
	if strings.Join(f.notes, ",") != "story a440|2025,story pv4|2024" || strings.Join(f.audits, "|") != "222222222222222222 tournament.stories worlds" {
		t.Errorf("changes %q, audits %q", f.notes, f.audits)
	}
	for _, tt := range []struct {
		name, path, origin, body string
		want                     int
	}{
		{"another site", "/v1/admin/tournaments/worlds/stories", "https://evil.example", `{"stories": []}`, http.StatusForbidden},
		{"no such tournament", "/v1/admin/tournaments/nope/stories", site, `{"stories": []}`, http.StatusNotFound},
		{"a story without a title", "/v1/admin/tournaments/worlds/stories", site, `{"stories": [{"player": "p", "badge": "", "title": "", "text": "x"}]}`, http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			if rec := adminDo(t, f, http.MethodPut, tt.path, tt.origin, tt.body); rec.Code != tt.want {
				t.Errorf("PUT = %d %s, want %d", rec.Code, rec.Body, tt.want)
			}
			if len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
	rec = adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/tournaments", "", "")
	var got []AdminTournament
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got[0].Stories) != 1 || got[0].Stories[0].Player != "a440" {
		t.Errorf("GET = %s, want the stories", rec.Body)
	}
}

func TestAnOrganiserSetsWhatOfATournamentIsRated(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	if rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds", site, `{"rated_player_counts": [2, 4]}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH player counts = %d %s, want 204", rec.Code, rec.Body)
	}
	if len(f.settings) != 1 || !slices.Equal(f.settings[0].RatedPlayerCounts, []int{2, 4}) {
		t.Errorf("settings = %+v, want player counts 2 and 4", f.settings)
	}
	if rec := adminDo(t, f, http.MethodPut, "/v1/admin/tournaments/worlds/season-rating", site, `{"season": "2023", "rated": false}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT season rating = %d %s, want 204", rec.Code, rec.Body)
	}
	if strings.Join(f.notes, "|") != "season 2023 rated false" || strings.Join(f.audits, "|") != org+" tournament.settings worlds|"+org+" tournament.season.rated worlds" {
		t.Errorf("changes %q, audits %q", f.notes, f.audits)
	}
	for _, tt := range []struct {
		name, method, path, origin, body string
		want                             int
	}{
		{"a 5-player count", http.MethodPatch, "/v1/admin/tournaments/worlds", site, `{"rated_player_counts": [2, 5]}`, http.StatusBadRequest},
		{"a season that does not exist", http.MethodPut, "/v1/admin/tournaments/worlds/season-rating", site, `{"season": "1999", "rated": false}`, http.StatusNotFound},
		{"no season", http.MethodPut, "/v1/admin/tournaments/worlds/season-rating", site, `{"season": "", "rated": false}`, http.StatusBadRequest},
		{"another site", http.MethodPut, "/v1/admin/tournaments/worlds/season-rating", "https://evil.example", `{"season": "2023", "rated": false}`, http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			if rec := adminDo(t, f, tt.method, tt.path, tt.origin, tt.body); rec.Code != tt.want {
				t.Errorf("%s = %d %s, want %d", tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.settings)+len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %+v %q %q", f.settings, f.notes, f.audits)
			}
		})
	}
	rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/tournaments", "", "")
	var got []AdminTournament
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || !slices.Equal(got[0].RatedPlayerCounts, []int{3, 4}) || !got[0].Seasons[0].Rated {
		t.Errorf("GET = %s, want the rated player counts and the season rated", rec.Body)
	}
}

func TestAdminsAndOrganisersDownloadTheData(t *testing.T) {
	const org = "222222222222222222"
	f := &fakeAdmins{}
	rec := adminDoAs(t, f, org, http.MethodGet, "/v1/admin/export", "", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "PK zip bytes" {
		t.Fatalf("GET export = %d %q, want 200 and the zip", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/zip" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="ttacommunity-data.zip"` {
		t.Errorf("Content-Disposition = %q", got)
	}
	if rec.Header().Get("Cache-Control") != "no-store" || strings.Join(f.audits, "|") != org+" data.export all" {
		t.Errorf("Cache-Control %q, audits %q", rec.Header().Get("Cache-Control"), f.audits)
	}
	if rec := adminDoAs(t, &fakeAdmins{}, "111111111111111111", http.MethodGet, "/v1/admin/export", "", ""); rec.Code != http.StatusOK {
		t.Errorf("an admin's export = %d, want 200", rec.Code)
	}
}
