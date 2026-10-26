package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/discord"
	"ttacommunity/backend/internal/store"
)

const (
	siteOrigin = "https://site.example"
)

// fakeSignIns keeps sessions by their token's hash, and stands in for Discord: code good-code signs in
// player_one, and only with the verifier its last authorize URL carried.
type fakeSignIns struct {
	// firstTime makes every sign-in the account's first.
	firstTime bool
	sessions  map[string]store.Account
	signedIn  []store.Identity
	verifier  string
	notes     []string
	signedOut int
	deleted   int
	// emailOff turns email sign-in off; links are the emailed links waiting, mails what was sent.
	emailOff bool
	links    map[string]store.EmailSignIn
	mails    []string
	// reportErr, when set, is what blitz reports, confirmations and flags answer (blitz_reports_test.go).
	reportErr error
	// microsoftOff and googleOff turn their sign-in off; nonce is the one the last authorize URL carried.
	microsoftOff, googleOff bool
	nonce                   string
	// kept is what KeptData answers for any live session.
	kept store.KeptData
}

func newFakeSignIns() *fakeSignIns {
	return &fakeSignIns{sessions: map[string]store.Account{}, links: map[string]store.EmailSignIn{}}
}

func (f *fakeSignIns) SignIn(_ context.Context, id store.Identity, hash []byte, _ time.Time) (bool, error) {
	f.signedIn = append(f.signedIn, id)
	f.sessions[string(hash)] = store.Account{ID: 7, Provider: id.Provider, Username: id.Username, DisplayName: id.DisplayName}
	return f.firstTime, nil
}

func (f *fakeSignIns) Session(_ context.Context, hash []byte, _ time.Time) (store.Account, error) {
	l, ok := f.sessions[string(hash)]
	if !ok {
		return store.Account{}, store.ErrNoSession
	}
	return l, nil
}

func (f *fakeSignIns) KeptData(_ context.Context, hash []byte, _ time.Time) (store.KeptData, error) {
	if _, ok := f.sessions[string(hash)]; !ok {
		return store.KeptData{}, store.ErrNoSession
	}
	return f.kept, nil
}

func (f *fakeSignIns) SignOut(_ context.Context, hash []byte) error {
	delete(f.sessions, string(hash))
	f.signedOut++
	return nil
}

func (f *fakeSignIns) DeleteAccount(_ context.Context, hash []byte, _ time.Time) error {
	if _, ok := f.sessions[string(hash)]; !ok {
		return store.ErrNoSession
	}
	delete(f.sessions, string(hash))
	f.deleted++
	return nil
}

func (f *fakeSignIns) RequestLink(_ context.Context, login int64, name string, _ time.Time) error {
	switch {
	case strings.TrimSpace(name) == "":
		return store.ErrBadLinkName
	case login == 8:
		return store.ErrAlreadyLinked
	case name == "taken":
		return store.ErrNameLinked
	}
	f.notes = append(f.notes, "request "+name)
	return nil
}

func (f *fakeSignIns) SetOwnDetails(_ context.Context, _ int64, country, zone *string) error {
	if country != nil && *country == "ZZ" {
		return store.ErrBadMember
	}
	note := "details"
	if country != nil {
		note += " country=" + *country
	}
	if zone != nil {
		note += " zone=" + *zone
	}
	f.notes = append(f.notes, note)
	return nil
}

func (*fakeSignIns) SiteOrigin() string { return siteOrigin }

func (*fakeSignIns) SupportAddress() string { return "support@example.test" }

func (f *fakeSignIns) DiscordAuthURL(state, verifier string) string {
	f.verifier = verifier
	return "https://discord.example/authorize?state=" + url.QueryEscape(state)
}

func (f *fakeSignIns) DiscordUser(_ context.Context, code, verifier string) (discord.User, error) {
	if verifier != f.verifier {
		return discord.User{}, errors.New("verifier does not match")
	}
	switch code {
	case "good-code":
		return discord.User{ID: "100000000000000001", Username: "player_one", DisplayName: "Player One"}, nil
	}
	return discord.User{}, errors.New("invalid_grant")
}

