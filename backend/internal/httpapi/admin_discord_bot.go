// Admin's Discord bot: its commands, each with its settings (on or off, who sees a reply, the format shown when the
// asker gives none, how many rows a list shows), and when the commands were last registered with Discord.
// Admins only.
package httpapi

import (
	"context"
	"errors"
	"slices"

	"ttacommunity/backend/internal/discordcommands"
	"ttacommunity/backend/internal/store"
)

const onlyAdminsBot = "only admins manage the Discord bot"

// commandOut is cmd with its settings c as the API sends them: a format only for a command with formats, rows
// only for one that lists.
func commandOut(cmd discordcommands.Command, c store.CommandSettings) DiscordBotCommand {
	out := DiscordBotCommand{Name: cmd.Name, Description: cmd.Description, On: c.On, Reply: DiscordReplyVisibility(c.Reply)}
	if cmd.Takes("format") {
		out.Format = &c.Format
	}
	if slices.Contains(listCommands, cmd.Name) {
		out.Results = &c.Results
	}
	return out
}

func (s *server) GetAdminDiscordBot(ctx context.Context, _ GetAdminDiscordBotRequestObject) (GetAdminDiscordBotResponseObject, error) {
	if _, ok, err := adminOnly(ctx); err != nil {
		return nil, err
	} else if !ok {
		return GetAdminDiscordBot403JSONResponse{forbidden(onlyAdminsBot)}, nil
	}
	saved, err := s.admins.DiscordCommands(ctx)
	if err != nil {
		return nil, err
	}
	reg, err := s.admins.BotRegistration(ctx)
	if err != nil {
		return nil, err
	}
	list, err := discordcommands.List()
	if err != nil {
		return nil, err
	}
	out := DiscordBot{RegisteredAt: inUTC(reg.At), Registered: reg.Commands, Commands: make([]DiscordBotCommand, len(list))}
	if out.Registered == nil {
		out.Registered = []string{}
	}
	for i, cmd := range list {
		c := commandDefaults(cmd.Name)
		if j := slices.IndexFunc(saved, func(sc store.CommandSettings) bool { return sc.Name == cmd.Name }); j >= 0 {
			c = saved[j]
		}
		out.Commands[i] = commandOut(cmd, c)
	}
	if url := discordcommands.InstallURL(s.admins.DiscordAppID()); url != "" {
		out.InstallUrl = &url
	}
	return GetAdminDiscordBot200JSONResponse{Body: out, Headers: GetAdminDiscordBot200ResponseHeaders{CacheControl: cacheNone}}, nil
}

// commandIn is the settings b asks for cmd, or the refusal of a setting cmd does not have or an unknown reply. A
// format or rows not given is cmd's default; the database checks the values.
func commandIn(cmd discordcommands.Command, b *DiscordCommandSetting) (store.CommandSettings, string) {
	if b == nil {
		return store.CommandSettings{}, "a command's settings: on and reply, and format and results where it has them"
	}
	if !b.Reply.Valid() {
		return store.CommandSettings{}, "reply is private, shared, always_private or always_shared"
	}
	c := commandDefaults(cmd.Name)
	c.On, c.Reply = b.On, string(b.Reply)
	switch {
	case b.Format != nil && !cmd.Takes("format"):
		return store.CommandSettings{}, "/" + cmd.Name + " has no format"
	case b.Format != nil:
		c.Format = *b.Format
	}
	switch {
	case b.Results != nil && !slices.Contains(listCommands, cmd.Name):
		return store.CommandSettings{}, "/" + cmd.Name + " lists nothing"
	case b.Results != nil:
		c.Results = *b.Results
	}
	return c, ""
}

func (s *server) PutAdminDiscordCommand(ctx context.Context, req PutAdminDiscordCommandRequestObject) (PutAdminDiscordCommandResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PutAdminDiscordCommand403JSONResponse{forbidden(onlyAdminsBot)}, nil
	}
	cmd, found := discordcommands.Find(req.Command)
	if !found {
		return PutAdminDiscordCommand404JSONResponse{notFound404("the bot has no such command")}, nil
	}
	c, msg := commandIn(cmd, req.Body)
	if msg != "" {
		return PutAdminDiscordCommand400JSONResponse{badRequest400(msg)}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetDiscordCommand(ctx, c); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "discord.command.set", c.Name, req.Body)
	})
	switch {
	case errors.Is(err, store.ErrBadCommandSetting):
		return PutAdminDiscordCommand400JSONResponse{badRequest400(store.ErrBadCommandSetting.Error())}, nil
	case err != nil:
		return nil, err
	}
	return PutAdminDiscordCommand204Response{}, nil
}
