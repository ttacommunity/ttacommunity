//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
)

// redditAt is when the tests' feeds are read.
var redditAt = time.Date(2026, 11, 17, 1, 0, 0, 0, time.UTC)

// allKinds are every kind of post, as a new feed sends.
var allKinds = []string{"text", "image", "video", "link"}

// redditStore is a fresh database's admin store, on the admin login as the job and the api use it.
func redditStore(t *testing.T) *AdminStore {
	t.Helper()
	return NewAdminStore(asAdminLogin(t, migrated(t)))
}

// addFeed adds a feed for subreddit and returns its id.
func addFeed(t *testing.T, a *AdminStore, subreddit string) int64 {
	t.Helper()
	id, err := a.AddRedditFeed(context.Background(), subreddit, redditAt)
	if err != nil {
		t.Fatalf("AddRedditFeed(%s): %v", subreddit, err)
	}
	return id
}

// post is an invented text post numbered n.
func post(n int) RedditPost {
	return RedditPost{
		PostID: fmt.Sprintf("t3_abc%d", n), Kind: "text", Title: fmt.Sprintf("Post %d", n),
		URL: fmt.Sprintf("https://www.reddit.com/r/example/comments/abc%d/post/", n), Author: "example_user",
		Body: "Hello", PublishedAt: redditAt.Add(-time.Duration(n) * time.Minute),
	}
}

// record records posts for feed at now and returns how many it queued.
func record(t *testing.T, a *AdminStore, feed int64, now time.Time, posts ...RedditPost) int {
	t.Helper()
	queued, err := a.RecordRedditPosts(context.Background(), feed, posts, now)
	if err != nil {
		t.Fatalf("RecordRedditPosts: %v", err)
	}
	return queued
}

