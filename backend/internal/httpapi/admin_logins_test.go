package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

// The fake's sign-ins: login 1 is linked by match to ann; login 2 waits on request 20 for "Bob", which names no
// member yet; login 4 was refused "Cat". Request 22 is for a member with a Discord login; 99 and
// login 99 do not exist.
func (f *fakeAdmins) Logins(context.Context) ([]store.LoginRow, error) {
	ann := int64(3)
	return []store.LoginRow{
		{ID: 2, Provider: "discord", Username: "player_two", SignedInAt: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC),
			Request: &store.PendingRequest{ID: 20, Name: "Bob", Country: "NZ", Zone: "Pacific/Auckland", RequestedAt: time.Date(2026, 10, 18, 12, 1, 0, 0, time.UTC)}},
		{ID: 1, Provider: "discord", Username: "player_one", DisplayName: "Player One", MemberID: &ann, Member: "ann", LinkedHow: "match",
			SignedInAt: time.Date(2026, 10, 18, 11, 0, 0, 0, time.UTC), AutoLink: true},
		{ID: 4, Provider: "discord", Username: "player_four", SignedInAt: time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC),
			Refused: &store.RefusedRequest{Name: "Cat", Reason: "Not the same player"}},
	}, nil
}

func (f *fakeAdmins) ApproveLink(_ context.Context, request int64, _ time.Time) (store.LinkApproved, error) {
	switch request {
	case 20:
		f.notes = append(f.notes, "approve 20")
		return store.LinkApproved{Member: "Bob", Created: true}, nil
	case 22:
		return store.LinkApproved{}, store.ErrMemberHasLogin
	}
	return store.LinkApproved{}, store.ErrNoRequest
}

func (f *fakeAdmins) RefuseLink(_ context.Context, request int64, reason string, _ time.Time) error {
	if len(reason) > 500 {
		return store.ErrBadReason
	}
	if request != 20 {
		return store.ErrNoRequest
	}
	f.notes = append(f.notes, "refuse 20 "+reason)
	return nil
}

func (f *fakeAdmins) UnlinkLogin(_ context.Context, login int64) error {
	if login != 1 {
		return store.ErrNoLogin
	}
	f.notes = append(f.notes, "unlink 1")
	return nil
}

func (f *fakeAdmins) RemoveLogin(_ context.Context, login int64) error {
	if login != 1 && login != 2 {
		return store.ErrNoLogin
	}
	f.notes = append(f.notes, fmt.Sprint("remove ", login))
	return nil
}

func TestAdminListsSignInsWithWhatTheyAsk(t *testing.T) {
	rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/logins", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET logins = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "0.request.name", `"Bob"`)
	wantJSON(t, rec.Body.Bytes(), "0.request.member", `""`)
	wantJSON(t, rec.Body.Bytes(), "0.request.country", `"NZ"`)
	wantJSON(t, rec.Body.Bytes(), "0.request.zone", `"Pacific/Auckland"`)
	wantJSON(t, rec.Body.Bytes(), "0.request.member_zone", `""`)
	wantJSON(t, rec.Body.Bytes(), "0.linked_how", `null`)
	wantJSON(t, rec.Body.Bytes(), "1.member", `"ann"`)
	wantJSON(t, rec.Body.Bytes(), "1.linked_how", `"match"`)
	wantJSON(t, rec.Body.Bytes(), "1.request", `null`)
	wantJSON(t, rec.Body.Bytes(), "1.refused", `null`)
	wantJSON(t, rec.Body.Bytes(), "2.refused.name", `"Cat"`)
	wantJSON(t, rec.Body.Bytes(), "2.refused.reason", `"Not the same player"`)
	wantJSON(t, rec.Body.Bytes(), "1.auto_link", `true`)
	wantJSON(t, rec.Body.Bytes(), "2.auto_link", `false`)
}

func TestAnsweringALinkRequest(t *testing.T) {
	tests := []struct {
		name, method, path, body string
		want                     int
		wantNote                 string
	}{
		{"approved", http.MethodPut, "/v1/admin/link-requests/20/approval", "", http.StatusOK, "approve 20"},
		{"approving a member who has a Discord login", http.MethodPut, "/v1/admin/link-requests/22/approval", "", http.StatusConflict, ""},
		{"approving a request not waiting", http.MethodPut, "/v1/admin/link-requests/99/approval", "", http.StatusNotFound, ""},
		{"refused with a reason", http.MethodPut, "/v1/admin/link-requests/20/refusal", `{"reason":"Message us first"}`, http.StatusNoContent, "refuse 20 Message us first"},
		{"refused with a long reason", http.MethodPut, "/v1/admin/link-requests/20/refusal", `{"reason":"` + strings.Repeat("x", 501) + `"}`, http.StatusBadRequest, ""},
		{"refusing a request not waiting", http.MethodPut, "/v1/admin/link-requests/99/refusal", `{"reason":""}`, http.StatusNotFound, ""},
		{"unlinked", http.MethodDelete, "/v1/admin/logins/1/member", "", http.StatusNoContent, "unlink 1"},
		{"unlinking a login that is not there", http.MethodDelete, "/v1/admin/logins/99/member", "", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			rec := adminDo(t, f, tt.method, tt.path, "https://ttac-admin.example", tt.body)
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if got := strings.Join(f.notes, "|"); got != tt.wantNote {
				t.Errorf("changes = %q, want %q", got, tt.wantNote)
			}
			if wantAudit := tt.wantNote != ""; (len(f.audits) == 1) != wantAudit {
				t.Errorf("audits = %q, want one %v", f.audits, wantAudit)
			}
		})
	}
	rec := adminDo(t, &fakeAdmins{}, http.MethodPut, "/v1/admin/link-requests/20/approval", "https://ttac-admin.example", "")
	wantJSON(t, rec.Body.Bytes(), "member", `"Bob"`)
	wantJSON(t, rec.Body.Bytes(), "created", `true`)
}

func TestOnlyAnAdminRemovesASignIn(t *testing.T) {
	f := &fakeAdmins{}
	if rec := adminDoAs(t, f, "222222222222222222", http.MethodDelete, "/v1/admin/logins/1", "https://ttac-admin.example", ""); rec.Code != http.StatusForbidden {
		t.Errorf("an organiser removing a sign-in = %d, want 403", rec.Code)
	}
	if rec := adminDoAs(t, f, "111111111111111111", http.MethodDelete, "/v1/admin/logins/1", "https://ttac-admin.example", ""); rec.Code != http.StatusNoContent {
		t.Errorf("an admin removing a sign-in = %d %s, want 204", rec.Code, rec.Body)
	}
	if rec := adminDoAs(t, f, "111111111111111111", http.MethodDelete, "/v1/admin/logins/99", "https://ttac-admin.example", ""); rec.Code != http.StatusNotFound {
		t.Errorf("removing a sign-in that is not there = %d, want 404", rec.Code)
	}
	if got := strings.Join(f.notes, "|"); got != "remove 1" {
		t.Errorf("changes = %q, want only the admin's removal", got)
	}
	if len(f.audits) != 1 || f.audits[0] != "111111111111111111 login.remove 1" {
		t.Errorf("audits = %q, want the admin's removal", f.audits)
	}
}
