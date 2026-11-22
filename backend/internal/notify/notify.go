// Package notify writes notifications as Discord messages and posts them to Discord webhooks. A message names
// players as typed, never as markdown, and links to where admin answers it; only a blitz round tags its players, by
// the Discord ids of their sign-ins.
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
	"strconv"
	"strings"
	"time"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/discordbot"
	"ttacommunity/backend/internal/store"
)

// maxContent is the most characters Discord takes in a message, and maxMentions the most roles or users it lets
// one ping.
const (
	maxContent  = 2000
	maxMentions = 100
)

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

// parts is n's heading, such as Link request, and what it tells, linking to where admin, at origin admin, answers
// it.
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
	case "feed_failed":
		why := ""
		if n.FeedError != "" {
			why = ": " + esc(n.FeedError)
		}
		return feedKinds[n.FeedKind] + " failing", fmt.Sprintf("%s has failed its last %d reads%s. <%s/#notifications>",
			feedNamed(n), store.FeedFailuresNotified, why, admin)
	case "feed_recovered":
		return feedKinds[n.FeedKind] + " reading again", fmt.Sprintf("%s reads again; its new posts are posted. <%s/#notifications>", feedNamed(n), admin)
	}
	where := fmt.Sprintf("%s %s, round %d", esc(n.Series), esc(n.Edition), n.Round)
	if n.Game > 0 {
		where += fmt.Sprintf(", game %d", n.Game)
	}
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

// feedKinds name each kind of feed that can fail.
var feedKinds = map[string]string{"rss": "RSS feed"}

// feedNamed is a failing feed's name as a message shows it: its title or address.
func feedNamed(n store.Notification) string { return discordbot.EscapeMarkdown(n.FeedName) }

// prefixed puts e's prefix, if any, before text.
func prefixed(e store.NotificationEndpoint, text string) string {
	if e.Prefix == "" {
		return text
	}
	return discordbot.EscapeMarkdown(e.Prefix) + " " + text
}

