// Admin management of YouTube creator channels and overrides of videos' verdicts.
package httpapi

import (
	"context"
	"errors"
	"strings"
	"time"

	"ttacommunity/backend/internal/enginejob"
	"ttacommunity/backend/internal/movies"
	"ttacommunity/backend/internal/store"
)

const onlyAdminsMovies = "only admins manage movie channels"

func (s *server) GetAdminMovieChannels(ctx context.Context, _ GetAdminMovieChannelsRequestObject) (GetAdminMovieChannelsResponseObject, error) {
	if _, ok, err := adminOnly(ctx); err != nil {
		return nil, err
	} else if !ok {
		return GetAdminMovieChannels403JSONResponse{forbidden(onlyAdminsMovies)}, nil
	}
	state, err := s.admins.AdminMovies(ctx)
	if err != nil {
		return nil, err
	}
	channels := make([]AdminVideoChannel, len(state.Channels))
	for i, c := range state.Channels {
		channels[i] = AdminVideoChannel{
			ChannelId:    c.ChannelID,
			Name:         c.Name,
			Player:       c.Player,
			IncludeAll:   c.IncludeAll,
			CreatedAt:    c.CreatedAt.UTC(),
			SubscribedAt: inUTC(c.SubscribedAt),
			LeaseEndsAt:  inUTC(c.LeaseEndsAt),
			LastPushAt:   inUTC(c.LastPushAt),
			LastError:    c.LastError,
			LastErrorAt:  inUTC(c.LastErrorAt),
			VideoCount:   c.VideoCount,
			VideosFound:  c.VideosFound,
		}
	}
	videos := make([]AdminVideo, len(state.Videos))
	for i, v := range state.Videos {
		videos[i] = AdminVideo{
			VideoId:      v.VideoID,
			ChannelId:    v.ChannelID,
			CreatorName:  v.CreatorName,
			Title:        v.Title,
			PublishedAt:  v.PublishedAt.UTC(),
			ThumbnailUrl: v.ThumbnailURL,
			IsTta:        v.IsTTA,
			TtaReason:    v.TTAReason,
			TtaOverride:  v.TTAOverride,
			Shown:        v.Shown,
		}
	}
	running, err := s.admins.MovieJobGoing(ctx)
	if err != nil && !errors.Is(err, enginejob.ErrNotSetUp) {
		s.log.Error("asking the cluster whether the Movies job is going failed; it is shown as not running", "err", err)
	}
	return GetAdminMovieChannels200JSONResponse{
		Body:    AdminMoviesState{Channels: channels, Videos: videos, JobRunning: running && err == nil},
		Headers: GetAdminMovieChannels200ResponseHeaders{CacheControl: cacheNone},
	}, nil
}

func (s *server) PostAdminMovieChannel(ctx context.Context, req PostAdminMovieChannelRequestObject) (PostAdminMovieChannelResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PostAdminMovieChannel403JSONResponse{forbidden(onlyAdminsMovies)}, nil
	}
	if req.Body == nil || strings.TrimSpace(req.Body.Channel) == "" || strings.TrimSpace(req.Body.Name) == "" {
		return PostAdminMovieChannel400JSONResponse{badRequest400("channel and creator name are required")}, nil
	}
	channelID, err := s.admins.ResolveMovieChannel(ctx, req.Body.Channel)
	for _, refused := range []error{movies.ErrNotAChannel, movies.ErrNoSuchChannel, movies.ErrNoYouTube} {
		if errors.Is(err, refused) {
			return PostAdminMovieChannel400JSONResponse{badRequest400(refused.Error())}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	now := s.now().UTC().Truncate(time.Second)
	var ch store.AdminVideoChannel
	err = s.admins.Change(ctx, func(tx Admins) error {
		var err error
		ch, err = tx.AddMovieChannel(ctx, channelID, req.Body.Name, req.Body.Player, req.Body.IncludeAll, now)
		if err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "movie.channel.add", channelID, map[string]any{"name": ch.Name})
	})
	switch {
	case errors.Is(err, store.ErrChannelExists):
		return PostAdminMovieChannel409JSONResponse{conflict409("that YouTube channel is already added")}, nil
	case errors.Is(err, store.ErrNoPlayerName):
		return PostAdminMovieChannel400JSONResponse{badRequest400("no player with that name")}, nil
	case err != nil:
		return nil, err
	}
	return PostAdminMovieChannel201JSONResponse{
		ChannelId:  ch.ChannelID,
		Name:       ch.Name,
		Player:     ch.Player,
		IncludeAll: ch.IncludeAll,
		CreatedAt:  ch.CreatedAt.UTC(),
		VideoCount: ch.VideoCount,
	}, nil
}

