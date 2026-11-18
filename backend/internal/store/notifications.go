// The notification outbox: players' requests to be linked, blitz reports and flags waiting to be posted, queued in
// the same transaction as what they tell of, and sent by admin's side, each once, with later tries after a failure.
package store

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

// MaxNotificationAttempts is how many times a notification is tried before it is dropped.
const MaxNotificationAttempts = 10

// Notification is a notification as it is sent: its kind (sign_up, link_request, blitz_report or blitz_flag) and
// what it tells of, read when it is sent.
type Notification struct {
	ID   int64
	Kind string
	// A first sign-in: how the account signs in (discord, email, …), and how it was linked (match, approved or "")
	// with the member's name as Name. A request to be linked: the TTA username asked for as Name, and SignIns.
	Name      string
	SignIns   []string
	LinkedHow string
	// A blitz report or flag: the edition, round and players, the result as reported (a, b, draw or double_loss),
	// who reported it, whether they corrected an earlier report, and the opponent's reason for a flag.
	Slug, Series, Edition string
	Round                 int
	A, B, Result          string
	ReportedBy            string
	Corrected             bool
	FlagReason            string
	// A Reddit post: the post and its subreddit, posted to the endpoints taking FeedID. A feed failing or reading
	// again: its subreddit, and FeedError, why its last read failed.
	FeedID    int64
	Reddit    RedditNotice
	FeedError string
	// A Twitch stream going live or ending: the stream, the twitch_streams row it is (StreamRef), and FeedID.
	StreamRef int64
	Twitch    TwitchNotice
}

// NotificationsSent counts what SendNotifications did: sent, failed (to be tried again), and given up on after
// MaxNotificationAttempts failures (counted as failed too).
type NotificationsSent struct {
	Sent, Failed, GaveUp int
}

// queueNotification puts a notification of kind in the outbox, due at now, about what ref names: a new account for
// sign_up, a link request for link_request, else a blitz match.
func queueNotification(ctx context.Context, tx pgx.Tx, kind string, ref int64, now time.Time) error {
	if _, err := tx.Exec(ctx, `INSERT INTO notification_outbox (kind, account_id, request_id, match_id, created_at, next_at)
		VALUES ($1, CASE WHEN $1 = 'sign_up' THEN $2::bigint END, CASE WHEN $1 = 'link_request' THEN $2::bigint END,
		        CASE WHEN $1 IN ('blitz_report', 'blitz_flag') THEN $2::bigint END, $3, $3)`, kind, ref, now); err != nil {
		return fmt.Errorf("queueing a notification: %w", err)
	}
	return nil
}

// delivery is how a post to an endpoint went: sent, or why not (an error that names no URL).
type delivery struct {
	endpoint int64
	err      error
}

// recordDeliveries keeps on each endpoint how its last post in deliveries went at now, endpoint by endpoint, each
// in its own statement: outside the sending transaction, so no endpoint is locked while Discord answers.
func (a *AdminStore) recordDeliveries(ctx context.Context, deliveries []delivery, now time.Time) error {
	slices.SortStableFunc(deliveries, func(x, y delivery) int { return cmp.Compare(x.endpoint, y.endpoint) })
	for _, d := range deliveries {
		var err error
		if d.err == nil {
			_, err = a.pool.Exec(ctx, "UPDATE notification_endpoints SET last_sent_at = $2, last_error = NULL, last_error_at = NULL WHERE endpoint_id = $1", d.endpoint, now)
		} else {
			msg := []rune(d.err.Error())
			_, err = a.pool.Exec(ctx, "UPDATE notification_endpoints SET last_error = $2, last_error_at = $3 WHERE endpoint_id = $1",
				d.endpoint, string(msg[:min(len(msg), maxFeedError)]), now)
		}
		if err != nil {
			return fmt.Errorf("recording how a notification went: %w", err)
		}
	}
	return nil
}

