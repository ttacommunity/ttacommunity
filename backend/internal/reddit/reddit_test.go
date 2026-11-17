package reddit

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const comments = "https://www.reddit.com/r/example/comments/abc1/a_post/"

// entry is an invented Atom entry as Reddit writes one: content is the post's HTML, link its [link], thumbnail
// a media:thumbnail ("" for none).
func entry(content, link, thumbnail string) string {
	md := `<!-- SC_OFF --><div class="md">` + content + `</div><!-- SC_ON -->`
	if content == "" {
		md = ""
	}
	body := md + ` &#32; submitted by &#32; <a href="https://www.reddit.com/user/example_user"> /u/example_user </a> <br/> <span><a href="` +
		link + `">[link]</a></span> &#32; <span><a href="` + comments + `">[comments]</a></span>`
	thumb := ""
	if thumbnail != "" {
		thumb = `<media:thumbnail url="` + thumbnail + `" />`
	}
	return `<entry><author><name>/u/example_user</name><uri>https://www.reddit.com/user/example_user</uri></author>` +
		`<category term="example" label="r/example"/><content type="html">` + html.EscapeString(body) + `</content>` +
		`<id>t3_abc1</id>` + thumb + `<link href="` + comments + `" /><updated>2026-11-14T10:04:38+00:00</updated>` +
		`<published>2026-11-14T10:04:38+00:00</published><title>A post &amp; a title</title></entry>`
}

// feed wraps entries in Reddit's Atom feed.
func feed(entries ...string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><feed xmlns="http://www.w3.org/2005/Atom" xmlns:media="http://search.yahoo.com/mrss/">` +
		`<category term="example" label="r/example"/><title>newest submissions : example</title>` + strings.Join(entries, "") + `</feed>`
}

func TestParseReadsAPostsFields(t *testing.T) {
	posts, err := Parse(strings.NewReader(feed(entry("<p>Hello</p>", comments, ""))))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("Parse = %d posts, want 1", len(posts))
	}
	p := posts[0]
	want := time.Date(2026, 11, 14, 10, 4, 38, 0, time.UTC)
	if p.PostID != "t3_abc1" || p.Title != "A post & a title" || p.URL != comments || p.Author != "example_user" || !p.PublishedAt.Equal(want) {
		t.Errorf("post = %+v, want t3_abc1, %q, %s, example_user, %v", p, "A post & a title", comments, want)
	}
}

func TestParseTellsAPostsKindAndImage(t *testing.T) {
	const preview = "https://preview.redd.it/abc.jpeg?width=640&crop=smart"
	for _, tt := range []struct {
		name, link, thumbnail string
		kind, image           string
	}{
		{"a text post", comments, "", "text", ""},
		{"an image", "https://i.redd.it/abc.jpeg", preview, "image", "https://i.redd.it/abc.jpeg"},
		{"a gallery", "https://www.reddit.com/gallery/abc1", preview, "gallery", preview},
		{"a video", "https://v.redd.it/abc", preview, "video", preview},
		{"a link", "https://example.com/tool", preview, "link", preview},
		{"a link without a thumbnail", "https://example.com/tool", "", "link", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			posts, err := Parse(strings.NewReader(feed(entry("<p>Hi</p>", tt.link, html.EscapeString(tt.thumbnail)))))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := posts[0]; got.Kind != tt.kind || got.ImageURL != tt.image {
				t.Errorf("kind, image = %s, %q; want %s, %q", got.Kind, got.ImageURL, tt.kind, tt.image)
			}
		})
	}
}

func TestParseWritesAPostsTextAsDiscordMarkdown(t *testing.T) {
	for _, tt := range []struct {
		name, html, want string
	}{
		{"paragraphs", "<p>Hello,</p> <p>Second\nline</p>", "Hello,\n\nSecond line"},
		{"a link", `<p>See <a href="https://example.com/a">the tool</a></p>`, "See [the tool](https://example.com/a)"},
		{"a link showing its address", `<p><a href="https://example.com/a">https://example.com/a</a></p>`, "https://example.com/a"},
		{"emphasis", "<p><strong>bold</strong> and <em>soft</em></p>", "**bold** and *soft*"},
		{"a list", "<ul>\n<li>one</li>\n<li>two</li>\n</ul>", "- one\n- two"},
		{"a numbered list", "<ol>\n<li>one</li>\n<li>two</li>\n</ol>", "1. one\n2. two"},
		{"a quote", "<blockquote>\n<p>quoted</p>\n</blockquote> <p>reply</p>", "> quoted\n\nreply"},
		{"a heading", "<h1>Title</h1> <p>text</p>", "**Title**\n\ntext"},
		{"code", "<p>run <code>go test</code></p> <pre><code>a  *b*\n</code></pre>", "run `go test`\n\n```\na  *b*\n```"},
		{"a line break", "<p>one<br/>two</p>", "one\ntwo"},
		{"markdown in the text, escaped", "<p>2*3 = 6 and some_thing</p>", `2\*3 = 6 and some\_thing`},
		{"entities, with < and > escaped as Discord needs", "<p>it&#39;s &amp; &lt;b&gt;</p>", `it's & \<b\>`},
		{"no text", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			posts, err := Parse(strings.NewReader(feed(entry(tt.html, comments, ""))))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := posts[0].Body; got != tt.want {
				t.Errorf("Body of %q = %q, want %q", tt.html, got, tt.want)
			}
		})
	}
}

