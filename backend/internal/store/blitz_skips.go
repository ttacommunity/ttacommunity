// Blitz skips: an entrant sitting out the next Swiss round, kept by the round's number, so that round may be paired,
// deleted and paired again without them.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrBlitzEntrantDropped means a dropped entrant cannot skip a round: they play none.
var ErrBlitzEntrantDropped = errors.New("the entrant has dropped: reinstate them first")

// ErrBlitzSkipNotSwiss means the next round is the knockout, or there is none: only a Swiss round is skipped.
var ErrBlitzSkipNotSwiss = errors.New("only a Swiss round can be skipped, and the next round is not one")

// ErrBlitzNotSkipping means the entrant skips no round still to be published.
var ErrBlitzNotSkipping = errors.New("the entrant is not skipping the next round")

// ErrBlitzEntrantSkipping means an entrant skipping the next round cannot be dropped: reinstate them first.
var ErrBlitzEntrantSkipping = errors.New("the entrant is skipping the next round: reinstate them first")

// nextSwissRound is the number of edition edID's next round, and whether it is a draft already: the draft, else the
// one after the latest. It returns ErrBlitzSkipNotSwiss when that round is the knockout's, or there is none.
func nextSwissRound(ctx context.Context, tx pgx.Tx, edID int64) (number int, draft bool, err error) {
	var swissMade, swissRounds int
	if err := tx.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM blitz_rounds r WHERE r.tournament_id = e.tournament_id AND r.kind = 'swiss')::int, e.swiss_rounds
		FROM blitz_editions e WHERE e.tournament_id = $1`, edID).Scan(&swissMade, &swissRounds); err != nil {
		return 0, false, fmt.Errorf("finding the next round: %w", err)
	}
	var kind string
	published := true
	err = tx.QueryRow(ctx, "SELECT number, kind, published_at IS NOT NULL FROM blitz_rounds WHERE tournament_id = $1 ORDER BY number DESC LIMIT 1",
		edID).Scan(&number, &kind, &published)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, fmt.Errorf("finding the next round: %w", err)
	}
	switch {
	case !published && kind == "swiss":
		return number, true, nil
	case !published, swissMade >= swissRounds:
		return 0, false, ErrBlitzSkipNotSwiss
	}
	return number + 1, false, nil
}

// SkipBlitzRound has an entrant sit out the edition's next Swiss round and returns its number. When that round is a
// draft, they come out of their match in it, their opponent left unpaired. Skipping it again changes nothing. It
// returns ErrNoBlitzEdition, ErrNoBlitzEntrant, ErrBlitzEntrantDropped, ErrBlitzSkipNotSwiss, or ErrBlitzMatchDecided
// for one with a result in the draft.
func (a *AdminStore) SkipBlitzRound(ctx context.Context, slug, edition string, id int64) (int, error) {
	var number int
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		edID, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		var status string
		err = tx.QueryRow(ctx, "SELECT status FROM blitz_entries WHERE entry_id = $1 AND tournament_id = $2 FOR UPDATE", id, edID).Scan(&status)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return ErrNoBlitzEntrant
		case err != nil:
			return fmt.Errorf("skipping a round of %s %s: %w", slug, edition, err)
		case status == "dropped":
			return ErrBlitzEntrantDropped
		}
		var draft bool
		if number, draft, err = nextSwissRound(ctx, tx, edID); err != nil {
			return err
		}
		if draft {
			if err := leaveDraft(ctx, tx, edID, id, number); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, "INSERT INTO blitz_skips (entry_id, number) VALUES ($1, $2) ON CONFLICT DO NOTHING", id, number); err != nil {
			return fmt.Errorf("skipping round %d of %s %s: %w", number, slug, edition, err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return number, nil
}

// leaveDraft takes entrant id out of their match in the draft round number, or returns ErrBlitzMatchDecided when it
// has a result.
func leaveDraft(ctx context.Context, tx pgx.Tx, edID, id int64, number int) error {
	var decided bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM blitz_matches m JOIN blitz_rounds r USING (round_id)
		               WHERE r.tournament_id = $1 AND r.number = $3 AND $2 IN (m.entry_a, m.entry_b) AND m.result IS NOT NULL)`,
		edID, id, number).Scan(&decided); err != nil {
		return fmt.Errorf("taking entrant %d out of round %d: %w", id, number, err)
	}
	if decided {
		return fmt.Errorf("%w: they have a result in round %d", ErrBlitzMatchDecided, number)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM blitz_matches m USING blitz_rounds r
		WHERE m.round_id = r.round_id AND r.tournament_id = $1 AND r.number = $3 AND $2 IN (m.entry_a, m.entry_b)`, edID, id, number); err != nil {
		return fmt.Errorf("taking entrant %d out of round %d: %w", id, number, err)
	}
	return nil
}

// unpublishedSkip is the condition that skip s is of a round the edition $2 has not published, made or not.
const unpublishedSkip = `NOT EXISTS (SELECT 1 FROM blitz_rounds r WHERE r.tournament_id = $2 AND r.number = s.number AND r.published_at IS NOT NULL)`

// UnskipBlitzRound undoes an entrant's skip of a round not yet published: they are paired in it again (by hand, or by
// Pair, when it is a draft). It returns ErrNoBlitzEdition, ErrNoBlitzEntrant or ErrBlitzNotSkipping.
func (a *AdminStore) UnskipBlitzRound(ctx context.Context, slug, edition string, id int64) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		edID, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			DELETE FROM blitz_skips s USING blitz_entries x
			WHERE s.entry_id = x.entry_id AND x.entry_id = $1 AND x.tournament_id = $2 AND `+unpublishedSkip, id, edID)
		if err != nil {
			return fmt.Errorf("undoing a skip in %s %s: %w", slug, edition, err)
		}
		if tag.RowsAffected() > 0 {
			return nil
		}
		var known bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM blitz_entries WHERE entry_id = $1 AND tournament_id = $2)", id, edID).Scan(&known); err != nil {
			return fmt.Errorf("undoing a skip in %s %s: %w", slug, edition, err)
		}
		if !known {
			return ErrNoBlitzEntrant
		}
		return ErrBlitzNotSkipping
	})
}

