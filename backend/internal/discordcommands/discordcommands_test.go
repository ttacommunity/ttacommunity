package discordcommands_test

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

	"ttacommunity/backend/internal/discordcommands"
)

const (
	app    = "100000000000000001"
	server = "100000000000000002"
)

func TestRegisterReplacesTheCommands(t *testing.T) {
	tests := []struct {
		name, server, wantPath string
	}{
		{"everywhere the app is", "", "/applications/" + app + "/commands"},
		{"in one server", server, "/applications/" + app + "/guilds/" + server + "/commands"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var method, path, auth string
			var body []byte
			discord := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				method, path, auth = r.Method, r.URL.Path, r.Header.Get("Authorization")
				body, _ = io.ReadAll(r.Body)
				_, _ = w.Write([]byte(`[{"name":"rating"}]`))
			}))
			defer discord.Close()
			names, err := discordcommands.Register(context.Background(), discord.Client(), discord.URL, app, tt.server, "invented-token", discordcommands.Commands)
			if err != nil {
				t.Fatalf("Register: %v", err)
			}
			if !slices.Equal(names, []string{"/rating"}) {
				t.Errorf("names = %q, want [/rating]", names)
			}
			if method != http.MethodPut || path != tt.wantPath {
				t.Errorf("request = %s %s, want PUT %s", method, path, tt.wantPath)
			}
			if auth != "Bot invented-token" {
				t.Errorf("Authorization = %q, want the bot token", auth)
			}
			if string(body) != string(discordcommands.Commands) {
				t.Errorf("body = %s, want commands.json", body)
			}
		})
	}
}

func TestRegisterRefuses(t *testing.T) {
	tests := []struct {
		name, app, server, token string
		discordStatus            int
		wantErr                  string
		wantAsked                bool
	}{
		{"an application id that is not one", "12", "", "invented-token", 0, "Discord ids", false},
		{"a server id that is not one", app, "server", "invented-token", 0, "Discord ids", false},
		{"no bot token", app, "", "", 0, "DISCORD_BOT_TOKEN", false},
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
			_, err := discordcommands.Register(context.Background(), discord.Client(), discord.URL, tt.app, tt.server, tt.token, discordcommands.Commands)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Register error = %v, want one naming %q", err, tt.wantErr)
			}
			if err != nil && tt.token != "" && strings.Contains(err.Error(), tt.token) {
				t.Errorf("Register error = %v, want the token kept out of it", err)
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
	if err := json.Unmarshal(discordcommands.Commands, &cmds); err != nil {
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
		"roll":     {"dice required=true", "private required=false"},
		"versus":   {"player required=true", "other required=true", "format required=false", "share required=false"},
		"rivals":   {"player required=true", "format required=false", "share required=false"},
		"seats":    {"player required=true", "format required=false", "share required=false"},
		"compare": {"player1 required=true", "player2 required=true", "player3 required=false", "player4 required=false",
			"share required=false"},
	}
	if !maps.EqualFunc(got, want, slices.Equal) {
		t.Errorf("commands = %q, want %q", got, want)
	}
}
