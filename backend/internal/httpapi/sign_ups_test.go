package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

const (
	adminOrigin = "https://ttac-admin.example"
	mySignUp    = "/v1/me/sign-ups/us_open/2026"
	signUpsPath = "/v1/admin/tournaments/us_open/seasons/2026/sign-ups"
)

func (f *fakeSignIns) SignUp(_ context.Context, hash []byte, slug, edition, name string, _ time.Time) (string, error) {
	if f.signUpErr != nil {
		return "", f.signUpErr
	}
	l := f.sessions[string(hash)]
	f.notes = append(f.notes, fmt.Sprintf("sign up %s %s by %s name=%s", slug, edition, l.Username, name))
	if l.Member != nil {
		return "entered", nil
	}
	f.pending = &store.PendingSignUp{ID: 3, Account: l.ID, Name: name}
	return "pending", nil
}

func (f *fakeSignIns) WithdrawSignUp(_ context.Context, hash []byte, slug, edition string, _ time.Time) error {
	if f.signUpErr != nil {
		return f.signUpErr
	}
	f.notes = append(f.notes, fmt.Sprintf("withdraw %s %s by %s", slug, edition, f.sessions[string(hash)].Username))
	return nil
}

func (f *fakeSignIns) MySignUp(_ context.Context, _ []byte, slug, edition string, _ time.Time) (store.PendingSignUp, bool, error) {
	if f.pending == nil || slug != "us_open" || edition != "2026" {
		return store.PendingSignUp{}, false, nil
	}
	return *f.pending, true, nil
}

func (f *fakeAdmins) SignUpsOf(_ context.Context, slug, edition string) (store.SignUps, error) {
	if slug != "us_open" || edition != "2026" {
		return store.SignUps{}, store.ErrNoBlitzEdition
	}
	return f.signUps, nil
}

func (f *fakeAdmins) PendingSignUps(_ context.Context, _, _ string) ([]store.PendingSignUp, error) {
	return f.pending, nil
}

func (f *fakeAdmins) SetSignUps(_ context.Context, slug, edition string, s store.SignUps, _ time.Time) error {
	if f.signUpErr != nil {
		return f.signUpErr
	}
	closes := "none"
	if s.ClosesAt != nil {
		closes = s.ClosesAt.UTC().Format(time.RFC3339)
	}
	f.notes = append(f.notes, fmt.Sprintf("sign-ups %s %s open=%v closes=%s", slug, edition, s.Open, closes))
	return nil
}

func (f *fakeAdmins) AcceptSignUp(_ context.Context, slug, edition string, id int64, _ time.Time) error {
	if f.signUpErr != nil {
		return f.signUpErr
	}
	f.notes = append(f.notes, fmt.Sprintf("accept %s %s %d", slug, edition, id))
	return nil
}

func (f *fakeAdmins) RemoveSignUp(_ context.Context, slug, edition string, id int64) error {
	if f.signUpErr != nil {
		return f.signUpErr
	}
	f.notes = append(f.notes, fmt.Sprintf("remove %s %s %d", slug, edition, id))
	return nil
}

