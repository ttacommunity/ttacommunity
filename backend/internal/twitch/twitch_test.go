package twitch

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)

// fakeTwitch is Twitch's token endpoint and Helix, answering with handle after checking the token and client id.
func fakeTwitch(t *testing.T, handle func(w http.ResponseWriter, r *http.Request)) (*Client, *atomic.Int32) {
	t.Helper()
	var tokens atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			if r.FormValue("client_secret") != "client-secret" || r.FormValue("grant_type") != "client_credentials" {
				http.Error(w, "bad", http.StatusBadRequest)
				return
			}
			n := tokens.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "token-" + string(rune('0'+n)), "expires_in": 3600})
			return
		}
		if r.Header.Get("Client-Id") != "client-id" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer token-") {
			http.Error(w, "unauthorised", http.StatusUnauthorized)
			return
		}
		handle(w, r)
	}))
	t.Cleanup(srv.Close)
	return New(srv.Client(), srv.URL, srv.URL, "client-id", "client-secret", func() time.Time { return testNow }), &tokens
}

func TestUserFindsAStreamerByLogin(t *testing.T) {
	tests := []struct {
		name, answer string
		want         User
		wantErr      error
	}{
		{"found", `{"data":[{"id":"100000001","login":"some_streamer","display_name":"Some_Streamer"}]}`,
			User{ID: "100000001", Login: "some_streamer", DisplayName: "Some_Streamer"}, nil},
		{"no such streamer", `{"data":[]}`, User{}, ErrNoUser},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := fakeTwitch(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/users" || r.URL.Query().Get("login") != "some_streamer" {
					http.Error(w, "unexpected", http.StatusNotFound)
					return
				}
				_, _ = io.WriteString(w, tt.answer)
			})
			got, err := c.User(context.Background(), "some_streamer")
			if got != tt.want || !errors.Is(err, tt.wantErr) {
				t.Errorf("User = %+v, %v; want %+v, %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestStreamReadsALiveStreamWithItsPreviewSized(t *testing.T) {
	c, _ := fakeTwitch(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"id":"200000001","user_id":"100000001","game_name":"Through the Ages","title":"Friday games",
			"started_at":"2026-11-18T07:59:00Z","thumbnail_url":"https://static-cdn.jtvnw.net/previews-ttv/live_user_x-{width}x{height}.jpg"}]}`)
	})
	got, live, err := c.Stream(context.Background(), "100000001")
	if err != nil || !live {
		t.Fatalf("Stream = %+v, %t, %v; want a live stream", got, live, err)
	}
	if got.ID != "200000001" || got.Category != "Through the Ages" || got.Title != "Friday games" ||
		got.ThumbnailURL != "https://static-cdn.jtvnw.net/previews-ttv/live_user_x-1280x720.jpg" || !got.StartedAt.Equal(testNow.Add(-time.Minute)) {
		t.Errorf("Stream = %+v", got)
	}
}

func TestStreamSaysWhenNotLive(t *testing.T) {
	c, _ := fakeTwitch(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{"data":[]}`) })
	if _, live, err := c.Stream(context.Background(), "100000001"); live || err != nil {
		t.Errorf("Stream: live %t, %v; want not live, no error", live, err)
	}
}

