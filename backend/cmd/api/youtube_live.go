// Checking YouTube Live: every minute, the streams due a check are read from the Data API in one call, and what
// each is now recorded, which queues its post when it starts and its edit when it ends.
package main

import (
	"context"
	"log/slog"
	"time"

	"ttacommunity/backend/internal/movies"
	"ttacommunity/backend/internal/store"
)

// youtubeCheckEvery is how often streams due a check are looked for; youtubeBatch is how many one Data API call
// reads, its most.
const (
	youtubeCheckEvery = time.Minute
	youtubeBatch      = 50
)

// youtubeStreams is the store as the checks read and record it.
type youtubeStreams interface {
	DueYouTubeStreams(ctx context.Context, now time.Time, limit int) ([]store.YouTubeStream, error)
	RecordYouTubeCheck(ctx context.Context, ref int64, c store.YouTubeCheck, now time.Time) error
}

// youtubeVideos is the Data API as the checks read it.
type youtubeVideos interface {
	Videos(ctx context.Context, ids []string) (map[string]movies.VideoDetails, error)
}

// watchYouTubeStreams checks the streams due every youtubeCheckEvery until ctx ends.
func watchYouTubeStreams(ctx context.Context, streams youtubeStreams, videos youtubeVideos, log *slog.Logger) {
	for {
		checkYouTubeStreams(ctx, streams, videos, time.Now().UTC(), log)
		select {
		case <-ctx.Done():
			return
		case <-time.After(youtubeCheckEvery):
		}
	}
}

// checkYouTubeStreams reads up to youtubeBatch streams due at now from videos, asking nothing when none is, and
// records each: gone when YouTube no longer shows it, else its title, start, end and next check. A failure is
// logged and leaves the streams due, for the next pass. Logs carry counts, never a video or a title.
func checkYouTubeStreams(ctx context.Context, streams youtubeStreams, videos youtubeVideos, now time.Time, log *slog.Logger) {
	due, err := streams.DueYouTubeStreams(ctx, now, youtubeBatch)
	if err != nil || len(due) == 0 {
		if err != nil && ctx.Err() == nil {
			log.Warn("listing YouTube streams due failed", "err", err)
		}
		return
	}
	ids := make([]string, len(due))
	for i, s := range due {
		ids[i] = s.VideoID
	}
	details, err := videos.Videos(ctx, ids)
	if err != nil {
		log.Warn("checking YouTube streams failed", "streams", len(due), "err", err)
		return
	}
	for _, s := range due {
		c := store.YouTubeCheck{Gone: true}
		if d, ok := details[s.VideoID]; ok {
			b := d.Broadcast
			c = store.YouTubeCheck{Title: d.Title, StartedAt: b.StartedAt, EndedAt: b.EndedAt, NextCheckAt: movies.NextCheck(b, s.RecordedAt, now)}
		}
		if err := streams.RecordYouTubeCheck(ctx, s.Ref, c, now); err != nil {
			log.Warn("recording a YouTube stream's check failed", "err", err)
		}
	}
}
