package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"ttacommunity/backend/internal/movies"
	"ttacommunity/backend/internal/store"
)

// stubStreams lists due and records each check it is given, by video.
type stubStreams struct {
	due     []store.YouTubeStream
	checked map[int64]store.YouTubeCheck
}

func (s *stubStreams) DueYouTubeStreams(_ context.Context, _ time.Time, _ int) ([]store.YouTubeStream, error) {
	return s.due, nil
}

func (s *stubStreams) RecordYouTubeCheck(_ context.Context, ref int64, c store.YouTubeCheck, _ time.Time) error {
	if s.checked == nil {
		s.checked = map[int64]store.YouTubeCheck{}
	}
	s.checked[ref] = c
	return nil
}

func TestCheckYouTubeStreams(t *testing.T) {
	now := time.Date(2026, 11, 14, 14, 0, 0, 0, time.UTC)
	started := now.Add(-10 * time.Minute)
	streams := &stubStreams{due: []store.YouTubeStream{
		{Ref: 1, VideoID: "vid00000001", RecordedAt: now.Add(-time.Hour)},
		{Ref: 2, VideoID: "vid00000002", RecordedAt: now.Add(-time.Hour)},
	}}
	videos := &stubVideos{details: map[string]movies.VideoDetails{
		"vid00000001": {Title: "Live final", Broadcast: movies.Broadcast{Content: "live", StartedAt: &started}},
	}}
	checkYouTubeStreams(context.Background(), streams, videos, now, slog.New(slog.DiscardHandler))
	if videos.calls != 1 {
		t.Errorf("Data API calls = %d, want 1 for both", videos.calls)
	}
	live := streams.checked[1]
	if live.Gone || live.Title != "Live final" || live.StartedAt == nil || !live.StartedAt.Equal(started) ||
		live.NextCheckAt == nil || !live.NextCheckAt.Equal(*movies.NextCheck(movies.Broadcast{Content: "live", StartedAt: &started}, now, now)) {
		t.Errorf("vid00000001's check = %+v, want live since %v, checked again as NextCheck says", live, started)
	}
	if gone := streams.checked[2]; !gone.Gone {
		t.Errorf("vid00000002's check = %+v, want gone: YouTube no longer shows it", gone)
	}
}

func TestCheckYouTubeStreamsAsksNothingWhenNothingIsDue(t *testing.T) {
	videos := &stubVideos{}
	checkYouTubeStreams(context.Background(), &stubStreams{}, videos, time.Now(), slog.New(slog.DiscardHandler))
	if videos.calls != 0 {
		t.Errorf("Data API calls = %d, want none", videos.calls)
	}
}

func TestCheckYouTubeStreamsRecordsNothingWhenTheDataAPIFails(t *testing.T) {
	streams := &stubStreams{due: []store.YouTubeStream{{Ref: 1, VideoID: "vid00000001"}}}
	checkYouTubeStreams(context.Background(), streams, &stubVideos{err: errors.New("HTTP 403 (quotaExceeded)")}, time.Now(), slog.New(slog.DiscardHandler))
	if len(streams.checked) != 0 {
		t.Errorf("checked = %+v, want nothing, so each is due again at the next pass", streams.checked)
	}
}
