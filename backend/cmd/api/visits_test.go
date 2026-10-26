package main

import (
	"maps"
	"strings"
	"testing"
)

// visitsEnv is a complete config for counting visits, with invented values.
var visitsEnv = map[string]string{
	"CLOUDFLARE_ANALYTICS_TOKEN": "invented-token",
	"CLOUDFLARE_ACCOUNT_ID":      "invented-account",
}

func TestVisitCountingIsOffWithNoneOfItsConfig(t *testing.T) {
	_, on, err := visitsConfig(func(string) string { return "" }, "https://site.example.test")
	if err != nil || on {
		t.Errorf("visitsConfig with nothing set = on %v, %v; want off and no error", on, err)
	}
}

func TestVisitCountingCountsTheSitesHostname(t *testing.T) {
	cfg, on, err := visitsConfig(func(k string) string { return visitsEnv[k] }, "https://site.example.test")
	if err != nil || !on {
		t.Fatalf("visitsConfig with a full config = on %v, %v; want on", on, err)
	}
	if cfg.Token != "invented-token" || cfg.Account != "invented-account" || cfg.Host != "site.example.test" {
		t.Errorf("visitsConfig = %v, token set: %v; want the token, account invented-account and host site.example.test", cfg, cfg.Token != "")
	}
}

func TestVisitCountingRefusesHalfItsConfig(t *testing.T) {
	tests := []struct {
		name, key, value, site, want string
	}{
		{"no token", "CLOUDFLARE_ANALYTICS_TOKEN", "", "https://site.example.test", "CLOUDFLARE_ANALYTICS_TOKEN"},
		{"no account", "CLOUDFLARE_ACCOUNT_ID", "", "https://site.example.test", "CLOUDFLARE_ACCOUNT_ID"},
		{"no site", "", "", "", "SITE_ORIGIN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := maps.Clone(visitsEnv)
			if tt.key != "" {
				env[tt.key] = tt.value
			}
			_, _, err := visitsConfig(func(k string) string { return env[k] }, tt.site)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("visitsConfig with %s = %v, want a refusal naming %s", tt.name, err, tt.want)
			}
		})
	}
}
