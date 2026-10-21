// Package microsoft signs a player in with any Microsoft account, personal, work or school: the authorization code
// flow with state, PKCE and a nonce, asking for openid, profile and email, then checking the ID token Microsoft sends.
package microsoft

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
)

// microsoftAuthority is where Microsoft signs people in, used when Config leaves it empty.
const microsoftAuthority = "https://login.microsoftonline.com"

// refetchAfter limits how often an unknown key id makes the client fetch Microsoft's keys again.
const refetchAfter = 5 * time.Minute

// Config is the Microsoft app players sign in through, and where Microsoft sends them back.
type Config struct {
	ClientID, ClientSecret string
	// RedirectURL is the api's callback, as registered with the app: https, or http on 127.0.0.1.
	RedirectURL string
	// Authority is Microsoft's when empty; a stub on 127.0.0.1 may stand in for tests.
	Authority string
}

// String keeps the client secret out of logs.
func (c Config) String() string {
	return fmt.Sprintf("microsoft.Config{ClientID: %s, RedirectURL: %s}", c.ClientID, c.RedirectURL)
}

// User is who signed in: their id for this app, their name, and their email when Microsoft says its domain is
// verified (empty otherwise).
type User struct {
	Subject, Name, Email string
}

// Client starts sign-ins and finishes them.
type Client struct {
	oauth     oauth2.Config
	authority string
	http      *http.Client
	now       func() time.Time

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

var tenantID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// New returns a client for cfg, calling Microsoft through hc and checking token times against now. It refuses a
// config without a client id, secret or redirect URL, a plain-http redirect URL off this machine, or an authority
// that is neither Microsoft's nor this machine.
func New(cfg Config, hc *http.Client, now func() time.Time) (*Client, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("microsoft: the client id and secret are both needed")
	}
	if !strings.HasPrefix(cfg.RedirectURL, "https://") && !strings.HasPrefix(cfg.RedirectURL, "http://127.0.0.1:") {
		return nil, errors.New("microsoft: the redirect URL must be https, or http on 127.0.0.1")
	}
	if cfg.Authority == "" {
		cfg.Authority = microsoftAuthority
	}
	if cfg.Authority != microsoftAuthority && !strings.HasPrefix(cfg.Authority, "http://127.0.0.1:") {
		return nil, errors.New("microsoft: the authority must be Microsoft's, or a stub on 127.0.0.1")
	}
	return &Client{
		oauth: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       []string{"openid", "profile", "email"},
			Endpoint: oauth2.Endpoint{
				AuthURL:   cfg.Authority + "/common/oauth2/v2.0/authorize",
				TokenURL:  cfg.Authority + "/common/oauth2/v2.0/token",
				AuthStyle: oauth2.AuthStyleInParams,
			},
		},
		authority: cfg.Authority,
		http:      hc,
		now:       now,
		keys:      map[string]*rsa.PublicKey{},
	}, nil
}

// AuthCodeURL is where to send a player to sign in, carrying state, the S256 challenge of verifier, and nonce.
func (c *Client) AuthCodeURL(state, verifier, nonce string) string {
	return c.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("nonce", nonce), oauth2.SetAuthURLParam("response_mode", "query"))
}

type idClaims struct {
	jwt.RegisteredClaims
	Tenant   string `json:"tid"`
	Nonce    string `json:"nonce"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Verified any    `json:"xms_edov"`
}

// User exchanges the code Microsoft sent back, with the verifier its challenge came from, and checks the ID token:
// Microsoft's RS256 signature, this app's audience, expiry, nonce, and an issuer naming the token's own tenant. Any
// failure is an error; the token is never trusted in part.
func (c *Client) User(ctx context.Context, code, verifier, nonce string) (User, error) {
	tok, err := c.oauth.Exchange(context.WithValue(ctx, oauth2.HTTPClient, c.http), code, oauth2.VerifierOption(verifier))
	if err != nil {
		return User{}, fmt.Errorf("microsoft: exchanging the code: %w", err)
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return User{}, errors.New("microsoft: no ID token")
	}
	var cl idClaims
	_, err = jwt.ParseWithClaims(raw, &cl, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return c.key(ctx, kid)
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithAudience(c.oauth.ClientID),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(c.now),
	)
	if err != nil {
		return User{}, fmt.Errorf("microsoft: %w", err)
	}
	if !tenantID.MatchString(cl.Tenant) || cl.Issuer != c.authority+"/"+cl.Tenant+"/v2.0" {
		return User{}, errors.New("microsoft: the token's issuer is not its tenant's")
	}
	if cl.Nonce == "" || cl.Nonce != nonce {
		return User{}, errors.New("microsoft: the token's nonce is not this sign-in's")
	}
	if cl.Subject == "" || len(cl.Subject) > 255 {
		return User{}, errors.New("microsoft: the token has no subject")
	}
	u := User{Subject: cl.Subject, Name: truncate(strings.TrimSpace(cl.Name), 100)}
	if cl.Verified == true {
		u.Email = cl.Email
	}
	return u, nil
}

// key returns Microsoft's signing key with this id, fetching the keys when it is unknown and they were not fetched
// lately.
func (c *Client) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if k, ok := c.keys[kid]; ok {
		return k, nil
	}
	if !c.fetched.IsZero() && c.now().Sub(c.fetched) < refetchAfter {
		return nil, errors.New("unknown signing key")
	}
	keys, err := c.fetch(ctx)
	c.fetched = c.now()
	if err != nil {
		return nil, err
	}
	c.keys = keys
	if k, ok := keys[kid]; ok {
		return k, nil
	}
	return nil, errors.New("unknown signing key")
}

// fetch reads Microsoft's RSA signing keys.
func (c *Client) fetch(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.authority+"/common/discovery/v2.0/keys", nil)
	if err != nil {
		return nil, fmt.Errorf("fetching signing keys: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching signing keys: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching signing keys: status %d", resp.StatusCode)
	}
	var body struct {
		Keys []struct {
			Kid, Kty, N, E string
		} `json:"keys"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("reading signing keys: %w", err)
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range body.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		n, errN := base64.RawURLEncoding.DecodeString(k.N)
		e, errE := base64.RawURLEncoding.DecodeString(k.E)
		if errN != nil || errE != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	if len(keys) == 0 {
		return nil, errors.New("fetching signing keys: none usable")
	}
	return keys, nil
}

// truncate cuts s to at most n characters.
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
