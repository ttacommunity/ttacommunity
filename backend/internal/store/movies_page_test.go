//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

const (
	aliceChannel = "UC0000000000000000000001"
	bobChannel   = "UC0000000000000000000002"
)

// moviesFixture links Alice's channel (30 shown videos, an hour apart, titled "Game 01"…) and Bob's unlinked one
// (2 shown, 1 not TTA), and returns a reader.
func moviesFixture(t *testing.T) *RatingsReader {
	t.Helper()
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "Alice")
	a := NewAdminStore(asAdminLogin(t, pool))
	now := time.Date(2026, 11, 14, 15, 0, 0, 0, time.UTC)
	alice := "Alice"
	if _, err := a.AddMovieChannel(ctx, aliceChannel, "Alice Casts", &alice, false, now); err != nil {
		t.Fatalf("AddMovieChannel Alice: %v", err)
	}
	if _, err := a.AddMovieChannel(ctx, bobChannel, "Bob Plays", nil, false, now); err != nil {
		t.Fatalf("AddMovieChannel Bob: %v", err)
	}
	save := func(id, channel, title string, at time.Time, tta bool) {
		v := PushedVideo{VideoID: id, ChannelID: channel, Title: title, PublishedAt: at, ThumbnailURL: "x", TTA: tta, Reason: "invented"}
		if err := a.SaveMovieVideo(ctx, v, now); err != nil {
			t.Fatalf("SaveMovieVideo %s: %v", id, err)
		}
	}
	for i := 1; i <= 30; i++ {
		save(fmt.Sprintf("alice%06d", i), aliceChannel, fmt.Sprintf("Game %02d", i), now.Add(time.Duration(i-40)*time.Hour), true)
	}
	save("bob00000001", bobChannel, "Another Opening", now.Add(-time.Minute), true)
	save("bob00000002", bobChannel, "Zebra Strategy", now.Add(-100*time.Hour), true)
	save("bob00000003", bobChannel, "Chess Stream", now, false)
	return NewRatingsReader(asReader(t, pool))
}

func TestMoviesPagesSortsAndFilters(t *testing.T) {
	pub := moviesFixture(t)
	tests := []struct {
		name                  string
		q                     MoviesQuery
		wantPage, wantPages   int
		wantTotal, wantOnPage int
		wantFirst             string
	}{
		{"every creator, newest first", MoviesQuery{}, 1, 2, 32, 24, "bob00000001"},
		{"the second page holds the rest", MoviesQuery{Page: 2}, 2, 2, 32, 8, "alice000007"},
		{"a page past the last gives the last", MoviesQuery{Page: 9}, 2, 2, 32, 8, "alice000007"},
		{"oldest first", MoviesQuery{Sort: "oldest"}, 1, 2, 32, 24, "bob00000002"},
		{"by title", MoviesQuery{Sort: "title"}, 1, 2, 32, 24, "bob00000001"},
		{"by creator, then newest", MoviesQuery{Sort: "creator"}, 1, 2, 32, 24, "alice000030"},
		{"one creator", MoviesQuery{Channels: []string{bobChannel}}, 1, 1, 2, 2, "bob00000001"},
		{"a player's channels, in any case", MoviesQuery{Player: "aLiCe"}, 1, 2, 30, 24, "alice000030"},
		{"a title search, in any case", MoviesQuery{Search: "zEBRA"}, 1, 1, 1, 1, "bob00000002"},
		{"a search and a creator", MoviesQuery{Search: "game 2", Channels: []string{aliceChannel}}, 1, 1, 10, 10, "alice000029"},
		{"a search for a wildcard is literal", MoviesQuery{Search: "%"}, 1, 1, 0, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feed, err := pub.Movies(context.Background(), tt.q)
			if err != nil {
				t.Fatalf("Movies(%+v): %v", tt.q, err)
			}
			if feed.Page != tt.wantPage || feed.Pages != tt.wantPages || feed.Total != tt.wantTotal {
				t.Errorf("got page %d of %d, total %d, want %d of %d, total %d", feed.Page, feed.Pages, feed.Total, tt.wantPage, tt.wantPages, tt.wantTotal)
			}
			if len(feed.Videos) != tt.wantOnPage {
				t.Fatalf("got %d videos on the page, want %d", len(feed.Videos), tt.wantOnPage)
			}
			if tt.wantOnPage == 0 {
				return
			}
			if got := feed.Videos[0].VideoID; got != tt.wantFirst {
				t.Errorf("got first = %q, want %q", got, tt.wantFirst)
			}
		})
	}
}

