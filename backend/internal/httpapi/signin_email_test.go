package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"ttacommunity/backend/internal/store"
	"ttacommunity/backend/internal/turnstile"
)

// The fake's email sign-in: many@ is over its limit, taken@ has an account of its own, bounce@ cannot be mailed;
// account 9 has Discord alone; Turnstile passes "pass" and cannot be asked for "down".

func (f *fakeSignIns) EmailReady() bool { return !f.emailOff }

func (f *fakeSignIns) NewEmailLink(_ context.Context, address string, account *int64, back string, hash []byte, _ time.Time) error {
	switch {
	case address == "many@example.test":
		return store.ErrTooManyLinks
	case account != nil && address == "taken@example.test":
		return store.ErrLoginTaken
	}
	f.links[string(hash)] = store.EmailSignIn{Back: back, Added: account != nil}
	return nil
}

func (f *fakeSignIns) UseEmailLink(_ context.Context, linkHash, sessionHash []byte, _ time.Time) (store.EmailSignIn, error) {
	got, ok := f.links[string(linkHash)]
	if !ok {
		return store.EmailSignIn{}, store.ErrLinkNotValid
	}
	delete(f.links, string(linkHash))
	f.sessions[string(sessionHash)] = store.Account{ID: 7, Provider: "email"}
	return got, nil
}

func (*fakeSignIns) VerifyTurnstile(_ context.Context, token string) error {
	switch token {
	case "pass":
		return nil
	case "down":
		return errors.New("siteverify answered 500")
	}
	return turnstile.ErrFailed
}

func (f *fakeSignIns) SendSignInLink(_ context.Context, address, link string) error {
	if address == "bounce@example.test" {
		return errors.New("resend: sending: status 422")
	}
	f.mails = append(f.mails, address+" "+link)
	return nil
}

func (f *fakeSignIns) AddLogin(_ context.Context, hash []byte, id store.Identity, _ time.Time) error {
	a, ok := f.sessions[string(hash)]
	switch {
	case !ok:
		return store.ErrNoSession
	case a.ID == 9:
		return store.ErrHasLoginOfKind
	case a.ID == 10:
		return store.ErrLoginTaken
	}
	f.notes = append(f.notes, "add "+id.Provider+" "+id.Username)
	return nil
}

func (f *fakeSignIns) RemoveOwnLogin(_ context.Context, hash []byte, provider string, _ time.Time) error {
	a, ok := f.sessions[string(hash)]
	switch {
	case !ok:
		return store.ErrNoSession
	case a.ID == 9 && provider == "discord":
		return store.ErrLastLogin
	case a.ID == 9:
		return store.ErrNoLogin
	}
	f.notes = append(f.notes, "remove "+provider)
	return nil
}

// mailedToken is the token in the one link f mailed to address, failing the test otherwise.
func mailedToken(t *testing.T, f *fakeSignIns, address string) string {
	t.Helper()
	prefix := address + " " + siteOrigin + "/account/email/#"
	if len(f.mails) != 1 || !strings.HasPrefix(f.mails[0], prefix) {
		t.Fatalf("mails = %q, want one link to %s on the site's email page", f.mails, address)
	}
	return strings.TrimPrefix(f.mails[0], prefix)
}

