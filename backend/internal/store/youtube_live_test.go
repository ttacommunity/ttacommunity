//go:build integration

package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// liveSetup adds the YouTube Live feed and stores the pushed video pushVideo of the linked channel, about TTA or
// not, and returns the stores, the feed's id and the time.
func liveSetup(t *testing.T, tta bool) (*PlayerStore, *AdminStore, int64, time.Time) {
	t.Helper()
	p, a, _, now := pushSetup(t)
	feed, err := a.AddYouTubeFeed(context.Background(), now)
	if err != nil {
		t.Fatalf("AddYouTubeFeed: %v", err)
	}
	if err := p.StorePushedVideo(context.Background(), pushed("Friday final", tta), now); err != nil {
		t.Fatalf("StorePushedVideo: %v", err)
	}
	return p, a, feed, now
}

// noteStream notes pushVideo as a stream at at and returns its ref, failing when it is not due then.
func noteStream(t *testing.T, p *PlayerStore, a *AdminStore, at time.Time) int64 {
	t.Helper()
	if err := p.NoteYouTubeStream(context.Background(), pushVideo, pushChannel, at); err != nil {
		t.Fatalf("NoteYouTubeStream: %v", err)
	}
	due := dueVideos(t, a, at)
	if len(due) != 1 {
		t.Fatalf("due after noting = %+v, want the one stream", due)
	}
	return due[0].Ref
}

// dueVideos is what DueYouTubeStreams gives at now.
func dueVideos(t *testing.T, a *AdminStore, now time.Time) []YouTubeStream {
	t.Helper()
	due, err := a.DueYouTubeStreams(context.Background(), now, 50)
	if err != nil {
		t.Fatalf("DueYouTubeStreams: %v", err)
	}
	return due
}

// checkStream records c of the stream ref at now.
func checkStream(t *testing.T, a *AdminStore, ref int64, c YouTubeCheck, now time.Time) {
	t.Helper()
	if err := a.RecordYouTubeCheck(context.Background(), ref, c, now); err != nil {
		t.Fatalf("RecordYouTubeCheck(%+v): %v", c, err)
	}
}

// liveSent sends what is due at now and returns each post as "endpoint kind title", keeping, when keep is set,
// each live post an endpoint edits once the stream ends, as the api does.
func liveSent(t *testing.T, a *AdminStore, now time.Time, keep bool) []string {
	t.Helper()
	var got []string
	if _, err := a.SendNotifications(context.Background(), now, 50, func(ctx context.Context, n Notification, e NotificationEndpoint) error {
		got = append(got, e.Name+" "+n.Kind+" "+n.YouTube.Title)
		if keep && n.Kind == "youtube_live" && e.TakesWasLive(n.FeedID) {
			return a.RecordYouTubePost(ctx, n.StreamRef, e.ID, DiscordPost{MessageID: "100000000000000201", ChannelID: "100000000000000301"})
		}
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	slices.Sort(got)
	return got
}

func timeAt(t time.Time) *time.Time { return &t }

func TestNoteYouTubeStreamNeedsAnUnpausedFeedAndAStoredVideo(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		feed    bool
		paused  bool
		video   string
		channel string
		want    int
	}{
		{name: "the feed and a stored video", feed: true, video: pushVideo, channel: pushChannel, want: 1},
		{name: "no feed", video: pushVideo, channel: pushChannel},
		{name: "the feed paused", feed: true, paused: true, video: pushVideo, channel: pushChannel},
		{name: "a video not stored", feed: true, video: "vid00000009", channel: pushChannel},
		{name: "another channel's video", feed: true, video: pushVideo, channel: pushUnlinked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, a, _, now := pushSetup(t)
			if err := p.StorePushedVideo(ctx, pushed("Friday final", true), now); err != nil {
				t.Fatalf("StorePushedVideo: %v", err)
			}
			if tt.feed {
				id, err := a.AddYouTubeFeed(ctx, now)
				if err != nil {
					t.Fatalf("AddYouTubeFeed: %v", err)
				}
				if err := a.SetYouTubeFeedPaused(ctx, id, tt.paused); err != nil {
					t.Fatalf("SetYouTubeFeedPaused: %v", err)
				}
			}
			if err := p.NoteYouTubeStream(ctx, tt.video, tt.channel, now); err != nil {
				t.Fatalf("NoteYouTubeStream: %v", err)
			}
			if got := dueVideos(t, a, now); len(got) != tt.want {
				t.Errorf("due = %+v, want %d", got, tt.want)
			}
		})
	}
}

func TestDueYouTubeStreamsWaitForTheirCheck(t *testing.T) {
	p, a, _, now := liveSetup(t, true)
	ref := noteStream(t, p, a, now)
	checkStream(t, a, ref, YouTubeCheck{Title: "Friday final", NextCheckAt: timeAt(now.Add(time.Hour))}, now)
	if got := dueVideos(t, a, now.Add(59*time.Minute)); len(got) != 0 {
		t.Errorf("due before its check = %+v, want none", got)
	}
	if got := dueVideos(t, a, now.Add(time.Hour)); len(got) != 1 || got[0].VideoID != pushVideo || !got[0].RecordedAt.Equal(now) {
		t.Errorf("due at its check = %+v, want %s noted at %v", got, pushVideo, now)
	}
}

