package microsoft_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"ttacommunity/backend/internal/microsoft"
)

const (
	clientID = "00000000-0000-0000-0000-000000000099"
	tenant   = "00000000-0000-0000-0000-0000000000aa"
	// consumers is Microsoft's tenant for personal accounts.
	consumers = "9188040d-6c67-4c5b-b112-36a304b66dad"
	redirect  = "https://api.example.test/v1/auth/microsoft/callback"
	subject   = "AAAAAAAAAAAAAAAAAAAAAIkzqFVrSaSaFHy782bbtaQ"
)

var now = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// signingKey is Microsoft's key in these tests; otherKey is one Microsoft never published.
var signingKey, otherKey = newKey(), newKey()

func newKey() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}

// stub is a Microsoft that accepts one code and verifier for the test's client, answers with idToken, and publishes
// signingKey as kid "key-1".
func stub(t *testing.T, idToken func(issuerBase string) string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var s *httptest.Server
	mux.HandleFunc("POST /common/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("token request form: %v", err)
		}
		want := map[string]string{"grant_type": "authorization_code", "code": "good-code", "code_verifier": "the-verifier",
			"client_id": clientID, "client_secret": "test-secret", "redirect_uri": redirect}
		for k, v := range want {
			if r.PostForm.Get(k) != v {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access-1", "token_type": "Bearer", "expires_in": 3600,
			"id_token": idToken(s.URL)})
	})
	mux.HandleFunc("GET /common/discovery/v2.0/keys", func(w http.ResponseWriter, r *http.Request) {
		pub := signingKey.PublicKey
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "key-1", "kty": "RSA", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	})
	s = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func client(t *testing.T, s *httptest.Server) *microsoft.Client {
	t.Helper()
	c, err := microsoft.New(microsoft.Config{ClientID: clientID, ClientSecret: "test-secret", RedirectURL: redirect, Authority: s.URL},
		s.Client(), func() time.Time { return now })
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// claims are an ID token's claims as Microsoft sends them for a personal account with a verified email.
func claims(issuerBase string) jwt.MapClaims {
	return jwt.MapClaims{
		"iss": issuerBase + "/" + tenant + "/v2.0", "aud": clientID, "tid": tenant, "sub": subject,
		"iat": now.Add(-time.Minute).Unix(), "nbf": now.Add(-time.Minute).Unix(), "exp": now.Add(time.Hour).Unix(),
		"nonce": "the-nonce", "name": "Player One", "email": "player@example.test", "xms_edov": true,
	}
}

// personal makes c a personal account's: Microsoft's consumer tenant, with the issuer to match.
func personal(c jwt.MapClaims) {
	c["iss"] = strings.Replace(c["iss"].(string), tenant, consumers, 1)
	c["tid"] = consumers
}

// signed is c signed with key as kid.
func signed(t *testing.T, c jwt.MapClaims, key *rsa.PrivateKey, kid string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	return s
}

func TestTheAuthorizeURLAsksForSignInAndEmailWithStatePKCEAndNonce(t *testing.T) {
	c := client(t, stub(t, nil))
	u, err := url.Parse(c.AuthCodeURL("the-state", "the-verifier", "the-nonce"))
	if err != nil {
		t.Fatalf("AuthCodeURL is not a URL: %v", err)
	}
	if u.Path != "/common/oauth2/v2.0/authorize" {
		t.Errorf("AuthCodeURL's path = %q, want the common endpoint", u.Path)
	}
	sum := sha256.Sum256([]byte("the-verifier"))
	want := map[string]string{
		"client_id": clientID, "redirect_uri": redirect, "response_type": "code", "response_mode": "query",
		"scope": "openid profile email", "state": "the-state", "nonce": "the-nonce",
		"code_challenge": base64.RawURLEncoding.EncodeToString(sum[:]), "code_challenge_method": "S256",
	}
	for k, v := range want {
		if got := u.Query().Get(k); got != v {
			t.Errorf("AuthCodeURL's %s = %q, want %q", k, got, v)
		}
	}
}

func TestUserReturnsWhoSignedInWithTheEmailOnlyWhenVerified(t *testing.T) {
	tests := []struct {
		name      string
		edit      func(jwt.MapClaims)
		wantEmail string
	}{
		{"a verified email", func(jwt.MapClaims) {}, "player@example.test"},
		{"an email not said to be verified", func(c jwt.MapClaims) { delete(c, "xms_edov") }, ""},
		{"an email said to be unverified", func(c jwt.MapClaims) { c["xms_edov"] = false }, ""},
		{"verified said as text", func(c jwt.MapClaims) { c["xms_edov"] = "true" }, ""},
		{"no email", func(c jwt.MapClaims) { delete(c, "email") }, ""},
		{"a personal account's email, which Microsoft proved", func(c jwt.MapClaims) { personal(c); delete(c, "xms_edov") }, "player@example.test"},
		{"a personal account with no email", func(c jwt.MapClaims) { personal(c); delete(c, "xms_edov"); delete(c, "email") }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := client(t, stub(t, func(base string) string {
				cl := claims(base)
				tt.edit(cl)
				return signed(t, cl, signingKey, "key-1")
			}))
			got, err := c.User(context.Background(), "good-code", "the-verifier", "the-nonce")
			if err != nil {
				t.Fatalf("User: %v", err)
			}
			if want := (microsoft.User{Subject: subject, Name: "Player One", Email: tt.wantEmail}); got != want {
				t.Errorf("User = %+v, want %+v", got, want)
			}
		})
	}
}

