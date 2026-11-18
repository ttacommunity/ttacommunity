package notify_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
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
		{"a Reddit feed failing", store.Notification{Kind: "feed_failed", FeedKind: "reddit", FeedName: "some_sub", FeedError: "reddit answered 403"},
			`**Reddit feed failing**: r/some\_sub has failed its last 3 reads: reddit answered 403. <` + admin + "/#notifications>"},
		{"a Reddit feed failing, its reason gone by when it is sent", store.Notification{Kind: "feed_failed", FeedKind: "reddit", FeedName: "example"},
			"**Reddit feed failing**: r/example has failed its last 3 reads. <" + admin + "/#notifications>"},
		{"an RSS feed failing", store.Notification{Kind: "feed_failed", FeedKind: "rss", FeedName: "Example *news*", FeedError: "the feed answered 404"},
			`**RSS feed failing**: Example \*news\* has failed its last 3 reads: the feed answered 404. <` + admin + "/#notifications>"},
		{"an RSS feed reading again", store.Notification{Kind: "feed_recovered", FeedKind: "rss", FeedName: "https://news.example.com/feed"},
			`**RSS feed reading again**: https://news.example.com/feed reads again; its new posts are posted. <` + admin + "/#notifications>"},
		{"a Reddit feed reading again", store.Notification{Kind: "feed_recovered", FeedKind: "reddit", FeedName: "some_sub"},
			`**Reddit feed reading again**: r/some\_sub reads again; its new posts are posted. <` + admin + "/#notifications>"},
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

func redditPost(kind, image string) store.Notification {
	return store.Notification{Kind: "reddit_post", Reddit: store.RedditNotice{Subreddit: "example", Post: store.RedditPost{
		PostID: "t3_abc1", Kind: kind, Title: "A *bold* claim", URL: "https://www.reddit.com/r/example/comments/abc1/a/",
		Author: "some_user", Body: "Hello", ImageURL: image, PublishedAt: time.Date(2026, 11, 14, 10, 4, 38, 0, time.UTC),
	}}}
}

