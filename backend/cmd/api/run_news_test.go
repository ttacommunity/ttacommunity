package main

import (
	"context"
	"slices"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

// stubBoards holds each run's board, by run, all active with enough opponents, best first.
type stubBoards struct {
	boards map[int64][]string
	reads  int
}

func (s *stubBoards) Leaderboard(_ context.Context, run int64, model string, format int) ([]store.LeaderboardRow, error) {
	s.reads++
	names, ok := s.boards[run]
	if !ok || model != "glicko2_daneo" || format != 0 {
		return nil, store.ErrRunNotFound
	}
	last := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	var rows []store.LeaderboardRow
	for i, n := range names {
		rows = append(rows, store.LeaderboardRow{Name: n, CRating: 2000 - float64(i), Opponents: 40, LastPlayed: &last})
	}
	return rows, nil
}

func (s *stubBoards) RunLastGame(context.Context, int64) (*time.Time, error) {
	last := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	return &last, nil
}

func board(names ...string) []string { return names }

func TestWithRunNews(t *testing.T) {
	ten := board("a", "b", "c", "d", "e", "f", "g", "h", "i", "j")
	tests := []struct {
		name     string
		n        store.Notification
		boards   map[int64][]string
		wantPost bool
		wantIn   []string
		wantErr  bool
	}{
		{name: "a run published", n: store.Notification{Kind: "ratings_run", Run: store.RunNotice{ID: 7, BeforeID: 6}}, wantPost: true},
		{name: "the top 10 changed", n: store.Notification{Kind: "ratings_top10", Run: store.RunNotice{ID: 7, BeforeID: 6}},
			boards: map[int64][]string{6: ten, 7: board("a", "b", "c", "d", "e", "f", "g", "h", "i", "k", "j")}, wantPost: true, wantIn: []string{"k"}},
		{name: "the top 10 the same", n: store.Notification{Kind: "ratings_top10", Run: store.RunNotice{ID: 7, BeforeID: 6}},
			boards: map[int64][]string{6: ten, 7: ten}},
		{name: "the run before deleted", n: store.Notification{Kind: "ratings_top10", Run: store.RunNotice{ID: 7}},
			boards: map[int64][]string{7: ten}},
		{name: "a board that cannot be read", n: store.Notification{Kind: "ratings_top10", Run: store.RunNotice{ID: 7, BeforeID: 6}},
			boards: map[int64][]string{7: ten}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, post, err := withRunNews(context.Background(), &stubBoards{boards: tt.boards}, "https://site.example", tt.n)
			if (err != nil) != tt.wantErr || post != tt.wantPost {
				t.Fatalf("withRunNews = post %v, %v; want post %v, error %v", post, err, tt.wantPost, tt.wantErr)
			}
			if post && got.Run.Link != "https://site.example/ratings/" {
				t.Errorf("link = %q, want the site's leaderboard", got.Run.Link)
			}
			if !slices.Equal(got.Top10.In, tt.wantIn) {
				t.Errorf("into the top 10 = %q, want %q", got.Top10.In, tt.wantIn)
			}
		})
	}
}

func TestRunNewsIsWorkedOutOncePerNotification(t *testing.T) {
	boards := &stubBoards{boards: map[int64][]string{6: board("a"), 7: board("b")}}
	news := newRunNews(boards, stubEditions{}, "https://site.example")
	n := store.Notification{ID: 1, Kind: "ratings_top10", Run: store.RunNotice{ID: 7, BeforeID: 6}}
	for range 3 {
		if _, post, err := news.fill(context.Background(), n); !post || err != nil {
			t.Fatalf("fill = post %v, %v; want a post", post, err)
		}
	}
	if boards.reads != 2 {
		t.Errorf("boards read = %d, want 2: each run's once for every endpoint", boards.reads)
	}
}