// sentPosts sends the outbox at now and returns each post sent, as "endpoint:title".
func sentPosts(t *testing.T, a *AdminStore, now time.Time) []string {
	t.Helper()
	var got []string
	if _, err := a.SendNotifications(context.Background(), now, 10, func(_ context.Context, n Notification, e NotificationEndpoint) error {
		got = append(got, fmt.Sprintf("%s:%s", e.Name, n.Reddit.Post.Title))
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	return got
}

func TestAFeedsFirstReadPostsNothingAndLaterOnesPostWhatIsNew(t *testing.T) {
	a := redditStore(t)
	feed := addFeed(t, a, "example")
	if _, err := a.AddNotificationEndpoint(context.Background(), NotificationEndpoint{Name: "Reddit", URL: webhook(1), Format: "discord", Feeds: []int64{feed}}, redditAt); err != nil {
		t.Fatalf("AddNotificationEndpoint: %v", err)
	}
	if got := record(t, a, feed, redditAt, post(1), post(2)); got != 0 {
		t.Errorf("first read queued %d, want 0: a new feed posts no backlog", got)
	}
	if got := record(t, a, feed, redditAt.Add(10*time.Minute), post(3), post(1), post(2)); got != 1 {
		t.Errorf("second read queued %d, want 1 (post 3 only)", got)
	}
	if got := record(t, a, feed, redditAt.Add(20*time.Minute), post(3), post(1)); got != 0 {
		t.Errorf("a read with nothing new queued %d, want 0", got)
	}
	if got, want := sentPosts(t, a, redditAt.Add(30*time.Minute)), []string{"Reddit:Post 3"}; !slices.Equal(got, want) {
		t.Errorf("sent = %q, want %q", got, want)
	}
}

func TestARedditPostGoesOnlyToEndpointsTakingItsFeed(t *testing.T) {
	ctx := context.Background()
	a := redditStore(t)
	ours, other := addFeed(t, a, "example"), addFeed(t, a, "elsewhere")
	for i, e := range []NotificationEndpoint{
		{Name: "Ours", Feeds: []int64{ours}},
		{Name: "Both", Feeds: []int64{other, ours}},
		{Name: "Other", Feeds: []int64{other}},
		{Name: "Site events", Events: []string{"sign_up", "link_request", "blitz_report", "blitz_flag"}},
	} {
		e.URL, e.Format = webhook(i+1), "discord"
		if _, err := a.AddNotificationEndpoint(ctx, e, redditAt); err != nil {
			t.Fatalf("AddNotificationEndpoint(%s): %v", e.Name, err)
		}
	}
	record(t, a, ours, redditAt)
	record(t, a, ours, redditAt.Add(time.Minute), post(1))
	got := sentPosts(t, a, redditAt.Add(2*time.Minute))
	slices.Sort(got)
	if want := []string{"Both:Post 1", "Ours:Post 1"}; !slices.Equal(got, want) {
		t.Errorf("sent = %q, want %q", got, want)
	}
}

func TestARedditNotificationCarriesThePost(t *testing.T) {
	ctx := context.Background()
	a := redditStore(t)
	feed := addFeed(t, a, "Example")
	if _, err := a.AddNotificationEndpoint(ctx, NotificationEndpoint{Name: "Reddit", URL: webhook(1), Format: "discord", Feeds: []int64{feed}}, redditAt); err != nil {
		t.Fatalf("AddNotificationEndpoint: %v", err)
	}
	record(t, a, feed, redditAt)
	p := post(1)
	p.Kind, p.ImageURL = "image", "https://preview.redd.it/example.jpeg"
	record(t, a, feed, redditAt.Add(time.Minute), p)
	var got Notification
	if _, err := a.SendNotifications(ctx, redditAt.Add(time.Minute), 10, func(_ context.Context, n Notification, _ NotificationEndpoint) error {
		got = n
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	want := RedditNotice{Subreddit: "Example", Post: p}
	want.Post.PublishedAt = want.Post.PublishedAt.UTC()
	got.Reddit.Post.PublishedAt = got.Reddit.Post.PublishedAt.UTC()
	if got.Kind != "reddit_post" || got.Reddit.Subreddit != want.Subreddit || got.Reddit.Post != want.Post {
		t.Errorf("notification = %s %+v, want reddit_post %+v", got.Kind, got.Reddit, want)
	}
}

func TestAResumedFeedPostsNothingMissedWhilePaused(t *testing.T) {
	ctx := context.Background()
	a := redditStore(t)
	feed := addFeed(t, a, "example")
	record(t, a, feed, redditAt, post(1))
	for _, paused := range []bool{true, false} {
		if err := a.SetRedditFeed(ctx, feed, paused, allKinds); err != nil {
			t.Fatalf("SetRedditFeed(%t): %v", paused, err)
		}
	}
	if got := record(t, a, feed, redditAt.Add(time.Hour), post(2), post(1)); got != 0 {
		t.Errorf("the first read after resuming queued %d, want 0", got)
	}
	if got := record(t, a, feed, redditAt.Add(2*time.Hour), post(3), post(2)); got != 1 {
		t.Errorf("the next read queued %d, want 1", got)
	}
}

func TestRedditFeedsShowTheirState(t *testing.T) {
	ctx := context.Background()
	a := redditStore(t)
	feed := addFeed(t, a, "example")
	record(t, a, feed, redditAt, post(2), post(1))
	if err := a.RedditFeedFailed(ctx, feed, "Reddit answered 429", redditAt.Add(time.Hour)); err != nil {
		t.Fatalf("RedditFeedFailed: %v", err)
	}
	list, err := a.RedditFeeds(ctx)
	if err != nil {
		t.Fatalf("RedditFeeds: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("RedditFeeds = %+v, want one feed", list)
	}
	f := list[0]
	if f.CheckedAt == nil || !f.CheckedAt.Equal(redditAt) || f.LastPostAt == nil || !f.LastPostAt.Equal(post(1).PublishedAt) ||
		f.LastError != "Reddit answered 429" || f.LastErrorAt == nil || !f.Primed || f.Paused {
		t.Errorf("feed = %+v, want checked at %v, last post at %v, the 429 kept, primed and not paused", f, redditAt, post(1).PublishedAt)
	}
	record(t, a, feed, redditAt.Add(2*time.Hour))
	if list, _ = a.RedditFeeds(ctx); list[0].LastError != "" || list[0].LastErrorAt != nil {
		t.Errorf("after a good read: last error = %q at %v, want none", list[0].LastError, list[0].LastErrorAt)
	}
}

func TestRedditFeedsRefuseWhatTheDatabaseRefuses(t *testing.T) {
	ctx := context.Background()
	a := redditStore(t)
	addFeed(t, a, "example")
	for _, tt := range []struct {
		name, subreddit string
		want            error
	}{
		{"a name with a slash", "r/example", ErrBadRedditFeed},
		{"too short", "ab", ErrBadRedditFeed},
		{"too long", "abcdefghijklmnopqrstuv", ErrBadRedditFeed},
		{"one already there, in other letters", "Example", ErrRedditFeedExists},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := a.AddRedditFeed(ctx, tt.subreddit, redditAt); !errors.Is(err, tt.want) {
				t.Errorf("AddRedditFeed(%q): err = %v, want %v", tt.subreddit, err, tt.want)
			}
		})
	}
	for name, err := range map[string]error{
		"pausing":  a.SetRedditFeed(ctx, 999, true, allKinds),
		"removing": a.RemoveRedditFeed(ctx, 999),
	} {
		if !errors.Is(err, ErrNoRedditFeed) {
			t.Errorf("%s an unknown feed: err = %v, want %v", name, err, ErrNoRedditFeed)
		}
	}
}

func TestAnEndpointKeepsItsFeedsAndRefusesAnUnknownOne(t *testing.T) {
	ctx := context.Background()
	a := redditStore(t)
	one, two := addFeed(t, a, "example"), addFeed(t, a, "elsewhere")
	id, err := a.AddNotificationEndpoint(ctx, NotificationEndpoint{Name: "Reddit", URL: webhook(1), Format: "discord", Feeds: []int64{two, one, two}}, redditAt)
	if err != nil {
		t.Fatalf("AddNotificationEndpoint: %v", err)
	}
	if e, _ := a.NotificationEndpoint(ctx, id); !slices.Equal(e.Feeds, []int64{one, two}) {
		t.Errorf("feeds = %v, want %v", e.Feeds, []int64{one, two})
	}
	if err := a.SetNotificationEndpoint(ctx, NotificationEndpoint{ID: id, Name: "Reddit", URL: webhook(1), Format: "discord", Feeds: []int64{two}}); err != nil {
		t.Fatalf("SetNotificationEndpoint: %v", err)
	}
	if e, _ := a.NotificationEndpoint(ctx, id); !slices.Equal(e.Feeds, []int64{two}) {
		t.Errorf("feeds after a change = %v, want %v", e.Feeds, []int64{two})
	}
	if err := a.SetNotificationEndpoint(ctx, NotificationEndpoint{ID: id, Name: "Reddit", URL: webhook(1), Format: "discord", Feeds: []int64{999}}); !errors.Is(err, ErrBadNotificationEndpoint) {
		t.Errorf("an unknown feed: err = %v, want %v", err, ErrBadNotificationEndpoint)
	}
	if err := a.RemoveRedditFeed(ctx, two); err != nil {
		t.Fatalf("RemoveRedditFeed: %v", err)
	}
	if e, _ := a.NotificationEndpoint(ctx, id); len(e.Feeds) != 0 {
		t.Errorf("feeds after the feed was removed = %v, want none", e.Feeds)
	}
}

func TestTheRedditMigrationGoesDownAndUp(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	feed := addFeed(t, a, "example")
	if _, err := a.AddNotificationEndpoint(ctx, NotificationEndpoint{Name: "Reddit", URL: webhook(1), Format: "discord", Feeds: []int64{feed}}, redditAt); err != nil {
		t.Fatalf("AddNotificationEndpoint: %v", err)
	}
	record(t, a, feed, redditAt)
	record(t, a, feed, redditAt.Add(time.Minute), post(1))
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 85); return err }); err != nil {
		t.Fatalf("migrating down past 86: %v", err)
	}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.Up(ctx); return err }); err != nil {
		t.Fatalf("migrating up again: %v", err)
	}
	b := NewAdminStore(asAdminLogin(t, pool))
	if list, err := b.RedditFeeds(ctx); err != nil || len(list) != 0 {
		t.Errorf("RedditFeeds after down and up = %v, %v; want none", list, err)
	}
}

