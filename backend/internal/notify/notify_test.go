package notify_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/notify"
	"ttacommunity/backend/internal/store"
)

const admin = "https://admin.example.test"

func blitz(kind, result string) store.Notification {
	return store.Notification{Kind: kind, Slug: "us_open", Series: "US Open", Edition: "2026", Round: 2, A: "Ann", B: "Bob",
		Result: result, ReportedBy: "Ann", FlagReason: "I won on culture"}
}

// text is n as a plain endpoint's embed says it: its heading in bold, then its text.
func text(n store.Notification) string {
	e := notify.Payload(n, store.NotificationEndpoint{}, admin).Embeds[0]
	return "**" + e.Title + "**: " + e.Description
}

func TestText(t *testing.T) {
	corrected := blitz("blitz_report", "b")
	corrected.Corrected = true
	byBob := blitz("blitz_flag", "draw")
	byBob.ReportedBy = "Bob"
	tests := []struct {
		name string
		n    store.Notification
		want string
	}{
		{"a new player linked automatically", store.Notification{Kind: "sign_up", Name: "Dee", SignIns: []string{"discord"}, LinkedHow: "match"},
			"**New player**: someone signed in with Discord for the first time and was linked automatically to **Dee**. <" + admin + "/#members>"},
		{"a new player not linked", store.Notification{Kind: "sign_up", SignIns: []string{"email"}},
			"**New player**: someone signed in with email for the first time and is not linked to a TTA username yet. <" + admin + "/#members>"},
		{"a new player an admin linked before this was sent", store.Notification{Kind: "sign_up", Name: "Dee", SignIns: []string{"google"}, LinkedHow: "approved"},
			"**New player**: someone signed in with Google for the first time and was linked by an admin to **Dee**. <" + admin + "/#members>"},
		{"a link request", store.Notification{Kind: "link_request", Name: "Eve", SignIns: []string{"discord"}},
			"**Link request**: a player who signs in with Discord asks to be linked to **Eve**. Answer it in admin: <" + admin + "/#members>"},
		{"a link request from an account with several sign-ins", store.Notification{Kind: "link_request", Name: "Eve", SignIns: []string{"discord", "email", "google"}},
			"**Link request**: a player who signs in with Discord, email and Google asks to be linked to **Eve**. Answer it in admin: <" + admin + "/#members>"},
		{"a report", blitz("blitz_report", "a"),
			"**Blitz result reported**: US Open 2026, round 2: Ann beat Bob, reported by Ann. <" + admin + "/blitz/?t=us_open&e=2026>"},
		{"a correction", corrected,
			"**Blitz result corrected**: US Open 2026, round 2: Bob beat Ann, reported by Ann. <" + admin + "/blitz/?t=us_open&e=2026>"},
		{"a double loss", blitz("blitz_report", "double_loss"),
			"**Blitz result reported**: US Open 2026, round 2: Ann and Bob both lost, reported by Ann. <" + admin + "/blitz/?t=us_open&e=2026>"},
		{"a flag", blitz("blitz_flag", "a"),
			"**Blitz report flagged**: US Open 2026, round 2: Bob says Ann's report (Ann beat Bob) is wrong: I won on culture. <" + admin + "/blitz/?t=us_open&e=2026>"},
		{"a flag of the second player's report", byBob,
			"**Blitz report flagged**: US Open 2026, round 2: Ann says Bob's report (Ann and Bob drew) is wrong: I won on culture. <" + admin + "/blitz/?t=us_open&e=2026>"},
		{"names written as typed, not as markdown or mentions", store.Notification{Kind: "link_request", Name: "@everyone <@100000000000000001> **x**", SignIns: []string{"email"}},
			`**Link request**: a player who signs in with email asks to be linked to **@everyone \<@100000000000000001\> \*\*x\*\***. Answer it in admin: <` + admin + "/#members>"},
		{"a feed failing, its reason gone by when it is sent", store.Notification{Kind: "feed_failed", FeedKind: "rss", FeedName: "Example news"},
			"**RSS feed failing**: Example news has failed its last 3 reads. <" + admin + "/#notifications>"},
		{"an RSS feed failing", store.Notification{Kind: "feed_failed", FeedKind: "rss", FeedName: "Example *news*", FeedError: "the feed answered 404"},
			`**RSS feed failing**: Example \*news\* has failed its last 3 reads: the feed answered 404. <` + admin + "/#notifications>"},
		{"an RSS feed reading again", store.Notification{Kind: "feed_recovered", FeedKind: "rss", FeedName: "https://news.example.com/feed"},
			`**RSS feed reading again**: https://news.example.com/feed reads again; its new posts are posted. <` + admin + "/#notifications>"},
		{"an edition's address escaped", store.Notification{Kind: "blitz_report", Slug: "us_open", Series: "US Open", Edition: "Season 1 & 2", Round: 1, A: "Ann", B: "Bob", Result: "a", ReportedBy: "Ann"},
			"**Blitz result reported**: US Open Season 1 & 2, round 1: Ann beat Bob, reported by Ann. <" + admin + "/blitz/?t=us_open&e=Season+1+%26+2>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := text(tt.n); got != tt.want {
				t.Errorf("Text(%+v)\n got = %q\nwant %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestPayloadPutsTheEndpointsPrefixAsTheMessagesText(t *testing.T) {
	n := store.Notification{Kind: "link_request", Name: "Eve", SignIns: []string{"discord"}}
	for _, tt := range []struct{ prefix, want string }{{"", ""}, {"[dev]", `\[dev\]`}} {
		e := store.NotificationEndpoint{Prefix: tt.prefix}
		if got := notify.Payload(n, e, admin).Content; got != tt.want {
			t.Errorf("Payload with prefix %q: text = %q, want %q", tt.prefix, got, tt.want)
		}
		if got := notify.TestPayload(e).Content; got != tt.want {
			t.Errorf("TestPayload with prefix %q: text = %q, want %q", tt.prefix, got, tt.want)
		}
	}
}

// discord is a stub Discord webhook answering status, recording the body it was sent.
func discord(t *testing.T, status int, got *map[string]any) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("request = %s %s, want POST of application/json", r.Method, r.Header.Get("Content-Type"))
		}
		if err := json.Unmarshal(body, got); err != nil {
			t.Errorf("body %q is not JSON: %v", body, err)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestPostSendsTheMessageWithNoMentionsAllowed(t *testing.T) {
	var got map[string]any
	s := discord(t, http.StatusNoContent, &got)
	if err := notify.NewDiscord(s.Client()).Send(context.Background(), s.URL+"/api/webhooks/1/token", notify.WebhookMessage{Content: "hello @everyone"}); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if got["content"] != "hello @everyone" {
		t.Errorf("content = %v, want the message", got["content"])
	}
	mentions, _ := json.Marshal(got["allowed_mentions"])
	if string(mentions) != `{"parse":[]}` {
		t.Errorf("allowed_mentions = %s, want {\"parse\":[]}", mentions)
	}
}

func TestPostCutsAMessageToDiscordsLimit(t *testing.T) {
	var got map[string]any
	s := discord(t, http.StatusNoContent, &got)
	if err := notify.NewDiscord(s.Client()).Send(context.Background(), s.URL, notify.WebhookMessage{Content: strings.Repeat("é", 2500)}); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if n := len([]rune(got["content"].(string))); n != 2000 {
		t.Errorf("content is %d characters, want 2000", n)
	}
}

func TestPostFailsWithoutSayingTheURL(t *testing.T) {
	refusing := func(status int) string {
		var got map[string]any
		return discord(t, status, &got).URL + "/api/webhooks/1/secret-token"
	}
	tests := []struct {
		name, url  string
		wantStatus int
	}{
		{"a webhook that is gone", refusing(http.StatusNotFound), http.StatusNotFound},
		{"rate limited", refusing(http.StatusTooManyRequests), http.StatusTooManyRequests},
		{"Discord failing", refusing(http.StatusInternalServerError), http.StatusInternalServerError},
		{"nothing listening", "http://127.0.0.1:1/api/webhooks/1/secret-token", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := notify.NewDiscord(&http.Client{Timeout: 5 * time.Second}).Send(context.Background(), tt.url, notify.WebhookMessage{Content: "hello"})
			if err == nil {
				t.Fatal("Post: nil, want an error")
			}
			if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "127.0.0.1") {
				t.Errorf("error %q holds the URL", err)
			}
			var refused *notify.RefusedError
			if got := errors.As(err, &refused); got != (tt.wantStatus != 0) || (got && refused.Status != tt.wantStatus) {
				t.Errorf("error = %v, want a RefusedError of status %d only when Discord answered", err, tt.wantStatus)
			}
		})
	}
}

func urlOf(i *notify.EmbedImage) string {
	if i == nil {
		return ""
	}
	return i.URL
}

func TestOtherNotificationsAreEmbedsHeadedByWhatTheyAre(t *testing.T) {
	e := store.NotificationEndpoint{Look: store.EmbedLook{Colour: "#123abc"}}
	for _, n := range []store.Notification{
		{Kind: "link_request", Name: "Eve", SignIns: []string{"discord"}},
		{Kind: "feed_failed", FeedKind: "rss", FeedName: "Example news", FeedError: "the feed answered 403"},
	} {
		p := notify.Payload(n, e, admin)
		if len(p.Embeds) != 1 {
			t.Fatalf("%s: payload = %+v, want one embed", n.Kind, p)
		}
		got := p.Embeds[0]
		if "**"+got.Title+"**: "+got.Description != text(n) || got.Color != 0x123abc {
			t.Errorf("%s: embed = %q / %q in %x, want Text's heading and body in the endpoint's colour", n.Kind, got.Title, got.Description, got.Color)
		}
	}
	if got := notify.TestPayload(e).Embeds[0]; got.Description != notify.TestText || got.Color != 0x123abc {
		t.Errorf("test embed = %q in %x, want TestText in the endpoint's colour", got.Description, got.Color)
	}
}

func TestSendPostsEmbedsWithNoMentionsAllowed(t *testing.T) {
	var got map[string]any
	s := discord(t, http.StatusNoContent, &got)
	p := notify.Payload(rssItem("A *bold* claim", ""), store.NotificationEndpoint{}, admin)
	if err := notify.NewDiscord(s.Client()).Send(context.Background(), s.URL, p); err != nil {
		t.Fatalf("Send: %v", err)
	}
	embeds, _ := got["embeds"].([]any)
	if len(embeds) != 1 || embeds[0].(map[string]any)["title"] != `A \*bold\* claim` {
		t.Errorf("embeds = %v, want the post's", got["embeds"])
	}
	if _, ok := got["content"]; ok {
		t.Errorf("content = %v, want none sent", got["content"])
	}
	if mentions, _ := json.Marshal(got["allowed_mentions"]); string(mentions) != `{"parse":[]}` {
		t.Errorf("allowed_mentions = %s, want {\"parse\":[]}", mentions)
	}
}

func TestAnEndpointsNameAvatarForumAndRoleShapeEveryMessage(t *testing.T) {
	e := store.NotificationEndpoint{Prefix: "[dev]", Username: "TTA News", AvatarURL: "https://example.com/a.png", Forum: true, RoleID: "100000000000000002"}
	tests := []struct {
		name, wantThread string
		p                notify.WebhookMessage
	}{
		{"a feed item", "A *bold* claim", notify.Payload(rssItem("A *bold* claim", ""), e, admin)},
		{"a link request", "Link request", notify.Payload(store.Notification{Kind: "link_request", Name: "Eve", SignIns: []string{"discord"}}, e, admin)},
		{"a test", "Test from ttacommunity admin", notify.TestPayload(e)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.p
			if p.Username != "TTA News" || p.AvatarURL != "https://example.com/a.png" || p.ThreadName != tt.wantThread {
				t.Errorf("name, avatar, thread = %q, %q, %q; want the endpoint's, thread %q", p.Username, p.AvatarURL, p.ThreadName, tt.wantThread)
			}
			if !strings.HasPrefix(p.Content, `\[dev\] <@&100000000000000002>`) || !slices.Equal(p.Roles, []string{"100000000000000002"}) {
				t.Errorf("content = %q, roles = %v; want the prefix, then the role pinged and allowed", p.Content, p.Roles)
			}
		})
	}
	plain := notify.Payload(rssItem("A title", ""), store.NotificationEndpoint{}, admin)
	if plain.ThreadName != "" || plain.Username != "" || len(plain.Roles) != 0 {
		t.Errorf("a plain endpoint's payload = %+v, want no thread, name or role", plain)
	}
}

