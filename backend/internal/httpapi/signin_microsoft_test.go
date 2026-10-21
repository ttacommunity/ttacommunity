package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"ttacommunity/backend/internal/microsoft"
	"ttacommunity/backend/internal/store"
)

// The fake's Microsoft: good-code signs in Player One with a verified email, nameless-code an account with only an
// email, bare-code one with neither; each only with the verifier and nonce its last authorize URL carried.

func (f *fakeSignIns) MicrosoftReady() bool { return !f.microsoftOff }

func (f *fakeSignIns) MicrosoftAuthURL(state, verifier, nonce string) string {
	f.verifier, f.nonce = verifier, nonce
	return "https://microsoft.example/authorize?state=" + url.QueryEscape(state)
}

func (f *fakeSignIns) MicrosoftUser(_ context.Context, code, verifier, nonce string) (microsoft.User, error) {
	if verifier != f.verifier || nonce != f.nonce {
		return microsoft.User{}, errors.New("verifier or nonce does not match")
	}
	switch code {
	case "good-code":
		return microsoft.User{Subject: "AAAAAAAAAAAAAAAAAAAAAIkzqFVrSaSaFHy782bbtaQ", Name: "Player One", Email: "player@example.test"}, nil
	case "nameless-code":
		return microsoft.User{Subject: "AAAAAAAAAAAAAAAAAAAAAIkzqFVrSaSaFHy782bbtaR", Email: "player@example.test"}, nil
	case "bare-code":
		return microsoft.User{Subject: "AAAAAAAAAAAAAAAAAAAAAIkzqFVrSaSaFHy782bbtaS"}, nil
	}
	return microsoft.User{}, errors.New("invalid_grant")
}

// startedMicrosoft starts a Microsoft sign-in (adding when add) and returns the flow cookie and the state sent.
func startedMicrosoft(t *testing.T, f *fakeSignIns, query string) (*http.Cookie, string) {
	t.Helper()
	rec := playerDo(t, f, http.MethodGet, "/v1/auth/microsoft?"+query, "", "")
	if rec.Code != http.StatusFound {
		t.Fatalf("starting a Microsoft sign-in = %d %s, want 302", rec.Code, rec.Body)
	}
	to, err := url.Parse(rec.Header().Get("Location"))
	if err != nil || to.Host != "microsoft.example" {
		t.Fatalf("starting a Microsoft sign-in went to %q, want Microsoft", rec.Header().Get("Location"))
	}
	flow := cookieNamed(rec, flowCookie)
	if flow == nil || !flow.HttpOnly || !flow.Secure || flow.SameSite != http.SameSiteLaxMode || flow.Path != "/v1/auth/microsoft" || flow.MaxAge <= 0 {
		t.Fatalf("flow cookie = %+v, want HttpOnly, Secure, Lax, on /v1/auth/microsoft, short-lived", flow)
	}
	return &http.Cookie{Name: flowCookie, Value: flow.Value}, to.Query().Get("state")
}

func TestFinishingAMicrosoftSignIn(t *testing.T) {
	tests := []struct {
		name        string
		cookie      bool
		state       string // "" for the state Microsoft was sent
		query       string
		want        string
		wantSession bool
	}{
		{"the state matches", true, "", "code=good-code", siteOrigin + "/ratings/?signin=ok", true},
		{"another state", true, "another", "code=good-code", siteOrigin + "/account/?back=%2Fratings%2F&signin=failed", false},
		{"no flow cookie", false, "", "code=good-code", siteOrigin + "/account/?signin=failed", false},
		{"declined at Microsoft", true, "", "error=access_denied", siteOrigin + "/account/?back=%2Fratings%2F&signin=failed", false},
		{"a code Microsoft refuses", true, "", "code=bad-code", siteOrigin + "/account/?back=%2Fratings%2F&signin=failed", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSignIns()
			flow, state := startedMicrosoft(t, f, "return=%2Fratings%2F")
			if tt.state != "" {
				state = tt.state
			}
			var cookies []*http.Cookie
			if tt.cookie {
				cookies = append(cookies, flow)
			}
			rec := playerDo(t, f, http.MethodGet, "/v1/auth/microsoft/callback?state="+url.QueryEscape(state)+"&"+tt.query, "", "", cookies...)
			if rec.Code != http.StatusFound || rec.Header().Get("Location") != tt.want {
				t.Errorf("callback = %d to %q, want 302 to %q", rec.Code, rec.Header().Get("Location"), tt.want)
			}
			if c := cookieNamed(rec, flowCookie); c == nil || c.MaxAge >= 0 || c.Path != "/v1/auth/microsoft" {
				t.Errorf("flow cookie after the callback = %+v, want it cleared on its path", c)
			}
			if session := cookieNamed(rec, sessionCookie); (session != nil) != tt.wantSession || len(f.sessions) != len(f.signedIn) {
				t.Errorf("session cookie = %+v and sessions %d, want a session %v", session, len(f.sessions), tt.wantSession)
			}
		})
	}
}

