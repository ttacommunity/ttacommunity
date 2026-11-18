// Admin's RSS feeds: the RSS and Atom feeds whose new items the feeds job queues for Discord. Admins only.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"

	"ttacommunity/backend/internal/rss"
	"ttacommunity/backend/internal/store"
)

const onlyAdminsRSS = "only admins manage RSS feeds"

// rssFeedOut is a feed as the API sends it.
func rssFeedOut(f store.RSSFeed) RSSFeed {
	out := RSSFeed{Id: f.ID, Url: f.URL, Title: f.Title, Paused: f.Paused, CheckedAt: inUTC(f.CheckedAt), LastItemAt: inUTC(f.LastItemAt), LastErrorAt: inUTC(f.LastErrorAt)}
	if f.LastError != "" {
		out.LastError = &f.LastError
	}
	return out
}

// notPublicHost says whether typed names a host that is plainly not on the public internet: localhost, or an
// address that rss.Public refuses. Any other name is checked by the feeds job as it connects.
func notPublicHost(typed string) bool {
	u, err := url.Parse(strings.TrimSpace(typed))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	a, err := netip.ParseAddr(host)
	return err == nil && !rss.Public(a)
}

func (s *server) GetAdminRssFeeds(ctx context.Context, _ GetAdminRssFeedsRequestObject) (GetAdminRssFeedsResponseObject, error) {
	if _, ok, err := adminOnly(ctx); err != nil {
		return nil, err
	} else if !ok {
		return GetAdminRssFeeds403JSONResponse{forbidden(onlyAdminsRSS)}, nil
	}
	list, err := s.admins.RSSFeeds(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RSSFeed, len(list))
	for i, f := range list {
		out[i] = rssFeedOut(f)
	}
	return GetAdminRssFeeds200JSONResponse{Body: out, Headers: GetAdminRssFeeds200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PostAdminRssFeed(ctx context.Context, req PostAdminRssFeedRequestObject) (PostAdminRssFeedResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PostAdminRssFeed403JSONResponse{forbidden(onlyAdminsRSS)}, nil
	}
	if req.Body == nil || strings.TrimSpace(req.Body.Url) == "" {
		return PostAdminRssFeed400JSONResponse{badRequest400("a feed's address")}, nil
	}
	if notPublicHost(req.Body.Url) {
		return PostAdminRssFeed400JSONResponse{badRequest400("a feed's address must be on the public internet")}, nil
	}
	f := store.RSSFeed{URL: strings.TrimSpace(req.Body.Url)}
	err = s.admins.Change(ctx, func(tx Admins) error {
		var err error
		if f.ID, err = tx.AddRSSFeed(ctx, f.URL, s.now()); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "rss.feed.add", f.URL, map[string]any{})
	})
	switch {
	case errors.Is(err, store.ErrBadRSSFeed):
		return PostAdminRssFeed400JSONResponse{badRequest400(store.ErrBadRSSFeed.Error())}, nil
	case errors.Is(err, store.ErrRSSFeedExists):
		return PostAdminRssFeed409JSONResponse{conflict409(store.ErrRSSFeedExists.Error())}, nil
	case err != nil:
		return nil, err
	}
	return PostAdminRssFeed201JSONResponse{Body: rssFeedOut(f), Headers: PostAdminRssFeed201ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PutAdminRssFeed(ctx context.Context, req PutAdminRssFeedRequestObject) (PutAdminRssFeedResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PutAdminRssFeed403JSONResponse{forbidden(onlyAdminsRSS)}, nil
	}
	if req.Body == nil {
		return PutAdminRssFeed400JSONResponse{badRequest400("a feed's setting: paused")}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetRSSFeed(ctx, req.Feed, req.Body.Paused); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "rss.feed.set", fmt.Sprint(req.Feed), req.Body)
	})
	switch {
	case errors.Is(err, store.ErrNoRSSFeed):
		return PutAdminRssFeed404JSONResponse{notFound404("no such feed")}, nil
	case err != nil:
		return nil, err
	}
	return PutAdminRssFeed204Response{}, nil
}

func (s *server) DeleteAdminRssFeed(ctx context.Context, req DeleteAdminRssFeedRequestObject) (DeleteAdminRssFeedResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteAdminRssFeed403JSONResponse{forbidden(onlyAdminsRSS)}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.RemoveRSSFeed(ctx, req.Feed); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "rss.feed.remove", fmt.Sprint(req.Feed), map[string]any{})
	})
	switch {
	case errors.Is(err, store.ErrNoRSSFeed):
		return DeleteAdminRssFeed404JSONResponse{notFound404("no such feed")}, nil
	case err != nil:
		return nil, err
	}
	return DeleteAdminRssFeed204Response{}, nil
}
