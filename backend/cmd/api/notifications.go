// Sending notifications: the outbox posted to admin's Discord endpoints every few seconds in the background.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"ttacommunity/backend/internal/notify"
	"ttacommunity/backend/internal/store"
)

// How often the outbox is sent, and how many notifications each send takes at most.
const (
	notifyEvery = 15 * time.Second
	notifyBatch = 10
)

// PostTestNotification sends e notify.TestPayload.
func (d adminDeps) PostTestNotification(ctx context.Context, e store.NotificationEndpoint) error {
	return d.poster.Send(ctx, e.URL, notify.TestPayload(e))
}

// siteReader reads what the site shows: run boards and blitz editions.
type siteReader interface {
	runBoards
	blitzEditions
}

// sendNotifications queues the news of blitz editions just finished and the blitz reminders due, then sends the
// outbox through a to its endpoints with poster, linking to admin at origin admin and to the site at site, with a
// run's and a blitz edition's news read from shown (and its players' Discord ids through a), every notifyEvery
// until ctx ends. News with nothing to post is done unsent. Logs name a notification's kind and an endpoint's id,
// never a name or a URL.
func sendNotifications(ctx context.Context, a *store.AdminStore, shown siteReader, poster discordPoster, admin, site string, log *slog.Logger) {
	for {
		if _, err := a.QueueFinishedBlitz(ctx, time.Now(), editionFinished); err != nil && ctx.Err() == nil {
			log.Warn("checking for finished blitz editions failed", "err", err)
		}
		if _, err := a.QueueBlitzReminders(ctx, time.Now()); err != nil && ctx.Err() == nil {
			log.Warn("queueing blitz reminders failed", "err", err)
		}
		news := newRunNews(shown, shown, a, site)
		done, err := a.SendNotifications(ctx, time.Now(), notifyBatch, func(ctx context.Context, n store.Notification, e store.NotificationEndpoint) error {
			n, post, err := news.fill(ctx, n)
			if err != nil {
				log.Warn("reading a run's news failed", "kind", n.Kind, "err", err)
				return err
			}
			if !post {
				return nil
			}
			err = deliver(ctx, a, poster, n, e, admin, log)
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

// discordPoster posts to Discord webhooks, and edits what it posted.
type discordPoster interface {
	Send(ctx context.Context, webhook string, m notify.WebhookMessage) error
	Post(ctx context.Context, webhook string, m notify.WebhookMessage) (store.DiscordPost, error)
	Edit(ctx context.Context, e store.NotificationEndpoint, p store.DiscordPost, m notify.WebhookMessage) error
}

// deliver posts n to e with p, linking to admin at origin admin. A stream going live is posted, and its message
// kept through keeper when e edits it once the stream ends; a stream ended is that edit, and one whose post was
// deleted since is done. Keeping a message that was posted only logs a failure, so it is not posted twice.
func deliver(ctx context.Context, keeper interface {
	RecordTwitchPost(ctx context.Context, ref, endpoint int64, post store.DiscordPost) error
	RecordYouTubePost(ctx context.Context, ref, endpoint int64, post store.DiscordPost) error
}, p discordPoster, n store.Notification, e store.NotificationEndpoint, admin string, log *slog.Logger) error {
	m := notify.Payload(n, e, admin)
	switch {
	case (n.Kind == "twitch_live" || n.Kind == "youtube_live") && e.TakesWasLive(n.FeedID):
		posted, err := p.Post(ctx, e.URL, m)
		if err != nil {
			return err
		}
		keep := keeper.RecordTwitchPost
		if n.Kind == "youtube_live" {
			keep = keeper.RecordYouTubePost
		}
		if err := keep(ctx, n.StreamRef, e.ID, posted); err != nil {
			log.Warn("keeping a stream's post to edit failed", "kind", n.Kind, "endpoint", e.ID, "err", err)
		}
		return nil
	case n.Kind == "twitch_ended" || n.Kind == "youtube_ended":
		post := n.Twitch.Posts[e.ID]
		if n.Kind == "youtube_ended" {
			post = n.YouTube.Posts[e.ID]
		}
		err := p.Edit(ctx, e, post, m)
		var refused *notify.RefusedError
		if errors.As(err, &refused) && refused.Status == http.StatusNotFound {
			return nil
		}
		return err
	}
	return p.Send(ctx, e.URL, m)
}
