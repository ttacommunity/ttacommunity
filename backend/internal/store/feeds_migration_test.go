//go:build integration

package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

func TestMigration92KeepsRedditFeedsDownAndUp(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	feed := addFeed(t, a, "throughtheages")
	if err := a.SetRedditFeed(ctx, feed, false, []string{"text", "link"}); err != nil {
		t.Fatalf("SetRedditFeed: %v", err)
	}
	record(t, a, feed, redditAt, post(1))
	if _, err := a.AddNotificationEndpoint(ctx, NotificationEndpoint{Name: "Reddit", URL: webhook(1), Format: "discord", Feeds: []int64{feed}}, redditAt); err != nil {
		t.Fatalf("AddNotificationEndpoint: %v", err)
	}
	want := feedState(t, pool)
	for _, step := range []func(p *goose.Provider) error{
		func(p *goose.Provider) error { _, err := p.DownTo(ctx, 91); return err },
		func(p *goose.Provider) error { _, err := p.Up(ctx); return err },
	} {
		if err := withRatingsMigrations(pool, step); err != nil {
			t.Fatalf("migrating: %v", err)
		}
	}
	if got := feedState(t, pool); len(got) != len(want) || got[0] != want[0] {
		t.Errorf("after down and up: %q, want %q", got, want)
	}
	if _, err := a.AddRedditFeed(ctx, "ThroughTheAges", redditAt); err == nil {
		t.Errorf("AddRedditFeed of the same subreddit in other letters after down and up: no error, want ErrRedditFeedExists")
	}
}

// feedState is each Reddit feed with its kinds, posts and subscriptions, a line each, as the store reads them.
func feedState(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	a := NewAdminStore(asAdminLogin(t, pool))
	feeds, err := a.RedditFeeds(context.Background())
	if err != nil {
		t.Fatalf("RedditFeeds: %v", err)
	}
	ends, err := a.NotificationEndpoints(context.Background())
	if err != nil {
		t.Fatalf("NotificationEndpoints: %v", err)
	}
	var lines []string
	for _, f := range feeds {
		lines = append(lines, fmt.Sprintf("%s %v %v %v", f.Subreddit, f.Kinds, f.LastPostAt.UTC(), ends[0].Feeds))
	}
	return lines
}

func TestMigration93DropsOnlyTwitchGoingDown(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	reddit := addFeed(t, a, "throughtheages")
	feed := twitchFeed(t, a, "some_streamer", "100000001")
	subscribed(t, a, "Twitch", 1, FeedOptions{Feed: feed, Keywords: []string{"tta"}, WasLive: true})
	if err := a.TwitchStreamOnline(ctx, "msg-1", stream("1", "Friday games", ""), twitchAt); err != nil {
		t.Fatalf("TwitchStreamOnline: %v", err)
	}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 92); return err }); err != nil {
		t.Fatalf("migrating down to 92: %v", err)
	}
	if got := rowsOf(t, pool, "SELECT kind || ' ' || source FROM feeds ORDER BY feed_id"); len(got) != 1 || got[0] != "reddit throughtheages" {
		t.Errorf("feeds at 92 = %q, want only the Reddit feed", got)
	}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.Up(ctx); return err }); err != nil {
		t.Fatalf("migrating up: %v", err)
	}
	if feeds, err := a.RedditFeeds(ctx); err != nil || len(feeds) != 1 || feeds[0].ID != reddit {
		t.Errorf("Reddit feeds after down and up = %+v, %v; want the one feed", feeds, err)
	}
}