// sentKinds sends the outbox at now and returns each notification sent, as "endpoint:kind r/subreddit".
func sentKinds(t *testing.T, a *AdminStore, now time.Time) []string {
	t.Helper()
	var got []string
	if _, err := a.SendNotifications(context.Background(), now, 10, func(_ context.Context, n Notification, e NotificationEndpoint) error {
		got = append(got, fmt.Sprintf("%s:%s r/%s", e.Name, n.Kind, n.Reddit.Subreddit))
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	return got
}

func TestAFeedFailingThreeReadsInARowIsNotifiedOnceAndSoIsItsRecovery(t *testing.T) {
	ctx := context.Background()
	a := redditStore(t)
	feed := addFeed(t, a, "example")
	for i, e := range []NotificationEndpoint{{Name: "Ops", Events: []string{"reddit_failing"}}, {Name: "Posts", Feeds: []int64{feed}}} {
		e.URL, e.Format = webhook(i+1), "discord"
		if _, err := a.AddNotificationEndpoint(ctx, e, redditAt); err != nil {
			t.Fatalf("AddNotificationEndpoint(%s): %v", e.Name, err)
		}
	}
	record(t, a, feed, redditAt)
	fail := func(n int) {
		for i := range n {
			if err := a.RedditFeedFailed(ctx, feed, "reddit answered 429", redditAt.Add(time.Duration(i)*time.Minute)); err != nil {
				t.Fatalf("RedditFeedFailed: %v", err)
			}
		}
	}
	steps := []struct {
		name string
		do   func()
		want []string
	}{
		{"two failures", func() { fail(2) }, nil},
		{"a good read after them", func() { record(t, a, feed, redditAt) }, nil},
		{"three failures", func() { fail(3) }, []string{"Ops:reddit_failed r/example"}},
		{"more failures", func() { fail(4) }, nil},
		{"a good read", func() { record(t, a, feed, redditAt) }, []string{"Ops:reddit_recovered r/example"}},
	}
	for _, s := range steps {
		s.do()
		if got := sentKinds(t, a, redditAt.Add(time.Hour)); !slices.Equal(got, s.want) {
			t.Errorf("after %s: sent %q, want %q", s.name, got, s.want)
		}
	}
}

func TestAFailingNotificationCarriesTheError(t *testing.T) {
	ctx := context.Background()
	a := redditStore(t)
	feed := addFeed(t, a, "example")
	if _, err := a.AddNotificationEndpoint(ctx, NotificationEndpoint{Name: "Ops", URL: webhook(1), Format: "discord", Events: []string{"reddit_failing"}}, redditAt); err != nil {
		t.Fatalf("AddNotificationEndpoint: %v", err)
	}
	for range FeedFailuresNotified {
		if err := a.RedditFeedFailed(ctx, feed, "reddit answered 403", redditAt); err != nil {
			t.Fatalf("RedditFeedFailed: %v", err)
		}
	}
	var got Notification
	if _, err := a.SendNotifications(ctx, redditAt, 10, func(_ context.Context, n Notification, _ NotificationEndpoint) error {
		got = n
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	if got.Kind != "reddit_failed" || got.Reddit.Subreddit != "example" || got.FeedError != "reddit answered 403" {
		t.Errorf("notification = %s r/%s %q, want reddit_failed r/example with its error", got.Kind, got.Reddit.Subreddit, got.FeedError)
	}
}

func TestAPostsTextIsKeptOnlyUntilItIsPosted(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	feed := addFeed(t, a, "example")
	if _, err := a.AddNotificationEndpoint(ctx, NotificationEndpoint{Name: "Posts", URL: webhook(1), Format: "discord", Feeds: []int64{feed}}, redditAt); err != nil {
		t.Fatalf("AddNotificationEndpoint: %v", err)
	}
	kept := func() []string {
		t.Helper()
		var got []string
		rows, err := pool.Query(ctx, "SELECT post_id || ':' || title || author || body || url || image_url FROM reddit_posts ORDER BY post_id")
		if err != nil {
			t.Fatalf("reading reddit_posts: %v", err)
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatalf("reading reddit_posts: %v", err)
			}
			got = append(got, s)
		}
		return got
	}
	record(t, a, feed, redditAt, post(1))
	if got, want := kept(), []string{"t3_abc1:"}; !slices.Equal(got, want) {
		t.Errorf("after the first read, kept %q, want %q: a post never posted keeps no text", got, want)
	}
	record(t, a, feed, redditAt.Add(time.Minute), post(2), post(1))
	if got := kept(); len(got) != 2 || got[1] == "t3_abc2:" {
		t.Errorf("before sending, kept %q, want post 2's text kept to send", got)
	}
	sentPosts(t, a, redditAt.Add(time.Minute))
	if got, want := kept(), []string{"t3_abc1:", "t3_abc2:"}; !slices.Equal(got, want) {
		t.Errorf("after sending, kept %q, want %q", got, want)
	}
}

func TestAFeedSendsOnlyTheKindsItTakes(t *testing.T) {
	ctx := context.Background()
	a := redditStore(t)
	feed := addFeed(t, a, "example")
	if _, err := a.AddNotificationEndpoint(ctx, NotificationEndpoint{Name: "Posts", URL: webhook(1), Format: "discord", Feeds: []int64{feed}}, redditAt); err != nil {
		t.Fatalf("AddNotificationEndpoint: %v", err)
	}
	if err := a.SetRedditFeed(ctx, feed, false, []string{"image", "text"}); err != nil {
		t.Fatalf("SetRedditFeed: %v", err)
	}
	record(t, a, feed, redditAt)
	video, text := post(1), post(2)
	video.Kind = "video"
	if got := record(t, a, feed, redditAt.Add(time.Minute), video, text); got != 1 {
		t.Errorf("queued %d, want 1: the video is not a kind the feed takes", got)
	}
	if got, want := sentPosts(t, a, redditAt.Add(time.Minute)), []string{"Posts:Post 2"}; !slices.Equal(got, want) {
		t.Errorf("sent %q, want %q", got, want)
	}
}

func TestSavingAFeedUnprimesItOnlyWhenItIsResumed(t *testing.T) {
	ctx := context.Background()
	a := redditStore(t)
	feed := addFeed(t, a, "example")
	record(t, a, feed, redditAt)
	primed := func() bool {
		list, err := a.RedditFeeds(ctx)
		if err != nil {
			t.Fatalf("RedditFeeds: %v", err)
		}
		return list[0].Primed
	}
	for _, step := range []struct {
		name   string
		paused bool
		want   bool
	}{
		{"a change of kinds", false, true},
		{"pausing", true, true},
		{"resuming", false, false},
	} {
		if err := a.SetRedditFeed(ctx, feed, step.paused, []string{"text"}); err != nil {
			t.Fatalf("SetRedditFeed (%s): %v", step.name, err)
		}
		if got := primed(); got != step.want {
			t.Errorf("after %s: primed = %t, want %t", step.name, got, step.want)
		}
	}
}

func TestAFeedsKindsAreCheckedByTheDatabase(t *testing.T) {
	ctx := context.Background()
	a := redditStore(t)
	feed := addFeed(t, a, "example")
	if err := a.SetRedditFeed(ctx, feed, false, []string{"poll"}); !errors.Is(err, ErrBadRedditKinds) {
		t.Errorf("an unknown kind: err = %v, want %v", err, ErrBadRedditKinds)
	}
	if err := a.SetRedditFeed(ctx, 999, false, allKinds); !errors.Is(err, ErrNoRedditFeed) {
		t.Errorf("an unknown feed: err = %v, want %v", err, ErrNoRedditFeed)
	}
}

func TestAnEndpointWithoutALookHasTheDatabasesDefault(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO notification_endpoints (name, url, events, created_at)
		VALUES ('Raw', $1, '{}', now()) RETURNING endpoint_id`, webhook(1)).Scan(&id); err != nil {
		t.Fatalf("inserting an endpoint: %v", err)
	}
	e, err := a.NotificationEndpoint(ctx, id)
	if err != nil {
		t.Fatalf("NotificationEndpoint: %v", err)
	}
	if e.Look != DefaultLook {
		t.Errorf("the database's default look = %+v, DefaultLook = %+v: they must match", e.Look, DefaultLook)
	}
	added, err := a.AddNotificationEndpoint(ctx, NotificationEndpoint{Name: "Added", URL: webhook(2), Format: "discord"}, redditAt)
	if err != nil {
		t.Fatalf("AddNotificationEndpoint: %v", err)
	}
	if e, _ := a.NotificationEndpoint(ctx, added); e.Look != DefaultLook {
		t.Errorf("an endpoint added without a look has %+v, want %+v", e.Look, DefaultLook)
	}
}

func TestAPausedEndpointIsSentNothingAndEachEndpointKeepsHowItsLastPostWent(t *testing.T) {
	ctx := context.Background()
	a := redditStore(t)
	feed := addFeed(t, a, "example")
	ids := map[string]int64{}
	for i, e := range []NotificationEndpoint{
		{Name: "Working", Feeds: []int64{feed}},
		{Name: "Broken", Feeds: []int64{feed}},
		{Name: "Paused", Feeds: []int64{feed}, Paused: true},
	} {
		e.URL, e.Format = webhook(i+1), "discord"
		id, err := a.AddNotificationEndpoint(ctx, e, redditAt)
		if err != nil {
			t.Fatalf("AddNotificationEndpoint(%s): %v", e.Name, err)
		}
		ids[e.Name] = id
	}
	record(t, a, feed, redditAt)
	record(t, a, feed, redditAt.Add(time.Minute), post(1))
	var reached []string
	if _, err := a.SendNotifications(ctx, redditAt.Add(time.Minute), 10, func(_ context.Context, _ Notification, e NotificationEndpoint) error {
		reached = append(reached, e.Name)
		if e.Name == "Broken" {
			return errors.New("Discord answered 404")
		}
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	slices.Sort(reached)
	if want := []string{"Broken", "Working"}; !slices.Equal(reached, want) {
		t.Errorf("reached %q, want %q", reached, want)
	}
	for name, want := range map[string]string{"Working": "sent", "Broken": "Discord answered 404", "Paused": "nothing"} {
		e, err := a.NotificationEndpoint(ctx, ids[name])
		if err != nil {
			t.Fatalf("NotificationEndpoint(%s): %v", name, err)
		}
		got := "nothing"
		switch {
		case e.LastError != "" && e.LastErrorAt != nil:
			got = e.LastError
		case e.LastSentAt != nil:
			got = "sent"
		}
		if got != want {
			t.Errorf("%s's last post: %s, want %s", name, got, want)
		}
	}
}

func TestAnEndpointKeepsItsOptionsAndTheDatabaseChecksThem(t *testing.T) {
	ctx := context.Background()
	a := redditStore(t)
	good := NotificationEndpoint{Name: "Posts", URL: webhook(1), Format: "discord", Username: "TTA Reddit",
		AvatarURL: "https://example.com/reddit.png", Forum: true, RoleID: "100000000000000002", Paused: true,
		Look: EmbedLook{Colour: "#123abc", Text: "short", Author: true}}
	id, err := a.AddNotificationEndpoint(ctx, good, redditAt)
	if err != nil {
		t.Fatalf("AddNotificationEndpoint: %v", err)
	}
	e, _ := a.NotificationEndpoint(ctx, id)
	if e.Username != good.Username || e.AvatarURL != good.AvatarURL || !e.Forum || e.RoleID != good.RoleID || !e.Paused || e.Look != good.Look {
		t.Errorf("endpoint = %+v, want its options kept", e)
	}
	for _, tt := range []struct {
		name string
		edit func(e *NotificationEndpoint)
	}{
		{"a name Discord refuses", func(e *NotificationEndpoint) { e.Username = "Discord bot" }},
		{"an avatar over http", func(e *NotificationEndpoint) { e.AvatarURL = "http://example.com/a.png" }},
		{"a role that is not an id", func(e *NotificationEndpoint) { e.RoleID = "@mods" }},
		{"a colour without #", func(e *NotificationEndpoint) { e.Look.Colour = "123abc" }},
		{"a colour name", func(e *NotificationEndpoint) { e.Look.Colour = "orange" }},
		{"an unknown text choice", func(e *NotificationEndpoint) { e.Look.Text = "some" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bad := good
			bad.ID = id
			tt.edit(&bad)
			if err := a.SetNotificationEndpoint(ctx, bad); !errors.Is(err, ErrBadNotificationEndpoint) {
				t.Errorf("SetNotificationEndpoint: err = %v, want %v", err, ErrBadNotificationEndpoint)
			}
		})
	}
}

func TestSendingLocksNoEndpointWhileDiscordAnswers(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	a, other := NewAdminStore(asAdminLogin(t, pool)), NewAdminStore(asAdminLogin(t, pool))
	feed := addFeed(t, a, "example")
	var ids []int64
	for i, name := range []string{"First", "Second"} {
		id, err := a.AddNotificationEndpoint(ctx, NotificationEndpoint{Name: name, URL: webhook(i + 1), Format: "discord", Feeds: []int64{feed}}, redditAt)
		if err != nil {
			t.Fatalf("AddNotificationEndpoint(%s): %v", name, err)
		}
		ids = append(ids, id)
	}
	record(t, a, feed, redditAt)
	record(t, a, feed, redditAt.Add(time.Minute), post(1))
	var editErr error
	if _, err := a.SendNotifications(ctx, redditAt.Add(time.Minute), 10, func(_ context.Context, _ Notification, e NotificationEndpoint) error {
		if e.ID == ids[1] {
			short, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			editErr = other.SetNotificationEndpoint(short, NotificationEndpoint{ID: ids[0], Name: "First, renamed", URL: webhook(1), Format: "discord", Feeds: []int64{feed}})
		}
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	if editErr != nil {
		t.Errorf("editing the first endpoint while the second was sent to: %v, want no wait for the sending batch", editErr)
	}
	if e, _ := a.NotificationEndpoint(ctx, ids[0]); e.LastSentAt == nil || e.Name != "First, renamed" {
		t.Errorf("first endpoint = %q sent at %v, want the edit kept and its delivery recorded", e.Name, e.LastSentAt)
	}
}