func TestMoviesCountsEveryVideoOfEachCreator(t *testing.T) {
	pub := moviesFixture(t)
	feed, err := pub.Movies(context.Background(), MoviesQuery{Channels: []string{bobChannel}, Page: 1})
	if err != nil {
		t.Fatalf("Movies: %v", err)
	}
	got := map[string]int{}
	for _, c := range feed.Creators {
		got[c.ChannelID] = c.Videos
	}
	if got[aliceChannel] != 30 || got[bobChannel] != 2 {
		t.Errorf("got counts %v, want Alice 30 and Bob 2", got)
	}
}

func TestMoviesCountsWhatTheSearchFinds(t *testing.T) {
	pub := moviesFixture(t)
	feed, err := pub.Movies(context.Background(), MoviesQuery{Search: "strategy"})
	if err != nil {
		t.Fatalf("Movies: %v", err)
	}
	got := map[string]int{}
	for _, c := range feed.Creators {
		got[c.ChannelID] = c.Videos
	}
	if got[aliceChannel] != 0 || got[bobChannel] != 1 || len(feed.Creators) != 2 {
		t.Errorf("got counts %v, want both creators, Alice 0 and Bob 1", got)
	}
}

func TestMoviesPagesNeverOverlap(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	a := NewAdminStore(asAdminLogin(t, pool))
	now := time.Date(2026, 11, 14, 15, 0, 0, 0, time.UTC)
	if _, err := a.AddMovieChannel(ctx, aliceChannel, "Alice Casts", nil, false, now); err != nil {
		t.Fatalf("AddMovieChannel: %v", err)
	}
	for i := range 100 {
		v := PushedVideo{VideoID: fmt.Sprintf("same%07d", i), ChannelID: aliceChannel, Title: "Same Title", PublishedAt: now, ThumbnailURL: "x", TTA: true, Reason: "invented"}
		if err := a.SaveMovieVideo(ctx, v, now); err != nil {
			t.Fatalf("SaveMovieVideo: %v", err)
		}
	}
	pub := NewRatingsReader(asReader(t, pool))
	for _, sort := range []string{"newest", "oldest", "title", "creator"} {
		seen := map[string]bool{}
		for page := 1; page <= 5; page++ {
			feed, err := pub.Movies(ctx, MoviesQuery{Sort: sort, Page: page})
			if err != nil {
				t.Fatalf("Movies(%s, page %d): %v", sort, page, err)
			}
			for _, v := range feed.Videos {
				seen[v.VideoID] = true
			}
		}
		if len(seen) != 100 {
			t.Errorf("sort %s: got %d distinct videos over every page, want 100", sort, len(seen))
		}
	}
}

func TestMoviesRefusesAnUnknownCreator(t *testing.T) {
	pub := moviesFixture(t)
	for _, q := range []MoviesQuery{
		{Channels: []string{"UC9999999999999999999999"}},
		{Channels: []string{bobChannel, "UC9999999999999999999999"}},
		{Player: "Nobody"},
	} {
		if _, err := pub.Movies(context.Background(), q); !errors.Is(err, ErrNoChannel) {
			t.Errorf("Movies(%+v) error = %v, want ErrNoChannel", q, err)
		}
	}
}
