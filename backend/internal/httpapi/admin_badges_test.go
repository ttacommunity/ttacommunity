package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

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
		{http.MethodPut, "/v1/admin/badges/3", `{"label": "Org", "description": "", "players": ["Bob"]}`},
		{http.MethodPut, "/v1/admin/badges/3", `{"label": "Org", "description": "", "players": []}`},
		{http.MethodDelete, "/v1/admin/badges/3", ""},
	} {
		if rec := adminDoAs(t, f, owner, step.method, step.path, site, step.body); rec.Code != http.StatusNoContent {
			t.Errorf("%s %s = %d %s, want 204", step.method, step.path, rec.Code, rec.Body)
		}
	}
	if want := "add badge TD|set badge 3 Org []|set badge 3 Org [Bob]|set badge 3 Org []|remove badge 3"; strings.Join(f.notes, "|") != want {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	if want := []string{"badge.add TD", "badge.set 3", "badge.set 3", "badge.set 3", "badge.remove 3"}; len(f.audits) != len(want) {
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
		{"an organiser gives", org, http.MethodPut, "/v1/admin/badges/3", site, `{"label": "Organiser", "description": "", "players": ["Ann"]}`, http.StatusForbidden},
		{"another site", owner, http.MethodPost, "/v1/admin/badges", "https://evil.example", `{"label": "X", "description": ""}`, http.StatusForbidden},
		{"no label", owner, http.MethodPost, "/v1/admin/badges", site, `{"label": " ", "description": ""}`, http.StatusBadRequest},
		{"a long label", owner, http.MethodPost, "/v1/admin/badges", site, `{"label": "Seventeen letters", "description": ""}`, http.StatusBadRequest},
		{"a long description", owner, http.MethodPost, "/v1/admin/badges", site, `{"label": "X", "description": "` + strings.Repeat("x", 301) + `"}`, http.StatusBadRequest},
		{"a label taken", owner, http.MethodPost, "/v1/admin/badges", site, `{"label": "Organiser", "description": ""}`, http.StatusConflict},
		{"changing no badge", owner, http.MethodPut, "/v1/admin/badges/9", site, `{"label": "X", "description": ""}`, http.StatusNotFound},
		{"giving no badge", owner, http.MethodPut, "/v1/admin/badges/9", site, `{"label": "X", "description": "", "players": ["Ann"]}`, http.StatusNotFound},
		{"giving to nobody", owner, http.MethodPut, "/v1/admin/badges/3", site, `{"label": "Organiser", "description": "", "players": ["Ann", "Nobody"]}`, http.StatusNotFound},
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
