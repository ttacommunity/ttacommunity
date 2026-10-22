package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"ttacommunity/backend/internal/google"
	"ttacommunity/backend/internal/microsoft"
	"ttacommunity/backend/internal/store"
)

// The fake's Microsoft and Google: good-code signs in Player One with a verified email, nameless-code an account with
// only an email, bare-code one with neither; each only with the verifier and nonce its last authorize URL carried.

func (f *fakeSignIns) MicrosoftReady() bool { return !f.microsoftOff }

func (f *fakeSignIns) MicrosoftAuthURL(state, verifier, nonce string) string {
	f.verifier, f.nonce = verifier, nonce
	return "https://microsoft.example/authorize?state=" + url.QueryEscape(state)
}

func (f *fakeSignIns) MicrosoftUser(_ context.Context, code, verifier, nonce string) (microsoft.User, error) {
	subject, name, email, err := f.openIDUser("microsoft", code, verifier, nonce)
	return microsoft.User{Subject: subject, Name: name, Email: email}, err
}

func (f *fakeSignIns) GoogleReady() bool { return !f.googleOff }

func (f *fakeSignIns) GoogleAuthURL(state, verifier, nonce string) string {
	f.verifier, f.nonce = verifier, nonce
	return "https://google.example/authorize?state=" + url.QueryEscape(state)
}

func (f *fakeSignIns) GoogleUser(_ context.Context, code, verifier, nonce string) (google.User, error) {
	subject, name, email, err := f.openIDUser("google", code, verifier, nonce)
	return google.User{Subject: subject, Name: name, Email: email}, err
}

// openIDUser is who code signs in at provider, as the fake's Microsoft and Google describe.
func (f *fakeSignIns) openIDUser(provider, code, verifier, nonce string) (subject, name, email string, err error) {
	if verifier != f.verifier || nonce != f.nonce {
		return "", "", "", errors.New("verifier or nonce does not match")
	}
	switch code {
	case "good-code":
		return openIDSubject(provider, 'Q'), "Player One", "player@example.test", nil
	case "nameless-code":
		return openIDSubject(provider, 'R'), "", "player@example.test", nil
	case "bare-code":
		return openIDSubject(provider, 'S'), "", "", nil
	}
	return "", "", "", errors.New("invalid_grant")
}

// openIDSubject is an invented account id at provider, ending in last.
func openIDSubject(provider string, last rune) string {
	if provider == "google" {
		return "10000000000000000000" + string(last)
	}
	return "AAAAAAAAAAAAAAAAAAAAAIkzqFVrSaSaFHy782bbta" + string(last)
}

// openIDProviders are the ways in that share one flow: each one's id, name, and its fake's host.
var openIDProviders = []struct{ id, name, host string }{
	{"microsoft", "Microsoft", "microsoft.example"},
	{"google", "Google", "google.example"},
}

// startedOpenID starts a sign-in with provider (adding when the query says) and returns the flow cookie and the
// state sent.
func startedOpenID(t *testing.T, f *fakeSignIns, provider, query string) (*http.Cookie, string) {
	t.Helper()
	rec := playerDo(t, f, http.MethodGet, "/v1/auth/"+provider+"?"+query, "", "")
	if rec.Code != http.StatusFound {
		t.Fatalf("starting a %s sign-in = %d %s, want 302", provider, rec.Code, rec.Body)
	}
	to, err := url.Parse(rec.Header().Get("Location"))
	if err != nil || to.Host != provider+".example" {
		t.Fatalf("starting a %s sign-in went to %q, want %s", provider, rec.Header().Get("Location"), provider)
	}
	flow := cookieNamed(rec, flowCookie)
	if flow == nil || !flow.HttpOnly || !flow.Secure || flow.SameSite != http.SameSiteLaxMode || flow.Path != "/v1/auth/"+provider || flow.MaxAge <= 0 {
		t.Fatalf("flow cookie = %+v, want HttpOnly, Secure, Lax, on /v1/auth/%s, short-lived", flow, provider)
	}
	return &http.Cookie{Name: flowCookie, Value: flow.Value}, to.Query().Get("state")
}

func TestFinishingAMicrosoftOrGoogleSignIn(t *testing.T) {
	tests := []struct {
		name        string
		cookie      bool
		state       string // "" for the state the provider was sent
		query       string
		want        string
		wantSession bool
	}{
		{"the state matches", true, "", "code=good-code", siteOrigin + "/ratings/?signin=ok", true},
		{"another state", true, "another", "code=good-code", siteOrigin + "/account/?back=%2Fratings%2F&signin=failed", false},
		{"no flow cookie", false, "", "code=good-code", siteOrigin + "/account/?signin=failed", false},
		{"declined there", true, "", "error=access_denied", siteOrigin + "/account/?back=%2Fratings%2F&signin=failed", false},
		{"a code it refuses", true, "", "code=bad-code", siteOrigin + "/account/?back=%2Fratings%2F&signin=failed", false},
	}
	for _, p := range openIDProviders {
		for _, tt := range tests {
			t.Run(p.id+"/"+tt.name, func(t *testing.T) {
				f := newFakeSignIns()
				flow, state := startedOpenID(t, f, p.id, "return=%2Fratings%2F")
				if tt.state != "" {
					state = tt.state
				}
				var cookies []*http.Cookie
				if tt.cookie {
					cookies = append(cookies, flow)
				}
				rec := playerDo(t, f, http.MethodGet, "/v1/auth/"+p.id+"/callback?state="+url.QueryEscape(state)+"&"+tt.query, "", "", cookies...)
				if rec.Code != http.StatusFound || rec.Header().Get("Location") != tt.want {
					t.Errorf("callback = %d to %q, want 302 to %q", rec.Code, rec.Header().Get("Location"), tt.want)
				}
				if c := cookieNamed(rec, flowCookie); c == nil || c.MaxAge >= 0 || c.Path != "/v1/auth/"+p.id {
					t.Errorf("flow cookie after the callback = %+v, want it cleared on its path", c)
				}
				if session := cookieNamed(rec, sessionCookie); (session != nil) != tt.wantSession || len(f.sessions) != len(f.signedIn) {
					t.Errorf("session cookie = %+v and sessions %d, want a session %v", session, len(f.sessions), tt.wantSession)
				}
			})
		}
	}
}

