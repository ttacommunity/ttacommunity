// Blitz games' private threads: each made by the bot in its endpoint's channel, its players and the edition's
// organisers found in the server and brought in by the edition's message mentioning them, each step kept so a retry
// goes on from it; its
// message edited and its players told when the deadline moves; closed once its round is paired again without it.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"time"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/notify"
	"ttacommunity/backend/internal/store"
)

// threadBot is the bot as a game's thread needs it.
type threadBot interface {
	GuildOf(ctx context.Context, channel string) (string, error)
	PrivateThread(ctx context.Context, channel, name string) (string, error)
	FindMember(ctx context.Context, guild, username string) (string, error)
	IsMember(ctx context.Context, guild, user string) (bool, error)
	Say(ctx context.Context, channel, text string, users []string) (string, error)
	Edit(ctx context.Context, channel, msg, text string) error
	CloseThread(ctx context.Context, thread string) error
	CheckChannel(ctx context.Context, channel string) error
}

// threadKeeper keeps each game's thread.
type threadKeeper interface {
	BlitzThread(ctx context.Context, match, endpoint int64) (*store.BlitzDiscordThread, error)
	RecordBlitzThread(ctx context.Context, match, endpoint int64, thread string, now time.Time) error
	MarkBlitzThreadPosted(ctx context.Context, match, endpoint int64, msg string, now time.Time) error
	ReplacedBlitzThreads(ctx context.Context, roundID, endpoint int64) ([]string, error)
	CloseBlitzThread(ctx context.Context, endpoint int64, thread string, now time.Time) error
	UnremindedBlitzThreads(ctx context.Context, roundID, endpoint int64) ([]store.BlitzThreadToRemind, error)
	MarkBlitzThreadReminded(ctx context.Context, endpoint int64, thread string, now time.Time) error
}

// errLimited means a bot call left for the next pass, Discord having limited the bot in this one: put off, not
// counted as a failed try.
var errLimited = fmt.Errorf("the bot is limited by Discord: tried again later: %w", store.ErrPutOff)

// paced runs call, a delivery to e, unless e is a bot channel and Discord limited the bot earlier in the pass
// (limited), then returning errLimited; a bot call Discord limits (429) sets limited, and is put off too.
func paced(limited *bool, e store.NotificationEndpoint, call func() error) error {
	if e.Kind != "bot_channel" {
		return call()
	}
	if *limited {
		return errLimited
	}
	err := call()
	var refused *notify.RefusedError
	if errors.As(err, &refused) && refused.Status == http.StatusTooManyRequests {
		*limited = true
		return fmt.Errorf("%w: %w", err, store.ErrPutOff)
	}
	return err
}

// deliverToBot sends n to bot channel e through bot at now: a game's thread, a moved deadline, the closing of a
// round's replaced threads, or a round's reminder in its games' threads.
func deliverToBot(ctx context.Context, keep threadKeeper, bot threadBot, n store.Notification, e store.NotificationEndpoint, now time.Time, log *slog.Logger) error {
	switch n.Kind {
	case "blitz_thread":
		return deliverThread(ctx, keep, bot, n, e, now, log)
	case "blitz_deadline":
		return deliverDeadline(ctx, keep, bot, n, e)
	case "blitz_replaced":
		return closeReplaced(ctx, keep, bot, n, e, now)
	case "blitz_reminder":
		return remindThreads(ctx, keep, bot, n, e, now)
	}
	return fmt.Errorf("a bot channel takes no %s", n.Kind)
}

// discordUserID is what a Discord user id is.
var discordUserID = regexp.MustCompile(`^[0-9]{17,20}$`)

// deliverThread makes game n's private thread in bot channel e at now, unless keep has one: it adds the players and
// organisers, each by the Discord id of their sign-in or else found by their Discord username in the channel's
// server, and posts the edition's message, asking the organisers to add a player not found. A thread posted is done.
// Logs name no one.
func deliverThread(ctx context.Context, keep threadKeeper, bot threadBot, n store.Notification, e store.NotificationEndpoint, now time.Time, log *slog.Logger) error {
	t, g := n.Thread.Threads, n.Thread.Game
	made, err := keep.BlitzThread(ctx, n.MatchID, e.ID)
	if err != nil {
		return err
	}
	if made != nil && made.Posted {
		return nil
	}
	title, _ := blitz.GameThread(t, g, nil)
	if made == nil {
		id, err := bot.PrivateThread(ctx, e.ChannelID, title)
		if err != nil {
			return fmt.Errorf("making a thread: %w", err)
		}
		if err := keep.RecordBlitzThread(ctx, n.MatchID, e.ID, id, now); err != nil {
			log.Warn("keeping a thread made failed: a retry makes another", "endpoint", e.ID, "err", err)
			return err
		}
		made = &store.BlitzDiscordThread{ID: id}
	}
	t, g, users, missing, err := reachAll(ctx, bot, e, t, g)
	if err != nil {
		return err
	}
	_, message := blitz.GameThread(t, g, missing)
	msg, err := bot.Say(ctx, made.ID, message, users)
	if err != nil {
		return fmt.Errorf("posting in a thread: %w", err)
	}
	return keep.MarkBlitzThreadPosted(ctx, n.MatchID, e.ID, msg, now)
}

