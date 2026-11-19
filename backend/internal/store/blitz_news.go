// Blitz news for Discord: a finished edition's news queued once, and a round's games kept once posted, so a round
// paired again is posted only with other games.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// QueueFinishedBlitz queues, due at now, the news of each blitz edition with a published round that finished says
// is finished and whose finish was not queued before, marking it so it is queued once. It returns how many it
// queued. Editions another call is checking are left to it.
func (a *AdminStore) QueueFinishedBlitz(ctx context.Context, now time.Time, finished func(BlitzEdition) bool) (int, error) {
	queued := 0
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT e.tournament_id, s.slug, t.label
			FROM blitz_editions e JOIN tournaments t USING (tournament_id) JOIN series s USING (series_id)
			WHERE s.blitz AND NOT e.finish_posted
			  AND EXISTS (SELECT 1 FROM blitz_rounds r WHERE r.tournament_id = e.tournament_id AND r.published_at IS NOT NULL)
			ORDER BY e.tournament_id
			FOR UPDATE OF e SKIP LOCKED`)
		if err != nil {
			return fmt.Errorf("reading the blitz editions to check: %w", err)
		}
		type open struct {
			id          int64
			slug, label string
		}
		editions, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (open, error) {
			var o open
			return o, row.Scan(&o.id, &o.slug, &o.label)
		})
		if err != nil {
			return fmt.Errorf("reading the blitz editions to check: %w", err)
		}
		for _, o := range editions {
			ed, err := blitzEdition(ctx, tx, false, o.slug, o.label)
			if err != nil {
				return err
			}
			if !finished(ed) {
				continue
			}
			if _, err := tx.Exec(ctx, "UPDATE blitz_editions SET finish_posted = true WHERE tournament_id = $1", o.id); err != nil {
				return fmt.Errorf("marking %s %s finished: %w", o.slug, o.label, err)
			}
			if _, err := tx.Exec(ctx, "INSERT INTO notification_outbox (kind, edition_id, created_at, next_at) VALUES ('blitz_finished', $1, $2, $2)", o.id, now); err != nil {
				return fmt.Errorf("queueing %s %s's finish: %w", o.slug, o.label, err)
			}
			queued++
		}
		return nil
	})
	return queued, err
}

// keepRoundPost keeps the games of a round's notification id, once posted, as its round number's last post.
func keepRoundPost(ctx context.Context, tx pgx.Tx, id int64) error {
	if _, err := tx.Exec(ctx, `INSERT INTO blitz_round_posts (tournament_id, number, pairs)
		SELECT r.tournament_id, r.number, o.round_pairs FROM notification_outbox o JOIN blitz_rounds r USING (round_id)
		WHERE o.notification_id = $1
		ON CONFLICT (tournament_id, number) DO UPDATE SET pairs = EXCLUDED.pairs`, id); err != nil {
		return fmt.Errorf("keeping a round's post: %w", err)
	}
	return nil
}

// QueueBlitzReminders queues, due at now, a reminder of each published round of a shown blitz series for each
// endpoint taking blitz_reminder, not paused, once now is within the endpoint's hours of the round's deadline and
// before it, while a game of the round has no result: once per round and endpoint. A round published within those
// hours already had its post, and is not reminded. It returns how many it queued.
func (a *AdminStore) QueueBlitzReminders(ctx context.Context, now time.Time) (int, error) {
	tag, err := a.pool.Exec(ctx, `
		WITH due AS (
			INSERT INTO blitz_reminders (round_id, endpoint_id, queued_at)
			SELECT r.round_id, e.endpoint_id, $1
			FROM blitz_rounds r
			JOIN tournaments t ON t.tournament_id = r.tournament_id JOIN series s ON s.series_id = t.series_id
			JOIN notification_endpoints e ON 'blitz_reminder' = ANY (e.events) AND NOT e.paused
			WHERE s.visible AND r.published_at IS NOT NULL AND r.deadline > $1
			  AND r.deadline - make_interval(hours => e.blitz_reminder_hours) <= $1
			  AND r.published_at < r.deadline - make_interval(hours => e.blitz_reminder_hours)
			  AND EXISTS (SELECT 1 FROM blitz_matches m WHERE m.round_id = r.round_id AND m.entry_b IS NOT NULL AND m.result IS NULL)
			ON CONFLICT DO NOTHING
			RETURNING round_id, endpoint_id)
		INSERT INTO notification_outbox (kind, round_id, endpoint_id, created_at, next_at)
		SELECT 'blitz_reminder', round_id, endpoint_id, $1, $1 FROM due`, now)
	if err != nil {
		return 0, fmt.Errorf("queueing blitz reminders: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
