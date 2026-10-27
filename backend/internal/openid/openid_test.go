package openid_test

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

	"ttacommunity/backend/internal/openid"
)

const (
	clientID = "00000000-0000-0000-0000-000000000099"
	tenant   = "00000000-0000-0000-0000-0000000000aa"
	// consumers is Microsoft's tenant for personal accounts.
	consumers = "9188040d-6c67-4c5b-b112-36a304b66dad"
	redirect  = "https://api.example.test/v1/auth/callback"
	subject   = "AAAAAAAAAAAAAAAAAAAAAIkzqFVrSaSaFHy782bbtaQ"
)

var now = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// signingKey is the provider's key in these tests; otherKey is one it never published.
var signingKey, otherKey = newKey(), newKey()

func newKey() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}

// provider is one provider under test: its config, where its stub answers, and an ID token's claims as it sends them
// for an account with a verified email, given the stub's base URL.
type provider struct {
	name                string
	p                   openid.Provider
	tokenPath, keysPath string
	claims              func(base string) jwt.MapClaims
	verified            string
}

var providers = []provider{
	{"microsoft", openid.Microsoft, "/common/oauth2/v2.0/token", "/common/discovery/v2.0/keys", func(base string) jwt.MapClaims {
		return jwt.MapClaims{
			"iss": base + "/" + tenant + "/v2.0", "aud": clientID, "tid": tenant, "sub": subject,
			"iat": now.Add(-time.Minute).Unix(), "nbf": now.Add(-time.Minute).Unix(), "exp": now.Add(time.Hour).Unix(),
			"nonce": "the-nonce", "name": "Player One", "email": "player@example.test", "xms_edov": true,
		}
	}, "xms_edov"},
	{"google", openid.Google, "/token", "/oauth2/v3/certs", func(string) jwt.MapClaims {
		return jwt.MapClaims{
			"iss": "https://accounts.google.com", "azp": clientID, "aud": clientID, "sub": subject,
			"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(time.Hour).Unix(),
			"nonce": "the-nonce", "name": "Player One", "email": "player@example.test", "email_verified": true,
		}
	}, "email_verified"},
}

func byName(t *testing.T, name string) provider {
	t.Helper()
	for _, p := range providers {
		if p.name == name {
			return p
		}
	}
	t.Fatalf("no provider %q", name)
	return provider{}
}

