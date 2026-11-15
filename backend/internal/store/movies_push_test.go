//go:build integration

package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
)

const (
	pushChannel   = "UC0000000000000000000001"
	pushUnlinked  = "UC0000000000000000000002"
	pushVideo     = "vid00000001"
	pushThumbnail = "https://i.ytimg.com/vi/vid00000001/hqdefault.jpg"
)

func pushSetup(t *testing.T) (*PlayerStore, *AdminStore, *RatingsReader, time.Time) {
	t.Helper()
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	now := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	if _, err := a.AddMovieChannel(context.Background(), pushChannel, "Alice Casts", nil, false, now); err != nil {
		t.Fatalf("AddMovieChannel: %v", err)
	}
	return NewPlayerStore(asPlayerLogin(t, pool)), a, NewRatingsReader(asReader(t, pool)), now
}

func pushed(title string, tta bool) PushedVideo {
	return PushedVideo{
		VideoID: pushVideo, ChannelID: pushChannel, Title: title,
		PublishedAt: time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC), ThumbnailURL: pushThumbnail,
		TTA: tta, Reason: "test",
	}
}

func shownTitles(t *testing.T, r *RatingsReader) []string {
	t.Helper()
	feed, err := r.Movies(context.Background())
	if err != nil {
		t.Fatalf("Movies: %v", err)
	}
	var titles []string
	for _, v := range feed.Videos {
		titles = append(titles, v.Title)
	}
	return titles
}

func TestConfirmMovieHubConfirmsOnlyWhatWeAsked(t *testing.T) {
	tests := []struct {
		name    string
		channel string
		mode    string
		want    bool
	}{
		{"subscribe to a linked channel", pushChannel, "subscribe", true},
		{"subscribe to an unlinked channel", pushUnlinked, "subscribe", false},
		{"unsubscribe from a linked channel", pushChannel, "unsubscribe", false},
		{"unsubscribe from an unlinked channel", pushUnlinked, "unsubscribe", true},
		{"an unknown mode", pushChannel, "denied", false},
	}
	p, _, _, now := pushSetup(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.ConfirmMovieHub(context.Background(), tt.channel, tt.mode, 432000, now)
			if err != nil {
				t.Fatalf("ConfirmMovieHub: %v", err)
			}
			if got != tt.want {
				t.Errorf("ConfirmMovieHub(%q, %q) = %v, want %v", tt.channel, tt.mode, got, tt.want)
			}
		})
	}
}

func TestConfirmMovieHubRecordsTheLease(t *testing.T) {
	p, a, _, now := pushSetup(t)
	if _, err := p.ConfirmMovieHub(context.Background(), pushChannel, "subscribe", 432000, now); err != nil {
		t.Fatalf("ConfirmMovieHub: %v", err)
	}
	state, err := a.AdminMovies(context.Background())
	if err != nil {
		t.Fatalf("AdminMovies: %v", err)
	}
	ch := state.Channels[0]
	if ch.LeaseEndsAt == nil || !ch.LeaseEndsAt.Equal(now.Add(5*24*time.Hour)) {
		t.Errorf("lease ends at %v, want %v", ch.LeaseEndsAt, now.Add(5*24*time.Hour))
	}
	if ch.SubscribedAt == nil || !ch.SubscribedAt.Equal(now) {
		t.Errorf("subscribed at %v, want %v", ch.SubscribedAt, now)
	}
}

