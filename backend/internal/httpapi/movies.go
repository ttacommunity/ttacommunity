// Public Movies feed and signed-in player's seen marker.
package httpapi

import (
	"context"
	"time"
)

func (s *server) GetMovies(ctx context.Context, _ GetMoviesRequestObject) (GetMoviesResponseObject, error) {
	feed, err := s.ratings.Movies(ctx)
	if err != nil {
		return nil, err
	}
	creators := make([]MovieCreator, len(feed.Creators))
	for i, c := range feed.Creators {
		creators[i] = MovieCreator{ChannelId: c.ChannelID, Name: c.Name, Player: c.Player}
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
		Body:    MoviesFeed{Creators: creators, Videos: videos},
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
