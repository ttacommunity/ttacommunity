package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ttacommunity/backend/internal/access"
	"ttacommunity/backend/internal/store"
)

// fakeAdmins accepts the token "good-<id>" for Discord id <id>, and knows one admin and one organiser.
// It records each tournament change and audit, and knows one tournament, worlds.
type fakeAdmins struct {
	settings []store.TournamentSettings
	notes    []string
	audits   []string
}

func (*fakeAdmins) AllowedOrigin() string { return "https://ttac-admin.example" }

func (*fakeAdmins) Tournaments(context.Context) ([]store.AdminTournament, error) {
	return []store.AdminTournament{{Slug: "worlds", Name: "World Championship", Rated: true,
		Seasons: []store.AdminSeason{{Label: "2025", Stages: []store.AdminStage{{Label: "Stage 1", Note: "n"}}}}}}, nil
}

func (f *fakeAdmins) SetTournament(_ context.Context, slug string, s store.TournamentSettings) error {
	if slug != "worlds" {
		return store.ErrNoTournament
	}
	f.settings = append(f.settings, s)
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