// playerDo sends a request to a mux with sign-ins, from origin (none when empty), with cookies.
func playerDo(t *testing.T, signIns SignIns, method, path, origin, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	NewMux(&fakeRatings{}, nil, signIns, nil, nil, slog.New(slog.DiscardHandler), &recordedRequests{}, stepClock(time.Millisecond)).ServeHTTP(rec, req)
	return rec
}

// cookieNamed returns the cookie rec sets with that name, or nil.
func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// sessionFor gives f a session for login and returns its cookie.
func sessionFor(f *fakeSignIns, token string, login store.Account) *http.Cookie {
	f.sessions[string(hashToken(token))] = login
	return &http.Cookie{Name: sessionCookie, Value: token}
}

// started starts a Discord sign-in returning to back, and returns the flow cookie and the state Discord was sent.
func started(t *testing.T, f *fakeSignIns, back string) (*http.Cookie, string) {
	t.Helper()
	rec := playerDo(t, f, http.MethodGet, "/v1/auth/discord?return="+url.QueryEscape(back), "", "")
	if rec.Code != http.StatusFound {
		t.Fatalf("starting a sign-in = %d %s, want 302", rec.Code, rec.Body)
	}
	to, err := url.Parse(rec.Header().Get("Location"))
	if err != nil || to.Host != "discord.example" {
		t.Fatalf("starting a sign-in went to %q, want Discord", rec.Header().Get("Location"))
	}
	flow := cookieNamed(rec, flowCookie)
	if flow == nil || !flow.HttpOnly || !flow.Secure || flow.SameSite != http.SameSiteLaxMode || flow.Path != "/v1/auth/discord" || flow.MaxAge <= 0 {
		t.Fatalf("flow cookie = %+v, want HttpOnly, Secure, Lax, on /v1/auth/discord, short-lived", flow)
	}
	if rec.Header().Get("Cache-Control") != cacheNone {
		t.Errorf("starting a sign-in's Cache-Control = %q, want %q", rec.Header().Get("Cache-Control"), cacheNone)
	}
	return &http.Cookie{Name: flowCookie, Value: flow.Value}, to.Query().Get("state")
}

func TestFinishingADiscordSignIn(t *testing.T) {
	tests := []struct {
		name        string
		cookie      bool
		state       string // "" for the state Discord was sent
		query       string
		want        string
		wantSession bool
	}{
		{"the state matches", true, "", "code=good-code", siteOrigin + "/ratings/?signin=ok", true},
		{"another state", true, "another", "code=good-code", siteOrigin + "/account/?back=%2Fratings%2F&signin=failed", false},
		{"no flow cookie", false, "", "code=good-code", siteOrigin + "/account/?signin=failed", false},
		{"declined at Discord", true, "", "error=access_denied", siteOrigin + "/account/?back=%2Fratings%2F&signin=failed", false},
		{"no code", true, "", "", siteOrigin + "/account/?back=%2Fratings%2F&signin=failed", false},
		{"a code Discord refuses", true, "", "code=bad-code", siteOrigin + "/account/?back=%2Fratings%2F&signin=failed", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSignIns()
			flow, state := started(t, f, "/ratings/")
			if tt.state != "" {
				state = tt.state
			}
			var cookies []*http.Cookie
			if tt.cookie {
				cookies = append(cookies, flow)
			}
			q := "state=" + url.QueryEscape(state)
			if tt.query != "" {
				q += "&" + tt.query
			}
			rec := playerDo(t, f, http.MethodGet, "/v1/auth/discord/callback?"+q, "", "", cookies...)
			if rec.Code != http.StatusFound || rec.Header().Get("Location") != tt.want {
				t.Errorf("callback = %d to %q, want 302 to %q", rec.Code, rec.Header().Get("Location"), tt.want)
			}
			if c := cookieNamed(rec, flowCookie); c == nil || c.MaxAge >= 0 {
				t.Errorf("flow cookie after the callback = %+v, want it cleared", c)
			}
			session := cookieNamed(rec, sessionCookie)
			if (session != nil) != tt.wantSession {
				t.Fatalf("session cookie = %+v, want one %v", session, tt.wantSession)
			}
			if !tt.wantSession {
				if len(f.sessions) != 0 {
					t.Errorf("sessions = %d, want none: a failed sign-in leaves the player signed out", len(f.sessions))
				}
				return
			}
			if !session.HttpOnly || !session.Secure || session.SameSite != http.SameSiteLaxMode || session.Path != "/" || session.Domain != "" {
				t.Errorf("session cookie = %+v, want host-only, HttpOnly, Secure, Lax, on /", session)
			}
			want := store.Identity{Provider: "discord", Subject: "100000000000000001", Username: "player_one", DisplayName: "Player One"}
			if len(f.signedIn) != 1 || f.signedIn[0] != want {
				t.Errorf("signed in %+v, want %+v", f.signedIn, want)
			}
			if _, ok := f.sessions[string(hashToken(session.Value))]; !ok {
				t.Errorf("the session cookie's token is not the session kept: the store must hold its hash")
			}
		})
	}
}

