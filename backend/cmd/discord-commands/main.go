// Command discord-commands registers the Discord bot's slash commands (commands.json) in one Discord server,
// replacing the ones there. The bot token comes from DISCORD_BOT_TOKEN, never a flag, so no shell history holds it.
package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

//go:embed commands.json
var commands []byte

const discordAPI = "https://discord.com/api/v10"

var snowflake = regexp.MustCompile(`^[0-9]{17,20}$`)

func main() {
	app := flag.String("app", "", "the Discord application's id")
	server := flag.String("server", "", "the id of the Discord server to register the commands in")
	flag.Parse()
	names, err := register(context.Background(), &http.Client{Timeout: 30 * time.Second}, discordAPI, *app, *server, os.Getenv("DISCORD_BOT_TOKEN"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("registered in %s: %s\n", *server, strings.Join(names, ", "))
}

// register replaces app's commands in server with commands.json at api, with the bot token, returning their names.
func register(ctx context.Context, client *http.Client, api, app, server, token string) ([]string, error) {
	if !snowflake.MatchString(app) || !snowflake.MatchString(server) {
		return nil, errors.New("-app and -server must be Discord ids (17 to 20 digits)")
	}
	if token == "" {
		return nil, errors.New("DISCORD_BOT_TOKEN is not set")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, api+"/applications/"+app+"/guilds/"+server+"/commands", bytes.NewReader(commands))
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