// forgetPostText clears a Reddit post's text once its notification n, about post ref, has left the outbox: what
// stays is its id and date, so it is never posted again.
func forgetPostText(ctx context.Context, tx pgx.Tx, n Notification, ref int64) error {
	if n.Kind != "reddit_post" {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE reddit_posts SET title = '', url = '', author = '', body = '', image_url = ''
		WHERE reddit_post_id = $1`, ref); err != nil {
		return fmt.Errorf("forgetting a Reddit post's text: %w", err)
	}
	return nil
}

// takes says whether e takes n: a Reddit post when e takes its feed, a feed failing or reading again when e takes
// reddit_failing, anything else when e takes its kind.
func (e NotificationEndpoint) takes(n Notification) bool {
	switch n.Kind {
	case "reddit_post":
		return slices.Contains(e.Feeds, n.FeedID)
	case "twitch_live":
		o, ok := e.options(n.FeedID)
		return ok && o.matches(n.Twitch)
	case "twitch_ended":
		_, ok := n.Twitch.Posts[e.ID]
		return ok
	case "reddit_failed", "reddit_recovered":
		return slices.Contains(e.Events, "reddit_failing")
	}
	return slices.Contains(e.Events, n.Kind)
}

// notificationRetry is how long after its attempts-th failure a notification is tried again: a minute, doubling,
// at most an hour.
func notificationRetry(attempts int) time.Duration {
	return min(time.Minute<<(attempts-1), time.Hour)
}

// SendNotifications hands each of up to limit notifications due at now to send, in the order they were queued, once
// for each endpoint taking its kind, and deletes it once every one has it; one an endpoint failed is tried again
// later, for the endpoints that failed. One no endpoint takes, whose request was answered, or whose report was
// removed or its flag resolved, is dropped unsent. Notifications another call is sending are left to it.
func (a *AdminStore) SendNotifications(ctx context.Context, now time.Time, limit int,
	send func(context.Context, Notification, NotificationEndpoint) error) (NotificationsSent, error) {
	var done NotificationsSent
	var deliveries []delivery
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT "+endpointColumns+" FROM notification_endpoints ORDER BY endpoint_id")
		if err != nil {
			return fmt.Errorf("reading the notification endpoints: %w", err)
		}
		endpoints, err := pgx.CollectRows(rows, scanEndpoint)
		if err != nil {
			return fmt.Errorf("reading the notification endpoints: %w", err)
		}
		rows, err = tx.Query(ctx, `
			SELECT o.notification_id, o.kind, coalesce(o.account_id, o.request_id, o.match_id, o.reddit_post_id, o.feed_id, o.twitch_stream_id), o.attempts, o.sent_to,
			       CASE WHEN o.kind IN ('sign_up', 'reddit_post', 'reddit_failed', 'reddit_recovered', 'twitch_live', 'twitch_ended') THEN true
			            WHEN o.kind = 'link_request' THEN lr.status = 'pending'
			            WHEN o.kind = 'blitz_report' THEN bm.report IS NOT NULL
			            ELSE bm.report IS NOT DISTINCT FROM 'flagged' END,
			       coalesce(lr.tta_name, am.tta_name, ''),
			       coalesce((SELECT array_agg(DISTINCT l.provider ORDER BY l.provider) FROM logins l
			                 WHERE l.account_id = coalesce(o.account_id, lr.account_id)), '{}'),
			       coalesce(ac.linked_how, ''),
			       coalesce(s.slug, ''), coalesce(s.name, ''), coalesce(t.label, ''), coalesce(r.number, 0),
			       coalesce(ma.tta_name, ''), coalesce(mb.tta_name, ''), coalesce(bm.result, ''), coalesce(mr.tta_name, ''),
			       coalesce((SELECT count(*) FROM admin_audit au WHERE au.action = 'blitz.match.report' AND au.target = bm.match_id::text) > 1, false),
			       coalesce(bm.flag_reason, ''),
			       coalesce(rp.feed_id, o.feed_id, ts.feed_id, 0), coalesce(rf.source, ff.source, ''), coalesce(rp.post_id, ''), coalesce(rp.kind, ''),
			       coalesce(rp.title, ''), coalesce(rp.url, ''), coalesce(rp.author, ''), coalesce(rp.body, ''),
			       coalesce(rp.image_url, ''), coalesce(rp.published_at, o.created_at), coalesce(ff.last_error, ''),
			       coalesce(o.twitch_stream_id, 0), coalesce(tf.source, ''), coalesce(tf.display_name, ''), coalesce(ts.title, ''),
			       coalesce(ts.category, ''), coalesce(ts.thumbnail_url, ''), coalesce(ts.started_at, o.created_at), ts.ended_at,
			       coalesce((SELECT json_object_agg(tp.endpoint_id, json_build_object('message_id', tp.message_id, 'channel_id', tp.channel_id))
			                 FROM twitch_posts tp WHERE tp.twitch_stream_id = o.twitch_stream_id), '{}')
			FROM notification_outbox o
			LEFT JOIN twitch_streams ts ON ts.twitch_stream_id = o.twitch_stream_id LEFT JOIN feeds tf ON tf.feed_id = ts.feed_id
			LEFT JOIN reddit_posts rp ON rp.reddit_post_id = o.reddit_post_id LEFT JOIN feeds rf ON rf.feed_id = rp.feed_id
			LEFT JOIN feeds ff ON ff.feed_id = o.feed_id
			LEFT JOIN accounts ac ON ac.account_id = o.account_id LEFT JOIN members am ON am.member_id = ac.member_id
			LEFT JOIN link_requests lr ON lr.request_id = o.request_id
			LEFT JOIN blitz_matches bm ON bm.match_id = o.match_id
			LEFT JOIN blitz_rounds r ON r.round_id = bm.round_id
			LEFT JOIN tournaments t ON t.tournament_id = r.tournament_id
			LEFT JOIN series s ON s.series_id = t.series_id
			LEFT JOIN blitz_entries xa ON xa.entry_id = bm.entry_a LEFT JOIN members ma ON ma.member_id = xa.member_id
			LEFT JOIN blitz_entries xb ON xb.entry_id = bm.entry_b LEFT JOIN members mb ON mb.member_id = xb.member_id
			LEFT JOIN blitz_entries xr ON xr.entry_id = bm.reported_by LEFT JOIN members mr ON mr.member_id = xr.member_id
			WHERE o.next_at <= $1
			ORDER BY o.notification_id
			LIMIT $2
			FOR UPDATE OF o SKIP LOCKED`, now, limit)
		if err != nil {
			return fmt.Errorf("reading the notifications due: %w", err)
		}
		type due struct {
			n        Notification
			ref      int64
			attempts int
			sentTo   []int64
			open     bool
		}
		var all []due
		for rows.Next() {
			var d due
			n := &d.n
			if err := rows.Scan(&n.ID, &n.Kind, &d.ref, &d.attempts, &d.sentTo, &d.open, &n.Name, &n.SignIns, &n.LinkedHow, &n.Slug, &n.Series,
				&n.Edition, &n.Round, &n.A, &n.B, &n.Result, &n.ReportedBy, &n.Corrected, &n.FlagReason,
				&n.FeedID, &n.Reddit.Subreddit, &n.Reddit.Post.PostID, &n.Reddit.Post.Kind, &n.Reddit.Post.Title, &n.Reddit.Post.URL,
				&n.Reddit.Post.Author, &n.Reddit.Post.Body, &n.Reddit.Post.ImageURL, &n.Reddit.Post.PublishedAt, &n.FeedError,
				&n.StreamRef, &n.Twitch.Login, &n.Twitch.DisplayName, &n.Twitch.Title, &n.Twitch.Category, &n.Twitch.ThumbnailURL,
				&n.Twitch.StartedAt, &n.Twitch.EndedAt, &n.Twitch.Posts); err != nil {
				rows.Close()
				return fmt.Errorf("reading a notification: %w", err)
			}
			all = append(all, d)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("reading the notifications due: %w", err)
		}
		// A repeat of a notification in this batch tells nothing more, being read from the same data.
		seen := map[string]bool{}
		for _, d := range all {
			key := fmt.Sprint(d.n.Kind, d.ref)
			if seen[key] {
				if _, err := tx.Exec(ctx, "DELETE FROM notification_outbox WHERE notification_id = $1", d.n.ID); err != nil {
					return fmt.Errorf("deleting a repeated notification: %w", err)
				}
				continue
			}
			seen[key] = true
			failed := false
			for _, e := range endpoints {
				if !d.open || e.Paused || !e.takes(d.n) || slices.Contains(d.sentTo, e.ID) {
					continue
				}
				sendErr := send(ctx, d.n, e)
				deliveries = append(deliveries, delivery{e.ID, sendErr})
				if sendErr != nil {
					failed = true
					continue
				}
				d.sentTo = append(d.sentTo, e.ID)
			}
			attempts := d.attempts + 1
			if failed {
				done.Failed++
			} else if d.open && len(d.sentTo) > 0 {
				done.Sent++
			}
			if failed && attempts >= MaxNotificationAttempts {
				done.GaveUp++
			}
			if !failed || attempts >= MaxNotificationAttempts {
				if _, err := tx.Exec(ctx, "DELETE FROM notification_outbox WHERE notification_id = $1", d.n.ID); err != nil {
					return fmt.Errorf("deleting a notification: %w", err)
				}
				if err := forgetPostText(ctx, tx, d.n, d.ref); err != nil {
					return err
				}
				continue
			}
			if _, err := tx.Exec(ctx, "UPDATE notification_outbox SET attempts = $2, next_at = $3, sent_to = $4 WHERE notification_id = $1",
				d.n.ID, attempts, now.Add(notificationRetry(attempts)), d.sentTo); err != nil {
				return fmt.Errorf("putting a notification off: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return NotificationsSent{}, err
	}
	return done, a.recordDeliveries(ctx, deliveries, now)
}
