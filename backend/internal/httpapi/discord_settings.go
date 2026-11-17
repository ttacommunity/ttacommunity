// A command's settings as it is answered: those admins saved, or its defaults, applied to what the asker gave.
package httpapi

import (
	"context"
	"slices"

	"ttacommunity/backend/internal/discordcommands"
	"ttacommunity/backend/internal/store"
)

// defaultRows is how many rows a list shows until admins choose.
const defaultRows = 5

// listCommands are the commands that show a list, whose rows admins may set.
var listCommands = []string{"rivals", "upsets"}

// What Discord's options are, by number.
const discordInteger, discordBoolean = 4, 5

// commandDefaults are name's settings until admins save others: on; a reply private unless the asker shares it, but
// shared unless asked private for a command with a private option; every format; defaultRows for a list.
func commandDefaults(name string) store.CommandSettings {
	c := store.CommandSettings{Name: name, On: true, Reply: "private"}
	if cmd, ok := discordcommands.Find(name); ok && cmd.Takes("private") {
		c.Reply = "shared"
	}
	if slices.Contains(listCommands, name) {
		c.Results = defaultRows
	}
	return c
}

// settingsFor are name's settings: those saved, or its defaults when none are or they cannot be read.
func (s *server) settingsFor(ctx context.Context, name string) store.CommandSettings {
	c, found, err := s.discord.CommandSettings(ctx, name)
	if err != nil {
		s.log.Error("reading a Discord command's settings failed", "request_id", requestID(ctx), "err", err)
	}
	if err != nil || !found {
		return commandDefaults(name)
	}
	return c
}

// applySettings returns the settings of i's command and, when it is on, fills in i's options from them: who sees
// the reply (its share option, or its private one), and the format when the asker gave none.
func (s *server) applySettings(ctx context.Context, i *DiscordInteraction) store.CommandSettings {
	name := commandName(*i)
	c := s.settingsFor(ctx, name)
	cmd, ok := discordcommands.Find(name)
	if !c.On || !ok || i.Data == nil {
		return c
	}
	visibility, shared := "share", true
	if cmd.Takes("private") {
		visibility, shared = "private", false
	}
	given := option(*i, visibility) != nil
	switch {
	case c.Reply == "always_shared", c.Reply == "shared" && !given:
		setOption(i, visibility, discordBoolean, shared)
	case c.Reply == "always_private", c.Reply == "private" && !given:
		setOption(i, visibility, discordBoolean, !shared)
	}
	if cmd.Takes("format") && option(*i, "format") == nil && c.Format != 0 {
		setOption(i, "format", discordInteger, float64(c.Format))
	}
	return c
}

// rows is how many rows a list command with settings c shows.
func rows(c store.CommandSettings) int {
	if c.Results > 0 {
		return c.Results
	}
	return defaultRows
}

// setOption gives i's option name, of Discord's type kind, the value, replacing what the asker gave.
func setOption(i *DiscordInteraction, name string, kind int, value any) {
	var options []DiscordOption
	if i.Data.Options != nil {
		options = slices.DeleteFunc(*i.Data.Options, func(o DiscordOption) bool { return o.Name == name })
	}
	options = append(options, DiscordOption{Name: name, Type: kind, Value: value})
	i.Data.Options = &options
}
