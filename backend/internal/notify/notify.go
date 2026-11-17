// Package notify writes notifications as Discord messages and posts them to Discord webhooks. A message names
// players as typed, never as markdown or mentions, and links to where admin answers it.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ttacommunity/backend/internal/discordbot"
	"ttacommunity/backend/internal/store"
)

// maxContent is the most characters Discord takes in a message.
const maxContent = 2000

// TestText is the message admin's Test button posts.
const TestText = "A test from ttacommunity admin: notifications reach this channel."

// RefusedError means Discord answered a post with a status other than success.
type RefusedError struct{ Status int }

func (e *RefusedError) Error() string { return fmt.Sprintf("Discord answered %d", e.Status) }

// signInNames are how the message names each way an account signs in.
var signInNames = map[string]string{"discord": "Discord", "email": "email", "microsoft": "Microsoft", "google": "Google"}

// list joins items as a sentence does: "a", "a and b", "a, b and c".
func list(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// outcome says how a blitz match went: result a, b, draw or double_loss between a and b.
func outcome(result, a, b string) string {
	switch result {
	case "a":
		return a + " beat " + b
	case "b":
		return b + " beat " + a
	case "draw":
		return a + " and " + b + " drew"
	case "double_loss":
		return a + " and " + b + " both lost"
	}
	return "no result"
}

// Text is n as a Discord message: its heading in bold, then what it tells, linking to where admin, at origin admin,
// answers it.
func Text(n store.Notification, admin string) string {
	heading, body := parts(n, admin)
	return "**" + heading + "**: " + body
}

// parts is n's heading, such as Link request, and what it tells, as Text writes them.
func parts(n store.Notification, admin string) (string, string) {
	esc := discordbot.EscapeMarkdown
	ways := make([]string, len(n.SignIns))
	for i, s := range n.SignIns {
		ways[i] = signInNames[s]
	}
	switch n.Kind {
	case "sign_up":
		linked := "is not linked to a TTA username yet"
		switch n.LinkedHow {
		case "match":
			linked = fmt.Sprintf("was linked automatically to **%s**", esc(n.Name))
		case "approved":
			linked = fmt.Sprintf("was linked by an admin to **%s**", esc(n.Name))
		}
		return "New player", fmt.Sprintf("someone signed in with %s for the first time and %s. <%s/#members>", list(ways), linked, admin)
	case "link_request":
		return "Link request", fmt.Sprintf("a player who signs in with %s asks to be linked to **%s**. Answer it in admin: <%s/#members>",
			list(ways), esc(n.Name), admin)
	case "reddit_failed":
		return "Reddit feed failing", fmt.Sprintf("r/%s has failed its last %d reads: %s. <%s/#notifications>",
			esc(n.Reddit.Subreddit), store.FeedFailuresNotified, esc(n.FeedError), admin)
	case "reddit_recovered":
		return "Reddit feed reading again", fmt.Sprintf("r/%s reads again; its new posts are posted. <%s/#notifications>", esc(n.Reddit.Subreddit), admin)
	}
	where := fmt.Sprintf("%s %s, round %d", esc(n.Series), esc(n.Edition), n.Round)
	link := fmt.Sprintf("<%s/blitz/?t=%s&e=%s>", admin, url.QueryEscape(n.Slug), url.QueryEscape(n.Edition))
	how := outcome(n.Result, esc(n.A), esc(n.B))
	if n.Kind == "blitz_flag" {
		flagger := n.A
		if n.ReportedBy == n.A {
			flagger = n.B
		}
		return "Blitz report flagged", fmt.Sprintf("%s: %s says %s's report (%s) is wrong: %s. %s",
			where, esc(flagger), esc(n.ReportedBy), how, esc(n.FlagReason), link)
	}
	title := "Blitz result reported"
	if n.Corrected {
		title = "Blitz result corrected"
	}
	return title, fmt.Sprintf("%s: %s, reported by %s. %s", where, how, esc(n.ReportedBy), link)
}

// prefixed puts e's prefix, if any, before text.
func prefixed(e store.NotificationEndpoint, text string) string {
	if e.Prefix == "" {
		return text
	}
	return discordbot.EscapeMarkdown(e.Prefix) + " " + text
}

// WebhookMessage is what a webhook is sent: text, embeds, or both, with the name and picture it shows (the
// webhook's own when empty), the thread it opens in a forum channel, and the roles it may ping.
type WebhookMessage struct {
	Content    string   `json:"content,omitempty"`
	Embeds     []Embed  `json:"embeds,omitempty"`
	Username   string   `json:"username,omitempty"`
	AvatarURL  string   `json:"avatar_url,omitempty"`
	ThreadName string   `json:"thread_name,omitempty"`
	Roles      []string `json:"-"`
}

// Embed is a Discord embed: a framed card with a heading (its author), a linked title, text, fields and pictures.
type Embed struct {
	Author      *EmbedName   `json:"author,omitempty"`
	Title       string       `json:"title,omitempty"`
	URL         string       `json:"url,omitempty"`
	Description string       `json:"description,omitempty"`
	Color       int          `json:"color,omitempty"`
	Fields      []EmbedField `json:"fields,omitempty"`
	Image       *EmbedImage  `json:"image,omitempty"`
	Thumbnail   *EmbedImage  `json:"thumbnail,omitempty"`
	Timestamp   string       `json:"timestamp,omitempty"`
}

// EmbedName is an embed's heading.
type EmbedName struct {
	Name string `json:"name"`
}

// EmbedField is a named value in an embed.
type EmbedField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// EmbedImage is a picture in an embed, by its address.
type EmbedImage struct {
	URL string `json:"url"`
}

// Discord's limits on an embed's parts and a thread's name, in characters.
const (
	maxEmbedTitle = 256
	maxEmbedText  = 4096
	maxFieldValue = 1024
	maxThreadName = 100
)

// ShortText is the most characters of a post's text a feed showing its start shows.
const ShortText = 300

// testHeading heads admin's test message, and names the thread it opens in a forum channel.
const testHeading = "Test from ttacommunity admin"

// redditKinds name each kind of post in an embed's heading.
var redditKinds = map[string]string{"text": "post", "image": "image post", "video": "video post", "link": "link post"}

// Payload is n as e is sent it: an embed in e's colour, a Reddit post's in e's look and any other headed by what
// it is; under e's prefix and role, shown with e's name and picture, and opening a thread when e is a forum
// channel's.
func Payload(n store.Notification, e store.NotificationEndpoint, admin string) WebhookMessage {
	if n.Kind != "reddit_post" {
		heading, body := parts(n, admin)
		return shaped(e, Embed{Title: heading, Description: body, Color: colour(e.Look.Colour)}, heading)
	}
	return shaped(e, redditEmbed(n.Reddit, e.Look), n.Reddit.Post.Title)
}

// TestPayload is what admin's Test button sends e: TestText, shaped as e shapes every message.
func TestPayload(e store.NotificationEndpoint) WebhookMessage {
	return shaped(e, Embed{Title: testHeading, Description: TestText, Color: colour(e.Look.Colour)}, testHeading)
}

// shaped is embed as e sends it: under e's prefix and its role's ping, with e's name and picture, and opening a
// thread named thread when e is a forum channel's.
func shaped(e store.NotificationEndpoint, embed Embed, thread string) WebhookMessage {
	m := WebhookMessage{Embeds: []Embed{embed}, Username: e.Username, AvatarURL: e.AvatarURL}
	var ping string
	if e.RoleID != "" {
		ping = "<@&" + e.RoleID + ">"
		m.Roles = []string{e.RoleID}
	}
	m.Content = strings.TrimSpace(prefixed(e, ping))
	if e.Forum {
		m.ThreadName = cut(strings.TrimSpace(thread), maxThreadName)
	}
	return m
}

// redditEmbed is a Reddit post as Mee6 showed one, in look: colour, how much text, image and author.
func redditEmbed(r store.RedditNotice, look store.EmbedLook) Embed {
	p := r.Post
	embed := Embed{
		Author:    &EmbedName{Name: fmt.Sprintf("New %s on /r/%s", redditKinds[p.Kind], r.Subreddit)},
		Title:     cut(discordbot.EscapeMarkdown(p.Title), maxEmbedTitle),
		URL:       p.URL,
		Color:     colour(look.Colour),
		Timestamp: p.PublishedAt.UTC().Format(time.RFC3339),
	}
	switch look.Text {
	case "full":
		embed.Description = cut(p.Body, maxEmbedText)
	case "short":
		embed.Description = atWord(p.Body, ShortText)
	}
	if look.Author && p.Author != "" {
		embed.Fields = []EmbedField{{Name: "Post Author", Value: cut("/u/"+discordbot.EscapeMarkdown(p.Author), maxFieldValue)}}
	}
	// Reddit's feed has a full image only for i.redd.it; its other previews are 140 pixels wide.
	switch {
	case !look.Image:
	case strings.HasPrefix(p.ImageURL, "https://i.redd.it/"):
		embed.Image = &EmbedImage{URL: p.ImageURL}
	case p.ImageURL != "":
		embed.Thumbnail = &EmbedImage{URL: p.ImageURL}
	}
	return embed
}

// colour is #rrggbb as Discord takes it, or Reddit's orange when it is not one.
func colour(hex string) int {
	if v, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32); err == nil && len(hex) == 7 && hex[0] == '#' {
		return int(v)
	}
	return redditOrange
}

