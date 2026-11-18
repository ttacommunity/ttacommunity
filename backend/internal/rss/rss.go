// Package rss reads RSS 2.0, RSS 1.0 and Atom feeds from addresses admins type: over https only, within a size
// and a time, and never from an address that is not public, redirects included. Items' text is kept as plain text.
package rss

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"ttacommunity/backend/internal/discordbot"
	"ttacommunity/backend/internal/store"
)

// UserAgent names the project to the sites it reads.
const UserAgent = "ttacommunity-feeds/1.0 (+https://github.com/ttacommunity/ttacommunity)"

// MaxFeed is the most bytes of a feed read; a larger one is refused.
const MaxFeed = 4 << 20

// Timeout bounds each read, redirects and body included.
const Timeout = 20 * time.Second

// MaxSummary is the most characters of an item's summary kept, and maxItems the most items of a read.
const (
	MaxSummary = 3000
	maxItems   = 100
	maxTitle   = 300
	maxID      = 1000
	maxHops    = 5
)

// ErrNotPublic means an address that is not on the public internet: refused, so a feed cannot reach our own network.
var ErrNotPublic = errors.New("the address is not public")

// notPublic are the ranges, beyond what net/netip already tells, that are not on the public internet.
var notPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("100::/64"), netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

// Public says whether a is on the public internet: not loopback, private, link-local, shared, reserved or
// documentation space, nor a translation of one.
func Public(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() {
		return false
	}
	for _, p := range notPublic {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

// SafeClient is an HTTP client on a copy of base that connects only to addresses allow accepts, checked on the
// address dialled, so a name cannot resolve past it, follows redirects only to https, and gives up after Timeout.
// It uses no proxy.
func SafeClient(base *http.Transport, allow func(netip.Addr) bool) *http.Client {
	t := base.Clone()
	t.Proxy = nil
	dialer := &net.Dialer{Timeout: Timeout, Control: func(_, address string, _ syscall.RawConn) error {
		ap, err := netip.ParseAddrPort(address)
		if err != nil || !allow(ap.Addr()) {
			return ErrNotPublic
		}
		return nil
	}}
	t.DialContext = dialer.DialContext
	return &http.Client{Transport: t, Timeout: Timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return errors.New("it redirects to an address that is not https")
		}
		if len(via) >= maxHops {
			return errors.New("it redirects too many times")
		}
		return nil
	}}
}

// Feed is a feed as read: its title and its items, as the feed lists them.
type Feed struct {
	Title string
	Items []store.RSSItem
}

// Client reads feeds with hc, which should be a SafeClient.
type Client struct {
	hc *http.Client
}

// New returns a client reading feeds with hc.
func New(hc *http.Client) *Client { return &Client{hc: hc} }

// Read reads the feed at address, an https one, at now, the date of an item that gives none. A feed that answers
// other than 200, is larger than MaxFeed or is not a feed is an error saying so, without the address.
func (c *Client) Read(ctx context.Context, address string, now time.Time) (Feed, error) {
	if u, err := url.Parse(address); err != nil || u.Scheme != "https" || u.Host == "" {
		return Feed{}, errors.New("the feed's address is not https")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return Feed{}, errors.New("the feed's address is not https")
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml;q=0.9, text/xml;q=0.8")
	resp, err := c.hc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return Feed{}, fmt.Errorf("reading the feed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Feed{}, fmt.Errorf("the feed answered %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxFeed+1))
	if err != nil {
		return Feed{}, fmt.Errorf("reading the feed: %w", err)
	}
	if len(body) > MaxFeed {
		return Feed{}, errTooLarge
	}
	return Parse(strings.NewReader(string(body)), now)
}

var errTooLarge = fmt.Errorf("the feed is larger than %d MB", MaxFeed>>20)

// node is an element of an item: its text (with Atom's type: text, html or xhtml, whose markup is Inner), or the
// address it points at (a link's href, or a picture's or enclosure's url, with its type and medium).
type node struct {
	Type   string `xml:"type,attr"`
	Href   string `xml:"href,attr"`
	Rel    string `xml:"rel,attr"`
	URL    string `xml:"url,attr"`
	Medium string `xml:"medium,attr"`
	Body   string `xml:",chardata"`
	Inner  string `xml:",innerxml"`
}