// WebhookMessage is what a webhook is sent: text, embeds, or both, with the name and picture it shows (the
// webhook's own when empty), the thread it opens in a forum channel or the existing thread it goes into (ThreadID),
// and the roles and users (by Discord id) it may ping.
type WebhookMessage struct {
	Content    string   `json:"content,omitempty"`
	Embeds     []Embed  `json:"embeds,omitempty"`
	Username   string   `json:"username,omitempty"`
	AvatarURL  string   `json:"avatar_url,omitempty"`
	ThreadName string   `json:"thread_name,omitempty"`
	ThreadID   string   `json:"-"`
	Roles      []string `json:"-"`
	Users      []string `json:"-"`
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

// shortText is the most characters of a post's text an endpoint showing its start shows.
const shortText = 300

// testHeading heads admin's test message, and names the thread it opens in a forum channel.
const testHeading = "Test from ttacommunity admin"

// Payload is n as e is sent it: an embed in e's colour, an RSS item's in e's look, a run's news linking the
// leaderboard, a blitz round or podium as text tagging its players, a Twitch or YouTube stream's linking it (a stream ended is the edit of its post, its embed only) and
// any other headed by what it is; under e's prefix and role, shown with e's name and picture, and opening a thread
// when e is a forum channel's.
func Payload(n store.Notification, e store.NotificationEndpoint, admin string) WebhookMessage {
	switch n.Kind {
	case "twitch_live":
		t := n.Twitch
		return shaped(e, twitchEmbed(t, e.Look), t.DisplayName+" is live: "+t.Title)
	case "twitch_ended":
		return WebhookMessage{Embeds: []Embed{twitchEmbed(n.Twitch, e.Look)}}
	case "youtube_live":
		y := n.YouTube
		return shaped(e, youtubeEmbed(y, e.Look), y.Channel+" is live: "+y.Title)
	case "youtube_ended":
		return WebhookMessage{Embeds: []Embed{youtubeEmbed(n.YouTube, e.Look)}}
	case "ratings_run", "ratings_top10":
		embed := runEmbed(n, e.Look)
		return shaped(e, embed, embed.Title)
	case "blitz_round", "blitz_reminder", "blitz_finished", "blitz_sign_ups":
		text := roundText
		switch n.Kind {
		case "blitz_finished":
			text = podiumText
		case "blitz_sign_ups":
			text = signUpsText
		}
		title, body, users := text(n)
		m := texted(e, body, title)
		m.Users = users
		return m
	case "rss_item":
		thread := n.RSS.Item.Title
		if strings.TrimSpace(thread) == "" {
			thread = n.RSS.Feed
		}
		return shaped(e, rssEmbed(n.RSS, e.Look), thread)
	}
	heading, body := parts(n, admin)
	return shaped(e, Embed{Title: heading, Description: body, Color: colour(e.Look.Colour)}, heading)
}

// TestPayload is what admin's Test button sends e: TestText, shaped as e shapes every message.
func TestPayload(e store.NotificationEndpoint) WebhookMessage {
	return shaped(e, Embed{Title: testHeading, Description: TestText, Color: colour(e.Look.Colour)}, testHeading)
}

// shaped is embed as e sends it: under e's prefix and its role's ping, with e's name and picture. Into e's thread
// when it has one; else, for a forum channel, it opens a thread named thread, and the embed's text moves to the
// message, which the forum's card shows.
func shaped(e store.NotificationEndpoint, embed Embed, thread string) WebhookMessage {
	m := texted(e, "", thread)
	m.Embeds = []Embed{embed}
	head := m.Content
	if e.Forum && e.ThreadID == "" {
		m.ThreadName = cut(strings.TrimSpace(thread), maxThreadName)
		room := maxContent - len([]rune(head)) - 1
		m.Content = strings.TrimSpace(head + " " + discordbot.CutAtWord(m.Embeds[0].Description, room))
		m.Embeds[0].Description = ""
	}
	return m
}

// texted is text as e sends it, with no card: under e's prefix and its role's ping, with e's name and picture, into
// e's thread when it has one, else opening a thread named thread in a forum channel.
func texted(e store.NotificationEndpoint, text, thread string) WebhookMessage {
	m := WebhookMessage{Username: e.Username, AvatarURL: e.AvatarURL, ThreadID: e.ThreadID}
	var ping string
	if e.RoleID != "" {
		ping = "<@&" + e.RoleID + ">"
		m.Roles = []string{e.RoleID}
	}
	m.Content = strings.TrimSpace(strings.TrimSpace(prefixed(e, ping)) + "\n" + text)
	if e.Forum && e.ThreadID == "" {
		m.ThreadName = cut(strings.TrimSpace(thread), maxThreadName)
	}
	return m
}

// rssEmbed is an RSS item as a card in look: headed by its feed, its title linking the item, its summary as much as
// look shows, and its image when look shows images. Its summary is plain text already escaped.
func rssEmbed(r store.RSSNotice, look store.EmbedLook) Embed {
	it := r.Item
	embed := Embed{
		Author:    &EmbedName{Name: cut("New on "+r.Feed, maxEmbedTitle)},
		Title:     cut(discordbot.EscapeMarkdown(it.Title), maxEmbedTitle),
		URL:       it.URL,
		Color:     colour(look.Colour),
		Timestamp: it.PublishedAt.UTC().Format(time.RFC3339),
	}
	switch look.Text {
	case "full":
		embed.Description = cut(it.Summary, maxEmbedText)
	case "short":
		embed.Description = discordbot.CutAtWord(it.Summary, shortText)
	}
	if look.Image && it.ImageURL != "" {
		embed.Image = &EmbedImage{URL: it.ImageURL}
	}
	return embed
}

// twitchEmbed is stream t as a live stream's card, in look's colour: the streamer live, the title linking their
// channel, the category and the preview; once ended, the streamer was live, for how long, without the preview.
func twitchEmbed(t store.TwitchNotice, look store.EmbedLook) Embed {
	embed := Embed{
		Author:    &EmbedName{Name: t.DisplayName + " is live on Twitch"},
		Title:     cut(discordbot.EscapeMarkdown(t.Title), maxEmbedTitle),
		URL:       "https://www.twitch.tv/" + url.PathEscape(t.Login),
		Color:     colour(look.Colour),
		Timestamp: t.StartedAt.UTC().Format(time.RFC3339),
	}
	if t.Category != "" {
		embed.Fields = []EmbedField{{Name: "Category", Value: cut(discordbot.EscapeMarkdown(t.Category), maxFieldValue)}}
	}
	if t.EndedAt != nil {
		embed.Author.Name = t.DisplayName + " was live on Twitch"
		embed.Description = "Streamed for " + ran(t.EndedAt.Sub(t.StartedAt))
		return embed
	}
	// Discord keeps a picture it has fetched: a new address per stream shows this stream's preview.
	if t.ThumbnailURL != "" {
		embed.Image = &EmbedImage{URL: t.ThumbnailURL + "?t=" + strconv.FormatInt(t.StartedAt.Unix(), 10)}
	}
	return embed
}

// runEmbed is a published run's news as a card in look's colour, linking the leaderboard: for ratings_run, the
// games it covers and, with a run before, the titles gained and lost; for ratings_top10, a new number one and who
// came into and went out of the top 10.
func runEmbed(n store.Notification, look store.EmbedLook) Embed {
	r := n.Run
	if n.Kind == "ratings_run" {
		text := "Covers " + thousands(r.Games) + " games."
		if r.BeforeID != 0 {
			text += fmt.Sprintf(" Titles gained: %d · titles lost: %d", r.TitlesGained, r.TitlesLost)
		}
		return Embed{Title: "New ratings are out", URL: r.Link, Description: text, Color: colour(look.Colour)}
	}
	names := func(list []string) string {
		esc := make([]string, len(list))
		for i, s := range list {
			esc[i] = discordbot.EscapeMarkdown(s)
		}
		return strings.Join(esc, ", ")
	}
	var lines []string
	if c := n.Top10; c.NumberOne != "" {
		lines = append(lines, "New number one: **"+discordbot.EscapeMarkdown(c.NumberOne)+"**")
	}
	if len(n.Top10.In) > 0 {
		lines = append(lines, "Into the top 10: "+names(n.Top10.In))
	}
	if len(n.Top10.Out) > 0 {
		lines = append(lines, "Out of the top 10: "+names(n.Top10.Out))
	}
	return Embed{Title: "Top 10 changes", URL: r.Link, Description: cut(strings.Join(lines, "\n"), maxEmbedText), Color: colour(look.Colour)}
}

// places name a podium's places.
var places = map[int]string{1: "1st", 2: "2nd", 3: "3rd"}

// tagger names players as a blitz post shows them, and keeps whom it tagged: each by username, in bold when asked,
// with their tag after it in brackets when they signed in with Discord (Discord notifies only a tag in a message's
// text, never in a card).
type tagger struct{ users []string }

// name is p as a post shows them, bold or not.
func (t *tagger) name(p blitz.Person, bold bool) string {
	name := discordbot.EscapeMarkdown(p.Name)
	if bold {
		name = "**" + name + "**"
	}
	if tag, ok := blitz.TagOf(p); ok {
		t.users = append(t.users, p.DiscordID)
		return name + " (" + tag + ")"
	}
	return name
}

// heading is a blitz post's title as a bold link to its edition, without a preview. Discord shows a backslash
// escape inside a link's text as written, so the title is not escaped; only brackets, which would end it, become
// round ones.
func heading(title, link string) string {
	return "**[" + strings.NewReplacer("[", "(", "]", ")").Replace(title) + "](<" + link + ">)**"
}

// roundText is a published round as text, with no card (Discord shows a card below the text): its title, which also
// names it, as a bold link to the edition, then its deadline on each reader's clock once set, then its games, any
// third-place match and its byes, a line each; for a reminder (blitz_reminder), the games still to play. It returns the title, the text and
// whom it tags.
func roundText(n store.Notification) (title, text string, users []string) {
	b := n.Blitz
	title = n.Series + " " + n.Edition + " · " + b.Title
	switch {
	case n.Kind == "blitz_reminder":
		title += " reminder"
	case b.Again:
		title += ", paired again"
	}
	lines := []string{heading(title, b.Link)}
	if b.Deadline != nil {
		at := strconv.FormatInt(b.Deadline.Unix(), 10)
		lines = append(lines, "Deadline: <t:"+at+":F> (<t:"+at+":R>)")
	}
	section := "**Pairings**"
	if n.Kind == "blitz_reminder" {
		section = "**Still to play**"
	}
	lines = append(lines, section)
	var t tagger
	for _, g := range b.Games {
		lines = append(lines, t.name(g[0], false)+" vs "+t.name(g[1], false))
	}
	if g := b.ThirdPlace; g != nil {
		lines = append(lines, "Third-place match: "+t.name(g[0], false)+" vs "+t.name(g[1], false))
	}
	for _, p := range b.Byes {
		lines = append(lines, "Bye: "+t.name(p, false))
	}
	return title, strings.Join(lines, "\n"), t.users
}

// podiumText is a finished edition's final results as text: its title as a bold link to the edition, then a line
// per place, the winner in bold. It returns the title, the text and whom it tags.
func podiumText(n store.Notification) (title, text string, users []string) {
	title = n.Series + " " + n.Edition + " final results"
	lines := []string{heading(title, n.Blitz.Link)}
	var t tagger
	for _, p := range n.Blitz.Podium {
		who := make([]string, len(p.People))
		for i, person := range p.People {
			who[i] = t.name(person, p.Place == 1)
		}
		lines = append(lines, places[p.Place]+": "+list(who))
	}
	return title, strings.Join(lines, "\n"), t.users
}

// signUpsText is an edition's open sign-ups as text: its title as a bold link to the edition, its format, its
// closing time on each reader's clock when it has one, and a link to sign up. It returns the title and the text; it tags no
// one.
func signUpsText(n store.Notification) (title, text string, users []string) {
	title = n.Series + " " + n.Edition + " sign-ups are open"
	f := n.Blitz.Format
	format := strconv.Itoa(f.SwissRounds) + " Swiss round"
	if f.SwissRounds != 1 {
		format += "s"
	}
	if f.KnockoutSize > 0 {
		format += ", then a knockout of the top " + strconv.Itoa(f.KnockoutSize)
	}
	lines := []string{heading(title, n.Blitz.Link), "Format: " + format}
	if c := n.Blitz.Closes; c != nil {
		at := strconv.FormatInt(c.Unix(), 10)
		lines = append(lines, "Sign-ups close: <t:"+at+":F> (<t:"+at+":R>)")
	}
	lines = append(lines, "[Sign up on the site](<"+n.Blitz.Link+">) to enter.")
	return title, strings.Join(lines, "\n"), nil
}

// thousands is n with commas between its thousands: 205,123.
func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// youtubeEmbed is YouTube stream y as a live stream's card, in look's colour: the channel live, the title linking
// the video, and its picture; once ended, the channel was live, for how long, without the picture.
func youtubeEmbed(y store.YouTubeNotice, look store.EmbedLook) Embed {
	embed := Embed{
		Author:    &EmbedName{Name: cut(y.Channel+" is live on YouTube", maxEmbedTitle)},
		Title:     cut(discordbot.EscapeMarkdown(y.Title), maxEmbedTitle),
		URL:       "https://www.youtube.com/watch?v=" + url.QueryEscape(y.VideoID),
		Color:     colour(look.Colour),
		Timestamp: y.StartedAt.UTC().Format(time.RFC3339),
	}
	if y.EndedAt != nil {
		embed.Author.Name = cut(y.Channel+" was live on YouTube", maxEmbedTitle)
		embed.Description = "Streamed for " + ran(y.EndedAt.Sub(y.StartedAt))
		return embed
	}
	// Discord keeps a picture it has fetched: a new address per stream shows the picture as it is now.
	if y.ThumbnailURL != "" {
		embed.Image = &EmbedImage{URL: y.ThumbnailURL + "?t=" + strconv.FormatInt(y.StartedAt.Unix(), 10)}
	}
	return embed
}

// ran is d as "2 h 5 min", "45 min" or "3 h".
func ran(d time.Duration) string {
	h, m := int(d.Hours()), int(d.Minutes())%60
	switch {
	case h == 0:
		return fmt.Sprintf("%d min", m)
	case m == 0:
		return fmt.Sprintf("%d h", h)
	}
	return fmt.Sprintf("%d h %d min", h, m)
}

// colour is #rrggbb as Discord takes it, or defaultColour when it is not one.
func colour(hex string) int {
	if v, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32); err == nil && len(hex) == 7 && hex[0] == '#' {
		return int(v)
	}
	return defaultColour
}