func TestAMicrosoftOrGoogleLoginIsNamedByItsNameNeverItsEmail(t *testing.T) {
	tests := []struct {
		code      string
		last      rune
		wantName  string // "" for "<provider> account"
		wantEmail string
	}{
		{"good-code", 'Q', "Player One", "player@example.test"},
		{"nameless-code", 'R', "", "player@example.test"},
		{"bare-code", 'S', "", ""},
	}
	for _, p := range openIDProviders {
		for _, tt := range tests {
			t.Run(p.id+"/"+tt.code, func(t *testing.T) {
				f := newFakeSignIns()
				flow, state := startedOpenID(t, f, p.id, "")
				playerDo(t, f, http.MethodGet, "/v1/auth/"+p.id+"/callback?code="+tt.code+"&state="+url.QueryEscape(state), "", "", flow)
				want := store.Identity{Provider: p.id, Subject: openIDSubject(p.id, tt.last), Username: tt.wantName, Email: tt.wantEmail}
				if want.Username == "" {
					want.Username = p.name + " account"
				}
				if len(f.signedIn) != 1 || f.signedIn[0] != want {
					t.Errorf("signed in %+v, want %+v", f.signedIn, want)
				}
			})
		}
	}
}

func TestEachMicrosoftOrGoogleSignInHasItsOwnNonce(t *testing.T) {
	for _, p := range openIDProviders {
		t.Run(p.id, func(t *testing.T) {
			f := newFakeSignIns()
			startedOpenID(t, f, p.id, "")
			first := f.nonce
			startedOpenID(t, f, p.id, "")
			if first == "" || f.nonce == first {
				t.Errorf("nonces = %q then %q, want two different ones", first, f.nonce)
			}
		})
	}
}

func TestAddingMicrosoftOrGoogleFromTheAccountPage(t *testing.T) {
	tests := []struct {
		name     string
		account  int64
		signedIn bool
		want     string // with the provider's id after way=
		added    bool
	}{
		{"signed in", 7, true, "/account/?signin=added&way=", true},
		{"an account of its own", 10, true, "/account/?signin=taken&way=", false},
		{"an account with another of the kind", 9, true, "/account/?signin=has-kind&way=", false},
		{"signed out meanwhile", 7, false, "/account/?signin=failed&way=", false},
	}
	for _, p := range openIDProviders {
		for _, tt := range tests {
			t.Run(p.id+"/"+tt.name, func(t *testing.T) {
				f := newFakeSignIns()
				flow, state := startedOpenID(t, f, p.id, "add=true")
				if !strings.HasSuffix(flow.Value, "."+addMark) {
					t.Fatalf("flow cookie = %q, want it marked to add", flow.Value)
				}
				cookies := []*http.Cookie{flow}
				if tt.signedIn {
					cookies = append(cookies, sessionFor(f, "token", store.Account{ID: tt.account, Provider: "email"}))
				}
				rec := playerDo(t, f, http.MethodGet, "/v1/auth/"+p.id+"/callback?code=good-code&state="+url.QueryEscape(state), "", "", cookies...)
				if got, want := rec.Header().Get("Location"), siteOrigin+tt.want+p.id; got != want {
					t.Errorf("callback went to %q, want %q", got, want)
				}
				wantNote := ""
				if tt.added {
					wantNote = "add " + p.id + " Player One"
				}
				if got := strings.Join(f.notes, "|"); got != wantNote {
					t.Errorf("changes = %q, want %q", got, wantNote)
				}
				if len(f.signedIn) != 0 || cookieNamed(rec, sessionCookie) != nil {
					t.Errorf("adding %s signed in %+v with a new session cookie, want neither", p.name, f.signedIn)
				}
			})
		}
	}
}

func TestMicrosoftOrGoogleSignInRefusesWhenNotSetUp(t *testing.T) {
	for _, p := range openIDProviders {
		t.Run(p.id, func(t *testing.T) {
			f := newFakeSignIns()
			f.microsoftOff, f.googleOff = p.id == "microsoft", p.id == "google"
			for _, path := range []string{"/v1/auth/" + p.id, "/v1/auth/" + p.id + "/callback?code=good-code&state=x"} {
				if rec := playerDo(t, f, http.MethodGet, path, "", ""); rec.Code != http.StatusServiceUnavailable {
					t.Errorf("GET %s with %s not set up = %d, want 503", path, p.name, rec.Code)
				}
			}
		})
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
