// The Twitch wiring: the app's client and EventSub secret from config, admin's side of Twitch, and the callback's
// dependencies.
package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ttacommunity/backend/internal/httpapi"
	"ttacommunity/backend/internal/store"
	"ttacommunity/backend/internal/twitch"
)

// twitchSetup is Twitch as configured here: the client, the callback EventSub pushes to and the secret it signs
// with, and the callback's dependencies. All are zero when Twitch is not set up.
type twitchSetup struct {
	client   *twitch.Client
	callback string
	secret   string
	hub      *twitchHubDeps
}

// String names the setup without its secret.
func (t twitchSetup) String() string {
	return "twitchSetup{callback: " + t.callback + ", secret: redacted}"
}

// GoString names the setup without its secret.
func (t twitchSetup) GoString() string { return t.String() }

// openTwitch builds Twitch from TWITCH_CLIENT_ID, TWITCH_CLIENT_SECRET, TWITCH_EVENTSUB_SECRET and API_ORIGIN, the
// callback's host, storing through admins. With none of the three set, Twitch is off; with some but not all, a
// secret of a length Twitch refuses, or a callback neither https nor local, it refuses to start. Twitch itself only
// pushes to https.
func openTwitch(getenv func(string) string, admins *store.AdminStore) (twitchSetup, error) {
	id, secret, events := getenv("TWITCH_CLIENT_ID"), getenv("TWITCH_CLIENT_SECRET"), getenv("TWITCH_EVENTSUB_SECRET")
	if id == "" && secret == "" && events == "" {
		return twitchSetup{}, nil
	}
	for name, v := range map[string]string{"TWITCH_CLIENT_ID": id, "TWITCH_CLIENT_SECRET": secret, "TWITCH_EVENTSUB_SECRET": events} {
		if v == "" {
			return twitchSetup{}, fmt.Errorf("%s is not set, but other Twitch settings are", name)
		}
	}
	if n := len(events); n < twitch.MinSecret || n > twitch.MaxSecret {
		return twitchSetup{}, fmt.Errorf("TWITCH_EVENTSUB_SECRET must be %d to %d characters", twitch.MinSecret, twitch.MaxSecret)
	}
	origin := getenv("API_ORIGIN")
	if !strings.HasPrefix(origin, "https://") && !strings.HasPrefix(origin, "http://127.0.0.1:") {
		return twitchSetup{}, fmt.Errorf("API_ORIGIN must be https, or http://127.0.0.1:port locally, for Twitch's callback; it is %q", origin)
	}
	client := twitch.New(&http.Client{Timeout: 10 * time.Second}, twitch.HelixAPI, twitch.AuthAPI, id, secret, time.Now)
	return twitchSetup{client: client, callback: strings.TrimRight(origin, "/") + twitch.CallbackPath, secret: events,
		hub: &twitchHubDeps{AdminStore: admins, client: client, secret: []byte(events)}}, nil
}

// twitchHubDeps is the callback's dependencies: the admin login stores what Twitch pushes, the client describes a
// stream going live.
type twitchHubDeps struct {
	*store.AdminStore
	client *twitch.Client
	secret []byte
}

// Secret is the secret EventSub signs our messages with.
func (d *twitchHubDeps) Secret() []byte { return d.secret }

// String names the dependencies without the secret.
func (d *twitchHubDeps) String() string { return "twitchHubDeps{secret: redacted}" }

// GoString names the dependencies without the secret.
func (d *twitchHubDeps) GoString() string { return d.String() }

// Stream returns userID's live stream from Twitch, and false when Twitch does not show it live.
func (d *twitchHubDeps) Stream(ctx context.Context, userID string) (twitch.Stream, bool, error) {
	return d.client.Stream(ctx, userID)
}

// TwitchUser looks a streamer up on Twitch, or returns httpapi.ErrTwitchNotSetUp.
func (d adminDeps) TwitchUser(ctx context.Context, login string) (twitch.User, error) {
	if d.twitch.client == nil {
		return twitch.User{}, httpapi.ErrTwitchNotSetUp
	}
	return d.twitch.client.User(ctx, login)
}

// SubscribeTwitch asks EventSub to push userID's events of typ to our callback, or returns httpapi.ErrTwitchNotSetUp.
func (d adminDeps) SubscribeTwitch(ctx context.Context, typ, userID string) (twitch.Subscription, error) {
	if d.twitch.client == nil {
		return twitch.Subscription{}, httpapi.ErrTwitchNotSetUp
	}
	return d.twitch.client.Subscribe(ctx, typ, userID, d.twitch.callback, d.twitch.secret)
}

// UnsubscribeTwitch deletes an EventSub subscription, or returns httpapi.ErrTwitchNotSetUp.
func (d adminDeps) UnsubscribeTwitch(ctx context.Context, id string) error {
	if d.twitch.client == nil {
		return httpapi.ErrTwitchNotSetUp
	}
	return d.twitch.client.Unsubscribe(ctx, id)
}
