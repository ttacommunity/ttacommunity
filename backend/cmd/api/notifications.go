// Sending notifications: the outbox posted to admin's Discord endpoints every few seconds in the background.
package main

import (
	"context"
	"log/slog"
	"time"

	"ttacommunity/backend/internal/notify"
	"ttacommunity/backend/internal/store"
)

// How often the outbox is sent, and how many notifications each send takes at most.
const (
	notifyEvery = 15 * time.Second
	notifyBatch = 10
)

// PostTestNotification posts notify.TestMessage to e.
func (d adminDeps) PostTestNotification(ctx context.Context, e store.NotificationEndpoint) error {
	return d.poster.Post(ctx, e.URL, notify.TestMessage(e))
}

// sendNotifications sends the outbox through a to its endpoints with poster, linking to admin at origin admin, every
// notifyEvery until ctx ends. Logs name a notification's kind and an endpoint's id, never a name or a URL.
func sendNotifications(ctx context.Context, a *store.AdminStore, poster *notify.Discord, admin string, log *slog.Logger) {
	for {
		done, err := a.SendNotifications(ctx, time.Now(), notifyBatch, func(ctx context.Context, n store.Notification, e store.NotificationEndpoint) error {
			err := poster.Post(ctx, e.URL, notify.Message(n, e, admin))
			if err != nil {
				log.Warn("posting a notification failed", "kind", n.Kind, "endpoint", e.ID, "err", err)
			}
			return err
		})
		if err != nil && ctx.Err() == nil {
			log.Warn("sending notifications failed", "err", err)
		}
		if done.GaveUp > 0 {
			log.Warn("notifications given up on", "count", done.GaveUp)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(notifyEvery):
		}
	}
}
