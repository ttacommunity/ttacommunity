// Package resend sends an email through Resend's API, from one sender address.
package resend

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

// api is Resend's, used when Config leaves URL empty.
const api = "https://api.resend.com"

// Config is the API key and the sender every message comes from, such as "TTA Community <noreply@example.org>".
// URL is Resend's API, or a stub on 127.0.0.1.
type Config struct {
	APIKey, From, URL string
}

// String keeps the key out of logs.
func (c Config) String() string {
	return fmt.Sprintf("resend.Config{From: %s}", c.From)
}

// Message is one email to one address, as plain text and, when HTML is set, HTML. IdempotencyKey, when set, makes
// a retry send once.
type Message struct {
	To, Subject, Text, HTML string
	IdempotencyKey          string
}

// Client sends messages.
type Client struct {
	cfg  Config
	http *http.Client
}

// New returns a client for cfg, calling Resend through hc. It refuses a config without a key or a sender, or an
// API URL that is neither Resend's nor on this machine.
func New(cfg Config, hc *http.Client) (*Client, error) {
	if cfg.APIKey == "" || cfg.From == "" {
		return nil, errors.New("resend: the API key and the sender are both needed")
	}
	if cfg.URL == "" {
		cfg.URL = api
	}
	if cfg.URL != api && !strings.HasPrefix(cfg.URL, "http://127.0.0.1:") {
		return nil, errors.New("resend: the API must be Resend's, or a stub on 127.0.0.1")
	}
	return &Client{cfg: cfg, http: hc}, nil
}

// Send sends m, returning an error when Resend does not accept it.
func (c *Client) Send(ctx context.Context, m Message) error {
	msg := map[string]any{"from": c.cfg.From, "to": []string{m.To}, "subject": m.Subject, "text": m.Text}
	if m.HTML != "" {
		msg["html"] = m.HTML
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("resend: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.URL+"/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("resend: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	if m.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", m.IdempotencyKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("resend: sending: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("resend: sending: status %d", resp.StatusCode)
	}
	return nil
}