// skippingNext says whether entrant id skips a round edition edID has not published.
func skippingNext(ctx context.Context, tx pgx.Tx, edID, id int64) (bool, error) {
	var skipping bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM blitz_skips s WHERE s.entry_id = $1 AND "+unpublishedSkip+")", id, edID).Scan(&skipping); err != nil {
		return false, fmt.Errorf("checking entrant %d's skips: %w", id, err)
	}
	return skipping, nil
}

// blitzSkips reads the rounds each entrant of an edition skips, in order: every one for admin, the published ones for
// the site.
func blitzSkips(ctx context.Context, db conn, public bool, slug, edition string) (map[int64][]int, error) {
	rows, err := db.Query(ctx, `
		SELECT s.entry_id, s.number FROM blitz_skips s JOIN blitz_entries x USING (entry_id)
		JOIN tournaments t ON t.tournament_id = x.tournament_id JOIN series sr ON sr.series_id = t.series_id
		WHERE sr.slug = $1 AND t.label = $2
		  AND (NOT $3 OR EXISTS (SELECT 1 FROM blitz_rounds r WHERE r.tournament_id = x.tournament_id AND r.number = s.number AND r.published_at IS NOT NULL))
		ORDER BY s.number`, slug, edition, public)
	if err != nil {
		return nil, fmt.Errorf("reading %s %s's skips: %w", slug, edition, err)
	}
	skips := map[int64][]int{}
	var id int64
	var number int
	if _, err := pgx.ForEachRow(rows, []any{&id, &number}, func() error {
		skips[id] = append(skips[id], number)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("reading %s %s's skips: %w", slug, edition, err)
	}
	return skips, nil
}
