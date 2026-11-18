// YouTube Live: the one feed of Movies' channels' live streams, the streams the hub pushed with when to check each
// next, and the notifications a check queues, once each: going live when Movies shows the video, ending for the
// posts kept of it.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// YouTubeFeed is the YouTube Live feed: paused or not, how many Movies channels it watches, and how many of their
// streams are waiting to start or live.
type YouTubeFeed struct {
	ID                      int64
	Paused                  bool
	Channels, Waiting, Live int
	CreatedAt               time.Time
}

// YouTubeStream is a stream due a check: its row (Ref), its video and when it was first noted.
type YouTubeStream struct {
	Ref        int64
	VideoID    string
	RecordedAt time.Time
}

// YouTubeCheck is what a check of a stream found: Gone when YouTube no longer shows the video, else its title, when
// it started and ended where it has, and when to check it next (nil for never).
type YouTubeCheck struct {
	Gone                            bool
	Title                           string
	StartedAt, EndedAt, NextCheckAt *time.Time
}

// YouTubeNotice is what a youtube_live or youtube_ended notification tells of: the Movies channel's name, the
// video, its title and picture, when it started and ended (nil while live), and each endpoint's post, by id.
type YouTubeNotice struct {
	Channel, VideoID, Title, ThumbnailURL string
	StartedAt                             time.Time
	EndedAt                               *time.Time
	Posts                                 map[int64]DiscordPost
}

var (
	// ErrNoYouTubeFeed means no YouTube feed has that id.
	ErrNoYouTubeFeed = errors.New("no such YouTube feed")
	// ErrYouTubeFeedExists means the YouTube Live feed is there already: there is only one.
	ErrYouTubeFeedExists = errors.New("the YouTube Live feed is already there")
)

// AddYouTubeFeed adds the YouTube Live feed at now and returns its id, or ErrYouTubeFeedExists.
func (a *AdminStore) AddYouTubeFeed(ctx context.Context, now time.Time) (int64, error) {
	var id int64
	err := a.pool.QueryRow(ctx, `INSERT INTO feeds (kind, source, primed, created_at) VALUES ('youtube', 'movies', true, $1)
		RETURNING feed_id`, now).Scan(&id)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return 0, ErrYouTubeFeedExists
	}
	if err != nil {
		return 0, fmt.Errorf("adding the YouTube feed: %w", err)
	}
	return id, nil
}

// YouTubeFeeds lists the YouTube Live feed, or nothing when it has not been added.
func (a *AdminStore) YouTubeFeeds(ctx context.Context) ([]YouTubeFeed, error) {
	rows, err := a.pool.Query(ctx, `SELECT f.feed_id, f.paused, f.created_at, (SELECT count(*) FROM video_channels),
		(SELECT count(*) FROM youtube_streams s WHERE s.feed_id = f.feed_id AND s.started_at IS NULL AND s.next_check_at IS NOT NULL),
		(SELECT count(*) FROM youtube_streams s WHERE s.feed_id = f.feed_id AND s.started_at IS NOT NULL AND s.ended_at IS NULL)
		FROM feeds f WHERE f.kind = 'youtube'`)
	if err != nil {
		return nil, fmt.Errorf("listing YouTube feeds: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (YouTubeFeed, error) {
		var f YouTubeFeed
		err := row.Scan(&f.ID, &f.Paused, &f.CreatedAt, &f.Channels, &f.Waiting, &f.Live)
		return f, err
	})
	if err != nil {
		return nil, fmt.Errorf("listing YouTube feeds: %w", err)
	}
	return list, nil
}

