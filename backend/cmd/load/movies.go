// The daily Movies job: it fills in each new channel's latest videos, renews every channel's subscription to
// YouTube's pushes, then drops the videos YouTube no longer shows and reads the details still missing.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ttacommunity/backend/internal/movies"
	"ttacommunity/backend/internal/store"
)

// videosPerCall is the most ids one videos.list call takes.
const videosPerCall = 50

// newChannelVideos is how many of a new channel's latest uploads are filled in.
const newChannelVideos = 15

// movieJobStore is what the job reads and writes, through the admin login.
type movieJobStore interface {
	MovieJobChannels(ctx context.Context) ([]store.MovieJobChannel, error)
	MovieJobVideos(ctx context.Context) ([]store.MovieJobVideo, error)
	MovieTournamentNames(ctx context.Context) ([]string, error)
	SaveMovieVideo(ctx context.Context, v store.PushedVideo, at time.Time) error
	DeleteMovieVideo(ctx context.Context, videoID string) error
	MarkMovieChannelBackfilled(ctx context.Context, channelID string, at time.Time) error
	SetMovieChannelError(ctx context.Context, channelID string, msg *string, at time.Time) error
}

// movieJob is one run of the job at now.
type movieJob struct {
	store   movieJobStore
	youtube interface {
		Videos(ctx context.Context, ids []string) (map[string]movies.VideoDetails, error)
		LatestUploads(ctx context.Context, channelID string, n int) ([]string, error)
	}
	hub interface {
		Subscribe(ctx context.Context, channelID string) error
	}
	now time.Time
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
	channels, err := j.store.MovieJobChannels(ctx)
	if err != nil {
		return r, err
	}
	includeAll := map[string]bool{}
	for _, ch := range channels {
		includeAll[ch.ChannelID] = ch.IncludeAll
		r.Channels++
		var problems []string
		if ch.BackfilledAt == nil {
			filled, err := j.fillIn(ctx, ch, tournaments)
			if err != nil {
				problems = append(problems, "filling in its videos: "+err.Error())
			} else if err := j.store.MarkMovieChannelBackfilled(ctx, ch.ChannelID, j.now); err != nil {
				return r, err
			}
			r.Filled += filled
		}
		if err := j.hub.Subscribe(ctx, ch.ChannelID); err != nil {
			problems = append(problems, "subscribing: "+err.Error())
		} else {
			r.Subscribed++
		}
		var msg *string
		if len(problems) > 0 {
			r.Failed++
			joined := strings.Join(problems, "; ")
			msg = &joined
		}
		if err := j.store.SetMovieChannelError(ctx, ch.ChannelID, msg, j.now); err != nil && !errors.Is(err, store.ErrNoChannel) {
			return r, err
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

// fillIn saves ch's latest uploads that YouTube still shows, with their details, and returns how many it saved.
func (j movieJob) fillIn(ctx context.Context, ch store.MovieJobChannel, tournaments []string) (int, error) {
	ids, err := j.youtube.LatestUploads(ctx, ch.ChannelID, newChannelVideos)
	if err != nil {
		return 0, err
	}
	details, err := j.youtube.Videos(ctx, ids)
	if err != nil {
		return 0, err
	}
	saved := 0
	for _, id := range ids {
		d, ok := details[id]
		if !ok {
			continue
		}
		v := store.PushedVideo{VideoID: id, ChannelID: ch.ChannelID, PublishedAt: d.PublishedAt}
		if err := j.save(ctx, v, d, ch.IncludeAll, tournaments); err != nil {
			return 0, err
		}
		saved++
	}
	return saved, nil
}

// check asks the Data API about every stored video, 50 at a time: it deletes those YouTube no longer shows,
// and classifies with their details those stored without. A failed call stops it, deleting nothing more.
func (j movieJob) check(ctx context.Context, includeAll map[string]bool, tournaments []string, r *movieJobReport) error {
	stored, err := j.store.MovieJobVideos(ctx)
	if err != nil {
		return err
	}
	for start := 0; start < len(stored); start += videosPerCall {
		batch := stored[start:min(start+videosPerCall, len(stored))]
		ids := make([]string, len(batch))
		for i, v := range batch {
			ids[i] = v.VideoID
		}
		details, err := j.youtube.Videos(ctx, ids)
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
				if err := j.save(ctx, pv, d, includeAll[v.ChannelID], tournaments); err != nil {
					return err
				}
				r.Classified++
			}
		}
	}
	return nil
}

// save stores v with its verdict and title from d, the Data API's details, read now.
func (j movieJob) save(ctx context.Context, v store.PushedVideo, d movies.VideoDetails, includeAll bool, tournaments []string) error {
	if d.Title != "" {
		v.Title = d.Title
	}
	v.ThumbnailURL = movies.Thumbnail(v.VideoID)
	verdict := movies.Classify(movies.VideoText{Title: v.Title, Description: d.Description, Tags: d.Tags}, includeAll, tournaments)
	v.TTA, v.Reason = verdict.TTA, verdict.Reason
	v.DetailsFetchedAt = &j.now
	if err := j.store.SaveMovieVideo(ctx, v, j.now); err != nil && !errors.Is(err, store.ErrNoChannel) {
		return err
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
func runMovies(now time.Time) error {
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
		store:   admin,
		youtube: movies.NewYouTube(nil, movies.DataAPI, cfg.youtubeKey),
		hub:     movies.NewHubClient(nil, movies.Hub, cfg.callback, cfg.hubSecret),
		now:     now.UTC(),
	}
	r, err := job.run(ctx)
	fmt.Printf("channels: %d, subscribed: %d, failed: %d; videos filled in: %d, removed: %d, classified: %d\n",
		r.Channels, r.Subscribed, r.Failed, r.Filled, r.Removed, r.Classified)
	return err
}
