// Package openid signs a player in with Microsoft or Google: the authorization code flow with state, PKCE and a
// nonce, then checking the ID token the provider sends. Each provider is a Provider; the flow and checks are shared.
package openid

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"

	"ttacommunity/backend/internal/jwks"
)

// Provider is what differs between providers: endpoints, scopes, extra authorize parameters, and the rules for a
// token's issuer and for when its email is verified.
type Provider struct {
	name                    string
	authURL, tokenURL, keys string
	scopes                  []string
	authParams              map[string]string
	// issuer reports whether iss is the provider's, for a token with these claims; origin is its endpoints' origin.
	issuer func(cl *idClaims, origin string) bool
	// verified reports whether the provider vouches for the token's email.
	verified func(cl *idClaims) bool
}

// microsoftTenant is a Microsoft directory id; consumers is the one for personal accounts, which prove their email to
// Microsoft with a code.
var microsoftTenant = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

const consumers = "9188040d-6c67-4c5b-b112-36a304b66dad"

// Microsoft signs in any Microsoft account, personal, work or school. Its tokens name their own tenant as issuer, and
// its email is vouched for when the account is personal or its domain was verified by its directory.
var Microsoft = Provider{
	name:       "microsoft",
	authURL:    "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
	tokenURL:   "https://login.microsoftonline.com/common/oauth2/v2.0/token",
	keys:       "https://login.microsoftonline.com/common/discovery/v2.0/keys",
	scopes:     []string{"openid", "profile", "email"},
	authParams: map[string]string{"response_mode": "query"},
	issuer: func(cl *idClaims, origin string) bool {
		return microsoftTenant.MatchString(cl.Tenant) && cl.Issuer == origin+"/"+cl.Tenant+"/v2.0"
	},
	verified: func(cl *idClaims) bool { return cl.Tenant == consumers || cl.MicrosoftVerified == true },
}

// Google signs in a Google account, with Google's endpoints from its OpenID discovery document. Its tokens name it
// as issuer in one of two spellings.
var Google = Provider{
	name:     "google",
	authURL:  "https://accounts.google.com/o/oauth2/v2/auth",
	tokenURL: "https://oauth2.googleapis.com/token",
	keys:     "https://www.googleapis.com/oauth2/v3/certs",
	scopes:   []string{"openid", "email", "profile"},
	issuer: func(cl *idClaims, _ string) bool {
		return slices.Contains([]string{"https://accounts.google.com", "accounts.google.com"}, cl.Issuer)
	},
	verified: func(cl *idClaims) bool { return cl.GoogleVerified == true },
}

// Config is the app players sign in through, and where the provider sends them back.
type Config struct {
	ClientID, ClientSecret string
	// RedirectURL is the api's callback, as registered with the app: https, or http on 127.0.0.1.
	RedirectURL string
	// Stub, when set, stands in for every one of the provider's endpoints, on 127.0.0.1, for tests.
	Stub string
}

// String keeps the client secret out of logs.
func (c Config) String() string {
	return fmt.Sprintf("openid.Config{ClientID: %s, RedirectURL: %s}", c.ClientID, c.RedirectURL)
}

// User is who signed in: their id for this app, their name, and their email when the provider vouches for it
// (empty otherwise).
type User struct {
	Subject, Name, Email string
}

// Client starts sign-ins with one provider and finishes them.
type Client struct {
	p      Provider
	oauth  oauth2.Config
	origin string
	keys   *jwks.Keys
	http   *http.Client
	now    func() time.Time
}

// New returns a client for p and cfg, calling the provider through hc and checking token times against now. It
// refuses a config without a client id, secret or redirect URL, a plain-http redirect URL off this machine, or a stub
// off this machine.
func New(p Provider, cfg Config, hc *http.Client, now func() time.Time) (*Client, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("%s: the client id and secret are both needed", p.name)
	}
	if !strings.HasPrefix(cfg.RedirectURL, "https://") && !strings.HasPrefix(cfg.RedirectURL, "http://127.0.0.1:") {
		return nil, fmt.Errorf("%s: the redirect URL must be https, or http on 127.0.0.1", p.name)
	}
	if cfg.Stub != "" && !strings.HasPrefix(cfg.Stub, "http://127.0.0.1:") {
		return nil, fmt.Errorf("%s: a stand-in for the provider must be on 127.0.0.1", p.name)
	}
	auth, token, keys := at(cfg.Stub, p.authURL), at(cfg.Stub, p.tokenURL), at(cfg.Stub, p.keys)
	return &Client{
		p: p,
		oauth: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       p.scopes,
			Endpoint:     oauth2.Endpoint{AuthURL: auth, TokenURL: token, AuthStyle: oauth2.AuthStyleInParams},
		},
		origin: strings.TrimSuffix(auth, pathOf(p.authURL)),
		keys:   jwks.New(keys, hc, now),
		http:   hc,
		now:    now,
	}, nil
}

// at is endpoint with its origin replaced by stub, or endpoint itself when stub is empty.
func at(stub, endpoint string) string {
	if stub == "" {
		return endpoint
	}
	return stub + pathOf(endpoint)
}

// pathOf is the path of a well-formed endpoint URL.
func pathOf(endpoint string) string {
	u, _ := url.Parse(endpoint)
	return u.Path
}

// AuthCodeURL is where to send a player to sign in, carrying state, the S256 challenge of verifier, and nonce.
func (c *Client) AuthCodeURL(state, verifier, nonce string) string {
	opts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("nonce", nonce)}
	for k, v := range c.p.authParams {
		opts = append(opts, oauth2.SetAuthURLParam(k, v))
	}
	return c.oauth.AuthCodeURL(state, opts...)
}

type idClaims struct {
	jwt.RegisteredClaims
	Tenant            string `json:"tid"`
	Nonce             string `json:"nonce"`
	Name              string `json:"name"`
	Email             string `json:"email"`
	MicrosoftVerified any    `json:"xms_edov"`
	GoogleVerified    any    `json:"email_verified"`
}

// User exchanges the code the provider sent back, with the verifier its challenge came from, and checks the ID token:
// the provider's RS256 signature, this app's audience, expiry, nonce, and the provider's issuer. Any failure is an
// error; the token is never trusted in part.
func (c *Client) User(ctx context.Context, code, verifier, nonce string) (User, error) {
	name := c.p.name
	tok, err := c.oauth.Exchange(context.WithValue(ctx, oauth2.HTTPClient, c.http), code, oauth2.VerifierOption(verifier))
	if err != nil {
		return User{}, fmt.Errorf("%s: exchanging the code: %w", name, err)
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return User{}, fmt.Errorf("%s: no ID token", name)
	}
	var cl idClaims
	_, err = jwt.ParseWithClaims(raw, &cl, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return c.keys.Key(ctx, kid)
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithAudience(c.oauth.ClientID),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(c.now),
	)
	if err != nil {
		return User{}, fmt.Errorf("%s: %w", name, err)
	}
	if !c.p.issuer(&cl, c.origin) {
		return User{}, fmt.Errorf("%s: the token's issuer is not the provider's", name)
	}
	if cl.Nonce == "" || cl.Nonce != nonce {
		return User{}, fmt.Errorf("%s: the token's nonce is not this sign-in's", name)
	}
	if cl.Subject == "" || len(cl.Subject) > 255 {
		return User{}, errors.New(name + ": the token has no subject")
	}
	u := User{Subject: cl.Subject, Name: truncate(strings.TrimSpace(cl.Name), 100)}
	if c.p.verified(&cl) {
		u.Email = cl.Email
	}
	return u, nil
}

// truncate cuts s to at most n characters.
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