func TestAYouTubeStreamIsPostedOnceWhenItStarts(t *testing.T) {
	p, a, feed, now := liveSetup(t, true)
	subscribed(t, a, "Streams", 1, FeedOptions{Feed: feed})
	ref := noteStream(t, p, a, now)
	checkStream(t, a, ref, YouTubeCheck{Title: "Friday final", NextCheckAt: timeAt(now.Add(time.Minute))}, now)
	if got := liveSent(t, a, now, false); len(got) != 0 {
		t.Errorf("sent while scheduled = %q, want nothing", got)
	}
	started := now.Add(2 * time.Minute)
	checkStream(t, a, ref, YouTubeCheck{Title: "Friday final, live", StartedAt: timeAt(started), NextCheckAt: timeAt(started.Add(5 * time.Minute))}, started)
	if got, want := liveSent(t, a, started, false), []string{"Streams youtube_live Friday final, live"}; !slices.Equal(got, want) {
		t.Errorf("sent once live = %q, want %q", got, want)
	}
	checkStream(t, a, ref, YouTubeCheck{Title: "Friday final, live", StartedAt: timeAt(started), NextCheckAt: timeAt(started.Add(10 * time.Minute))}, started.Add(5*time.Minute))
	if got := liveSent(t, a, started.Add(5*time.Minute), false); len(got) != 0 {
		t.Errorf("sent at the next check = %q, want nothing more", got)
	}
}

func TestAYouTubeStreamIsPostedOnlyWhenMoviesShowsIt(t *testing.T) {
	ctx := context.Background()
	yes, no := true, false
	tests := []struct {
		name     string
		tta      bool
		override *bool
		want     int
	}{
		{name: "about TTA", tta: true, want: 1},
		{name: "not about TTA", tta: false, want: 0},
		{name: "not about TTA, shown by an admin", tta: false, override: &yes, want: 1},
		{name: "about TTA, hidden by an admin", tta: true, override: &no, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, a, feed, now := liveSetup(t, tt.tta)
			subscribed(t, a, "Streams", 1, FeedOptions{Feed: feed})
			if tt.override != nil {
				if err := a.SetMovieVideoOverride(ctx, pushVideo, tt.override); err != nil {
					t.Fatalf("SetMovieVideoOverride: %v", err)
				}
			}
			ref := noteStream(t, p, a, now)
			checkStream(t, a, ref, YouTubeCheck{Title: "Friday final", StartedAt: timeAt(now), NextCheckAt: timeAt(now.Add(5 * time.Minute))}, now)
			if got := liveSent(t, a, now, false); len(got) != tt.want {
				t.Errorf("sent = %q, want %d post", got, tt.want)
			}
		})
	}
}

func TestAYouTubeStreamIsPostedToEndpointsWhoseKeywordsItHas(t *testing.T) {
	p, a, feed, now := liveSetup(t, true)
	subscribed(t, a, "Any", 1, FeedOptions{Feed: feed})
	subscribed(t, a, "Finals", 2, FeedOptions{Feed: feed, Keywords: []string{"FINAL"}})
	subscribed(t, a, "Leagues", 3, FeedOptions{Feed: feed, Keywords: []string{"league"}})
	ref := noteStream(t, p, a, now)
	checkStream(t, a, ref, YouTubeCheck{Title: "Friday final", StartedAt: timeAt(now), NextCheckAt: timeAt(now.Add(5 * time.Minute))}, now)
	if got, want := liveSent(t, a, now, false), []string{"Any youtube_live Friday final", "Finals youtube_live Friday final"}; !slices.Equal(got, want) {
		t.Errorf("sent = %q, want %q", got, want)
	}
}

func TestAYouTubeStreamEndingEditsOnlyThePostsKept(t *testing.T) {
	tests := []struct {
		name  string
		ended YouTubeCheck
	}{
		{name: "YouTube says it ended", ended: YouTubeCheck{Title: "Friday final"}},
		{name: "YouTube no longer shows it", ended: YouTubeCheck{Gone: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, a, feed, now := liveSetup(t, true)
			subscribed(t, a, "Was live", 1, FeedOptions{Feed: feed, WasLive: true})
			subscribed(t, a, "Live only", 2, FeedOptions{Feed: feed})
			ref := noteStream(t, p, a, now)
			checkStream(t, a, ref, YouTubeCheck{Title: "Friday final", StartedAt: timeAt(now), NextCheckAt: timeAt(now.Add(5 * time.Minute))}, now)
			if got := liveSent(t, a, now, true); len(got) != 2 {
				t.Fatalf("sent live = %q, want both endpoints", got)
			}
			end := now.Add(2 * time.Hour)
			c := tt.ended
			if !c.Gone {
				c.StartedAt, c.EndedAt = timeAt(now), timeAt(end)
			}
			checkStream(t, a, ref, c, end)
			checkStream(t, a, ref, c, end.Add(time.Minute))
			want := []string{"Was live youtube_ended Friday final"}
			if got := liveSent(t, a, end.Add(time.Minute), false); !slices.Equal(got, want) {
				t.Errorf("sent once ended = %q, want %q", got, want)
			}
			if got := dueVideos(t, a, end.Add(time.Hour)); len(got) != 0 {
				t.Errorf("due once ended = %+v, want none", got)
			}
		})
	}
}

