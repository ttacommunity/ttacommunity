// Registering the Discord bot's slash commands with Discord as the api starts, so a deploy brings new commands.
package main

import (
	"context"
	"log/slog"
	"net/http"

	"ttacommunity/backend/internal/discordcommands"
)

// registerCommands registers the bot's commands everywhere its app is installed, from getenv's DISCORD_APP_ID and
// DISCORD_BOT_TOKEN, asking Discord at api. Either unset skips it; a failure is logged and never stops the api.
func registerCommands(ctx context.Context, getenv func(string) string, client *http.Client, api string, log *slog.Logger) {
	app, token := getenv("DISCORD_APP_ID"), getenv("DISCORD_BOT_TOKEN")
	if app == "" || token == "" {
		log.Info("not registering the Discord bot's commands: DISCORD_APP_ID or DISCORD_BOT_TOKEN is not set")
		return
	}
	names, err := discordcommands.Register(ctx, client, api, app, "", token, discordcommands.Commands)
	if err != nil {
		log.Error("registering the Discord bot's commands failed", "err", err)
		return
	}
	log.Info("registered the Discord bot's commands", "commands", len(names))
}
