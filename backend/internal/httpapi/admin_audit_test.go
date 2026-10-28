package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAnAdminReadsTheAuditLog(t *testing.T) {
	f := &fakeAdmins{}
	rec := adminDoAs(t, f, "111111111111111111", http.MethodGet,
		"/v1/admin/audit?action=badge.add&q=%20Gold%20&from=2026-10-21T13:00:00Z&to=2026-10-28T13:00:00%2B11:00&page=2", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET audit = %d %s, want 200", rec.Code, rec.Body)
	}
	from, to := time.Date(2026, 10, 21, 13, 0, 0, 0, time.UTC), time.Date(2026, 10, 28, 2, 0, 0, 0, time.UTC)
	got := f.logAsked
	if got == nil || got.Action != "badge.add" || got.Text != "Gold" || got.From == nil || !got.From.Equal(from) || got.To == nil || !got.To.Equal(to) || f.logPage != 2 {
		t.Errorf("AuditLog asked for %+v page %d, want badge.add, Gold, from %v to %v, page 2", got, f.logPage, from, to)
	}
	wantJSON(t, rec.Body.Bytes(), "page", `2`)
	wantJSON(t, rec.Body.Bytes(), "actions", `["badge.add"]`)
	wantJSON(t, rec.Body.Bytes(), "entries.0", `{"id": 7, "at": "2026-10-28T01:13:00Z", "admin": "Owner", "by_player": false,
		"action": "badge.add", "target": "Gold", "detail": "{\"label\": \"Gold\"}"}`)
}

func TestTheAuditLogIsRefusedWhenItShouldBe(t *testing.T) {
	long := strings.Repeat("x", 101)
	tests := []struct {
		name, who, query string
		want             int
	}{
		{"an organiser", "222222222222222222", "", http.StatusForbidden},
		{"page 0", "111111111111111111", "?page=0", http.StatusBadRequest},
		{"a search over 100 characters", "111111111111111111", "?q=" + long, http.StatusBadRequest},
		{"an action over 100 characters", "111111111111111111", "?action=" + long, http.StatusBadRequest},
		{"to before from", "111111111111111111", "?from=2026-10-28T00:00:00Z&to=2026-10-27T00:00:00Z", http.StatusBadRequest},
		{"a time that is not one", "111111111111111111", "?from=yesterday", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			rec := adminDoAs(t, f, tt.who, http.MethodGet, "/v1/admin/audit"+tt.query, "", "")
			if rec.Code != tt.want {
				t.Errorf("GET audit%s as %s = %d %s, want %d", tt.query, tt.who, rec.Code, rec.Body, tt.want)
			}
			if f.logAsked != nil {
				t.Errorf("a refused read still read the log: %+v", f.logAsked)
			}
		})
	}
}
