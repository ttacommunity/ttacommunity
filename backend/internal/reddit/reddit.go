// Package reddit reads a subreddit's newest posts from Reddit's Atom feed, which needs no sign-in, and writes each
// post's text as Discord markdown. It waits out Reddit's rate limit (429) a few times before giving up.
package reddit

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ttacommunity/backend/internal/discordbot"
	"ttacommunity/backend/internal/store"
)

// Reddit is where the feeds are read.
const Reddit = "https://www.reddit.com"

// UserAgent names the project to Reddit, which refuses requests from generic clients.
const UserAgent = "ttacommunity-reddit-feed/1.0 (+https://github.com/ttacommunity/ttacommunity)"

// MaxBody is the most characters of a post's text kept.
const MaxBody = 3000

// Tries is how many times a feed is asked for while Reddit answers 429.
const Tries = 3

// defaultWait and maxWait bound the wait after a 429: Reddit's X-Ratelimit-Reset, in seconds.
const (
	defaultWait = 10 * time.Second
	maxWait     = time.Minute
)

// maxFeed is the most bytes of a feed read.
const maxFeed = 4 << 20

// Client reads feeds from base with hc; Wait waits between tries, and returns early with ctx's error.
type Client struct {
	hc   *http.Client
	base string
	Wait func(ctx context.Context, d time.Duration) error
}

// New returns a client reading feeds from base (Reddit, or a test server) with hc, whose timeout bounds each read.
func New(hc *http.Client, base string) *Client {
	return &Client{hc: hc, base: strings.TrimSuffix(base, "/"), Wait: wait}
}

