//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
)

// rssAt is when the tests' RSS feeds are read.
var rssAt = time.Date(2026, 11, 18, 1, 0, 0, 0, time.UTC)

// rssFeed adds an RSS feed read from an invented address numbered n, and returns its id.
func rssFeed(t *testing.T, a *AdminStore, n int) int64 {
	t.Helper()
	id, err := a.AddRSSFeed(context.Background(), fmt.Sprintf("https://news.example.com/feed-%d.xml", n), rssAt)
	if err != nil {
		t.Fatalf("AddRSSFeed: %v", err)
	}
	return id
}

// item is an invented item numbered n, with its title and summary.
func item(n int, title, summary string) RSSItem {
	return RSSItem{ItemID: fmt.Sprintf("https://news.example.com/item-%d", n), Title: title, URL: fmt.Sprintf("https://news.example.com/item-%d", n),
		Summary: summary, ImageURL: "https://news.example.com/item.png", PublishedAt: rssAt.Add(-time.Duration(n) * time.Minute)}
}

// recordItems records items for feed at now, titled "Example news", and returns how many it queued.
func recordItems(t *testing.T, a *AdminStore, feed int64, now time.Time, items ...RSSItem) int {
	t.Helper()
	queued, err := a.RecordRSSItems(context.Background(), feed, "Example news", items, now)
	if err != nil {
		t.Fatalf("RecordRSSItems: %v", err)
	}
	return queued
}

