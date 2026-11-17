// Reddit feeds: the subreddits the reddit job reads, the posts it has seen, and the notification each new one
// queues. The database checks each field; the admin login reads and writes them all.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// RedditFeed is a subreddit read for new posts: paused ones are not read; one not primed records its next read
// without posting it. CheckedAt is its last good read, LastPostAt its newest post seen, and LastError the last
// read's failure, cleared by a good read.
type RedditFeed struct {
	ID             int64
	Subreddit      string
	Paused, Primed bool
	CheckedAt      *time.Time
	LastPostAt     *time.Time
	LastError      string
	LastErrorAt    *time.Time
}

// RedditPost is a post as the feed shows it: its id (t3_…), kind (text, image, video or link), title, link, author
// (without /u/), text, image ("" for none) and when it was posted.
type RedditPost struct {
	PostID, Kind, Title, URL, Author, Body, ImageURL string
	PublishedAt                                      time.Time
}

// RedditNotice is what a reddit_post notification tells of: the post and its feed's subreddit.
type RedditNotice struct {
	Subreddit string
	Post      RedditPost
}

var (
	// ErrNoRedditFeed means no feed has that id.
	ErrNoRedditFeed = errors.New("no such Reddit feed")
	// ErrBadRedditFeed means a subreddit name the database refuses.
	ErrBadRedditFeed = errors.New("a subreddit is named by 3 to 21 letters, digits or underscores, without r/")
	// ErrRedditFeedExists means a feed for that subreddit is already there.
	ErrRedditFeedExists = errors.New("that subreddit is already a feed")
)

// maxFeedError is the most characters of a read's failure kept on its feed.
const maxFeedError = 200

const feedColumns = `f.feed_id, f.subreddit, f.paused, f.primed, f.checked_at,
	(SELECT max(p.published_at) FROM reddit_posts p WHERE p.feed_id = f.feed_id), coalesce(f.last_error, ''), f.last_error_at`

// RedditFeeds lists the feeds, by subreddit.
func (a *AdminStore) RedditFeeds(ctx context.Context) ([]RedditFeed, error) {
	rows, err := a.pool.Query(ctx, "SELECT "+feedColumns+" FROM reddit_feeds f ORDER BY lower(f.subreddit)")
	if err != nil {
		return nil, fmt.Errorf("listing Reddit feeds: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (RedditFeed, error) {
		var f RedditFeed
		err := row.Scan(&f.ID, &f.Subreddit, &f.Paused, &f.Primed, &f.CheckedAt, &f.LastPostAt, &f.LastError, &f.LastErrorAt)
		return f, err
	})
	if err != nil {
		return nil, fmt.Errorf("listing Reddit feeds: %w", err)
	}
	return list, nil
}

// AddRedditFeed adds a feed for subreddit at now and returns its id: ErrBadRedditFeed for a name Reddit cannot
// have, ErrRedditFeedExists for one already there in any letters.
func (a *AdminStore) AddRedditFeed(ctx context.Context, subreddit string, now time.Time) (int64, error) {
	var id int64
	err := a.pool.QueryRow(ctx, "INSERT INTO reddit_feeds (subreddit, created_at) VALUES ($1, $2) RETURNING feed_id",
		strings.TrimSpace(subreddit), now).Scan(&id)
	var pg *pgconn.PgError
	switch {
	case errors.As(err, &pg) && pg.Code == "23514":
		return 0, ErrBadRedditFeed
	case errors.As(err, &pg) && pg.Code == "23505":
		return 0, ErrRedditFeedExists
	case err != nil:
		return 0, fmt.Errorf("adding a Reddit feed: %w", err)
	}
	return id, nil
}

// SetRedditFeedPaused pauses or resumes a feed, or returns ErrNoRedditFeed. A resumed feed's next read posts
// nothing, so what came while it was paused is not posted at once.
func (a *AdminStore) SetRedditFeedPaused(ctx context.Context, id int64, paused bool) error {
	tag, err := a.pool.Exec(ctx, "UPDATE reddit_feeds SET paused = $2, primed = primed AND $2 WHERE feed_id = $1", id, paused)
	if err != nil {
		return fmt.Errorf("pausing a Reddit feed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoRedditFeed
	}
	return nil
}

// RemoveRedditFeed removes a feed with its posts and subscriptions, or returns ErrNoRedditFeed.
func (a *AdminStore) RemoveRedditFeed(ctx context.Context, id int64) error {
	tag, err := a.pool.Exec(ctx, "DELETE FROM reddit_feeds WHERE feed_id = $1", id)
	if err != nil {
		return fmt.Errorf("removing a Reddit feed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoRedditFeed
	}
	return nil
}

// RecordRedditPosts records a good read of feed at now: each post not seen before is kept and, once the feed is
// primed, queued to be posted, all in one transaction, so a second read never posts one twice. It returns how many
// it queued.
func (a *AdminStore) RecordRedditPosts(ctx context.Context, feed int64, posts []RedditPost, now time.Time) (int, error) {
	queued := 0
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var primed bool
		err := tx.QueryRow(ctx, "SELECT primed FROM reddit_feeds WHERE feed_id = $1 FOR UPDATE", feed).Scan(&primed)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoRedditFeed
		}
		if err != nil {
			return fmt.Errorf("reading a Reddit feed: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE reddit_feeds SET primed = true, checked_at = $2, last_error = NULL, last_error_at = NULL
			WHERE feed_id = $1`, feed, now); err != nil {
			return fmt.Errorf("marking a Reddit feed read: %w", err)
		}
		for _, p := range posts {
			var id int64
			err := tx.QueryRow(ctx, `INSERT INTO reddit_posts (feed_id, post_id, kind, title, url, author, body, image_url, published_at, recorded_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) ON CONFLICT (feed_id, post_id) DO NOTHING RETURNING reddit_post_id`,
				feed, p.PostID, p.Kind, p.Title, p.URL, p.Author, p.Body, p.ImageURL, p.PublishedAt, now).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return fmt.Errorf("recording a Reddit post: %w", err)
			}
			if !primed {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO notification_outbox (kind, reddit_post_id, created_at, next_at)
				VALUES ('reddit_post', $1, $2, $2)`, id, now); err != nil {
				return fmt.Errorf("queueing a Reddit post: %w", err)
			}
			queued++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return queued, nil
}

// RedditFeedFailed records that reading feed failed at now with msg, cut to what the database keeps.
func (a *AdminStore) RedditFeedFailed(ctx context.Context, feed int64, msg string, now time.Time) error {
	if r := []rune(msg); len(r) > maxFeedError {
		msg = string(r[:maxFeedError])
	}
	if _, err := a.pool.Exec(ctx, "UPDATE reddit_feeds SET last_error = $2, last_error_at = $3 WHERE feed_id = $1", feed, msg, now); err != nil {
		return fmt.Errorf("recording a Reddit feed's failure: %w", err)
	}
	return nil
}
