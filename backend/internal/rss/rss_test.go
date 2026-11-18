package rss

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

var readAt = time.Date(2026, 11, 18, 1, 0, 0, 0, time.UTC)

// rss2 wraps items in an invented RSS 2.0 feed titled Example news.
func rss2(items ...string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0" xmlns:media="http://search.yahoo.com/mrss/" ` +
		`xmlns:content="http://purl.org/rss/1.0/modules/content/" xmlns:atom="http://www.w3.org/2005/Atom"><channel>` +
		`<title>Example &amp; news</title><link>https://news.example.com/</link>` +
		`<atom:link href="https://news.example.com/feed.xml" rel="self"/>` + strings.Join(items, "") + `</channel></rss>`
}

// atom wraps entries in an invented Atom feed titled Example news.
func atom(entries ...string) string {
	return `<?xml version="1.0" encoding="utf-8"?><feed xmlns="http://www.w3.org/2005/Atom" xmlns:media="http://search.yahoo.com/mrss/">` +
		`<title type="text">Example news</title><id>urn:example:feed</id>` + strings.Join(entries, "") + `</feed>`
}

func TestParseReadsEachFormatsItems(t *testing.T) {
	at := time.Date(2026, 11, 14, 10, 4, 38, 0, time.UTC)
	for _, tt := range []struct {
		name, doc        string
		wantID, wantLink string
	}{
		{"RSS 2.0", rss2(`<item><title>A title &amp; more</title><link>https://news.example.com/1</link><guid isPermaLink="false">item-1</guid>` +
			`<pubDate>Sat, 14 Nov 2026 10:04:38 +0000</pubDate></item>`), "item-1", "https://news.example.com/1"},
		{"RSS 2.0 with a one-digit day and a zone name", rss2(`<item><title>A title &amp; more</title><link>https://news.example.com/1</link>` +
			`<pubDate>Sat, 14 Nov 2026 10:04:38 GMT</pubDate></item>`), "https://news.example.com/1", "https://news.example.com/1"},
		{"RSS 1.0", `<?xml version="1.0"?><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns="http://purl.org/rss/1.0/" ` +
			`xmlns:dc="http://purl.org/dc/elements/1.1/"><channel><title>Example &amp; news</title></channel>` +
			`<item rdf:about="https://news.example.com/1"><title>A title &amp; more</title><link>https://news.example.com/1</link>` +
			`<dc:date>2026-11-14T10:04:38Z</dc:date></item></rdf:RDF>`, "https://news.example.com/1", "https://news.example.com/1"},
		{"Atom", atom(`<entry><id>urn:example:1</id><title type="html">A title &amp;amp; more</title>` +
			`<link rel="replies" href="https://news.example.com/1#comments"/><link href="https://news.example.com/1"/>` +
			`<updated>2026-11-15T00:00:00Z</updated><published>2026-11-14T11:04:38+01:00</published></entry>`), "urn:example:1", "https://news.example.com/1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse(strings.NewReader(tt.doc), readAt)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(f.Items) != 1 {
				t.Fatalf("Parse = %d items, want 1", len(f.Items))
			}
			it := f.Items[0]
			if f.Title != "Example & news" && f.Title != "Example news" {
				t.Errorf("feed title = %q, want Example news", f.Title)
			}
			if it.ItemID != tt.wantID || it.URL != tt.wantLink || it.Title != "A title & more" || !it.PublishedAt.Equal(at) {
				t.Errorf("item = %+v, want id %q, link %q, title %q, published %v", it, tt.wantID, tt.wantLink, "A title & more", at)
			}
		})
	}
}

func TestParseKnowsAnItemByItsLinkOrLeavesItOut(t *testing.T) {
	f, err := Parse(strings.NewReader(rss2(
		`<item><title>No id or link</title></item>`,
		`<item><title>A relative link</title><link>/1</link></item>`,
		`<item><description>A post without a title</description><link>https://social.example.com/@someone/1</link></item>`,
		`<item><guid>`+strings.Repeat("a", 1001)+`</guid><link>https://news.example.com/long</link></item>`,
	)), readAt)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var got []string
	for _, it := range f.Items {
		got = append(got, it.ItemID)
	}
	want := []string{"https://social.example.com/@someone/1", "https://news.example.com/long"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("items = %q, want %q", got, want)
	}
	if !f.Items[0].PublishedAt.Equal(readAt) {
		t.Errorf("an item without a date: published %v, want the read's time %v", f.Items[0].PublishedAt, readAt)
	}
}