// redditOrange is a Reddit embed's colour when its endpoint has none.
const redditOrange = 0xFF4500

// atWord shortens s to at most n characters at a word's end, ending with … when it was longer.
func atWord(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	short := string(r[:n-1])
	if i := strings.LastIndexAny(short, " \n"); i > n/2 {
		short = short[:i]
	}
	return strings.TrimSpace(short) + "…"
}

// cut shortens s to at most n characters, ending with … when it was longer.
func cut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// Discord posts messages to Discord webhooks.
type Discord struct{ hc *http.Client }

// NewDiscord returns a poster using hc, whose timeout bounds each post.
func NewDiscord(hc *http.Client) *Discord { return &Discord{hc: hc} }

// Post posts content, cut to Discord's limit, to the webhook at webhook, allowing no mentions. It returns a
// *RefusedError when Discord answers other than success; no error holds the URL, whose path is the webhook's secret.
func (d *Discord) Post(ctx context.Context, webhook, content string) error {
	return d.Send(ctx, webhook, WebhookMessage{Content: content})
}

// Send posts m, its text cut to Discord's limit, to the webhook at webhook, as Post does, allowing only m's roles
// to be pinged.
func (d *Discord) Send(ctx context.Context, webhook string, m WebhookMessage) error {
	if r := []rune(m.Content); len(r) > maxContent {
		m.Content = string(r[:maxContent])
	}
	mentions := map[string][]string{"parse": {}}
	if len(m.Roles) > 0 {
		mentions["roles"] = m.Roles
	}
	body, err := json.Marshal(struct {
		WebhookMessage
		AllowedMentions map[string][]string `json:"allowed_mentions"`
	}{m, mentions})
	if err != nil {
		return fmt.Errorf("writing a Discord message: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(body))
	if err != nil {
		return errors.New("posting to Discord: not a URL")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.hc.Do(req)
	if err != nil {
		var u *url.Error
		if errors.As(err, &u) {
			err = u.Err
		}
		return fmt.Errorf("posting to Discord: %s", strings.ReplaceAll(err.Error(), req.URL.Host, "Discord"))
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &RefusedError{Status: resp.StatusCode}
	}
	return nil
}
