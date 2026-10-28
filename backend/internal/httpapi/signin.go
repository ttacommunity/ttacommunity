// Player sign-in: signing in with Discord, Microsoft or Google (or adding one as another way in) and out; the
// signed-in player's own routes are in me.go. /v1/me, sign-out and the email routes are fetched by the site alone, with
// credentials; signing in with Discord, Microsoft or Google is a page the browser opens.
package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ttacommunity/backend/internal/discord"
	"ttacommunity/backend/internal/openid"
	"ttacommunity/backend/internal/store"
)

// SignIns signs players in with Discord, Microsoft, Google or email, and keeps their accounts, sessions, members and
// requests.
type SignIns interface {
	// SignIn reports whether this was the account's first sign-in.
	SignIn(ctx context.Context, id store.Identity, tokenHash []byte, now time.Time) (bool, error)
	Session(ctx context.Context, tokenHash []byte, now time.Time) (store.Account, error)
	// KeptData is everything kept about the player whose session it is.
	KeptData(ctx context.Context, tokenHash []byte, now time.Time) (store.KeptData, error)
	SignOut(ctx context.Context, tokenHash []byte) error
	// DeleteAccount deletes the account whose session hashes to tokenHash; an ended session is store.ErrNoSession.
	DeleteAccount(ctx context.Context, tokenHash []byte, now time.Time) error
	RequestLink(ctx context.Context, login int64, name string, now time.Time) error
	SetOwnDetails(ctx context.Context, login int64, country, zone *string) error
	// ReportBlitzGame, ConfirmBlitzResult and FlagBlitzResult are a player's report of their own blitz match, and the
	// opponent's answer to it, as the session hashing to tokenHash.
	ReportBlitzGame(ctx context.Context, tokenHash []byte, slug, edition string, matchID int64, g store.BlitzGame, now time.Time) error
	ConfirmBlitzResult(ctx context.Context, tokenHash []byte, slug, edition string, matchID int64, now time.Time) error
	FlagBlitzResult(ctx context.Context, tokenHash []byte, slug, edition string, matchID int64, reason string, now time.Time) error
	// SiteOrigin is the site's origin: the only one players' requests may come from, and where sign-in returns.
	SiteOrigin() string
	// SupportAddress is where players write with trouble, or "" when none is set.
	SupportAddress() string
	// DiscordAuthURL and DiscordUser start and finish a sign-in with Discord.
	DiscordAuthURL(state, verifier string) string
	DiscordUser(ctx context.Context, code, verifier string) (discord.User, error)
	// OpenIDReady reports whether signing in with provider ("microsoft" or "google") is set up here; OpenIDAuthURL
	// and OpenIDUser start and finish one.
	OpenIDReady(provider string) bool
	OpenIDAuthURL(provider, state, verifier, nonce string) string
	OpenIDUser(ctx context.Context, provider, code, verifier, nonce string) (openid.User, error)
	// AddLogin adds a Discord, Microsoft or Google account to the account whose session hashes to tokenHash.
	AddLogin(ctx context.Context, tokenHash []byte, id store.Identity, now time.Time) error
	// RemoveOwnLogin removes one way into the account whose session hashes to tokenHash.
	RemoveOwnLogin(ctx context.Context, tokenHash []byte, provider string, now time.Time) error
	// EmailReady reports whether signing in by email is set up here; the methods after it need it.
	EmailReady() bool
	NewEmailLink(ctx context.Context, address string, account *int64, back string, tokenHash []byte, now time.Time) error
	UseEmailLink(ctx context.Context, linkHash, sessionHash []byte, now time.Time) (store.EmailSignIn, error)
	// VerifyTurnstile checks the email form's Turnstile token: turnstile.ErrFailed when it did not pass.
	VerifyTurnstile(ctx context.Context, token string) error
	// SendSignInLink emails link to address.
	SendSignInLink(ctx context.Context, address, link string) error
}

