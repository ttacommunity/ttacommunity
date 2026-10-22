package google_test

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

	"ttacommunity/backend/internal/google"
)

const (
	clientID = "100000000001-test.apps.googleusercontent.com"
	redirect = "https://api.example.test/v1/auth/google/callback"
	subject  = "100000000000000000001"
)

var now = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

// signingKey is Google's key in these tests; otherKey is one Google never published.
var signingKey, otherKey = newKey(), newKey()

func newKey() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}

// stub is a Google that accepts one code and verifier for the test's client, answers with idToken, and publishes
// signingKey as kid "key-1".
func stub(t *testing.T, idToken func() string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
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
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access-1", "token_type": "Bearer", "expires_in": 3599,
			"id_token": idToken()})
	})
	mux.HandleFunc("GET /oauth2/v3/certs", func(w http.ResponseWriter, r *http.Request) {
		pub := signingKey.PublicKey
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "key-1", "kty": "RSA", "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func client(t *testing.T, s *httptest.Server) *google.Client {
	t.Helper()
	c, err := google.New(google.Config{ClientID: clientID, ClientSecret: "test-secret", RedirectURL: redirect, Stub: s.URL},
		s.Client(), func() time.Time { return now })
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// claims are an ID token's claims as Google sends them for an account with a verified email.
func claims() jwt.MapClaims {
	return jwt.MapClaims{
		"iss": "https://accounts.google.com", "azp": clientID, "aud": clientID, "sub": subject,
		"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(time.Hour).Unix(),
		"nonce": "the-nonce", "name": "Player One", "email": "player@example.test", "email_verified": true,
	}
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
	if u.Path != "/o/oauth2/v2/auth" {
		t.Errorf("AuthCodeURL's path = %q, want Google's authorization endpoint", u.Path)
	}
	sum := sha256.Sum256([]byte("the-verifier"))
	want := map[string]string{
		"client_id": clientID, "redirect_uri": redirect, "response_type": "code",
		"scope": "openid email profile", "state": "the-state", "nonce": "the-nonce",
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
		{"the issuer without its scheme, as Google may send it", func(c jwt.MapClaims) { c["iss"] = "accounts.google.com" }, "player@example.test"},
		{"an email not said to be verified", func(c jwt.MapClaims) { delete(c, "email_verified") }, ""},
		{"an email said to be unverified", func(c jwt.MapClaims) { c["email_verified"] = false }, ""},
		{"verified said as text", func(c jwt.MapClaims) { c["email_verified"] = "true" }, ""},
		{"no email", func(c jwt.MapClaims) { delete(c, "email") }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := client(t, stub(t, func() string {
				cl := claims()
				tt.edit(cl)
				return signed(t, cl, signingKey, "key-1")
			}))
			got, err := c.User(context.Background(), "good-code", "the-verifier", "the-nonce")
			if err != nil {
				t.Fatalf("User: %v", err)
			}
			if want := (google.User{Subject: subject, Name: "Player One", Email: tt.wantEmail}); got != want {
				t.Errorf("User = %+v, want %+v", got, want)
			}
		})
	}
}

func TestUserIsRefused(t *testing.T) {
	tests := []struct {
		name, code, verifier, nonce string
		token                       func(t *testing.T) string
	}{
		{"a code Google refuses", "bad-code", "the-verifier", "the-nonce", nil},
		{"a verifier that does not match", "good-code", "another-verifier", "the-nonce", nil},
		{"another nonce", "good-code", "the-verifier", "another-nonce", nil},
		{"no nonce", "good-code", "the-verifier", "", func(t *testing.T) string {
			c := claims()
			delete(c, "nonce")
			return signed(t, c, signingKey, "key-1")
		}},
		{"a key Google never published", "good-code", "the-verifier", "the-nonce", func(t *testing.T) string {
			return signed(t, claims(), otherKey, "key-1")
		}},
		{"an unknown key id", "good-code", "the-verifier", "the-nonce", func(t *testing.T) string {
			return signed(t, claims(), signingKey, "key-2")
		}},
		{"no signature", "good-code", "the-verifier", "the-nonce", func(t *testing.T) string {
			s, _ := jwt.NewWithClaims(jwt.SigningMethodNone, claims()).SignedString(jwt.UnsafeAllowNoneSignatureType)
			return s
		}},
		{"Google's key with another algorithm", "good-code", "the-verifier", "the-nonce", func(t *testing.T) string {
			tok := jwt.NewWithClaims(jwt.SigningMethodPS256, claims())
			tok.Header["kid"] = "key-1"
			s, err := tok.SignedString(signingKey)
			if err != nil {
				t.Fatalf("signing: %v", err)
			}
			return s
		}},
		{"another app's token", "good-code", "the-verifier", "the-nonce", func(t *testing.T) string {
			c := claims()
			c["aud"] = "100000000002-other.apps.googleusercontent.com"
			return signed(t, c, signingKey, "key-1")
		}},
		{"expired", "good-code", "the-verifier", "the-nonce", func(t *testing.T) string {
			c := claims()
			c["exp"] = now.Add(-time.Minute).Unix()
			return signed(t, c, signingKey, "key-1")
		}},
		{"no expiry", "good-code", "the-verifier", "the-nonce", func(t *testing.T) string {
			c := claims()
			delete(c, "exp")
			return signed(t, c, signingKey, "key-1")
		}},
		{"an issuer elsewhere", "good-code", "the-verifier", "the-nonce", func(t *testing.T) string {
			c := claims()
			c["iss"] = "https://accounts.example.test"
			return signed(t, c, signingKey, "key-1")
		}},
		{"no subject", "good-code", "the-verifier", "the-nonce", func(t *testing.T) string {
			c := claims()
			delete(c, "sub")
			return signed(t, c, signingKey, "key-1")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := client(t, stub(t, func() string {
				if tt.token == nil {
					return signed(t, claims(), signingKey, "key-1")
				}
				return tt.token(t)
			}))
			if got, err := c.User(context.Background(), tt.code, tt.verifier, tt.nonce); err == nil {
				t.Errorf("User = %+v, want an error", got)
			}
		})
	}
}

func TestNewRefusesAnIncompleteConfig(t *testing.T) {
	full := google.Config{ClientID: clientID, ClientSecret: "s", RedirectURL: redirect}
	tests := []struct {
		name string
		edit func(*google.Config)
	}{
		{"no client id", func(c *google.Config) { c.ClientID = "" }},
		{"no client secret", func(c *google.Config) { c.ClientSecret = "" }},
		{"no redirect URL", func(c *google.Config) { c.RedirectURL = "" }},
		{"a redirect URL over plain http elsewhere than this machine", func(c *google.Config) { c.RedirectURL = "http://api.example.test/cb" }},
		{"a stand-in for Google elsewhere than this machine", func(c *google.Config) { c.Stub = "https://accounts.example.test" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := full
			tt.edit(&cfg)
			if _, err := google.New(cfg, http.DefaultClient, time.Now); err == nil {
				t.Errorf("New(%+v) = nil error, want refused", cfg)
			}
		})
	}
	if _, err := google.New(full, http.DefaultClient, time.Now); err != nil {
		t.Errorf("New with every setting = %v, want nil", err)
	}
}

func TestTheConfigKeepsItsSecretOutOfLogs(t *testing.T) {
	cfg := google.Config{ClientID: clientID, ClientSecret: "test-secret", RedirectURL: redirect}
	if s := cfg.String(); !strings.Contains(s, clientID) || strings.Contains(s, "test-secret") {
		t.Errorf("Config.String() = %q, want the client id without the secret", s)
	}
}
