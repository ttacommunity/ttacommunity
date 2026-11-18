// Package twitch talks to Twitch: an app access token, Helix's users and streams, EventSub subscriptions, and
// checking and reading the messages EventSub pushes to our callback.
package twitch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Twitch's addresses: Helix, and its token endpoint.
const (
	HelixAPI = "https://api.twitch.tv/helix"
	AuthAPI  = "https://id.twitch.tv"
)

// ErrNoUser means Twitch has no user with that login.
var ErrNoUser = errors.New("no Twitch user has that name")

// Client calls Helix with an app access token it gets, renews when it expires or Twitch refuses it.
type Client struct {
	hc               *http.Client
	api, auth        string
	clientID, secret string
	now              func() time.Time
	mu               sync.Mutex
	token            string
	expires          time.Time
}

// New returns a Client for Helix at api and the token endpoint at auth, as app clientID with secret; now is the clock.
func New(hc *http.Client, api, auth, clientID, secret string, now func() time.Time) *Client {
	return &Client{hc: hc, api: strings.TrimRight(api, "/"), auth: strings.TrimRight(auth, "/"), clientID: clientID, secret: secret, now: now}
}

// String names the client without its secret or token.
func (c *Client) String() string { return "twitch.Client{secret: redacted}" }

// GoString names the client without its secret or token.
func (c *Client) GoString() string { return c.String() }

// User is a Twitch user: their id, login and display name.
type User struct {
	ID, Login, DisplayName string
}

// Stream is a live stream: its id, title, category, preview picture at 1280×720, and when it started.
type Stream struct {
	ID, Title, Category, ThumbnailURL string
	StartedAt                         time.Time
}

// Subscription is an EventSub subscription: its id and status.
type Subscription struct {
	ID, Status string
}

// User returns the user with login, or ErrNoUser.
func (c *Client) User(ctx context.Context, login string) (User, error) {
	var resp struct {
		Data []struct {
			ID          string `json:"id"`
			Login       string `json:"login"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/users?"+url.Values{"login": {login}}.Encode(), nil, &resp); err != nil {
		return User{}, fmt.Errorf("finding a Twitch user: %w", err)
	}
	if len(resp.Data) == 0 {
		return User{}, ErrNoUser
	}
	u := resp.Data[0]
	return User{ID: u.ID, Login: u.Login, DisplayName: u.DisplayName}, nil
}

// Stream returns userID's live stream, and false when they are not live.
func (c *Client) Stream(ctx context.Context, userID string) (Stream, bool, error) {
	var resp struct {
		Data []struct {
			ID        string    `json:"id"`
			Title     string    `json:"title"`
			GameName  string    `json:"game_name"`
			Thumbnail string    `json:"thumbnail_url"`
			StartedAt time.Time `json:"started_at"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/streams?"+url.Values{"user_id": {userID}}.Encode(), nil, &resp); err != nil {
		return Stream{}, false, fmt.Errorf("reading a Twitch stream: %w", err)
	}
	if len(resp.Data) == 0 {
		return Stream{}, false, nil
	}
	s := resp.Data[0]
	thumb := strings.NewReplacer("{width}", "1280", "{height}", "720").Replace(s.Thumbnail)
	return Stream{ID: s.ID, Title: s.Title, Category: s.GameName, ThumbnailURL: thumb, StartedAt: s.StartedAt}, true, nil
}

// Subscribe asks EventSub to push events of typ (stream.online, stream.offline) for userID to callback, signed
// with secret, and returns the subscription Twitch made.
func (c *Client) Subscribe(ctx context.Context, typ, userID, callback, secret string) (Subscription, error) {
	body := map[string]any{
		"type": typ, "version": "1",
		"condition": map[string]string{"broadcaster_user_id": userID},
		"transport": map[string]string{"method": "webhook", "callback": callback, "secret": secret},
	}
	var resp struct {
		Data []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodPost, "/eventsub/subscriptions", body, &resp); err != nil {
		return Subscription{}, fmt.Errorf("subscribing to %s: %w", typ, err)
	}
	if len(resp.Data) == 0 {
		return Subscription{}, fmt.Errorf("subscribing to %s: Twitch answered no subscription", typ)
	}
	return Subscription{ID: resp.Data[0].ID, Status: resp.Data[0].Status}, nil
}

// Unsubscribe deletes the EventSub subscription with id; one Twitch no longer has is no error.
func (c *Client) Unsubscribe(ctx context.Context, id string) error {
	err := c.do(ctx, http.MethodDelete, "/eventsub/subscriptions?"+url.Values{"id": {id}}.Encode(), nil, nil)
	var status *StatusError
	if errors.As(err, &status) && status.Status == http.StatusNotFound {
		return nil
	}
	if err != nil {
		return fmt.Errorf("unsubscribing: %w", err)
	}
	return nil
}

// StatusError means Twitch answered a request with a status other than success.
type StatusError struct{ Status int }

func (e *StatusError) Error() string { return fmt.Sprintf("Twitch answered %d", e.Status) }

// do sends a Helix request, its body as JSON, decoding the answer into out (nil to ignore it); a token Twitch
// refuses is renewed and the request sent once more.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	err := c.once(ctx, method, path, body, out)
	var status *StatusError
	if errors.As(err, &status) && status.Status == http.StatusUnauthorized {
		c.mu.Lock()
		c.token = ""
		c.mu.Unlock()
		err = c.once(ctx, method, path, body, out)
	}
	return err
}

// once sends a Helix request once.
func (c *Client) once(ctx context.Context, method, path string, body, out any) error {
	token, err := c.appToken(ctx)
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.api+path, reader)
	if err != nil {
		return errors.New("not a Twitch address")
	}
	req.Header.Set("Client-Id", c.clientID)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.send(req, out)
}

// appToken returns the app access token, asking Twitch for one when there is none or it expires within a minute.
func (c *Client) appToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.now().Before(c.expires.Add(-time.Minute)) {
		return c.token, nil
	}
	form := url.Values{"client_id": {c.clientID}, "client_secret": {c.secret}, "grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.auth+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", errors.New("not a Twitch address")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var resp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := c.send(req, &resp); err != nil {
		return "", fmt.Errorf("getting a Twitch app token: %w", err)
	}
	if resp.AccessToken == "" {
		return "", errors.New("getting a Twitch app token: Twitch answered none")
	}
	c.token, c.expires = resp.AccessToken, c.now().Add(time.Duration(resp.ExpiresIn)*time.Second)
	return c.token, nil
}

// send sends req and decodes a successful answer into out (nil to ignore it). Its errors name no address, which
// can hold the client secret, and no token.
func (c *Client) send(req *http.Request, out any) error {
	resp, err := c.hc.Do(req)
	if err != nil {
		var u *url.Error
		if errors.As(err, &u) {
			err = u.Err
		}
		return fmt.Errorf("Twitch could not be reached: %s", strings.ReplaceAll(err.Error(), req.URL.Host, "Twitch"))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &StatusError{Status: resp.StatusCode}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return fmt.Errorf("reading Twitch's answer: %w", err)
	}
	return nil
}
