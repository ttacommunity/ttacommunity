// Admin management of YouTube creator channels, video visibility, and RSS feed syncing.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"ttacommunity/backend/internal/movies"
)

// AdminVideoChannel is one YouTube creator channel in admin.
type AdminVideoChannel struct {
	ChannelID    string
	Name         string
	Player       *string
	IncludeAll   bool
	CreatedAt    time.Time
	SubscribedAt *time.Time
	LeaseEndsAt  *time.Time
	LastPushAt   *time.Time
	LastError    *string
	LastErrorAt  *time.Time
	VideoCount   int
}

// AdminVideo is one fetched video in admin (including hidden videos).
type AdminVideo struct {
	VideoID      string
	ChannelID    string
	CreatorName  string
	Title        string
	PublishedAt  time.Time
	ThumbnailURL string
	Hidden       bool
}

// AdminMoviesState is the admin Movies tab payload.
type AdminMoviesState struct {
	Channels []AdminVideoChannel
	Videos   []AdminVideo
}

// MoviesSyncResult reports how many channels were checked and how many new videos were stored.
type MoviesSyncResult struct {
	Checked int
	Added   int
}

var (
	// ErrChannelExists means that YouTube channel is already registered.
	ErrChannelExists = errors.New("that YouTube channel is already added")
	// ErrNoChannel means no channel has that id.
	ErrNoChannel = errors.New("no such YouTube channel")
	// ErrNoVideo means no video has that id.
	ErrNoVideo = errors.New("no such video")
	// ErrNoPlayerName means no player has that name.
	ErrNoPlayerName = errors.New("no player with that name")
)

// MovieFeedFetcher fetches YouTube channel feeds and optional watch-page keywords.
type MovieFeedFetcher interface {
	FetchChannel(ctx context.Context, channelID string) ([]movies.Entry, error)
	FetchKeywords(ctx context.Context, videoID string) (string, error)
}

// AdminMovies lists all configured YouTube channels and the latest 100 fetched videos.
func (a *AdminStore) AdminMovies(ctx context.Context) (AdminMoviesState, error) {
	cRows, err := a.pool.Query(ctx, `
		SELECT vc.channel_id, vc.name, p.name, vc.include_all, vc.created_at, vc.subscribed_at, vc.lease_ends_at,
		       vc.last_push_at, vc.last_error, vc.last_error_at, count(v.video_id)::int
		FROM video_channels vc
		LEFT JOIN players p ON p.player_id = vc.player_id
		LEFT JOIN videos v ON v.channel_id = vc.channel_id AND coalesce(v.tta_override, v.is_tta)
		GROUP BY vc.channel_id, p.name
		ORDER BY lower(vc.name), vc.channel_id`)
	if err != nil {
		return AdminMoviesState{}, fmt.Errorf("reading video channels: %w", err)
	}
	channels, err := pgx.CollectRows(cRows, pgx.RowToStructByPos[AdminVideoChannel])
	if err != nil {
		return AdminMoviesState{}, fmt.Errorf("reading video channels: %w", err)
	}

	vRows, err := a.pool.Query(ctx, `
		SELECT v.video_id, v.channel_id, vc.name, v.title, v.published_at, v.thumbnail_url, NOT coalesce(v.tta_override, v.is_tta)
		FROM videos v
		JOIN video_channels vc USING (channel_id)
		ORDER BY v.published_at DESC, v.video_id
		LIMIT 100`)
	if err != nil {
		return AdminMoviesState{}, fmt.Errorf("reading admin videos: %w", err)
	}
	videos, err := pgx.CollectRows(vRows, pgx.RowToStructByPos[AdminVideo])
	if err != nil {
		return AdminMoviesState{}, fmt.Errorf("reading admin videos: %w", err)
	}
	return AdminMoviesState{Channels: channels, Videos: videos}, nil
}

// AddMovieChannel stores a new YouTube creator channel, optionally linked to a player by name.
func (a *AdminStore) AddMovieChannel(ctx context.Context, channelID, name string, player *string, includeAll bool, now time.Time) (AdminVideoChannel, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return AdminVideoChannel{}, errors.New("creator name is required")
	}
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return AdminVideoChannel{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	playerID, resolvedName, err := resolveOptionalPlayer(ctx, tx, player)
	if err != nil {
		return AdminVideoChannel{}, err
	}
	var ch AdminVideoChannel
	err = tx.QueryRow(ctx, `
		INSERT INTO video_channels (channel_id, name, player_id, include_all, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING channel_id, name, include_all, created_at`,
		channelID, name, playerID, includeAll, now).Scan(&ch.ChannelID, &ch.Name, &ch.IncludeAll, &ch.CreatedAt)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return AdminVideoChannel{}, ErrChannelExists
		}
		return AdminVideoChannel{}, fmt.Errorf("adding video channel: %w", err)
	}
	ch.Player = resolvedName
	return ch, tx.Commit(ctx)
}