// The cookies: the session's opaque token, and a sign-in's state, verifier and return path until Discord, Microsoft
// or Google answers, on that provider's path.
const (
	sessionCookie = "ttac_session"
	flowCookie    = "ttac_signin"
	discordPath   = "/v1/auth/discord"
	flowLife      = 10 * time.Minute
	// cookieLife outlasts any session: the session itself ends after store.SessionLife unused.
	cookieLife = 400 * 24 * time.Hour
)

// signInPaths are the routes signInGate fronts.
func isSignInPath(path string) bool {
	return path == "/v1/me" || strings.HasPrefix(path, "/v1/me/") || strings.HasPrefix(path, "/v1/auth/")
}

// fetchedBySite reports whether a sign-in path is fetched by the site (with CORS) rather than opened as a page.
func fetchedBySite(path string) bool {
	return path == "/v1/me" || strings.HasPrefix(path, "/v1/me/") || path == "/v1/auth/sign-out" || strings.HasPrefix(path, "/v1/auth/email")
}

type (
	sessionKey struct{}
	accountKey struct{}
	flowKey    struct{}
)

// hashToken is what the store keeps of a session token.
func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// signInGate fronts the sign-in routes. A fetched route gets CORS for the site's origin alone, with credentials;
// a change from anywhere else, or a preflight, is refused, and /v1/me needs a live session, whose login it puts
// in the context. Every route gets its cookies' values in the context. With signIns nil, every route answers 503.
func signInGate(next http.Handler, signIns SignIns, now func() time.Time, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", cacheNone)
		if signIns == nil {
			writeError(w, http.StatusServiceUnavailable, "player sign-in is not set up here")
			return
		}
		ctx := r.Context()
		if c, err := r.Cookie(flowCookie); err == nil {
			ctx = context.WithValue(ctx, flowKey{}, c.Value)
		}
		if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
			ctx = context.WithValue(ctx, sessionKey{}, hashToken(c.Value))
		}
		if !fetchedBySite(r.URL.Path) {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		site, fromSite := signIns.SiteOrigin(), r.Header.Get("Origin") == signIns.SiteOrigin()
		w.Header().Add("Vary", "Origin")
		if fromSite {
			w.Header().Set("Access-Control-Allow-Origin", site)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !fromSite {
			log.Info("player change refused: not from the site")
			writeError(w, http.StatusForbidden, "changes come only from the site")
			return
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, PATCH, POST, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/auth/") {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		hash, _ := ctx.Value(sessionKey{}).([]byte)
		if hash == nil {
			writeError(w, http.StatusUnauthorized, "not signed in")
			return
		}
		login, err := signIns.Session(ctx, hash, now())
		if errors.Is(err, store.ErrNoSession) {
			writeError(w, http.StatusUnauthorized, "not signed in")
			return
		}
		if err != nil {
			log.Error("reading a session failed", "request_id", requestID(ctx), "err", err)
			serverError(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, accountKey{}, login)))
	})
}

// accountFrom returns the account signInGate found for this request.
func accountFrom(ctx context.Context) (store.Account, error) {
	l, ok := ctx.Value(accountKey{}).(store.Account)
	if !ok {
		return store.Account{}, errors.New("a /v1/me route reached without the sign-in gate")
	}
	return l, nil
}

// routeSignIns sends the sign-in paths through their gate, and every other path to rest.
func routeSignIns(rest, signIns http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isSignInPath(r.URL.Path) {
			signIns.ServeHTTP(w, r)
			return
		}
		rest.ServeHTTP(w, r)
	})
}

// redirect sends the browser on to a URL, setting cookies on the way. It answers every route that redirects.
type redirect struct {
	to      string
	cookies []*http.Cookie
}

func (r redirect) visit(w http.ResponseWriter) error {
	for _, c := range r.cookies {
		http.SetCookie(w, c)
	}
	w.Header().Set("Cache-Control", cacheNone)
	w.Header().Set("Location", r.to)
	w.WriteHeader(http.StatusFound)
	return nil
}