// defaultColour is an embed's colour when its endpoint has none: orange, the database's default.
const defaultColour = 0xFF4500

// cut shortens s to at most n characters, ending with … when it was longer.
func cut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// Discord posts messages to Discord webhooks.
type Discord struct {
	hc *http.Client
}

// NewDiscord returns a poster using hc, whose timeout bounds each post.
func NewDiscord(hc *http.Client) *Discord {
	return &Discord{hc: hc}
}

// Send posts m, its text cut to Discord's limit, to the webhook at webhook, allowing only m's roles and users to be
// pinged.
// It returns a *RefusedError when Discord answers other than success; no error holds the URL, whose path is the
// webhook's secret.
func (d *Discord) Send(ctx context.Context, webhook string, m WebhookMessage) error {
	_, err := d.post(ctx, webhook, m, false)
	return err
}

// Post is Send, returning the message Discord made and the channel or forum thread it is in.
func (d *Discord) Post(ctx context.Context, webhook string, m WebhookMessage) (store.DiscordPost, error) {
	return d.post(ctx, webhook, m, true)
}

// post sends m as Send does, waiting for and returning the message made when wait is true.
func (d *Discord) post(ctx context.Context, webhook string, m WebhookMessage, wait bool) (store.DiscordPost, error) {
	if r := []rune(m.Content); len(r) > maxContent {
		m.Content = string(r[:maxContent])
	}
	payload, err := withMentions(m)
	if err != nil {
		return store.DiscordPost{}, err
	}
	query := url.Values{}
	if m.ThreadID != "" {
		query.Set("thread_id", m.ThreadID)
	}
	if wait {
		query.Set("wait", "true")
	}
	if len(query) > 0 {
		webhook += "?" + query.Encode()
	}
	var made struct {
		ID        string `json:"id"`
		ChannelID string `json:"channel_id"`
	}
	var out any
	if wait {
		out = &made
	}
	if err := d.do(ctx, http.MethodPost, webhook, bytes.NewReader(payload), "application/json", out); err != nil {
		return store.DiscordPost{}, err
	}
	return store.DiscordPost{MessageID: made.ID, ChannelID: made.ChannelID}, nil
}

