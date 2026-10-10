package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

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

func TestAdminStandingsCheckIsForAdminsAndOrganisers(t *testing.T) {
	for _, who := range []string{"111111111111111111", "222222222222222222"} {
		rec := adminDoAs(t, &fakeAdmins{}, who, http.MethodGet, "/v1/admin/standings-check", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET standings check as %s = %d %s", who, rec.Code, rec.Body)
		}
		body := rec.Body.Bytes()
		wantJSON(t, body, "stages_only_on_cge", `18`)
		wantJSON(t, body, "stages.0", `{"cge_tournament": 19, "cge_season": 4, "players": 520, "agree": 519}`)
		wantJSON(t, body, "stages.0.differences.0", `{"player": "Heron_Hill", "kind": "places_after_a_timeout", "cge_games": 6, "held_never_played": 0}`)
		wantJSON(t, body, "stages.0.differences.0.cge_places", `{"p2": null, "p3": [0, 1, 5], "p4": null}`)
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