// wait waits d, or until ctx ends.
func wait(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// Newest returns subreddit's newest posts, newest first. A 429 is tried again after Reddit's reset, Tries times in
// all; any other answer than success is an error naming its status.
func (c *Client) Newest(ctx context.Context, subreddit string) ([]store.RedditPost, error) {
	for try := 1; ; try++ {
		posts, retry, err := c.read(ctx, subreddit)
		if retry == 0 || try == Tries {
			return posts, err
		}
		if err := c.Wait(ctx, retry); err != nil {
			return nil, err
		}
	}
}

// read asks for subreddit's feed once; on a 429 it returns how long to wait before asking again.
func (c *Client) read(ctx context.Context, subreddit string) ([]store.RedditPost, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/r/"+url.PathEscape(subreddit)+"/new/.rss", nil)
	if err != nil {
		return nil, 0, fmt.Errorf("asking Reddit: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("asking Reddit: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		posts, err := Parse(io.LimitReader(resp.Body, maxFeed))
		return posts, 0, err
	case http.StatusTooManyRequests:
		retry := defaultWait
		if s, err := strconv.ParseFloat(resp.Header.Get("X-Ratelimit-Reset"), 64); err == nil && s > 0 {
			retry = min(time.Duration(s*float64(time.Second)), maxWait)
		}
		return nil, retry, fmt.Errorf("reddit answered %d", resp.StatusCode)
	}
	return nil, 0, fmt.Errorf("reddit answered %d", resp.StatusCode)
}

// atomEntry is a post as Reddit's feed writes it.
type atomEntry struct {
	ID     string `xml:"id"`
	Title  string `xml:"title"`
	Author string `xml:"author>name"`
	Link   struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
	Published string `xml:"published"`
	Content   string `xml:"content"`
	Thumbnail struct {
		URL string `xml:"url,attr"`
	} `xml:"http://search.yahoo.com/mrss/ thumbnail"`
}

var postID = regexp.MustCompile(`^t3_[a-z0-9]{1,20}$`)

// Parse reads Reddit's Atom feed: each post with its kind, text and image. A feed that is not Atom, or a post
// without a post id or date, is an error.
func Parse(r io.Reader) ([]store.RedditPost, error) {
	var feed struct {
		XMLName xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
		Entries []atomEntry `xml:"entry"`
	}
	if err := xml.NewDecoder(r).Decode(&feed); err != nil {
		return nil, fmt.Errorf("reading Reddit's feed: %w", err)
	}
	posts := make([]store.RedditPost, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		if !postID.MatchString(e.ID) {
			return nil, fmt.Errorf("reading Reddit's feed: a post id %q", e.ID)
		}
		at, err := time.Parse(time.RFC3339, e.Published)
		if err != nil {
			return nil, fmt.Errorf("reading Reddit's feed: post %s's date: %w", e.ID, err)
		}
		c := readContent(e.Content)
		kind := kindOf(c.link, e.Link.Href)
		image := e.Thumbnail.URL
		if kind == "image" && strings.HasPrefix(c.link, "https://i.redd.it/") {
			image = c.link
		}
		body := c.text
		if kind == "link" {
			body = strings.TrimSpace(c.link + "\n\n" + body)
		}
		posts = append(posts, store.RedditPost{
			PostID: e.ID, Kind: kind, Title: strings.TrimSpace(e.Title), URL: e.Link.Href,
			Author: strings.TrimPrefix(strings.TrimSpace(e.Author), "/u/"), Body: cut(body, MaxBody),
			ImageURL: image, PublishedAt: at.UTC(),
		})
	}
	return posts, nil
}

// kindOf tells a post's kind from what its [link] points at: the post itself for text, Reddit's image or video
// hosts, or anywhere else.
func kindOf(link, post string) string {
	u, err := url.Parse(link)
	switch {
	case link == "" || link == post || err != nil:
		return "text"
	case u.Host == "i.redd.it" || strings.HasPrefix(u.Path, "/gallery/"):
		return "image"
	case u.Host == "v.redd.it":
		return "video"
	}
	return "link"
}

// cut shortens text to at most n characters, ending with … at a word's end.
func cut(text string, n int) string {
	r := []rune(text)
	if len(r) <= n {
		return text
	}
	short := string(r[:n-1])
	if i := strings.LastIndexAny(short, " \n"); i > n/2 {
		short = short[:i]
	}
	return strings.TrimSpace(short) + "…"
}

// content is what a post's HTML holds: its text, as Discord markdown, and where its [link] points.
type content struct {
	text, link string
}

// readContent reads a post's HTML: the text in its div.md and the address of its [link].
func readContent(h string) content {
	d := xml.NewDecoder(strings.NewReader("<x>" + h + "</x>"))
	d.Strict, d.AutoClose, d.Entity = false, xml.HTMLAutoClose, xml.HTMLEntity
	w := &writer{}
	var c content
	depth, mdAt := 0, -1
	var href string
	var linkText strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if mdAt < 0 && t.Name.Local == "div" && attr(t, "class") == "md" {
				mdAt = depth
				continue
			}
			if mdAt >= 0 {
				w.start(t)
			} else if t.Name.Local == "a" {
				href = attr(t, "href")
				linkText.Reset()
			}
		case xml.EndElement:
			if depth == mdAt {
				mdAt = -1
			} else if mdAt >= 0 {
				w.end(t.Name.Local)
			} else if t.Name.Local == "a" && strings.TrimSpace(linkText.String()) == "[link]" {
				c.link = href
			}
			depth--
		case xml.CharData:
			if mdAt >= 0 {
				w.text(string(t))
			} else {
				linkText.Write(t)
			}
		}
	}
	c.text = w.String()
	return c
}