// UpdateMovieChannel updates a channel's display name, optional linked player, or include_all flag.
func (a *AdminStore) UpdateMovieChannel(ctx context.Context, channelID string, name *string, player *string, clearPlayer bool, includeAll *bool) error {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var playerID *int64
	updatePlayer := clearPlayer || player != nil
	if player != nil && strings.TrimSpace(*player) != "" {
		playerID, _, err = resolveOptionalPlayer(ctx, tx, player)
		if err != nil {
			return err
		}
	}
	var trimmedName *string
	if name != nil {
		t := strings.TrimSpace(*name)
		if t == "" {
			return errors.New("creator name cannot be empty")
		}
		trimmedName = &t
	}
	tag, err := tx.Exec(ctx, `
		UPDATE video_channels
		SET name = coalesce($2, name),
		    player_id = CASE WHEN $3 THEN $4 ELSE player_id END,
		    include_all = coalesce($5, include_all)
		WHERE channel_id = $1`,
		channelID, trimmedName, updatePlayer, playerID, includeAll)
	if err != nil {
		return fmt.Errorf("updating video channel: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoChannel
	}
	return tx.Commit(ctx)
}

// DeleteMovieChannel removes a channel and its videos.
func (a *AdminStore) DeleteMovieChannel(ctx context.Context, channelID string) error {
	tag, err := a.pool.Exec(ctx, "DELETE FROM video_channels WHERE channel_id = $1", channelID)
	if err != nil {
		return fmt.Errorf("deleting video channel: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoChannel
	}
	return nil
}

// SetMovieVideoHidden hides or unhides a video in the public Movies feed.
func (a *AdminStore) SetMovieVideoHidden(ctx context.Context, videoID string, hidden bool) error {
	tag, err := a.pool.Exec(ctx, "UPDATE videos SET tta_override = NOT $2 WHERE video_id = $1", videoID, hidden)
	if err != nil {
		return fmt.Errorf("updating video: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoVideo
	}
	return nil
}

// SyncMovies fetches every registered YouTube channel's RSS feed and stores new TTA videos.
func (a *AdminStore) SyncMovies(ctx context.Context, fetcher MovieFeedFetcher, now time.Time) (MoviesSyncResult, error) {
	type chRow struct {
		id         string
		includeAll bool
	}
	rows, err := a.pool.Query(ctx, "SELECT channel_id, include_all FROM video_channels ORDER BY channel_id")
	if err != nil {
		return MoviesSyncResult{}, fmt.Errorf("listing channels for sync: %w", err)
	}
	var channels []chRow
	for rows.Next() {
		var c chRow
		if err := rows.Scan(&c.id, &c.includeAll); err != nil {
			rows.Close()
			return MoviesSyncResult{}, err
		}
		channels = append(channels, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return MoviesSyncResult{}, err
	}
	if len(channels) == 0 {
		return MoviesSyncResult{}, nil
	}

	tRows, err := a.pool.Query(ctx, "SELECT name FROM series WHERE btrim(name) <> ''")
	if err != nil {
		return MoviesSyncResult{}, fmt.Errorf("reading tournament names: %w", err)
	}
	tournaments, err := pgx.CollectRows(tRows, pgx.RowTo[string])
	if err != nil {
		return MoviesSyncResult{}, err
	}

	var res MoviesSyncResult
	for _, ch := range channels {
		entries, err := fetcher.FetchChannel(ctx, ch.id)
		if err != nil {
			continue
		}
		res.Checked++
		for _, e := range entries {
			var exists bool
			if err := a.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM videos WHERE video_id = $1)", e.VideoID).Scan(&exists); err != nil || exists {
				continue
			}
			if !ch.includeAll && !movies.IsTTAVideo(e.Title, e.Description, "", tournaments) {
				kw, _ := fetcher.FetchKeywords(ctx, e.VideoID)
				if !movies.IsTTAVideo(e.Title, e.Description, kw, tournaments) {
					continue
				}
			}
			tag, err := a.pool.Exec(ctx, `
				INSERT INTO videos (video_id, channel_id, title, published_at, thumbnail_url, is_tta, tta_reason, created_at)
				VALUES ($1, $2, $3, $4, $5, true, 'feed sync', $6)
				ON CONFLICT (video_id) DO NOTHING`,
				e.VideoID, ch.id, e.Title, e.PublishedAt, e.ThumbnailURL, now)
			if err == nil && tag.RowsAffected() > 0 {
				res.Added++
			}
		}
	}
	return res, nil
}

func resolveOptionalPlayer(ctx context.Context, tx pgx.Tx, player *string) (*int64, *string, error) {
	if player == nil || strings.TrimSpace(*player) == "" {
		return nil, nil, nil
	}
	var id int64
	var name string
	err := tx.QueryRow(ctx, "SELECT player_id, name FROM players WHERE lower(name) = lower($1)", strings.TrimSpace(*player)).Scan(&id, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrNoPlayerName
	}
	if err != nil {
		return nil, nil, fmt.Errorf("looking up player: %w", err)
	}
	return &id, &name, nil
}
