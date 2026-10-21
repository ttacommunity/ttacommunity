// Package turnstile checks a Cloudflare Turnstile token with siteverify: solved, on our site, for the form's action.
package turnstile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// siteverify is Cloudflare's, used when Config leaves URL empty.
const siteverify = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// maxToken is the longest token Turnstile issues.
const maxToken = 2048

// ErrFailed means the token was not solved, has expired or been used, or was solved on another site or form.
var ErrFailed = errors.New("turnstile: the check was not passed")

// Config is the widget's secret and the site's hostname. An empty Hostname accepts any hostname and action, as
// Cloudflare's test keys need: they answer with neither. URL is siteverify's, or a stub on 127.0.0.1.
type Config struct {
	Secret, Hostname, URL string
}

// String keeps the secret out of logs.
func (c Config) String() string {
	return fmt.Sprintf("turnstile.Config{Hostname: %s}", c.Hostname)
}

// Client checks tokens.
type Client struct {
	cfg  Config
	http *http.Client
}

// New returns a client for cfg, calling siteverify through hc. It refuses a config without a secret, or a
// siteverify URL that is neither Cloudflare's nor on this machine.
func New(cfg Config, hc *http.Client) (*Client, error) {
	if cfg.Secret == "" {
		return nil, errors.New("turnstile: the secret is needed")
	}
	if cfg.URL == "" {
		cfg.URL = siteverify
	}
	if cfg.URL != siteverify && !strings.HasPrefix(cfg.URL, "http://127.0.0.1:") {
		return nil, errors.New("turnstile: siteverify must be Cloudflare's, or a stub on 127.0.0.1")
	}
	return &Client{cfg: cfg, http: hc}, nil
}

// Verify returns nil when token was solved on the configured hostname for action, ErrFailed when it was not, and
// another error when siteverify could not be asked.
func (c *Client) Verify(ctx context.Context, token, action string) error {
	if token == "" || len(token) > maxToken {
		return ErrFailed
	}
	body, err := json.Marshal(map[string]string{"secret": c.cfg.Secret, "response": token})
	if err != nil {
		return fmt.Errorf("turnstile: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("turnstile: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("turnstile: asking siteverify: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("turnstile: siteverify answered %d", resp.StatusCode)
	}
	var got struct {
		Success  bool   `json:"success"`
		Hostname string `json:"hostname"`
		Action   string `json:"action"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&got); err != nil {
		return fmt.Errorf("turnstile: reading siteverify's answer: %w", err)
	}
	if !got.Success || (c.cfg.Hostname != "" && (got.Hostname != c.cfg.Hostname || got.Action != action)) {
		return ErrFailed
	}
	return nil
}
