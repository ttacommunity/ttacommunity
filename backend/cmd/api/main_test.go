package main

import (
	"context"
	"maps"
	"strings"
	"testing"
)

func TestMigrateRefusesWithoutTheOwnersURL(t *testing.T) {
	err := migrate(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "RATINGS_DATABASE_URL") {
		t.Errorf("migrate without a URL = %v, want a refusal naming RATINGS_DATABASE_URL", err)
	}
}

func TestSignInRefusesToStartWithoutItsConfig(t *testing.T) {
	full := map[string]string{
		"SITE_ORIGIN":                 "https://site.example",
		"API_ORIGIN":                  "https://api.example",
		"RATINGS_PLAYER_DATABASE_URL": "postgres://player@127.0.0.1:1/none",
		"DISCORD_CLIENT_ID":           "100000000000000099",
		"DISCORD_CLIENT_SECRET":       "test-secret",
	}
	tests := []struct {
		name, key, value, want string
	}{
		{"no site origin", "SITE_ORIGIN", "", "SITE_ORIGIN"},
		{"a site origin over plain http", "SITE_ORIGIN", "http://site.example", "SITE_ORIGIN"},
		{"a site origin with a path", "SITE_ORIGIN", "https://site.example/", "SITE_ORIGIN"},
		{"no api origin", "API_ORIGIN", "", "API_ORIGIN"},
		{"no player login", "RATINGS_PLAYER_DATABASE_URL", "", "RATINGS_PLAYER_DATABASE_URL"},
		{"no Discord client id", "DISCORD_CLIENT_ID", "", "DISCORD"},
		{"no Discord client secret", "DISCORD_CLIENT_SECRET", "", "DISCORD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := maps.Clone(full)
			env[tt.key] = tt.value
			_, _, err := openSignIns(context.Background(), func(k string) string { return env[k] })
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("openSignIns with %s=%q = %v, want a refusal naming %s", tt.key, tt.value, err, tt.want)
			}
		})
	}
}

func TestTheDiscordStandInIsOnlyForThisMachine(t *testing.T) {
	local := map[string]string{
		"SITE_ORIGIN":                 "http://127.0.0.1:4321",
		"API_ORIGIN":                  "http://127.0.0.1:8080",
		"API_ADDR":                    "127.0.0.1:8080",
		"RATINGS_PLAYER_DATABASE_URL": "postgres://player@127.0.0.1:1/none",
		"DISCORD_DEV_USER":            "local_player",
	}
	tests := []struct {
		name, key, value string
	}{
		{"listening beyond this machine", "API_ADDR", ":8080"},
		{"in Kubernetes", "KUBERNETES_SERVICE_HOST", "10.0.0.1"},
		{"a site that is not on this machine", "SITE_ORIGIN", "https://site.example"},
		{"a Discord app as well", "DISCORD_CLIENT_ID", "100000000000000099"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := maps.Clone(local)
			env[tt.key] = tt.value
			_, _, err := openSignIns(context.Background(), func(k string) string { return env[k] })
			if err == nil || !strings.Contains(err.Error(), "DISCORD_DEV_USER") {
				t.Errorf("openSignIns with %s=%q = %v, want a refusal naming DISCORD_DEV_USER", tt.key, tt.value, err)
			}
		})
	}
	_, _, err := openSignIns(context.Background(), func(k string) string { return local[k] })
	if err == nil || !strings.Contains(err.Error(), "player login") {
		t.Errorf("openSignIns on this machine = %v, want it past the config, failing only to reach the database", err)
	}
}

// openIDApps are the env prefixes of the sign-ins set up by a client id and secret, each with an invented client id.
var openIDApps = []struct{ prefix, clientID string }{
	{"MICROSOFT", "00000000-0000-0000-0000-000000000099"},
	{"GOOGLE", "100000000099-test.apps.googleusercontent.com"},
}

func TestMicrosoftAndGoogleSignInAreOffOrFullySetUp(t *testing.T) {
	full := map[string]string{
		"SITE_ORIGIN":                 "https://site.example",
		"API_ORIGIN":                  "https://api.example",
		"RATINGS_PLAYER_DATABASE_URL": "postgres://player@127.0.0.1:1/none",
		"DISCORD_CLIENT_ID":           "100000000000000099",
		"DISCORD_CLIENT_SECRET":       "test-secret",
	}
	tests := []struct {
		name         string
		id, secret   bool
		wantPrefixed bool // the refusal names the prefix; else it gets past the config to the database
	}{
		{"off", false, false, false},
		{"set up", true, true, false},
		{"a client id alone", true, false, true},
		{"a secret alone", false, true, true},
	}
	for _, app := range openIDApps {
		for _, tt := range tests {
			t.Run(app.prefix+"/"+tt.name, func(t *testing.T) {
				env := maps.Clone(full)
				if tt.id {
					env[app.prefix+"_CLIENT_ID"] = app.clientID
				}
				if tt.secret {
					env[app.prefix+"_CLIENT_SECRET"] = "test-secret"
				}
				want := "player login"
				if tt.wantPrefixed {
					want = app.prefix
				}
				_, _, err := openSignIns(context.Background(), func(k string) string { return env[k] })
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("openSignIns = %v, want an error naming %s", err, want)
				}
			})
		}
	}
}

func TestTheMicrosoftAndGoogleStandInsAreOnlyForThisMachine(t *testing.T) {
	tests := []struct {
		name, key, value string // a key starting with _ follows the provider's prefix
	}{
		{"listening beyond this machine", "API_ADDR", ":8080"},
		{"in Kubernetes", "KUBERNETES_SERVICE_HOST", "10.0.0.1"},
		{"a site that is not on this machine", "SITE_ORIGIN", "https://site.example"},
		{"a real app as well", "_CLIENT_ID", ""},
	}
	for _, app := range openIDApps {
		local := map[string]string{
			"SITE_ORIGIN":                 "http://127.0.0.1:4321",
			"API_ORIGIN":                  "http://127.0.0.1:8080",
			"API_ADDR":                    "127.0.0.1:8080",
			"RATINGS_PLAYER_DATABASE_URL": "postgres://player@127.0.0.1:1/none",
			"DISCORD_DEV_USER":            "local_player",
			app.prefix + "_DEV_USER":      "Local Player",
		}
		for _, tt := range tests {
			t.Run(app.prefix+"/"+tt.name, func(t *testing.T) {
				env := maps.Clone(local)
				if tt.key == "_CLIENT_ID" {
					env[app.prefix+"_CLIENT_ID"], env[app.prefix+"_CLIENT_SECRET"] = app.clientID, "test-secret"
				} else {
					env[tt.key] = tt.value
				}
				_, _, err := openSignIns(context.Background(), func(k string) string { return env[k] })
				if err == nil || !strings.Contains(err.Error(), app.prefix+"_DEV_USER") {
					t.Errorf("openSignIns with %s=%q = %v, want a refusal naming %s_DEV_USER", tt.key, tt.value, err, app.prefix)
				}
			})
		}
		_, _, err := openSignIns(context.Background(), func(k string) string { return local[k] })
		if err == nil || !strings.Contains(err.Error(), "player login") {
			t.Errorf("openSignIns on this machine with %s_DEV_USER = %v, want it past the config, failing only to reach the database", app.prefix, err)
		}
	}
}
