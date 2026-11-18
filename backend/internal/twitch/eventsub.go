// Checking and reading the messages EventSub pushes to our callback.
package twitch

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// EventSub's message headers.
const (
	HeaderID        = "Twitch-Eventsub-Message-Id"
	HeaderTimestamp = "Twitch-Eventsub-Message-Timestamp"
	HeaderSignature = "Twitch-Eventsub-Message-Signature"
	HeaderType      = "Twitch-Eventsub-Message-Type"
)

// CallbackPath is the api's callback EventSub pushes to.
const CallbackPath = "/v1/twitch/eventsub"

// MinSecret and MaxSecret bound the secret our subscriptions are signed with, in characters, as Twitch does.
const (
	MinSecret = 10
	MaxSecret = 100
)

// maxAge is how far a message's timestamp may be from now before it is refused, so an old one cannot be replayed.
const maxAge = 10 * time.Minute

// Verify says whether a message with id, timestamp and body carries signature, the HMAC-SHA256 of the three by
// secret as Twitch writes it (sha256=<hex>), and was sent within maxAge of now.
func Verify(secret []byte, id, timestamp, signature string, body []byte, now time.Time) bool {
	at, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil || at.Before(now.Add(-maxAge)) || at.After(now.Add(maxAge)) {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(id))
	mac.Write([]byte(timestamp))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(want))
}

// Message is what an EventSub message tells: its type (notification, webhook_callback_verification or revocation),
// the challenge to echo for a verification, its subscription's id, type and status, and for a stream event, the
// stream's id, the streamer's user id and when it started.
type Message struct {
	Type                                     string
	Challenge                                string
	SubscriptionID, SubscriptionType, Status string
	StreamID, UserID                         string
	StartedAt                                time.Time
}

// Parse reads body, a message of type kind (the Twitch-Eventsub-Message-Type header).
func Parse(kind string, body []byte) (Message, error) {
	var raw struct {
		Challenge    string `json:"challenge"`
		Subscription *struct {
			ID     string `json:"id"`
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"subscription"`
		Event struct {
			ID        string    `json:"id"`
			UserID    string    `json:"broadcaster_user_id"`
			StartedAt time.Time `json:"started_at"`
		} `json:"event"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Message{}, fmt.Errorf("reading an EventSub message: %w", err)
	}
	switch {
	case kind != "notification" && kind != "webhook_callback_verification" && kind != "revocation":
		return Message{}, fmt.Errorf("an EventSub message of unknown type %q", kind)
	case raw.Subscription == nil || raw.Subscription.ID == "":
		return Message{}, errors.New("an EventSub message without its subscription")
	case kind == "webhook_callback_verification" && raw.Challenge == "":
		return Message{}, errors.New("an EventSub verification without its challenge")
	}
	return Message{Type: kind, Challenge: raw.Challenge, SubscriptionID: raw.Subscription.ID, SubscriptionType: raw.Subscription.Type,
		Status: raw.Subscription.Status, StreamID: raw.Event.ID, UserID: raw.Event.UserID, StartedAt: raw.Event.StartedAt}, nil
}
