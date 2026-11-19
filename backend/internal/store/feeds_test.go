//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"
)

// feedStore is a fresh database's admin store, on the admin login as the job and the api use it.
func feedStore(t *testing.T) *AdminStore {
	t.Helper()
	return NewAdminStore(asAdminLogin(t, migrated(t)))
}

// newItem reads feed once, priming it, then records the new item n, which queues it.
func newItem(t *testing.T, a *AdminStore, feed int64, n int) {
	t.Helper()
	recordItems(t, a, feed, rssAt)
	recordItems(t, a, feed, rssAt.Add(time.Minute), item(n, fmt.Sprintf("Item %d", n), ""))
}

func TestAFeedFailingThreeReadsInARowIsNotifiedOnceAndSoIsItsRecovery(t *testing.T) {
	ctx := context.Background()
	a := feedStore(t)
	feed := rssFeed(t, a, 1)
	endpoint(t, a, "Ops", 1, "feed_failing")
	subscribed(t, a, "Items", 2, FeedOptions{Feed: feed})
	recordItems(t, a, feed, rssAt)
	fail := func(n int) {
		for i := range n {
			if err := a.FeedFailed(ctx, feed, "the feed answered 503", rssAt.Add(time.Duration(i)*time.Minute)); err != nil {
				t.Fatalf("FeedFailed: %v", err)
			}
		}
	}
	steps := []struct {
		name string
		do   func()
		want []string
	}{
		{"two failures", func() { fail(2) }, nil},
		{"a good read after them", func() { recordItems(t, a, feed, rssAt) }, nil},
		{"three failures", func() { fail(3) }, []string{"Ops:feed_failed Example news"}},
		{"more failures", func() { fail(4) }, nil},
		{"a good read", func() { recordItems(t, a, feed, rssAt) }, []string{"Ops:feed_recovered Example news"}},
	}
	for _, s := range steps {
		s.do()
		var got []string
		if _, err := a.SendNotifications(ctx, rssAt.Add(time.Hour), 10, func(_ context.Context, n Notification, e NotificationEndpoint) error {
			got = append(got, fmt.Sprintf("%s:%s %s", e.Name, n.Kind, n.FeedName))
			return nil
		}); err != nil {
			t.Fatalf("SendNotifications: %v", err)
		}
		if !slices.Equal(got, s.want) {
			t.Errorf("after %s: sent %q, want %q", s.name, got, s.want)
		}
	}
}

