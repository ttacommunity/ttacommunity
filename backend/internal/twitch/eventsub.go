// Checking the messages EventSub pushes to our callback.
package twitch

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
