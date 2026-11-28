// Public Movies feed queries: active YouTube creators and chronological non-hidden videos.
package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// hasVideos is whether player p has a linked channel with a video the Movies page shows.
const hasVideos = `EXISTS (
	SELECT 1 FROM video_channels vc JOIN videos v USING (channel_id)
	WHERE vc.player_id = p.player_id AND coalesce(v.tta_override, v.is_tta)
)`

// MovieCreator is one YouTube creator listed in the Movies filter, with how many videos the page shows of theirs.
type MovieCreator struct {
	ChannelID string
	Name      string
	Player    *string
	Videos    int
}

// MovieVideo is one visible video in the public Movies feed.
type MovieVideo struct {
	VideoID       string
	ChannelID     string
	CreatorName   string
	CreatorPlayer *string
	Title         string
	PublishedAt   time.Time
	ThumbnailURL  string
}

// MoviesQuery picks a page of the Movies feed: the creators' channels or a player's (all when both are empty),
// the sort (newest, oldest, title or creator; newest when empty) and the page.
type MoviesQuery struct {
	Channels []string
	Player   string
	Sort     string
	Page     int
}

// MoviesFeed holds every creator and one page of the chosen creators' videos.
type MoviesFeed struct {
	Creators           []MovieCreator
	Page, Pages, Total int
	Videos             []MovieVideo
}

// moviesPerPage is how many videos one page of the Movies feed holds.
const moviesPerPage = 24

// movieOrders maps each sort to its ORDER BY, each ending in video_id so pages never overlap.
var movieOrders = map[string]string{
	"":        "v.published_at DESC, v.video_id",
	"newest":  "v.published_at DESC, v.video_id",
	"oldest":  "v.published_at, v.video_id",
	"title":   "lower(v.title), v.video_id",
	"creator": "lower(vc.name), vc.channel_id, v.published_at DESC, v.video_id",
}

// Movies returns every creator, with their count of shown videos, and one page of the chosen creators' videos.
// It returns ErrNoChannel when a channel is not linked or the player has none.
func (s *RatingsReader) Movies(ctx context.Context, q MoviesQuery) (MoviesFeed, error) {
	order, ok := movieOrders[q.Sort]
	if !ok {
		return MoviesFeed{}, fmt.Errorf("unknown movie sort %q", q.Sort)
	}
	cRows, err := s.pool.Query(ctx, `
		SELECT vc.channel_id, vc.name, p.name,
		       (SELECT count(*) FROM videos v WHERE v.channel_id = vc.channel_id AND coalesce(v.tta_override, v.is_tta))::int
		FROM video_channels vc
		LEFT JOIN players p ON p.player_id = vc.player_id
		ORDER BY lower(vc.name), vc.channel_id`)
	if err != nil {
		return MoviesFeed{}, fmt.Errorf("reading movie creators: %w", err)
	}
	creators, err := pgx.CollectRows(cRows, pgx.RowToStructByPos[MovieCreator])
	if err != nil {
		return MoviesFeed{}, fmt.Errorf("reading movie creators: %w", err)
	}

	chosen, err := chosenChannels(creators, q)
	if err != nil {
		return MoviesFeed{}, err
	}
	feed := MoviesFeed{Creators: creators}
	for _, c := range creators {
		if chosen == nil || slices.Contains(chosen, c.ChannelID) {
			feed.Total += c.Videos
		}
	}
	feed.Pages = max(1, (feed.Total+moviesPerPage-1)/moviesPerPage)
	feed.Page = min(max(q.Page, 1), feed.Pages)

	vRows, err := s.pool.Query(ctx, `
		SELECT v.video_id, v.channel_id, vc.name, p.name, v.title, v.published_at, v.thumbnail_url
		FROM videos v
		JOIN video_channels vc USING (channel_id)
		LEFT JOIN players p ON p.player_id = vc.player_id
		WHERE coalesce(v.tta_override, v.is_tta) AND ($1::text[] IS NULL OR v.channel_id = ANY($1))
		ORDER BY `+order+`
		LIMIT $2 OFFSET $3`, chosen, moviesPerPage, (feed.Page-1)*moviesPerPage)
	if err != nil {
		return MoviesFeed{}, fmt.Errorf("reading movie videos: %w", err)
	}
	feed.Videos, err = pgx.CollectRows(vRows, pgx.RowToStructByPos[MovieVideo])
	if err != nil {
		return MoviesFeed{}, fmt.Errorf("reading movie videos: %w", err)
	}
	return feed, nil
}

// chosenChannels returns the channels q names, or the player's, from creators; nil for every creator.
func chosenChannels(creators []MovieCreator, q MoviesQuery) ([]string, error) {
	if len(q.Channels) == 0 && q.Player == "" {
		return nil, nil
	}
	var chosen []string
	playerFound := false
	for _, c := range creators {
		ofPlayer := q.Player != "" && c.Player != nil && strings.EqualFold(*c.Player, q.Player)
		playerFound = playerFound || ofPlayer
		if ofPlayer || slices.Contains(q.Channels, c.ChannelID) {
			chosen = append(chosen, c.ChannelID)
		}
	}
	if q.Player != "" && !playerFound {
		return nil, ErrNoChannel
	}
	for _, id := range q.Channels {
		if !slices.ContainsFunc(creators, func(c MovieCreator) bool { return c.ChannelID == id }) {
			return nil, ErrNoChannel
		}
	}
	return chosen, nil
}

// MovieClassifying returns what a verdict on channelID's video needs: whether its every video counts, and every
// tournament's name. It returns ErrNoChannel when the channel is not linked.
func (s *RatingsReader) MovieClassifying(ctx context.Context, channelID string) (bool, []string, error) {
	var includeAll bool
	err := s.pool.QueryRow(ctx, "SELECT include_all FROM video_channels WHERE channel_id = $1", channelID).Scan(&includeAll)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil, ErrNoChannel
	}
	if err != nil {
		return false, nil, fmt.Errorf("reading a video channel: %w", err)
	}
	rows, err := s.pool.Query(ctx, "SELECT name FROM series WHERE btrim(name) <> '' ORDER BY name")
	if err != nil {
		return false, nil, fmt.Errorf("reading tournament names: %w", err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return false, nil, fmt.Errorf("reading tournament names: %w", err)
	}
	return includeAll, names, nil
}