func TestAMicrosoftLoginIsNamedByItsNameElseItsEmail(t *testing.T) {
	tests := []struct {
		code string
		want store.Identity
	}{
		{"good-code", store.Identity{Provider: "microsoft", Subject: "AAAAAAAAAAAAAAAAAAAAAIkzqFVrSaSaFHy782bbtaQ", Username: "Player One", Email: "player@example.test"}},
		{"nameless-code", store.Identity{Provider: "microsoft", Subject: "AAAAAAAAAAAAAAAAAAAAAIkzqFVrSaSaFHy782bbtaR", Username: "player@example.test", Email: "player@example.test"}},
		{"bare-code", store.Identity{Provider: "microsoft", Subject: "AAAAAAAAAAAAAAAAAAAAAIkzqFVrSaSaFHy782bbtaS", Username: "Microsoft account"}},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			f := newFakeSignIns()
			flow, state := startedMicrosoft(t, f, "")
			playerDo(t, f, http.MethodGet, "/v1/auth/microsoft/callback?code="+tt.code+"&state="+url.QueryEscape(state), "", "", flow)
			if len(f.signedIn) != 1 || f.signedIn[0] != tt.want {
				t.Errorf("signed in %+v, want %+v", f.signedIn, tt.want)
			}
		})
	}
}

func TestEachMicrosoftSignInHasItsOwnNonce(t *testing.T) {
	f := newFakeSignIns()
	startedMicrosoft(t, f, "")
	first := f.nonce
	startedMicrosoft(t, f, "")
	if first == "" || f.nonce == first {
		t.Errorf("nonces = %q then %q, want two different ones", first, f.nonce)
	}
}

func TestAddingMicrosoftFromTheAccountPage(t *testing.T) {
	tests := []struct {
		name     string
		account  int64
		signedIn bool
		want     string
		wantNote string
	}{
		{"signed in", 7, true, "/account/?signin=added", "add microsoft Player One"},
		{"a Microsoft account with an account of its own", 10, true, "/account/?signin=taken", ""},
		{"an account with another Microsoft account", 9, true, "/account/?signin=has-kind", ""},
		{"signed out meanwhile", 7, false, "/account/?signin=failed", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSignIns()
			flow, state := startedMicrosoft(t, f, "add=true")
			if !strings.HasSuffix(flow.Value, "."+addMark) {
				t.Fatalf("flow cookie = %q, want it marked to add", flow.Value)
			}
			cookies := []*http.Cookie{flow}
			if tt.signedIn {
				cookies = append(cookies, sessionFor(f, "token", store.Account{ID: tt.account, Provider: "email"}))
			}
			rec := playerDo(t, f, http.MethodGet, "/v1/auth/microsoft/callback?code=good-code&state="+url.QueryEscape(state), "", "", cookies...)
			if got := rec.Header().Get("Location"); got != siteOrigin+tt.want {
				t.Errorf("callback went to %q, want %q", got, siteOrigin+tt.want)
			}
			if got := strings.Join(f.notes, "|"); got != tt.wantNote {
				t.Errorf("changes = %q, want %q", got, tt.wantNote)
			}
			if len(f.signedIn) != 0 || cookieNamed(rec, sessionCookie) != nil {
				t.Errorf("adding Microsoft signed in %+v with a new session cookie, want neither", f.signedIn)
			}
		})
	}
}

func TestMicrosoftSignInRefusesWhenNotSetUp(t *testing.T) {
	f := newFakeSignIns()
	f.microsoftOff = true
	for _, path := range []string{"/v1/auth/microsoft", "/v1/auth/microsoft/callback?code=good-code&state=x"} {
		if rec := playerDo(t, f, http.MethodGet, path, "", ""); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("GET %s with Microsoft not set up = %d, want 503", path, rec.Code)
		}
	}
}

func TestMeSaysWhereAWayInCanBeWrittenTo(t *testing.T) {
	f := newFakeSignIns()
	cookie := sessionFor(f, "token", store.Account{ID: 7, Provider: "microsoft", Username: "Player One",
		Logins: []store.LoginName{{Provider: "microsoft", Username: "Player One", Email: "player@example.test"}}})
	rec := playerDo(t, f, http.MethodGet, "/v1/me", siteOrigin, "", cookie)
	wantJSON(t, rec.Body.Bytes(), "provider", `"microsoft"`)
	wantJSON(t, rec.Body.Bytes(), "logins.0.email", `"player@example.test"`)
}
