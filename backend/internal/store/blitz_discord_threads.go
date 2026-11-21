// Blitz games' private Discord threads: a notification queued per game once its round is published with a
// deadline, per game posted when its deadline moves, and per round published again without some games given threads;
// and each thread the bot made, kept per round, pair of players and endpoint so none is made or reminded twice.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// BlitzDiscordThread is a game's thread on one endpoint: its Discord id, whether its message is posted, and the
// message's id ("" before).
type BlitzDiscordThread struct {
	ID        string
	Posted    bool
	MessageID string
}

// threadOf is the blitz_discord_threads key of match matchID and endpoint $2: its edition, round number and pair.
const threadOf = `FROM blitz_matches m JOIN blitz_rounds r USING (round_id)
	JOIN blitz_discord_threads d ON d.tournament_id = r.tournament_id AND d.number = r.number
	 AND d.entry_low = least(m.entry_a, m.entry_b) AND d.entry_high = greatest(m.entry_a, m.entry_b)
	WHERE m.match_id = $1 AND d.endpoint_id = $2 AND d.closed_at IS NULL`

// queueRoundThreads queues a blitz_thread for each game of round roundID, once it is published with a deadline,
// while an endpoint takes blitz_threads: each game neither queued nor given a thread already.
func queueRoundThreads(ctx context.Context, tx pgx.Tx, roundID int64) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO notification_outbox (kind, match_id, created_at, next_at)
		SELECT 'blitz_thread', m.match_id, now(), now()
		FROM blitz_matches m JOIN blitz_rounds r USING (round_id)
		WHERE m.round_id = $1 AND m.entry_b IS NOT NULL AND r.published_at IS NOT NULL AND r.deadline IS NOT NULL
		  AND EXISTS (SELECT 1 FROM notification_endpoints e WHERE 'blitz_threads' = ANY (e.events))
		  AND NOT EXISTS (SELECT 1 FROM notification_outbox o WHERE o.kind = 'blitz_thread' AND o.match_id = m.match_id)
		  AND NOT EXISTS (SELECT 1 FROM blitz_discord_threads d WHERE d.tournament_id = r.tournament_id AND d.number = r.number
		                  AND d.entry_low = least(m.entry_a, m.entry_b) AND d.entry_high = greatest(m.entry_a, m.entry_b) AND d.closed_at IS NULL)
		ORDER BY m.match_id`, roundID); err != nil {
		return fmt.Errorf("queueing round %d's threads: %w", roundID, err)
	}
	return nil
}

// BlitzThread returns match's thread on endpoint, nil while it has none or its thread was closed.
func (a *AdminStore) BlitzThread(ctx context.Context, match, endpoint int64) (*BlitzDiscordThread, error) {
	var t BlitzDiscordThread
	err := a.pool.QueryRow(ctx, "SELECT d.thread_id, d.posted_at IS NOT NULL, d.message_id "+threadOf, match, endpoint).Scan(&t.ID, &t.Posted, &t.MessageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading match %d's thread: %w", match, err)
	}
	return &t, nil
}

// RecordBlitzThread keeps thread, just made at now, as match's thread on endpoint, in place of a closed one.
func (a *AdminStore) RecordBlitzThread(ctx context.Context, match, endpoint int64, thread string, now time.Time) error {
	if _, err := a.pool.Exec(ctx, `
		INSERT INTO blitz_discord_threads (tournament_id, number, entry_low, entry_high, endpoint_id, thread_id, made_at)
		SELECT r.tournament_id, r.number, least(m.entry_a, m.entry_b), greatest(m.entry_a, m.entry_b), $2, $3, $4
		FROM blitz_matches m JOIN blitz_rounds r USING (round_id) WHERE m.match_id = $1 AND m.entry_b IS NOT NULL
		ON CONFLICT (tournament_id, number, entry_low, entry_high, endpoint_id) DO UPDATE
		SET thread_id = EXCLUDED.thread_id, made_at = EXCLUDED.made_at, posted_at = NULL, message_id = '', closed_at = NULL`,
		match, endpoint, thread, now); err != nil {
		return fmt.Errorf("keeping match %d's thread: %w", match, err)
	}
	return nil
}

// MarkBlitzThreadPosted keeps that match's thread on endpoint had its message, message, posted at now.
func (a *AdminStore) MarkBlitzThreadPosted(ctx context.Context, match, endpoint int64, message string, now time.Time) error {
	if _, err := a.pool.Exec(ctx, `
		UPDATE blitz_discord_threads d SET posted_at = $3, message_id = $4
		FROM blitz_matches m JOIN blitz_rounds r USING (round_id)
		WHERE m.match_id = $1 AND d.endpoint_id = $2 AND d.tournament_id = r.tournament_id AND d.number = r.number
		  AND d.entry_low = least(m.entry_a, m.entry_b) AND d.entry_high = greatest(m.entry_a, m.entry_b)`, match, endpoint, now, message); err != nil {
		return fmt.Errorf("marking match %d's thread posted: %w", match, err)
	}
	return nil
}

// queueDeadlineMoved queues a blitz_deadline for each game of round roundID whose thread is posted, unless one is
// waiting: its thread is told the new deadline.
func queueDeadlineMoved(ctx context.Context, tx pgx.Tx, roundID int64) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO notification_outbox (kind, match_id, created_at, next_at)
		SELECT 'blitz_deadline', m.match_id, now(), now()
		FROM blitz_matches m JOIN blitz_rounds r USING (round_id)
		WHERE m.round_id = $1 AND m.entry_b IS NOT NULL
		  AND EXISTS (SELECT 1 FROM blitz_discord_threads d WHERE d.tournament_id = r.tournament_id AND d.number = r.number
		              AND d.entry_low = least(m.entry_a, m.entry_b) AND d.entry_high = greatest(m.entry_a, m.entry_b)
		              AND d.posted_at IS NOT NULL AND d.closed_at IS NULL)
		  AND NOT EXISTS (SELECT 1 FROM notification_outbox o WHERE o.kind = 'blitz_deadline' AND o.match_id = m.match_id)
		ORDER BY m.match_id`, roundID); err != nil {
		return fmt.Errorf("queueing round %d's deadline notices: %w", roundID, err)
	}
	return nil
}

