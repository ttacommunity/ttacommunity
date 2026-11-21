// The bot's own calls to Discord's API, with its token: a blitz game's private thread made in a channel, the
// server's members found by username and added to it, its message posted and edited, and the thread closed.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// DiscordAPI is Discord's API, which NewBot is given outside tests.
const DiscordAPI = "https://discord.com/api/v10"

// ErrNoBot means a bot channel to post to without the bot's token.
var ErrNoBot = errors.New("the bot's token, DISCORD_BOT_TOKEN, is not set")

// privateThread is Discord's type for a private thread, and weekLong the longest it lets one wait before archiving.
const (
	privateThread = 12
	weekLong      = 10080
)

// Bot calls Discord's API at api as the bot, whose token is token.
type Bot struct {
	hc         *http.Client
	api, token string
}

// NewBot returns the bot calling api with token over hc, whose timeout bounds each call.
func NewBot(hc *http.Client, api, token string) *Bot {
	return &Bot{hc: hc, api: strings.TrimSuffix(api, "/"), token: token}
}

// GuildOf returns the id of the server channel is in.
func (b *Bot) GuildOf(ctx context.Context, channel string) (string, error) {
	var c struct {
		GuildID string `json:"guild_id"`
	}
	if err := b.call(ctx, http.MethodGet, "/channels/"+url.PathEscape(channel), nil, &c); err != nil {
		return "", err
	}
	if c.GuildID == "" {
		return "", errors.New("the bot's channel is in no server")
	}
	return c.GuildID, nil
}

// PrivateThread makes a private thread named name in channel, which no player can invite others to, archived
// after a week without messages, and returns its id.
func (b *Bot) PrivateThread(ctx context.Context, channel, name string) (string, error) {
	var made struct {
		ID string `json:"id"`
	}
	body := map[string]any{"name": cut(name, maxThreadName), "type": privateThread, "invitable": false, "auto_archive_duration": weekLong}
	if err := b.call(ctx, http.MethodPost, "/channels/"+url.PathEscape(channel)+"/threads", body, &made); err != nil {
		return "", err
	}
	return made.ID, nil
}

// FindMember returns the id of guild's member whose username is username, in any letters, or "" for none.
func (b *Bot) FindMember(ctx context.Context, guild, username string) (string, error) {
	var members []struct {
		User struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"user"`
	}
	query := url.Values{"query": {username}, "limit": {"100"}}
	if err := b.call(ctx, http.MethodGet, "/guilds/"+url.PathEscape(guild)+"/members/search?"+query.Encode(), nil, &members); err != nil {
		return "", err
	}
	for _, m := range members {
		if strings.EqualFold(m.User.Username, username) {
			return m.User.ID, nil
		}
	}
	return "", nil
}

// AddToThread adds user to thread; adding a member again changes nothing.
func (b *Bot) AddToThread(ctx context.Context, thread, user string) error {
	return b.call(ctx, http.MethodPut, "/channels/"+url.PathEscape(thread)+"/thread-members/"+url.PathEscape(user), nil, nil)
}

// Say posts text, cut to Discord's limit, in channel, letting it ping only users, at most Discord's 100, and returns
// the message's id.
func (b *Bot) Say(ctx context.Context, channel, text string, users []string) (string, error) {
	var made struct {
		ID string `json:"id"`
	}
	err := b.call(ctx, http.MethodPost, "/channels/"+url.PathEscape(channel)+"/messages", message(text, users), &made)
	return made.ID, err
}

// Edit replaces the text of the bot's message in channel with text, cut to Discord's limit, pinging no one.
func (b *Bot) Edit(ctx context.Context, channel, msg, text string) error {
	return b.call(ctx, http.MethodPatch, "/channels/"+url.PathEscape(channel)+"/messages/"+url.PathEscape(msg), message(text, nil), nil)
}

// CloseThread archives and locks thread, so only moderators reopen it; without the Manage Threads permission to
// lock it, it is only archived, and a message reopens it.
func (b *Bot) CloseThread(ctx context.Context, thread string) error {
	path := "/channels/" + url.PathEscape(thread)
	err := b.call(ctx, http.MethodPatch, path, map[string]bool{"archived": true, "locked": true}, nil)
	var refused *RefusedError
	if errors.As(err, &refused) && refused.Status == http.StatusForbidden {
		return b.call(ctx, http.MethodPatch, path, map[string]bool{"archived": true}, nil)
	}
	return err
}

// message is text, cut to Discord's limit, as a message that may ping only users, each once (Discord refuses a
// repeat), at most Discord's 100.
func message(text string, users []string) map[string]any {
	users = slices.Compact(slices.Sorted(slices.Values(users)))
	if users == nil {
		users = []string{}
	}
	return map[string]any{"content": cut(text, maxContent),
		"allowed_mentions": map[string][]string{"parse": {}, "users": users[:min(len(users), maxMentions)]}}
}

// call sends body, as JSON unless nil, to path with the bot's token, decoding Discord's answer into out unless it
// is nil. It returns a *RefusedError when Discord answers other than success; no error holds the token.
func (b *Bot) call(ctx context.Context, method, path string, body, out any) error {
	var payload io.Reader
	if body != nil {
		j, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("writing a call to Discord: %w", err)
		}
		payload = bytes.NewReader(j)
	}
	req, err := http.NewRequestWithContext(ctx, method, b.api+path, payload)
	if err != nil {
		return errors.New("calling Discord: not a URL")
	}
	req.Header.Set("Authorization", "Bot "+b.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.hc.Do(req)
	if err != nil {
		var u *url.Error
		if errors.As(err, &u) {
			err = u.Err
		}
		return fmt.Errorf("calling Discord: %s", strings.ReplaceAll(err.Error(), req.URL.Host, "Discord"))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &RefusedError{Status: resp.StatusCode}
	}
	if out != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
			return errors.New("calling Discord: its answer was not what was asked for")
		}
	}
	return nil
}