// reachAll finds game g's players and t's organisers in e's channel's server, each by the Discord id of their
// sign-in, checked to be a member, or else by their Discord username, and returns t and g with the ids found,
// everyone found, whom a message mentions into the thread, and the players with a Discord name not found or not in
// the server.
func reachAll(ctx context.Context, bot threadBot, e store.NotificationEndpoint, t blitz.Threads, g blitz.ThreadGame) (blitz.Threads, blitz.ThreadGame, []string, []blitz.Person, error) {
	guild, err := bot.GuildOf(ctx, e.ChannelID)
	if err != nil {
		return t, g, nil, nil, fmt.Errorf("reading the channel's server: %w", err)
	}
	var users []string
	var missing []blitz.Person
	reach := func(p *blitz.Person, player bool) error {
		id := p.DiscordID
		name, named := blitz.Username(p.Discord)
		reachable := discordUserID.MatchString(id) || named
		if !discordUserID.MatchString(id) {
			id = ""
			if named {
				if id, err = bot.FindMember(ctx, guild, name); err != nil {
					return fmt.Errorf("finding a member: %w", err)
				}
			}
		}
		if id != "" && discordUserID.MatchString(p.DiscordID) {
			in, err := bot.IsMember(ctx, guild, id)
			if err != nil {
				return fmt.Errorf("checking a member: %w", err)
			}
			if !in {
				id = ""
			}
		}
		if id == "" {
			if player && reachable {
				missing = append(missing, *p)
			}
			return nil
		}
		p.DiscordID = id
		users = append(users, id)
		return nil
	}
	t.Organisers = slices.Clone(t.Organisers)
	for _, p := range []*blitz.Person{&g.A, &g.B} {
		if err := reach(p, true); err != nil {
			return t, g, nil, nil, err
		}
	}
	for i := range t.Organisers {
		if err := reach(&t.Organisers[i], false); err != nil {
			return t, g, nil, nil, err
		}
	}
	return t, g, users, missing, nil
}

// deliverDeadline edits game n's posted thread on e to its new deadline and tells its players there; a thread not
// posted yet will post it, and one gone needs nothing.
func deliverDeadline(ctx context.Context, keep threadKeeper, bot threadBot, n store.Notification, e store.NotificationEndpoint) error {
	made, err := keep.BlitzThread(ctx, n.MatchID, e.ID)
	if err != nil || made == nil || !made.Posted || made.MessageID == "" {
		return err
	}
	t, g, _, missing, err := reachAll(ctx, bot, e, n.Thread.Threads, n.Thread.Game)
	if err != nil {
		return err
	}
	_, message := blitz.GameThread(t, g, missing)
	if err := bot.Edit(ctx, made.ID, made.MessageID, message); err != nil {
		return fmt.Errorf("editing a thread's message: %w", err)
	}
	var players []string
	for _, p := range []blitz.Person{g.A, g.B} {
		if discordUserID.MatchString(p.DiscordID) && !slices.Contains(missing, p) {
			players = append(players, p.DiscordID)
		}
	}
	if _, err := bot.Say(ctx, made.ID, blitz.DeadlineMoved(t, g), players); err != nil {
		return fmt.Errorf("posting in a thread: %w", err)
	}
	return nil
}

// closeReplaced tells each of e's threads of round n.RoundID whose game the round no longer has that it was paired
// again, and closes it at now; a thread deleted since is only marked closed.
func closeReplaced(ctx context.Context, keep threadKeeper, bot threadBot, n store.Notification, e store.NotificationEndpoint, now time.Time) error {
	threads, err := keep.ReplacedBlitzThreads(ctx, n.RoundID, e.ID)
	if err != nil {
		return err
	}
	for _, id := range threads {
		_, err := bot.Say(ctx, id, blitz.ReplacedThread, nil)
		if err == nil {
			err = bot.CloseThread(ctx, id)
		}
		var refused *notify.RefusedError
		if err != nil && !(errors.As(err, &refused) && refused.Status == http.StatusNotFound) {
			return fmt.Errorf("closing a replaced thread: %w", err)
		}
		if err := keep.CloseBlitzThread(ctx, e.ID, id, now); err != nil {
			return err
		}
	}
	return nil
}

// remindThreads reminds the players in each of e's threads of round n.RoundID whose game is still to play, pinging
// them, and keeps each reminded at now, so a retry reminds only the rest.
func remindThreads(ctx context.Context, keep threadKeeper, bot threadBot, n store.Notification, e store.NotificationEndpoint, now time.Time) error {
	due, err := keep.UnremindedBlitzThreads(ctx, n.RoundID, e.ID)
	if err != nil {
		return err
	}
	players := n.Thread.Threads
	players.Organisers = nil
	for _, d := range due {
		g, ok := n.Thread.ByMatch[d.MatchID]
		if !ok {
			continue
		}
		t, g, users, _, err := reachAll(ctx, bot, e, players, g)
		if err != nil {
			return err
		}
		if _, err := bot.Say(ctx, d.ThreadID, blitz.ThreadReminder(t, g), users); err != nil {
			return fmt.Errorf("reminding a thread: %w", err)
		}
		if err := keep.MarkBlitzThreadReminded(ctx, e.ID, d.ThreadID, now); err != nil {
			return err
		}
	}
	return nil
}
