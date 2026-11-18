package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
	"ttacommunity/backend/internal/twitch"
)

var twitchSecret = []byte("invented-eventsub-secret")

// twitchNow is when the test clock reads; messages are stamped with it.
const twitchNow = "2026-10-11T12:00:00Z"

// fakeTwitchHub knows one live stream, records what the callback asks of it, and fails a lookup with streamErr.
type fakeTwitchHub struct {
	calls     []string
	streamErr error
}

func (f *fakeTwitchHub) Secret() []byte { return twitchSecret }

func (f *fakeTwitchHub) Stream(_ context.Context, userID string) (twitch.Stream, bool, error) {
	if f.streamErr != nil {
		return twitch.Stream{}, false, f.streamErr
	}
	return twitch.Stream{ID: "200000001", Title: "Friday games", Category: "Through the Ages", ThumbnailURL: "https://static-cdn.jtvnw.net/x.jpg"}, userID == "100000001", nil
}

func (f *fakeTwitchHub) TwitchStreamOnline(_ context.Context, message string, s store.TwitchStream, _ time.Time) error {
	f.calls = append(f.calls, "online "+message+" "+s.UserID+" "+s.StreamID+" "+s.Title+" "+s.Category)
	return nil
}

func (f *fakeTwitchHub) TwitchStreamOffline(_ context.Context, message, userID string, _ time.Time) error {
	f.calls = append(f.calls, "offline "+message+" "+userID)
	return nil
}

func (f *fakeTwitchHub) TwitchSubscriptionStatus(_ context.Context, id, status string, _ time.Time) error {
	f.calls = append(f.calls, "status "+id+" "+status)
	return nil
}

// twitchPush sends hub an EventSub message of kind with body, signed with secret (unsigned when nil) and stamped at.
func twitchPush(hub TwitchHub, kind, body string, secret []byte, at string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/twitch/eventsub", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(twitch.HeaderID, "msg-1")
	req.Header.Set(twitch.HeaderType, kind)
	req.Header.Set(twitch.HeaderTimestamp, at)
	if secret != nil {
		mac := hmac.New(sha256.New, secret)
		mac.Write([]byte("msg-1" + at + body))
		req.Header.Set(twitch.HeaderSignature, "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	rec := httptest.NewRecorder()
	NewMux(&fakeRatings{}, nil, nil, nil, nil, hub, nil, slog.New(slog.DiscardHandler), &recordedRequests{}, stepClock(time.Millisecond)).ServeHTTP(rec, req)
	return rec
}

const (
	onlineBody  = `{"subscription":{"id":"sub-1","type":"stream.online","status":"enabled"},"event":{"id":"200000001","broadcaster_user_id":"100000001","started_at":"2026-10-11T11:59:00Z"}}`
	offlineBody = `{"subscription":{"id":"sub-2","type":"stream.offline","status":"enabled"},"event":{"broadcaster_user_id":"100000001"}}`
)

func TestTheTwitchCallbackTakesEachKindOfMessage(t *testing.T) {
	tests := []struct {
		name, kind, body string
		wantStatus       int
		wantBody         string
		wantCalls        []string
	}{
		{"a stream going live, looked up for its title", "notification", onlineBody, http.StatusNoContent, "",
			[]string{"online msg-1 100000001 200000001 Friday games Through the Ages"}},
		{"a stream ending", "notification", offlineBody, http.StatusNoContent, "", []string{"offline msg-1 100000001"}},
		{"Twitch checking our callback", "webhook_callback_verification",
			`{"challenge":"a-challenge","subscription":{"id":"sub-1","type":"stream.online","status":"webhook_callback_verification_pending"}}`,
			http.StatusOK, "a-challenge", []string{"status sub-1 enabled"}},
		{"a subscription revoked", "revocation", `{"subscription":{"id":"sub-1","type":"stream.online","status":"user_removed"}}`,
			http.StatusNoContent, "", []string{"status sub-1 user_removed"}},
		{"another subscription type", "notification", `{"subscription":{"id":"sub-3","type":"channel.follow","status":"enabled"},"event":{}}`,
			http.StatusNoContent, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := &fakeTwitchHub{}
			rec := twitchPush(hub, tt.kind, tt.body, twitchSecret, twitchNow)
			if rec.Code != tt.wantStatus || rec.Body.String() != tt.wantBody && tt.wantBody != "" {
				t.Errorf("answered %d %q, want %d %q", rec.Code, rec.Body, tt.wantStatus, tt.wantBody)
			}
			if !slices.Equal(hub.calls, tt.wantCalls) {
				t.Errorf("asked %q, want %q", hub.calls, tt.wantCalls)
			}
		})
	}
}

func TestTheTwitchCallbackRefusesWhatTwitchDidNotSend(t *testing.T) {
	tests := []struct {
		name   string
		secret []byte
		at     string
		hub    bool
		want   int
	}{
		{"unsigned", nil, twitchNow, true, http.StatusForbidden},
		{"signed with another secret", []byte("another-secret-xyz"), twitchNow, true, http.StatusForbidden},
		{"an old message", twitchSecret, "2026-10-11T11:45:00Z", true, http.StatusForbidden},
		{"Twitch not set up here", twitchSecret, twitchNow, false, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := &fakeTwitchHub{}
			var h TwitchHub
			if tt.hub {
				h = hub
			}
			if rec := twitchPush(h, "notification", onlineBody, tt.secret, tt.at); rec.Code != tt.want {
				t.Errorf("answered %d, want %d", rec.Code, tt.want)
			}
			if len(hub.calls) != 0 {
				t.Errorf("asked %q, want nothing stored", hub.calls)
			}
		})
	}
}

func TestAStreamTwitchCannotDescribeYetIsLeftForTwitchToSendAgain(t *testing.T) {
	tests := []struct {
		name string
		hub  *fakeTwitchHub
		body string
	}{
		{"Twitch unreachable", &fakeTwitchHub{streamErr: errors.New("down")}, onlineBody},
		{"not live yet on Helix", &fakeTwitchHub{}, strings.Replace(onlineBody, "100000001", "100000009", 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := twitchPush(tt.hub, "notification", tt.body, twitchSecret, twitchNow); rec.Code < 500 {
				t.Errorf("answered %d, want a 5xx so Twitch sends it again", rec.Code)
			}
			if len(tt.hub.calls) != 0 {
				t.Errorf("asked %q, want nothing stored", tt.hub.calls)
			}
		})
	}
}

func TestATwitchMessageWithoutItsTypeIsRefused(t *testing.T) {
	hub := &fakeTwitchHub{}
	if rec := twitchPush(hub, "", onlineBody, twitchSecret, twitchNow); rec.Code != http.StatusBadRequest || len(hub.calls) != 0 {
		t.Errorf("answered %d, asked %q; want 400 and nothing stored", rec.Code, hub.calls)
	}
}