func TestASignInReturnsOnlyToAPathOnTheSite(t *testing.T) {
	for _, tt := range []struct{ back, want string }{
		{"/blitz/", "/blitz/?signin=ok"},
		{"/ratings/player/?name=a+b", "/ratings/player/?name=a+b&signin=ok"},
		{"/blitz/?t=us_open&e=2026", "/blitz/?t=us_open&e=2026&signin=ok"},
		{"", "/account/?signin=ok"},
		{"//evil.example/", "/account/?signin=ok"},
		{"https://evil.example/", "/account/?signin=ok"},
		{"/\\evil.example", "/account/?signin=ok"},
		{"/ratings/#top", "/account/?signin=ok"},
		{"/" + strings.Repeat("a", 300), "/account/?signin=ok"},
	} {
		t.Run(tt.back, func(t *testing.T) {
			f := newFakeSignIns()
			flow, state := started(t, f, tt.back)
			rec := playerDo(t, f, http.MethodGet, "/v1/auth/discord/callback?code=good-code&state="+url.QueryEscape(state), "", "", flow)
			if got, want := rec.Header().Get("Location"), siteOrigin+tt.want; got != want {
				t.Errorf("returning to %q went to %q, want %q", tt.back, got, want)
			}
		})
	}
}

func TestAFirstSignInOpensTheAccountPageWithTheWayBack(t *testing.T) {
	for _, tt := range []struct{ back, want string }{
		{"/blitz/?t=us_open&e=2026", "/account/?back=%2Fblitz%2F%3Ft%3Dus_open%26e%3D2026&signin=first"},
		{"/account/", "/account/?signin=first"},
		{"//evil.example/", "/account/?signin=first"},
	} {
		t.Run(tt.back, func(t *testing.T) {
			f := newFakeSignIns()
			f.firstTime = true
			flow, state := started(t, f, tt.back)
			rec := playerDo(t, f, http.MethodGet, "/v1/auth/discord/callback?code=good-code&state="+url.QueryEscape(state), "", "", flow)
			if got, want := rec.Header().Get("Location"), siteOrigin+tt.want; got != want {
				t.Errorf("a first sign-in from %q went to %q, want %q", tt.back, got, want)
			}
		})
	}
}

