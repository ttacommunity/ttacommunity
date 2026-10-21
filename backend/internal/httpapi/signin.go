// Player sign-in: signing in with Discord (or adding it as another way in) and out, and the signed-in player reading
// themselves, asking to be linked to a member and setting their own country and zone. /v1/me, sign-out and the
// email routes are fetched by the site alone, with credentials; signing in with Discord is a page the browser opens.
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
	"ttacommunity/backend/internal/store"
)

// SignIns signs players in with Discord and keeps their accounts, sessions, members and requests.
type SignIns interface {
	// SignIn reports whether this was the account's first sign-in.
	SignIn(ctx context.Context, id store.Identity, tokenHash []byte, now time.Time) (bool, error)
	Session(ctx context.Context, tokenHash []byte, now time.Time) (store.Account, error)
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
	// DiscordAuthURL and DiscordUser start and finish a sign-in with Discord; AddDiscord adds it to an account.
	DiscordAuthURL(state, verifier string) string
	DiscordUser(ctx context.Context, code, verifier string) (discord.User, error)
	AddDiscord(ctx context.Context, tokenHash []byte, id store.Identity, now time.Time) error
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

// The cookies: the session's opaque token, and a sign-in's state, verifier and return path until Discord answers.
const (
	sessionCookie = "ttac_session"
	flowCookie    = "ttac_signin"
	flowPath      = "/v1/auth/discord"
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

func (r redirect) VisitStartDiscordSignInResponse(w http.ResponseWriter) error  { return r.visit(w) }
func (r redirect) VisitFinishDiscordSignInResponse(w http.ResponseWriter) error { return r.visit(w) }

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

// flowCookieWith is the cookie a sign-in keeps its state, verifier and return path in until Discord answers.
func flowCookieWith(value string, maxAge int) *http.Cookie {
	return &http.Cookie{Name: flowCookie, Value: value, Path: flowPath, MaxAge: maxAge, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode}
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

// addMark ends a flow cookie that adds Discord to the signed-in account rather than signing in.
const addMark = "add"

// newSecret is a random string of 52 characters from A-Z and 2-7: 260 bits, fit for a token or a PKCE verifier.
func newSecret() string {
	return rand.Text() + rand.Text()
}

func (s *server) StartDiscordSignIn(_ context.Context, req StartDiscordSignInRequestObject) (StartDiscordSignInResponseObject, error) {
	state, verifier := newSecret(), newSecret()
	flow := state + "." + verifier + "." + base64.RawURLEncoding.EncodeToString([]byte(returnPath(req.Params.Return)))
	if req.Params.Add != nil && *req.Params.Add {
		flow += "." + addMark
	}
	return redirect{to: s.signIns.DiscordAuthURL(state, verifier), cookies: []*http.Cookie{flowCookieWith(flow, int(flowLife.Seconds()))}}, nil
}

// readFlow takes a flow cookie apart: its state, verifier and return path, and whether it adds Discord.
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

func (s *server) FinishDiscordSignIn(ctx context.Context, req FinishDiscordSignInRequestObject) (FinishDiscordSignInResponseObject, error) {
	state, verifier, back, add, ok := readFlow(ctx)
	cookies := []*http.Cookie{flowCookieWith("", -1)}
	// A returning player goes back where they started. The account page welcomes a first sign-in, and says why
	// another did not finish; both keep the way back.
	outcome := func(o string) redirect {
		to := accountWith(back, o)
		if o == "ok" {
			to = withSignIn(back, o)
		}
		return redirect{to: s.signIns.SiteOrigin() + to, cookies: cookies}
	}
	p := req.Params
	if !ok || p.Error != nil || p.Code == nil || p.State == nil || subtle.ConstantTimeCompare([]byte(*p.State), []byte(state)) != 1 {
		s.log.Info("discord sign-in refused: declined, or no matching state")
		return outcome("failed"), nil
	}
	u, err := s.signIns.DiscordUser(ctx, *p.Code, verifier)
	if err != nil {
		s.log.Info("discord sign-in failed", "err", err)
		return outcome("failed"), nil
	}
	id := store.Identity{Provider: "discord", Subject: u.ID, Username: u.Username, DisplayName: u.DisplayName}
	if add {
		return s.addDiscord(ctx, id, cookies), nil
	}
	token := newSecret()
	first, err := s.signIns.SignIn(ctx, id, hashToken(token), s.now())
	if err != nil {
		s.log.Error("recording a sign-in failed", "request_id", requestID(ctx), "err", err)
		return outcome("failed"), nil
	}
	cookies = append(cookies, sessionCookieWith(token, int(cookieLife.Seconds())))
	if first {
		return outcome("first"), nil
	}
	return outcome("ok"), nil
}

// addDiscord adds id to the signed-in account and goes back to the account page saying how that went.
func (s *server) addDiscord(ctx context.Context, id store.Identity, cookies []*http.Cookie) redirect {
	hash, _ := ctx.Value(sessionKey{}).([]byte)
	err := s.signIns.AddDiscord(ctx, hash, id, s.now())
	outcome := "added"
	switch {
	case errors.Is(err, store.ErrLoginTaken):
		outcome = "taken"
	case errors.Is(err, store.ErrHasLoginOfKind):
		outcome = "has-kind"
	case errors.Is(err, store.ErrNoSession):
		outcome = "failed"
	case err != nil:
		s.log.Error("adding Discord to an account failed", "request_id", requestID(ctx), "err", err)
		outcome = "failed"
	}
	return redirect{to: s.signIns.SiteOrigin() + withSignIn(accountPath, outcome), cookies: cookies}
}

func (s *server) SignOut(ctx context.Context, _ SignOutRequestObject) (SignOutResponseObject, error) {
	if hash, ok := ctx.Value(sessionKey{}).([]byte); ok {
		if err := s.signIns.SignOut(ctx, hash); err != nil {
			return nil, err
		}
	}
	return signedOut{}, nil
}

func (s *server) GetMe(ctx context.Context, _ GetMeRequestObject) (GetMeResponseObject, error) {
	l, err := accountFrom(ctx)
	if err != nil {
		return nil, err
	}
	me := Me{Provider: LoginProvider(l.Provider), Username: l.Username, DisplayName: l.DisplayName, Country: nullableText(l.Country), Zone: l.Zone,
		Logins: make([]MyLogin, len(l.Logins))}
	for i, w := range l.Logins {
		me.Logins[i] = MyLogin{Provider: LoginProvider(w.Provider), Username: w.Username, DisplayName: w.DisplayName}
	}
	if l.Member != nil {
		me.Member = &MyMember{Name: l.Member.Name, Player: l.Member.Player, Banned: l.Banned, BanReason: l.BanReason}
	}
	if l.Request != nil {
		me.Request = &MyLinkRequest{Name: l.Request.Name, Status: MyLinkRequestStatus(l.Request.Status), Reason: l.Request.Reason,
			RequestedAt: l.Request.RequestedAt}
	}
	if l.AdminRole != "" {
		role := MeAdminRole(l.AdminRole)
		me.AdminRole = &role
	}
	return GetMe200JSONResponse{Body: me, Headers: GetMe200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) DeleteMe(ctx context.Context, _ DeleteMeRequestObject) (DeleteMeResponseObject, error) {
	hash, _ := ctx.Value(sessionKey{}).([]byte)
	err := s.signIns.DeleteAccount(ctx, hash, s.now())
	if errors.Is(err, store.ErrNoSession) {
		return DeleteMe401JSONResponse{SignedOutJSONResponse{Body: Error{Error: "not signed in"}, Headers: SignedOutResponseHeaders{CacheControl: cacheNone}}}, nil
	}
	if err != nil {
		return nil, err
	}
	return signedOut{}, nil
}

func (s *server) PutMyLinkRequest(ctx context.Context, req PutMyLinkRequestRequestObject) (PutMyLinkRequestResponseObject, error) {
	l, err := accountFrom(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return PutMyLinkRequest400JSONResponse{badRequest400("the TTA username to be linked to: name")}, nil
	}
	err = s.signIns.RequestLink(ctx, l.ID, req.Body.Name, s.now())
	switch {
	case errors.Is(err, store.ErrBadLinkName):
		return PutMyLinkRequest400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrAlreadyLinked):
		return PutMyLinkRequest409JSONResponse{conflict409("you are linked to a member already")}, nil
	case errors.Is(err, store.ErrNameLinked):
		return PutMyLinkRequest409JSONResponse{conflict409(nameLinkedMessage(s.signIns.SupportAddress()))}, nil
	case err != nil:
		return nil, err
	}
	return PutMyLinkRequest204Response{}, nil
}

func (s *server) PatchMyDetails(ctx context.Context, req PatchMyDetailsRequestObject) (PatchMyDetailsResponseObject, error) {
	l, err := accountFrom(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return PatchMyDetails400JSONResponse{badRequest400("a country, a zone, or both")}, nil
	}
	if z := req.Body.Zone; z != nil {
		if msg := zoneProblem(strings.TrimSpace(*z)); msg != "" {
			return PatchMyDetails400JSONResponse{badRequest400(msg)}, nil
		}
	}
	err = s.signIns.SetOwnDetails(ctx, l.ID, req.Body.Country, req.Body.Zone)
	switch {
	case errors.Is(err, store.ErrBadMember):
		return PatchMyDetails400JSONResponse{badRequest400("a country as its two-letter ISO code (NZ), and a time zone")}, nil
	case err != nil:
		return nil, err
	}
	return PatchMyDetails204Response{}, nil
}

func (s *server) GetCountries(context.Context, GetCountriesRequestObject) (GetCountriesResponseObject, error) {
	return GetCountries200JSONResponse{Body: store.CountryCodes(), Headers: GetCountries200ResponseHeaders{CacheControl: cacheDay}}, nil
}
