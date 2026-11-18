//go:build integration

package store

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// twitchAt is when the tests' streams go live.
var twitchAt = time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)

// twitchFeed adds an invented streamer and returns the feed's id.
func twitchFeed(t *testing.T, a *AdminStore, login, userID string) int64 {
	t.Helper()
	id, err := a.AddTwitchFeed(context.Background(), login, userID, strings.ToUpper(login[:1])+login[1:], twitchAt)
	if err != nil {
		t.Fatalf("AddTwitchFeed(%s): %v", login, err)
	}
	return id
}

// subscribed adds an endpoint named name taking feed with opts, and returns its id.
func subscribed(t *testing.T, a *AdminStore, name string, n int, opts FeedOptions) int64 {
	t.Helper()
	id, err := a.AddNotificationEndpoint(context.Background(), NotificationEndpoint{Name: name, URL: webhook(n), Format: "discord",
		Feeds: []int64{opts.Feed}, FeedOptions: []FeedOptions{opts}}, twitchAt)
	if err != nil {
		t.Fatalf("AddNotificationEndpoint(%s): %v", name, err)
	}
	return id
}

// stream is an invented stream of the streamer with user id 100000001.
func stream(id, title, category string) TwitchStream {
	return TwitchStream{StreamID: id, UserID: "100000001", Title: title, Category: category,
		ThumbnailURL: "https://static-cdn.jtvnw.net/previews-ttv/live_user_streamer-1280x720.jpg", StartedAt: twitchAt}
}

// twitchSent sends what is due at now and returns each post as "endpoint kind title".
func twitchSent(t *testing.T, a *AdminStore, now time.Time) []string {
	t.Helper()
	var got []string
	if _, err := a.SendNotifications(context.Background(), now, 50, func(_ context.Context, n Notification, e NotificationEndpoint) error {
		got = append(got, e.Name+" "+n.Kind+" "+n.Twitch.Title)
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	slices.Sort(got)
	return got
}

func TestAdminKeepsTwitchFeeds(t *testing.T) {
	a := redditStore(t)
	ctx := context.Background()
	id := twitchFeed(t, a, "some_streamer", "100000001")
	if err := a.SetTwitchSubscription(ctx, id, "stream.online", "sub-1", "webhook_callback_verification_pending", twitchAt); err != nil {
		t.Fatalf("SetTwitchSubscription: %v", err)
	}
	if err := a.TwitchSubscriptionStatus(ctx, "sub-1", "enabled", twitchAt); err != nil {
		t.Fatalf("TwitchSubscriptionStatus: %v", err)
	}
	feeds, err := a.TwitchFeeds(ctx)
	if err != nil || len(feeds) != 1 {
		t.Fatalf("TwitchFeeds = %+v, %v; want the one feed", feeds, err)
	}
	f := feeds[0]
	if f.Login != "some_streamer" || f.UserID != "100000001" || len(f.Subscriptions) != 1 || f.Subscriptions[0].Status != "enabled" {
		t.Errorf("the feed = %+v, want some_streamer with its subscription enabled", f)
	}
	if reddit, err := a.RedditFeeds(ctx); err != nil || len(reddit) != 0 {
		t.Errorf("RedditFeeds = %+v, %v; want no Twitch feed among them", reddit, err)
	}
	if err := a.RemoveTwitchFeed(ctx, id); err != nil {
		t.Fatalf("RemoveTwitchFeed: %v", err)
	}
	if err := a.RemoveTwitchFeed(ctx, id); !errors.Is(err, ErrNoTwitchFeed) {
		t.Errorf("RemoveTwitchFeed again: %v, want ErrNoTwitchFeed", err)
	}
}

func TestATwitchFeedIsRefusedWhenItShouldBe(t *testing.T) {
	tests := []struct {
		name, login, userID string
		want                error
	}{
		{"a login with a space", "some streamer", "100000002", ErrBadTwitchFeed},
		{"a login too short", "ab", "100000002", ErrBadTwitchFeed},
		{"a user id that is not a number", "other_streamer", "abc", ErrBadTwitchFeed},
		{"a streamer already there, in other letters", "Some_Streamer", "100000002", ErrTwitchFeedExists},
		{"a user id already there", "renamed_streamer", "100000001", ErrTwitchFeedExists},
	}
	a := redditStore(t)
	twitchFeed(t, a, "some_streamer", "100000001")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := a.AddTwitchFeed(context.Background(), tt.login, tt.userID, "", twitchAt); !errors.Is(err, tt.want) {
				t.Errorf("AddTwitchFeed(%q, %q): %v, want %v", tt.login, tt.userID, err, tt.want)
			}
		})
	}
}