// attr is t's attribute name, or "".
func attr(t xml.StartElement, name string) string {
	for _, a := range t.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

var (
	spaces     = regexp.MustCompile(`[ \t\r\n]+`)
	blankLines = regexp.MustCompile(`\n{3,}`)
)

// writer writes a post's HTML as Discord markdown: one builder per open link or quote, whose text is wrapped as it
// closes.
type writer struct {
	parts []*part
	lists []int // each open list's next number; 0 for a bulleted one
	pre   int
}

type part struct {
	tag, href string
	b         strings.Builder
}

func (w *writer) top() *strings.Builder {
	if len(w.parts) == 0 {
		w.parts = append(w.parts, &part{})
	}
	return &w.parts[len(w.parts)-1].b
}

func (w *writer) write(s string) { w.top().WriteString(s) }

func (w *writer) start(t xml.StartElement) {
	switch t.Name.Local {
	case "p", "div", "table":
		w.write("\n\n")
	case "br", "tr":
		w.write("\n")
	case "td", "th":
		w.write(" | ")
	case "strong", "b":
		w.write("**")
	case "em", "i":
		w.write("*")
	case "del", "s":
		w.write("~~")
	case "h1", "h2", "h3", "h4", "h5", "h6":
		w.write("\n\n**")
	case "ul":
		w.lists = append(w.lists, 0)
	case "ol":
		w.lists = append(w.lists, 1)
	case "li":
		w.write("\n")
		if n := len(w.lists); n > 0 && w.lists[n-1] > 0 {
			w.write(strconv.Itoa(w.lists[n-1]) + ". ")
			w.lists[n-1]++
		} else {
			w.write("- ")
		}
	case "pre":
		w.pre++
		w.write("\n\n```\n")
	case "code":
		if w.pre == 0 {
			w.write("`")
		}
	case "a", "blockquote":
		w.top()
		w.parts = append(w.parts, &part{tag: t.Name.Local, href: attr(t, "href")})
	}
}

func (w *writer) end(tag string) {
	switch tag {
	case "p", "div", "table", "ul", "ol":
		w.write("\n\n")
		if (tag == "ul" || tag == "ol") && len(w.lists) > 0 {
			w.lists = w.lists[:len(w.lists)-1]
		}
	case "strong", "b":
		w.write("**")
	case "em", "i":
		w.write("*")
	case "del", "s":
		w.write("~~")
	case "h1", "h2", "h3", "h4", "h5", "h6":
		w.write("**\n\n")
	case "pre":
		w.pre--
		code := strings.TrimRight(w.top().String(), "\n")
		w.top().Reset()
		w.write(code + "\n```\n\n")
	case "code":
		if w.pre == 0 {
			w.write("`")
		}
	case "a", "blockquote":
		if n := len(w.parts); n > 1 && w.parts[n-1].tag == tag {
			p := w.parts[n-1]
			w.parts = w.parts[:n-1]
			w.write(wrap(p.tag, p.href, p.b.String()))
		}
	}
}

// wrap writes a closed link as [text](href), or its address alone when that is its text; a closed quote has each
// line begin with >.
func wrap(tag, href, inner string) string {
	if tag == "a" {
		text := strings.TrimSpace(inner)
		if href == "" || text == discordbot.EscapeMarkdown(href) || text == "" {
			return href
		}
		return "[" + text + "](" + href + ")"
	}
	lines := strings.Split(tidy(inner), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight("> "+l, " ")
	}
	return "\n\n" + strings.Join(lines, "\n") + "\n\n"
}

func (w *writer) text(s string) {
	if w.pre > 0 {
		w.write(strings.TrimPrefix(s, "\n"))
		return
	}
	w.write(discordbot.EscapeMarkdown(spaces.ReplaceAllString(s, " ")))
}

// String is the text written, with no space at the ends of lines and at most one blank line in a row.
func (w *writer) String() string {
	for len(w.parts) > 1 {
		p := w.parts[len(w.parts)-1]
		w.parts = w.parts[:len(w.parts)-1]
		w.write(p.b.String())
	}
	if len(w.parts) == 0 {
		return ""
	}
	return tidy(w.parts[0].b.String())
}

// tidy trims each line outside code blocks, collapses blank lines, and trims the ends.
func tidy(s string) string {
	lines := strings.Split(s, "\n")
	inCode := false
	for i, l := range lines {
		if strings.TrimSpace(l) == "```" {
			inCode = !inCode
			lines[i] = "```"
			continue
		}
		if !inCode {
			lines[i] = strings.TrimSpace(l)
		}
	}
	return strings.TrimSpace(blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}
