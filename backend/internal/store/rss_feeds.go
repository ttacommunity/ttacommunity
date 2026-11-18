// RSS and Atom feeds: the addresses the feeds job reads, the items it has seen, and the notification each new one
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

// RSSFeed is a feed read for new items by its https address: paused ones are not read; Title is the feed's own,
// as last read ("" until then). CheckedAt is its last good read, LastItemAt its newest item seen, and LastError
// the last read's failure, cleared by a good read.
type RSSFeed struct {
	ID          int64
	URL, Title  string
	Paused      bool
	CheckedAt   *time.Time
	LastItemAt  *time.Time
	LastError   string
	LastErrorAt *time.Time
}

// RSSItem is an item as its feed shows it: its id (the feed's, or its link), title, link, summary as plain text,
// image ("" for none) and when it was published.
type RSSItem struct {
	ItemID, Title, URL, Summary, ImageURL string
	PublishedAt                           time.Time
}

// RSSNotice is what an rss_item notification tells of: the item and its feed's name, its title or else its address.
type RSSNotice struct {
	Feed string
	Item RSSItem
}

var (
	// ErrNoRSSFeed means no RSS feed has that id.
	ErrNoRSSFeed = errors.New("no such RSS feed")
	// ErrBadRSSFeed means a feed address the database refuses.
	ErrBadRSSFeed = errors.New("a feed's address is https://, without spaces or a password, of at most 2000 characters")
	// ErrRSSFeedExists means a feed with that address is already there.
	ErrRSSFeedExists = errors.New("that feed is already there")
)

// maxFeedTitle is the most characters of a feed's title kept.
const maxFeedTitle = 100

const rssFeedColumns = `f.feed_id, f.source, f.display_name, f.paused, f.checked_at,
	(SELECT max(i.published_at) FROM rss_items i WHERE i.feed_id = f.feed_id), coalesce(f.last_error, ''), f.last_error_at`

// RSSFeeds lists the RSS feeds, by title, then address.
func (a *AdminStore) RSSFeeds(ctx context.Context) ([]RSSFeed, error) {
	rows, err := a.pool.Query(ctx, "SELECT "+rssFeedColumns+" FROM feeds f WHERE f.kind = 'rss' ORDER BY lower(f.display_name), f.source")
	if err != nil {
		return nil, fmt.Errorf("listing RSS feeds: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (RSSFeed, error) {
		var f RSSFeed
		err := row.Scan(&f.ID, &f.URL, &f.Title, &f.Paused, &f.CheckedAt, &f.LastItemAt, &f.LastError, &f.LastErrorAt)
		return f, err
	})
	if err != nil {
		return nil, fmt.Errorf("listing RSS feeds: %w", err)
	}
	return list, nil
}

// AddRSSFeed adds a feed read from url at now and returns its id: ErrBadRSSFeed for an address the database
// refuses, ErrRSSFeedExists for one already there in any letters.
func (a *AdminStore) AddRSSFeed(ctx context.Context, url string, now time.Time) (int64, error) {
	var id int64
	err := a.pool.QueryRow(ctx, "INSERT INTO feeds (kind, source, created_at) VALUES ('rss', $1, $2) RETURNING feed_id",
		strings.TrimSpace(url), now).Scan(&id)
	var pg *pgconn.PgError
	switch {
	case errors.As(err, &pg) && pg.Code == "23514":
		return 0, ErrBadRSSFeed
	case errors.As(err, &pg) && pg.Code == "23505":
		return 0, ErrRSSFeedExists
	case err != nil:
		return 0, fmt.Errorf("adding an RSS feed: %w", err)
	}
	return id, nil
}

// SetRSSFeed pauses or resumes a feed, or returns ErrNoRSSFeed. A resumed feed's next read posts nothing.
func (a *AdminStore) SetRSSFeed(ctx context.Context, id int64, paused bool) error {
	tag, err := a.pool.Exec(ctx, `UPDATE feeds SET primed = primed AND NOT (paused AND NOT $2), paused = $2
		WHERE feed_id = $1 AND kind = 'rss'`, id, paused)
	if err != nil {
		return fmt.Errorf("changing an RSS feed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoRSSFeed
	}
	return nil
}

// RemoveRSSFeed removes a feed with its items and subscriptions, or returns ErrNoRSSFeed.
func (a *AdminStore) RemoveRSSFeed(ctx context.Context, id int64) error {
	tag, err := a.pool.Exec(ctx, "DELETE FROM feeds WHERE feed_id = $1 AND kind = 'rss'", id)
	if err != nil {
		return fmt.Errorf("removing an RSS feed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoRSSFeed
	}
	return nil
}

// RecordRSSItems records a good read of feed at now, titled title: each item not seen before is kept and, once the
// feed is primed, queued to be posted, all in one transaction, so a second read never posts one twice. An item not
// queued keeps only its id and date. It returns how many items it queued.
func (a *AdminStore) RecordRSSItems(ctx context.Context, feed int64, title string, items []RSSItem, now time.Time) (int, error) {
	queued := 0
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		primed, err := readWell(ctx, tx, feed, now)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE feeds SET display_name = $2 WHERE feed_id = $1", feed, cutRunes(strings.TrimSpace(title), maxFeedTitle)); err != nil {
			return fmt.Errorf("naming an RSS feed: %w", err)
		}
		for _, it := range items {
			if !primed {
				it = RSSItem{ItemID: it.ItemID, PublishedAt: it.PublishedAt}
			}
			var id int64
			err := tx.QueryRow(ctx, `INSERT INTO rss_items (feed_id, item_id, title, url, summary, image_url, published_at, recorded_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (feed_id, item_id) DO NOTHING RETURNING rss_item_id`,
				feed, it.ItemID, it.Title, it.URL, it.Summary, it.ImageURL, it.PublishedAt, now).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return fmt.Errorf("recording an RSS item: %w", err)
			}
			if !primed {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO notification_outbox (kind, rss_item_id, created_at, next_at)
				VALUES ('rss_item', $1, $2, $2)`, id, now); err != nil {
				return fmt.Errorf("queueing an RSS item: %w", err)
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

// cutRunes shortens s to at most n characters.
func cutRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
