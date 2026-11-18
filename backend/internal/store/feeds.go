// What every kind of feed shares: a feed that fails reads in a row, and reads again, is told of through the
// outbox, and a good read primes a feed so its next one posts what is new.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrNoFeed means no feed of any kind has that id.
var ErrNoFeed = errors.New("no such feed")

// maxFeedError is the most characters of a read's failure kept on its feed.
const maxFeedError = 200

// FeedFailuresNotified is how many reads in a row a feed fails before admins are told, so a passing rate limit
// stays quiet; they are told again when it reads well.
const FeedFailuresNotified = 3

// readWell marks feed, of any kind, read well at now in tx, locking it, and returns whether it was primed: its
// failures are cleared, and a feed that had failed FeedFailuresNotified reads queues word that it reads again.
func readWell(ctx context.Context, tx pgx.Tx, feed int64, now time.Time) (bool, error) {
	var primed bool
	var failures int
	err := tx.QueryRow(ctx, "SELECT primed, failures FROM feeds WHERE feed_id = $1 FOR UPDATE", feed).Scan(&primed, &failures)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNoFeed
	}
	if err != nil {
		return false, fmt.Errorf("reading a feed: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE feeds SET primed = true, checked_at = $2, last_error = NULL, last_error_at = NULL,
		failures = 0 WHERE feed_id = $1`, feed, now); err != nil {
		return false, fmt.Errorf("marking a feed read: %w", err)
	}
	if failures >= FeedFailuresNotified {
		if err := queueFeedNotice(ctx, tx, "feed_recovered", feed, now); err != nil {
			return false, err
		}
	}
	return primed, nil
}

// FeedFailed records that reading feed, of any kind, failed at now with msg, cut to what the database keeps, or
// returns ErrNoFeed; the FeedFailuresNotified-th failure in a row queues word of it.
func (a *AdminStore) FeedFailed(ctx context.Context, feed int64, msg string, now time.Time) error {
	msg = cutRunes(msg, maxFeedError)
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var failures int
		err := tx.QueryRow(ctx, `UPDATE feeds SET last_error = $2, last_error_at = $3, failures = failures + 1
			WHERE feed_id = $1 RETURNING failures`, feed, msg, now).Scan(&failures)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoFeed
		}
		if err != nil {
			return fmt.Errorf("recording a feed's failure: %w", err)
		}
		if failures != FeedFailuresNotified {
			return nil
		}
		return queueFeedNotice(ctx, tx, "feed_failed", feed, now)
	})
}

// queueFeedNotice queues word, of kind feed_failed or feed_recovered, that feed is failing or reads again.
func queueFeedNotice(ctx context.Context, tx pgx.Tx, kind string, feed int64, now time.Time) error {
	if _, err := tx.Exec(ctx, "INSERT INTO notification_outbox (kind, feed_id, created_at, next_at) VALUES ($1, $2, $3, $3)", kind, feed, now); err != nil {
		return fmt.Errorf("queueing a feed's notice: %w", err)
	}
	return nil
}
