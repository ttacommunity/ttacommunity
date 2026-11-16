package movies

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

const inventedKey = "invented-api-key-0000000000000000000000"

// fakeDataAPI answers like the Data API: videos.list with the ids it knows, channels.list?forHandle with
// handles, and 403 to a request without the key in its header.
func fakeDataAPI(t *testing.T, requests *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r.URL.Path+"?"+r.URL.RawQuery)
		if r.Header.Get("X-Goog-Api-Key") != inventedKey || r.URL.Query().Has("key") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{"error":{"code":403,"errors":[{"reason":"forbidden"}]}}`)
			return
		}
		switch r.URL.Path {
		case "/videos":
			var items []string
			for _, id := range strings.Split(r.URL.Query().Get("id"), ",") {
				if id == "vid00000001" {
					items = append(items, `{"id":"vid00000001","snippet":{"channelId":"UC0000000000000000000001",`+
						`"title":"A final","description":"Through the Ages, game 3","tags":["boardgame","strategy"],`+
						`"publishedAt":"2026-11-14T14:17:33Z"}}`)
				}
			}
			_, _ = fmt.Fprintf(w, `{"items":[%s]}`, strings.Join(items, ","))
		case "/channels":
			if r.URL.Query().Get("forHandle") == "@InventedCaster" {
				_, _ = fmt.Fprint(w, `{"items":[{"id":"UC0000000000000000000001"}]}`)
				return
			}
			if r.URL.Query().Get("id") == "UC0000000000000000000001" {
				_, _ = fmt.Fprint(w, `{"items":[{"id":"UC0000000000000000000001","contentDetails":{"relatedPlaylists":{"uploads":"UU0000000000000000000001"}}}]}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"kind":"youtube#channelListResponse"}`)
		case "/playlistItems":
			if r.URL.Query().Get("playlistId") != "UU0000000000000000000001" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprint(w, `{"error":{"code":404,"errors":[{"reason":"playlistNotFound"}]}}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"items":[{"contentDetails":{"videoId":"vid00000002"}},{"contentDetails":{"videoId":"vid00000001"}}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestYouTubeVideos(t *testing.T) {
	var requests []string
	y := NewYouTube(nil, fakeDataAPI(t, &requests).URL, inventedKey)
	got, err := y.Videos(context.Background(), []string{"vid00000001", "vid00000009"})
	if err != nil {
		t.Fatalf("Videos: %v", err)
	}
	v, ok := got["vid00000001"]
	if !ok {
		t.Fatalf("got %v, want vid00000001's details", got)
	}
	if v.Description != "Through the Ages, game 3" || !slices.Equal(v.Tags, []string{"boardgame", "strategy"}) || v.ChannelID != "UC0000000000000000000001" {
		t.Errorf("got = %+v, want the description, tags and channel", v)
	}
	if want := time.Date(2026, 11, 14, 14, 17, 33, 0, time.UTC); !v.PublishedAt.Equal(want) {
		t.Errorf("PublishedAt = %v, want %v", v.PublishedAt, want)
	}
	if _, ok := got["vid00000009"]; ok {
		t.Errorf("got a video YouTube did not return: %+v", got["vid00000009"])
	}
	if want := []string{"/videos?id=vid00000001%2Cvid00000009&part=snippet"}; !slices.Equal(requests, want) {
		t.Errorf("requests = %q, want %q", requests, want)
	}
}

func TestYouTubeVideosRefusesMoreThanOnePage(t *testing.T) {
	var requests []string
	y := NewYouTube(nil, fakeDataAPI(t, &requests).URL, inventedKey)
	ids := make([]string, 51)
	for i := range ids {
		ids[i] = fmt.Sprintf("vid%08d", i)
	}
	if _, err := y.Videos(context.Background(), ids); err == nil {
		t.Error("Videos took 51 ids, want an error")
	}
	if len(requests) != 0 {
		t.Errorf("requests = %q, want none", requests)
	}
}

func TestYouTubeErrorsNeverCarryTheKey(t *testing.T) {
	var requests []string
	y := NewYouTube(nil, fakeDataAPI(t, &requests).URL, inventedKey+"-wrong")
	_, err := y.Videos(context.Background(), []string{"vid00000001"})
	if err == nil {
		t.Fatal("Videos with a refused key succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("err = %q, want the status and Google's reason", err)
	}
	for _, s := range []string{err.Error(), y.String(), fmt.Sprintf("%v %+v", y, y)} {
		if strings.Contains(s, "invented-api-key") {
			t.Errorf("%q shows the key", s)
		}
	}
}

func TestYouTubeResolveChannel(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		want         string
		wantErr      error
		wantRequests int
	}{
		{"a channel id", " UC0000000000000000000002 ", "UC0000000000000000000002", nil, 0},
		{"a /channel/ address", "https://www.youtube.com/channel/UC0000000000000000000002", "UC0000000000000000000002", nil, 0},
		{"a bare handle", "@InventedCaster", "UC0000000000000000000001", nil, 1},
		{"a handle's address", "https://www.youtube.com/@InventedCaster/videos", "UC0000000000000000000001", nil, 1},
		{"a phone handle's address", "https://m.youtube.com/@InventedCaster", "UC0000000000000000000001", nil, 1},
		{"a handle with no channel", "@NobodyInvented", "", ErrNoSuchChannel, 1},
		{"another site", "https://example.com/@InventedCaster", "", ErrNotAChannel, 0},
		{"a video address", "https://www.youtube.com/watch?v=vid00000001", "", ErrNotAChannel, 0},
		{"a handle too short", "@ab", "", ErrNotAChannel, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests []string
			y := NewYouTube(nil, fakeDataAPI(t, &requests).URL, inventedKey)
			got, err := y.ResolveChannel(context.Background(), tt.raw)
			if !errors.Is(err, tt.wantErr) || got != tt.want {
				t.Errorf("ResolveChannel(%q) = %q, %v; want %q, %v", tt.raw, got, err, tt.want, tt.wantErr)
			}
			if len(requests) != tt.wantRequests {
				t.Errorf("requests = %q, want %d", requests, tt.wantRequests)
			}
		})
	}
}

