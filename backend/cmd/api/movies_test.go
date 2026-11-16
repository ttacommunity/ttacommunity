package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/enginejob"
	"ttacommunity/backend/internal/movies"
	"ttacommunity/backend/internal/store"
)

func TestTheMovieHubIsOffOrHasALongSecretAndAKey(t *testing.T) {
	long := strings.Repeat("s", 32)
	tests := []struct {
		name, secret, key string
		wantOn            bool
		wantErrNaming     string
	}{
		{"off", "", "", false, ""},
		{"off, with a key for admin's handles", "", "invented-key", false, ""},
		{"a secret too short to trust", long[:31], "invented-key", false, "MOVIES_HUB_SECRET"},
		{"a secret without a key", long, "", false, "YOUTUBE_API_KEY"},
		{"a long secret and a key", long, "invented-key", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{"MOVIES_HUB_SECRET": tt.secret, "YOUTUBE_API_KEY": tt.key}
			getenv := func(k string) string { return env[k] }
			hub, err := openMovieHub(getenv, nil, nil, openYouTube(getenv), slog.New(slog.DiscardHandler))
			if (err != nil) != (tt.wantErrNaming != "") {
				t.Fatalf("openMovieHub error = %v, want one naming %q", err, tt.wantErrNaming)
			}
			if err != nil && !strings.Contains(err.Error(), tt.wantErrNaming) {
				t.Errorf("openMovieHub error = %v, want it to name %s", err, tt.wantErrNaming)
			}
			if (hub != nil) != tt.wantOn {
				t.Fatalf("openMovieHub = %v, want the hub on: %v", hub, tt.wantOn)
			}
			if tt.wantOn && string(hub.Secret()) != long {
				t.Error("openMovieHub's secret is not MOVIES_HUB_SECRET")
			}
		})
	}
}

func TestOpenYouTubeNeedsAKey(t *testing.T) {
	if y := openYouTube(func(string) string { return "" }); y != nil {
		t.Errorf("openYouTube without YOUTUBE_API_KEY = %v, want nil", y)
	}
	if y := openYouTube(func(string) string { return "invented-key" }); y == nil {
		t.Error("openYouTube with YOUTUBE_API_KEY = nil, want a client")
	}
}

func TestTheMovieHubNeverPrintsItsSecret(t *testing.T) {
	secret := strings.Repeat("s", 32)
	getenv := func(string) string { return secret }
	hub, err := openMovieHub(getenv, nil, nil, openYouTube(getenv), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("openMovieHub: %v", err)
	}
	for _, verb := range []string{"%v", "%+v", "%s", "%#v"} {
		if got := fmt.Sprintf(verb, hub); strings.Contains(got, secret) {
			t.Errorf("Sprintf(%q) = %q, shows the secret", verb, got)
		}
	}
}

// stubClassifying links UC…01 and names one tournament.
type stubClassifying struct{}

func (stubClassifying) MovieClassifying(_ context.Context, channelID string) (bool, []string, error) {
	if channelID != "UC0000000000000000000001" {
		return false, nil, store.ErrNoChannel
	}
	return false, []string{"Invented Open"}, nil
}

// stubVideos answers with details, or fails with err, and counts the calls.
type stubVideos struct {
	details map[string]movies.VideoDetails
	err     error
	calls   int
}

func (s *stubVideos) Videos(_ context.Context, ids []string) (map[string]movies.VideoDetails, error) {
	s.calls++
	return s.details, s.err
}

