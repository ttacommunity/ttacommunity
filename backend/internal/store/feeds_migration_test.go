//go:build integration

package store

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// migrateTo moves pool's ratings schema down or up to version.
func migrateTo(t *testing.T, pool *pgxpool.Pool, version int64, down bool) {
	t.Helper()
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error {
		var err error
		if down {
			_, err = p.DownTo(context.Background(), version)
		} else {
			_, err = p.UpTo(context.Background(), version)
		}
		return err
	}); err != nil {
		t.Fatalf("migrating to %d: %v", version, err)
	}
}

func TestMigration93DropsOnlyTwitchGoingDown(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	feed := twitchFeed(t, a, "some_streamer", "100000001")
	subscribed(t, a, "Twitch", 1, FeedOptions{Feed: feed, Keywords: []string{"tta"}, WasLive: true})
	if err := a.TwitchStreamOnline(ctx, "msg-1", stream("1", "Friday games", ""), twitchAt); err != nil {
		t.Fatalf("TwitchStreamOnline: %v", err)
	}
	migrateTo(t, pool, 94, true)
	if _, err := pool.Exec(ctx, "INSERT INTO feeds (kind, source, created_at) VALUES ('reddit', 'throughtheages', now())"); err != nil {
		t.Fatalf("adding a Reddit feed at 94: %v", err)
	}
	migrateTo(t, pool, 92, true)
	if got := rowsOf(t, pool, "SELECT kind || ' ' || source FROM feeds ORDER BY feed_id"); len(got) != 1 || got[0] != "reddit throughtheages" {
		t.Errorf("feeds at 92 = %q, want only the Reddit feed", got)
	}
	migrateTo(t, pool, 95, false)
}

func TestMigration95DropsRedditAndKeepsEveryOtherFeedAndEndpoint(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	rss := rssFeed(t, a, 1)
	streamer := twitchFeed(t, a, "some_streamer", "100000001")
	recordItems(t, a, rss, rssAt)
	recordItems(t, a, rss, rssAt.Add(time.Minute), item(1, "Queued", ""))
	if _, err := a.AddNotificationEndpoint(ctx, NotificationEndpoint{Name: "News", URL: webhook(1), Format: "discord", Events: []string{"feed_failing"},
		Feeds: []int64{rss, streamer}, FeedOptions: []FeedOptions{{Feed: rss, Keywords: []string{"tta"}}, {Feed: streamer, WasLive: true}},
		Look: EmbedLook{Colour: "#123abc", Text: "short", Image: false}}, rssAt); err != nil {
		t.Fatalf("AddNotificationEndpoint: %v", err)
	}
	const state = `SELECT kind || ' ' || source FROM feeds
		UNION ALL SELECT 'item ' || item_id FROM rss_items
		UNION ALL SELECT 'subscription ' || feed_id || ' ' || array_to_string(keywords, ',') || ' ' || was_live FROM notification_feed_subscriptions
		UNION ALL SELECT 'endpoint ' || name || ' ' || array_to_string(events, ',') || ' ' || colour || ' ' || show_text || ' ' || show_image FROM notification_endpoints
		UNION ALL SELECT 'outbox ' || kind FROM notification_outbox ORDER BY 1`
	want := rowsOf(t, pool, state)
	migrateTo(t, pool, 94, true)
	for _, q := range []string{
		"INSERT INTO feeds (kind, source, created_at) VALUES ('reddit', 'throughtheages', now())",
		`INSERT INTO reddit_posts (feed_id, post_id, kind, published_at, recorded_at)
			SELECT feed_id, 't3_abc1', 'text', now(), now() FROM feeds WHERE kind = 'reddit'`,
		`INSERT INTO notification_outbox (kind, reddit_post_id, created_at, next_at) SELECT 'reddit_post', reddit_post_id, now(), now() FROM reddit_posts`,
		`INSERT INTO notification_feed_subscriptions (endpoint_id, feed_id) SELECT e.endpoint_id, f.feed_id FROM notification_endpoints e, feeds f WHERE f.kind = 'reddit'`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("adding Reddit rows at 94: %v", err)
		}
	}
	migrateTo(t, pool, 95, false)
	if got := rowsOf(t, pool, state); !slices.Equal(got, want) {
		t.Errorf("after 95 = %q, want %q: Reddit gone and the rest as before", got, want)
	}
	if got := rowsOf(t, pool, "SELECT column_name::text FROM information_schema.columns WHERE table_name IN ('feeds', 'notification_endpoints', 'notification_outbox') AND column_name IN ('post_kinds', 'show_author', 'reddit_post_id')"); len(got) != 0 {
		t.Errorf("Reddit's columns after 95 = %q, want none", got)
	}
}
