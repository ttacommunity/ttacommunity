// Admin's Reddit feeds: the subreddits whose new posts the Reddit job queues for Discord. Admins only.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ttacommunity/backend/internal/store"
)

const onlyAdminsReddit = "only admins manage Reddit feeds"

// feedOut is a feed as the API sends it.
func feedOut(f store.RedditFeed) RedditFeed {
	out := RedditFeed{Id: f.ID, Subreddit: f.Subreddit, Paused: f.Paused, CheckedAt: inUTC(f.CheckedAt), LastPostAt: inUTC(f.LastPostAt), LastErrorAt: inUTC(f.LastErrorAt)}
	if f.LastError != "" {
		out.LastError = &f.LastError
	}
	return out
}

// subredditName is the name typed, without spaces, r/ or a trailing slash, as admins often paste it.
func subredditName(typed string) string {
	name := strings.Trim(strings.TrimSpace(typed), "/")
	if len(name) > 2 && strings.EqualFold(name[:2], "r/") {
		name = name[2:]
	}
	return name
}

func (s *server) GetAdminRedditFeeds(ctx context.Context, _ GetAdminRedditFeedsRequestObject) (GetAdminRedditFeedsResponseObject, error) {
	if _, ok, err := adminOnly(ctx); err != nil {
		return nil, err
	} else if !ok {
		return GetAdminRedditFeeds403JSONResponse{forbidden(onlyAdminsReddit)}, nil
	}
	list, err := s.admins.RedditFeeds(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RedditFeed, len(list))
	for i, f := range list {
		out[i] = feedOut(f)
	}
	return GetAdminRedditFeeds200JSONResponse{Body: out, Headers: GetAdminRedditFeeds200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PostAdminRedditFeed(ctx context.Context, req PostAdminRedditFeedRequestObject) (PostAdminRedditFeedResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PostAdminRedditFeed403JSONResponse{forbidden(onlyAdminsReddit)}, nil
	}
	if req.Body == nil || subredditName(req.Body.Subreddit) == "" {
		return PostAdminRedditFeed400JSONResponse{badRequest400("a subreddit's name")}, nil
	}
	f := store.RedditFeed{Subreddit: subredditName(req.Body.Subreddit)}
	err = s.admins.Change(ctx, func(tx Admins) error {
		var err error
		if f.ID, err = tx.AddRedditFeed(ctx, f.Subreddit, s.now()); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "reddit.feed.add", f.Subreddit, map[string]any{})
	})
	switch {
	case errors.Is(err, store.ErrBadRedditFeed):
		return PostAdminRedditFeed400JSONResponse{badRequest400(store.ErrBadRedditFeed.Error())}, nil
	case errors.Is(err, store.ErrRedditFeedExists):
		return PostAdminRedditFeed409JSONResponse{conflict409(store.ErrRedditFeedExists.Error())}, nil
	case err != nil:
		return nil, err
	}
	return PostAdminRedditFeed201JSONResponse{Body: feedOut(f), Headers: PostAdminRedditFeed201ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PutAdminRedditFeed(ctx context.Context, req PutAdminRedditFeedRequestObject) (PutAdminRedditFeedResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PutAdminRedditFeed403JSONResponse{forbidden(onlyAdminsReddit)}, nil
	}
	if req.Body == nil {
		return PutAdminRedditFeed400JSONResponse{badRequest400("paused: true or false")}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetRedditFeedPaused(ctx, req.Feed, req.Body.Paused); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "reddit.feed.set", fmt.Sprint(req.Feed), req.Body)
	})
	switch {
	case errors.Is(err, store.ErrNoRedditFeed):
		return PutAdminRedditFeed404JSONResponse{notFound404("no such feed")}, nil
	case err != nil:
		return nil, err
	}
	return PutAdminRedditFeed204Response{}, nil
}

func (s *server) DeleteAdminRedditFeed(ctx context.Context, req DeleteAdminRedditFeedRequestObject) (DeleteAdminRedditFeedResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteAdminRedditFeed403JSONResponse{forbidden(onlyAdminsReddit)}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.RemoveRedditFeed(ctx, req.Feed); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "reddit.feed.remove", fmt.Sprint(req.Feed), map[string]any{})
	})
	switch {
	case errors.Is(err, store.ErrNoRedditFeed):
		return DeleteAdminRedditFeed404JSONResponse{notFound404("no such feed")}, nil
	case err != nil:
		return nil, err
	}
	return DeleteAdminRedditFeed204Response{}, nil
}