func (r redirect) VisitStartDiscordSignInResponse(w http.ResponseWriter) error    { return r.visit(w) }
func (r redirect) VisitFinishDiscordSignInResponse(w http.ResponseWriter) error   { return r.visit(w) }
func (r redirect) VisitStartMicrosoftSignInResponse(w http.ResponseWriter) error  { return r.visit(w) }
func (r redirect) VisitFinishMicrosoftSignInResponse(w http.ResponseWriter) error { return r.visit(w) }
func (r redirect) VisitStartGoogleSignInResponse(w http.ResponseWriter) error     { return r.visit(w) }
func (r redirect) VisitFinishGoogleSignInResponse(w http.ResponseWriter) error    { return r.visit(w) }

// signedOut answers a sign-out or a deleted account, clearing the session cookie.
type signedOut struct{}

func (signedOut) VisitSignOutResponse(w http.ResponseWriter) error {
	http.SetCookie(w, sessionCookieWith("", -1))
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (o signedOut) VisitDeleteMeResponse(w http.ResponseWriter) error {
	return o.VisitSignOutResponse(w)
}

// sessionCookieWith is the session cookie holding token: host-only (no Domain), HttpOnly, Secure, SameSite=Lax.
func sessionCookieWith(token string, maxAge int) *http.Cookie {
	return &http.Cookie{Name: sessionCookie, Value: token, Path: "/", MaxAge: maxAge, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode}
}

// flowCookieWith is the cookie a sign-in keeps its state, verifier and return path in, on the provider's path, until
// the provider answers.
func flowCookieWith(value, path string, maxAge int) *http.Cookie {
	return &http.Cookie{Name: flowCookie, Value: value, Path: path, MaxAge: maxAge, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode}
}

// nameLinkedMessage tells a player the TTA username they asked for has an account already, and what to do.
func nameLinkedMessage(support string) string {
	msg := "Sorry, this TTA username is already linked to another account. If that is you, sign in with that account; " +
		"you can add this way of signing in to it there, then delete this one."
	if support != "" {
		msg += " If you think this is a mistake, write to " + support + "."
	}
	return msg
}

// accountPath is the account page, where a sign-in returns when it has nowhere else to go.
const accountPath = "/account/"

// returnPath is back when it is a path on the site, with or without a query, else the account page: never another
// site or a fragment.
func returnPath(back *string) string {
	if back == nil {
		return accountPath
	}
	p := *back
	if len(p) > 300 || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.ContainsAny(p, "\\#") {
		return accountPath
	}
	for _, c := range p {
		if c < 0x21 || c > 0x7e {
			return accountPath
		}
	}
	return p
}

// withSignIn adds how a sign-in ended to a path on the site, which may have a query already.
func withSignIn(path, outcome string) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + "signin=" + outcome
}

// accountWith is the account page told how a sign-in ended, and the page to go back to when there is one.
func accountWith(back, outcome string) string {
	q := url.Values{"signin": {outcome}}
	if back != accountPath {
		q.Set("back", back)
	}
	return accountPath + "?" + q.Encode()
}

// addMark ends a flow cookie that adds the provider to the signed-in account rather than signing in.
const addMark = "add"

// newSecret is a random string of 52 characters from A-Z and 2-7: 260 bits, fit for a token or a PKCE verifier.
func newSecret() string {
	return rand.Text() + rand.Text()
}

// startFlow sends the browser to a provider through authURL, keeping the sign-in's state, verifier, return path and
// whether it adds to an account in a flow cookie on path.
func startFlow(path string, back *string, add *bool, authURL func(state, verifier string) string) redirect {
	state, verifier := newSecret(), newSecret()
	flow := state + "." + verifier + "." + base64.RawURLEncoding.EncodeToString([]byte(returnPath(back)))
	if add != nil && *add {
		flow += "." + addMark
	}
	return redirect{to: authURL(state, verifier), cookies: []*http.Cookie{flowCookieWith(flow, path, int(flowLife.Seconds()))}}
}

