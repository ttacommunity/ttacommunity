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

// The fake's accounts: account 1 is linked by match to ann, by Discord and email; account 2 waits on request 20 for
// "Bob", which names no member yet; account 4 was refused "Cat". Request 22 is for a member linked to another
// account; request 99 and account 99 do not exist.
func (f *fakeAdmins) Accounts(context.Context) ([]store.AccountRow, error) {
	ann := int64(3)
	at := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	return []store.AccountRow{
		{ID: 2, Logins: []store.AccountLogin{{Provider: "discord", Username: "player_two", SignedInAt: at}}, SignedInAt: at,
			Request: &store.PendingRequest{ID: 20, Name: "Bob", Country: "NZ", Zone: "Pacific/Auckland", MemberEmail: "", RequestedAt: at.Add(time.Minute)}},
		{ID: 1, Logins: []store.AccountLogin{{Provider: "discord", Username: "player_one", DisplayName: "Player One", SignedInAt: at},
			{Provider: "email", Username: "player@example.test", SignedInAt: at.Add(time.Hour)}},
			MemberID: &ann, Member: "ann", LinkedHow: "match", SignedInAt: at.Add(time.Hour), AutoLink: true},
		{ID: 4, Logins: []store.AccountLogin{{Provider: "email", Username: "four@example.test", SignedInAt: at}}, SignedInAt: at,
			Refused: &store.RefusedRequest{Name: "Cat", Reason: "Not the same player"}},
	}, nil
}

func (f *fakeAdmins) ApproveLink(_ context.Context, request int64, _ time.Time) (store.LinkApproved, error) {
	switch request {
	case 20:
		f.notes = append(f.notes, "approve 20")
		return store.LinkApproved{Member: "Bob", Created: true}, nil
	case 22:
		return store.LinkApproved{}, store.ErrMemberHasAccount
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

func (f *fakeAdmins) UnlinkAccount(_ context.Context, account int64) error {
	if account != 1 {
		return store.ErrNoAccount
	}
	f.notes = append(f.notes, "unlink 1")
	return nil
}

func (f *fakeAdmins) RemoveAccount(_ context.Context, account int64) error {
	if account != 1 && account != 2 {
		return store.ErrNoAccount
	}
	f.notes = append(f.notes, fmt.Sprint("remove ", account))
	return nil
}

func TestAdminListsAccountsWithWhatTheyAsk(t *testing.T) {
	rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/accounts", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET accounts = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "0.request.name", `"Bob"`)
	wantJSON(t, rec.Body.Bytes(), "0.request.member", `""`)
	wantJSON(t, rec.Body.Bytes(), "0.request.country", `"NZ"`)
	wantJSON(t, rec.Body.Bytes(), "0.request.zone", `"Pacific/Auckland"`)
	wantJSON(t, rec.Body.Bytes(), "0.request.member_zone", `""`)
	wantJSON(t, rec.Body.Bytes(), "0.request.member_email", `""`)
	wantJSON(t, rec.Body.Bytes(), "1.logins.0.provider", `"discord"`)
	wantJSON(t, rec.Body.Bytes(), "1.logins.1.provider", `"email"`)
	wantJSON(t, rec.Body.Bytes(), "1.logins.1.username", `"player@example.test"`)
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
		{"approving a member linked to another account", http.MethodPut, "/v1/admin/link-requests/22/approval", "", http.StatusConflict, ""},
		{"approving a request not waiting", http.MethodPut, "/v1/admin/link-requests/99/approval", "", http.StatusNotFound, ""},
		{"refused with a reason", http.MethodPut, "/v1/admin/link-requests/20/refusal", `{"reason":"Message us first"}`, http.StatusNoContent, "refuse 20 Message us first"},
		{"refused with a long reason", http.MethodPut, "/v1/admin/link-requests/20/refusal", `{"reason":"` + strings.Repeat("x", 501) + `"}`, http.StatusBadRequest, ""},
		{"refusing a request not waiting", http.MethodPut, "/v1/admin/link-requests/99/refusal", `{"reason":""}`, http.StatusNotFound, ""},
		{"unlinked", http.MethodDelete, "/v1/admin/accounts/1/member", "", http.StatusNoContent, "unlink 1"},
		{"unlinking an account that is not there", http.MethodDelete, "/v1/admin/accounts/99/member", "", http.StatusNotFound, ""},
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

func TestOnlyAnAdminRemovesAnAccount(t *testing.T) {
	f := &fakeAdmins{}
	if rec := adminDoAs(t, f, "222222222222222222", http.MethodDelete, "/v1/admin/accounts/1", "https://ttac-admin.example", ""); rec.Code != http.StatusForbidden {
		t.Errorf("an organiser removing an account = %d, want 403", rec.Code)
	}
	if rec := adminDoAs(t, f, "111111111111111111", http.MethodDelete, "/v1/admin/accounts/1", "https://ttac-admin.example", ""); rec.Code != http.StatusNoContent {
		t.Errorf("an admin removing an account = %d %s, want 204", rec.Code, rec.Body)
	}
	if rec := adminDoAs(t, f, "111111111111111111", http.MethodDelete, "/v1/admin/accounts/99", "https://ttac-admin.example", ""); rec.Code != http.StatusNotFound {
		t.Errorf("removing an account that is not there = %d, want 404", rec.Code)
	}
	if got := strings.Join(f.notes, "|"); got != "remove 1" {
		t.Errorf("changes = %q, want only the admin's removal", got)
	}
	if len(f.audits) != 1 || f.audits[0] != "111111111111111111 account.remove 1" {
		t.Errorf("audits = %q, want the admin's removal", f.audits)
	}
}