func TestAFailingNotificationCarriesTheError(t *testing.T) {
	ctx := context.Background()
	a := feedStore(t)
	feed := rssFeed(t, a, 1)
	endpoint(t, a, "Ops", 1, "feed_failing")
	for range FeedFailuresNotified {
		if err := a.FeedFailed(ctx, feed, "the feed answered 403", rssAt); err != nil {
			t.Fatalf("FeedFailed: %v", err)
		}
	}
	var got Notification
	if _, err := a.SendNotifications(ctx, rssAt, 10, func(_ context.Context, n Notification, _ NotificationEndpoint) error {
		got = n
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	if got.Kind != "feed_failed" || got.FeedKind != "rss" || got.FeedName != "https://news.example.com/feed-1.xml" || got.FeedError != "the feed answered 403" {
		t.Errorf("notification = %s %s %q %q, want feed_failed rss, the feed's address (it has no title before a good read) and its error", got.Kind, got.FeedKind, got.FeedName, got.FeedError)
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
	added, err := a.AddNotificationEndpoint(ctx, NotificationEndpoint{Name: "Added", URL: webhook(2), Format: "discord"}, rssAt)
	if err != nil {
		t.Fatalf("AddNotificationEndpoint: %v", err)
	}
	if e, _ := a.NotificationEndpoint(ctx, added); e.Look != DefaultLook {
		t.Errorf("an endpoint added without a look has %+v, want %+v", e.Look, DefaultLook)
	}
}

func TestAPausedEndpointIsSentNothingAndEachEndpointKeepsHowItsLastPostWent(t *testing.T) {
	ctx := context.Background()
	a := feedStore(t)
	feed := rssFeed(t, a, 1)
	ids := map[string]int64{}
	for i, e := range []NotificationEndpoint{
		{Name: "Working", Feeds: []int64{feed}},
		{Name: "Broken", Feeds: []int64{feed}},
		{Name: "Paused", Feeds: []int64{feed}, Paused: true},
	} {
		e.URL, e.Format = webhook(i+1), "discord"
		id, err := a.AddNotificationEndpoint(ctx, e, rssAt)
		if err != nil {
			t.Fatalf("AddNotificationEndpoint(%s): %v", e.Name, err)
		}
		ids[e.Name] = id
	}
	newItem(t, a, feed, 1)
	var reached []string
	if _, err := a.SendNotifications(ctx, rssAt.Add(time.Minute), 10, func(_ context.Context, _ Notification, e NotificationEndpoint) error {
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
	a := feedStore(t)
	good := NotificationEndpoint{Name: "Posts", URL: webhook(1), Format: "discord", Username: "TTA News",
		AvatarURL: "https://example.com/news.png", Forum: true, RoleID: "100000000000000002", Paused: true,
		ThreadID: "100000000000000021", Look: EmbedLook{Colour: "#123abc", Text: "short"}}
	id, err := a.AddNotificationEndpoint(ctx, good, rssAt)
	if err != nil {
		t.Fatalf("AddNotificationEndpoint: %v", err)
	}
	e, _ := a.NotificationEndpoint(ctx, id)
	if e.Username != good.Username || e.AvatarURL != good.AvatarURL || !e.Forum || e.RoleID != good.RoleID || !e.Paused || e.Look != good.Look ||
		e.ThreadID != good.ThreadID {
		t.Errorf("endpoint = %+v, want its options kept", e)
	}
	for _, tt := range []struct {
		name string
		edit func(e *NotificationEndpoint)
	}{
		{"a name Discord refuses", func(e *NotificationEndpoint) { e.Username = "Discord bot" }},
		{"an avatar over http", func(e *NotificationEndpoint) { e.AvatarURL = "http://example.com/a.png" }},
		{"a role that is not an id", func(e *NotificationEndpoint) { e.RoleID = "@mods" }},
		{"a thread that is not an id", func(e *NotificationEndpoint) { e.ThreadID = "https://discord.com/channels/1/2/3" }},
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

func TestMigration99KeepsEndpointsGoingDown(t *testing.T) {
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	if _, err := a.AddNotificationEndpoint(context.Background(), NotificationEndpoint{Name: "Thread", URL: webhook(1), Format: "discord",
		ThreadID: "100000000000000021"}, rssAt); err != nil {
		t.Fatalf("AddNotificationEndpoint: %v", err)
	}
	migrateTo(t, pool, 98, true)
	if got := rowsOf(t, pool, "SELECT name FROM notification_endpoints"); len(got) != 1 {
		t.Errorf("endpoints at 98 = %q, want the one kept without its thread", got)
	}
	migrateTo(t, pool, 99, false)
}

func TestSendingLocksNoEndpointWhileDiscordAnswers(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	a, other := NewAdminStore(asAdminLogin(t, pool)), NewAdminStore(asAdminLogin(t, pool))
	feed := rssFeed(t, a, 1)
	var ids []int64
	for i, name := range []string{"First", "Second"} {
		id, err := a.AddNotificationEndpoint(ctx, NotificationEndpoint{Name: name, URL: webhook(i + 1), Format: "discord", Feeds: []int64{feed}}, rssAt)
		if err != nil {
			t.Fatalf("AddNotificationEndpoint(%s): %v", name, err)
		}
		ids = append(ids, id)
	}
	newItem(t, a, feed, 1)
	var editErr error
	if _, err := a.SendNotifications(ctx, rssAt.Add(time.Minute), 10, func(_ context.Context, _ Notification, e NotificationEndpoint) error {
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

func TestAnEndpointKeepsItsFeedsAndRefusesAnUnknownOne(t *testing.T) {
	ctx := context.Background()
	a := feedStore(t)
	one, two := rssFeed(t, a, 1), rssFeed(t, a, 2)
	id, err := a.AddNotificationEndpoint(ctx, NotificationEndpoint{Name: "News", URL: webhook(1), Format: "discord", Feeds: []int64{two, one, two}}, rssAt)
	if err != nil {
		t.Fatalf("AddNotificationEndpoint: %v", err)
	}
	if e, _ := a.NotificationEndpoint(ctx, id); !slices.Equal(e.Feeds, []int64{one, two}) {
		t.Errorf("feeds = %v, want %v", e.Feeds, []int64{one, two})
	}
	if err := a.SetNotificationEndpoint(ctx, NotificationEndpoint{ID: id, Name: "News", URL: webhook(1), Format: "discord", Feeds: []int64{two}}); err != nil {
		t.Fatalf("SetNotificationEndpoint: %v", err)
	}
	if e, _ := a.NotificationEndpoint(ctx, id); !slices.Equal(e.Feeds, []int64{two}) {
		t.Errorf("feeds after a change = %v, want %v", e.Feeds, []int64{two})
	}
	if err := a.SetNotificationEndpoint(ctx, NotificationEndpoint{ID: id, Name: "News", URL: webhook(1), Format: "discord", Feeds: []int64{999}}); !errors.Is(err, ErrBadNotificationEndpoint) {
		t.Errorf("an unknown feed: err = %v, want %v", err, ErrBadNotificationEndpoint)
	}
	if err := a.RemoveRSSFeed(ctx, two); err != nil {
		t.Fatalf("RemoveRSSFeed: %v", err)
	}
	if e, _ := a.NotificationEndpoint(ctx, id); len(e.Feeds) != 0 {
		t.Errorf("feeds after the feed was removed = %v, want none", e.Feeds)
	}
}
