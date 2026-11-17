package notify_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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
		{"a Reddit feed failing", store.Notification{Kind: "reddit_failed", Reddit: store.RedditNotice{Subreddit: "some_sub"}, FeedError: "reddit answered 403"},
			`**Reddit feed failing**: r/some\_sub has failed its last 3 reads: reddit answered 403. <` + admin + "/#notifications>"},
		{"a Reddit feed reading again", store.Notification{Kind: "reddit_recovered", Reddit: store.RedditNotice{Subreddit: "some_sub"}},
			`**Reddit feed reading again**: r/some\_sub reads again; its new posts are posted. <` + admin + "/#notifications>"},
		{"an edition's address escaped", store.Notification{Kind: "blitz_report", Slug: "us_open", Series: "US Open", Edition: "Season 1 & 2", Round: 1, A: "Ann", B: "Bob", Result: "a", ReportedBy: "Ann"},
			"**Blitz result reported**: US Open Season 1 & 2, round 1: Ann beat Bob, reported by Ann. <" + admin + "/blitz/?t=us_open&e=Season+1+%26+2>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := notify.Text(tt.n, admin); got != tt.want {
				t.Errorf("Text(%+v)\n got = %q\nwant %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestMessagePutsTheEndpointsPrefixFirst(t *testing.T) {
	n := store.Notification{Kind: "link_request", Name: "Eve", SignIns: []string{"discord"}}
	tests := []struct{ prefix, want string }{
		{"", notify.Text(n, admin)},
		{"[dev]", `\[dev\] ` + notify.Text(n, admin)},
	}
	for _, tt := range tests {
		if got := notify.Message(n, store.NotificationEndpoint{Prefix: tt.prefix}, admin); got != tt.want {
			t.Errorf("Message with prefix %q = %q, want %q", tt.prefix, got, tt.want)
		}
	}
	if got, want := notify.TestMessage(store.NotificationEndpoint{Prefix: "[dev]"}), `\[dev\] `+notify.TestText; got != want {
		t.Errorf("TestMessage = %q, want %q", got, want)
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
	if err := notify.NewDiscord(s.Client()).Post(context.Background(), s.URL+"/api/webhooks/1/token", "hello @everyone"); err != nil {
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
	if err := notify.NewDiscord(s.Client()).Post(context.Background(), s.URL, strings.Repeat("é", 2500)); err != nil {
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
			err := notify.NewDiscord(&http.Client{Timeout: 5 * time.Second}).Post(context.Background(), tt.url, "hello")
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
		{"a gallery, its small preview in the corner", "image", preview, "New image post on /r/example", "", preview},
		{"a video", "video", preview, "New video post on /r/example", "", preview},
		{"a link", "link", preview, "New link post on /r/example", "", preview},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := notify.Payload(redditPost(tt.kind, tt.image), store.NotificationEndpoint{}, admin)
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
	p := notify.Payload(redditPost("text", ""), store.NotificationEndpoint{Prefix: "[dev]"}, admin)
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
	if e := notify.Payload(n, store.NotificationEndpoint{}, admin).Embeds[0]; len(e.Fields) != 0 {
		t.Errorf("fields = %+v, want none", e.Fields)
	}
}

func TestOtherNotificationsAreText(t *testing.T) {
	n := store.Notification{Kind: "link_request", Name: "Eve", SignIns: []string{"discord"}}
	e := store.NotificationEndpoint{Prefix: "[dev]"}
	if p := notify.Payload(n, e, admin); p.Content != notify.Message(n, e, admin) || len(p.Embeds) != 0 {
		t.Errorf("payload = %+v, want Message's text alone", p)
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
