// The feeds job's Reddit half: it reads each subreddit's newest posts and queues those not seen before, for the api to
// post to the endpoints taking that feed. A feed that cannot be read keeps its failure, shown in admin, and the job
// carries on.
package main

import (
	"context"
	"time"

	"ttacommunity/backend/internal/store"
)

// redditJobStore is what the job reads and writes, through the admin login.
type redditJobStore interface {
	RedditFeeds(ctx context.Context) ([]store.RedditFeed, error)
	RecordRedditPosts(ctx context.Context, feed int64, posts []store.RedditPost, now time.Time) (int, error)
	FeedFailed(ctx context.Context, feed int64, msg string, now time.Time) error
}

// redditReader reads a subreddit's newest posts.
type redditReader interface {
	Newest(ctx context.Context, subreddit string) ([]store.RedditPost, error)
}

// redditJob is one run of the job at now.
type redditJob struct {
	store  redditJobStore
	reddit redditReader
	now    time.Time
}

// feedsReport counts what a run of either half did.
type feedsReport struct {
	Feeds, Read, Failed, Queued int
}

// run reads every feed not paused. Only the database failing is run's error: Reddit refusing a read is recorded
// on its feed, which tells the admins taking feed_failing after a few in a row, so a passing rate limit is quiet.
func (j redditJob) run(ctx context.Context) (feedsReport, error) {
	var r feedsReport
	feeds, err := j.store.RedditFeeds(ctx)
	if err != nil {
		return r, err
	}
	r.Feeds = len(feeds)
	for _, f := range feeds {
		if f.Paused {
			continue
		}
		posts, err := j.reddit.Newest(ctx, f.Subreddit)
		if err != nil {
			r.Failed++
			if err := j.store.FeedFailed(ctx, f.ID, err.Error(), j.now); err != nil {
				return r, err
			}
			continue
		}
		queued, err := j.store.RecordRedditPosts(ctx, f.ID, posts, j.now)
		if err != nil {
			return r, err
		}
		r.Read++
		r.Queued += queued
	}
	return r, nil
}
