//go:build integration

package store

import (
	"context"
	"testing"
	"time"
)

func TestMoviesChannelsFeedAndOverride(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "Alice")
	a := NewAdminStore(asAdminLogin(t, pool))
	pub := NewRatingsReader(asReader(t, pool))
	now := time.Date(2026, 11, 14, 15, 0, 0, 0, time.UTC)

	player := "Alice"
	ch, err := a.AddMovieChannel(ctx, "UC0000000000000000000001", "Alice Casts", &player, false, now)
	if err != nil {
		t.Fatalf("AddMovieChannel: %v", err)
	}
	if ch.Player == nil || *ch.Player != "Alice" {
		t.Fatalf("got player = %v, want Alice", ch.Player)
	}

	for _, v := range []PushedVideo{
		{VideoID: "vid00000001", ChannelID: ch.ChannelID, Title: "Through the Ages Final", PublishedAt: now.Add(-time.Hour), ThumbnailURL: "x", TTA: true, Reason: "mentions Through the Ages"},
		{VideoID: "vid00000002", ChannelID: ch.ChannelID, Title: "Random Chess Stream", PublishedAt: now, ThumbnailURL: "x", TTA: false, Reason: "no mention of TTA"},
	} {
		if err := a.SaveMovieVideo(ctx, v, now); err != nil {
			t.Fatalf("SaveMovieVideo: %v", err)
		}
	}

	feed, err := pub.Movies(ctx, MoviesQuery{})
	if err != nil {
		t.Fatalf("Movies: %v", err)
	}
	if len(feed.Videos) != 1 || feed.Videos[0].VideoID != "vid00000001" {
		t.Fatalf("got feed videos %+v, want vid00000001", feed.Videos)
	}

	off := false
	if err := a.SetMovieVideoOverride(ctx, "vid00000001", &off); err != nil {
		t.Fatalf("SetMovieVideoOverride: %v", err)
	}
	feed, err = pub.Movies(ctx, MoviesQuery{})
	if err != nil {
		t.Fatalf("Movies after the override: %v", err)
	}
	if len(feed.Videos) != 0 {
		t.Fatalf("got %d videos after the override, want 0", len(feed.Videos))
	}
}
