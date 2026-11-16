// The Movies wiring: the Data API key and hub secret from config, and a pushed video's verdict from its channel,
// the tournaments' names and what the Data API says of it.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ttacommunity/backend/internal/httpapi"
	"ttacommunity/backend/internal/movies"
	"ttacommunity/backend/internal/store"
)

// openYouTube returns a Data API client with YOUTUBE_API_KEY, or nil without one.
func openYouTube(getenv func(string) string) *movies.YouTube {
	key := getenv("YOUTUBE_API_KEY")
	if key == "" {
		return nil
	}
	return movies.NewYouTube(nil, movies.DataAPI, key)
}

// startMovieJob starts a Movies Job through starter, for channelID alone when it is set (store.ErrNoChannel when
// channels does not link it), else for every channel.
func startMovieJob(ctx context.Context, channels interface {
	MovieJobChannel(ctx context.Context, channelID string) (store.MovieJobChannel, error)
}, starter interface {
	StartWith(ctx context.Context, args ...string) (string, error)
}, channelID string) (string, error) {
	if channelID == "" {
		return starter.StartWith(ctx)
	}
	if _, err := channels.MovieJobChannel(ctx, channelID); err != nil {
		return "", err
	}
	return starter.StartWith(ctx, "-channel", channelID)
}

// openMovieHub returns the hub's callback's dependencies, or nil (the callback a 404) without MOVIES_HUB_SECRET.
// A secret shorter than movies.MinHubSecret, or one without a Data API client, refuses to start.
func openMovieHub(getenv func(string) string, players *store.PlayerStore, ratings *store.RatingsReader, youtube *movies.YouTube, log *slog.Logger) (httpapi.MovieHub, error) {
	secret := getenv("MOVIES_HUB_SECRET")
	if secret == "" {
		return nil, nil
	}
	if len(secret) < movies.MinHubSecret {
		return nil, fmt.Errorf("MOVIES_HUB_SECRET must be at least %d bytes", movies.MinHubSecret)
	}
	if youtube == nil {
		return nil, errors.New("MOVIES_HUB_SECRET is set without YOUTUBE_API_KEY, which pushed videos are classified with")
	}
	return &movieHubDeps{PlayerStore: players, classifying: ratings, videos: youtube, log: log, secret: []byte(secret)}, nil
}

// movieHubDeps stores what the hub pushes through the player login, deciding verdicts from the read login and
// the Data API.
type movieHubDeps struct {
	*store.PlayerStore
	classifying interface {
		MovieClassifying(ctx context.Context, channelID string) (bool, []string, error)
	}
	videos interface {
		Videos(ctx context.Context, ids []string) (map[string]movies.VideoDetails, error)
	}
	log    *slog.Logger
	secret []byte
}

// Secret is the hub secret pushes are signed with.
func (d *movieHubDeps) Secret() []byte { return d.secret }

// String names the type without its secret.
func (d *movieHubDeps) String() string { return "movieHubDeps{secret: redacted}" }

// GoString names the type without its secret, for %#v.
func (d *movieHubDeps) GoString() string { return d.String() }

// ClassifyPushed decides whether a pushed video is about TTA from its title, description and tags. When the
// Data API fails or no longer shows the video, it decides from the title alone and leaves DetailsFetchedAt
// unset, for the daily job to try again.
func (d *movieHubDeps) ClassifyPushed(ctx context.Context, e movies.PushedEntry, at time.Time) (store.PushedVideo, error) {
	v := store.PushedVideo{
		VideoID: e.VideoID, ChannelID: e.ChannelID, Title: e.Title, PublishedAt: e.PublishedAt,
		ThumbnailURL: movies.Thumbnail(e.VideoID),
	}
	includeAll, tournaments, err := d.classifying.MovieClassifying(ctx, e.ChannelID)
	if errors.Is(err, store.ErrNoChannel) {
		return v, nil
	}
	if err != nil {
		return store.PushedVideo{}, err
	}
	text := movies.VideoText{Title: e.Title}
	details, err := d.videos.Videos(ctx, []string{e.VideoID})
	if err != nil {
		d.log.Warn("a pushed video's details were not read; classified by its title", "err", err)
	}
	if got, ok := details[e.VideoID]; ok {
		text.Description, text.Tags = got.Description, got.Tags
		v.DetailsFetchedAt = &at
	}
	verdict := movies.Classify(text, includeAll, tournaments)
	v.TTA, v.Reason = verdict.TTA, verdict.Reason
	return v, nil
}
