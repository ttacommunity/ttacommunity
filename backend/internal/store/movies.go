// Public Movies feed queries: active YouTube creators and chronological non-hidden videos.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// MovieCreator is one YouTube creator listed in the Movies filter.
type MovieCreator struct {
	ChannelID string
	Name      string
	Player    *string
}

// MovieVideo is one visible video in the public Movies feed.
type MovieVideo struct {
	VideoID       string
	ChannelID     string
	CreatorName   string
	CreatorPlayer *string
	Title         string
	PublishedAt   time.Time
	ThumbnailURL  string
}

// MoviesFeed holds all creators and visible videos (newest first, up to 200).
type MoviesFeed struct {
	Creators []MovieCreator
	Videos   []MovieVideo
}

// Movies returns active creators and visible community videos newest first.
func (s *RatingsReader) Movies(ctx context.Context) (MoviesFeed, error) {
	cRows, err := s.pool.Query(ctx, `
		SELECT vc.channel_id, vc.name, p.name
		FROM video_channels vc
		LEFT JOIN players p ON p.player_id = vc.player_id
		ORDER BY lower(vc.name), vc.channel_id`)
	if err != nil {
		return MoviesFeed{}, fmt.Errorf("reading movie creators: %w", err)
	}
	creators, err := pgx.CollectRows(cRows, pgx.RowToStructByPos[MovieCreator])
	if err != nil {
		return MoviesFeed{}, fmt.Errorf("reading movie creators: %w", err)
	}

	vRows, err := s.pool.Query(ctx, `
		SELECT v.video_id, v.channel_id, vc.name, p.name, v.title, v.published_at, v.thumbnail_url
		FROM videos v
		JOIN video_channels vc USING (channel_id)
		LEFT JOIN players p ON p.player_id = vc.player_id
		WHERE coalesce(v.tta_override, v.is_tta)
		ORDER BY v.published_at DESC, v.video_id
		LIMIT 200`)
	if err != nil {
		return MoviesFeed{}, fmt.Errorf("reading movie videos: %w", err)
	}
	videos, err := pgx.CollectRows(vRows, pgx.RowToStructByPos[MovieVideo])
	if err != nil {
		return MoviesFeed{}, fmt.Errorf("reading movie videos: %w", err)
	}
	return MoviesFeed{Creators: creators, Videos: videos}, nil
}
