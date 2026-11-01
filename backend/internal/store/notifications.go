// The notification outbox: players' requests to be linked, blitz reports and flags waiting to be posted, queued in
// the same transaction as what they tell of, and sent by admin's side, each once, with later tries after a failure.
package store

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

// MaxNotificationAttempts is how many times a notification is tried before it is dropped.
const MaxNotificationAttempts = 10

// Notification is a notification as it is sent: its kind (link_request, blitz_report or blitz_flag) and what it
// tells of, read when it is sent.
type Notification struct {
	ID   int64
	Kind string
	// A request to be linked: the TTA username asked for, and how the account signs in (discord, email, …).
	Name    string
	SignIns []string
	// A blitz report or flag: the edition, round and players, the result as reported (a, b, draw or double_loss),
	// who reported it, whether they corrected an earlier report, and the opponent's reason for a flag.
	Slug, Series, Edition string
	Round                 int
	A, B, Result          string
	ReportedBy            string
	Corrected             bool
	FlagReason            string
}

// NotificationsSent counts what SendNotifications did: sent, failed (to be tried again), and given up on after
// MaxNotificationAttempts failures (counted as failed too).
type NotificationsSent struct {
	Sent, Failed, GaveUp int
}

// queueNotification puts a notification of kind in the outbox, due at now, about a link request or a blitz match.
func queueNotification(ctx context.Context, tx pgx.Tx, kind string, request, match *int64, now time.Time) error {
	if _, err := tx.Exec(ctx, `INSERT INTO notification_outbox (kind, request_id, match_id, created_at, next_at)
		VALUES ($1, $2, $3, $4, $4)`, kind, request, match, now); err != nil {
		return fmt.Errorf("queueing a notification: %w", err)
	}
	return nil
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
			SELECT o.notification_id, o.kind, coalesce(o.request_id, o.match_id), o.attempts, o.sent_to,
			       CASE o.kind WHEN 'link_request' THEN lr.status = 'pending'
			                   WHEN 'blitz_report' THEN bm.report IS NOT NULL
			                   ELSE bm.report IS NOT DISTINCT FROM 'flagged' END,
			       coalesce(lr.tta_name, ''),
			       coalesce((SELECT array_agg(DISTINCT l.provider ORDER BY l.provider) FROM logins l WHERE l.account_id = lr.account_id), '{}'),
			       coalesce(s.slug, ''), coalesce(s.name, ''), coalesce(t.label, ''), coalesce(r.number, 0),
			       coalesce(ma.tta_name, ''), coalesce(mb.tta_name, ''), coalesce(bm.result, ''), coalesce(mr.tta_name, ''),
			       coalesce((SELECT count(*) FROM admin_audit au WHERE au.action = 'blitz.match.report' AND au.target = bm.match_id::text) > 1, false),
			       coalesce(bm.flag_reason, '')
			FROM notification_outbox o
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
			if err := rows.Scan(&n.ID, &n.Kind, &d.ref, &d.attempts, &d.sentTo, &d.open, &n.Name, &n.SignIns, &n.Slug, &n.Series,
				&n.Edition, &n.Round, &n.A, &n.B, &n.Result, &n.ReportedBy, &n.Corrected, &n.FlagReason); err != nil {
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
				if !d.open || !slices.Contains(e.Events, d.n.Kind) || slices.Contains(d.sentTo, e.ID) {
					continue
				}
				if send(ctx, d.n, e) != nil {
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
	return done, nil
}
