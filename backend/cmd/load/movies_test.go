package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/movies"
	"ttacommunity/backend/internal/store"
)

const (
	jobNew  = "UC0000000000000000000001"
	jobOld  = "UC0000000000000000000002"
	jobAll  = "UC0000000000000000000003"
	jobFail = "UC0000000000000000000004"
)

var jobNow = time.Date(2026, 11, 16, 4, 17, 0, 0, time.UTC)

// fakeJobStore holds the job's channels and videos, and records what the job writes.
type fakeJobStore struct {
	channels   []store.MovieJobChannel
	videos     []store.MovieJobVideo
	saved      []store.PushedVideo
	deleted    []string
	backfilled []string
	errs       map[string]*string
}

func (f *fakeJobStore) MovieJobChannels(context.Context) ([]store.MovieJobChannel, error) {
	return f.channels, nil
}

func (f *fakeJobStore) MovieJobVideos(context.Context) ([]store.MovieJobVideo, error) {
	return f.videos, nil
}

func (f *fakeJobStore) MovieTournamentNames(context.Context) ([]string, error) {
	return []string{"Invented Open"}, nil
}

func (f *fakeJobStore) SaveMovieVideo(_ context.Context, v store.PushedVideo, _ time.Time) error {
	f.saved = append(f.saved, v)
	return nil
}

func (f *fakeJobStore) DeleteMovieVideo(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *fakeJobStore) MarkMovieChannelBackfilled(_ context.Context, id string, _ time.Time) error {
	f.backfilled = append(f.backfilled, id)
	return nil
}

func (f *fakeJobStore) SetMovieChannelError(_ context.Context, id string, msg *string, _ time.Time) error {
	if f.errs == nil {
		f.errs = map[string]*string{}
	}
	f.errs[id] = msg
	return nil
}

// fakeYouTube knows the videos in details, fails every call with err, and records each call's ids.
type fakeYouTube struct {
	details map[string]movies.VideoDetails
	err     error
	calls   [][]string
}

func (f *fakeYouTube) Videos(_ context.Context, ids []string) (map[string]movies.VideoDetails, error) {
	f.calls = append(f.calls, ids)
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]movies.VideoDetails{}
	for _, id := range ids {
		if d, ok := f.details[id]; ok {
			out[id] = d
		}
	}
	return out, nil
}

// fakeFeeds answers each channel's feed, and fails jobFail's.
type fakeFeeds struct {
	entries map[string][]movies.Entry
	asked   []string
}

func (f *fakeFeeds) FetchChannel(_ context.Context, id string) ([]movies.Entry, error) {
	f.asked = append(f.asked, id)
	if id == jobFail {
		return nil, errors.New("HTTP 500 from YouTube")
	}
	return f.entries[id], nil
}

// fakeHub accepts every subscribe but those in refuse.
type fakeHub struct {
	refuse     map[string]bool
	subscribed []string
}

func (f *fakeHub) Subscribe(_ context.Context, id string) error {
	if f.refuse[id] {
		return errors.New("the hub answered HTTP 503 to a subscribe")
	}
	f.subscribed = append(f.subscribed, id)
	return nil
}

func newJob(s *fakeJobStore, y *fakeYouTube, f *fakeFeeds, h *fakeHub) movieJob {
	return movieJob{store: s, videos: y, feeds: f, hub: h, now: jobNow}
}

func TestTheMovieJobFillsInANewChannelOnce(t *testing.T) {
	s := &fakeJobStore{channels: []store.MovieJobChannel{{ChannelID: jobNew}, {ChannelID: jobOld, BackfilledAt: &jobNow}}}
	y := &fakeYouTube{details: map[string]movies.VideoDetails{
		"vid00000001": {Title: "Game 5", Description: "Through the Ages, round 2"},
		"vid00000002": {Title: "Chess"},
	}}
	f := &fakeFeeds{entries: map[string][]movies.Entry{jobNew: {
		{VideoID: "vid00000001", Title: "Game 5", PublishedAt: jobNow.Add(-time.Hour)},
		{VideoID: "vid00000002", Title: "Chess", PublishedAt: jobNow.Add(-2 * time.Hour)},
		{VideoID: "vid00000003", Title: "Gone private", Description: "Invented Open final", PublishedAt: jobNow.Add(-3 * time.Hour)},
	}}}
	if _, err := newJob(s, y, f, &fakeHub{}).run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !slices.Equal(f.asked, []string{jobNew}) || !slices.Equal(s.backfilled, []string{jobNew}) {
		t.Errorf("feeds read %q, marked filled in %q; want only the new channel's", f.asked, s.backfilled)
	}
	verdicts := map[string]string{}
	for _, v := range s.saved {
		verdicts[v.VideoID] = fmt.Sprintf("%v %s %v", v.TTA, v.Reason, v.DetailsFetchedAt != nil)
	}
	want := map[string]string{
		"vid00000001": "true mentions Through the Ages true",
		"vid00000002": "false no mention of TTA true",
		"vid00000003": "true names the tournament Invented Open false",
	}
	for id, w := range want {
		if verdicts[id] != w {
			t.Errorf("%s saved as %q, want %q", id, verdicts[id], w)
		}
	}
}