func TestAYouTubeStreamNoticeTellsOfItsVideo(t *testing.T) {
	p, a, feed, now := liveSetup(t, true)
	subscribed(t, a, "Streams", 1, FeedOptions{Feed: feed})
	ref := noteStream(t, p, a, now)
	checkStream(t, a, ref, YouTubeCheck{Title: "Friday final", StartedAt: timeAt(now), NextCheckAt: timeAt(now.Add(5 * time.Minute))}, now)
	var got []YouTubeNotice
	if _, err := a.SendNotifications(context.Background(), now, 50, func(_ context.Context, n Notification, _ NotificationEndpoint) error {
		got = append(got, n.YouTube)
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("notices = %+v, want one", got)
	}
	y := got[0]
	if y.Channel != "Alice Casts" || y.VideoID != pushVideo || y.ThumbnailURL != pushThumbnail || !y.StartedAt.Equal(now) || y.EndedAt != nil {
		t.Errorf("notice = %+v, want Alice Casts' %s with its picture, started at %v, live", y, pushVideo, now)
	}
}

func TestAPausedYouTubeFeedIsNotCheckedOrPosted(t *testing.T) {
	ctx := context.Background()
	p, a, feed, now := liveSetup(t, true)
	subscribed(t, a, "Streams", 1, FeedOptions{Feed: feed})
	ref := noteStream(t, p, a, now)
	if err := a.SetYouTubeFeedPaused(ctx, feed, true); err != nil {
		t.Fatalf("SetYouTubeFeedPaused: %v", err)
	}
	if got := dueVideos(t, a, now); len(got) != 0 {
		t.Errorf("due while paused = %+v, want none", got)
	}
	checkStream(t, a, ref, YouTubeCheck{Title: "Friday final", StartedAt: timeAt(now), NextCheckAt: timeAt(now.Add(5 * time.Minute))}, now)
	if got := liveSent(t, a, now, false); len(got) != 0 {
		t.Errorf("sent while paused = %q, want nothing", got)
	}
}

func TestAdminKeepsOneYouTubeFeed(t *testing.T) {
	ctx := context.Background()
	p, a, feed, now := liveSetup(t, true)
	if _, err := a.AddYouTubeFeed(ctx, now); !errors.Is(err, ErrYouTubeFeedExists) {
		t.Errorf("adding a second = %v, want ErrYouTubeFeedExists", err)
	}
	ref := noteStream(t, p, a, now)
	checkStream(t, a, ref, YouTubeCheck{Title: "Friday final", NextCheckAt: timeAt(now.Add(time.Hour))}, now)
	feeds, err := a.YouTubeFeeds(ctx)
	if err != nil || len(feeds) != 1 {
		t.Fatalf("YouTubeFeeds = %+v, %v; want the one feed", feeds, err)
	}
	if f := feeds[0]; f.ID != feed || f.Channels != 1 || f.Waiting != 1 || f.Live != 0 {
		t.Errorf("the feed = %+v, want %d with 1 channel and 1 stream waiting", f, feed)
	}
	if twitch, err := a.TwitchFeeds(ctx); err != nil || len(twitch) != 0 {
		t.Errorf("TwitchFeeds = %+v, %v; want no YouTube feed among them", twitch, err)
	}
	if err := a.RemoveYouTubeFeed(ctx, feed); err != nil {
		t.Fatalf("RemoveYouTubeFeed: %v", err)
	}
	if got := dueVideos(t, a, now.Add(time.Hour)); len(got) != 0 {
		t.Errorf("due once removed = %+v, want none", got)
	}
	for name, err := range map[string]error{
		"pausing":  a.SetYouTubeFeedPaused(ctx, feed, true),
		"removing": a.RemoveYouTubeFeed(ctx, feed),
	} {
		if !errors.Is(err, ErrNoYouTubeFeed) {
			t.Errorf("%s a removed feed = %v, want ErrNoYouTubeFeed", name, err)
		}
	}
}

func TestMigration96DropsOnlyYouTubeGoingDown(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	rss := rssFeed(t, a, 1)
	youtube, err := a.AddYouTubeFeed(ctx, rssAt)
	if err != nil {
		t.Fatalf("AddYouTubeFeed: %v", err)
	}
	subscribed(t, a, "Both", 1, FeedOptions{Feed: youtube, WasLive: true})
	migrateTo(t, pool, 95, true)
	if got := rowsOf(t, pool, "SELECT kind FROM feeds ORDER BY feed_id"); !slices.Equal(got, []string{"rss"}) {
		t.Errorf("feeds at 95 = %q, want only the RSS feed (%d)", got, rss)
	}
	migrateTo(t, pool, 96, false)
}
