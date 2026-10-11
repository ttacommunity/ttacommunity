package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestAdminGroupsRelatedTournaments(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	rec := adminDo(t, f, http.MethodPost, "/v1/admin/tournaments", site, `{"name": "Big Cups", "kind": "cup", "members": ["worlds", "wimbledon"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST = %d %s, want 201", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "slug", `"big_cups"`)
	if rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds", site, `{"parent": "big_cups", "description": "Every year", "name": "Worlds"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH parent = %d %s, want 204", rec.Code, rec.Body)
	}
	if len(f.settings) != 1 || *f.settings[0].Parent != "big_cups" || *f.settings[0].Description != "Every year" || *f.settings[0].Name != "Worlds" {
		t.Errorf("settings = %+v, want worlds in big_cups with its description and name", f.settings)
	}
	if rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/big_cups", site, `{"members": ["wimbledon"]}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH members = %d %s, want 204", rec.Code, rec.Body)
	}
	if got, want := strings.Join(f.notes, "|"), "group Big Cups cup [worlds wimbledon]|members big_cups [wimbledon]"; got != want {
		t.Errorf("changes = %q, want %q", got, want)
	}
	if got, want := strings.Join(f.audits, "|"), org+" tournament.group big_cups|"+org+" tournament.settings worlds|"+org+" tournament.settings big_cups"; got != want {
		t.Errorf("audits = %q, want %q", got, want)
	}
}

func TestAdminRefusesAGroupItCannotMake(t *testing.T) {
	const site = "https://ttac-admin.example"
	tests := []struct {
		name, method, path, origin, body string
		want                             int
	}{
		{"a name with no letter", http.MethodPost, "/v1/admin/tournaments", site, `{"name": "!", "kind": "cup", "members": []}`, http.StatusBadRequest},
		{"no such kind", http.MethodPost, "/v1/admin/tournaments", site, `{"name": "Cups", "kind": "bowl", "members": []}`, http.StatusBadRequest},
		{"no such member", http.MethodPost, "/v1/admin/tournaments", site, `{"name": "Cups", "kind": "cup", "members": ["no_such"]}`, http.StatusNotFound},
		{"another site", http.MethodPost, "/v1/admin/tournaments", "https://evil.example", `{"name": "Cups", "kind": "cup", "members": []}`, http.StatusForbidden},
		{"a group in itself", http.MethodPatch, "/v1/admin/tournaments/worlds", site, `{"parent": "worlds"}`, http.StatusBadRequest},
		{"members for a tournament", http.MethodPatch, "/v1/admin/tournaments/worlds", site, `{"members": ["wimbledon"]}`, http.StatusBadRequest},
		{"no such member", http.MethodPatch, "/v1/admin/tournaments/big_cups", site, `{"members": ["no_such"]}`, http.StatusNotFound},
		{"members from another site", http.MethodPatch, "/v1/admin/tournaments/big_cups", "https://evil.example", `{"members": []}`, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			if rec := adminDo(t, f, tt.method, tt.path, tt.origin, tt.body); rec.Code != tt.want {
				t.Errorf("%s %s %s = %d %s, want %d", tt.method, tt.path, tt.body, rec.Code, rec.Body, tt.want)
			}
			if len(f.audits) != 0 {
				t.Errorf("audits = %q, want none", f.audits)
			}
		})
	}
}
