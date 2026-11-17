// Registering the Discord bot's slash commands with Discord as the api starts, so a deploy brings new commands.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ttacommunity/backend/internal/discordcommands"
)

// registered keeps that the commands names were registered with Discord at a time, for admin to show.
type registered func(ctx context.Context, names []string, now time.Time) error

// registerCommands registers the bot's commands everywhere its app is installed, from getenv's DISCORD_APP_ID and
// DISCORD_BOT_TOKEN, asking Discord at api, and keeps what Discord then holds with record at now. Either unset
// skips it; a failure is logged and never stops the api.
func registerCommands(ctx context.Context, getenv func(string) string, client *http.Client, api string, record registered, now func() time.Time, log *slog.Logger) {
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
	bare := make([]string, len(names))
	for i, n := range names {
		bare[i] = strings.TrimPrefix(n, "/")
	}
	if err := record(ctx, bare, now()); err != nil {
		log.Error("recording the Discord bot's commands failed", "err", err)
	}
}
