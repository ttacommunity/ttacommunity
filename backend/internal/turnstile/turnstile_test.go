package turnstile_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ttacommunity/backend/internal/turnstile"
)

// stub is a siteverify that answers with reply when sent the test's secret, and counts the calls it gets.
func stub(t *testing.T, reply map[string]any, status int) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct{ Secret, Response string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Secret != "test-secret" || body.Response != "a-token" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(reply)
	}))
	t.Cleanup(s.Close)
	return s, &calls
}

func TestVerifyPassesOnlyASolvedTokenForThisSiteAndAction(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		reply    map[string]any
		status   int
		want     error
	}{
		{"solved here for this action", "site.example.test", map[string]any{"success": true, "hostname": "site.example.test", "action": "email-signin"}, http.StatusOK, nil},
		{"not solved", "site.example.test", map[string]any{"success": false, "error-codes": []string{"invalid-input-response"}}, http.StatusOK, turnstile.ErrFailed},
		{"solved on another site", "site.example.test", map[string]any{"success": true, "hostname": "other.example.test", "action": "email-signin"}, http.StatusOK, turnstile.ErrFailed},
		{"solved for another action", "site.example.test", map[string]any{"success": true, "hostname": "site.example.test", "action": "other"}, http.StatusOK, turnstile.ErrFailed},
		{"any site and action when no site is set", "", map[string]any{"success": true, "hostname": "example.com"}, http.StatusOK, nil},
		{"not solved when no site is set", "", map[string]any{"success": false}, http.StatusOK, turnstile.ErrFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := stub(t, tt.reply, tt.status)
			c, err := turnstile.New(turnstile.Config{Secret: "test-secret", Hostname: tt.hostname, URL: s.URL}, s.Client())
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if err := c.Verify(context.Background(), "a-token", "email-signin"); !errors.Is(err, tt.want) {
				t.Errorf("Verify = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestVerifyFailsWhenSiteverifyCannotAnswer(t *testing.T) {
	s, _ := stub(t, map[string]any{}, http.StatusInternalServerError)
	c, err := turnstile.New(turnstile.Config{Secret: "test-secret", URL: s.URL}, s.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = c.Verify(context.Background(), "a-token", "email-signin")
	if err == nil || errors.Is(err, turnstile.ErrFailed) {
		t.Errorf("Verify with siteverify down = %v, want an error that is not ErrFailed", err)
	}
}

func TestVerifyRefusesAnEmptyOrOverlongTokenWithoutAsking(t *testing.T) {
	for _, token := range []string{"", strings.Repeat("x", 2049)} {
		s, calls := stub(t, map[string]any{"success": true}, http.StatusOK)
		c, err := turnstile.New(turnstile.Config{Secret: "test-secret", URL: s.URL}, s.Client())
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if err := c.Verify(context.Background(), token, "email-signin"); !errors.Is(err, turnstile.ErrFailed) || *calls != 0 {
			t.Errorf("Verify(a token of %d characters) = %v after %d calls, want ErrFailed and none", len(token), err, *calls)
		}
	}
}

func TestNewRefusesAConfigThatCannotBeChecked(t *testing.T) {
	tests := []struct {
		name string
		cfg  turnstile.Config
	}{
		{"no secret", turnstile.Config{}},
		{"siteverify somewhere else", turnstile.Config{Secret: "s", URL: "https://example.test/siteverify"}},
		{"siteverify over plain http", turnstile.Config{Secret: "s", URL: "http://challenges.cloudflare.com/turnstile/v0/siteverify"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := turnstile.New(tt.cfg, http.DefaultClient); err == nil {
				t.Errorf("New(%s) = nil error, want a refusal", tt.name)
			}
		})
	}
}

func TestConfigKeepsTheSecretOutOfItsString(t *testing.T) {
	if s := (turnstile.Config{Secret: "the-secret", Hostname: "site.example.test"}).String(); strings.Contains(s, "the-secret") {
		t.Errorf("Config.String() = %q, want the secret left out", s)
	}
}
