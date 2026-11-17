// Package discordcommands holds the Discord bot's slash commands (commands.json) and registers them with Discord,
// everywhere the app is installed or in one server.
package discordcommands

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
)

// Commands is commands.json: every command, with the options the api reads.
//
//go:embed commands.json
var Commands []byte

// API is Discord's API, which Register is given outside tests.
const API = "https://discord.com/api/v10"

var snowflake = regexp.MustCompile(`^[0-9]{17,20}$`)

// Register replaces app's commands with commands at api, with the bot token: everywhere the app is installed when
// server is "", else in that server alone. It returns the names Discord now holds.
func Register(ctx context.Context, client *http.Client, api, app, server, token string, commands []byte) ([]string, error) {
	if !snowflake.MatchString(app) || (server != "" && !snowflake.MatchString(server)) {
		return nil, errors.New("the application and server must be Discord ids (17 to 20 digits)")
	}
	if token == "" {
		return nil, errors.New("DISCORD_BOT_TOKEN is not set")
	}
	path := api + "/applications/" + app + "/commands"
	if server != "" {
		path = api + "/applications/" + app + "/guilds/" + server + "/commands"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, path, bytes.NewReader(commands))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bot "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("asking Discord: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading Discord's answer: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		var refusal struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &refusal)
		// "Missing Access" (403) means the application is not in that server.
		return nil, fmt.Errorf("the Discord API refused (%d): %s", res.StatusCode, refusal.Message)
	}
	var registered []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &registered); err != nil {
		return nil, fmt.Errorf("reading Discord's answer: %w", err)
	}
	names := make([]string, len(registered))
	for i, c := range registered {
		names[i] = "/" + c.Name
	}
	return names, nil
}