// sentItems sends the outbox at now and returns each item sent, as "endpoint:feed:title".
func sentItems(t *testing.T, a *AdminStore, now time.Time) []string {
	t.Helper()
	var got []string
	if _, err := a.SendNotifications(context.Background(), now, 10, func(_ context.Context, n Notification, e NotificationEndpoint) error {
		got = append(got, fmt.Sprintf("%s:%s:%s", e.Name, n.RSS.Feed, n.RSS.Item.Title))
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	slices.Sort(got)
	return got
}

func TestAnRSSFeedsFirstReadPostsNothingAndLaterOnesPostEachNewItemOnce(t *testing.T) {
	a := redditStore(t)
	feed := rssFeed(t, a, 1)
	subscribed(t, a, "News", 1, FeedOptions{Feed: feed})
	if got := recordItems(t, a, feed, rssAt, item(1, "One", ""), item(2, "Two", "")); got != 0 {
		t.Errorf("first read queued %d, want 0: a new feed posts no backlog", got)
	}
	if got := recordItems(t, a, feed, rssAt.Add(10*time.Minute), item(3, "Three", ""), item(1, "One", "")); got != 1 {
		t.Errorf("second read queued %d, want 1 (item 3 only)", got)
	}
	if got := recordItems(t, a, feed, rssAt.Add(20*time.Minute), item(3, "Three", "")); got != 0 {
		t.Errorf("third read queued %d, want 0: item 3 is queued once", got)
	}
	if got, want := sentItems(t, a, rssAt.Add(time.Hour)), []string{"News:Example news:Three"}; !slices.Equal(got, want) {
		t.Errorf("sent %q, want %q", got, want)
	}
}

func TestAResumedRSSFeedPostsNothingMissedWhilePaused(t *testing.T) {
	ctx := context.Background()
	a := redditStore(t)
	feed := rssFeed(t, a, 1)
	subscribed(t, a, "News", 1, FeedOptions{Feed: feed})
	recordItems(t, a, feed, rssAt)
	for _, paused := range []bool{true, false} {
		if err := a.SetRSSFeed(ctx, feed, paused); err != nil {
			t.Fatalf("SetRSSFeed(%v): %v", paused, err)
		}
	}
	if got := recordItems(t, a, feed, rssAt.Add(time.Hour), item(1, "Came while paused", "")); got != 0 {
		t.Errorf("the read after resuming queued %d, want 0", got)
	}
	if got := recordItems(t, a, feed, rssAt.Add(2*time.Hour), item(2, "New", "")); got != 1 {
		t.Errorf("the read after that queued %d, want 1", got)
	}
}

func TestAnRSSItemIsPostedOnlyWhereItsKeywordsMatch(t *testing.T) {
	a := redditStore(t)
	feed := rssFeed(t, a, 1)
	subscribed(t, a, "All", 1, FeedOptions{Feed: feed})
	subscribed(t, a, "TTA", 2, FeedOptions{Feed: feed, Keywords: []string{"Through the Ages", "tta"}})
	recordItems(t, a, feed, rssAt)
	recordItems(t, a, feed, rssAt.Add(time.Minute),
		item(1, "A TTA tournament", ""), item(2, "Weekly news", "Through the ages gets an update"), item(3, "Other games", "Nothing here"))
	want := []string{"All:Example news:A TTA tournament", "All:Example news:Other games", "All:Example news:Weekly news",
		"TTA:Example news:A TTA tournament", "TTA:Example news:Weekly news"}
	if got := sentItems(t, a, rssAt.Add(time.Hour)); !slices.Equal(got, want) {
		t.Errorf("sent %q, want %q", got, want)
	}
}

func TestAnRSSNotificationCarriesTheItemAndAnRSSFeedShowsItsState(t *testing.T) {
	ctx := context.Background()
	a := redditStore(t)
	feed := rssFeed(t, a, 1)
	subscribed(t, a, "News", 1, FeedOptions{Feed: feed})
	recordItems(t, a, feed, rssAt)
	it := item(1, "A title", "A summary")
	recordItems(t, a, feed, rssAt.Add(time.Minute), it)
	var got Notification
	if _, err := a.SendNotifications(ctx, rssAt.Add(time.Minute), 10, func(_ context.Context, n Notification, _ NotificationEndpoint) error {
		got = n
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	gotAt := got.RSS.Item.PublishedAt
	got.RSS.Item.PublishedAt = it.PublishedAt
	if got.Kind != "rss_item" || got.FeedID != feed || got.RSS.Feed != "Example news" || got.RSS.Item != it || !gotAt.Equal(it.PublishedAt) {
		t.Errorf("notification = %s feed %d %+v, want rss_item feed %d %+v from Example news", got.Kind, got.FeedID, got.RSS, feed, it)
	}
	feeds, err := a.RSSFeeds(ctx)
	if err != nil || len(feeds) != 1 {
		t.Fatalf("RSSFeeds = %+v, %v; want the one feed", feeds, err)
	}
	if f := feeds[0]; f.Title != "Example news" || f.CheckedAt == nil || !f.CheckedAt.Equal(rssAt.Add(time.Minute)) || f.LastItemAt == nil || !f.LastItemAt.Equal(it.PublishedAt) {
		t.Errorf("feed = %+v, want titled Example news, read at %v, newest item %v", f, rssAt.Add(time.Minute), it.PublishedAt)
	}
}

func TestAnRSSItemsTextIsKeptOnlyUntilItIsPosted(t *testing.T) {
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	feed := rssFeed(t, a, 1)
	subscribed(t, a, "News", 1, FeedOptions{Feed: feed})
	const kept = "SELECT item_id || ':' || title || url || summary || image_url FROM rss_items ORDER BY item_id"
	recordItems(t, a, feed, rssAt, item(1, "One", "Hello"))
	if got, want := rowsOf(t, pool, kept), []string{"https://news.example.com/item-1:"}; !slices.Equal(got, want) {
		t.Errorf("after the first read, kept %q, want %q: an item never posted keeps no text", got, want)
	}
	recordItems(t, a, feed, rssAt.Add(time.Minute), item(2, "Two", "Hello"))
	sentItems(t, a, rssAt.Add(time.Minute))
	if got, want := rowsOf(t, pool, kept), []string{"https://news.example.com/item-1:", "https://news.example.com/item-2:"}; !slices.Equal(got, want) {
		t.Errorf("after sending, kept %q, want %q", got, want)
	}
}

func TestRSSFeedsRefuseWhatTheDatabaseRefuses(t *testing.T) {
	ctx := context.Background()
	a := redditStore(t)
	rssFeed(t, a, 1)
	for _, tt := range []struct {
		name, url string
		want      error
	}{
		{"http", "http://news.example.com/feed.xml", ErrBadRSSFeed},
		{"no scheme", "news.example.com/feed.xml", ErrBadRSSFeed},
		{"a password", "https://user:secret@news.example.com/feed.xml", ErrBadRSSFeed},
		{"a space", "https://news.example.com/my feed.xml", ErrBadRSSFeed},
		{"too long", "https://news.example.com/" + strings.Repeat("a", 2000), ErrBadRSSFeed},
		{"one already there, in other letters", "https://NEWS.example.com/feed-1.xml", ErrRSSFeedExists},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := a.AddRSSFeed(ctx, tt.url, rssAt); !errors.Is(err, tt.want) {
				t.Errorf("AddRSSFeed(%q): err = %v, want %v", tt.url, err, tt.want)
			}
		})
	}
	for name, err := range map[string]error{
		"pausing":  a.SetRSSFeed(ctx, 999, true),
		"removing": a.RemoveRSSFeed(ctx, 999),
	} {
		if !errors.Is(err, ErrNoRSSFeed) {
			t.Errorf("%s an unknown feed: err = %v, want %v", name, err, ErrNoRSSFeed)
		}
	}
	reddit := addFeed(t, a, "example")
	if err := a.SetRSSFeed(ctx, reddit, true); !errors.Is(err, ErrNoRSSFeed) {
		t.Errorf("pausing a Reddit feed as RSS: err = %v, want %v", err, ErrNoRSSFeed)
	}
}

func TestAnRSSFeedFailingIsNotifiedByItsName(t *testing.T) {
	ctx := context.Background()
	a := redditStore(t)
	feed := rssFeed(t, a, 1)
	endpoint(t, a, "Ops", 1, "feed_failing")
	for range FeedFailuresNotified {
		if err := a.FeedFailed(ctx, feed, "the feed answered 404", rssAt); err != nil {
			t.Fatalf("FeedFailed: %v", err)
		}
	}
	recordItems(t, a, feed, rssAt.Add(time.Minute))
	var got []string
	if _, err := a.SendNotifications(ctx, rssAt.Add(time.Hour), 10, func(_ context.Context, n Notification, _ NotificationEndpoint) error {
		got = append(got, fmt.Sprintf("%s %s %s %q %q", n.Kind, n.FeedKind, n.FeedName, n.FeedError, n.Reddit.Subreddit))
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	// Its error is cleared by the good read before the notice is sent.
	want := []string{`feed_failed rss Example news "" ""`, `feed_recovered rss Example news "" ""`}
	if !slices.Equal(got, want) {
		t.Errorf("sent %q, want %q", got, want)
	}
	if err := a.FeedFailed(ctx, 999, "no", rssAt); !errors.Is(err, ErrNoFeed) {
		t.Errorf("FeedFailed of an unknown feed: err = %v, want %v", err, ErrNoFeed)
	}
}

func TestMigration94DropsOnlyRSSGoingDownAndKeepsFeedFailing(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	addFeed(t, a, "throughtheages")
	feed := rssFeed(t, a, 1)
	recordItems(t, a, feed, rssAt)
	recordItems(t, a, feed, rssAt.Add(time.Minute), item(1, "Queued", ""))
	endpoint(t, a, "Ops", 1, "feed_failing")
	const state = "SELECT kind || ' ' || source FROM feeds UNION ALL SELECT array_to_string(events, ',') FROM notification_endpoints ORDER BY 1"
	steps := []struct {
		to   int64
		want []string
	}{
		{93, []string{"reddit throughtheages", "reddit_failing"}},
		{94, []string{"feed_failing", "reddit throughtheages"}},
	}
	for _, s := range steps {
		if err := withRatingsMigrations(pool, func(p *goose.Provider) error {
			if s.to == 93 {
				_, err := p.DownTo(ctx, s.to)
				return err
			}
			_, err := p.UpTo(ctx, s.to)
			return err
		}); err != nil {
			t.Fatalf("migrating to %d: %v", s.to, err)
		}
		if got := rowsOf(t, pool, state); !slices.Equal(got, s.want) {
			t.Errorf("at %d: %q, want %q", s.to, got, s.want)
		}
	}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.Up(ctx); return err }); err != nil {
		t.Fatalf("migrating up: %v", err)
	}
}
