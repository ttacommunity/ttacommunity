// Package webanalytics counts the site's visits in Cloudflare Web Analytics, through Cloudflare's GraphQL API.
package webanalytics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// endpoint is Cloudflare's GraphQL API, used when Config leaves URL empty.
const endpoint = "https://api.cloudflare.com/client/v4/graphql"

// query sums the visits people, not bots, made to one host. With no dimensions it answers one group, or none.
const query = `query Visits($account: string!, $host: string!, $from: Time!, $to: Time!) {
  viewer {
    accounts(filter: {accountTag: $account}) {
      rumPageloadEventsAdaptiveGroups(limit: 1, filter: {requestHost: $host, bot: 0, datetime_geq: $from, datetime_lt: $to}) {
        sum { visits }
      }
    }
  }
}`

// Config is a token that can read the account's analytics, the account, and the site's hostname. URL is
// Cloudflare's GraphQL API, or a stub on 127.0.0.1.
type Config struct {
	Token, Account, Host, URL string
}

// String keeps the token out of logs.
func (c Config) String() string {
	return fmt.Sprintf("webanalytics.Config{Account: %s, Host: %s}", c.Account, c.Host)
}

// Client asks Cloudflare for visit counts.
type Client struct {
	cfg  Config
	http *http.Client
}

// New returns a client for cfg, calling Cloudflare through hc. It refuses a config missing any of token, account
// and host, or an API URL that is neither Cloudflare's nor on this machine.
func New(cfg Config, hc *http.Client) (*Client, error) {
	if cfg.Token == "" || cfg.Account == "" || cfg.Host == "" {
		return nil, errors.New("webanalytics: the token, account and host are needed")
	}
	if cfg.URL == "" {
		cfg.URL = endpoint
	}
	if cfg.URL != endpoint && !strings.HasPrefix(cfg.URL, "http://127.0.0.1:") {
		return nil, errors.New("webanalytics: the API must be Cloudflare's, or a stub on 127.0.0.1")
	}
	return &Client{cfg: cfg, http: hc}, nil
}

// Visits returns the visits people, not bots, made to the configured host from from up to to.
func (c *Client) Visits(ctx context.Context, from, to time.Time) (int, error) {
	body, err := json.Marshal(map[string]any{"query": query, "variables": map[string]string{
		"account": c.cfg.Account, "host": c.cfg.Host,
		"from": from.UTC().Format(time.RFC3339), "to": to.UTC().Format(time.RFC3339),
	}})
	if err != nil {
		return 0, fmt.Errorf("webanalytics: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("webanalytics: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("webanalytics: asking Cloudflare: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("webanalytics: Cloudflare answered %d", resp.StatusCode)
	}
	var got struct {
		Data struct {
			Viewer struct {
				Accounts []struct {
					Groups []struct {
						Sum struct {
							Visits int `json:"visits"`
						} `json:"sum"`
					} `json:"rumPageloadEventsAdaptiveGroups"`
				} `json:"accounts"`
			} `json:"viewer"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&got); err != nil {
		return 0, fmt.Errorf("webanalytics: reading Cloudflare's answer: %w", err)
	}
	if len(got.Errors) > 0 {
		return 0, fmt.Errorf("webanalytics: Cloudflare answered: %s", got.Errors[0].Message)
	}
	if len(got.Data.Viewer.Accounts) != 1 {
		return 0, errors.New("webanalytics: the token cannot read the account")
	}
	visits := 0
	for _, g := range got.Data.Viewer.Accounts[0].Groups {
		visits += g.Sum.Visits
	}
	return visits, nil
}

// Days is how many days a count covers.
const Days = 28

// shownFor is how long a count is shown after it was taken. Past that, refreshing has failed for a day.
const shownFor = 24 * time.Hour

// Count is the visits in the Days days up to AsOf.
type Count struct {
	Visits, Days int
	AsOf         time.Time
}

// Source counts the site's visits from from up to to: a Client, or a stand-in.
type Source interface {
	Visits(ctx context.Context, from, to time.Time) (int, error)
}

// Counter keeps the latest count from a source, for any number of readers.
type Counter struct {
	source Source
	now    func() time.Time
	mu     sync.Mutex
	last   Count
}

// NewCounter returns a counter asking source, timed by now. It has no count until its first Refresh.
func NewCounter(source Source, now func() time.Time) *Counter {
	return &Counter{source: source, now: now}
}

// Refresh counts the visits in the Days days up to now. On an error the last count is kept.
func (c *Counter) Refresh(ctx context.Context) error {
	to := c.now()
	visits, err := c.source.Visits(ctx, to.AddDate(0, 0, -Days), to)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.last = Count{Visits: visits, Days: Days, AsOf: to}
	return nil
}

// Latest returns the last count, and false when there is none from the last day.
func (c *Counter) Latest() (Count, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.last.AsOf.IsZero() || c.now().Sub(c.last.AsOf) > shownFor {
		return Count{}, false
	}
	return c.last, true
}
