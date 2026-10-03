// Package access verifies the Cloudflare Access token that admin requests carry,
// and reads the Discord id of the person it was issued to.
package access

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"

	"crypto/rsa"

	"github.com/golang-jwt/jwt/v5"
)

// Config is the Access team and application a token must come from.
type Config struct {
	// Team is the team domain as an https URL, the tokens' issuer: https://<team>.cloudflareaccess.com
	Team string
	// AUD is the admin application's audience tag.
	AUD string
}

// Identity is who a verified token was issued to.
type Identity struct {
	DiscordID string
}

// Verifier checks Access tokens against the team's signing keys, fetched and cached.
type Verifier struct {
	cfg    Config
	client *http.Client
	now    func() time.Time

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

// refetchAfter limits how often an unknown key id makes the verifier fetch the keys again.
const refetchAfter = 5 * time.Minute

var discordID = regexp.MustCompile(`^[0-9]{5,25}$`)

// NewVerifier returns a verifier for cfg. It refuses an empty AUD or a team that is not an https URL,
// so admin routes never start open.
func NewVerifier(cfg Config, client *http.Client, now func() time.Time) (*Verifier, error) {
	if cfg.AUD == "" {
		return nil, errors.New("access: the admin application's AUD is not set")
	}
	u, err := url.Parse(cfg.Team)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Path != "" {
		return nil, errors.New("access: the team must be an https URL with no path")
	}
	return &Verifier{cfg: cfg, client: client, now: now, keys: map[string]*rsa.PublicKey{}}, nil
}

type tokenClaims struct {
	jwt.RegisteredClaims
	Type   string `json:"type"`
	Custom struct {
		ID string `json:"id"`
	} `json:"custom"`
}

// Verify checks a token's RS256 signature, issuer, audience, expiry and type, and returns
// the Discord id in its custom claims. Any failure is an error; the token is never trusted in part.
func (v *Verifier) Verify(ctx context.Context, token string) (Identity, error) {
	if token == "" {
		return Identity{}, errors.New("access: no token")
	}
	var c tokenClaims
	_, err := jwt.ParseWithClaims(token, &c, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return v.key(ctx, kid)
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(v.cfg.Team),
		jwt.WithAudience(v.cfg.AUD),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(v.now),
	)
	if err != nil {
		return Identity{}, fmt.Errorf("access: %w", err)
	}
	if c.Type != "app" {
		return Identity{}, fmt.Errorf("access: token type %q, want app", c.Type)
	}
	if !discordID.MatchString(c.Custom.ID) {
		return Identity{}, errors.New("access: the token carries no Discord id")
	}
	return Identity{DiscordID: c.Custom.ID}, nil
}

// key returns the signing key with this id, fetching the keys when it is unknown and they were not fetched lately.
func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	if !v.fetched.IsZero() && v.now().Sub(v.fetched) < refetchAfter {
		return nil, errors.New("unknown signing key")
	}
	keys, err := v.fetch(ctx)
	v.fetched = v.now()
	if err != nil {
		return nil, err
	}
	v.keys = keys
	if k, ok := keys[kid]; ok {
		return k, nil
	}
	return nil, errors.New("unknown signing key")
}

// fetch reads the team's RSA signing keys from its certs endpoint.
func (v *Verifier) fetch(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.cfg.Team+"/cdn-cgi/access/certs", nil)
	if err != nil {
		return nil, fmt.Errorf("fetching signing keys: %w", err)
	}
	resp, err := v.client.Do(req)
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
