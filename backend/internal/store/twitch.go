// Twitch streamers as feeds: their EventSub subscriptions, the streams they go live with, and the notifications
// each queues, once each however often Twitch pushes it. The database checks each field.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// TwitchFeed is a streamer whose streams going live are posted: their Twitch login, user id and display name,
// paused or not, and their EventSub subscriptions.
type TwitchFeed struct {
	ID                         int64
	Login, UserID, DisplayName string
	Paused                     bool
	Subscriptions              []TwitchSubscription
	CreatedAt                  time.Time
}

// TwitchSubscription is one of a feed's EventSub subscriptions: its type (stream.online or stream.offline), its id
// at Twitch and its status as Twitch last gave it.
type TwitchSubscription struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Status string `json:"status"`
}

// TwitchStream is a stream as Twitch tells of it: its id, the streamer's user id, its title, category and preview
// picture, and when it started.
type TwitchStream struct {
	StreamID, UserID, Title, Category, ThumbnailURL string
	StartedAt                                       time.Time
}

// TwitchPost is the Discord message an endpoint was posted for a stream, and the channel or forum thread it is in,
// kept to edit it when the stream ends.
type TwitchPost struct {
	MessageID string `json:"message_id"`
	ChannelID string `json:"channel_id"`
}

// TwitchNotice is what a twitch_live or twitch_ended notification tells of: the streamer, the stream, when it
// ended (nil while live), and the post each endpoint was sent of it, by endpoint id.
type TwitchNotice struct {
	Login, DisplayName, Title, Category, ThumbnailURL string
	StartedAt                                         time.Time
	EndedAt                                           *time.Time
	Posts                                             map[int64]TwitchPost
}

var (
	// ErrNoTwitchFeed means no Twitch feed has that id.
	ErrNoTwitchFeed = errors.New("no such Twitch feed")
	// ErrBadTwitchFeed means a login or user id the database refuses.
	ErrBadTwitchFeed = errors.New("a Twitch login is 3 to 25 letters, digits or underscores")
	// ErrTwitchFeedExists means that streamer is already a feed.
	ErrTwitchFeedExists = errors.New("that streamer is already a feed")
)

// twitchMessagesKept is how long a push message's id is kept to spot Twitch sending it again; Twitch refuses
// messages older than 10 minutes.
const twitchMessagesKept = 24 * time.Hour

// AddTwitchFeed adds streamer login, with Twitch user id userID and displayName, at now and returns the feed's id:
// ErrBadTwitchFeed for a login or id the database refuses, ErrTwitchFeedExists for one already there.
func (a *AdminStore) AddTwitchFeed(ctx context.Context, login, userID, displayName string, now time.Time) (int64, error) {
	var id int64
	err := a.pool.QueryRow(ctx, `INSERT INTO feeds (kind, source, remote_id, display_name, primed, created_at)
		VALUES ('twitch', $1, $2, $3, true, $4) RETURNING feed_id`, login, userID, displayName, now).Scan(&id)
	var pg *pgconn.PgError
	switch {
	case errors.As(err, &pg) && pg.Code == "23514":
		return 0, ErrBadTwitchFeed
	case errors.As(err, &pg) && pg.Code == "23505":
		return 0, ErrTwitchFeedExists
	case err != nil:
		return 0, fmt.Errorf("adding a Twitch feed: %w", err)
	}
	return id, nil
}

// TwitchFeeds lists the Twitch feeds, by login.
func (a *AdminStore) TwitchFeeds(ctx context.Context) ([]TwitchFeed, error) {
	return a.twitchFeeds(ctx, "", 0)
}

// TwitchFeed returns the Twitch feed with that id, or ErrNoTwitchFeed.
func (a *AdminStore) TwitchFeed(ctx context.Context, id int64) (TwitchFeed, error) {
	list, err := a.twitchFeeds(ctx, "AND f.feed_id = $1", id)
	if err != nil {
		return TwitchFeed{}, err
	}
	if len(list) == 0 {
		return TwitchFeed{}, ErrNoTwitchFeed
	}
	return list[0], nil
}