func TestAPlayerReadsWhereTheirSignUpStands(t *testing.T) {
	unlinked := store.Account{ID: 1, Provider: "discord", Username: "newcomer"}
	tests := []struct {
		name    string
		account store.Account
		pending *store.PendingSignUp
		want    string
	}{
		{"an entrant", member(1007, "Ann"), nil, `{"status": "entered", "name": null}`},
		{"linked, not entered", member(1050, "Eve"), nil, `{"status": "none", "name": null}`},
		{"unlinked, waiting", unlinked, &store.PendingSignUp{ID: 3, Account: 1, Name: "Newcomer"}, `{"status": "pending", "name": "Newcomer"}`},
		{"unlinked, not signed up", unlinked, nil, `{"status": "none", "name": null}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSignIns()
			f.pending = tt.pending
			rec := playerDoWith(t, &fakeRatings{blitzEdition: reportsEdition(nil)}, f, http.MethodGet, mySignUp, "", "", sessionFor(f, "token", tt.account))
			if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != cacheNone {
				t.Fatalf("GET = %d %s, Cache-Control %q; want 200, %q", rec.Code, rec.Body, rec.Header().Get("Cache-Control"), cacheNone)
			}
			wantJSON(t, rec.Body.Bytes(), "", tt.want)
		})
	}
}

func TestReadingASignUpNeedsASessionAndAnEdition(t *testing.T) {
	f := newFakeSignIns()
	ratings := &fakeRatings{blitzEdition: reportsEdition(nil)}
	if rec := playerDoWith(t, ratings, f, http.MethodGet, mySignUp, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET signed out = %d, want 401", rec.Code)
	}
	cookie := sessionFor(f, "token", member(1007, "Ann"))
	if rec := playerDoWith(t, ratings, f, http.MethodGet, "/v1/me/sign-ups/us_open/1999", "", "", cookie); rec.Code != http.StatusNotFound {
		t.Errorf("GET an unknown edition = %d, want 404", rec.Code)
	}
}

func TestAPlayerSignsUpAndWithdraws(t *testing.T) {
	tests := []struct {
		name     string
		account  store.Account
		method   string
		body     string
		wantCode int
		wantBody string
		wantNote string
	}{
		{"linked", member(1050, "Eve"), http.MethodPut, `{}`, http.StatusOK, `{"status": "entered", "name": null}`, "sign up us_open 2026 by eve name="},
		{"unlinked, with a username", store.Account{ID: 1, Provider: "discord", Username: "newcomer"}, http.MethodPut, `{"tta_name": "Newcomer"}`,
			http.StatusOK, `{"status": "pending", "name": "Newcomer"}`, "sign up us_open 2026 by newcomer name=Newcomer"},
		{"withdrawing", member(1050, "Eve"), http.MethodDelete, "", http.StatusNoContent, "", "withdraw us_open 2026 by eve"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSignIns()
			rec := playerDoWith(t, &fakeRatings{blitzEdition: reportsEdition(nil)}, f, tt.method, mySignUp, siteOrigin, tt.body, sessionFor(f, "token", tt.account))
			if rec.Code != tt.wantCode {
				t.Fatalf("%s = %d %s, want %d", tt.method, rec.Code, rec.Body, tt.wantCode)
			}
			if tt.wantBody != "" {
				wantJSON(t, rec.Body.Bytes(), "", tt.wantBody)
			}
			if !slices.Equal(f.notes, []string{tt.wantNote}) {
				t.Errorf("notes = %q, want %q", f.notes, tt.wantNote)
			}
		})
	}
}

func TestASignUpOrWithdrawalIsRefusedAsItShouldBe(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"not signed in", store.ErrNoSession, http.StatusUnauthorized},
		{"banned", store.ErrMemberBanned, http.StatusForbidden},
		{"no such edition", store.ErrNoBlitzEdition, http.StatusNotFound},
		{"not taking entries", store.ErrSignUpsClosed, http.StatusConflict},
		{"no username", store.ErrSignUpNameNeeded, http.StatusBadRequest},
		{"a username another account holds", store.ErrNameLinked, http.StatusConflict},
	}
	for _, tt := range tests {
		for _, method := range []string{http.MethodPut, http.MethodDelete} {
			if method == http.MethodDelete && (errors.Is(tt.err, store.ErrSignUpNameNeeded) || errors.Is(tt.err, store.ErrNameLinked)) {
				continue
			}
			t.Run(tt.name+", "+method, func(t *testing.T) {
				f := newFakeSignIns()
				f.signUpErr = tt.err
				rec := playerDoWith(t, &fakeRatings{blitzEdition: reportsEdition(nil)}, f, method, mySignUp, siteOrigin, `{"tta_name": "Newcomer"}`,
					sessionFor(f, "token", member(1050, "Eve")))
				if rec.Code != tt.want {
					t.Errorf("%s = %d %s, want %d", method, rec.Code, rec.Body, tt.want)
				}
				if strings.Contains(rec.Body.String(), "ERROR") {
					t.Errorf("%s answered a database error: %s", method, rec.Body)
				}
			})
		}
	}
}

func TestASignUpFromAnotherOriginIsRefused(t *testing.T) {
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		f := newFakeSignIns()
		rec := playerDoWith(t, &fakeRatings{blitzEdition: reportsEdition(nil)}, f, method, mySignUp, "https://evil.example", `{}`,
			sessionFor(f, "token", member(1050, "Eve")))
		if rec.Code != http.StatusForbidden || len(f.notes) != 0 {
			t.Errorf("%s from another origin = %d, notes %q; want 403 and nothing done", method, rec.Code, f.notes)
		}
	}
}

func TestTheEditionSaysWhetherItTakesSignUps(t *testing.T) {
	closes := time.Date(2026, 11, 1, 12, 0, 0, 0, time.FixedZone("AEDT", 11*3600))
	ed := reportsEdition(func(ed *store.BlitzEdition, _ *store.BlitzMatch) {
		ed.SignUps = store.SignUps{Open: true, ClosesAt: &closes, Taking: true}
	})
	rec, _ := serve(t, &fakeRatings{blitzEdition: ed}, "/v1/blitz/us_open/2026")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "sign_ups", `{"open": true, "closes_at": "2026-11-01T01:00:00Z", "taking": true}`)
}

func TestAdminReadsASeasonsSignUps(t *testing.T) {
	at := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	f := &fakeAdmins{signUps: store.SignUps{Open: true, Taking: true},
		pending: []store.PendingSignUp{{ID: 3, Account: 41, Name: "Newcomer", SignedUpAt: at, Request: 12}, {ID: 4, Account: 42, Name: "Later", SignedUpAt: at, Linked: "Later"}}}
	rec := adminDo(t, f, http.MethodGet, signUpsPath, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "sign_ups", `{"open": true, "closes_at": null, "taking": true}`)
	wantJSON(t, rec.Body.Bytes(), "pending", `[
		{"id": 3, "account": 41, "name": "Newcomer", "signed_up_at": "2026-10-20T09:00:00Z", "request": 12, "linked": null},
		{"id": 4, "account": 42, "name": "Later", "signed_up_at": "2026-10-20T09:00:00Z", "request": null, "linked": "Later"}]`)
	if rec := adminDo(t, f, http.MethodGet, "/v1/admin/tournaments/us_open/seasons/1999/sign-ups", "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("GET an unknown season = %d, want 404", rec.Code)
	}
}

func TestAdminChangesSignUpsAndAuditsIt(t *testing.T) {
	tests := []struct {
		name, method, path, body string
		wantNote, wantAudit      string
	}{
		{"opening", http.MethodPut, signUpsPath, `{"open": true, "closes_at": "2026-11-01T12:00:00+11:00"}`,
			"sign-ups us_open 2026 open=true closes=2026-11-01T01:00:00Z", "222222222222222222 sign_ups.set us_open/2026"},
		{"closing", http.MethodPut, signUpsPath, `{"open": false}`, "sign-ups us_open 2026 open=false closes=none", "222222222222222222 sign_ups.set us_open/2026"},
		{"accepting", http.MethodPost, signUpsPath + "/3/accept", "", "accept us_open 2026 3", "222222222222222222 sign_up.accept 3"},
		{"removing", http.MethodDelete, signUpsPath + "/3", "", "remove us_open 2026 3", "222222222222222222 sign_up.remove 3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			rec := adminDo(t, f, tt.method, tt.path, adminOrigin, tt.body)
			if rec.Code != http.StatusNoContent {
				t.Fatalf("%s %s = %d %s, want 204", tt.method, tt.path, rec.Code, rec.Body)
			}
			if !slices.Equal(f.notes, []string{tt.wantNote}) {
				t.Errorf("notes = %q, want %q", f.notes, tt.wantNote)
			}
			if !slices.Equal(f.audits, []string{tt.wantAudit}) {
				t.Errorf("audits = %q, want %q", f.audits, tt.wantAudit)
			}
		})
	}
}

func TestAdminSignUpChangesAreRefusedAsTheyShouldBe(t *testing.T) {
	tests := []struct {
		name, method, path string
		err                error
		want               int
	}{
		{"opening a tournament that is not blitz", http.MethodPut, signUpsPath, store.ErrSignUpsNotOffered, http.StatusBadRequest},
		{"a closing time in the past", http.MethodPut, signUpsPath, store.ErrBadSignUps, http.StatusBadRequest},
		{"opening a hidden tournament", http.MethodPut, signUpsPath, store.ErrSignUpsHidden, http.StatusConflict},
		{"opening once round 1 is paired", http.MethodPut, signUpsPath, store.ErrSignUpsStarted, http.StatusConflict},
		{"an unknown season", http.MethodPut, signUpsPath, store.ErrNoBlitzEdition, http.StatusNotFound},
		{"accepting an unknown sign-up", http.MethodPost, signUpsPath + "/3/accept", store.ErrNoSignUp, http.StatusNotFound},
		{"accepting with no request waiting", http.MethodPost, signUpsPath + "/3/accept", store.ErrNoRequest, http.StatusConflict},
		{"accepting a banned member", http.MethodPost, signUpsPath + "/3/accept", store.ErrMemberBanned, http.StatusConflict},
		{"accepting a member another account holds", http.MethodPost, signUpsPath + "/3/accept", store.ErrMemberHasAccount, http.StatusConflict},
		{"removing an unknown sign-up", http.MethodDelete, signUpsPath + "/3", store.ErrNoSignUp, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{signUpErr: tt.err}
			rec := adminDo(t, f, tt.method, tt.path, adminOrigin, `{"open": true}`)
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.audits) != 0 {
				t.Errorf("audits = %q, want none for a refusal", f.audits)
			}
		})
	}
}

func TestAdminSignUpChangesFromAnotherOriginAreRefused(t *testing.T) {
	for _, r := range []struct{ method, path string }{{http.MethodPut, signUpsPath}, {http.MethodPost, signUpsPath + "/3/accept"}, {http.MethodDelete, signUpsPath + "/3"}} {
		f := &fakeAdmins{}
		rec := adminDo(t, f, r.method, r.path, "https://evil.example", `{"open": true}`)
		if rec.Code != http.StatusForbidden || len(f.notes) != 0 {
			t.Errorf("%s %s from another origin = %d, notes %q; want 403 and nothing done", r.method, r.path, rec.Code, f.notes)
		}
	}
}