func TestAnEmailedLinkSignsInOnTheSite(t *testing.T) {
	f := newFakeSignIns()
	rec := playerDo(t, f, http.MethodPost, "/v1/auth/email", siteOrigin, `{"email":" Player@Example.test ","turnstile":"pass","return":"/ratings/"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("asking for a link = %d %s, want 204", rec.Code, rec.Body)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != siteOrigin || rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("CORS = %q, want the site's with credentials", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	token := mailedToken(t, f, "player@example.test")
	rec = playerDo(t, f, http.MethodPost, "/v1/auth/email/finish", siteOrigin, `{"token":"`+token+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("opening the link = %d %s, want 200", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "outcome", `"ok"`)
	wantJSON(t, rec.Body.Bytes(), "back", `"/ratings/"`)
	session := cookieNamed(rec, sessionCookie)
	if session == nil || !session.HttpOnly || !session.Secure || session.SameSite != http.SameSiteLaxMode || session.Domain != "" {
		t.Fatalf("session cookie = %+v, want host-only, HttpOnly, Secure, Lax", session)
	}
	if _, ok := f.sessions[string(hashToken(session.Value))]; !ok {
		t.Errorf("the session cookie's token is not the session kept")
	}
	if rec := playerDo(t, f, http.MethodPost, "/v1/auth/email/finish", siteOrigin, `{"token":"`+token+`"}`); rec.Code != http.StatusGone {
		t.Errorf("opening the link again = %d, want 410", rec.Code)
	}
}

func TestAskingForAnEmailedLinkIsRefused(t *testing.T) {
	tests := []struct {
		name, origin, body string
		emailOff           bool
		want               int
		wantMail           bool
	}{
		{"from another site", "https://evil.example", `{"email":"player@example.test","turnstile":"pass"}`, false, http.StatusForbidden, false},
		{"not an address", siteOrigin, `{"email":"player","turnstile":"pass"}`, false, http.StatusBadRequest, false},
		{"Turnstile not passed", siteOrigin, `{"email":"player@example.test","turnstile":"robot"}`, false, http.StatusBadRequest, false},
		{"Turnstile cannot be asked", siteOrigin, `{"email":"player@example.test","turnstile":"down"}`, false, http.StatusServiceUnavailable, false},
		{"over the limits", siteOrigin, `{"email":"many@example.test","turnstile":"pass"}`, false, http.StatusTooManyRequests, false},
		{"the mail is not sent", siteOrigin, `{"email":"bounce@example.test","turnstile":"pass"}`, false, http.StatusServiceUnavailable, false},
		{"email sign-in not set up", siteOrigin, `{"email":"player@example.test","turnstile":"pass"}`, true, http.StatusServiceUnavailable, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSignIns()
			f.emailOff = tt.emailOff
			rec := playerDo(t, f, http.MethodPost, "/v1/auth/email", tt.origin, tt.body)
			if rec.Code != tt.want {
				t.Errorf("asking for a link = %d %s, want %d", rec.Code, rec.Body, tt.want)
			}
			if (len(f.mails) > 0) != tt.wantMail {
				t.Errorf("mails = %q, want any %v", f.mails, tt.wantMail)
			}
		})
	}
}

func TestOpeningAnEmailedLinkIsRefused(t *testing.T) {
	f := newFakeSignIns()
	f.links[string(hashToken("good"))] = store.EmailSignIn{Back: "/account/"}
	for _, tt := range []struct {
		name, origin, token string
		want                int
	}{
		{"a link never sent", siteOrigin, "unknown", http.StatusGone},
		{"from another site", "https://evil.example", "good", http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := playerDo(t, f, http.MethodPost, "/v1/auth/email/finish", tt.origin, `{"token":"`+tt.token+`"}`)
			if rec.Code != tt.want || cookieNamed(rec, sessionCookie) != nil {
				t.Errorf("opening = %d with session cookie %v, want %d and none", rec.Code, cookieNamed(rec, sessionCookie), tt.want)
			}
		})
	}
	if _, ok := f.links[string(hashToken("good"))]; !ok {
		t.Errorf("a refused request spent the link")
	}
}

func TestAddingAnEmailAsAnotherWayIn(t *testing.T) {
	tests := []struct {
		name, body string
		account    int64
		signedIn   bool
		want       int
		wantMail   bool
	}{
		{"signed in", `{"email":"Player@example.test"}`, 7, true, http.StatusNoContent, true},
		{"signed out", `{"email":"player@example.test"}`, 7, false, http.StatusUnauthorized, false},
		{"an address with an account of its own", `{"email":"taken@example.test"}`, 7, true, http.StatusConflict, false},
		{"not an address", `{"email":"player"}`, 7, true, http.StatusBadRequest, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSignIns()
			var cookies []*http.Cookie
			if tt.signedIn {
				cookies = append(cookies, sessionFor(f, "token", store.Account{ID: tt.account, Provider: "discord"}))
			}
			rec := playerDo(t, f, http.MethodPost, "/v1/me/logins/email", siteOrigin, tt.body, cookies...)
			if rec.Code != tt.want {
				t.Errorf("adding an email = %d %s, want %d", rec.Code, rec.Body, tt.want)
			}
			if !tt.wantMail {
				if len(f.mails) != 0 {
					t.Errorf("mails = %q, want none", f.mails)
				}
				return
			}
			token := mailedToken(t, f, "player@example.test")
			rec = playerDo(t, f, http.MethodPost, "/v1/auth/email/finish", siteOrigin, `{"token":"`+token+`"}`)
			wantJSON(t, rec.Body.Bytes(), "outcome", `"added"`)
			wantJSON(t, rec.Body.Bytes(), "back", `"/account/"`)
		})
	}
}

