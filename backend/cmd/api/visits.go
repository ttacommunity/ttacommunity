// Counting the site's visits: Cloudflare Web Analytics, asked every hour in the background.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"ttacommunity/backend/internal/httpapi"
	"ttacommunity/backend/internal/webanalytics"
)

// How often the visits are counted, and how soon a failed count is tried again.
const (
	visitsEvery = time.Hour
	visitsRetry = 5 * time.Minute
)

// visitsConfig reads getenv's CLOUDFLARE_ANALYTICS_TOKEN and CLOUDFLARE_ACCOUNT_ID, to count visits to site's
// hostname. With neither set, counting is off; with one alone, or no site, it is an error.
func visitsConfig(getenv func(string) string, site string) (webanalytics.Config, bool, error) {
	token, account := getenv("CLOUDFLARE_ANALYTICS_TOKEN"), getenv("CLOUDFLARE_ACCOUNT_ID")
	if token == "" && account == "" {
		return webanalytics.Config{}, false, nil
	}
	if token == "" {
		return webanalytics.Config{}, false, errors.New("CLOUDFLARE_ANALYTICS_TOKEN is not set, and CLOUDFLARE_ACCOUNT_ID is")
	}
	if account == "" {
		return webanalytics.Config{}, false, errors.New("CLOUDFLARE_ACCOUNT_ID is not set, and CLOUDFLARE_ANALYTICS_TOKEN is")
	}
	u, err := url.Parse(site)
	if err != nil || u.Hostname() == "" {
		return webanalytics.Config{}, false, errors.New("SITE_ORIGIN must be set to count the site's visits")
	}
	return webanalytics.Config{Token: token, Account: account, Host: u.Hostname()}, true, nil
}

// openVisits returns the site's visit counter, counting in the background until ctx ends, or nil when counting
// is off. Half a config is an error.
func openVisits(ctx context.Context, getenv func(string) string, site string, log *slog.Logger) (httpapi.Visits, error) {
	cfg, on, err := visitsConfig(getenv, site)
	if err != nil || !on {
		return nil, err
	}
	client, err := webanalytics.New(cfg, &http.Client{Timeout: 10 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("CLOUDFLARE_ANALYTICS_TOKEN and CLOUDFLARE_ACCOUNT_ID: %w", err)
	}
	counter := webanalytics.NewCounter(client, time.Now)
	go countVisits(ctx, counter, log)
	log.Info("counting the site's visits", "host", cfg.Host)
	return counter, nil
}

// countVisits refreshes counter at once, then every visitsEvery, or visitsRetry after a failure, until ctx ends.
func countVisits(ctx context.Context, counter *webanalytics.Counter, log *slog.Logger) {
	for {
		wait := visitsEvery
		if err := counter.Refresh(ctx); err != nil && ctx.Err() == nil {
			log.Warn("counting the site's visits failed", "err", err)
			wait = visitsRetry
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}
