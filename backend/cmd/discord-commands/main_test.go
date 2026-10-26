package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

const (
	app    = "100000000000000001"
	server = "100000000000000002"
)

func TestRegisterReplacesTheServersCommands(t *testing.T) {
	var method, path, auth string
	var body []byte
	discord := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, auth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		body, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`[{"name":"rating"}]`))
	}))
	defer discord.Close()
	names, err := register(context.Background(), discord.Client(), discord.URL, app, server, "invented-token")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if !slices.Equal(names, []string{"/rating"}) {
		t.Errorf("names = %q, want [/rating]", names)
	}
	if want := "/applications/" + app + "/guilds/" + server + "/commands"; method != http.MethodPut || path != want {
		t.Errorf("request = %s %s, want PUT %s", method, path, want)
	}
	if auth != "Bot invented-token" {
		t.Errorf("Authorization = %q, want the bot token", auth)
	}
	if string(body) != string(commands) {
		t.Errorf("body = %s, want commands.json", body)
	}
}

func TestRegisterRefuses(t *testing.T) {
	tests := []struct {
		name, app, server, token string
		discordStatus            int
		wantErr                  string
		wantAsked                bool
	}{
		{"an application id that is not one", "12", server, "invented-token", 0, "Discord ids", false},
		{"a server id that is not one", app, "server", "invented-token", 0, "Discord ids", false},
		{"no bot token", app, server, "", 0, "DISCORD_BOT_TOKEN", false},
		{"an application not in that server", app, server, "invented-token", http.StatusForbidden, "Missing Access", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asked := false
			discord := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				asked = true
				w.WriteHeader(tt.discordStatus)
				_, _ = w.Write([]byte(`{"message":"Missing Access","code":50001}`))
			}))
			defer discord.Close()
			_, err := register(context.Background(), discord.Client(), discord.URL, tt.app, tt.server, tt.token)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("register error = %v, want one naming %q", err, tt.wantErr)
			}
			if err != nil && tt.token != "" && strings.Contains(err.Error(), tt.token) {
				t.Errorf("register error = %v, want the token kept out of it", err)
			}
			if asked != tt.wantAsked {
				t.Errorf("Discord asked = %v, want %v", asked, tt.wantAsked)
			}
		})
	}
}

func TestTheCommandsHaveTheOptionsTheApiReads(t *testing.T) {
	var cmds []struct {
		Name    string `json:"name"`
		Options []struct {
			Name     string `json:"name"`
			Required bool   `json:"required"`
		} `json:"options"`
	}
	if err := json.Unmarshal(commands, &cmds); err != nil {
		t.Fatalf("commands.json: %v", err)
	}
	got := map[string][]string{}
	for _, c := range cmds {
		for _, o := range c.Options {
			got[c.Name] = append(got[c.Name], fmt.Sprintf("%s required=%v", o.Name, o.Required))
		}
	}
	want := map[string][]string{
		"rating":   {"player required=true", "format required=false", "share required=false"},
		"myrating": {"format required=false", "share required=false"},
	}
	if !maps.EqualFunc(got, want, slices.Equal) {
		t.Errorf("commands = %q, want %q", got, want)
	}
}
