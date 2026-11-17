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

// Text is n as a Discord message, linking to where admin, at origin admin, answers it.
func Text(n store.Notification, admin string) string {
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
		return fmt.Sprintf("**New player**: someone signed in with %s for the first time and %s. <%s/#members>", list(ways), linked, admin)
	case "link_request":
		return fmt.Sprintf("**Link request**: a player who signs in with %s asks to be linked to **%s**. Answer it in admin: <%s/#members>",
			list(ways), esc(n.Name), admin)
	}
	where := fmt.Sprintf("%s %s, round %d", esc(n.Series), esc(n.Edition), n.Round)
	link := fmt.Sprintf("<%s/blitz/?t=%s&e=%s>", admin, url.QueryEscape(n.Slug), url.QueryEscape(n.Edition))
	how := outcome(n.Result, esc(n.A), esc(n.B))
	if n.Kind == "blitz_flag" {
		flagger := n.A
		if n.ReportedBy == n.A {
			flagger = n.B
		}
		return fmt.Sprintf("**Blitz report flagged**: %s: %s says %s's report (%s) is wrong: %s. %s",
			where, esc(flagger), esc(n.ReportedBy), how, esc(n.FlagReason), link)
	}
	title := "Blitz result reported"
	if n.Corrected {
		title = "Blitz result corrected"
	}
	return fmt.Sprintf("**%s**: %s: %s, reported by %s. %s", title, where, how, esc(n.ReportedBy), link)
}

// Message is n as e posts it: Text after e's prefix.
func Message(n store.Notification, e store.NotificationEndpoint, admin string) string {
	return prefixed(e, Text(n, admin))
}

// TestMessage is what admin's Test button posts to e: TestText after e's prefix.
func TestMessage(e store.NotificationEndpoint) string { return prefixed(e, TestText) }

// prefixed puts e's prefix, if any, before text.
func prefixed(e store.NotificationEndpoint, text string) string {
	if e.Prefix == "" {
		return text
	}
	return discordbot.EscapeMarkdown(e.Prefix) + " " + text
}

// WebhookMessage is what a webhook is sent: text, embeds, or both.
type WebhookMessage struct {
	Content string  `json:"content,omitempty"`
	Embeds  []Embed `json:"embeds,omitempty"`
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

// Discord's limits on an embed's parts, in characters.
const (
	maxEmbedTitle = 256
	maxEmbedText  = 4096
	maxFieldValue = 1024
)

// redditOrange is the embed's colour for a Reddit post.
const redditOrange = 0xFF4500

// redditKinds name each kind of post in an embed's heading.
var redditKinds = map[string]string{"text": "post", "image": "image post", "video": "video post", "link": "link post"}

// Payload is n as e is sent it: a Reddit post as an embed under e's prefix, anything else as Message's text.
func Payload(n store.Notification, e store.NotificationEndpoint, admin string) WebhookMessage {
	if n.Kind != "reddit_post" {
		return WebhookMessage{Content: Message(n, e, admin)}
	}
	r, p := n.Reddit, n.Reddit.Post
	embed := Embed{
		Author:      &EmbedName{Name: fmt.Sprintf("New %s on /r/%s", redditKinds[p.Kind], r.Subreddit)},
		Title:       cut(discordbot.EscapeMarkdown(p.Title), maxEmbedTitle),
		URL:         p.URL,
		Description: cut(p.Body, maxEmbedText),
		Color:       redditOrange,
		Timestamp:   p.PublishedAt.UTC().Format(time.RFC3339),
	}
	if p.Author != "" {
		embed.Fields = []EmbedField{{Name: "Post Author", Value: cut("/u/"+discordbot.EscapeMarkdown(p.Author), maxFieldValue)}}
	}
	// Reddit's feed has a full image only for i.redd.it; its other previews are 140 pixels wide.
	switch {
	case strings.HasPrefix(p.ImageURL, "https://i.redd.it/"):
		embed.Image = &EmbedImage{URL: p.ImageURL}
	case p.ImageURL != "":
		embed.Thumbnail = &EmbedImage{URL: p.ImageURL}
	}
	var prefix string
	if e.Prefix != "" {
		prefix = discordbot.EscapeMarkdown(e.Prefix)
	}
	return WebhookMessage{Content: prefix, Embeds: []Embed{embed}}
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

// Send posts m, its text cut to Discord's limit, to the webhook at webhook, as Post does.
func (d *Discord) Send(ctx context.Context, webhook string, m WebhookMessage) error {
	if r := []rune(m.Content); len(r) > maxContent {
		m.Content = string(r[:maxContent])
	}
	body, err := json.Marshal(struct {
		WebhookMessage
		AllowedMentions map[string][]string `json:"allowed_mentions"`
	}{m, map[string][]string{"parse": {}}})
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
