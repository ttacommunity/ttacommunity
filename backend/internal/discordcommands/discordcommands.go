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
	"slices"
	"sync"
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

// Command is a command as admin lists it: its name, without /, what it does, and its options.
type Command struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Options     []Option `json:"options"`
}

// Option is one of a command's options, by name.
type Option struct {
	Name string `json:"name"`
}

// Takes says whether c has the option name.
func (c Command) Takes(name string) bool {
	return slices.ContainsFunc(c.Options, func(o Option) bool { return o.Name == name })
}

// List is every command in commands.json, in its order.
func List() ([]Command, error) { return list() }

// list reads commands.json once.
var list = sync.OnceValues(func() ([]Command, error) {
	var commands []Command
	if err := json.Unmarshal(Commands, &commands); err != nil {
		return nil, fmt.Errorf("reading commands.json: %w", err)
	}
	return commands, nil
})

// Find is the command named name, with false when there is none or commands.json cannot be read.
func Find(name string) (Command, bool) {
	commands, err := list()
	if err != nil {
		return Command{}, false
	}
	i := slices.IndexFunc(commands, func(c Command) bool { return c.Name == name })
	if i < 0 {
		return Command{}, false
	}
	return commands[i], true
}

// InstallURL is the link that adds app's commands to a server, or "" when app is not a Discord id.
func InstallURL(app string) string {
	if !snowflake.MatchString(app) {
		return ""
	}
	return "https://discord.com/oauth2/authorize?client_id=" + app + "&scope=applications.commands"
}