func TestClassifyPushedReadsTheDescriptionAndTags(t *testing.T) {
	at := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	pushed := movies.PushedEntry{VideoID: "vid00000001", ChannelID: "UC0000000000000000000001", Title: "Game 5"}
	tests := []struct {
		name        string
		videos      *stubVideos
		wantTTA     bool
		wantReason  string
		wantFetched bool
	}{
		{"the description names the game", &stubVideos{details: map[string]movies.VideoDetails{
			"vid00000001": {Title: "Game 5", Description: "Through the Ages, round 2"}}}, true, "mentions Through the Ages", true},
		{"a tag names a tournament", &stubVideos{details: map[string]movies.VideoDetails{
			"vid00000001": {Title: "Game 5", Tags: []string{"invented open"}}}}, true, "names the tournament Invented Open", true},
		{"nothing names the game", &stubVideos{details: map[string]movies.VideoDetails{
			"vid00000001": {Title: "Game 5", Description: "Chess"}}}, false, "no mention of TTA", true},
		{"the Data API fails", &stubVideos{err: errors.New("HTTP 403")}, false, "no mention of TTA", false},
		{"YouTube no longer shows the video", &stubVideos{details: map[string]movies.VideoDetails{}}, false, "no mention of TTA", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &movieHubDeps{classifying: stubClassifying{}, videos: tt.videos, log: slog.New(slog.DiscardHandler)}
			v, err := d.ClassifyPushed(context.Background(), pushed, at)
			if err != nil {
				t.Fatalf("ClassifyPushed: %v", err)
			}
			if v.TTA != tt.wantTTA || v.Reason != tt.wantReason {
				t.Errorf("verdict = %v %q, want %v %q", v.TTA, v.Reason, tt.wantTTA, tt.wantReason)
			}
			if (v.DetailsFetchedAt != nil) != tt.wantFetched || (tt.wantFetched && !v.DetailsFetchedAt.Equal(at)) {
				t.Errorf("DetailsFetchedAt = %v, want set to %v: %v", v.DetailsFetchedAt, at, tt.wantFetched)
			}
			if v.Title != "Game 5" || v.ThumbnailURL != movies.Thumbnail("vid00000001") {
				t.Errorf("got title %q, thumbnail %q; want the pushed title and the video's thumbnail", v.Title, v.ThumbnailURL)
			}
		})
	}
}

func TestClassifyPushedAsksNothingForAnUnlinkedChannel(t *testing.T) {
	videos := &stubVideos{}
	d := &movieHubDeps{classifying: stubClassifying{}, videos: videos, log: slog.New(slog.DiscardHandler)}
	v, err := d.ClassifyPushed(context.Background(), movies.PushedEntry{VideoID: "vid00000002", ChannelID: "UC0000000000000000000002"}, time.Time{})
	if err != nil {
		t.Fatalf("ClassifyPushed: %v", err)
	}
	if videos.calls != 0 || v.TTA {
		t.Errorf("asked the Data API %d times, verdict %v; want no call and no verdict", videos.calls, v.TTA)
	}
}

// fakeMovieStarter records the args each Movies Job starts with.
type fakeMovieStarter struct{ started [][]string }

func (f *fakeMovieStarter) StartWith(_ context.Context, args ...string) (string, error) {
	f.started = append(f.started, args)
	return "movies-admin-x1y2z", nil
}

// linkedChannels links only UC0000000000000000000001.
type linkedChannels struct{}

func (linkedChannels) MovieJobChannel(_ context.Context, id string) (store.MovieJobChannel, error) {
	if id != "UC0000000000000000000001" {
		return store.MovieJobChannel{}, store.ErrNoChannel
	}
	return store.MovieJobChannel{ChannelID: id}, nil
}

func TestStartMovieJobPassesTheChannel(t *testing.T) {
	tests := []struct {
		name, channel string
		wantArgs      []string
		wantErr       error
	}{
		{"every channel", "", []string{}, nil},
		{"one channel", "UC0000000000000000000001", []string{"-channel", "UC0000000000000000000001"}, nil},
		{"a channel not linked", "UC0000000000000000000009", nil, store.ErrNoChannel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeMovieStarter{}
			_, err := startMovieJob(context.Background(), linkedChannels{}, f, tt.channel)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("startMovieJob error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantArgs == nil {
				if len(f.started) != 0 {
					t.Errorf("started %q, want no Job", f.started)
				}
				return
			}
			if len(f.started) != 1 || !slices.Equal(f.started[0], tt.wantArgs) {
				t.Errorf("started %q, want one Job with args %q", f.started, tt.wantArgs)
			}
		})
	}
}

func TestStartMovieJobWithoutACronJobIsNotSetUp(t *testing.T) {
	if _, err := (adminDeps{}).StartMovieJob(context.Background(), ""); !errors.Is(err, enginejob.ErrNotSetUp) {
		t.Errorf("StartMovieJob error = %v, want %v", err, enginejob.ErrNotSetUp)
	}
	if _, err := (adminDeps{}).MovieJobGoing(context.Background()); !errors.Is(err, enginejob.ErrNotSetUp) {
		t.Errorf("MovieJobGoing error = %v, want %v", err, enginejob.ErrNotSetUp)
	}
}