func TestParseStartsALinkPostsTextWithItsLink(t *testing.T) {
	posts, err := Parse(strings.NewReader(feed(entry("<p>Made this</p>", "https://example.com/tool", ""))))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := posts[0].Body, "https://example.com/tool\n\nMade this"; got != want {
		t.Errorf("Body = %q, want %q", got, want)
	}
}

func TestParseCutsALongText(t *testing.T) {
	long := strings.Repeat("word ", 2000)
	posts, err := Parse(strings.NewReader(feed(entry("<p>"+long+"</p>", comments, ""))))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := []rune(posts[0].Body); len(got) > MaxBody || !strings.HasSuffix(string(got), "…") {
		t.Errorf("Body is %d characters ending %q, want at most %d ending …", len(got), string(got[len(got)-5:]), MaxBody)
	}
}

func TestParseRefusesWhatIsNotAFeed(t *testing.T) {
	for name, body := range map[string]string{
		"an HTML page":    "<!doctype html><html><body>blocked</body></html>",
		"a bad post id":   strings.Replace(feed(entry("<p>x</p>", comments, "")), "t3_abc1", "abc1", 1),
		"a bad post date": strings.Replace(feed(entry("<p>x</p>", comments, "")), "<published>2026-11-14T10:04:38+00:00", "<published>yesterday", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(body)); err == nil {
				t.Errorf("Parse(%s) = no error, want one", name)
			}
		})
	}
}

// fakeReddit answers each request with the next of statuses, the feed on 200, counting the requests and the
// User-Agent each sent.
func fakeReddit(t *testing.T, statuses ...int) (*httptest.Server, *[]string) {
	t.Helper()
	var agents []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/r/example/new/.rss" {
			t.Errorf("path = %s, want /r/example/new/.rss", r.URL.Path)
		}
		status := statuses[min(len(agents), len(statuses)-1)]
		agents = append(agents, r.UserAgent())
		w.Header().Set("X-Ratelimit-Reset", "7")
		w.WriteHeader(status)
		if status == http.StatusOK {
			fmt.Fprint(w, feed(entry("<p>Hi</p>", comments, "")))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &agents
}

func TestNewestWaitsOutReddit429s(t *testing.T) {
	for _, tt := range []struct {
		name     string
		statuses []int
		requests int
		waits    []time.Duration
		err      string
	}{
		{"answered at once", []int{200}, 1, nil, ""},
		{"after a 429", []int{429, 200}, 2, []time.Duration{7 * time.Second}, ""},
		{"429 every time", []int{429}, Tries, []time.Duration{7 * time.Second, 7 * time.Second}, "reddit answered 429"},
		{"refused", []int{403}, 1, nil, "reddit answered 403"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv, agents := fakeReddit(t, tt.statuses...)
			var waits []time.Duration
			c := New(srv.Client(), srv.URL)
			c.Wait = func(_ context.Context, d time.Duration) error {
				waits = append(waits, d)
				return nil
			}
			posts, err := c.Newest(context.Background(), "example")
			if tt.err == "" && (err != nil || len(posts) != 1) {
				t.Errorf("Newest = %d posts, %v; want 1, no error", len(posts), err)
			}
			if tt.err != "" && (err == nil || err.Error() != tt.err) {
				t.Errorf("Newest: err = %v, want %q", err, tt.err)
			}
			if len(*agents) != tt.requests || fmt.Sprint(waits) != fmt.Sprint(tt.waits) {
				t.Errorf("requests, waits = %d, %v; want %d, %v", len(*agents), waits, tt.requests, tt.waits)
			}
			if (*agents)[0] != UserAgent {
				t.Errorf("User-Agent = %q, want %q", (*agents)[0], UserAgent)
			}
		})
	}
}

func TestNewestStopsWaitingWhenCancelled(t *testing.T) {
	srv, _ := fakeReddit(t, 429)
	c := New(srv.Client(), srv.URL)
	stop := errors.New("cancelled")
	c.Wait = func(context.Context, time.Duration) error { return stop }
	if _, err := c.Newest(context.Background(), "example"); !errors.Is(err, stop) {
		t.Errorf("Newest: err = %v, want %v", err, stop)
	}
}