// Edit replaces the embeds of message p, posted to endpoint e, with m's, keeping its text and allowing no pings.
// A message of a forum endpoint or one with a thread is found in its thread.
func (d *Discord) Edit(ctx context.Context, e store.NotificationEndpoint, p store.DiscordPost, m WebhookMessage) error {
	payload, err := withMentions(WebhookMessage{Embeds: m.Embeds})
	if err != nil {
		return err
	}
	address := e.URL + "/messages/" + url.PathEscape(p.MessageID)
	if e.Forum || e.ThreadID != "" {
		address += "?" + url.Values{"thread_id": {p.ChannelID}}.Encode()
	}
	return d.do(ctx, http.MethodPatch, address, bytes.NewReader(payload), "application/json", nil)
}

// withMentions is m as JSON, allowing only m's roles and users to be pinged, at most Discord's 100 of each.
func withMentions(m WebhookMessage) ([]byte, error) {
	mentions := map[string][]string{"parse": {}}
	if len(m.Roles) > 0 {
		mentions["roles"] = m.Roles[:min(len(m.Roles), maxMentions)]
	}
	if len(m.Users) > 0 {
		users := slices.Compact(slices.Sorted(slices.Values(m.Users)))
		mentions["users"] = users[:min(len(users), maxMentions)]
	}
	payload, err := json.Marshal(struct {
		WebhookMessage
		AllowedMentions map[string][]string `json:"allowed_mentions"`
	}{m, mentions})
	if err != nil {
		return nil, fmt.Errorf("writing a Discord message: %w", err)
	}
	return payload, nil
}

// do sends body to address, decoding Discord's answer into out unless it is nil. It returns a *RefusedError when
// Discord answers other than success; no error holds the address, whose path is the webhook's secret.
func (d *Discord) do(ctx context.Context, method, address string, body io.Reader, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, address, body)
	if err != nil {
		return errors.New("posting to Discord: not a URL")
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := d.hc.Do(req)
	if err != nil {
		var u *url.Error
		if errors.As(err, &u) {
			err = u.Err
		}
		return fmt.Errorf("posting to Discord: %s", strings.ReplaceAll(err.Error(), req.URL.Host, "Discord"))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &RefusedError{Status: resp.StatusCode}
	}
	if out != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
			return errors.New("posting to Discord: its answer was not a message")
		}
	}
	return nil
}