// nonceFor is the OpenID nonce a sign-in sends with verifier: derived from it, so the flow cookie needs no more, and
// it reveals nothing of the verifier.
func nonceFor(verifier string) string {
	h := sha256.Sum256([]byte("nonce." + verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func (s *server) StartDiscordSignIn(_ context.Context, req StartDiscordSignInRequestObject) (StartDiscordSignInResponseObject, error) {
	return startFlow(discordPath, req.Params.Return, req.Params.Add, s.signIns.DiscordAuthURL), nil
}

func (s *server) StartMicrosoftSignIn(_ context.Context, req StartMicrosoftSignInRequestObject) (StartMicrosoftSignInResponseObject, error) {
	if !s.signIns.OpenIDReady("microsoft") {
		return StartMicrosoftSignIn503JSONResponse{notSetUp("Microsoft")}, nil
	}
	return s.startOpenID("microsoft", req.Params.Return, req.Params.Add), nil
}

func (s *server) StartGoogleSignIn(_ context.Context, req StartGoogleSignInRequestObject) (StartGoogleSignInResponseObject, error) {
	if !s.signIns.OpenIDReady("google") {
		return StartGoogleSignIn503JSONResponse{notSetUp("Google")}, nil
	}
	return s.startOpenID("google", req.Params.Return, req.Params.Add), nil
}

// startOpenID sends the browser to provider, Microsoft or Google, with the nonce its verifier gives.
func (s *server) startOpenID(provider string, back *string, add *bool) redirect {
	return startFlow("/v1/auth/"+provider, back, add, func(state, verifier string) string {
		return s.signIns.OpenIDAuthURL(provider, state, verifier, nonceFor(verifier))
	})
}

// notSetUp answers a route of provider, such as Microsoft, on an api without sign-in with it.
func notSetUp(provider string) NotSetUpJSONResponse {
	return NotSetUpJSONResponse{Body: Error{Error: "signing in with " + provider + " is not set up here"}, Headers: NotSetUpResponseHeaders{CacheControl: cacheNone}}
}

// readFlow takes a flow cookie apart: its state, verifier and return path, and whether it adds to an account.
func readFlow(ctx context.Context) (state, verifier, back string, add, ok bool) {
	raw, _ := ctx.Value(flowKey{}).(string)
	parts := strings.Split(raw, ".")
	if len(parts) == 4 && parts[3] == addMark {
		parts, add = parts[:3], true
	}
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
		return "", "", accountPath, false, false
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", "", accountPath, false, false
	}
	p := string(b)
	return parts[0], parts[1], returnPath(&p), add, true
}

// finishFlow finishes a sign-in with provider whose flow cookie is on path: it checks the state the provider sent
// back against the cookie, asks who for the identity behind the code, and signs them in, or adds the identity to the
// signed-in account. Anything wrong leaves the player signed out, back on the site.
func (s *server) finishFlow(ctx context.Context, provider, path string, code, state, declined *string,
	who func(code, verifier string) (store.Identity, error)) redirect {
	wantState, verifier, back, add, ok := readFlow(ctx)
	cookies := []*http.Cookie{flowCookieWith("", path, -1)}
	// A returning player goes back where they started. The account page welcomes a first sign-in, and says why
	// another did not finish; both keep the way back.
	outcome := func(o string) redirect {
		to := accountWith(back, o)
		if o == "ok" {
			to = withSignIn(back, o)
		}
		return redirect{to: s.signIns.SiteOrigin() + to, cookies: cookies}
	}
	if !ok || declined != nil || code == nil || state == nil || subtle.ConstantTimeCompare([]byte(*state), []byte(wantState)) != 1 {
		s.log.Info("sign-in refused: declined, or no matching state", "provider", provider)
		return outcome("failed")
	}
	id, err := who(*code, verifier)
	if err != nil {
		s.log.Info("sign-in failed", "provider", provider, "err", err)
		return outcome("failed")
	}
	if add {
		return s.addLogin(ctx, id, cookies)
	}
	token := newSecret()
	first, err := s.signIns.SignIn(ctx, id, hashToken(token), s.now())
	if err != nil {
		s.log.Error("recording a sign-in failed", "request_id", requestID(ctx), "err", err)
		return outcome("failed")
	}
	cookies = append(cookies, sessionCookieWith(token, int(cookieLife.Seconds())))
	if first {
		return outcome("first")
	}
	return outcome("ok")
}

func (s *server) FinishDiscordSignIn(ctx context.Context, req FinishDiscordSignInRequestObject) (FinishDiscordSignInResponseObject, error) {
	p := req.Params
	return s.finishFlow(ctx, "discord", discordPath, p.Code, p.State, p.Error, func(code, verifier string) (store.Identity, error) {
		u, err := s.signIns.DiscordUser(ctx, code, verifier)
		return store.Identity{Provider: "discord", Subject: u.ID, Username: u.Username, DisplayName: u.DisplayName}, err
	}), nil
}

func (s *server) FinishMicrosoftSignIn(ctx context.Context, req FinishMicrosoftSignInRequestObject) (FinishMicrosoftSignInResponseObject, error) {
	if !s.signIns.OpenIDReady("microsoft") {
		return FinishMicrosoftSignIn503JSONResponse{notSetUp("Microsoft")}, nil
	}
	p := req.Params
	return s.finishOpenID(ctx, "microsoft", "Microsoft", p.Code, p.State, p.Error), nil
}

func (s *server) FinishGoogleSignIn(ctx context.Context, req FinishGoogleSignInRequestObject) (FinishGoogleSignInResponseObject, error) {
	if !s.signIns.OpenIDReady("google") {
		return FinishGoogleSignIn503JSONResponse{notSetUp("Google")}, nil
	}
	p := req.Params
	return s.finishOpenID(ctx, "google", "Google", p.Code, p.State, p.Error), nil
}

// finishOpenID finishes a sign-in with provider, Microsoft or Google, whose login is called by the account's name, else
// "<Name> account"; never by its email, which the site's nav would show to anyone looking.
func (s *server) finishOpenID(ctx context.Context, provider, name string, code, state, declined *string) redirect {
	return s.finishFlow(ctx, provider, "/v1/auth/"+provider, code, state, declined, func(code, verifier string) (store.Identity, error) {
		u, err := s.signIns.OpenIDUser(ctx, provider, code, verifier, nonceFor(verifier))
		if u.Name == "" {
			u.Name = name + " account"
		}
		return store.Identity{Provider: provider, Subject: u.Subject, Username: u.Name, Email: u.Email}, err
	})
}

// addLogin adds id to the signed-in account and goes back to the account page saying how that went, and which way in.
func (s *server) addLogin(ctx context.Context, id store.Identity, cookies []*http.Cookie) redirect {
	hash, _ := ctx.Value(sessionKey{}).([]byte)
	err := s.signIns.AddLogin(ctx, hash, id, s.now())
	outcome := "added"
	switch {
	case errors.Is(err, store.ErrLoginTaken):
		outcome = "taken"
	case errors.Is(err, store.ErrHasLoginOfKind):
		outcome = "has-kind"
	case errors.Is(err, store.ErrNoSession):
		outcome = "failed"
	case err != nil:
		s.log.Error("adding a way in failed", "provider", id.Provider, "request_id", requestID(ctx), "err", err)
		outcome = "failed"
	}
	q := url.Values{"signin": {outcome}, "way": {id.Provider}}
	return redirect{to: s.signIns.SiteOrigin() + accountPath + "?" + q.Encode(), cookies: cookies}
}

func (s *server) SignOut(ctx context.Context, _ SignOutRequestObject) (SignOutResponseObject, error) {
	if hash, ok := ctx.Value(sessionKey{}).([]byte); ok {
		if err := s.signIns.SignOut(ctx, hash); err != nil {
			return nil, err
		}
	}
	return signedOut{}, nil
}
