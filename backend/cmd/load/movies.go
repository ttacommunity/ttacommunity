// The daily Movies job: it reads every channel's videos again, renews every channel's subscription to
// YouTube's pushes, then drops the videos YouTube no longer shows and reads the details still missing.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"ttacommunity/backend/internal/movies"
	"ttacommunity/backend/internal/moviesync"
	"ttacommunity/backend/internal/store"
)

// videosPerCall is the most ids one videos.list call takes.
const videosPerCall = 50

// movieJobStore is what the job reads and writes, through the admin login.
type movieJobStore interface {
	MovieJobChannels(ctx context.Context) ([]store.MovieJobChannel, error)
	MovieJobChannel(ctx context.Context, channelID string) (store.MovieJobChannel, error)
	MovieJobVideos(ctx context.Context) ([]store.MovieJobVideo, error)
	MovieTournamentNames(ctx context.Context) ([]string, error)
	DeleteMovieVideo(ctx context.Context, videoID string) error
	moviesync.Store
}

// movieJob is one run of the job at now, setting each channel up with sync, whose store is store; only the
// channel only when it is set.
type movieJob struct {
	store movieJobStore
	sync  moviesync.Syncer
	now   time.Time
	only  string
}

// movieJobReport counts what a run did.
type movieJobReport struct {
	Channels, Subscribed, Failed, Filled, Removed, Classified int
}

// run does the day's work. A channel's failure is recorded on the channel, and the run carries on; any failure
// makes run's error, so the job shows as failed.
func (j movieJob) run(ctx context.Context) (movieJobReport, error) {
	var r movieJobReport
	tournaments, err := j.store.MovieTournamentNames(ctx)
	if err != nil {
		return r, err
	}
	channels, err := j.channels(ctx)
	if err != nil {
		return r, err
	}
	includeAll := map[string]bool{}
	for _, ch := range channels {
		includeAll[ch.ChannelID] = ch.IncludeAll
		r.Channels++
		done, err := j.sync.Channel(ctx, ch, tournaments, j.now)
		if err != nil {
			return r, err
		}
		r.Filled += done.Filled
		if done.Subscribed {
			r.Subscribed++
		}
		if done.Problem != "" {
			r.Failed++
		}
	}
	if err := j.check(ctx, includeAll, tournaments, &r); err != nil {
		return r, fmt.Errorf("checking the stored videos: %w", err)
	}
	if r.Failed > 0 {
		return r, fmt.Errorf("%d of %d channels failed: admin's Movies tab says why", r.Failed, r.Channels)
	}
	return r, nil
}

// channels returns the linked channels, or only the channel only when it is set: store.ErrNoChannel when that
// one is not linked.
func (j movieJob) channels(ctx context.Context) ([]store.MovieJobChannel, error) {
	if j.only == "" {
		return j.store.MovieJobChannels(ctx)
	}
	ch, err := j.store.MovieJobChannel(ctx, j.only)
	if err != nil {
		return nil, err
	}
	return []store.MovieJobChannel{ch}, nil
}

// check asks the Data API about every stored video, 50 at a time: it deletes those YouTube no longer shows,
// and classifies with their details those stored without. A failed call stops it, deleting nothing more.
func (j movieJob) check(ctx context.Context, includeAll map[string]bool, tournaments []string, r *movieJobReport) error {
	stored, err := j.store.MovieJobVideos(ctx)
	if err != nil {
		return err
	}
	if j.only != "" {
		stored = slices.DeleteFunc(stored, func(v store.MovieJobVideo) bool { return v.ChannelID != j.only })
	}
	for start := 0; start < len(stored); start += videosPerCall {
		batch := stored[start:min(start+videosPerCall, len(stored))]
		ids := make([]string, len(batch))
		for i, v := range batch {
			ids[i] = v.VideoID
		}
		details, err := j.sync.YouTube.Videos(ctx, ids)
		if err != nil {
			return err
		}
		for _, v := range batch {
			d, ok := details[v.VideoID]
			switch {
			case !ok:
				if err := j.store.DeleteMovieVideo(ctx, v.VideoID); err != nil {
					return err
				}
				r.Removed++
			case !v.DetailsFetched:
				pv := store.PushedVideo{VideoID: v.VideoID, ChannelID: v.ChannelID, Title: v.Title, PublishedAt: v.PublishedAt}
				if err := j.sync.Save(ctx, pv, d, includeAll[v.ChannelID], tournaments, j.now); err != nil {
					return err
				}
				r.Classified++
			}
		}
	}
	return nil
}

// movieJobConfig is what the job needs from its environment.
type movieJobConfig struct {
	databaseURL, youtubeKey, callback string
	hubSecret                         []byte
}

// String names the type without its secrets.
func (c movieJobConfig) String() string {
	return fmt.Sprintf("movieJobConfig{callback: %s, secrets: redacted}", c.callback)
}

// GoString names the type without its secrets, for %#v.
func (c movieJobConfig) GoString() string { return c.String() }

// movieJobConfigFrom reads RATINGS_ADMIN_DATABASE_URL, YOUTUBE_API_KEY, MOVIES_HUB_SECRET and API_ORIGIN, whose
// https callback the hub checks and pushes to. Any missing, a short secret or a plain-http origin is an error.
func movieJobConfigFrom(getenv func(string) string) (movieJobConfig, error) {
	c := movieJobConfig{
		databaseURL: getenv("RATINGS_ADMIN_DATABASE_URL"),
		youtubeKey:  getenv("YOUTUBE_API_KEY"),
		hubSecret:   []byte(getenv("MOVIES_HUB_SECRET")),
	}
	origin := getenv("API_ORIGIN")
	for name, v := range map[string]string{"RATINGS_ADMIN_DATABASE_URL": c.databaseURL, "YOUTUBE_API_KEY": c.youtubeKey, "API_ORIGIN": origin} {
		if v == "" {
			return movieJobConfig{}, fmt.Errorf("%s is not set", name)
		}
	}
	if len(c.hubSecret) < movies.MinHubSecret {
		return movieJobConfig{}, fmt.Errorf("MOVIES_HUB_SECRET must be at least %d bytes", movies.MinHubSecret)
	}
	if !strings.HasPrefix(origin, "https://") {
		return movieJobConfig{}, errors.New("API_ORIGIN must be the api's https origin, which YouTube's hub can reach")
	}
	c.callback = strings.TrimSuffix(origin, "/") + movies.HubPath
	return c, nil
}

// runMovies runs the job at now on the admin login, refusing a login that can change more than admin edits.
func runMovies(now time.Time, only string) error {
	cfg, err := movieJobConfigFrom(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := store.OpenPool(ctx, cfg.databaseURL)
	if err != nil {
		return errors.New("opening the admin pool failed")
	}
	defer pool.Close()
	admin := store.NewAdminStore(pool)
	if err := admin.CheckLimits(ctx); err != nil {
		return err
	}
	job := movieJob{
		store: admin,
		sync: moviesync.Syncer{
			Store:   admin,
			YouTube: movies.NewYouTube(nil, movies.DataAPI, cfg.youtubeKey),
			Hub:     movies.NewHubClient(nil, movies.Hub, cfg.callback, cfg.hubSecret),
		},
		now:  now.UTC(),
		only: only,
	}
	r, err := job.run(ctx)
	fmt.Printf("channels: %d, subscribed: %d, failed: %d; videos filled in: %d, removed: %d, classified: %d\n",
		r.Channels, r.Subscribed, r.Failed, r.Filled, r.Removed, r.Classified)
	return err
}
