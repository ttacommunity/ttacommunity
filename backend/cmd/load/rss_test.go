package main

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"ttacommunity/backend/internal/rss"
	"ttacommunity/backend/internal/store"
)

// fakeRSSFeeds holds the job's RSS feeds and records what it writes.
type fakeRSSFeeds struct {
	feeds    []store.RSSFeed
	recorded map[int64]string
	failed   map[int64]string
	listErr  error
}

func (f *fakeRSSFeeds) RSSFeeds(context.Context) ([]store.RSSFeed, error) { return f.feeds, f.listErr }

func (f *fakeRSSFeeds) RecordRSSItems(_ context.Context, feed int64, title string, items []store.RSSItem, _ time.Time) (int, error) {
	f.recorded[feed] = title
	return len(items), nil
}

func (f *fakeRSSFeeds) FeedFailed(_ context.Context, feed int64, msg string, _ time.Time) error {
	f.failed[feed] = msg
	return nil
}

// fakeRSS answers each address with a feed of one item titled by the address, or with its error.
type fakeRSS struct {
	errs  map[string]error
	asked []string
}

func (r *fakeRSS) Read(_ context.Context, address string, _ time.Time) (rss.Feed, error) {
	r.asked = append(r.asked, address)
	if err := r.errs[address]; err != nil {
		return rss.Feed{}, err
	}
	return rss.Feed{Title: "Feed at " + address, Items: []store.RSSItem{{ItemID: "item-1"}}}, nil
}

func TestTheRSSJobReadsEachFeedNotPausedAndKeepsEachFailure(t *testing.T) {
	s := &fakeRSSFeeds{
		feeds: []store.RSSFeed{
			{ID: 1, URL: "https://news.example.com/feed"},
			{ID: 2, URL: "https://quiet.example.com/feed", Paused: true},
			{ID: 3, URL: "https://gone.example.com/feed"},
		},
		recorded: map[int64]string{}, failed: map[int64]string{},
	}
	r := &fakeRSS{errs: map[string]error{"https://gone.example.com/feed": errors.New("the feed answered 404")}}
	report, err := rssJob{store: s, rss: r, now: redditNow}.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v, want none: a feed's failure is recorded on it", err)
	}
	if want := []string{"https://news.example.com/feed", "https://gone.example.com/feed"}; !slices.Equal(r.asked, want) {
		t.Errorf("asked for %q, want %q", r.asked, want)
	}
	if s.recorded[1] != "Feed at https://news.example.com/feed" || s.failed[3] != "the feed answered 404" || len(s.recorded) != 1 || len(s.failed) != 1 {
		t.Errorf("recorded %v, failed %v; want feed 1 recorded with its title and feed 3's 404 kept", s.recorded, s.failed)
	}
	if want := (feedsReport{Feeds: 3, Read: 1, Failed: 1, Queued: 1}); report != want {
		t.Errorf("report = %+v, want %+v", report, want)
	}
}

func TestTheRSSJobFailsWhenItCannotReadItsFeeds(t *testing.T) {
	s := &fakeRSSFeeds{listErr: errors.New("database down")}
	if _, err := (rssJob{store: s, rss: &fakeRSS{}, now: redditNow}).run(context.Background()); err == nil {
		t.Error("run: no error, want the store's")
	}
}
