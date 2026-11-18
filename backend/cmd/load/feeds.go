// The feeds job: on the admin login, it reads each RSS and Atom feed admin lists, carrying on past a feed that
// fails, and stops only when the database does.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ttacommunity/backend/internal/rss"
	"ttacommunity/backend/internal/store"
)

// feedsReport counts what a run did.
type feedsReport struct {
	Feeds, Read, Failed, Queued int
}

// runFeeds runs the job at now on the admin login, from RATINGS_ADMIN_DATABASE_URL, refusing a login that can
// change more than admin edits.
func runFeeds(now time.Time) error {
	url := os.Getenv("RATINGS_ADMIN_DATABASE_URL")
	if url == "" {
		return errors.New("RATINGS_ADMIN_DATABASE_URL is not set")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := store.OpenPool(ctx, url)
	if err != nil {
		return errors.New("opening the admin pool failed")
	}
	defer pool.Close()
	admin := store.NewAdminStore(pool)
	if err := admin.CheckLimits(ctx); err != nil {
		return err
	}
	safe := rss.SafeClient(http.DefaultTransport.(*http.Transport), rss.Public)
	r, err := rssJob{store: admin, rss: rss.New(safe), now: now.UTC()}.run(ctx)
	fmt.Printf("rss feeds: %d, read: %d, failed: %d; items queued: %d\n", r.Feeds, r.Read, r.Failed, r.Queued)
	return err
}