func TestMeAnswersTheSiteAboutItsSession(t *testing.T) {
	f := newFakeSignIns()
	org := store.RoleOrganiser
	linked := sessionFor(f, "linked", store.Account{ID: 8, Provider: "discord", Username: "player_one", DisplayName: "Player One",
		Member:  &store.LinkedMember{ID: 3, Name: "ann", Player: "Ann"},
		Country: "NZ", Zone: "Pacific/Auckland", Banned: true, BanReason: "Left games unfinished",
		Request:   &store.LinkRequest{Name: "ann", Status: "approved", RequestedAt: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)},
		AdminRole: org})
	tests := []struct {
		name     string
		origin   string
		cookies  []*http.Cookie
		wantCode int
		wantCORS bool
	}{
		{"a session, from the site", siteOrigin, []*http.Cookie{linked}, http.StatusOK, true},
		{"no session, from the site", siteOrigin, nil, http.StatusUnauthorized, true},
		{"an ended session", siteOrigin, []*http.Cookie{{Name: sessionCookie, Value: "ended"}}, http.StatusUnauthorized, true},
		{"a session, from another site", "https://evil.example", []*http.Cookie{linked}, http.StatusOK, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := playerDo(t, f, http.MethodGet, "/v1/me", tt.origin, "", tt.cookies...)
			if rec.Code != tt.wantCode {
				t.Fatalf("GET /v1/me = %d %s, want %d", rec.Code, rec.Body, tt.wantCode)
			}
			allowed := rec.Header().Get("Access-Control-Allow-Origin") == siteOrigin && rec.Header().Get("Access-Control-Allow-Credentials") == "true"
			if allowed != tt.wantCORS || rec.Header().Get("Access-Control-Allow-Origin") == "*" {
				t.Errorf("CORS = %q, credentials %q; want the site's alone %v", rec.Header().Get("Access-Control-Allow-Origin"),
					rec.Header().Get("Access-Control-Allow-Credentials"), tt.wantCORS)
			}
			if rec.Header().Get("Cache-Control") != cacheNone {
				t.Errorf("Cache-Control = %q, want %q", rec.Header().Get("Cache-Control"), cacheNone)
			}
		})
	}
	rec := playerDo(t, f, http.MethodGet, "/v1/me", siteOrigin, "", linked)
	wantJSON(t, rec.Body.Bytes(), "username", `"player_one"`)
	wantJSON(t, rec.Body.Bytes(), "member.name", `"ann"`)
	wantJSON(t, rec.Body.Bytes(), "member.banned", `true`)
	wantJSON(t, rec.Body.Bytes(), "member.ban_reason", `"Left games unfinished"`)
	wantJSON(t, rec.Body.Bytes(), "country", `"NZ"`)
	wantJSON(t, rec.Body.Bytes(), "zone", `"Pacific/Auckland"`)
	wantJSON(t, rec.Body.Bytes(), "request.status", `"approved"`)
	wantJSON(t, rec.Body.Bytes(), "admin_role", `"organiser"`)
}

func TestPlayerChangesComeOnlyFromTheSite(t *testing.T) {
	tests := []struct {
		name, method, path, origin string
		want                       int
	}{
		{"asking to be linked from another site", http.MethodPut, "/v1/me/link-request", "https://evil.example", http.StatusForbidden},
		{"asking to be linked with no Origin", http.MethodPut, "/v1/me/link-request", "", http.StatusForbidden},
		{"signing out from another site", http.MethodPost, "/v1/auth/sign-out", "https://evil.example", http.StatusForbidden},
		{"deleting an account from another site", http.MethodDelete, "/v1/me", "https://evil.example", http.StatusForbidden},
		{"deleting an account with no Origin", http.MethodDelete, "/v1/me", "", http.StatusForbidden},
		{"a preflight from another site", http.MethodOptions, "/v1/me/link-request", "https://evil.example", http.StatusForbidden},
		{"a preflight from the site", http.MethodOptions, "/v1/me/link-request", siteOrigin, http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSignIns()
			rec := playerDo(t, f, tt.method, tt.path, tt.origin, `{"name":"ann"}`, sessionFor(f, "token", store.Account{ID: 7}))
			if rec.Code != tt.want {
				t.Errorf("%s %s from %q = %d, want %d", tt.method, tt.path, tt.origin, rec.Code, tt.want)
			}
			if tt.want == http.StatusForbidden && (len(f.notes) != 0 || f.signedOut != 0 || f.deleted != 0) {
				t.Errorf("a refused change reached the store: %q, %d signed out, %d deleted", f.notes, f.signedOut, f.deleted)
			}
		})
	}
	rec := playerDo(t, newFakeSignIns(), http.MethodOptions, "/v1/me/details", siteOrigin, "")
	if !strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), "PATCH") || rec.Header().Get("Access-Control-Allow-Headers") != "Content-Type" {
		t.Errorf("preflight allows %q with %q, want PATCH with Content-Type", rec.Header().Get("Access-Control-Allow-Methods"),
			rec.Header().Get("Access-Control-Allow-Headers"))
	}
	if rec := playerDo(t, newFakeSignIns(), http.MethodOptions, "/v1/me", siteOrigin, ""); !strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), "DELETE") {
		t.Errorf("preflight for /v1/me allows %q, want DELETE", rec.Header().Get("Access-Control-Allow-Methods"))
	}
}

