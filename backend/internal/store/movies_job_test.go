//go:build integration

package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestTheMovieJobSavesThroughTheAdminLogin(t *testing.T) {
	_, a, r, now := pushSetup(t)
	ctx := context.Background()
	v := pushed("Through the Ages final", true)
	v.DetailsFetchedAt = &now
	if err := a.SaveMovieVideo(ctx, v, now); err != nil {
		t.Fatalf("SaveMovieVideo: %v", err)
	}
	if got, want := shownTitles(t, r), []string{"Through the Ages final"}; !slices.Equal(got, want) {
		t.Errorf("shown = %q, want %q", got, want)
	}
	other := pushed("Elsewhere", true)
	other.ChannelID = pushUnlinked
	if err := a.SaveMovieVideo(ctx, other, now); !errors.Is(err, ErrNoChannel) {
		t.Errorf("saving an unlinked channel's video: err = %v, want %v", err, ErrNoChannel)
	}
	videos, err := a.MovieJobVideos(ctx)
	if err != nil {
		t.Fatalf("MovieJobVideos: %v", err)
	}
	if len(videos) != 1 || videos[0].VideoID != pushVideo || !videos[0].DetailsFetched {
		t.Errorf("MovieJobVideos = %+v, want %s with its details fetched", videos, pushVideo)
	}
	state, err := a.AdminMovies(ctx)
	if err != nil {
		t.Fatalf("AdminMovies: %v", err)
	}
	if state.Channels[0].LastPushAt != nil {
		t.Errorf("last_push_at = %v after the job saved a video, want unset: the hub pushed nothing", state.Channels[0].LastPushAt)
	}
}

func TestTheMovieJobKeepsAnAdminsOverride(t *testing.T) {
	_, a, r, now := pushSetup(t)
	ctx := context.Background()
	if err := a.SaveMovieVideo(ctx, pushed("Chess", false), now); err != nil {
		t.Fatalf("SaveMovieVideo: %v", err)
	}
	on := true
	if err := a.SetMovieVideoOverride(ctx, pushVideo, &on); err != nil {
		t.Fatalf("SetMovieVideoOverride: %v", err)
	}
	if err := a.SaveMovieVideo(ctx, pushed("Chess, recut", false), now.Add(time.Hour)); err != nil {
		t.Fatalf("SaveMovieVideo again: %v", err)
	}
	if got, want := shownTitles(t, r), []string{"Chess, recut"}; !slices.Equal(got, want) {
		t.Errorf("shown = %q, want %q", got, want)
	}
}

func TestTheMovieJobDeletesAVideo(t *testing.T) {
	_, a, r, now := pushSetup(t)
	ctx := context.Background()
	if err := a.SaveMovieVideo(ctx, pushed("Through the Ages final", true), now); err != nil {
		t.Fatalf("SaveMovieVideo: %v", err)
	}
	if err := a.DeleteMovieVideo(ctx, pushVideo); err != nil {
		t.Fatalf("DeleteMovieVideo: %v", err)
	}
	if got := shownTitles(t, r); len(got) != 0 {
		t.Errorf("shown = %q after the delete, want none", got)
	}
}

func TestTheMovieJobRecordsEachChannelsRun(t *testing.T) {
	_, a, _, now := pushSetup(t)
	ctx := context.Background()
	channels, err := a.MovieJobChannels(ctx)
	if err != nil || len(channels) != 1 || channels[0].ChannelID != pushChannel || channels[0].BackfilledAt != nil {
		t.Fatalf("MovieJobChannels = %+v, %v; want %s, not yet filled in", channels, err, pushChannel)
	}
	if err := a.MarkMovieChannelBackfilled(ctx, pushChannel, now); err != nil {
		t.Fatalf("MarkMovieChannelBackfilled: %v", err)
	}
	failed := "the hub answered HTTP 503"
	if err := a.SetMovieChannelError(ctx, pushChannel, &failed, now); err != nil {
		t.Fatalf("SetMovieChannelError: %v", err)
	}
	state, err := a.AdminMovies(ctx)
	if err != nil {
		t.Fatalf("AdminMovies: %v", err)
	}
	if c := state.Channels[0]; c.LastError == nil || *c.LastError != failed || c.LastErrorAt == nil || !c.LastErrorAt.Equal(now) {
		t.Errorf("last error = %v at %v, want %q at %v", c.LastError, c.LastErrorAt, failed, now)
	}
	if err := a.SetMovieChannelError(ctx, pushChannel, nil, now.Add(24*time.Hour)); err != nil {
		t.Fatalf("SetMovieChannelError(nil): %v", err)
	}
	channels, _ = a.MovieJobChannels(ctx)
	state, _ = a.AdminMovies(ctx)
	if channels[0].BackfilledAt == nil || !channels[0].BackfilledAt.Equal(now) {
		t.Errorf("backfilled_at = %v, want %v", channels[0].BackfilledAt, now)
	}
	if c := state.Channels[0]; c.LastError != nil || c.LastErrorAt != nil {
		t.Errorf("last error = %v at %v after a clean run, want none", c.LastError, c.LastErrorAt)
	}
	if err := a.SetMovieChannelError(ctx, pushUnlinked, nil, now); !errors.Is(err, ErrNoChannel) {
		t.Errorf("an unlinked channel: err = %v, want %v", err, ErrNoChannel)
	}
}

func TestTheMovieJobReadsTheTournaments(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO series (slug, name) VALUES ('invented_open', 'Invented Open'), ('blank', ' ')`); err != nil {
		t.Fatalf("adding series: %v", err)
	}
	got, err := NewAdminStore(asAdminLogin(t, pool)).MovieTournamentNames(ctx)
	if err != nil {
		t.Fatalf("MovieTournamentNames: %v", err)
	}
	if !slices.Equal(got, []string{"Invented Open"}) {
		t.Errorf("MovieTournamentNames = %q, want [Invented Open]", got)
	}
}

func TestThePlayerLoginCannotSaveAVideoDirectly(t *testing.T) {
	pool := migrated(t)
	_, err := asPlayerLogin(t, pool).Exec(context.Background(),
		`SELECT save_movie_video('vid00000001', $1, 't', now(), 'x', true, 'r', NULL, now())`, pushChannel)
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "42501" {
		t.Errorf("the player login running save_movie_video: err = %v, want permission denied (42501): it stores only through store_pushed_video", err)
	}
}
