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
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Cf-Access-Jwt-Assertion", "good-222222222222222222")
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