// stub is the provider, accepting one code and verifier for the test's client, answering with idToken, and publishing
// signingKey as kid "key-1".
func stub(t *testing.T, pr provider, idToken func(base string) string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var s *httptest.Server
	mux.HandleFunc("POST "+pr.tokenPath, func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("GET "+pr.keysPath, func(w http.ResponseWriter, r *http.Request) {
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

func client(t *testing.T, pr provider, s *httptest.Server) *openid.Client {
	t.Helper()
	c, err := openid.New(pr.p, openid.Config{ClientID: clientID, ClientSecret: "test-secret", RedirectURL: redirect, Stub: s.URL},
		s.Client(), func() time.Time { return now })
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
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

// userWith signs in through pr's stub with its claims after edit, and returns who signed in.
func userWith(t *testing.T, pr provider, edit func(jwt.MapClaims)) (openid.User, error) {
	t.Helper()
	c := client(t, pr, stub(t, pr, func(base string) string {
		cl := pr.claims(base)
		edit(cl)
		return signed(t, cl, signingKey, "key-1")
	}))
	return c.User(context.Background(), "good-code", "the-verifier", "the-nonce")
}

func TestTheAuthorizeURLAsksForSignInAndEmailWithStatePKCEAndNonce(t *testing.T) {
	tests := []struct {
		provider, path, scope, responseMode string
	}{
		{"microsoft", "/common/oauth2/v2.0/authorize", "openid profile email", "query"},
		{"google", "/o/oauth2/v2/auth", "openid email profile", ""},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			pr := byName(t, tt.provider)
			c := client(t, pr, stub(t, pr, nil))
			u, err := url.Parse(c.AuthCodeURL("the-state", "the-verifier", "the-nonce"))
			if err != nil {
				t.Fatalf("AuthCodeURL is not a URL: %v", err)
			}
			if u.Path != tt.path {
				t.Errorf("AuthCodeURL's path = %q, want %q", u.Path, tt.path)
			}
			sum := sha256.Sum256([]byte("the-verifier"))
			want := map[string]string{
				"client_id": clientID, "redirect_uri": redirect, "response_type": "code", "response_mode": tt.responseMode,
				"scope": tt.scope, "state": "the-state", "nonce": "the-nonce",
				"code_challenge": base64.RawURLEncoding.EncodeToString(sum[:]), "code_challenge_method": "S256",
			}
			for k, v := range want {
				if got := u.Query().Get(k); got != v {
					t.Errorf("AuthCodeURL's %s = %q, want %q", k, got, v)
				}
			}
		})
	}
}

func TestUserReturnsWhoSignedInWithTheEmailOnlyWhenVerified(t *testing.T) {
	tests := []struct {
		name      string
		edit      func(c jwt.MapClaims, verified string)
		wantEmail string
	}{
		{"a verified email", func(jwt.MapClaims, string) {}, "player@example.test"},
		{"an email not said to be verified", func(c jwt.MapClaims, v string) { delete(c, v) }, ""},
		{"an email said to be unverified", func(c jwt.MapClaims, v string) { c[v] = false }, ""},
		{"verified said as text", func(c jwt.MapClaims, v string) { c[v] = "true" }, ""},
		{"no email", func(c jwt.MapClaims, _ string) { delete(c, "email") }, ""},
		{"a name with spaces round it", func(c jwt.MapClaims, _ string) { c["name"] = "  Player One " }, "player@example.test"},
	}
	for _, pr := range providers {
		for _, tt := range tests {
			t.Run(pr.name+"/"+tt.name, func(t *testing.T) {
				got, err := userWith(t, pr, func(c jwt.MapClaims) { tt.edit(c, pr.verified) })
				if err != nil {
					t.Fatalf("User: %v", err)
				}
				if want := (openid.User{Subject: subject, Name: "Player One", Email: tt.wantEmail}); got != want {
					t.Errorf("User = %+v, want %+v", got, want)
				}
			})
		}
	}
}

func TestUserCutsALongNameTo100Characters(t *testing.T) {
	for _, pr := range providers {
		t.Run(pr.name, func(t *testing.T) {
			got, err := userWith(t, pr, func(c jwt.MapClaims) { c["name"] = strings.Repeat("é", 120) })
			if err != nil {
				t.Fatalf("User: %v", err)
			}
			if want := strings.Repeat("é", 100); got.Name != want {
				t.Errorf("User's name has %d characters, want 100", len([]rune(got.Name)))
			}
		})
	}
}

func TestUserTakesAMicrosoftPersonalAccountsEmailAsProved(t *testing.T) {
	personal := func(c jwt.MapClaims) {
		c["iss"] = strings.Replace(c["iss"].(string), tenant, consumers, 1)
		c["tid"] = consumers
		delete(c, "xms_edov")
	}
	tests := []struct {
		name      string
		edit      func(jwt.MapClaims)
		wantEmail string
	}{
		{"a personal account's email, which Microsoft proved", personal, "player@example.test"},
		{"a personal account with no email", func(c jwt.MapClaims) { personal(c); delete(c, "email") }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := userWith(t, byName(t, "microsoft"), tt.edit)
			if err != nil {
				t.Fatalf("User: %v", err)
			}
			if got.Email != tt.wantEmail {
				t.Errorf("User's email = %q, want %q", got.Email, tt.wantEmail)
			}
		})
	}
}

func TestUserTakesGooglesIssuerWithoutItsScheme(t *testing.T) {
	got, err := userWith(t, byName(t, "google"), func(c jwt.MapClaims) { c["iss"] = "accounts.google.com" })
	if err != nil {
		t.Fatalf("User with issuer accounts.google.com: %v", err)
	}
	if got.Subject != subject {
		t.Errorf("User's subject = %q, want %q", got.Subject, subject)
	}
}

func TestUserIsRefused(t *testing.T) {
	tests := []struct {
		name, code, verifier, nonce string
		token                       func(t *testing.T, c jwt.MapClaims) string
	}{
		{"a code the provider refuses", "bad-code", "the-verifier", "the-nonce", nil},
		{"a verifier that does not match", "good-code", "another-verifier", "the-nonce", nil},
		{"another nonce", "good-code", "the-verifier", "another-nonce", nil},
		{"no nonce", "good-code", "the-verifier", "", func(t *testing.T, c jwt.MapClaims) string {
			delete(c, "nonce")
			return signed(t, c, signingKey, "key-1")
		}},
		{"a key the provider never published", "good-code", "the-verifier", "the-nonce", func(t *testing.T, c jwt.MapClaims) string {
			return signed(t, c, otherKey, "key-1")
		}},
		{"an unknown key id", "good-code", "the-verifier", "the-nonce", func(t *testing.T, c jwt.MapClaims) string {
			return signed(t, c, signingKey, "key-2")
		}},
		{"no signature", "good-code", "the-verifier", "the-nonce", func(t *testing.T, c jwt.MapClaims) string {
			s, _ := jwt.NewWithClaims(jwt.SigningMethodNone, c).SignedString(jwt.UnsafeAllowNoneSignatureType)
			return s
		}},
		{"the provider's key with another algorithm", "good-code", "the-verifier", "the-nonce", func(t *testing.T, c jwt.MapClaims) string {
			tok := jwt.NewWithClaims(jwt.SigningMethodPS256, c)
			tok.Header["kid"] = "key-1"
			s, err := tok.SignedString(signingKey)
			if err != nil {
				t.Fatalf("signing: %v", err)
			}
			return s
		}},
		{"another app's token", "good-code", "the-verifier", "the-nonce", func(t *testing.T, c jwt.MapClaims) string {
			c["aud"] = "00000000-0000-0000-0000-000000000098"
			return signed(t, c, signingKey, "key-1")
		}},
		{"expired", "good-code", "the-verifier", "the-nonce", func(t *testing.T, c jwt.MapClaims) string {
			c["exp"] = now.Add(-time.Minute).Unix()
			return signed(t, c, signingKey, "key-1")
		}},
		{"no expiry", "good-code", "the-verifier", "the-nonce", func(t *testing.T, c jwt.MapClaims) string {
			delete(c, "exp")
			return signed(t, c, signingKey, "key-1")
		}},
		{"an issuer elsewhere", "good-code", "the-verifier", "the-nonce", func(t *testing.T, c jwt.MapClaims) string {
			c["iss"] = "https://issuer.example.test/" + tenant + "/v2.0"
			return signed(t, c, signingKey, "key-1")
		}},
		{"no subject", "good-code", "the-verifier", "the-nonce", func(t *testing.T, c jwt.MapClaims) string {
			delete(c, "sub")
			return signed(t, c, signingKey, "key-1")
		}},
		{"a subject over 255 characters", "good-code", "the-verifier", "the-nonce", func(t *testing.T, c jwt.MapClaims) string {
			c["sub"] = strings.Repeat("a", 256)
			return signed(t, c, signingKey, "key-1")
		}},
	}
	for _, pr := range providers {
		for _, tt := range tests {
			t.Run(pr.name+"/"+tt.name, func(t *testing.T) {
				c := client(t, pr, stub(t, pr, func(base string) string {
					if tt.token == nil {
						return signed(t, pr.claims(base), signingKey, "key-1")
					}
					return tt.token(t, pr.claims(base))
				}))
				if got, err := c.User(context.Background(), tt.code, tt.verifier, tt.nonce); err == nil {
					t.Errorf("User = %+v, want an error", got)
				}
			})
		}
	}
}

func TestUserRefusesAMicrosoftIssuerThatIsNotTheTokensTenant(t *testing.T) {
	tests := []struct {
		name string
		edit func(c jwt.MapClaims, base string)
	}{
		{"an issuer for another tenant than the token's", func(c jwt.MapClaims, base string) {
			c["iss"] = base + "/00000000-0000-0000-0000-0000000000bb/v2.0"
		}},
		{"a tenant that is not a tenant id", func(c jwt.MapClaims, base string) {
			c["tid"], c["iss"] = "common", base+"/common/v2.0"
		}},
	}
	pr := byName(t, "microsoft")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := client(t, pr, stub(t, pr, func(base string) string {
				cl := pr.claims(base)
				tt.edit(cl, base)
				return signed(t, cl, signingKey, "key-1")
			}))
			if got, err := c.User(context.Background(), "good-code", "the-verifier", "the-nonce"); err == nil {
				t.Errorf("User = %+v, want an error", got)
			}
		})
	}
}

