// Package access verifies the Cloudflare Access token that admin requests carry,
// and reads the Discord id of the person it was issued to.
package access

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"ttacommunity/backend/internal/jwks"
)

// Config is the Access team and application a token must come from.
type Config struct {
	// Team is the team domain as an https URL, the tokens' issuer: https://<team>.cloudflareaccess.com
	Team string
	// AUD is the admin application's audience tag.
	AUD string
	// DevUser, when set, accepts local requests without a Cloudflare token, returning this Discord ID.
	DevUser string
	// Origin is the allowed admin site origin, used to guard dev mode.
	Origin string
	// ListenAddr is the api's listening address, used to guard dev mode.
	ListenAddr string
	// InKubernetes says the api runs in a cluster, where dev mode is refused.
	InKubernetes bool
}

// Identity is who a verified token was issued to.
type Identity struct {
	DiscordID string
}

// Verifier checks Access tokens against the team's signing keys, fetched and cached.
type Verifier struct {
	cfg  Config
	keys *jwks.Keys
	now  func() time.Time
}

var discordID = regexp.MustCompile(`^[0-9]{5,25}$`)

// NewVerifier returns a verifier for cfg. It refuses an empty AUD or a team that is not an https URL,
// so admin routes never start open.
func NewVerifier(cfg Config, client *http.Client, now func() time.Time) (*Verifier, error) {
	if cfg.DevUser != "" {
		if err := devModeAllowed(cfg); err != nil {
			return nil, err
		}
		return &Verifier{cfg: cfg, now: now}, nil
	}
	if cfg.AUD == "" {
		return nil, errors.New("access: the admin application's AUD is not set")
	}
	u, err := url.Parse(cfg.Team)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Path != "" {
		return nil, errors.New("access: the team must be an https URL with no path")
	}
	return &Verifier{cfg: cfg, keys: jwks.New(cfg.Team+"/cdn-cgi/access/certs", client, now), now: now}, nil
}

// devModeAllowed refuses dev mode unless every independent sign of a local machine holds:
// a 127.0.0.1 admin origin, an api listening on 127.0.0.1 only, and no Kubernetes.
func devModeAllowed(cfg Config) error {
	if !discordID.MatchString(cfg.DevUser) {
		return errors.New("access: DevUser must be a valid Discord id")
	}
	if !strings.HasPrefix(cfg.Origin, "http://127.0.0.1:") {
		return errors.New("access: DevUser is only permitted with a http://127.0.0.1 admin origin")
	}
	if host, _, err := net.SplitHostPort(cfg.ListenAddr); err != nil || host != "127.0.0.1" {
		return errors.New("access: DevUser is only permitted when the api listens on 127.0.0.1 (API_ADDR)")
	}
	if cfg.InKubernetes {
		return errors.New("access: DevUser is never permitted in Kubernetes")
	}
	return nil
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
	if v.cfg.DevUser != "" {
		if token == "" || token == "dev" {
			return Identity{DiscordID: v.cfg.DevUser}, nil
		}
		if strings.HasPrefix(token, "dev-") && discordID.MatchString(token[4:]) {
			return Identity{DiscordID: token[4:]}, nil
		}
		return Identity{}, errors.New("access: invalid dev token")
	}
	if token == "" {
		return Identity{}, errors.New("access: no token")
	}
	var c tokenClaims
	_, err := jwt.ParseWithClaims(token, &c, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return v.keys.Key(ctx, kid)
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
