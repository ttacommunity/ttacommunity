// Admin's YouTube Live feed: the one feed of Movies' channels' live streams, added, paused and removed. Admins only.
package httpapi

import (
	"context"
	"errors"
	"fmt"

	"ttacommunity/backend/internal/store"
)

const onlyAdminsYouTube = "only admins manage the YouTube Live feed"

// youtubeFeedOut is the YouTube feed as the API sends it.
func youtubeFeedOut(f store.YouTubeFeed) YouTubeFeed {
	return YouTubeFeed{Id: f.ID, Paused: f.Paused, Channels: f.Channels, Waiting: f.Waiting, Live: f.Live, CreatedAt: f.CreatedAt}
}

func (s *server) GetAdminYouTubeFeeds(ctx context.Context, _ GetAdminYouTubeFeedsRequestObject) (GetAdminYouTubeFeedsResponseObject, error) {
	if _, ok, err := adminOnly(ctx); err != nil {
		return nil, err
	} else if !ok {
		return GetAdminYouTubeFeeds403JSONResponse{forbidden(onlyAdminsYouTube)}, nil
	}
	list, err := s.admins.YouTubeFeeds(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]YouTubeFeed, len(list))
	for i, f := range list {
		out[i] = youtubeFeedOut(f)
	}
	return GetAdminYouTubeFeeds200JSONResponse{Body: out, Headers: GetAdminYouTubeFeeds200ResponseHeaders{CacheControl: cacheNone}}, nil
}

// PostAdminYouTubeFeed adds the YouTube Live feed with its audit; 503 without the Movies hub callback, which is
// what tells of streams.
func (s *server) PostAdminYouTubeFeed(ctx context.Context, _ PostAdminYouTubeFeedRequestObject) (PostAdminYouTubeFeedResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PostAdminYouTubeFeed403JSONResponse{forbidden(onlyAdminsYouTube)}, nil
	}
	if s.movieHub == nil {
		return PostAdminYouTubeFeed503JSONResponse{unavailable503("YouTube's push is not set up here: MOVIES_HUB_SECRET is not set")}, nil
	}
	f := store.YouTubeFeed{CreatedAt: s.now().UTC()}
	err = s.admins.Change(ctx, func(tx Admins) error {
		var err error
		if f.ID, err = tx.AddYouTubeFeed(ctx, f.CreatedAt); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "youtube.feed.add", fmt.Sprint(f.ID), map[string]any{})
	})
	switch {
	case errors.Is(err, store.ErrYouTubeFeedExists):
		return PostAdminYouTubeFeed409JSONResponse{conflict409(store.ErrYouTubeFeedExists.Error())}, nil
	case err != nil:
		return nil, err
	}
	return PostAdminYouTubeFeed201JSONResponse{Body: youtubeFeedOut(f), Headers: PostAdminYouTubeFeed201ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PutAdminYouTubeFeed(ctx context.Context, req PutAdminYouTubeFeedRequestObject) (PutAdminYouTubeFeedResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PutAdminYouTubeFeed403JSONResponse{forbidden(onlyAdminsYouTube)}, nil
	}
	if req.Body == nil {
		return PutAdminYouTubeFeed400JSONResponse{badRequest400("the feed's setting: paused")}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetYouTubeFeedPaused(ctx, req.Feed, req.Body.Paused); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "youtube.feed.set", fmt.Sprint(req.Feed), req.Body)
	})
	switch {
	case errors.Is(err, store.ErrNoYouTubeFeed):
		return PutAdminYouTubeFeed404JSONResponse{notFound404("no such feed")}, nil
	case err != nil:
		return nil, err
	}
	return PutAdminYouTubeFeed204Response{}, nil
}

func (s *server) DeleteAdminYouTubeFeed(ctx context.Context, req DeleteAdminYouTubeFeedRequestObject) (DeleteAdminYouTubeFeedResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteAdminYouTubeFeed403JSONResponse{forbidden(onlyAdminsYouTube)}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.RemoveYouTubeFeed(ctx, req.Feed); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "youtube.feed.remove", fmt.Sprint(req.Feed), map[string]any{})
	})
	switch {
	case errors.Is(err, store.ErrNoYouTubeFeed):
		return DeleteAdminYouTubeFeed404JSONResponse{notFound404("no such feed")}, nil
	case err != nil:
		return nil, err
	}
	return DeleteAdminYouTubeFeed204Response{}, nil
}