// twitchFeeds lists the Twitch feeds matching where, a condition on f taking id as $1 ("" for every feed).
func (a *AdminStore) twitchFeeds(ctx context.Context, where string, id int64) ([]TwitchFeed, error) {
	query := `SELECT f.feed_id, f.source, f.remote_id, f.display_name, f.paused, f.created_at,
		coalesce((SELECT json_agg(json_build_object('type', s.type, 'id', s.subscription_id, 'status', s.status) ORDER BY s.type)
		          FROM twitch_subscriptions s WHERE s.feed_id = f.feed_id), '[]')
		FROM feeds f WHERE f.kind = 'twitch' ` + where + ` ORDER BY lower(f.source)`
	args := []any{}
	if where != "" {
		args = append(args, id)
	}
	rows, err := a.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing Twitch feeds: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (TwitchFeed, error) {
		var f TwitchFeed
		err := row.Scan(&f.ID, &f.Login, &f.UserID, &f.DisplayName, &f.Paused, &f.CreatedAt, &f.Subscriptions)
		return f, err
	})
	if err != nil {
		return nil, fmt.Errorf("listing Twitch feeds: %w", err)
	}
	return list, nil
}

// SetTwitchFeedPaused pauses or resumes a Twitch feed: a paused one's streams are not posted. It returns
// ErrNoTwitchFeed for no such feed.
func (a *AdminStore) SetTwitchFeedPaused(ctx context.Context, id int64, paused bool) error {
	tag, err := a.pool.Exec(ctx, "UPDATE feeds SET paused = $2 WHERE feed_id = $1 AND kind = 'twitch'", id, paused)
	if err != nil {
		return fmt.Errorf("pausing a Twitch feed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoTwitchFeed
	}
	return nil
}

// RemoveTwitchFeed removes a Twitch feed with its subscriptions, streams and posts, or returns ErrNoTwitchFeed.
func (a *AdminStore) RemoveTwitchFeed(ctx context.Context, id int64) error {
	tag, err := a.pool.Exec(ctx, "DELETE FROM feeds WHERE feed_id = $1 AND kind = 'twitch'", id)
	if err != nil {
		return fmt.Errorf("removing a Twitch feed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoTwitchFeed
	}
	return nil
}

// SetTwitchSubscription records feed's EventSub subscription of type typ: its id at Twitch and its status, at now.
func (a *AdminStore) SetTwitchSubscription(ctx context.Context, feed int64, typ, id, status string, now time.Time) error {
	if _, err := a.pool.Exec(ctx, `INSERT INTO twitch_subscriptions (feed_id, type, subscription_id, status, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (feed_id, type) DO UPDATE SET subscription_id = $3, status = $4, updated_at = $5`, feed, typ, id, status, now); err != nil {
		return fmt.Errorf("recording a Twitch subscription: %w", err)
	}
	return nil
}

// TwitchSubscriptionStatus records the status Twitch gives the subscription with that id, at now; an id not
// recorded is ignored.
func (a *AdminStore) TwitchSubscriptionStatus(ctx context.Context, id, status string, now time.Time) error {
	if _, err := a.pool.Exec(ctx, "UPDATE twitch_subscriptions SET status = $2, updated_at = $3 WHERE subscription_id = $1",
		id, status, now); err != nil {
		return fmt.Errorf("recording a Twitch subscription's status: %w", err)
	}
	return nil
}

// firstTwitchMessage records push message id at now, forgetting those older than twitchMessagesKept, and says
// whether it is new.
func firstTwitchMessage(ctx context.Context, tx pgx.Tx, id string, now time.Time) (bool, error) {
	if _, err := tx.Exec(ctx, "DELETE FROM twitch_messages WHERE received_at < $1", now.Add(-twitchMessagesKept)); err != nil {
		return false, fmt.Errorf("forgetting old Twitch messages: %w", err)
	}
	tag, err := tx.Exec(ctx, "INSERT INTO twitch_messages (message_id, received_at) VALUES ($1, $2) ON CONFLICT DO NOTHING", id, now)
	if err != nil {
		return false, fmt.Errorf("recording a Twitch message: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// TwitchStreamOnline records stream s, pushed in message, at now, and queues its post, in one transaction: a
// message seen before, a stream already recorded, and a streamer not listed or paused queue nothing.
func (a *AdminStore) TwitchStreamOnline(ctx context.Context, message string, s TwitchStream, now time.Time) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		if first, err := firstTwitchMessage(ctx, tx, message, now); err != nil || !first {
			return err
		}
		var ref int64
		err := tx.QueryRow(ctx, `INSERT INTO twitch_streams (feed_id, stream_id, title, category, thumbnail_url, started_at, recorded_at)
			SELECT feed_id, $2, $3, $4, $5, $6, $7 FROM feeds WHERE kind = 'twitch' AND remote_id = $1 AND NOT paused
			ON CONFLICT (stream_id) DO NOTHING RETURNING twitch_stream_id`,
			s.UserID, s.StreamID, s.Title, s.Category, s.ThumbnailURL, s.StartedAt, now).Scan(&ref)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("recording a Twitch stream: %w", err)
		}
		return queueTwitch(ctx, tx, "twitch_live", ref, now)
	})
}

// TwitchStreamOffline records that the streamer with userID ended their streams, pushed in message, at now, and
// queues the edit of each post kept of them, in one transaction; a message seen before changes nothing.
func (a *AdminStore) TwitchStreamOffline(ctx context.Context, message, userID string, now time.Time) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		if first, err := firstTwitchMessage(ctx, tx, message, now); err != nil || !first {
			return err
		}
		rows, err := tx.Query(ctx, `UPDATE twitch_streams s SET ended_at = $2 FROM feeds f
			WHERE f.feed_id = s.feed_id AND f.kind = 'twitch' AND f.remote_id = $1 AND s.ended_at IS NULL
			RETURNING s.twitch_stream_id, EXISTS (SELECT 1 FROM twitch_posts p WHERE p.twitch_stream_id = s.twitch_stream_id)`, userID, now)
		if err != nil {
			return fmt.Errorf("ending a Twitch stream: %w", err)
		}
		type ended struct {
			ref    int64
			posted bool
		}
		list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ended, error) {
			var e ended
			err := row.Scan(&e.ref, &e.posted)
			return e, err
		})
		if err != nil {
			return fmt.Errorf("ending a Twitch stream: %w", err)
		}
		for _, e := range list {
			if !e.posted {
				continue
			}
			if err := queueTwitch(ctx, tx, "twitch_ended", e.ref, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// queueTwitch queues a notification of kind twitch_live or twitch_ended about the stream ref, due at now.
func queueTwitch(ctx context.Context, tx pgx.Tx, kind string, ref int64, now time.Time) error {
	if _, err := tx.Exec(ctx, "INSERT INTO notification_outbox (kind, twitch_stream_id, created_at, next_at) VALUES ($1, $2, $3, $3)",
		kind, ref, now); err != nil {
		return fmt.Errorf("queueing a Twitch notification: %w", err)
	}
	return nil
}

// RecordTwitchPost keeps the message endpoint was posted of the stream ref, so it can be edited when the stream
// ends; a second post replaces the first.
func (a *AdminStore) RecordTwitchPost(ctx context.Context, ref, endpoint int64, p TwitchPost) error {
	if _, err := a.pool.Exec(ctx, `INSERT INTO twitch_posts (twitch_stream_id, endpoint_id, message_id, channel_id) VALUES ($1, $2, $3, $4)
		ON CONFLICT (twitch_stream_id, endpoint_id) DO UPDATE SET message_id = $3, channel_id = $4`, ref, endpoint, p.MessageID, p.ChannelID); err != nil {
		return fmt.Errorf("keeping a Twitch post: %w", err)
	}
	return nil
}
