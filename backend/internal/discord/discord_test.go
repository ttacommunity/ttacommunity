package discord_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"ttacommunity/backend/internal/discord"
)

// stub is a Discord that accepts one code and verifier for the test's client, and answers /users/@me with user.
func stub(t *testing.T, user map[string]any, meStatus int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("token request form: %v", err)
		}
		want := map[string]string{"grant_type": "authorization_code", "code": "good-code", "code_verifier": "the-verifier",
			"client_id": "100000000000000099", "client_secret": "test-secret", "redirect_uri": "https://api.example.test/v1/auth/discord/callback"}
		for k, v := range want {
			if r.PostForm.Get(k) != v {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"Bearer","expires_in":604800,"scope":"identify"}`))
	})
	mux.HandleFunc("GET /api/users/@me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-1" || meStatus != http.StatusOK {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(user)
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func client(t *testing.T, s *httptest.Server) *discord.Client {
	t.Helper()
	c, err := discord.New(discord.Config{
		ClientID: "100000000000000099", ClientSecret: "test-secret",
		RedirectURL: "https://api.example.test/v1/auth/discord/callback",
		AuthURL:     s.URL + "/oauth2/authorize", APIURL: s.URL + "/api",
	}, s.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestTheAuthorizeURLAsksForIdentifyWithStateAndPKCEAndNoRepeatedConsent(t *testing.T) {
	c := client(t, stub(t, nil, http.StatusOK))
	u, err := url.Parse(c.AuthCodeURL("the-state", "the-verifier"))
	if err != nil {
		t.Fatalf("AuthCodeURL is not a URL: %v", err)
	}
	sum := sha256.Sum256([]byte("the-verifier"))
	want := map[string]string{
		"client_id": "100000000000000099", "redirect_uri": "https://api.example.test/v1/auth/discord/callback",
		"response_type": "code", "scope": "identify", "state": "the-state",
		"code_challenge": base64.RawURLEncoding.EncodeToString(sum[:]), "code_challenge_method": "S256",
		"prompt": "none",
	}
	for k, v := range want {
		if got := u.Query().Get(k); got != v {
			t.Errorf("AuthCodeURL's %s = %q, want %q", k, got, v)
		}
	}
}

func TestUserReturnsWhoSignedIn(t *testing.T) {
	c := client(t, stub(t, map[string]any{"id": "100000000000000001", "username": "player_one", "global_name": "Player One"}, http.StatusOK))
	got, err := c.User(context.Background(), "good-code", "the-verifier")
	if err != nil {
		t.Fatalf("User: %v", err)
	}
	if got != (discord.User{ID: "100000000000000001", Username: "player_one", DisplayName: "Player One"}) {
		t.Errorf("User = %+v, want player_one", got)
	}
}

func TestUserIsRefused(t *testing.T) {
	ok := map[string]any{"id": "100000000000000001", "username": "player_one", "global_name": nil}
	tests := []struct {
		name, code, verifier string
		user                 map[string]any
		meStatus             int
	}{
		{"a code Discord refuses", "bad-code", "the-verifier", ok, http.StatusOK},
		{"a verifier that does not match", "good-code", "another-verifier", ok, http.StatusOK},
		{"Discord refusing the token", "good-code", "the-verifier", ok, http.StatusUnauthorized},
		{"an id that is not a Discord id", "good-code", "the-verifier", map[string]any{"id": "x1", "username": "player_one"}, http.StatusOK},
		{"no username", "good-code", "the-verifier", map[string]any{"id": "100000000000000001", "username": ""}, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := client(t, stub(t, tt.user, tt.meStatus))
			if got, err := c.User(context.Background(), tt.code, tt.verifier); err == nil {
				t.Errorf("User = %+v, want an error", got)
			}
		})
	}
}

func TestNewRefusesAnIncompleteConfig(t *testing.T) {
	full := discord.Config{ClientID: "100000000000000099", ClientSecret: "s", RedirectURL: "https://api.example.test/cb"}
	tests := []struct {
		name string
		edit func(*discord.Config)
	}{
		{"no client id", func(c *discord.Config) { c.ClientID = "" }},
		{"no client secret", func(c *discord.Config) { c.ClientSecret = "" }},
		{"no redirect URL", func(c *discord.Config) { c.RedirectURL = "" }},
		{"a redirect URL over plain http elsewhere than this machine", func(c *discord.Config) { c.RedirectURL = "http://api.example.test/cb" }},
		{"Discord's URLs moved off Discord, not to this machine", func(c *discord.Config) { c.APIURL = "https://discord.example.test/api" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := full
			tt.edit(&cfg)
			if _, err := discord.New(cfg, http.DefaultClient); err == nil {
				t.Errorf("New(%+v) = nil error, want refused", cfg)
			}
		})
	}
	if _, err := discord.New(full, http.DefaultClient); err != nil {
		t.Errorf("New with every setting = %v, want nil", err)
	}
}
