// The YouTube Data API v3, read with an API key: videos' details, a channel's latest uploads, and a handle's
// channel.
package movies

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DataAPI is the YouTube Data API's address.
const DataAPI = "https://www.googleapis.com/youtube/v3"

// maxVideosPerCall is the most ids videos.list takes at once.
const maxVideosPerCall = 50

var (
	// ErrNoYouTube means no Data API key is set up here, so only a channel's id or /channel/ address is taken.
	ErrNoYouTube = errors.New("YouTube is not set up here: give the channel's UC… id or its /channel/ address")
	// ErrNotAChannel means the text is neither a channel's id nor its /channel/ or @handle address.
	ErrNotAChannel = errors.New("not a YouTube channel: give its @handle, its address or its UC… id")
	// ErrNoSuchChannel means YouTube has no channel with that handle.
	ErrNoSuchChannel = errors.New("YouTube has no channel with that handle")

	handlePattern = regexp.MustCompile(`^@[\p{L}\p{N}._-]{3,30}$`)
)

// VideoDetails is what the Data API says of one video.
type VideoDetails struct {
	VideoID     string
	ChannelID   string
	Title       string
	Description string
	Tags        []string
	PublishedAt time.Time
}

// YouTube reads the Data API with a key, sent in a header so no address or error carries it.
type YouTube struct {
	client *http.Client
	base   string
	key    string
}

// NewYouTube reads the Data API at base (DataAPI, or a test server) with key, through client (a 15-second
// default when nil).
func NewYouTube(client *http.Client, base, key string) *YouTube {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &YouTube{client: client, base: strings.TrimSuffix(base, "/"), key: key}
}

// String names the type without its key.
func (y *YouTube) String() string { return "YouTube{key: redacted}" }

// GoString names the type without its key, for %#v.
func (y *YouTube) GoString() string { return y.String() }

// Videos returns the details of each of ids (at most 50) that YouTube still has and shows: a removed or
// private video is left out.
func (y *YouTube) Videos(ctx context.Context, ids []string) (map[string]VideoDetails, error) {
	if len(ids) > maxVideosPerCall {
		return nil, fmt.Errorf("asking for %d videos at once, at most %d", len(ids), maxVideosPerCall)
	}
	var resp struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				ChannelID   string    `json:"channelId"`
				Title       string    `json:"title"`
				Description string    `json:"description"`
				Tags        []string  `json:"tags"`
				PublishedAt time.Time `json:"publishedAt"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := y.get(ctx, "/videos", url.Values{"part": {"snippet"}, "id": {strings.Join(ids, ",")}}, &resp); err != nil {
		return nil, fmt.Errorf("reading videos: %w", err)
	}
	out := make(map[string]VideoDetails, len(resp.Items))
	for _, it := range resp.Items {
		out[it.ID] = VideoDetails{
			VideoID: it.ID, ChannelID: it.Snippet.ChannelID, Title: it.Snippet.Title,
			Description: it.Snippet.Description, Tags: it.Snippet.Tags, PublishedAt: it.Snippet.PublishedAt.UTC(),
		}
	}
	return out, nil
}

// LatestUploads returns the ids of channelID's newest n uploads (at most 50), newest first, from its uploads
// playlist. ErrNoSuchChannel when YouTube has no such channel.
func (y *YouTube) LatestUploads(ctx context.Context, channelID string, n int) ([]string, error) {
	if !channelIDPattern.MatchString(channelID) {
		return nil, errors.New("invalid channel id")
	}
	var channels struct {
		Items []struct {
			ContentDetails struct {
				RelatedPlaylists struct {
					Uploads string `json:"uploads"`
				} `json:"relatedPlaylists"`
			} `json:"contentDetails"`
		} `json:"items"`
	}
	if err := y.get(ctx, "/channels", url.Values{"part": {"contentDetails"}, "id": {channelID}}, &channels); err != nil {
		return nil, fmt.Errorf("reading a channel: %w", err)
	}
	if len(channels.Items) == 0 || channels.Items[0].ContentDetails.RelatedPlaylists.Uploads == "" {
		return nil, ErrNoSuchChannel
	}
	var items struct {
		Items []struct {
			ContentDetails struct {
				VideoID string `json:"videoId"`
			} `json:"contentDetails"`
		} `json:"items"`
	}
	query := url.Values{"part": {"contentDetails"}, "playlistId": {channels.Items[0].ContentDetails.RelatedPlaylists.Uploads},
		"maxResults": {strconv.Itoa(min(n, maxVideosPerCall))}}
	if err := y.get(ctx, "/playlistItems", query, &items); err != nil {
		return nil, fmt.Errorf("reading a channel's uploads: %w", err)
	}
	ids := make([]string, 0, len(items.Items))
	for _, it := range items.Items {
		if videoIDPattern.MatchString(it.ContentDetails.VideoID) {
			ids = append(ids, it.ContentDetails.VideoID)
		}
	}
	return ids, nil
}

// ResolveChannel returns the channel id raw names: a UC… id or a /channel/ address as it is, and an @handle or
// a youtube.com/@handle address from the Data API. A nil YouTube takes only the first two.
func (y *YouTube) ResolveChannel(ctx context.Context, raw string) (string, error) {
	if id, ok := ParseChannelID(raw); ok {
		return id, nil
	}
	handle, ok := parseHandle(raw)
	if !ok {
		return "", ErrNotAChannel
	}
	if y == nil {
		return "", ErrNoYouTube
	}
	var resp struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := y.get(ctx, "/channels", url.Values{"part": {"id"}, "forHandle": {handle}}, &resp); err != nil {
		return "", fmt.Errorf("looking up a handle: %w", err)
	}
	if len(resp.Items) == 0 || !channelIDPattern.MatchString(resp.Items[0].ID) {
		return "", ErrNoSuchChannel
	}
	return resp.Items[0].ID, nil
}

// parseHandle returns the @handle in raw, a bare handle or a youtube.com address whose path starts with one.
func parseHandle(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if !strings.HasPrefix(s, "@") {
		u, err := url.Parse(s)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return "", false
		}
		if host := strings.ToLower(u.Hostname()); host != "youtube.com" && !strings.HasSuffix(host, ".youtube.com") {
			return "", false
		}
		s, _, _ = strings.Cut(strings.TrimPrefix(u.Path, "/"), "/")
	}
	if !handlePattern.MatchString(s) {
		return "", false
	}
	return s, true
}

// get reads path with query into out, returning the status and Google's reason on a refusal.
func (y *YouTube) get(ctx context.Context, path string, query url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, y.base+path+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Goog-Api-Key", y.key)
	resp, err := y.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error struct {
				Errors []struct {
					Reason string `json:"reason"`
				} `json:"errors"`
			} `json:"error"`
		}
		reason := "no reason given"
		if json.Unmarshal(body, &e) == nil && len(e.Error.Errors) > 0 {
			reason = e.Error.Errors[0].Reason
		}
		return fmt.Errorf("the Data API answered HTTP %d (%s)", resp.StatusCode, reason)
	}
	return json.Unmarshal(body, out)
}