func TestAskingToBeLinked(t *testing.T) {
	tests := []struct {
		name  string
		login int64
		body  string
		want  int
	}{
		{"a name", 7, `{"name":"ann"}`, http.StatusNoContent},
		{"an empty name", 7, `{"name":" "}`, http.StatusBadRequest},
		{"no body", 7, ``, http.StatusBadRequest},
		{"already linked", 8, `{"name":"ann"}`, http.StatusConflict},
		{"a name another account holds", 7, `{"name":"taken"}`, http.StatusConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSignIns()
			rec := playerDo(t, f, http.MethodPut, "/v1/me/link-request", siteOrigin, tt.body, sessionFor(f, "token", store.Account{ID: tt.login}))
			if rec.Code != tt.want {
				t.Errorf("PUT /v1/me/link-request %s = %d %s, want %d", tt.body, rec.Code, rec.Body, tt.want)
			}
		})
	}
	f := newFakeSignIns()
	rec := playerDo(t, f, http.MethodPut, "/v1/me/link-request", siteOrigin, `{"name":"taken"}`, sessionFor(f, "token", store.Account{ID: 7}))
	if body := rec.Body.String(); !strings.Contains(body, "linked to another account") || !strings.Contains(body, "support@example.test") {
		t.Errorf("a name another account holds = %s, want it said, with where to write", body)
	}
	f = newFakeSignIns()
	playerDo(t, f, http.MethodPut, "/v1/me/link-request", siteOrigin, `{"name":"ann"}`)
	if len(f.notes) != 0 {
		t.Errorf("a request with no session reached the store: %q", f.notes)
	}
}

func TestSettingYourOwnDetails(t *testing.T) {
	tests := []struct {
		name     string
		login    int64
		body     string
		want     int
		wantNote string
	}{
		{"a country and a zone", 8, `{"country":"de","zone":"Europe/Berlin"}`, http.StatusNoContent, "details country=de zone=Europe/Berlin"},
		{"a region for a zone", 8, `{"zone":"US East"}`, http.StatusNoContent, "details zone=US East"},
		{"before being linked", 7, `{"country":"de"}`, http.StatusNoContent, "details country=de"},
		{"a zone that is not one", 8, `{"zone":"Mars/Olympus"}`, http.StatusBadRequest, ""},
		{"a country that is not one", 8, `{"country":"ZZ"}`, http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSignIns()
			rec := playerDo(t, f, http.MethodPatch, "/v1/me/details", siteOrigin, tt.body, sessionFor(f, "token", store.Account{ID: tt.login}))
			if rec.Code != tt.want {
				t.Errorf("PATCH /v1/me/details %s = %d %s, want %d", tt.body, rec.Code, rec.Body, tt.want)
			}
			if got := strings.Join(f.notes, "|"); got != tt.wantNote {
				t.Errorf("changes = %q, want %q", got, tt.wantNote)
			}
		})
	}
}

