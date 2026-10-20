// Package discord signs a player in with their Discord account: the authorization code flow with state and PKCE,
// asking only for the identify scope, then reading who they are. The access token is used once and not kept.
package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/oauth2"
)

// Discord's own addresses, used when Config leaves them empty.
const (
	discordAuthURL = "https://discord.com/oauth2/authorize"
	discordAPIURL  = "https://discord.com/api"
)

// Config is the Discord application players sign in through, and where Discord sends them back.
type Config struct {
	ClientID, ClientSecret string
	// RedirectURL is the api's callback, as registered with the application: https, or http on 127.0.0.1.
	RedirectURL string
	// AuthURL and APIURL are Discord's when empty; a stub on 127.0.0.1 may stand in for local development.
	AuthURL, APIURL string
}

// String keeps the client secret out of logs.
func (c Config) String() string {
	return fmt.Sprintf("discord.Config{ClientID: %s, RedirectURL: %s}", c.ClientID, c.RedirectURL)
}

// User is a Discord account as the identify scope shows it: the id, the unique username, the display name if set.
type User struct {
	ID, Username, DisplayName string
}

// Client starts sign-ins and finishes them.
type Client struct {
	oauth oauth2.Config
	api   string
	http  *http.Client
}

var discordID = regexp.MustCompile(`^[0-9]{17,20}$`)

// New returns a client for cfg, calling Discord through hc. It refuses a config without a client id, secret or
// redirect URL, a plain-http redirect URL off this machine, or Discord's URLs pointing anywhere but Discord or
// this machine.
func New(cfg Config, hc *http.Client) (*Client, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("discord: the client id and secret are both needed")
	}
	if !strings.HasPrefix(cfg.RedirectURL, "https://") && !strings.HasPrefix(cfg.RedirectURL, "http://127.0.0.1:") {
		return nil, errors.New("discord: the redirect URL must be https, or http on 127.0.0.1")
	}
	if cfg.AuthURL == "" {
		cfg.AuthURL = discordAuthURL
	}
	if cfg.APIURL == "" {
		cfg.APIURL = discordAPIURL
	}
	for _, u := range []string{cfg.AuthURL, cfg.APIURL} {
		if !strings.HasPrefix(u, "https://discord.com/") && !strings.HasPrefix(u, "http://127.0.0.1:") {
			return nil, errors.New("discord: its URLs must be Discord's, or a stub on 127.0.0.1")
		}
	}
	return &Client{
		oauth: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       []string{"identify"},
			Endpoint: oauth2.Endpoint{
				AuthURL:   cfg.AuthURL,
				TokenURL:  cfg.APIURL + "/oauth2/token",
				AuthStyle: oauth2.AuthStyleInParams,
			},
		},
		api:  cfg.APIURL,
		http: hc,
	}, nil
}

// AuthCodeURL is where to send a player to sign in, carrying state and the S256 challenge of verifier. A player
// who has authorized the app before is not shown Discord's consent screen again.
func (c *Client) AuthCodeURL(state, verifier string) string {
	return c.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("prompt", "none"))
}

// User exchanges the code Discord sent back, with the verifier its challenge came from, and returns the account
// that signed in. An account without a Discord id or a username is an error.
func (c *Client) User(ctx context.Context, code, verifier string) (User, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, c.http)
	tok, err := c.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return User{}, fmt.Errorf("discord: exchanging the code: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.api+"/users/@me", nil)
	if err != nil {
		return User{}, fmt.Errorf("discord: asking who signed in: %w", err)
	}
	tok.SetAuthHeader(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return User{}, fmt.Errorf("discord: asking who signed in: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return User{}, fmt.Errorf("discord: asking who signed in: status %d", resp.StatusCode)
	}
	var me struct {
		ID         string  `json:"id"`
		Username   string  `json:"username"`
		GlobalName *string `json:"global_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&me); err != nil {
		return User{}, fmt.Errorf("discord: reading who signed in: %w", err)
	}
	if !discordID.MatchString(me.ID) || me.Username == "" || utf8.RuneCountInString(me.Username) > 100 {
		return User{}, errors.New("discord: the account has no Discord id or username")
	}
	u := User{ID: me.ID, Username: me.Username}
	if me.GlobalName != nil {
		u.DisplayName = truncate(*me.GlobalName, 100)
	}
	return u, nil
}

// truncate cuts s to at most n characters.
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
