// The Reddit job: it reads each feed's newest posts and queues those not seen before, for the api to post to the
// endpoints taking that feed. A feed that cannot be read keeps its failure, shown in admin, and the job carries on.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ttacommunity/backend/internal/reddit"
	"ttacommunity/backend/internal/store"
)

// redditJobStore is what the job reads and writes, through the admin login.
type redditJobStore interface {
	RedditFeeds(ctx context.Context) ([]store.RedditFeed, error)
	RecordRedditPosts(ctx context.Context, feed int64, posts []store.RedditPost, now time.Time) (int, error)
	RedditFeedFailed(ctx context.Context, feed int64, msg string, now time.Time) error
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

// redditJobReport counts what a run did.
type redditJobReport struct {
	Feeds, Read, Failed, Queued int
}

// run reads every feed not paused. Only the database failing is run's error: Reddit refusing a read is recorded
// on its feed, so a passing rate limit does not alert anyone.
func (j redditJob) run(ctx context.Context) (redditJobReport, error) {
	var r redditJobReport
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
			if err := j.store.RedditFeedFailed(ctx, f.ID, err.Error(), j.now); err != nil {
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

// runReddit runs the job at now on the admin login, from RATINGS_ADMIN_DATABASE_URL, refusing a login that can
// change more than admin edits.
func runReddit(now time.Time) error {
	url := os.Getenv("RATINGS_ADMIN_DATABASE_URL")
	if url == "" {
		return errors.New("RATINGS_ADMIN_DATABASE_URL is not set")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := store.OpenPool(ctx, url)
	if err != nil {
		return errors.New("opening the admin pool failed")
	}
	defer pool.Close()
	admin := store.NewAdminStore(pool)
	if err := admin.CheckLimits(ctx); err != nil {
		return err
	}
	job := redditJob{store: admin, reddit: reddit.New(&http.Client{Timeout: 30 * time.Second}, reddit.Reddit), now: now.UTC()}
	r, err := job.run(ctx)
	fmt.Printf("feeds: %d, read: %d, failed: %d; posts queued: %d\n", r.Feeds, r.Read, r.Failed, r.Queued)
	return err
}
