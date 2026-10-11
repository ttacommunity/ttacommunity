package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

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
