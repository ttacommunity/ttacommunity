// Admin's Twitch streamers: the feeds whose streams going live are posted to Discord, with their subscriptions at
// Twitch. Admins only.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"ttacommunity/backend/internal/store"
	"ttacommunity/backend/internal/twitch"
)

const onlyAdminsTwitch = "only admins manage Twitch streamers"

// ErrTwitchNotSetUp means Twitch's app and EventSub secret are not configured here.
var ErrTwitchNotSetUp = errors.New("no Twitch app is set up here")

// twitchEvents are the EventSub subscriptions each streamer gets.
var twitchEvents = []string{"stream.online", "stream.offline"}

// twitchChannel finds a login in a channel's address.
var twitchChannel = regexp.MustCompile(`(?i)twitch\.tv/([^/?#\s]+)`)

// twitchLogin is the login in what was typed: a login or the channel's address, in lower case.
func twitchLogin(typed string) string {
	t := strings.TrimSpace(typed)
	if m := twitchChannel.FindStringSubmatch(t); m != nil {
		t = m[1]
	}
	return strings.ToLower(strings.Trim(t, "/@"))
}

// twitchFeedOut is a Twitch feed as the API sends it, with its subscriptions' types and statuses.
func twitchFeedOut(f store.TwitchFeed) TwitchFeed {
	subs := make([]TwitchSubscriptionState, len(f.Subscriptions))
	for i, s := range f.Subscriptions {
		subs[i] = TwitchSubscriptionState{Type: s.Type, Status: s.Status}
	}
	return TwitchFeed{Id: f.ID, Login: f.Login, DisplayName: f.DisplayName, UserId: f.UserID, Paused: f.Paused,
		Subscriptions: subs, CreatedAt: f.CreatedAt}
}

func (s *server) GetAdminTwitchFeeds(ctx context.Context, _ GetAdminTwitchFeedsRequestObject) (GetAdminTwitchFeedsResponseObject, error) {
	if _, ok, err := adminOnly(ctx); err != nil {
		return nil, err
	} else if !ok {
		return GetAdminTwitchFeeds403JSONResponse{forbidden(onlyAdminsTwitch)}, nil
	}
	list, err := s.admins.TwitchFeeds(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]TwitchFeed, len(list))
	for i, f := range list {
		out[i] = twitchFeedOut(f)
	}
	return GetAdminTwitchFeeds200JSONResponse{Body: out, Headers: GetAdminTwitchFeeds200ResponseHeaders{CacheControl: cacheNone}}, nil
}