func TestNewRefusesAnIncompleteConfig(t *testing.T) {
	full := openid.Config{ClientID: clientID, ClientSecret: "s", RedirectURL: redirect}
	tests := []struct {
		name string
		edit func(*openid.Config)
	}{
		{"no client id", func(c *openid.Config) { c.ClientID = "" }},
		{"no client secret", func(c *openid.Config) { c.ClientSecret = "" }},
		{"no redirect URL", func(c *openid.Config) { c.RedirectURL = "" }},
		{"a redirect URL over plain http elsewhere than this machine", func(c *openid.Config) { c.RedirectURL = "http://api.example.test/cb" }},
		{"a stand-in for the provider elsewhere than this machine", func(c *openid.Config) { c.Stub = "https://login.example.test" }},
	}
	for _, pr := range providers {
		for _, tt := range tests {
			t.Run(pr.name+"/"+tt.name, func(t *testing.T) {
				cfg := full
				tt.edit(&cfg)
				if _, err := openid.New(pr.p, cfg, http.DefaultClient, time.Now); err == nil {
					t.Errorf("New(%+v) = nil error, want refused", cfg)
				}
			})
		}
		if _, err := openid.New(pr.p, full, http.DefaultClient, time.Now); err != nil {
			t.Errorf("New(%s) with every setting = %v, want nil", pr.name, err)
		}
	}
}

func TestTheConfigKeepsItsSecretOutOfLogs(t *testing.T) {
	cfg := openid.Config{ClientID: clientID, ClientSecret: "test-secret", RedirectURL: redirect}
	if s := cfg.String(); !strings.Contains(s, clientID) || strings.Contains(s, "test-secret") {
		t.Errorf("Config.String() = %q, want the client id without the secret", s)
	}
}
