package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

const (
	site  = "https://ttac-admin.example"
	owner = "111111111111111111"
)

func TestAdminCommunityEditionsList(t *testing.T) {
	f := &fakeAdmins{}
	rec := adminDoAs(t, f, owner, http.MethodGet, "/v1/admin/community/editions", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/admin/community/editions: got status %d, want 200", rec.Code)
	}
	body := strings.TrimSpace(rec.Body.String())
	if !strings.Contains(body, `"slug":"race26"`) || !strings.Contains(body, `"name":"Race to 2026"`) {
		t.Errorf("GET /v1/admin/community/editions body = %s", body)
	}
}

func TestAdminCommunityEditionPostAndPatch(t *testing.T) {
	f := &fakeAdmins{}

	// POST new edition
	postPayload := `{"slug":"race27","name":"Race to 2027","title":"Official 2027","short_name":"Race27","badge":"⭐ Community","maintainer":"dee","maintainer_note":"Note","year":2027,"counting_events":8,"visible":true,"sort_order":2}`
	rec := adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/community/editions", site, postPayload)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /v1/admin/community/editions: got status %d, want 201: %s", rec.Code, rec.Body.String())
	}

	// PATCH existing edition
	patchPayload := `{"name":"Race to 2026 (Updated)","visible":false}`
	rec = adminDoAs(t, f, owner, http.MethodPatch, "/v1/admin/community/editions/race26", site, patchPayload)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH /v1/admin/community/editions/race26: got status %d, want 204: %s", rec.Code, rec.Body.String())
	}

	// PATCH non-existing edition
	rec = adminDoAs(t, f, owner, http.MethodPatch, "/v1/admin/community/editions/unknown", site, patchPayload)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("PATCH unknown: got status %d, want 404", rec.Code)
	}
}
