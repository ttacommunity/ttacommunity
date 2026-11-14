// Package movies fetches YouTube channel RSS feeds and filters uploads for Through the Ages content.
package movies

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	channelIDPattern = regexp.MustCompile(`^UC[A-Za-z0-9_-]{22}$`)
	channelInURL     = regexp.MustCompile(`/channel/(UC[A-Za-z0-9_-]{22})`)
	channelInHTML    = regexp.MustCompile(`"(?:channelId|externalId|browseId)"\s*:\s*"(UC[A-Za-z0-9_-]{22})"`)
	keywordsMeta     = regexp.MustCompile(`<meta\s+name="keywords"\s+content="([^"]*)"`)
	ttaWord          = regexp.MustCompile(`(?i)\btta\b`)
)

// Entry is one video from a YouTube channel's Atom feed.
type Entry struct {
	VideoID      string
	Title        string
	Description  string
	PublishedAt  time.Time
	ThumbnailURL string
}

// ParseChannelID extracts a YouTube UC... channel id from a bare id or /channel/ URL.
func ParseChannelID(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if channelIDPattern.MatchString(s) {
		return s, true
	}
	if m := channelInURL.FindStringSubmatch(s); len(m) == 2 {
		return m[1], true
	}
	return "", false
}

// IsTTAVideo reports whether a video's title, description, or tags indicate Through the Ages content.
func IsTTAVideo(title, description, keywords string, tournaments []string) bool {
	haystack := strings.ToLower(title + "\n" + description + "\n" + keywords)
	for _, needle := range []string{"#ttahub", "#throughtheages", "through the ages"} {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	if ttaWord.MatchString(haystack) {
		return true
	}
	for _, name := range tournaments {
		n := strings.TrimSpace(strings.ToLower(name))
		if len(n) >= 4 && strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

// ParseFeed parses a YouTube channel's Atom XML feed into video entries.
func ParseFeed(data []byte) ([]Entry, error) {
	var feed struct {
		Entries []struct {
			VideoID   string `xml:"http://www.youtube.com/xml/schemas/2015 videoId"`
			Title     string `xml:"title"`
			Published string `xml:"published"`
			Group     struct {
				Thumbnail struct {
					URL string `xml:"url,attr"`
				} `xml:"http://search.yahoo.com/mrss/ thumbnail"`
				Description string `xml:"http://search.yahoo.com/mrss/ description"`
			} `xml:"http://search.yahoo.com/mrss/ group"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parsing YouTube feed: %w", err)
	}
	out := make([]Entry, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		if e.VideoID == "" {
			continue
		}
		pub, err := time.Parse(time.RFC3339, strings.TrimSpace(e.Published))
		if err != nil {
			continue
		}
		thumb := e.Group.Thumbnail.URL
		if thumb == "" {
			thumb = fmt.Sprintf("https://i.ytimg.com/vi/%s/hqdefault.jpg", e.VideoID)
		}
		out = append(out, Entry{
			VideoID:      e.VideoID,
			Title:        strings.TrimSpace(e.Title),
			Description:  strings.TrimSpace(e.Group.Description),
			PublishedAt:  pub.UTC(),
			ThumbnailURL: thumb,
		})
	}
	return out, nil
}

// Fetcher resolves YouTube channels and fetches their Atom feeds and watch-page keywords.
type Fetcher struct {
	client *http.Client
}

// NewFetcher constructs a Fetcher using client (or a 15-second default client when nil).
func NewFetcher(client *http.Client) *Fetcher {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Fetcher{client: client}
}

// ResolveChannel resolves a channel id, /channel/ URL, or @handle URL to a UC... channel id.
func (f *Fetcher) ResolveChannel(ctx context.Context, raw string) (string, error) {
	if id, ok := ParseChannelID(raw); ok {
		return id, nil
	}
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "@") {
		s = "https://www.youtube.com/" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || (!strings.HasSuffix(u.Hostname(), "youtube.com") && !strings.HasSuffix(u.Hostname(), "youtu.be")) {
		return "", errors.New("invalid YouTube channel URL or id")
	}
	body, err := f.get(ctx, u.String(), 4<<20)
	if err != nil {
		return "", fmt.Errorf("fetching channel page: %w", err)
	}
	if m := channelInHTML.FindSubmatch(body); len(m) == 2 {
		return string(m[1]), nil
	}
	if m := channelInURL.FindSubmatch(body); len(m) == 2 {
		return string(m[1]), nil
	}
	return "", errors.New("could not find a YouTube channel id at that URL")
}

// FetchChannel fetches and parses a channel's public Atom feed.
func (f *Fetcher) FetchChannel(ctx context.Context, channelID string) ([]Entry, error) {
	if !channelIDPattern.MatchString(channelID) {
		return nil, errors.New("invalid channel id")
	}
	body, err := f.get(ctx, "https://www.youtube.com/feeds/videos.xml?channel_id="+url.QueryEscape(channelID), 2<<20)
	if err != nil {
		return nil, err
	}
	return ParseFeed(body)
}

// FetchKeywords fetches the video's watch page and returns its <meta name="keywords"> content.
func (f *Fetcher) FetchKeywords(ctx context.Context, videoID string) (string, error) {
	body, err := f.get(ctx, "https://www.youtube.com/watch?v="+url.QueryEscape(videoID), 2<<20)
	if err != nil {
		return "", err
	}
	if m := keywordsMeta.FindSubmatch(body); len(m) == 2 {
		return string(m[1]), nil
	}
	return "", nil
}

func (f *Fetcher) get(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cookie", "SOCS=CAI")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from YouTube", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}