func TestATokenIsReusedThenRenewedWhenTwitchRefusesIt(t *testing.T) {
	calls := 0
	c, tokens := fakeTwitch(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 {
			http.Error(w, "expired", http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"data":[]}`)
	})
	for range 3 {
		if _, _, err := c.Stream(context.Background(), "100000001"); err != nil {
			t.Fatalf("Stream: %v", err)
		}
	}
	if got := tokens.Load(); got != 2 {
		t.Errorf("tokens asked for = %d, want 2: one, then one more after Twitch refused it", got)
	}
}

func TestSubscribeAsksForAWebhookSubscription(t *testing.T) {
	var asked map[string]any
	c, _ := fakeTwitch(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/eventsub/subscriptions" {
			http.Error(w, "unexpected", http.StatusNotFound)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&asked)
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"data":[{"id":"sub-1","status":"webhook_callback_verification_pending"}]}`)
	})
	got, err := c.Subscribe(context.Background(), "stream.online", "100000001", "https://api.example/v1/twitch/eventsub", "a-secret-of-length")
	if err != nil || got.ID != "sub-1" || got.Status != "webhook_callback_verification_pending" {
		t.Fatalf("Subscribe = %+v, %v; want sub-1 pending", got, err)
	}
	body, _ := json.Marshal(asked)
	for _, want := range []string{`"type":"stream.online"`, `"version":"1"`, `"broadcaster_user_id":"100000001"`, `"method":"webhook"`,
		`"callback":"https://api.example/v1/twitch/eventsub"`, `"secret":"a-secret-of-length"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("asked %s, want it to hold %s", body, want)
		}
	}
}

func TestAnErrorNamesNoSecret(t *testing.T) {
	c, _ := fakeTwitch(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", http.StatusInternalServerError) })
	_, err := c.Subscribe(context.Background(), "stream.online", "100000001", "https://api.example/cb", "a-secret-of-length")
	if err == nil || strings.Contains(err.Error(), "a-secret-of-length") || strings.Contains(err.Error(), "client-secret") || strings.Contains(err.Error(), "token-") {
		t.Errorf("Subscribe's error = %v, want one naming no secret", err)
	}
	if s := c.String(); strings.Contains(s, "client-secret") {
		t.Errorf("String() = %q, want the secret redacted", s)
	}
}

// signed is Twitch's signature of a message with id, timestamp and body, by secret.
func signed(secret, id, timestamp, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(id + timestamp + body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyTakesOnlyFreshMessagesSignedWithOurSecret(t *testing.T) {
	const secret, id, body = "a-secret-of-length", "msg-1", `{"event":{}}`
	fresh := testNow.Add(-time.Minute).Format(time.RFC3339)
	tests := []struct {
		name, timestamp, signature string
		want                       bool
	}{
		{"signed and fresh", fresh, signed(secret, id, fresh, body), true},
		{"signed by another secret", fresh, signed("another-secret-x", id, fresh, body), false},
		{"a signature without its prefix", fresh, strings.TrimPrefix(signed(secret, id, fresh, body), "sha256="), false},
		{"no signature", fresh, "", false},
		{"older than 10 minutes", testNow.Add(-11 * time.Minute).Format(time.RFC3339), "", false},
		{"from the future", testNow.Add(11 * time.Minute).Format(time.RFC3339), "", false},
		{"no timestamp", "", signed(secret, id, "", body), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig := tt.signature
			if sig == "" && tt.timestamp != "" && tt.name != "no signature" {
				sig = signed(secret, id, tt.timestamp, body)
			}
			if got := Verify([]byte(secret), id, tt.timestamp, sig, []byte(body), testNow); got != tt.want {
				t.Errorf("Verify = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestParseReadsEachKindOfMessage(t *testing.T) {
	tests := []struct {
		name, kind, body string
		want             Message
	}{
		{"a stream going live", "notification",
			`{"subscription":{"id":"sub-1","type":"stream.online","status":"enabled"},"event":{"id":"200000001","broadcaster_user_id":"100000001","broadcaster_user_login":"some_streamer","type":"live","started_at":"2026-11-18T07:59:00Z"}}`,
			Message{Type: "notification", SubscriptionID: "sub-1", SubscriptionType: "stream.online", Status: "enabled", StreamID: "200000001", UserID: "100000001", StartedAt: testNow.Add(-time.Minute)}},
		{"a stream ending", "notification",
			`{"subscription":{"id":"sub-2","type":"stream.offline","status":"enabled"},"event":{"broadcaster_user_id":"100000001"}}`,
			Message{Type: "notification", SubscriptionID: "sub-2", SubscriptionType: "stream.offline", Status: "enabled", UserID: "100000001"}},
		{"Twitch checking our callback", "webhook_callback_verification",
			`{"challenge":"pogchamp-kappa-360noscope-vohiyo","subscription":{"id":"sub-1","type":"stream.online","status":"webhook_callback_verification_pending"}}`,
			Message{Type: "webhook_callback_verification", Challenge: "pogchamp-kappa-360noscope-vohiyo", SubscriptionID: "sub-1", SubscriptionType: "stream.online", Status: "webhook_callback_verification_pending"}},
		{"a subscription revoked", "revocation",
			`{"subscription":{"id":"sub-1","type":"stream.online","status":"user_removed"}}`,
			Message{Type: "revocation", SubscriptionID: "sub-1", SubscriptionType: "stream.online", Status: "user_removed"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.kind, []byte(tt.body))
			if err != nil || got != tt.want {
				t.Errorf("Parse = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func TestParseRefusesWhatIsNotAMessage(t *testing.T) {
	tests := []struct{ name, kind, body string }{
		{"not JSON", "notification", "nope"},
		{"an unknown kind", "pizza", `{"subscription":{"id":"sub-1"}}`},
		{"no subscription", "notification", `{"event":{}}`},
		{"a challenge without one", "webhook_callback_verification", `{"subscription":{"id":"sub-1"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(tt.kind, []byte(tt.body)); err == nil {
				t.Errorf("Parse(%s): no error, want one", tt.body)
			}
		})
	}
}