// html is n's text as HTML: xhtml's markup, html's text, or plain text escaped.
func (n node) html() string {
	switch n.Type {
	case "xhtml":
		return n.Inner
	case "text":
		var b strings.Builder
		_ = xml.EscapeText(&b, []byte(n.Body))
		return b.String()
	}
	return n.Body
}

// item is an RSS item or an Atom entry: the parts of either that are read. Content is Atom's text or media:content.
type item struct {
	Title       node   `xml:"title"`
	Links       []node `xml:"link"`
	GUID        string `xml:"guid"`
	ID          string `xml:"id"`
	Description node   `xml:"description"`
	Encoded     string `xml:"encoded"`
	Summary     node   `xml:"summary"`
	Content     []node `xml:"content"`
	PubDate     string `xml:"pubDate"`
	Date        string `xml:"date"`
	Published   string `xml:"published"`
	Updated     string `xml:"updated"`
	Enclosures  []node `xml:"enclosure"`
	Thumbnails  []node `xml:"thumbnail"`
	Group       struct {
		Content    []node `xml:"content"`
		Thumbnails []node `xml:"thumbnail"`
		Text       node   `xml:"description"`
	} `xml:"group"`
}

// Parse reads an RSS 2.0, RSS 1.0 or Atom feed, its items' dates defaulting to now. An item with neither an id
// nor a link is left out; a document that is not one of these feeds is an error.
func Parse(r io.Reader, now time.Time) (Feed, error) {
	var doc struct {
		XMLName      xml.Name
		ChannelTitle node   `xml:"channel>title"`
		Title        node   `xml:"title"`
		Items        []item `xml:"channel>item"`
		RDFItems     []item `xml:"item"`
		Entries      []item `xml:"entry"`
	}
	d := xml.NewDecoder(r)
	d.Entity = xml.HTMLEntity
	d.CharsetReader = charsetReader
	if err := d.Decode(&doc); err != nil {
		return Feed{}, errors.New("the feed is not RSS or Atom: " + shortError(err))
	}
	var f Feed
	switch doc.XMLName.Local {
	case "rss", "RDF":
		f.Title = plain(doc.ChannelTitle.html())
	case "feed":
		f.Title = plain(doc.Title.html())
	default:
		return Feed{}, errors.New("the feed is not RSS or Atom")
	}
	for _, it := range append(append(doc.Items, doc.RDFItems...), doc.Entries...) {
		if len(f.Items) == maxItems {
			break
		}
		if got, ok := it.read(now); ok {
			f.Items = append(f.Items, got)
		}
	}
	return f, nil
}

// read is it as an item, or false when it has neither an id nor a link to know it by.
func (it item) read(now time.Time) (store.RSSItem, bool) {
	link := it.link()
	id := strings.TrimSpace(it.GUID)
	if id == "" {
		id = strings.TrimSpace(it.ID)
	}
	if id == "" || len([]rune(id)) > maxID {
		id = link
	}
	if id == "" || len([]rune(id)) > maxID {
		return store.RSSItem{}, false
	}
	body := it.Description.html()
	if body == "" {
		body = it.Encoded
	}
	if body == "" {
		body = it.Summary.html()
	}
	for _, c := range it.Content {
		if body == "" && c.URL == "" {
			body = c.html()
		}
	}
	if body == "" {
		body = it.Group.Text.html()
	}
	summary, img := toText(body)
	return store.RSSItem{
		ItemID:      id,
		Title:       cut(plain(it.Title.html()), maxTitle),
		URL:         link,
		Summary:     discordbot.CutAtWord(summary, MaxSummary),
		ImageURL:    it.image(img),
		PublishedAt: dateOf(now, it.PubDate, it.Date, it.Published, it.Updated),
	}, true
}

// link is the item's address: RSS's link, or Atom's alternate one; only an absolute http or https address.
func (it item) link() string {
	for _, l := range it.Links {
		c := strings.TrimSpace(l.Body)
		if l.Href != "" && (l.Rel == "" || l.Rel == "alternate") {
			c = strings.TrimSpace(l.Href)
		}
		if u, err := url.Parse(c); err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && len([]rune(c)) <= maxID {
			return c
		}
	}
	return ""
}

