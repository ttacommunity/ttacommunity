//go:build integration

package store

import (
	"context"
	"testing"
	"time"

	"ttacommunity/backend/internal/movies"
)

type stubMovieFetcher struct {
	entries  map[string][]movies.Entry
	keywords map[string]string
}

func (f stubMovieFetcher) FetchChannel(_ context.Context, channelID string) ([]movies.Entry, error) {
	return f.entries[channelID], nil
}

func (f stubMovieFetcher) FetchKeywords(_ context.Context, videoID string) (string, error) {
	return f.keywords[videoID], nil
}

func TestMoviesChannelsSyncFeedAndHide(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "Alice")
	a := NewAdminStore(asAdminLogin(t, pool))
	pub := NewRatingsReader(asReader(t, pool))
	now := time.Date(2026, 11, 14, 15, 0, 0, 0, time.UTC)

	player := "Alice"
	ch, err := a.AddMovieChannel(ctx, "UC0l8n6q2SsB-Dg3BEvrX_Xg", "Alice Casts", &player, false, now)
	if err != nil {
		t.Fatalf("AddMovieChannel: %v", err)
	}
	if ch.Player == nil || *ch.Player != "Alice" {
		t.Fatalf("got player = %v, want Alice", ch.Player)
	}

	fetcher := stubMovieFetcher{
		entries: map[string][]movies.Entry{
			ch.ChannelID: {
				{VideoID: "vid00000001", Title: "Through the Ages Final", PublishedAt: now.Add(-time.Hour), ThumbnailURL: "https://i.ytimg.com/vi/vid00000001/hqdefault.jpg"},
				{VideoID: "vid00000002", Title: "Random Chess Stream", PublishedAt: now, ThumbnailURL: "https://i.ytimg.com/vi/vid00000002/hqdefault.jpg"},
			},
		},
	}
	res, err := a.SyncMovies(ctx, fetcher, now)
	if err != nil {
		t.Fatalf("SyncMovies: %v", err)
	}
	if res.Checked != 1 || res.Added != 1 {
		t.Fatalf("got sync %+v, want {Checked:1 Added:1}", res)
	}

	feed, err := pub.Movies(ctx)
	if err != nil {
		t.Fatalf("Movies: %v", err)
	}
	if len(feed.Videos) != 1 || feed.Videos[0].VideoID != "vid00000001" {
		t.Fatalf("got feed videos %+v, want vid00000001", feed.Videos)
	}

	if err := a.SetMovieVideoHidden(ctx, "vid00000001", true); err != nil {
		t.Fatalf("SetMovieVideoHidden: %v", err)
	}
	feed, err = pub.Movies(ctx)
	if err != nil {
		t.Fatalf("Movies after hide: %v", err)
	}
	if len(feed.Videos) != 0 {
		t.Fatalf("got %d videos after hide, want 0", len(feed.Videos))
	}
}