func TestParseWritesTheSummaryAsPlainText(t *testing.T) {
	for _, tt := range []struct {
		name, item, want string
	}{
		{"HTML in a description", `<description>&lt;p&gt;Hello &lt;b&gt;world&lt;/b&gt;&lt;/p&gt;&lt;p&gt;Second&amp;nbsp;line&lt;/p&gt;</description>`,
			"Hello world\n\nSecond line"},
		{"HTML in CDATA, with a script", `<description><![CDATA[<div>Hi<script>alert(1)</script><br>there</div>]]></description>`, "Hi\nthere"},
		{"markdown escaped", `<description>*bold* and @everyone</description>`, `\*bold\* and @everyone`},
		{"content:encoded when no description", `<content:encoded><![CDATA[<p>Full text</p>]]></content:encoded>`, "Full text"},
		{"none", ``, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse(strings.NewReader(rss2(`<item><link>https://news.example.com/1</link>`+tt.item+`</item>`)), readAt)
			if err != nil || len(f.Items) != 1 {
				t.Fatalf("Parse = %+v, %v; want one item", f, err)
			}
			if got := f.Items[0].Summary; got != tt.want {
				t.Errorf("summary = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseWritesAtomsTextOfEachType(t *testing.T) {
	for _, tt := range []struct {
		name, summary, want string
	}{
		{"text", `<summary type="text">1 &lt; 2</summary>`, `1 \< 2`},
		{"html", `<summary type="html">&lt;p&gt;Hello&lt;/p&gt;</summary>`, "Hello"},
		{"xhtml", `<content type="xhtml"><div xmlns="http://www.w3.org/1999/xhtml"><p>Hello <em>there</em></p></div></content>`, "Hello there"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse(strings.NewReader(atom(`<entry><id>urn:example:1</id>`+tt.summary+`</entry>`)), readAt)
			if err != nil || len(f.Items) != 1 {
				t.Fatalf("Parse = %+v, %v; want one item", f, err)
			}
			if got := f.Items[0].Summary; got != tt.want {
				t.Errorf("summary = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseFindsAnItemsImage(t *testing.T) {
	for _, tt := range []struct {
		name, parts, want string
	}{
		{"an image enclosure", `<enclosure url="https://cdn.example.com/a.jpg" type="image/jpeg" length="1"/>`, "https://cdn.example.com/a.jpg"},
		{"an audio enclosure is not one", `<enclosure url="https://cdn.example.com/a.mp3" type="audio/mpeg" length="1"/>`, ""},
		{"media:content", `<media:content url="https://cdn.example.com/b.png" medium="image"/>`, "https://cdn.example.com/b.png"},
		{"a thumbnail in a media:group", `<media:group><media:thumbnail url="https://cdn.example.com/c.jpg"/></media:group>`, "https://cdn.example.com/c.jpg"},
		{"the first in its text", `<description>&lt;img src="https://cdn.example.com/d.gif"&gt;</description>`, "https://cdn.example.com/d.gif"},
		{"not over http", `<media:thumbnail url="http://cdn.example.com/e.jpg"/>`, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse(strings.NewReader(rss2(`<item><link>https://news.example.com/1</link>`+tt.parts+`</item>`)), readAt)
			if err != nil || len(f.Items) != 1 {
				t.Fatalf("Parse = %+v, %v; want one item", f, err)
			}
			if got := f.Items[0].ImageURL; got != tt.want {
				t.Errorf("image = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseRefusesWhatIsNotAFeed(t *testing.T) {
	for _, tt := range []struct{ name, doc string }{
		{"HTML", `<html><body>Not a feed</body></html>`},
		{"not XML", `{"items": []}`},
		{"an encoding not read", `<?xml version="1.0" encoding="shift_jis"?><rss><channel></channel></rss>`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(tt.doc), readAt); err == nil {
				t.Errorf("Parse(%q): no error, want one", tt.doc)
			}
		})
	}
}

func TestParseReadsALatin1Feed(t *testing.T) {
	doc := "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><rss><channel><title>Caf\xe9</title></channel></rss>"
	if f, err := Parse(strings.NewReader(doc), readAt); err != nil || f.Title != "Café" {
		t.Errorf("Parse = %q, %v; want Café", f.Title, err)
	}
}

func TestPublicRefusesAddressesOffThePublicInternet(t *testing.T) {
	for _, tt := range []struct {
		addr string
		want bool
	}{
		{"93.184.215.14", true},
		{"2606:2800:21f:cb07:6820:80da:af6b:8b2c", true},
		{"127.0.0.1", false},
		{"10.1.2.3", false},
		{"172.16.0.1", false},
		{"192.168.1.1", false},
		{"169.254.169.254", false},
		{"100.64.0.1", false},
		{"0.0.0.0", false},
		{"255.255.255.255", false},
		{"::1", false},
		{"fd00::1", false},
		{"fe80::1", false},
		{"::ffff:10.0.0.1", false},
		{"64:ff9b::a00:1", false},
		{"2002:a00:1::", false},
	} {
		if got := Public(netip.MustParseAddr(tt.addr)); got != tt.want {
			t.Errorf("Public(%s) = %v, want %v", tt.addr, got, tt.want)
		}
	}
}

// loopbackOnly lets the tests' servers, on loopback, be reached, and nothing else.
func loopbackOnly(a netip.Addr) bool { return a.IsLoopback() }

// serve starts an https server answering with h, and returns a client allowed only loopback, the server's address
// and its client's transport.
func serve(t *testing.T, h http.HandlerFunc) (*Client, string) {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	return New(SafeClient(srv.Client().Transport.(*http.Transport), loopbackOnly)), srv.URL
}

func TestReadReadsAFeedOverHTTPS(t *testing.T) {
	var agent string
	c, base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		agent = r.UserAgent()
		_, _ = w.Write([]byte(rss2(`<item><title>One</title><link>https://news.example.com/1</link></item>`)))
	})
	f, err := c.Read(context.Background(), base+"/feed.xml", readAt)
	if err != nil || len(f.Items) != 1 || f.Items[0].Title != "One" {
		t.Fatalf("Read = %+v, %v; want the one item", f, err)
	}
	if agent != UserAgent {
		t.Errorf("User-Agent = %q, want %q", agent, UserAgent)
	}
}

func TestReadRefusesWhatIsNotSafeToRead(t *testing.T) {
	big := strings.Repeat("x", MaxFeed+1)
	for _, tt := range []struct {
		name    string
		path    string
		handler http.HandlerFunc
		want    string
	}{
		{"a status other than 200", "/feed", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }, "answered 404"},
		{"too large, by its length", "/feed", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(big)) }, "larger than"},
		{"too large, without a length", "/feed", func(w http.ResponseWriter, _ *http.Request) {
			for range MaxFeed/1000 + 2 {
				_, _ = w.Write([]byte(strings.Repeat("x", 1000)))
				w.(http.Flusher).Flush()
			}
		}, "larger than"},
		{"a redirect to http", "/feed", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://"+r.Host+"/other", http.StatusFound)
		}, "not https"},
		{"a redirect to an address not allowed", "/feed", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://10.0.0.1/feed", http.StatusFound)
		}, ErrNotPublic.Error()},
		{"redirects without end", "/feed", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/feed", http.StatusFound)
		}, "too many"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, base := serve(t, tt.handler)
			_, err := c.Read(context.Background(), base+tt.path, readAt)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Read: err = %v, want one saying %q", err, tt.want)
			}
			if err != nil && strings.Contains(err.Error(), base) {
				t.Errorf("Read: err = %v names the address", err)
			}
		})
	}
}

