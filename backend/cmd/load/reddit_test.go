package main

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

var redditNow = time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC)

// fakeFeeds holds the job's feeds and records what it writes.
type fakeFeeds struct {
	feeds    []store.RedditFeed
	recorded map[int64]int
	failed   map[int64]string
	listErr  error
}

func (f *fakeFeeds) RedditFeeds(context.Context) ([]store.RedditFeed, error) {
	return f.feeds, f.listErr
}

func (f *fakeFeeds) RecordRedditPosts(_ context.Context, feed int64, posts []store.RedditPost, _ time.Time) (int, error) {
	f.recorded[feed] = len(posts)
	return len(posts), nil
}

func (f *fakeFeeds) FeedFailed(_ context.Context, feed int64, msg string, _ time.Time) error {
	f.failed[feed] = msg
	return nil
}

// fakeReddit answers each subreddit with one post, or with its error.
type fakeReddit struct {
	errs  map[string]error
	asked []string
}

func (r *fakeReddit) Newest(_ context.Context, subreddit string) ([]store.RedditPost, error) {
	r.asked = append(r.asked, subreddit)
	if err := r.errs[subreddit]; err != nil {
		return nil, err
	}
	return []store.RedditPost{{PostID: "t3_abc1"}}, nil
}

func TestTheRedditJobReadsEachFeedNotPaused(t *testing.T) {
	s := &fakeFeeds{
		feeds: []store.RedditFeed{
			{ID: 1, Subreddit: "example"},
			{ID: 2, Subreddit: "quiet", Paused: true},
			{ID: 3, Subreddit: "blocked"},
		},
		recorded: map[int64]int{}, failed: map[int64]string{},
	}
	r := &fakeReddit{errs: map[string]error{"blocked": errors.New("Reddit answered 429")}}
	report, err := redditJob{store: s, reddit: r, now: redditNow}.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v, want none: a feed's failure is recorded on it", err)
	}
	if want := []string{"example", "blocked"}; !slices.Equal(r.asked, want) {
		t.Errorf("asked for %q, want %q", r.asked, want)
	}
	if s.recorded[1] != 1 || s.failed[3] != "Reddit answered 429" || len(s.recorded) != 1 || len(s.failed) != 1 {
		t.Errorf("recorded %v, failed %v; want feed 1 recorded and feed 3's 429 kept", s.recorded, s.failed)
	}
	if want := (redditJobReport{Feeds: 3, Read: 1, Failed: 1, Queued: 1}); report != want {
		t.Errorf("report = %+v, want %+v", report, want)
	}
}

func TestTheRedditJobFailsWhenItCannotReadItsFeeds(t *testing.T) {
	s := &fakeFeeds{listErr: errors.New("database down")}
	if _, err := (redditJob{store: s, reddit: &fakeReddit{}, now: redditNow}).run(context.Background()); err == nil {
		t.Error("run: no error, want the store's")
	}
}