func TestUserIsRefused(t *testing.T) {
	tests := []struct {
		name, code, verifier, nonce string
		token                       func(t *testing.T, base string) string
	}{
		{"a code Microsoft refuses", "bad-code", "the-verifier", "the-nonce", nil},
		{"a verifier that does not match", "good-code", "another-verifier", "the-nonce", nil},
		{"another nonce", "good-code", "the-verifier", "another-nonce", nil},
		{"a key Microsoft never published", "good-code", "the-verifier", "the-nonce", func(t *testing.T, base string) string {
			return signed(t, claims(base), otherKey, "key-1")
		}},
		{"an unknown key id", "good-code", "the-verifier", "the-nonce", func(t *testing.T, base string) string {
			return signed(t, claims(base), signingKey, "key-2")
		}},
		{"no signature", "good-code", "the-verifier", "the-nonce", func(t *testing.T, base string) string {
			s, _ := jwt.NewWithClaims(jwt.SigningMethodNone, claims(base)).SignedString(jwt.UnsafeAllowNoneSignatureType)
			return s
		}},
		{"another app's token", "good-code", "the-verifier", "the-nonce", func(t *testing.T, base string) string {
			c := claims(base)
			c["aud"] = "00000000-0000-0000-0000-000000000098"
			return signed(t, c, signingKey, "key-1")
		}},
		{"expired", "good-code", "the-verifier", "the-nonce", func(t *testing.T, base string) string {
			c := claims(base)
			c["exp"] = now.Add(-time.Minute).Unix()
			return signed(t, c, signingKey, "key-1")
		}},
		{"no expiry", "good-code", "the-verifier", "the-nonce", func(t *testing.T, base string) string {
			c := claims(base)
			delete(c, "exp")
			return signed(t, c, signingKey, "key-1")
		}},
		{"an issuer for another tenant than the token's", "good-code", "the-verifier", "the-nonce", func(t *testing.T, base string) string {
			c := claims(base)
			c["iss"] = base + "/00000000-0000-0000-0000-0000000000bb/v2.0"
			return signed(t, c, signingKey, "key-1")
		}},
		{"an issuer elsewhere", "good-code", "the-verifier", "the-nonce", func(t *testing.T, base string) string {
			c := claims(base)
			c["iss"] = "https://issuer.example.test/" + tenant + "/v2.0"
			return signed(t, c, signingKey, "key-1")
		}},
		{"a tenant that is not a tenant id", "good-code", "the-verifier", "the-nonce", func(t *testing.T, base string) string {
			c := claims(base)
			c["tid"], c["iss"] = "common", base+"/common/v2.0"
			return signed(t, c, signingKey, "key-1")
		}},
		{"no subject", "good-code", "the-verifier", "the-nonce", func(t *testing.T, base string) string {
			c := claims(base)
			delete(c, "sub")
			return signed(t, c, signingKey, "key-1")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := client(t, stub(t, func(base string) string {
				if tt.token == nil {
					return signed(t, claims(base), signingKey, "key-1")
				}
				return tt.token(t, base)
			}))
			if got, err := c.User(context.Background(), tt.code, tt.verifier, tt.nonce); err == nil {
				t.Errorf("User = %+v, want an error", got)
			}
		})
	}
}

func TestNewRefusesAnIncompleteConfig(t *testing.T) {
	full := microsoft.Config{ClientID: clientID, ClientSecret: "s", RedirectURL: redirect}
	tests := []struct {
		name string
		edit func(*microsoft.Config)
	}{
		{"no client id", func(c *microsoft.Config) { c.ClientID = "" }},
		{"no client secret", func(c *microsoft.Config) { c.ClientSecret = "" }},
		{"no redirect URL", func(c *microsoft.Config) { c.RedirectURL = "" }},
		{"a redirect URL over plain http elsewhere than this machine", func(c *microsoft.Config) { c.RedirectURL = "http://api.example.test/cb" }},
		{"an authority that is not Microsoft's or this machine", func(c *microsoft.Config) { c.Authority = "https://login.example.test" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := full
			tt.edit(&cfg)
			if _, err := microsoft.New(cfg, http.DefaultClient, time.Now); err == nil {
				t.Errorf("New(%+v) = nil error, want refused", cfg)
			}
		})
	}
	if _, err := microsoft.New(full, http.DefaultClient, time.Now); err != nil {
		t.Errorf("New with every setting = %v, want nil", err)
	}
}