// image is the item's picture: an image it attaches, its thumbnail, or else inText, the first in its text; only
// over https.
func (it item) image(inText string) string {
	var all []node
	for _, m := range slices.Concat(it.Enclosures, it.Content, it.Group.Content) {
		if strings.HasPrefix(m.Type, "image/") || m.Medium == "image" {
			all = append(all, m)
		}
	}
	all = append(slices.Concat(all, it.Thumbnails, it.Group.Thumbnails), node{URL: inText})
	for _, m := range all {
		if u, err := url.Parse(strings.TrimSpace(m.URL)); err == nil && u.Scheme == "https" && u.Host != "" {
			return u.String()
		}
	}
	return ""
}

// dateLayouts are the dates feeds write: RFC 822's in its variants (RSS), and RFC 3339's (Atom, dc:date).
var dateLayouts = []string{
	time.RFC1123Z, time.RFC1123, "Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST", "2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04 -0700", "Mon, 2 Jan 2006 15:04 MST", time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02",
}

// dateOf is the first of dates that reads, in UTC, or now.
func dateOf(now time.Time, dates ...string) time.Time {
	for _, s := range dates {
		s = strings.TrimSpace(s)
		for _, layout := range dateLayouts {
			if t, err := time.Parse(layout, s); err == nil {
				return t.UTC()
			}
		}
	}
	return now.UTC()
}

var (
	spaces     = regexp.MustCompile(`[ \t\r\n\x{a0}]+`)
	lineSpaces = regexp.MustCompile(` *\n *`)
	blankLines = regexp.MustCompile(`\n{3,}`)
)

// blocks are the elements that start a line.
var blocks = map[string]bool{"p": true, "div": true, "br": true, "li": true, "tr": true, "blockquote": true, "pre": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "ul": true, "ol": true, "table": true, "hr": true}

// toText is h, HTML, as plain text escaped for Discord, with no markup of its own, and the first image it shows.
// Script and style are left out.
func toText(h string) (string, string) {
	d := xml.NewDecoder(strings.NewReader("<x>" + h + "</x>"))
	d.Strict, d.AutoClose, d.Entity = false, xml.HTMLAutoClose, xml.HTMLEntity
	var b strings.Builder
	var img string
	skip := 0
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			if name == "script" || name == "style" {
				skip++
			}
			if name == "img" && img == "" {
				for _, a := range t.Attr {
					if a.Name.Local == "src" {
						img = a.Value
					}
				}
			}
			if blocks[name] {
				b.WriteString("\n")
			}
		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			if (name == "script" || name == "style") && skip > 0 {
				skip--
			}
			if blocks[name] && name != "br" && name != "hr" {
				b.WriteString("\n")
			}
		case xml.CharData:
			if skip == 0 {
				b.WriteString(spaces.ReplaceAllString(string(t), " "))
			}
		}
	}
	s := lineSpaces.ReplaceAllString(b.String(), "\n")
	s = strings.TrimSpace(blankLines.ReplaceAllString(s, "\n\n"))
	return discordbot.EscapeMarkdown(s), img
}

// plain is h's text on one line, without escaping.
func plain(h string) string {
	s, _ := toText(h)
	return strings.TrimSpace(spaces.ReplaceAllString(unescape(s), " "))
}

// unescape undoes discordbot.EscapeMarkdown's backslashes.
func unescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// cut shortens s to at most n characters.
func cut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// shortError is err's message cut to fit a feed's last error.
func shortError(err error) string { return cut(err.Error(), 120) }

// charsetReader reads ISO-8859-1 and Windows-1252 feeds as UTF-8 (the second's extra characters as the first's);
// any other encoding than UTF-8 is refused.
func charsetReader(label string, r io.Reader) (io.Reader, error) {
	switch strings.ToLower(label) {
	case "iso-8859-1", "latin1", "latin-1", "windows-1252", "cp1252":
		b, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		out := make([]rune, len(b))
		for i, c := range b {
			out[i] = rune(c)
		}
		return strings.NewReader(string(out)), nil
	}
	return nil, fmt.Errorf("its encoding %q is not supported", label)
}