func TestSendAllowsOnlyTheRoleItPings(t *testing.T) {
	var got map[string]any
	s := discord(t, http.StatusNoContent, &got)
	p := notify.WebhookMessage{Content: "<@&100000000000000002> hi @everyone", Roles: []string{"100000000000000002"}, ThreadName: "Hi", Username: "TTA"}
	if err := notify.NewDiscord(s.Client()).Send(context.Background(), s.URL, p); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if mentions, _ := json.Marshal(got["allowed_mentions"]); string(mentions) != `{"parse":[],"roles":["100000000000000002"]}` {
		t.Errorf("allowed_mentions = %s, want only that role", mentions)
	}
	if got["thread_name"] != "Hi" || got["username"] != "TTA" {
		t.Errorf("thread_name, username = %v, %v; want Hi, TTA", got["thread_name"], got["username"])
	}
}

func TestAForumPostPutsItsTextOnTheCard(t *testing.T) {
	forum := store.NotificationEndpoint{Forum: true, Prefix: "[dev]", Look: store.DefaultLook}
	tests := []struct {
		name        string
		look        store.EmbedLook
		wantContent string
	}{
		{"its text", store.DefaultLook, `\[dev\] Hello`},
		{"no text shown", store.EmbedLook{Colour: "#ff4500", Text: "none", Image: true}, `\[dev\]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := forum
			e.Look = tt.look
			n := rssItem("A title", "")
			n.RSS.Item.Summary = "Hello"
			p := notify.Payload(n, e, admin)
			if p.Content != tt.wantContent || p.Embeds[0].Description != "" {
				t.Errorf("text = %q, embed text = %q; want %q on the card and none in the embed", p.Content, p.Embeds[0].Description, tt.wantContent)
			}
		})
	}
	n := store.Notification{Kind: "link_request", Name: "Eve", SignIns: []string{"discord"}}
	p := notify.Payload(n, forum, admin)
	if !strings.HasPrefix(p.Content, `\[dev\] a player who signs in`) || p.Embeds[0].Description != "" || p.Embeds[0].Title != "Link request" {
		t.Errorf("a forum's link request = %q with embed %+v, want its body on the card and its heading in the embed", p.Content, p.Embeds[0])
	}
}

func TestAForumPostsLongTextIsCutAtAWordWithinDiscordsLimit(t *testing.T) {
	n := rssItem("A title", "")
	n.RSS.Item.Summary = strings.Repeat("word ", 700)
	e := store.NotificationEndpoint{Forum: true, Prefix: "[dev]", RoleID: "100000000000000002", Look: store.DefaultLook}
	p := notify.Payload(n, e, admin)
	if got := []rune(p.Content); len(got) > 2000 || !strings.HasSuffix(p.Content, "word…") || !strings.HasPrefix(p.Content, `\[dev\] <@&100000000000000002> word`) {
		t.Errorf("text is %d characters ending %q, want at most 2000, the prefix and ping first, cut at a word", len(got), string(got[len(got)-8:]))
	}
}

func rssItem(title, image string) store.Notification {
	return store.Notification{Kind: "rss_item", FeedID: 5, RSS: store.RSSNotice{Feed: "Example news", Item: store.RSSItem{
		ItemID: "item-1", Title: title, URL: "https://news.example.com/1", Summary: `A long summary of the news, \*escaped\* already`,
		ImageURL: image, PublishedAt: time.Date(2026, 11, 18, 1, 0, 0, 0, time.UTC),
	}}}
}

func TestAnRSSItemIsACardHeadedByItsFeedInItsEndpointsLook(t *testing.T) {
	const image = "https://cdn.example.com/a.jpg"
	for _, tt := range []struct {
		name      string
		look      store.EmbedLook
		wantText  string
		wantImage string
	}{
		{"all of it", store.EmbedLook{Colour: "#123abc", Text: "full", Image: true}, `A long summary of the news, \*escaped\* already`, image},
		{"its start", store.EmbedLook{Colour: "#123abc", Text: "short", Image: true}, `A long summary of the news, \*escaped\* already`, image},
		{"no text or image", store.EmbedLook{Colour: "#123abc", Text: "none"}, "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := notify.Payload(rssItem("A *bold* title", image), store.NotificationEndpoint{Look: tt.look}, admin)
			if len(p.Embeds) != 1 {
				t.Fatalf("payload = %+v, want one embed", p)
			}
			e := p.Embeds[0]
			if e.Author == nil || e.Author.Name != "New on Example news" || e.Title != `A \*bold\* title` || e.URL != "https://news.example.com/1" || e.Color != 0x123abc {
				t.Errorf("embed = %+v, want headed New on Example news, the title escaped, linking the item, in the endpoint's colour", e)
			}
			if e.Description != tt.wantText || urlOf(e.Image) != tt.wantImage {
				t.Errorf("text, image = %q, %q; want %q, %q", e.Description, urlOf(e.Image), tt.wantText, tt.wantImage)
			}
		})
	}
}

func TestAnRSSItemInAForumOpensAThreadByItsTitleOrElseItsFeed(t *testing.T) {
	forum := store.NotificationEndpoint{Forum: true, Look: store.DefaultLook}
	for _, tt := range []struct{ title, want string }{
		{"A title", "A title"},
		{"", "Example news"},
	} {
		p := notify.Payload(rssItem(tt.title, "https://cdn.example.com/a.jpg"), forum, admin)
		if p.ThreadName != tt.want || urlOf(p.Embeds[0].Image) != "https://cdn.example.com/a.jpg" {
			t.Errorf("title %q: thread %q, image %v; want thread %q and the image in the embed", tt.title, p.ThreadName, p.Embeds[0].Image, tt.want)
		}
	}
}