func TestARedditPostIsAnEmbedLikeMee6s(t *testing.T) {
	const preview = "https://preview.redd.it/abc.jpeg?width=140"
	tests := []struct {
		name, kind, image, wantHeading string
		wantImage, wantThumbnail       string
	}{
		{"a text post", "text", "", "New post on /r/example", "", ""},
		{"an image", "image", "https://i.redd.it/abc.jpeg", "New image post on /r/example", "https://i.redd.it/abc.jpeg", ""},
		{"an image with only a small preview, in the corner", "image", preview, "New image post on /r/example", "", preview},
		{"a gallery, its small preview in the corner", "gallery", preview, "New gallery post on /r/example", "", preview},
		{"a video", "video", preview, "New video post on /r/example", "", preview},
		{"a link", "link", preview, "New link post on /r/example", "", preview},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := notify.Payload(redditPost(tt.kind, tt.image), store.NotificationEndpoint{Look: store.DefaultLook}, admin)
			if p.Content != "" || len(p.Embeds) != 1 {
				t.Fatalf("payload = %+v, want one embed and no text", p)
			}
			e := p.Embeds[0]
			if e.Author == nil || e.Author.Name != tt.wantHeading {
				t.Errorf("heading = %+v, want %q", e.Author, tt.wantHeading)
			}
			if image, thumb := urlOf(e.Image), urlOf(e.Thumbnail); image != tt.wantImage || thumb != tt.wantThumbnail {
				t.Errorf("image, thumbnail = %q, %q; want %q, %q", image, thumb, tt.wantImage, tt.wantThumbnail)
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

func TestARedditEmbedLinksThePostAndNamesItsAuthor(t *testing.T) {
	p := notify.Payload(redditPost("text", ""), store.NotificationEndpoint{Prefix: "[dev]", Look: store.DefaultLook}, admin)
	e := p.Embeds[0]
	if p.Content != `\[dev\]` {
		t.Errorf("content = %q, want the prefix alone", p.Content)
	}
	if e.Title != `A \*bold\* claim` || e.URL != "https://www.reddit.com/r/example/comments/abc1/a/" || e.Description != "Hello" {
		t.Errorf("title, url, text = %q, %q, %q; want the post's, its title escaped", e.Title, e.URL, e.Description)
	}
	if len(e.Fields) != 1 || e.Fields[0].Name != "Post Author" || e.Fields[0].Value != `/u/some\_user` {
		t.Errorf("fields = %+v, want Post Author /u/some\\_user", e.Fields)
	}
	if e.Timestamp != "2026-11-14T10:04:38Z" {
		t.Errorf("timestamp = %q, want when it was posted", e.Timestamp)
	}
}

func TestARedditEmbedWithoutAnAuthorHasNoAuthorField(t *testing.T) {
	n := redditPost("text", "")
	n.Reddit.Post.Author = ""
	if e := notify.Payload(n, store.NotificationEndpoint{Look: store.DefaultLook}, admin).Embeds[0]; len(e.Fields) != 0 {
		t.Errorf("fields = %+v, want none", e.Fields)
	}
}

func TestOtherNotificationsAreEmbedsHeadedByWhatTheyAre(t *testing.T) {
	e := store.NotificationEndpoint{Look: store.EmbedLook{Colour: "#123abc"}}
	for _, n := range []store.Notification{
		{Kind: "link_request", Name: "Eve", SignIns: []string{"discord"}},
		{Kind: "feed_failed", FeedKind: "reddit", FeedName: "example", FeedError: "reddit answered 403"},
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
	p := notify.Payload(redditPost("text", ""), store.NotificationEndpoint{}, admin)
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

func TestARedditEmbedFollowsItsEndpointsLook(t *testing.T) {
	long := strings.Repeat("word ", 200)
	tests := []struct {
		name                 string
		look                 store.EmbedLook
		wantColour           int
		wantText             string
		wantImage, wantField bool
	}{
		{"all shown", store.EmbedLook{Colour: "#123abc", Text: "full", Image: true, Author: true}, 0x123abc, long, true, true},
		{"the start of the text", store.EmbedLook{Colour: "#ff4500", Text: "short", Image: true, Author: true}, 0xff4500, "short", true, true},
		{"no text, image or author", store.EmbedLook{Colour: "#ff4500", Text: "none"}, 0xff4500, "", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := redditPost("image", "https://i.redd.it/abc.jpeg")
			n.Reddit.Post.Body = long
			e := notify.Payload(n, store.NotificationEndpoint{Look: tt.look}, admin).Embeds[0]
			text := e.Description
			if tt.wantText == "short" {
				if n := len([]rune(text)); n > 300 || !strings.HasSuffix(text, "…") {
					t.Errorf("text is %d characters ending %q, want at most 300 ending …", n, text[max(0, len(text)-5):])
				}
			} else if text != tt.wantText {
				t.Errorf("text = %d characters, want %d", len(text), len(tt.wantText))
			}
			if e.Color != tt.wantColour || (e.Image != nil) != tt.wantImage || (len(e.Fields) > 0) != tt.wantField {
				t.Errorf("colour %x, image %t, author %t; want %x, %t, %t", e.Color, e.Image != nil, len(e.Fields) > 0, tt.wantColour, tt.wantImage, tt.wantField)
			}
		})
	}
}

func TestAnEndpointsNameAvatarForumAndRoleShapeEveryMessage(t *testing.T) {
	e := store.NotificationEndpoint{Prefix: "[dev]", Username: "TTA Reddit", AvatarURL: "https://example.com/a.png", Forum: true, RoleID: "100000000000000002"}
	tests := []struct {
		name, wantThread string
		p                notify.WebhookMessage
	}{
		{"a Reddit post", "A *bold* claim", notify.Payload(redditPost("text", ""), e, admin)},
		{"a link request", "Link request", notify.Payload(store.Notification{Kind: "link_request", Name: "Eve", SignIns: []string{"discord"}}, e, admin)},
		{"a test", "Test from ttacommunity admin", notify.TestPayload(e)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.p
			if p.Username != "TTA Reddit" || p.AvatarURL != "https://example.com/a.png" || p.ThreadName != tt.wantThread {
				t.Errorf("name, avatar, thread = %q, %q, %q; want the endpoint's, thread %q", p.Username, p.AvatarURL, p.ThreadName, tt.wantThread)
			}
			if !strings.HasPrefix(p.Content, `\[dev\] <@&100000000000000002>`) || !slices.Equal(p.Roles, []string{"100000000000000002"}) {
				t.Errorf("content = %q, roles = %v; want the prefix, then the role pinged and allowed", p.Content, p.Roles)
			}
		})
	}
	plain := notify.Payload(redditPost("text", ""), store.NotificationEndpoint{}, admin)
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

func TestAForumPostPutsItsTextOnTheCardAndItsImageUp(t *testing.T) {
	forum := store.NotificationEndpoint{Forum: true, Prefix: "[dev]", Look: store.DefaultLook}
	tests := []struct {
		name, image, wantUpload string
		look                    store.EmbedLook
		wantContent             string
	}{
		{"a text post", "", "", store.DefaultLook, `\[dev\] Hello`},
		{"an image post", "https://i.redd.it/abc.jpeg", "https://i.redd.it/abc.jpeg", store.DefaultLook, `\[dev\] Hello`},
		{"a gallery's preview", "https://preview.redd.it/abc.jpeg?width=140", "https://preview.redd.it/abc.jpeg?width=140", store.DefaultLook, `\[dev\] Hello`},
		{"an image the endpoint does not show", "https://i.redd.it/abc.jpeg", "", store.EmbedLook{Colour: "#ff4500", Text: "full"}, `\[dev\] Hello`},
		{"no text shown", "", "", store.EmbedLook{Colour: "#ff4500", Text: "none", Image: true}, `\[dev\]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := forum
			e.Look = tt.look
			p := notify.Payload(redditPost("image", tt.image), e, admin)
			embed := p.Embeds[0]
			if p.Content != tt.wantContent || embed.Description != "" {
				t.Errorf("text = %q, embed text = %q; want %q on the card and none in the embed", p.Content, embed.Description, tt.wantContent)
			}
			if p.Upload != tt.wantUpload || embed.Image != nil || embed.Thumbnail != nil {
				t.Errorf("upload = %q, image %v, thumbnail %v; want %q uploaded and no picture in the embed", p.Upload, embed.Image, embed.Thumbnail, tt.wantUpload)
			}
		})
	}
	n := store.Notification{Kind: "link_request", Name: "Eve", SignIns: []string{"discord"}}
	p := notify.Payload(n, forum, admin)
	if !strings.HasPrefix(p.Content, `\[dev\] a player who signs in`) || p.Embeds[0].Description != "" || p.Embeds[0].Title != "Link request" {
		t.Errorf("a forum's link request = %q with embed %+v, want its body on the card and its heading in the embed", p.Content, p.Embeds[0])
	}
}

// imageHost serves a small invented image at /abc.png, or status when not 200.
func imageHost(t *testing.T, status int) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(status)
		_, _ = w.Write([]byte("\x89PNG invented"))
	}))
	t.Cleanup(s.Close)
	return s
}

func TestSendUploadsTheImageItIsGiven(t *testing.T) {
	img := imageHost(t, http.StatusOK)
	var form *multipart.Form
	var contentType string
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err == nil {
			form = r.MultipartForm
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(hook.Close)
	d := notify.NewDiscord(hook.Client())
	d.ImageHosts = []string{strings.TrimPrefix(img.URL, "http://")}
	d.ImageScheme = "http"
	m := notify.WebhookMessage{Content: "Hello", Embeds: []notify.Embed{{Title: "A post"}}, ThreadName: "A post", Upload: img.URL + "/abc.png"}
	if err := d.Send(context.Background(), hook.URL, m); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.HasPrefix(contentType, "multipart/form-data") || form == nil || len(form.File["files[0]"]) != 1 {
		t.Fatalf("Discord got %q with form %v, want multipart with files[0]", contentType, form)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(form.Value["payload_json"][0]), &payload); err != nil || payload["thread_name"] != "A post" || payload["content"] != "Hello" {
		t.Errorf("payload_json = %v (%v), want the message with its thread", form.Value["payload_json"], err)
	}
	if f := form.File["files[0]"][0]; f.Filename != "image.png" {
		t.Errorf("file name = %q, want image.png", f.Filename)
	}
}

func TestSendPutsTheImageInTheEmbedWhenItCannotUploadIt(t *testing.T) {
	refusing := imageHost(t, http.StatusNotFound)
	for name, upload := range map[string]string{
		"an image host not allowed": "https://example.com/abc.png",
		"an image that is gone":     refusing.URL + "/abc.png",
	} {
		t.Run(name, func(t *testing.T) {
			var got map[string]any
			s := discord(t, http.StatusNoContent, &got)
			d := notify.NewDiscord(s.Client())
			d.ImageHosts = []string{strings.TrimPrefix(refusing.URL, "http://")}
			d.ImageScheme = "http"
			m := notify.WebhookMessage{Embeds: []notify.Embed{{Title: "A post"}}, Upload: upload}
			if err := d.Send(context.Background(), s.URL, m); err != nil {
				t.Fatalf("Send: %v", err)
			}
			embeds, _ := got["embeds"].([]any)
			thumb, _ := embeds[0].(map[string]any)["thumbnail"].(map[string]any)
			if thumb["url"] != upload {
				t.Errorf("embed thumbnail = %v, want %s, as it could not be uploaded and is not an i.redd.it image", thumb, upload)
			}
		})
	}
}

func TestAForumPostsLongTextIsCutAtAWordWithinDiscordsLimit(t *testing.T) {
	n := redditPost("text", "")
	n.Reddit.Post.Body = strings.Repeat("word ", 700)
	e := store.NotificationEndpoint{Forum: true, Prefix: "[dev]", RoleID: "100000000000000002", Look: store.DefaultLook}
	p := notify.Payload(n, e, admin)
	if got := []rune(p.Content); len(got) > 2000 || !strings.HasSuffix(p.Content, "word…") || !strings.HasPrefix(p.Content, `\[dev\] <@&100000000000000002> word`) {
		t.Errorf("text is %d characters ending %q, want at most 2000, the prefix and ping first, cut at a word", len(got), string(got[len(got)-8:]))
	}
}

func TestSendDoesNotFollowAnImagesRedirectToAnotherHost(t *testing.T) {
	elsewhere := imageHost(t, http.StatusOK)
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/abc.png", http.StatusFound)
	}))
	t.Cleanup(redirecting.Close)
	var got map[string]any
	hook := discord(t, http.StatusNoContent, &got)
	d := notify.NewDiscord(hook.Client())
	d.ImageHosts = []string{strings.TrimPrefix(redirecting.URL, "http://")}
	d.ImageScheme = "http"
	m := notify.WebhookMessage{Embeds: []notify.Embed{{Title: "A post"}}, Upload: redirecting.URL + "/abc.png"}
	if err := d.Send(context.Background(), hook.URL, m); err != nil {
		t.Fatalf("Send: %v", err)
	}
	embeds, _ := got["embeds"].([]any)
	if picture, _ := embeds[0].(map[string]any)["thumbnail"].(map[string]any); picture["url"] != m.Upload {
		t.Errorf("embeds = %v, want the image left as a link, not read from the host it redirected to", got["embeds"])
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
			if len(p.Embeds) != 1 || p.Upload != "" {
				t.Fatalf("payload = %+v, want one embed and nothing uploaded", p)
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
		if p.ThreadName != tt.want || p.Upload != "" || urlOf(p.Embeds[0].Image) != "https://cdn.example.com/a.jpg" {
			t.Errorf("title %q: thread %q, upload %q, image %v; want thread %q and the image in the embed, not uploaded", tt.title, p.ThreadName, p.Upload, p.Embeds[0].Image, tt.want)
		}
	}
}