func TestSigningOutEndsTheSessionAndClearsItsCookie(t *testing.T) {
	f := newFakeSignIns()
	cookie := sessionFor(f, "token", store.Account{ID: 7})
	rec := playerDo(t, f, http.MethodPost, "/v1/auth/sign-out", siteOrigin, "", cookie)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("sign-out = %d %s, want 204", rec.Code, rec.Body)
	}
	if len(f.sessions) != 0 {
		t.Errorf("sessions after signing out = %d, want none", len(f.sessions))
	}
	if c := cookieNamed(rec, sessionCookie); c == nil || c.MaxAge >= 0 {
		t.Errorf("session cookie after signing out = %+v, want it cleared", c)
	}
	if rec := playerDo(t, f, http.MethodPost, "/v1/auth/sign-out", siteOrigin, ""); rec.Code != http.StatusNoContent {
		t.Errorf("signing out when not signed in = %d, want 204", rec.Code)
	}
}

func TestDeletingYourAccountEndsItAndClearsTheCookie(t *testing.T) {
	f := newFakeSignIns()
	rec := playerDo(t, f, http.MethodDelete, "/v1/me", siteOrigin, "", sessionFor(f, "token", store.Account{ID: 7}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /v1/me = %d %s, want 204", rec.Code, rec.Body)
	}
	if f.deleted != 1 {
		t.Errorf("accounts deleted = %d, want 1", f.deleted)
	}
	if c := cookieNamed(rec, sessionCookie); c == nil || c.MaxAge >= 0 {
		t.Errorf("session cookie after deleting = %+v, want it cleared", c)
	}
	if rec := playerDo(t, f, http.MethodDelete, "/v1/me", siteOrigin, ""); rec.Code != http.StatusUnauthorized || f.deleted != 1 {
		t.Errorf("deleting when not signed in = %d with %d deleted, want 401 and nothing more deleted", rec.Code, f.deleted)
	}
}

func TestPlayerRoutesRefuseWhenSignInIsNotSetUp(t *testing.T) {
	for _, path := range []string{"/v1/me", "/v1/auth/discord", "/v1/auth/discord/callback?code=x&state=y"} {
		rec := playerDo(t, nil, http.MethodGet, path, siteOrigin, "")
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("GET %s with no sign-in set up = %d, want 503", path, rec.Code)
		}
	}
}

func TestMeShowsNoMemberOrRequestAsNull(t *testing.T) {
	f := newFakeSignIns()
	rec := playerDo(t, f, http.MethodGet, "/v1/me", siteOrigin, "", sessionFor(f, "token", store.Account{ID: 7, Provider: "discord", Username: "player_two"}))
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("GET /v1/me body %s: %v", rec.Body, err)
	}
	for _, k := range []string{"member", "request", "admin_role"} {
		if v, ok := got[k]; !ok || v != nil {
			t.Errorf("%s = %v (present %v), want null", k, v, ok)
		}
	}
}