func TestReadRefusesAnAddressNotHTTPS(t *testing.T) {
	c := New(SafeClient(http.DefaultTransport.(*http.Transport), Public))
	for _, address := range []string{"http://news.example.com/feed", "file:///etc/passwd", "news.example.com/feed"} {
		if _, err := c.Read(context.Background(), address, readAt); err == nil || !strings.Contains(err.Error(), "not https") {
			t.Errorf("Read(%q): err = %v, want not https", address, err)
		}
	}
}

func TestASafeClientRefusesAServerOnLoopbackAndANameResolvingThere(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(rss2())) }))
	t.Cleanup(srv.Close)
	c := New(SafeClient(srv.Client().Transport.(*http.Transport), Public))
	port := srv.URL[strings.LastIndex(srv.URL, ":"):]
	for _, address := range []string{srv.URL + "/feed", "https://localhost" + port + "/feed"} {
		if _, err := c.Read(context.Background(), address, readAt); !errors.Is(err, ErrNotPublic) {
			t.Errorf("Read(%s): err = %v, want %v", address, err, ErrNotPublic)
		}
	}
}

func TestReadGivesUpOnASlowFeed(t *testing.T) {
	release := make(chan struct{})
	c, base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := c.Read(ctx, base+"/feed", readAt); err == nil {
		t.Errorf("Read of a feed that never answers: no error, want one")
	}
	if c.hc.Timeout != Timeout {
		t.Errorf("the client's timeout = %v, want %v", c.hc.Timeout, Timeout)
	}
}
