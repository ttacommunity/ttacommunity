package resend_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ttacommunity/backend/internal/resend"
)

// sent is what the stub received.
type sent struct {
	auth, idempotency string
	body              map[string]any
}

// stub is a Resend that answers status and keeps what it was sent.
func stub(t *testing.T, status int) (*httptest.Server, *sent) {
	t.Helper()
	got := &sent{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/emails" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		got.auth, got.idempotency = r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key")
		_ = json.NewDecoder(r.Body).Decode(&got.body)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"id":"49a3999c-0ce1-4ea6-ab68-afcd6dc2e794"}`))
	}))
	t.Cleanup(s.Close)
	return s, got
}

func client(t *testing.T, s *httptest.Server) *resend.Client {
	t.Helper()
	c, err := resend.New(resend.Config{APIKey: "re_test", From: "TTA Community <noreply@example.test>", URL: s.URL}, s.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestSendPostsTheMessageFromTheSender(t *testing.T) {
	s, got := stub(t, http.StatusOK)
	m := resend.Message{To: "player@example.test", Subject: "Sign in", Text: "the link", HTML: "<p>the link</p>", IdempotencyKey: "key-1"}
	if err := client(t, s).Send(context.Background(), m); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got.auth != "Bearer re_test" || got.idempotency != "key-1" {
		t.Errorf("headers = %q, %q, want Bearer re_test and key-1", got.auth, got.idempotency)
	}
	want := map[string]any{"from": "TTA Community <noreply@example.test>", "to": []any{"player@example.test"}, "subject": "Sign in",
		"text": "the link", "html": "<p>the link</p>"}
	for k, v := range want {
		b, _ := json.Marshal(got.body[k])
		w, _ := json.Marshal(v)
		if string(b) != string(w) {
			t.Errorf("body %s = %s, want %s", k, b, w)
		}
	}
}

func TestSendLeavesOutAnEmptyHTML(t *testing.T) {
	s, got := stub(t, http.StatusOK)
	if err := client(t, s).Send(context.Background(), resend.Message{To: "player@example.test", Subject: "s", Text: "t"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if _, ok := got.body["html"]; ok {
		t.Errorf("body = %v, want no html for a plain-text message", got.body)
	}
}

func TestSendFailsWhenResendRefuses(t *testing.T) {
	for _, status := range []int{http.StatusUnprocessableEntity, http.StatusTooManyRequests, http.StatusInternalServerError} {
		s, _ := stub(t, status)
		if err := client(t, s).Send(context.Background(), resend.Message{To: "player@example.test", Subject: "s", Text: "t"}); err == nil {
			t.Errorf("Send with Resend answering %d = nil, want an error", status)
		}
	}
}

func TestNewRefusesAConfigThatCannotSend(t *testing.T) {
	tests := []struct {
		name string
		cfg  resend.Config
	}{
		{"no key", resend.Config{From: "noreply@example.test"}},
		{"no sender", resend.Config{APIKey: "re_test"}},
		{"another API", resend.Config{APIKey: "re_test", From: "noreply@example.test", URL: "https://example.test"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := resend.New(tt.cfg, http.DefaultClient); err == nil {
				t.Errorf("New(%s) = nil error, want a refusal", tt.name)
			}
		})
	}
}

func TestConfigKeepsTheKeyOutOfItsString(t *testing.T) {
	if s := (resend.Config{APIKey: "re_secret", From: "noreply@example.test"}).String(); strings.Contains(s, "re_secret") {
		t.Errorf("Config.String() = %q, want the key left out", s)
	}
}
