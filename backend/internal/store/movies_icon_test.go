//go:build integration

package store

import (
	"context"
	"testing"
	"time"
)

func TestLeaderboardAndPlayerSayWhoHasVideos(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	r := NewRatingsReader(asReader(t, pool))
	a := NewAdminStore(asAdminLogin(t, pool))
	at := time.Date(2026, 11, 16, 4, 0, 0, 0, time.UTC)

	addPlayers(t, pool, "ann", "bob", "cy")
	run := addRun(t, pool, "complete", at)
	for i, name := range []string{"ann", "bob", "cy"} {
		addRating(t, pool, run, "glicko2_daneo", 0, name, ptr(i+1), float64(1900-100*i))
	}
	for _, c := range []struct {
		channel, player, video string
		tta                    bool
	}{
		{"UC0000000000000000000001", "ann", "vid00000001", true},
		{"UC0000000000000000000002", "bob", "vid00000002", false},
	} {
		player := c.player
		if _, err := a.AddMovieChannel(ctx, c.channel, "Invented Caster", &player, false, at); err != nil {
			t.Fatalf("AddMovieChannel: %v", err)
		}
		v := PushedVideo{VideoID: c.video, ChannelID: c.channel, Title: "Game 5", PublishedAt: at, ThumbnailURL: "x", TTA: c.tta, Reason: "test"}
		if err := a.SaveMovieVideo(ctx, v, at); err != nil {
			t.Fatalf("SaveMovieVideo: %v", err)
		}
	}
	want := map[string]bool{"ann": true, "bob": false, "cy": false}

	board, err := r.Leaderboard(ctx, run, "glicko2_daneo", 0)
	if err != nil {
		t.Fatalf("Leaderboard: %v", err)
	}
	for _, row := range board {
		if row.HasVideos != want[row.Name] {
			t.Errorf("leaderboard: %s has videos = %v, want %v", row.Name, row.HasVideos, want[row.Name])
		}
	}
	year, err := r.YearBoard(ctx, run, 2025, "glicko2_daneo", 0)
	if err != nil || len(year.Rows) != 3 {
		t.Fatalf("YearBoard = %d rows, %v; want 3", len(year.Rows), err)
	}
	for _, row := range year.Rows {
		if row.HasVideos != want[row.Name] {
			t.Errorf("year board: %s has videos = %v, want %v", row.Name, row.HasVideos, want[row.Name])
		}
	}
	for name, w := range want {
		p, err := r.Player(ctx, run, name)
		if err != nil {
			t.Fatalf("Player(%s): %v", name, err)
		}
		if p.HasVideos != w {
			t.Errorf("player page: %s has videos = %v, want %v (a linked channel with a video the Movies page shows)", name, p.HasVideos, w)
		}
	}
}