func (s *server) PatchAdminMovieChannel(ctx context.Context, req PatchAdminMovieChannelRequestObject) (PatchAdminMovieChannelResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PatchAdminMovieChannel403JSONResponse{forbidden(onlyAdminsMovies)}, nil
	}
	if req.Body == nil {
		return PatchAdminMovieChannel400JSONResponse{badRequest400("request body is required")}, nil
	}
	clearPlayer := req.Body.Player != nil && strings.TrimSpace(*req.Body.Player) == ""
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.UpdateMovieChannel(ctx, req.ChannelId, req.Body.Name, req.Body.Player, clearPlayer, req.Body.IncludeAll); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "movie.channel.update", req.ChannelId, nil)
	})
	switch {
	case errors.Is(err, store.ErrNoChannel):
		return PatchAdminMovieChannel404JSONResponse{notFound404("no such YouTube channel")}, nil
	case errors.Is(err, store.ErrNoPlayerName):
		return PatchAdminMovieChannel400JSONResponse{badRequest400("no player with that name")}, nil
	case err != nil:
		return nil, err
	}
	return PatchAdminMovieChannel204Response{}, nil
}

func (s *server) DeleteAdminMovieChannel(ctx context.Context, req DeleteAdminMovieChannelRequestObject) (DeleteAdminMovieChannelResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteAdminMovieChannel403JSONResponse{forbidden(onlyAdminsMovies)}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.DeleteMovieChannel(ctx, req.ChannelId); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "movie.channel.delete", req.ChannelId, nil)
	})
	if errors.Is(err, store.ErrNoChannel) {
		return DeleteAdminMovieChannel404JSONResponse{notFound404("no such YouTube channel")}, nil
	}
	if err != nil {
		return nil, err
	}
	return DeleteAdminMovieChannel204Response{}, nil
}

func (s *server) PatchAdminMovieVideo(ctx context.Context, req PatchAdminMovieVideoRequestObject) (PatchAdminMovieVideoResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PatchAdminMovieVideo403JSONResponse{forbidden(onlyAdminsMovies)}, nil
	}
	if req.Body == nil {
		return PatchAdminMovieVideo400JSONResponse{badRequest400("request body is required")}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetMovieVideoOverride(ctx, req.VideoId, req.Body.Override); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "movie.video.override", req.VideoId, map[string]any{"override": req.Body.Override})
	})
	if errors.Is(err, store.ErrNoVideo) {
		return PatchAdminMovieVideo404JSONResponse{notFound404("no such video")}, nil
	}
	if err != nil {
		return nil, err
	}
	return PatchAdminMovieVideo204Response{}, nil
}

func (s *server) PostAdminMoviesRun(ctx context.Context, req PostAdminMoviesRunRequestObject) (PostAdminMoviesRunResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PostAdminMoviesRun403JSONResponse{forbidden(onlyAdminsMovies)}, nil
	}
	channel := ""
	if req.Body != nil && req.Body.Channel != nil {
		channel = strings.TrimSpace(*req.Body.Channel)
	}
	// The Job starts before the commit, so it runs unaudited only if the audit row or the commit fails.
	var job string
	err = s.admins.Change(ctx, func(tx Admins) error {
		var err error
		if job, err = tx.StartMovieJob(ctx, channel); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "movie.run", job, map[string]any{"channel": channel})
	})
	switch {
	case errors.Is(err, store.ErrNoChannel):
		return PostAdminMoviesRun404JSONResponse{notFound404("no such YouTube channel")}, nil
	case errors.Is(err, enginejob.ErrRunning):
		return PostAdminMoviesRun409JSONResponse{conflict409("a Movies job is already going")}, nil
	case errors.Is(err, enginejob.ErrNotSetUp):
		return PostAdminMoviesRun503JSONResponse{NotSetUpJSONResponse{Body: Error{Error: "starting the Movies job is not set up here"}, Headers: NotSetUpResponseHeaders{CacheControl: cacheNone}}}, nil
	case err != nil:
		return nil, err
	}
	return PostAdminMoviesRun202JSONResponse{Job: job}, nil
}