// replacedOf picks the threads of round $1's number not closed whose pair is no longer a game of it.
const replacedOf = `FROM blitz_rounds r JOIN blitz_discord_threads d ON d.tournament_id = r.tournament_id AND d.number = r.number
	WHERE r.round_id = $1 AND d.closed_at IS NULL
	  AND NOT EXISTS (SELECT 1 FROM blitz_matches m WHERE m.round_id = r.round_id
	                  AND least(m.entry_a, m.entry_b) = d.entry_low AND greatest(m.entry_a, m.entry_b) = d.entry_high)`

// queueReplacedThreads queues a blitz_replaced for round roundID, just published, when a thread of its number belongs
// to a game it no longer has, unless one is waiting.
func queueReplacedThreads(ctx context.Context, tx pgx.Tx, roundID int64) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO notification_outbox (kind, round_id, created_at, next_at)
		SELECT 'blitz_replaced', $1, now(), now()
		WHERE EXISTS (SELECT 1 `+replacedOf+`)
		  AND NOT EXISTS (SELECT 1 FROM notification_outbox o WHERE o.kind = 'blitz_replaced' AND o.round_id = $1)`, roundID); err != nil {
		return fmt.Errorf("queueing round %d's replaced threads: %w", roundID, err)
	}
	return nil
}

// ReplacedBlitzThreads returns the ids of endpoint's threads of round roundID's number, not closed, whose games the
// round no longer has.
func (a *AdminStore) ReplacedBlitzThreads(ctx context.Context, roundID, endpoint int64) ([]string, error) {
	rows, err := a.pool.Query(ctx, "SELECT d.thread_id "+replacedOf+" AND d.endpoint_id = $2 ORDER BY d.thread_id", roundID, endpoint)
	if err != nil {
		return nil, fmt.Errorf("reading round %d's replaced threads: %w", roundID, err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("reading round %d's replaced threads: %w", roundID, err)
	}
	return ids, nil
}

// CloseBlitzThread keeps that endpoint's thread was closed at now.
func (a *AdminStore) CloseBlitzThread(ctx context.Context, endpoint int64, thread string, now time.Time) error {
	if _, err := a.pool.Exec(ctx, "UPDATE blitz_discord_threads SET closed_at = $3 WHERE endpoint_id = $1 AND thread_id = $2", endpoint, thread, now); err != nil {
		return fmt.Errorf("closing a thread: %w", err)
	}
	return nil
}

// BlitzThreadToRemind is a game's thread to remind: its match and thread's id.
type BlitzThreadToRemind struct {
	MatchID  int64
	ThreadID string
}

// UnremindedBlitzThreads returns endpoint's posted threads of round roundID's games still without a result, open
// and not reminded yet, by match.
func (a *AdminStore) UnremindedBlitzThreads(ctx context.Context, roundID, endpoint int64) ([]BlitzThreadToRemind, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT m.match_id, d.thread_id
		FROM blitz_matches m JOIN blitz_rounds r USING (round_id)
		JOIN blitz_discord_threads d ON d.tournament_id = r.tournament_id AND d.number = r.number
		 AND d.entry_low = least(m.entry_a, m.entry_b) AND d.entry_high = greatest(m.entry_a, m.entry_b)
		WHERE m.round_id = $1 AND d.endpoint_id = $2 AND m.result IS NULL
		  AND d.posted_at IS NOT NULL AND d.closed_at IS NULL AND d.reminded_at IS NULL
		ORDER BY m.match_id`, roundID, endpoint)
	if err != nil {
		return nil, fmt.Errorf("reading round %d's threads to remind: %w", roundID, err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (BlitzThreadToRemind, error) {
		var t BlitzThreadToRemind
		return t, row.Scan(&t.MatchID, &t.ThreadID)
	})
	if err != nil {
		return nil, fmt.Errorf("reading round %d's threads to remind: %w", roundID, err)
	}
	return out, nil
}

// MarkBlitzThreadReminded keeps that endpoint's thread was reminded at now.
func (a *AdminStore) MarkBlitzThreadReminded(ctx context.Context, endpoint int64, thread string, now time.Time) error {
	if _, err := a.pool.Exec(ctx, "UPDATE blitz_discord_threads SET reminded_at = $3 WHERE endpoint_id = $1 AND thread_id = $2", endpoint, thread, now); err != nil {
		return fmt.Errorf("marking a thread reminded: %w", err)
	}
	return nil
}
