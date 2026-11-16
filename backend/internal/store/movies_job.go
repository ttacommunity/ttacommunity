// What the daily Movies job reads and writes through the admin login: the linked channels and their runs, the
// stored videos, and saving or deleting a video.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// MovieJobChannel is a linked channel as the daily job sees it: whether its every video counts, and when its
// videos were filled in from its feed (nil until they are).
type MovieJobChannel struct {
	ChannelID    string
	IncludeAll   bool
	BackfilledAt *time.Time
}

// MovieJobVideo is a stored video as the daily job sees it: enough to save it again, and whether its details
// were read.
type MovieJobVideo struct {
	VideoID        string
	ChannelID      string
	Title          string
	PublishedAt    time.Time
	ThumbnailURL   string
	DetailsFetched bool
}

// MovieJobChannels lists the linked channels.
func (a *AdminStore) MovieJobChannels(ctx context.Context) ([]MovieJobChannel, error) {
	rows, err := a.pool.Query(ctx, "SELECT channel_id, include_all, backfilled_at FROM video_channels ORDER BY channel_id")
	if err != nil {
		return nil, fmt.Errorf("listing channels for the movies job: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[MovieJobChannel])
}

// MovieJobVideos lists every stored video.
func (a *AdminStore) MovieJobVideos(ctx context.Context) ([]MovieJobVideo, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT video_id, channel_id, title, published_at, thumbnail_url, details_fetched_at IS NOT NULL
		FROM videos ORDER BY video_id`)
	if err != nil {
		return nil, fmt.Errorf("listing videos for the movies job: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[MovieJobVideo])
}

// MovieTournamentNames lists the tournaments' names a video may name.
func (a *AdminStore) MovieTournamentNames(ctx context.Context) ([]string, error) {
	rows, err := a.pool.Query(ctx, "SELECT name FROM series WHERE btrim(name) <> '' ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("reading tournament names: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// SaveMovieVideo stores or updates v with its verdict at at, keeping an admin's override. ErrNoChannel when its
// channel is not linked.
func (a *AdminStore) SaveMovieVideo(ctx context.Context, v PushedVideo, at time.Time) error {
	var saved bool
	if err := a.pool.QueryRow(ctx, "SELECT save_movie_video($1, $2, $3, $4, $5, $6, $7, $8, $9)",
		v.VideoID, v.ChannelID, v.Title, v.PublishedAt, v.ThumbnailURL, v.TTA, v.Reason, v.DetailsFetchedAt, at).Scan(&saved); err != nil {
		return fmt.Errorf("saving a video: %w", err)
	}
	if !saved {
		return ErrNoChannel
	}
	return nil
}

// DeleteMovieVideo drops a video YouTube no longer shows.
func (a *AdminStore) DeleteMovieVideo(ctx context.Context, videoID string) error {
	if _, err := a.pool.Exec(ctx, "DELETE FROM videos WHERE video_id = $1", videoID); err != nil {
		return fmt.Errorf("deleting a video: %w", err)
	}
	return nil
}

// MarkMovieChannelBackfilled records that channelID's videos were filled in from its feed at at.
func (a *AdminStore) MarkMovieChannelBackfilled(ctx context.Context, channelID string, at time.Time) error {
	return a.updateMovieChannel(ctx, "UPDATE video_channels SET backfilled_at = $2 WHERE channel_id = $1", channelID, at)
}

// SetMovieChannelError records what went wrong in channelID's run at at, or with msg nil clears it.
// ErrNoChannel when the channel is not linked.
func (a *AdminStore) SetMovieChannelError(ctx context.Context, channelID string, msg *string, at time.Time) error {
	return a.updateMovieChannel(ctx, `
		UPDATE video_channels SET last_error = $2, last_error_at = CASE WHEN $2::text IS NULL THEN NULL ELSE $3::timestamptz END
		WHERE channel_id = $1`, channelID, msg, at)
}

func (a *AdminStore) updateMovieChannel(ctx context.Context, sql, channelID string, args ...any) error {
	tag, err := a.pool.Exec(ctx, sql, append([]any{channelID}, args...)...)
	if err != nil {
		return fmt.Errorf("recording a channel's run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoChannel
	}
	return nil
}