func TestTheMovieJobRenewsEverySubscriptionAndRecordsFailures(t *testing.T) {
	failing := "an earlier failure"
	s := &fakeJobStore{
		channels: []store.MovieJobChannel{{ChannelID: jobOld, BackfilledAt: &jobNow}, {ChannelID: jobAll, BackfilledAt: &jobNow}, {ChannelID: jobFail}},
		errs:     map[string]*string{jobOld: &failing},
	}
	h := &fakeHub{refuse: map[string]bool{jobAll: true}}
	report, err := newJob(s, &fakeYouTube{}, &fakeFeeds{}, h).run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "2 of 3 channels") {
		t.Errorf("run = %v, want an error naming 2 of 3 channels failing", err)
	}
	if !slices.Equal(h.subscribed, []string{jobOld, jobFail}) {
		t.Errorf("subscribed %q, want every channel the hub accepts, a failed fill-in included", h.subscribed)
	}
	if s.errs[jobOld] != nil {
		t.Errorf("%s's error = %q after a clean run, want it cleared", jobOld, *s.errs[jobOld])
	}
	for id, want := range map[string]string{jobAll: "subscribing: the hub answered HTTP 503", jobFail: "filling in its videos: HTTP 500 from YouTube"} {
		if s.errs[id] == nil || !strings.Contains(*s.errs[id], want) {
			t.Errorf("%s's error = %v, want it to say %q", id, s.errs[id], want)
		}
	}
	if slices.Contains(s.backfilled, jobFail) {
		t.Error("a channel whose feed failed was marked filled in, want it tried again tomorrow")
	}
	if report.Subscribed != 2 || report.Failed != 2 {
		t.Errorf("report = %+v, want 2 subscribed and 2 failed", report)
	}
}

func TestTheMovieJobChecksEveryStoredVideo(t *testing.T) {
	s := &fakeJobStore{
		channels: []store.MovieJobChannel{{ChannelID: jobOld, BackfilledAt: &jobNow}, {ChannelID: jobAll, IncludeAll: true, BackfilledAt: &jobNow}},
		videos: []store.MovieJobVideo{
			{VideoID: "vid00000001", ChannelID: jobOld, Title: "Kept", DetailsFetched: true},
			{VideoID: "vid00000002", ChannelID: jobOld, Title: "Removed", DetailsFetched: true},
			{VideoID: "vid00000003", ChannelID: jobOld, Title: "Old title", DetailsFetched: false},
			{VideoID: "vid00000004", ChannelID: jobAll, Title: "Chess", DetailsFetched: false},
		},
	}
	y := &fakeYouTube{details: map[string]movies.VideoDetails{
		"vid00000001": {Title: "Kept"},
		"vid00000003": {Title: "New title", Tags: []string{"through the ages"}},
		"vid00000004": {Title: "Chess"},
	}}
	report, err := newJob(s, y, &fakeFeeds{}, &fakeHub{}).run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !slices.Equal(s.deleted, []string{"vid00000002"}) {
		t.Errorf("deleted %q, want the one YouTube no longer shows", s.deleted)
	}
	got := map[string]string{}
	for _, v := range s.saved {
		got[v.VideoID] = fmt.Sprintf("%s|%v|%s|%v", v.Title, v.TTA, v.Reason, v.DetailsFetchedAt != nil)
	}
	want := map[string]string{
		"vid00000003": "New title|true|mentions Through the Ages|true",
		"vid00000004": "Chess|true|the channel's every video counts|true",
	}
	if len(got) != len(want) {
		t.Errorf("saved %q, want only the videos without details: %q", got, want)
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s saved as %q, want %q", id, got[id], w)
		}
	}
	if report.Removed != 1 || report.Classified != 2 {
		t.Errorf("report = %+v, want 1 removed and 2 classified", report)
	}
}