func TestRemovingAWayIn(t *testing.T) {
	tests := []struct {
		name, provider string
		account        int64
		signedIn       bool
		want           int
		wantNote       string
	}{
		{"one of two", "discord", 7, true, http.StatusNoContent, "remove discord"},
		{"the only one", "discord", 9, true, http.StatusConflict, ""},
		{"one the account has not got", "email", 9, true, http.StatusNotFound, ""},
		{"signed out", "email", 7, false, http.StatusUnauthorized, ""},
		{"an unknown kind", "myspace", 7, true, http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSignIns()
			var cookies []*http.Cookie
			if tt.signedIn {
				cookies = append(cookies, sessionFor(f, "token", store.Account{ID: tt.account}))
			}
			rec := playerDo(t, f, http.MethodDelete, "/v1/me/logins/"+tt.provider, siteOrigin, "", cookies...)
			if rec.Code != tt.want {
				t.Errorf("removing %s = %d %s, want %d", tt.provider, rec.Code, rec.Body, tt.want)
			}
			if got := strings.Join(f.notes, "|"); got != tt.wantNote {
				t.Errorf("changes = %q, want %q", got, tt.wantNote)
			}
		})
	}
}

func TestAnUnknownWayInNamesEveryProvider(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("reading the spec: %v", err)
	}
	var spec struct {
		Components struct {
			Schemas struct {
				LoginProvider struct {
					Enum []string `yaml:"enum"`
				} `yaml:"LoginProvider"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parsing the spec: %v", err)
	}
	f := newFakeSignIns()
	rec := playerDo(t, f, http.MethodDelete, "/v1/me/logins/myspace", siteOrigin, "", sessionFor(f, "token", store.Account{ID: 7}))
	var body Error
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("removing myspace: body %q: %v", rec.Body, err)
	}
	for _, p := range spec.Components.Schemas.LoginProvider.Enum {
		if !strings.Contains(body.Error, p) {
			t.Errorf("removing myspace: error = %q, want it to name %q", body.Error, p)
		}
	}
}

func TestAddingDiscordFromTheAccountPage(t *testing.T) {
	tests := []struct {
		name     string
		account  int64
		signedIn bool
		want     string
		wantNote string
	}{
		{"signed in", 7, true, "/account/?signin=added&way=discord", "add discord player_one"},
		{"a Discord account with an account of its own", 10, true, "/account/?signin=taken&way=discord", ""},
		{"an account with another Discord", 9, true, "/account/?signin=has-kind&way=discord", ""},
		{"signed out meanwhile", 7, false, "/account/?signin=failed&way=discord", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSignIns()
			rec := playerDo(t, f, http.MethodGet, "/v1/auth/discord?add=true", "", "")
			flow := cookieNamed(rec, flowCookie)
			if flow == nil || !strings.HasSuffix(flow.Value, "."+addMark) {
				t.Fatalf("flow cookie = %+v, want it marked to add", flow)
			}
			to, _ := url.Parse(rec.Header().Get("Location"))
			cookies := []*http.Cookie{{Name: flowCookie, Value: flow.Value}}
			if tt.signedIn {
				cookies = append(cookies, sessionFor(f, "token", store.Account{ID: tt.account, Provider: "email"}))
			}
			rec = playerDo(t, f, http.MethodGet, "/v1/auth/discord/callback?code=good-code&state="+url.QueryEscape(to.Query().Get("state")), "", "", cookies...)
			if got := rec.Header().Get("Location"); got != siteOrigin+tt.want {
				t.Errorf("callback went to %q, want %q", got, siteOrigin+tt.want)
			}
			if got := strings.Join(f.notes, "|"); got != tt.wantNote {
				t.Errorf("changes = %q, want %q", got, tt.wantNote)
			}
			if len(f.signedIn) != 0 || cookieNamed(rec, sessionCookie) != nil {
				t.Errorf("adding Discord signed in %+v with a new session cookie %v, want neither", f.signedIn, cookieNamed(rec, sessionCookie))
			}
		})
	}
}

func TestMeListsEveryWayIn(t *testing.T) {
	f := newFakeSignIns()
	cookie := sessionFor(f, "token", store.Account{ID: 7, Provider: "email", Username: "player@example.test",
		Logins: []store.LoginName{{Provider: "discord", Username: "player_one"}, {Provider: "email", Username: "player@example.test"}}})
	rec := playerDo(t, f, http.MethodGet, "/v1/me", siteOrigin, "", cookie)
	wantJSON(t, rec.Body.Bytes(), "provider", `"email"`)
	wantJSON(t, rec.Body.Bytes(), "logins.0.provider", `"discord"`)
	wantJSON(t, rec.Body.Bytes(), "logins.1.username", `"player@example.test"`)
}
