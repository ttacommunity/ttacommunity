// Package moviesync sets up a Movies channel, for the daily job and for admin: it reads every upload YouTube
// still shows, subscribes the channel to YouTube's pushes, and records on the channel what failed.
package moviesync

import (
	"context"
	"errors"
	"strings"
	"time"

	"ttacommunity/backend/internal/movies"
	"ttacommunity/backend/internal/store"
)

// videosPerCall is the most ids one videos.list call takes.
const videosPerCall = 50

// Store is what setting a channel up writes, through the admin login.
type Store interface {
	SaveMovieVideo(ctx context.Context, v store.PushedVideo, at time.Time) error
	MarkMovieChannelBackfilled(ctx context.Context, channelID string, at time.Time) error
	SetMovieChannelError(ctx context.Context, channelID string, msg *string, at time.Time) error
}

// YouTube is the Data API, as setting a channel up reads it.
type YouTube interface {
	Videos(ctx context.Context, ids []string) (map[string]movies.VideoDetails, error)
	Uploads(ctx context.Context, channelID string) ([]string, error)
}

// Hub subscribes a channel to YouTube's pushes.
type Hub interface {
	Subscribe(ctx context.Context, channelID string) error
}

// Syncer sets channels up. Without a Hub it fills them in only.
type Syncer struct {
	Store   Store
	YouTube YouTube
	Hub     Hub
}

// Result is what setting one channel up did; Problem is what failed, empty when nothing did.
type Result struct {
	Filled     int
	Subscribed bool
	Problem    string
}

// Channel sets ch up at now: it reads every upload YouTube shows, saving each video's title and verdict anew (an
// admin's override stays), subscribes it, and records Problem on the channel, or clears it. The error is the
// store's alone; YouTube's and the hub's are the Problem.
func (s Syncer) Channel(ctx context.Context, ch store.MovieJobChannel, tournaments []string, now time.Time) (Result, error) {
	var r Result
	var problems []string
	filled, err := s.fillIn(ctx, ch, tournaments, now)
	if err != nil {
		problems = append(problems, "filling in its videos: "+err.Error())
	} else if err := s.Store.MarkMovieChannelBackfilled(ctx, ch.ChannelID, now); err != nil {
		return r, err
	}
	r.Filled = filled
	if s.Hub != nil {
		if err := s.Hub.Subscribe(ctx, ch.ChannelID); err != nil {
			problems = append(problems, "subscribing: "+err.Error())
		} else {
			r.Subscribed = true
		}
	}
	var msg *string
	if len(problems) > 0 {
		r.Problem = strings.Join(problems, "; ")
		msg = &r.Problem
	}
	if err := s.Store.SetMovieChannelError(ctx, ch.ChannelID, msg, now); err != nil && !errors.Is(err, store.ErrNoChannel) {
		return r, err
	}
	return r, nil
}

// fillIn saves every upload of ch that YouTube still shows, with its details, and returns how many it saved.
// Nothing is saved when a Data API call fails.
func (s Syncer) fillIn(ctx context.Context, ch store.MovieJobChannel, tournaments []string, now time.Time) (int, error) {
	ids, err := s.YouTube.Uploads(ctx, ch.ChannelID)
	if err != nil {
		return 0, err
	}
	details := map[string]movies.VideoDetails{}
	for start := 0; start < len(ids); start += videosPerCall {
		batch, err := s.YouTube.Videos(ctx, ids[start:min(start+videosPerCall, len(ids))])
		if err != nil {
			return 0, err
		}
		for id, d := range batch {
			details[id] = d
		}
	}
	saved := 0
	for _, id := range ids {
		d, ok := details[id]
		if !ok {
			continue
		}
		v := store.PushedVideo{VideoID: id, ChannelID: ch.ChannelID, PublishedAt: d.PublishedAt}
		if err := s.Save(ctx, v, d, ch.IncludeAll, tournaments, now); err != nil {
			return 0, err
		}
		saved++
	}
	return saved, nil
}

// Save stores v at now with its verdict and title from d, the Data API's details. A channel removed meanwhile
// is not an error.
func (s Syncer) Save(ctx context.Context, v store.PushedVideo, d movies.VideoDetails, includeAll bool, tournaments []string, now time.Time) error {
	if d.Title != "" {
		v.Title = d.Title
	}
	v.ThumbnailURL = movies.Thumbnail(v.VideoID)
	verdict := movies.Classify(movies.VideoText{Title: v.Title, Description: d.Description, Tags: d.Tags}, includeAll, tournaments)
	v.TTA, v.Reason = verdict.TTA, verdict.Reason
	v.DetailsFetchedAt = &now
	if err := s.Store.SaveMovieVideo(ctx, v, now); err != nil && !errors.Is(err, store.ErrNoChannel) {
		return err
	}
	return nil
}