func TestStorePushedVideoShowsOnlyTTAVideos(t *testing.T) {
	tests := []struct {
		name string
		tta  bool
		want []string
	}{
		{"a TTA video is shown", true, []string{"Through the Ages final"}},
		{"another game's video is kept but not shown", false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _, r, now := pushSetup(t)
			if err := p.StorePushedVideo(context.Background(), pushed("Through the Ages final", tt.tta), now); err != nil {
				t.Fatalf("StorePushedVideo: %v", err)
			}
			if got := shownTitles(t, r); !slices.Equal(got, tt.want) {
				t.Errorf("shown = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStorePushedVideoTakesANewTitleAndVerdict(t *testing.T) {
	p, _, r, now := pushSetup(t)
	ctx := context.Background()
	if err := p.StorePushedVideo(ctx, pushed("Chess stream", false), now); err != nil {
		t.Fatalf("StorePushedVideo: %v", err)
	}
	if err := p.StorePushedVideo(ctx, pushed("Through the Ages final", true), now.Add(time.Hour)); err != nil {
		t.Fatalf("StorePushedVideo again: %v", err)
	}
	if got, want := shownTitles(t, r), []string{"Through the Ages final"}; !slices.Equal(got, want) {
		t.Errorf("shown = %q, want %q", got, want)
	}
}

func TestStorePushedVideoKeepsAnAdminsOverride(t *testing.T) {
	p, a, r, now := pushSetup(t)
	ctx := context.Background()
	if err := p.StorePushedVideo(ctx, pushed("Through the Ages final", true), now); err != nil {
		t.Fatalf("StorePushedVideo: %v", err)
	}
	if err := a.SetMovieVideoHidden(ctx, pushVideo, true); err != nil {
		t.Fatalf("SetMovieVideoHidden: %v", err)
	}
	if err := p.StorePushedVideo(ctx, pushed("Through the Ages final, recut", true), now.Add(time.Hour)); err != nil {
		t.Fatalf("StorePushedVideo again: %v", err)
	}
	if got := shownTitles(t, r); len(got) != 0 {
		t.Errorf("shown = %q after an admin hid it, want none", got)
	}
}

func TestStorePushedVideoRefusesAnUnlinkedChannel(t *testing.T) {
	p, a, _, now := pushSetup(t)
	v := pushed("Through the Ages final", true)
	v.ChannelID = pushUnlinked
	if err := p.StorePushedVideo(context.Background(), v, now); !errors.Is(err, ErrNoChannel) {
		t.Fatalf("StorePushedVideo = %v, want ErrNoChannel", err)
	}
	state, err := a.AdminMovies(context.Background())
	if err != nil {
		t.Fatalf("AdminMovies: %v", err)
	}
	if len(state.Videos) != 0 {
		t.Errorf("stored %d videos for an unlinked channel, want 0", len(state.Videos))
	}
}

func TestStorePushedVideoLeavesAnotherChannelsVideo(t *testing.T) {
	p, a, r, now := pushSetup(t)
	ctx := context.Background()
	if _, err := a.AddMovieChannel(ctx, pushUnlinked, "Bob Plays", nil, false, now); err != nil {
		t.Fatalf("AddMovieChannel: %v", err)
	}
	if err := p.StorePushedVideo(ctx, pushed("Through the Ages final", true), now); err != nil {
		t.Fatalf("StorePushedVideo: %v", err)
	}
	other := pushed("Taken over", true)
	other.ChannelID = pushUnlinked
	if err := p.StorePushedVideo(ctx, other, now.Add(time.Hour)); err != nil {
		t.Fatalf("StorePushedVideo for the other channel: %v", err)
	}
	if got, want := shownTitles(t, r), []string{"Through the Ages final"}; !slices.Equal(got, want) {
		t.Errorf("shown = %q, want %q", got, want)
	}
}

func TestRemovePushedVideoDropsIt(t *testing.T) {
	p, _, r, now := pushSetup(t)
	ctx := context.Background()
	if err := p.StorePushedVideo(ctx, pushed("Through the Ages final", true), now); err != nil {
		t.Fatalf("StorePushedVideo: %v", err)
	}
	if err := p.RemovePushedVideo(ctx, pushVideo, pushChannel, now.Add(time.Hour)); err != nil {
		t.Fatalf("RemovePushedVideo: %v", err)
	}
	if got := shownTitles(t, r); len(got) != 0 {
		t.Errorf("shown = %q after removal, want none", got)
	}
}

func TestMigration85KeepsHiddenVideosHiddenBothWays(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	down := func(to int64) {
		t.Helper()
		if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, to); return err }); err != nil {
			t.Fatalf("migrating down to %d: %v", to, err)
		}
	}
	down(84)
	for _, q := range []string{
		`INSERT INTO video_channels (channel_id, name) VALUES ('` + pushChannel + `', 'Alice Casts')`,
		`INSERT INTO videos (video_id, channel_id, title, published_at, thumbnail_url, hidden) VALUES
		   ('vid00000001', '` + pushChannel + `', 'Shown', '2026-11-01', 'x', false),
		   ('vid00000002', '` + pushChannel + `', 'Hidden', '2026-11-02', 'x', true)`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("building the old shape: %v\n%s", err, q)
		}
	}
	if err := migrateRatingsTo(ctx, pool, 85); err != nil {
		t.Fatalf("migrateRatingsTo(85): %v", err)
	}
	if got, want := rowsOf(t, pool, `SELECT title FROM videos WHERE coalesce(tta_override, is_tta) ORDER BY 1`), []string{"Shown"}; !slices.Equal(got, want) {
		t.Errorf("after the migration, shown = %q, want %q", got, want)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO videos (video_id, channel_id, title, published_at, thumbnail_url, is_tta, tta_reason)
		VALUES ('vid00000003', '`+pushChannel+`', 'Chess', '2026-11-03', 'x', false, 'no match')`); err != nil {
		t.Fatalf("storing a rejected video: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE videos SET created_at = '2026-11-01T10:00:00Z'`); err != nil {
		t.Fatalf("setting created_at: %v", err)
	}
	down(84)
	if got, want := rowsOf(t, pool, `SELECT title || '|' || hidden FROM videos ORDER BY 1`), []string{"Hidden|true", "Shown|false"}; !slices.Equal(got, want) {
		t.Errorf("after the Down, videos (title|hidden) = %q, want %q (a rejected video is dropped)", got, want)
	}
	if got := rowsOf(t, pool, `SELECT DISTINCT (created_at = '2026-11-01T10:00:00Z')::text FROM videos`); !slices.Equal(got, []string{"true"}) {
		t.Errorf("after the Down, created_at kept = %q, want [true]", got)
	}
	columns := `SELECT string_agg(column_name, ',' ORDER BY ordinal_position) FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1`
	for table, want := range map[string]string{
		"videos":         "video_id,channel_id,title,published_at,thumbnail_url,hidden,created_at",
		"video_channels": "channel_id,name,player_id,include_all,created_at,last_checked_at",
	} {
		var got string
		if err := pool.QueryRow(ctx, columns, table).Scan(&got); err != nil {
			t.Fatalf("reading %s's columns: %v", table, err)
		}
		if got != want {
			t.Errorf("after the Down, %s's columns = %q, want 84's %q", table, got, want)
		}
	}
	if got := rowsOf(t, pool, `SELECT conname FROM pg_constraint WHERE conrelid = 'videos'::regclass AND conname LIKE 'videos_created%'`); !slices.Equal(got, []string{"videos_created_at_not_null"}) {
		t.Errorf("after the Down, created_at's constraint = %q, want 84's [videos_created_at_not_null]", got)
	}
}