func TestMyDataAnswersWithEverythingKept(t *testing.T) {
	f := newFakeSignIns()
	// The store's times come in the server's zone; the api answers in UTC.
	at := time.Date(2026, 10, 18, 23, 0, 0, 0, time.FixedZone("AEDT", 11*60*60))
	later, cge := at.Add(time.Hour), 501
	f.kept = store.KeptData{
		Account: store.KeptAccount{CreatedAt: at, LinkedHow: "approved", LinkedAt: &later, AutoLink: true},
		Logins: []store.KeptLogin{{Provider: "discord", Subject: "100000000000000001", Username: "player_one", DisplayName: "Player One",
			CreatedAt: at, SignedInAt: later}},
		Member: &store.Member{Name: "Ann", CGEUser: &cge, Player: "Ann", DiscordName: "player_one", Email: "ann@example.test",
			Country: "NZ", Zone: "Pacific/Auckland", Banned: true, BanReason: "Abusive", BannedAt: &later, CreatedAt: at},
		MemberEmailCleared: true,
		Requests:           []store.KeptRequest{{Name: "Ann", Status: "approved", RequestedAt: at, DecidedAt: &later}},
		Sessions:           []store.KeptSession{{Provider: "discord", CreatedAt: at, Ends: at.Add(store.SessionLife), ThisBrowser: true}},
		EmailLinks:         []store.KeptEmailLink{{Address: "ann@example.test", ReturnPath: "/account", CreatedAt: at, ExpiresAt: later, KeptUntil: at.Add(48 * time.Hour)}},
	}
	rec := playerDo(t, f, http.MethodGet, "/v1/me/data", siteOrigin, "", sessionFor(f, "token", store.Account{ID: 7}))
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != cacheNone {
		t.Fatalf("GET /v1/me/data = %d %s, Cache-Control %q; want 200, %q", rec.Code, rec.Body, rec.Header().Get("Cache-Control"), cacheNone)
	}
	for path, want := range map[string]string{
		"account.created_at":       `"2026-10-18T12:00:00Z"`,
		"account.country":          `null`,
		"account.linked_how":       `"approved"`,
		"account.linked_at":        `"2026-10-18T13:00:00Z"`,
		"account.auto_link":        `true`,
		"logins.0.id":              `"100000000000000001"`,
		"logins.0.signed_in_at":    `"2026-10-18T13:00:00Z"`,
		"member.email":             `"ann@example.test"`,
		"member.cge_user":          `501`,
		"member.email_cleared":     `true`,
		"member.country":           `"NZ"`,
		"member.banned_at":         `"2026-10-18T13:00:00Z"`,
		"requests.0.decided_at":    `"2026-10-18T13:00:00Z"`,
		"sessions.0.ends_at":       `"2026-11-17T12:00:00Z"`,
		"sessions.0.this_browser":  `true`,
		"email_links.0.used_at":    `null`,
		"email_links.0.kept_until": `"2026-10-20T12:00:00Z"`,
	} {
		wantJSON(t, rec.Body.Bytes(), path, want)
	}
}

func TestMyDataShowsNoLinkAsNull(t *testing.T) {
	f := newFakeSignIns()
	f.kept = store.KeptData{Account: store.KeptAccount{CreatedAt: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC), Country: "NZ", Zone: "Pacific/Auckland"}}
	rec := playerDo(t, f, http.MethodGet, "/v1/me/data", siteOrigin, "", sessionFor(f, "token", store.Account{ID: 7}))
	for path, want := range map[string]string{
		"member": `null`, "account.linked_how": `null`, "account.linked_at": `null`, "account.zone": `"Pacific/Auckland"`,
		"logins": `[]`, "requests": `[]`, "sessions": `[]`, "email_links": `[]`,
	} {
		wantJSON(t, rec.Body.Bytes(), path, want)
	}
}

func TestMyDataNeedsASession(t *testing.T) {
	f := newFakeSignIns()
	if rec := playerDo(t, f, http.MethodGet, "/v1/me/data", siteOrigin, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /v1/me/data signed out = %d %s, want 401", rec.Code, rec.Body)
	}
}

func TestAnyoneReadsTheCountriesAPlayerMayChoose(t *testing.T) {
	rec, _ := serve(t, &fakeRatings{}, "/v1/countries")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/countries = %d %s", rec.Code, rec.Body)
	}
	var codes []string
	if err := json.Unmarshal(rec.Body.Bytes(), &codes); err != nil || len(codes) < 200 || codes[0] != "AD" {
		t.Errorf("codes = %d starting %q (err %v), want the ISO list from AD", len(codes), codes[:min(1, len(codes))], err)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheDay {
		t.Errorf("Cache-Control = %q, want %q", got, cacheDay)
	}
}
