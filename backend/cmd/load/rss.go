// The feeds job's run: it reads each RSS or Atom feed and queues the items not seen before, for the api to post
// to the endpoints taking that feed. A feed that cannot be read keeps its failure, shown in admin, and the job
// carries on.
package main

import (
	"context"
	"time"

	"ttacommunity/backend/internal/rss"
	"ttacommunity/backend/internal/store"
)

// rssJobStore is what the job reads and writes, through the admin login.
type rssJobStore interface {
	RSSFeeds(ctx context.Context) ([]store.RSSFeed, error)
	RecordRSSItems(ctx context.Context, feed int64, title string, items []store.RSSItem, now time.Time) (int, error)
	FeedFailed(ctx context.Context, feed int64, msg string, now time.Time) error
}

// rssReader reads a feed at an address.
type rssReader interface {
	Read(ctx context.Context, address string, now time.Time) (rss.Feed, error)
}

// rssJob is one run of the job at now.
type rssJob struct {
	store rssJobStore
	rss   rssReader
	now   time.Time
}

// run reads every feed not paused. Only the database failing is run's error: a feed failing is recorded on it,
// which tells the admins taking feed_failing after a few in a row.
func (j rssJob) run(ctx context.Context) (feedsReport, error) {
	var r feedsReport
	feeds, err := j.store.RSSFeeds(ctx)
	if err != nil {
		return r, err
	}
	r.Feeds = len(feeds)
	for _, f := range feeds {
		if f.Paused {
			continue
		}
		feed, err := j.rss.Read(ctx, f.URL, j.now)
		if err != nil {
			r.Failed++
			if err := j.store.FeedFailed(ctx, f.ID, err.Error(), j.now); err != nil {
				return r, err
			}
			continue
		}
		queued, err := j.store.RecordRSSItems(ctx, f.ID, feed.Title, feed.Items, j.now)
		if err != nil {
			return r, err
		}
		r.Read++
		r.Queued += queued
	}
	return r, nil
}
