// The Movies hub's callback's wiring: the hub secret from config, and a pushed video's verdict from its channel
// and the tournaments' names.
package main

import (
	"context"
	"errors"
	"fmt"

	"ttacommunity/backend/internal/httpapi"
	"ttacommunity/backend/internal/movies"
	"ttacommunity/backend/internal/store"
)

// minHubSecret is the shortest MOVIES_HUB_SECRET taken, in bytes.
const minHubSecret = 32

// openMovieHub returns the hub's callback's dependencies, or nil (the callback a 404) without MOVIES_HUB_SECRET.
// A secret shorter than minHubSecret refuses to start.
func openMovieHub(getenv func(string) string, players *store.PlayerStore, ratings *store.RatingsReader) (httpapi.MovieHub, error) {
	secret := getenv("MOVIES_HUB_SECRET")
	if secret == "" {
		return nil, nil
	}
	if len(secret) < minHubSecret {
		return nil, fmt.Errorf("MOVIES_HUB_SECRET must be at least %d bytes", minHubSecret)
	}
	return &movieHubDeps{PlayerStore: players, ratings: ratings, secret: []byte(secret)}, nil
}

// movieHubDeps stores what the hub pushes through the player login, deciding verdicts from the read login.
type movieHubDeps struct {
	*store.PlayerStore
	ratings *store.RatingsReader
	secret  []byte
}

// Secret is the hub secret pushes are signed with.
func (d *movieHubDeps) Secret() []byte { return d.secret }

// String names the type without its secret.
func (d *movieHubDeps) String() string { return "movieHubDeps{secret: redacted}" }

// ClassifyPushed decides from its title whether a pushed video is about TTA.
func (d *movieHubDeps) ClassifyPushed(ctx context.Context, e movies.PushedEntry) (store.PushedVideo, error) {
	v := store.PushedVideo{
		VideoID: e.VideoID, ChannelID: e.ChannelID, Title: e.Title, PublishedAt: e.PublishedAt,
		ThumbnailURL: movies.Thumbnail(e.VideoID),
	}
	includeAll, tournaments, err := d.ratings.MovieClassifying(ctx, e.ChannelID)
	if errors.Is(err, store.ErrNoChannel) {
		return v, nil
	}
	if err != nil {
		return store.PushedVideo{}, err
	}
	verdict := movies.Classify(movies.VideoText{Title: e.Title}, includeAll, tournaments)
	v.TTA, v.Reason = verdict.TTA, verdict.Reason
	return v, nil
}