func TestResolveChannelWithoutAKeyTakesOnlyIDs(t *testing.T) {
	var y *YouTube
	if got, err := y.ResolveChannel(context.Background(), "UC0000000000000000000002"); err != nil || got != "UC0000000000000000000002" {
		t.Errorf("a channel id = %q, %v; want it back", got, err)
	}
	if _, err := y.ResolveChannel(context.Background(), "@InventedCaster"); !errors.Is(err, ErrNoYouTube) {
		t.Errorf("a handle: err = %v, want %v", err, ErrNoYouTube)
	}
}

func TestYouTubeLatestUploads(t *testing.T) {
	tests := []struct {
		name         string
		channel      string
		want         []string
		wantErr      bool
		wantRequests []string
	}{
		{"a channel's newest uploads", "UC0000000000000000000001", []string{"vid00000002", "vid00000001"}, false, []string{
			"/channels?id=UC0000000000000000000001&part=contentDetails",
			"/playlistItems?maxResults=15&part=contentDetails&playlistId=UU0000000000000000000001",
		}},
		{"a channel YouTube does not have", "UC0000000000000000000009", nil, true, []string{
			"/channels?id=UC0000000000000000000009&part=contentDetails",
		}},
		{"not a channel id", "not-a-channel", nil, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests []string
			y := NewYouTube(nil, fakeDataAPI(t, &requests).URL, inventedKey)
			got, err := y.LatestUploads(context.Background(), tt.channel, 15)
			if (err != nil) != tt.wantErr || !slices.Equal(got, tt.want) {
				t.Errorf("LatestUploads(%q) = %q, %v; want %q, an error: %v", tt.channel, got, err, tt.want, tt.wantErr)
			}
			if !slices.Equal(requests, tt.wantRequests) {
				t.Errorf("requests = %q, want %q", requests, tt.wantRequests)
			}
		})
	}
}