// PostAdminTwitchFeed looks the streamer up on Twitch, adds them with its audit, then subscribes to their events;
// a subscription Twitch refuses leaves the feed without it, shown in its subscriptions.
func (s *server) PostAdminTwitchFeed(ctx context.Context, req PostAdminTwitchFeedRequestObject) (PostAdminTwitchFeedResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PostAdminTwitchFeed403JSONResponse{forbidden(onlyAdminsTwitch)}, nil
	}
	login := ""
	if req.Body != nil {
		login = twitchLogin(req.Body.Login)
	}
	if login == "" {
		return PostAdminTwitchFeed400JSONResponse{badRequest400("a streamer's Twitch name")}, nil
	}
	user, err := s.admins.TwitchUser(ctx, login)
	switch {
	case errors.Is(err, twitch.ErrNoUser):
		return PostAdminTwitchFeed400JSONResponse{badRequest400("Twitch has no streamer named " + login)}, nil
	case errors.Is(err, ErrTwitchNotSetUp):
		return PostAdminTwitchFeed503JSONResponse{unavailable503(ErrTwitchNotSetUp.Error())}, nil
	case err != nil:
		s.log.Warn("looking a streamer up on Twitch failed", "err", err)
		return PostAdminTwitchFeed503JSONResponse{unavailable503("Twitch could not be reached; try again")}, nil
	}
	now := s.now().UTC()
	f := store.TwitchFeed{Login: user.Login, UserID: user.ID, DisplayName: user.DisplayName, CreatedAt: now}
	err = s.admins.Change(ctx, func(tx Admins) error {
		var err error
		if f.ID, err = tx.AddTwitchFeed(ctx, f.Login, f.UserID, f.DisplayName, now); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "twitch_feed.add", f.Login, map[string]string{"user_id": f.UserID})
	})
	switch {
	case errors.Is(err, store.ErrTwitchFeedExists):
		return PostAdminTwitchFeed409JSONResponse{conflict409(store.ErrTwitchFeedExists.Error())}, nil
	case errors.Is(err, store.ErrBadTwitchFeed):
		return PostAdminTwitchFeed400JSONResponse{badRequest400(store.ErrBadTwitchFeed.Error())}, nil
	case err != nil:
		return nil, err
	}
	for _, typ := range twitchEvents {
		sub, err := s.admins.SubscribeTwitch(ctx, typ, f.UserID)
		if err != nil {
			s.log.Warn("subscribing to a streamer's events failed", "type", typ, "err", err)
			continue
		}
		if err := s.admins.SetTwitchSubscription(ctx, f.ID, typ, sub.ID, sub.Status, now); err != nil {
			return nil, err
		}
		f.Subscriptions = append(f.Subscriptions, store.TwitchSubscription{Type: typ, ID: sub.ID, Status: sub.Status})
	}
	return PostAdminTwitchFeed201JSONResponse{Body: twitchFeedOut(f), Headers: PostAdminTwitchFeed201ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PutAdminTwitchFeed(ctx context.Context, req PutAdminTwitchFeedRequestObject) (PutAdminTwitchFeedResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PutAdminTwitchFeed403JSONResponse{forbidden(onlyAdminsTwitch)}, nil
	}
	if req.Body == nil {
		return PutAdminTwitchFeed400JSONResponse{badRequest400("a streamer's setting: paused")}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetTwitchFeedPaused(ctx, req.Feed, req.Body.Paused); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "twitch_feed.set", fmt.Sprint(req.Feed), req.Body)
	})
	if errors.Is(err, store.ErrNoTwitchFeed) {
		return PutAdminTwitchFeed404JSONResponse{notFound404("no such Twitch streamer")}, nil
	}
	if err != nil {
		return nil, err
	}
	return PutAdminTwitchFeed204Response{}, nil
}

// DeleteAdminTwitchFeed unsubscribes the streamer's events at Twitch, then removes the feed with its audit; a
// subscription Twitch cannot be asked to drop is left, its pushes ignored for a streamer no longer listed.
func (s *server) DeleteAdminTwitchFeed(ctx context.Context, req DeleteAdminTwitchFeedRequestObject) (DeleteAdminTwitchFeedResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteAdminTwitchFeed403JSONResponse{forbidden(onlyAdminsTwitch)}, nil
	}
	f, err := s.admins.TwitchFeed(ctx, req.Feed)
	if errors.Is(err, store.ErrNoTwitchFeed) {
		return DeleteAdminTwitchFeed404JSONResponse{notFound404("no such Twitch streamer")}, nil
	}
	if err != nil {
		return nil, err
	}
	for _, sub := range f.Subscriptions {
		if err := s.admins.UnsubscribeTwitch(ctx, sub.ID); err != nil {
			s.log.Warn("unsubscribing from a streamer's events failed", "type", sub.Type, "err", err)
		}
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.RemoveTwitchFeed(ctx, req.Feed); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "twitch_feed.remove", fmt.Sprint(req.Feed), map[string]any{})
	})
	if errors.Is(err, store.ErrNoTwitchFeed) {
		return DeleteAdminTwitchFeed404JSONResponse{notFound404("no such Twitch streamer")}, nil
	}
	if err != nil {
		return nil, err
	}
	return DeleteAdminTwitchFeed204Response{}, nil
}