func TestAStreamGoingLiveIsPostedToTheEndpointsWhoseOptionsItMatches(t *testing.T) {
	tests := []struct {
		name   string
		stream TwitchStream
		want   []string
	}{
		{"in the category", stream("1", "Friday games", "Through the Ages"), []string{"Any", "Category"}},
		{"in another category, with a keyword", stream("2", "A TTA tournament", "Board Games"), []string{"Any", "Keyword"}},
		{"keyword in other letters", stream("3", "TOURNAMENT night", "Chess"), []string{"Any", "Keyword"}},
		{"matching neither", stream("4", "Just chatting", "Just Chatting"), []string{"Any"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := redditStore(t)
			feed := twitchFeed(t, a, "some_streamer", "100000001")
			subscribed(t, a, "Any", 1, FeedOptions{Feed: feed})
			subscribed(t, a, "Category", 2, FeedOptions{Feed: feed, Categories: []string{"through the ages"}})
			subscribed(t, a, "Keyword", 3, FeedOptions{Feed: feed, Keywords: []string{"tournament"}})
			endpoint(t, a, "Not subscribed", 4, "sign_up")
			if err := a.TwitchStreamOnline(context.Background(), "msg-1", tt.stream, twitchAt); err != nil {
				t.Fatalf("TwitchStreamOnline: %v", err)
			}
			var got []string
			for _, line := range twitchSent(t, a, twitchAt) {
				got = append(got, strings.Fields(line)[0])
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("posted to %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAStreamIsPostedOnce(t *testing.T) {
	tests := []struct {
		name            string
		second          string
		secondStreamID  string
		wantSecondPosts int
	}{
		{"Twitch sending the same message again", "msg-1", "1", 0},
		{"another message for the same stream", "msg-2", "1", 0},
		{"a new stream", "msg-3", "2", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := redditStore(t)
			ctx := context.Background()
			subscribed(t, a, "Any", 1, FeedOptions{Feed: twitchFeed(t, a, "some_streamer", "100000001")})
			if err := a.TwitchStreamOnline(ctx, "msg-1", stream("1", "Friday games", ""), twitchAt); err != nil {
				t.Fatalf("TwitchStreamOnline: %v", err)
			}
			twitchSent(t, a, twitchAt)
			if err := a.TwitchStreamOnline(ctx, tt.second, stream(tt.secondStreamID, "Friday games", ""), twitchAt); err != nil {
				t.Fatalf("TwitchStreamOnline again: %v", err)
			}
			if got := twitchSent(t, a, twitchAt); len(got) != tt.wantSecondPosts {
				t.Errorf("second time posted %q, want %d posts", got, tt.wantSecondPosts)
			}
		})
	}
}

func TestAStreamOfNoFeedOrAPausedFeedIsNotPosted(t *testing.T) {
	a := redditStore(t)
	ctx := context.Background()
	feed := twitchFeed(t, a, "some_streamer", "100000001")
	subscribed(t, a, "Any", 1, FeedOptions{Feed: feed})
	unknown := stream("1", "Friday games", "")
	unknown.UserID = "100000009"
	if err := a.TwitchStreamOnline(ctx, "msg-1", unknown, twitchAt); err != nil {
		t.Fatalf("TwitchStreamOnline of a streamer not listed: %v", err)
	}
	if err := a.SetTwitchFeedPaused(ctx, feed, true); err != nil {
		t.Fatalf("SetTwitchFeedPaused: %v", err)
	}
	if err := a.TwitchStreamOnline(ctx, "msg-2", stream("2", "Friday games", ""), twitchAt); err != nil {
		t.Fatalf("TwitchStreamOnline of a paused feed: %v", err)
	}
	if got := twitchSent(t, a, twitchAt); len(got) != 0 {
		t.Errorf("posted %q, want nothing", got)
	}
}

func TestAStreamEndingEditsOnlyThePostsKept(t *testing.T) {
	a := redditStore(t)
	ctx := context.Background()
	feed := twitchFeed(t, a, "some_streamer", "100000001")
	kept := subscribed(t, a, "Was live", 1, FeedOptions{Feed: feed, WasLive: true})
	subscribed(t, a, "Left", 2, FeedOptions{Feed: feed})
	if err := a.TwitchStreamOnline(ctx, "msg-1", stream("1", "Friday games", ""), twitchAt); err != nil {
		t.Fatalf("TwitchStreamOnline: %v", err)
	}
	if _, err := a.SendNotifications(ctx, twitchAt, 50, func(_ context.Context, n Notification, e NotificationEndpoint) error {
		if e.ID == kept {
			return a.RecordTwitchPost(ctx, n.StreamRef, e.ID, TwitchPost{MessageID: "100000000000000011", ChannelID: "100000000000000012"})
		}
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	if err := a.TwitchStreamOffline(ctx, "msg-2", "100000001", twitchAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("TwitchStreamOffline: %v", err)
	}
	var got []string
	if _, err := a.SendNotifications(ctx, twitchAt.Add(2*time.Hour), 50, func(_ context.Context, n Notification, e NotificationEndpoint) error {
		got = append(got, e.Name+" "+n.Kind+" "+n.Twitch.Posts[e.ID].MessageID+" "+n.Twitch.EndedAt.Sub(n.Twitch.StartedAt).String())
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	if want := []string{"Was live twitch_ended 100000000000000011 2h0m0s"}; !slices.Equal(got, want) {
		t.Errorf("edited %q, want %q", got, want)
	}
}

func TestAnEndpointKeepsItsFeedOptions(t *testing.T) {
	a := redditStore(t)
	ctx := context.Background()
	feed := twitchFeed(t, a, "some_streamer", "100000001")
	want := FeedOptions{Feed: feed, Categories: []string{"Through the Ages"}, Keywords: []string{"tta", "tournament"}, WasLive: true}
	id := subscribed(t, a, "Twitch", 1, want)
	e, err := a.NotificationEndpoint(ctx, id)
	if err != nil {
		t.Fatalf("NotificationEndpoint: %v", err)
	}
	if len(e.FeedOptions) != 1 || !e.FeedOptions[0].WasLive || !slices.Equal(e.FeedOptions[0].Categories, want.Categories) ||
		!slices.Equal(e.FeedOptions[0].Keywords, want.Keywords) || !slices.Equal(e.Feeds, []int64{feed}) {
		t.Errorf("the endpoint's feeds = %v %+v, want %+v", e.Feeds, e.FeedOptions, want)
	}
}
