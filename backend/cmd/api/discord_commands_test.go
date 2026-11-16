package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTheApiRegistersTheCommandsEverywhereWhenSetUp(t *testing.T) {
	const app, token = "100000000000000003", "invented-bot-token"
	tests := []struct {
		name          string
		env           map[string]string
		discordStatus int
		wantPath      string
		wantLog       string
	}{
		{"set up", map[string]string{"DISCORD_APP_ID": app, "DISCORD_BOT_TOKEN": token}, http.StatusOK,
			"/applications/" + app + "/commands", "registered the Discord bot's commands"},
		{"Discord refusing, logged", map[string]string{"DISCORD_APP_ID": app, "DISCORD_BOT_TOKEN": token}, http.StatusUnauthorized,
			"/applications/" + app + "/commands", "registering the Discord bot's commands failed"},
		{"no token", map[string]string{"DISCORD_APP_ID": app}, http.StatusOK, "", "not registering"},
		{"no application", map[string]string{"DISCORD_BOT_TOKEN": token}, http.StatusOK, "", "not registering"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var path string
			discord := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path = r.URL.Path
				w.WriteHeader(tt.discordStatus)
				_, _ = w.Write([]byte(`[{"name":"rating"}]`))
			}))
			defer discord.Close()
			var logs bytes.Buffer
			registerCommands(context.Background(), func(k string) string { return tt.env[k] }, discord.Client(), discord.URL,
				slog.New(slog.NewJSONHandler(&logs, nil)))
			if path != tt.wantPath {
				t.Errorf("asked Discord at %q, want %q", path, tt.wantPath)
			}
			if !strings.Contains(logs.String(), tt.wantLog) {
				t.Errorf("logs = %s, want %q", logs.String(), tt.wantLog)
			}
			if strings.Contains(logs.String(), token) {
				t.Errorf("logs = %s, want no bot token in them", logs.String())
			}
		})
	}
}
