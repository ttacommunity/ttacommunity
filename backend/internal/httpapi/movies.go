// Public Movies feed and signed-in player's seen marker.
package httpapi

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"ttacommunity/backend/internal/movies"
	"ttacommunity/backend/internal/store"
)

// moviesQuery reads GetMovies' parameters, or says which one is refused.
func moviesQuery(p GetMoviesParams) (store.MoviesQuery, string) {
	q := store.MoviesQuery{Page: 1}
	if p.Page != nil {
		q.Page = *p.Page
	}
	if q.Page < 1 {
		return q, "bad parameter: page"
	}
	if p.Sort != nil {
		if !p.Sort.Valid() {
			return q, "bad parameter: sort"
		}
		q.Sort = string(*p.Sort)
	}
	if p.Creator != nil {
		if len(*p.Creator) > 50 {
			return q, "bad parameter: creator"
		}
		for _, c := range *p.Creator {
			if id, ok := movies.ParseChannelID(c); !ok || id != c {
				return q, "bad parameter: creator"
			}
		}
		q.Channels = *p.Creator
	}
	if p.Player != nil {
		q.Player = strings.TrimSpace(*p.Player)
		if q.Player == "" || utf8.RuneCountInString(*p.Player) > 200 {
			return q, "bad parameter: player"
		}
	}
	if p.Q != nil {
		if utf8.RuneCountInString(*p.Q) > 100 {
			return q, "bad parameter: q"
		}
		q.Search = strings.TrimSpace(*p.Q)
	}
	return q, ""
}

// GetMovies returns one page of the Movies feed: the chosen creators' videos in the chosen order, and every creator.
func (s *server) GetMovies(ctx context.Context, req GetMoviesRequestObject) (GetMoviesResponseObject, error) {
	q, msg := moviesQuery(req.Params)
	if msg != "" {
		return GetMovies400JSONResponse{badRequestBody(msg)}, nil
	}
	feed, err := s.ratings.Movies(ctx, q)
	if errors.Is(err, store.ErrNoChannel) {
		return GetMovies404JSONResponse{notFound404("no linked channel for that creator or player")}, nil
	}
	if err != nil {
		return nil, err
	}
	creators := make([]MovieCreator, len(feed.Creators))
	for i, c := range feed.Creators {
		creators[i] = MovieCreator{ChannelId: c.ChannelID, Name: c.Name, Player: c.Player, Videos: c.Videos}
	}
	videos := make([]MovieVideo, len(feed.Videos))
	for i, v := range feed.Videos {
		videos[i] = MovieVideo{
			VideoId:       v.VideoID,
			ChannelId:     v.ChannelID,
			CreatorName:   v.CreatorName,
			CreatorPlayer: v.CreatorPlayer,
			Title:         v.Title,
			PublishedAt:   v.PublishedAt,
			ThumbnailUrl:  v.ThumbnailURL,
		}
	}
	return GetMovies200JSONResponse{
		Body:    MoviesFeed{Creators: creators, Page: feed.Page, Pages: feed.Pages, Total: feed.Total, Videos: videos},
		Headers: GetMovies200ResponseHeaders{CacheControl: cacheCurrent},
	}, nil
}

func (s *server) PostMyMoviesSeen(ctx context.Context, _ PostMyMoviesSeenRequestObject) (PostMyMoviesSeenResponseObject, error) {
	l, err := accountFrom(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.signIns.MarkMoviesSeen(ctx, l.ID, s.now().UTC().Truncate(time.Second)); err != nil {
		return nil, err
	}
	return PostMyMoviesSeen204Response{}, nil
}
