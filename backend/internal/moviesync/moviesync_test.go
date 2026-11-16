package moviesync

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"ttacommunity/backend/internal/movies"
	"ttacommunity/backend/internal/store"
)

const channel = "UC0000000000000000000001"

var now = time.Date(2026, 11, 16, 4, 17, 0, 0, time.UTC)

// fakeStore records what setting a channel up writes.
type fakeStore struct {
	saved      []string
	backfilled []string
	errs       map[string]*string
}

func (f *fakeStore) SaveMovieVideo(_ context.Context, v store.PushedVideo, _ time.Time) error {
	f.saved = append(f.saved, v.VideoID)
	return nil
}

func (f *fakeStore) MarkMovieChannelBackfilled(_ context.Context, id string, _ time.Time) error {
	f.backfilled = append(f.backfilled, id)
	return nil
}

func (f *fakeStore) SetMovieChannelError(_ context.Context, id string, msg *string, _ time.Time) error {
	if f.errs == nil {
		f.errs = map[string]*string{}
	}
	f.errs[id] = msg
	return nil
}

// fakeYouTube shows uploads for every channel and knows every video, recording each videos call's size.
type fakeYouTube struct {
	uploads []string
	calls   []int
}

func (f *fakeYouTube) Uploads(context.Context, string) ([]string, error) { return f.uploads, nil }

func (f *fakeYouTube) Videos(_ context.Context, ids []string) (map[string]movies.VideoDetails, error) {
	f.calls = append(f.calls, len(ids))
	out := map[string]movies.VideoDetails{}
	for _, id := range ids {
		out[id] = movies.VideoDetails{Title: "Through the Ages " + id, PublishedAt: now}
	}
	return out, nil
}

type fakeHub struct{ subscribed []string }

func (f *fakeHub) Subscribe(_ context.Context, id string) error {
	f.subscribed = append(f.subscribed, id)
	return nil
}

func TestChannelFillsInEveryUploadFiftyAtATime(t *testing.T) {
	y := &fakeYouTube{}
	for i := range 120 {
		y.uploads = append(y.uploads, fmt.Sprintf("vid%08d", i))
	}
	s, h := &fakeStore{}, &fakeHub{}
	r, err := Syncer{Store: s, YouTube: y, Hub: h}.Channel(context.Background(), store.MovieJobChannel{ChannelID: channel}, nil, now)
	if err != nil {
		t.Fatalf("Channel: %v", err)
	}
	if !slices.Equal(y.calls, []int{50, 50, 20}) {
		t.Errorf("videos calls of %v ids, want 50, 50 and 20", y.calls)
	}
	if r.Filled != 120 || len(s.saved) != 120 || !slices.Equal(s.backfilled, []string{channel}) {
		t.Errorf("filled %d, saved %d, marked filled in %q; want all 120 and the channel marked", r.Filled, len(s.saved), s.backfilled)
	}
	if !r.Subscribed || !slices.Equal(h.subscribed, []string{channel}) || r.Problem != "" {
		t.Errorf("result %+v, subscribed %q; want subscribed with no problem", r, h.subscribed)
	}
}

func TestChannelWithoutAHubOnlyFillsIn(t *testing.T) {
	s := &fakeStore{}
	y := &fakeYouTube{uploads: []string{"vid00000001"}}
	r, err := Syncer{Store: s, YouTube: y}.Channel(context.Background(), store.MovieJobChannel{ChannelID: channel}, nil, now)
	if err != nil {
		t.Fatalf("Channel: %v", err)
	}
	if r.Subscribed || r.Problem != "" || r.Filled != 1 {
		t.Errorf("result %+v, want one filled in, not subscribed, and no problem", r)
	}
	if msg, ok := s.errs[channel]; !ok || msg != nil {
		t.Errorf("the channel's error = %v (recorded: %v), want it cleared", msg, ok)
	}
}

func TestChannelReadsAFilledInChannelAgain(t *testing.T) {
	s, h := &fakeStore{}, &fakeHub{}
	y := &fakeYouTube{uploads: []string{"vid00000001", "vid00000002"}}
	done := now.Add(-24 * time.Hour)
	r, err := Syncer{Store: s, YouTube: y, Hub: h}.Channel(context.Background(), store.MovieJobChannel{ChannelID: channel, BackfilledAt: &done}, nil, now)
	if err != nil || r.Filled != 2 || !slices.Equal(s.saved, []string{"vid00000001", "vid00000002"}) || !r.Subscribed {
		t.Errorf("Channel = %+v, %v, saved %q; want both uploads read again and the channel subscribed", r, err, s.saved)
	}
}