func TestTheMovieJobAsksForAtMost50VideosAtOnce(t *testing.T) {
	s := &fakeJobStore{}
	details := map[string]movies.VideoDetails{}
	for i := range 120 {
		id := fmt.Sprintf("vid%08d", i)
		s.videos = append(s.videos, store.MovieJobVideo{VideoID: id, ChannelID: jobOld, DetailsFetched: true})
		details[id] = movies.VideoDetails{}
	}
	y := &fakeYouTube{details: details}
	if _, err := newJob(s, y, &fakeFeeds{}, &fakeHub{}).run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}
	var sizes []int
	for _, c := range y.calls {
		sizes = append(sizes, len(c))
	}
	if !slices.Equal(sizes, []int{50, 50, 20}) || len(s.deleted) != 0 {
		t.Errorf("calls of %v ids, deleted %d; want 50, 50, 20 and none", sizes, len(s.deleted))
	}
}

func TestTheMovieJobDeletesNothingWhenTheDataAPIFails(t *testing.T) {
	s := &fakeJobStore{videos: []store.MovieJobVideo{{VideoID: "vid00000001", ChannelID: jobOld, DetailsFetched: true}}}
	y := &fakeYouTube{err: errors.New("the Data API answered HTTP 403 (quotaExceeded)")}
	if _, err := newJob(s, y, &fakeFeeds{}, &fakeHub{}).run(context.Background()); err == nil || !strings.Contains(err.Error(), "quotaExceeded") {
		t.Errorf("run = %v, want the Data API's failure", err)
	}
	if len(s.deleted) != 0 || len(s.saved) != 0 {
		t.Errorf("deleted %q, saved %d; want nothing changed", s.deleted, len(s.saved))
	}
}

func TestTheMovieJobNeedsItsConfig(t *testing.T) {
	full := map[string]string{
		"RATINGS_ADMIN_DATABASE_URL": "postgres://invented",
		"YOUTUBE_API_KEY":            "invented-key",
		"MOVIES_HUB_SECRET":          strings.Repeat("s", 32),
		"API_ORIGIN":                 "https://api.example",
	}
	tests := []struct {
		name, key, value string
		wantErrNaming    string
	}{
		{"all set", "", "", ""},
		{"no database", "RATINGS_ADMIN_DATABASE_URL", "", "RATINGS_ADMIN_DATABASE_URL"},
		{"no key", "YOUTUBE_API_KEY", "", "YOUTUBE_API_KEY"},
		{"no secret", "MOVIES_HUB_SECRET", "", "MOVIES_HUB_SECRET"},
		{"a short secret", "MOVIES_HUB_SECRET", strings.Repeat("s", 31), "MOVIES_HUB_SECRET"},
		{"no origin", "API_ORIGIN", "", "API_ORIGIN"},
		{"an origin the hub cannot reach", "API_ORIGIN", "http://127.0.0.1:8080", "API_ORIGIN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{}
			for k, v := range full {
				env[k] = v
			}
			if tt.key != "" {
				env[tt.key] = tt.value
			}
			cfg, err := movieJobConfigFrom(func(k string) string { return env[k] })
			if (err != nil) != (tt.wantErrNaming != "") {
				t.Fatalf("movieJobConfigFrom error = %v, want one naming %q", err, tt.wantErrNaming)
			}
			if err != nil && !strings.Contains(err.Error(), tt.wantErrNaming) {
				t.Errorf("error = %v, want it to name %s", err, tt.wantErrNaming)
			}
			if err == nil && cfg.callback != "https://api.example/v1/movies/hub" {
				t.Errorf("callback = %q, want the api's hub callback", cfg.callback)
			}
			if s := fmt.Sprintf("%v %+v %#v", cfg, cfg, cfg); strings.Contains(s, "invented-key") || strings.Contains(s, strings.Repeat("s", 32)) {
				t.Errorf("the config prints a secret: %s", s)
			}
		})
	}
}
