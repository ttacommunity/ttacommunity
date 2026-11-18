// A published run's news as it is sent: the link to the site's leaderboard, and for the top 10 the boards of the
// run and the one before, read as the site reads them and compared once for every endpoint.
package main

import (
	"context"
	"fmt"
	"time"

	"ttacommunity/backend/internal/leaderboardview"
	"ttacommunity/backend/internal/store"
)

// The leaderboard as it first shows: the model and format whose top 10 the news compares.
const (
	newsModel  = "glicko2_daneo"
	newsFormat = 0
)

// runBoards reads a published run's leaderboard and its last game, as the site's leaderboard does.
type runBoards interface {
	Leaderboard(ctx context.Context, run int64, model string, format int) ([]store.LeaderboardRow, error)
	RunLastGame(ctx context.Context, run int64) (*time.Time, error)
}

// runNews fills in run news for one pass of the outbox, each notification once however many endpoints take it.
type runNews struct {
	boards runBoards
	site   string
	done   map[int64]filled
}

// filled is a notification as filled in, and whether it is posted.
type filled struct {
	n    store.Notification
	post bool
}

// newRunNews fills in run news from boards, linking the leaderboard at site, the site's origin.
func newRunNews(boards runBoards, site string) *runNews {
	return &runNews{boards: boards, site: site, done: map[int64]filled{}}
}

// fill returns n with its run news filled in and whether to post it, as withRunNews does, reading each
// notification's boards once; a failure is not kept, so the next endpoint tries again.
func (r *runNews) fill(ctx context.Context, n store.Notification) (store.Notification, bool, error) {
	if f, ok := r.done[n.ID]; ok {
		return f.n, f.post, nil
	}
	got, post, err := withRunNews(ctx, r.boards, r.site, n)
	if err == nil {
		r.done[n.ID] = filled{got, post}
	}
	return got, post, err
}

// withRunNews returns n with its leaderboard link at site and, for ratings_top10, how the top 10 changed from the
// run before, and whether to post it: a top 10 that did not change, or whose run before was deleted, is not.
// Any other notification is returned as it is, to post.
func withRunNews(ctx context.Context, boards runBoards, site string, n store.Notification) (store.Notification, bool, error) {
	switch n.Kind {
	case "ratings_run":
		n.Run.Link = site + "/ratings/"
		return n, true, nil
	case "ratings_top10":
	default:
		return n, true, nil
	}
	if n.Run.BeforeID == 0 {
		return n, false, nil
	}
	n.Run.Link = site + "/ratings/"
	before, err := topTenOf(ctx, boards, n.Run.BeforeID)
	if err != nil {
		return n, false, err
	}
	after, err := topTenOf(ctx, boards, n.Run.ID)
	if err != nil {
		return n, false, err
	}
	change, changed := leaderboardview.TopTenChangeOf(before, after)
	n.Top10 = change
	return n, changed, nil
}

// topTenOf is run's top 10 as the site's leaderboard first shows it.
func topTenOf(ctx context.Context, boards runBoards, run int64) ([]string, error) {
	rows, err := boards.Leaderboard(ctx, run, newsModel, newsFormat)
	if err != nil {
		return nil, fmt.Errorf("reading run %d's board: %w", run, err)
	}
	last, err := boards.RunLastGame(ctx, run)
	if err != nil {
		return nil, fmt.Errorf("reading run %d's last game: %w", run, err)
	}
	return leaderboardview.TopTen(rows, last), nil
}