// SetYouTubeFeedPaused pauses or resumes the YouTube feed with that id: paused, its streams are neither checked nor
// posted. ErrNoYouTubeFeed for no such feed.
func (a *AdminStore) SetYouTubeFeedPaused(ctx context.Context, id int64, paused bool) error {
	tag, err := a.pool.Exec(ctx, "UPDATE feeds SET paused = $2 WHERE feed_id = $1 AND kind = 'youtube'", id, paused)
	if err != nil {
		return fmt.Errorf("pausing the YouTube feed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoYouTubeFeed
	}
	return nil
}

// RemoveYouTubeFeed removes the YouTube feed with that id, with its streams, posts and subscriptions, or returns
// ErrNoYouTubeFeed.
func (a *AdminStore) RemoveYouTubeFeed(ctx context.Context, id int64) error {
	tag, err := a.pool.Exec(ctx, "DELETE FROM feeds WHERE feed_id = $1 AND kind = 'youtube'", id)
	if err != nil {
		return fmt.Errorf("removing the YouTube feed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoYouTubeFeed
	}
	return nil
}

// NoteYouTubeStream marks channelID's stored video videoID, which the hub pushed live or scheduled, to be checked
// at at, through the player login; nothing without the YouTube Live feed, or while it is paused.
func (s *PlayerStore) NoteYouTubeStream(ctx context.Context, videoID, channelID string, at time.Time) error {
	if _, err := s.pool.Exec(ctx, "SELECT note_youtube_stream($1, $2, $3)", videoID, channelID, at); err != nil {
		return fmt.Errorf("noting a YouTube stream: %w", err)
	}
	return nil
}

// DueYouTubeStreams lists up to limit streams due a check at now, the longest due first; none while the feed is
// paused.
func (a *AdminStore) DueYouTubeStreams(ctx context.Context, now time.Time, limit int) ([]YouTubeStream, error) {
	rows, err := a.pool.Query(ctx, `SELECT s.youtube_stream_id, s.video_id, s.recorded_at
		FROM youtube_streams s JOIN feeds f ON f.feed_id = s.feed_id
		WHERE s.next_check_at <= $1 AND NOT f.paused ORDER BY s.next_check_at, s.youtube_stream_id LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("listing YouTube streams due: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (YouTubeStream, error) {
		var s YouTubeStream
		err := row.Scan(&s.Ref, &s.VideoID, &s.RecordedAt)
		return s, err
	})
	if err != nil {
		return nil, fmt.Errorf("listing YouTube streams due: %w", err)
	}
	return list, nil
}

// RecordYouTubeCheck keeps what check c of the stream ref found at now, in one transaction: a stream first seen
// started, and not ended, queues its post when Movies shows its video and the feed is not paused; one first seen
// ended, or gone once started, queues the edit of the posts kept of it. A stream gone before it started is checked
// no more. A stream removed meanwhile is not an error.
func (a *AdminStore) RecordYouTubeCheck(ctx context.Context, ref int64, c YouTubeCheck, now time.Time) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var wasStarted, wasEnded, shown, paused, posted bool
		err := tx.QueryRow(ctx, `SELECT s.started_at IS NOT NULL, s.ended_at IS NOT NULL, coalesce(v.tta_override, v.is_tta), f.paused,
			EXISTS (SELECT 1 FROM youtube_posts p WHERE p.youtube_stream_id = s.youtube_stream_id)
			FROM youtube_streams s JOIN videos v ON v.video_id = s.video_id JOIN feeds f ON f.feed_id = s.feed_id
			WHERE s.youtube_stream_id = $1 FOR UPDATE OF s`, ref).Scan(&wasStarted, &wasEnded, &shown, &paused, &posted)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading a YouTube stream: %w", err)
		}
		if wasEnded {
			return nil
		}
		started, ended, next := c.StartedAt, c.EndedAt, c.NextCheckAt
		if ended != nil && started == nil {
			started = ended
		}
		if c.Gone {
			next = nil
			if wasStarted {
				ended = &now
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE youtube_streams SET title = CASE WHEN $2 = '' THEN title ELSE left($2, 300) END,
			started_at = coalesce(started_at, $3), ended_at = $4, next_check_at = $5 WHERE youtube_stream_id = $1`,
			ref, c.Title, started, ended, next); err != nil {
			return fmt.Errorf("recording a YouTube stream's check: %w", err)
		}
		switch {
		case ended != nil && posted:
			return queueYouTube(ctx, tx, "youtube_ended", ref, now)
		case !wasStarted && started != nil && ended == nil && shown && !paused:
			return queueYouTube(ctx, tx, "youtube_live", ref, now)
		}
		return nil
	})
}

// queueYouTube queues a notification of kind youtube_live or youtube_ended about the stream ref, due at now.
func queueYouTube(ctx context.Context, tx pgx.Tx, kind string, ref int64, now time.Time) error {
	if _, err := tx.Exec(ctx, "INSERT INTO notification_outbox (kind, youtube_stream_id, created_at, next_at) VALUES ($1, $2, $3, $3)",
		kind, ref, now); err != nil {
		return fmt.Errorf("queueing a YouTube notification: %w", err)
	}
	return nil
}

// RecordYouTubePost keeps the message endpoint was posted of the stream ref, so it can be edited when the stream
// ends; a second post replaces the first.
func (a *AdminStore) RecordYouTubePost(ctx context.Context, ref, endpoint int64, p DiscordPost) error {
	if _, err := a.pool.Exec(ctx, `INSERT INTO youtube_posts (youtube_stream_id, endpoint_id, message_id, channel_id) VALUES ($1, $2, $3, $4)
		ON CONFLICT (youtube_stream_id, endpoint_id) DO UPDATE SET message_id = $3, channel_id = $4`, ref, endpoint, p.MessageID, p.ChannelID); err != nil {
		return fmt.Errorf("keeping a YouTube post: %w", err)
	}
	return nil
}
