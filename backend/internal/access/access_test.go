package access

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testAUD = "aud-of-the-admin-app"
	testKID = "key-1"
)

var now = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

type keys struct {
	priv    *rsa.PrivateKey
	kid     string
	fetches atomic.Int32
}

// serveKeys runs a stand-in for the team's certs endpoint over TLS, serving k's public key.
func serveKeys(t *testing.T, k *keys) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k.fetches.Add(1)
		pub := k.priv.PublicKey
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": k.kid, "kty": "RSA", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newKeys(t *testing.T) *keys {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return &keys{priv: priv, kid: testKID}
}

func sign(t *testing.T, k *keys, method jwt.SigningMethod, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(method, claims)
	tok.Header["kid"] = kid
	var key any = k.priv
	if method == jwt.SigningMethodHS256 {
		key = []byte("a shared secret")
	}
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func claims(issuer string) jwt.MapClaims {
	return jwt.MapClaims{
		"aud":    []string{testAUD},
		"iss":    issuer,
		"exp":    now.Add(time.Hour).Unix(),
		"iat":    now.Add(-time.Minute).Unix(),
		"nbf":    now.Add(-time.Minute).Unix(),
		"type":   "app",
		"sub":    "access-user-uuid",
		"custom": map[string]any{"id": "100000000000000001", "name": "Owner"},
	}
}

func verifier(t *testing.T, srv *httptest.Server) *Verifier {
	t.Helper()
	v, err := NewVerifier(Config{Team: srv.URL, AUD: testAUD}, srv.Client(), func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	return v
}

func TestVerifyAcceptsAValidTokenAndReadsTheDiscordID(t *testing.T) {
	k := newKeys(t)
	srv := serveKeys(t, k)
	team := srv.URL

	got, err := verifier(t, srv).Verify(context.Background(), sign(t, k, jwt.SigningMethodRS256, testKID, claims(team)))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.DiscordID != "100000000000000001" {
		t.Errorf("DiscordID = %q, want 100000000000000001", got.DiscordID)
	}
}

func TestVerifyRefusesABadToken(t *testing.T) {
	k := newKeys(t)
	srv := serveKeys(t, k)
	team := srv.URL
	other := newKeys(t)
	tests := []struct {
		name  string
		token func() string
	}{
		{"empty", func() string { return "" }},
		{"not a token", func() string { return "abc.def.ghi" }},
		{"another audience", func() string {
			c := claims(team)
			c["aud"] = []string{"another-app"}
			return sign(t, k, jwt.SigningMethodRS256, testKID, c)
		}},
		{"another issuer", func() string { return sign(t, k, jwt.SigningMethodRS256, testKID, claims("https://evil.example")) }},
		{"expired", func() string {
			c := claims(team)
			c["exp"] = now.Add(-time.Second).Unix()
			return sign(t, k, jwt.SigningMethodRS256, testKID, c)
		}},
		{"not yet valid", func() string {
			c := claims(team)
			c["nbf"] = now.Add(time.Hour).Unix()
			return sign(t, k, jwt.SigningMethodRS256, testKID, c)
		}},
		{"signed by another key", func() string { return sign(t, other, jwt.SigningMethodRS256, testKID, claims(team)) }},
		{"HS256", func() string { return sign(t, k, jwt.SigningMethodHS256, testKID, claims(team)) }},
		{"unknown key id", func() string { return sign(t, k, jwt.SigningMethodRS256, "key-2", claims(team)) }},
		{"no Discord id", func() string {
			c := claims(team)
			c["custom"] = map[string]any{"name": "Owner"}
			return sign(t, k, jwt.SigningMethodRS256, testKID, c)
		}},
		{"a service token", func() string {
			c := claims(team)
			c["type"] = "org"
			return sign(t, k, jwt.SigningMethodRS256, testKID, c)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := verifier(t, srv).Verify(context.Background(), tt.token()); err == nil {
				t.Errorf("Verify(%s) = %+v, nil; want an error", tt.name, got)
			}
		})
	}
}

func TestVerifyFetchesTheKeysAgainForAnUnknownKeyIDAtMostEveryFiveMinutes(t *testing.T) {
	k := newKeys(t)
	srv := serveKeys(t, k)
	clock := now
	v, err := NewVerifier(Config{Team: srv.URL, AUD: testAUD}, srv.Client(), func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(context.Background(), sign(t, k, jwt.SigningMethodRS256, testKID, claims(srv.URL))); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	k.kid = "rotated"
	rotated := sign(t, k, jwt.SigningMethodRS256, "rotated", claims(srv.URL))
	if _, err := v.Verify(context.Background(), rotated); err == nil {
		t.Error("Verify with a new key a moment later: nil error, want a refusal until five minutes pass")
	}
	clock = now.Add(6 * time.Minute)
	if _, err := v.Verify(context.Background(), rotated); err != nil {
		t.Fatalf("Verify after five minutes: %v", err)
	}
	if got := k.fetches.Load(); got != 2 {
		t.Errorf("certs fetched %d times, want 2", got)
	}
}

func TestNewVerifierRefusesAMissingSetting(t *testing.T) {
	for _, c := range []Config{
		{Team: "https://team.cloudflareaccess.com", AUD: ""},
		{Team: "", AUD: testAUD},
		{Team: "http://team.cloudflareaccess.com", AUD: testAUD},
	} {
		if _, err := NewVerifier(c, http.DefaultClient, time.Now); err == nil {
			t.Errorf("NewVerifier(%+v): nil error, want a refusal", c)
		}
	}
}

func TestNewVerifierAllowsDevUserWithLocalOrigin(t *testing.T) {
	cfg := Config{
		DevUser:    "100000000000000001",
		Origin:     "http://127.0.0.1:4322",
		ListenAddr: "127.0.0.1:8080",
	}
	v, err := NewVerifier(cfg, http.DefaultClient, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	for _, tt := range []struct {
		name    string
		token   string
		wantID  string
		wantErr bool
	}{
		{"empty token", "", "100000000000000001", false},
		{"dev token", "dev", "100000000000000001", false},
		{"specific dev user", "dev-222222222222222222", "222222222222222222", false},
		{"invalid dev token", "other-token", "", true},
		{"malformed dev id", "dev-bad", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := v.Verify(context.Background(), tt.token)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Verify(%q) error = %v, wantErr %v", tt.token, err, tt.wantErr)
			}
			if !tt.wantErr && got.DiscordID != tt.wantID {
				t.Errorf("DiscordID = %q, want %q", got.DiscordID, tt.wantID)
			}
		})
	}
}

func TestNewVerifierRefusesDevUserOutsideALocalDevelopmentSetup(t *testing.T) {
	local := Config{DevUser: "100000000000000001", Origin: "http://127.0.0.1:4322", ListenAddr: "127.0.0.1:8080"}
	for _, tt := range []struct {
		name string
		edit func(*Config)
	}{
		{"the admin site's https origin", func(c *Config) { c.Origin = "https://admin.example.ttacommunity.org" }},
		{"no origin", func(c *Config) { c.Origin = "" }},
		{"a localhost origin", func(c *Config) { c.Origin = "http://localhost:4322" }},
		{"a dev user that is not a Discord id", func(c *Config) { c.DevUser = "not-a-discord-id" }},
		{"listening on every interface", func(c *Config) { c.ListenAddr = ":8080" }},
		{"listening on every IPv4 interface", func(c *Config) { c.ListenAddr = "0.0.0.0:8080" }},
		{"listening on every IPv6 interface", func(c *Config) { c.ListenAddr = "[::]:8080" }},
		{"listening on a LAN address", func(c *Config) { c.ListenAddr = "192.168.1.5:8080" }},
		{"running in Kubernetes", func(c *Config) { c.InKubernetes = true }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := local
			tt.edit(&c)
			if _, err := NewVerifier(c, http.DefaultClient, time.Now); err == nil {
				t.Errorf("NewVerifier(%+v): nil error, want a refusal", c)
			}
		})
	}
}
