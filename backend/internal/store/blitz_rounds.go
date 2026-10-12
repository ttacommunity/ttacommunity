// Blitz rounds: a draft round made from a pairing, its matches changed by hand, published, or deleted while a
// draft; and an edition's rounds read for admin (all) and the site (published).
package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// BlitzMatch is one pairing of a round, by entrant ids; B is nil for the bye. Result is how it ended ("" for
// not yet): a, b, draw or double_loss, from the played Game (whose first seat is A) or from a Ruling, the
// organisers' reason, which may overrule a game.
type BlitzMatch struct {
	ID        int64
	A         int64
	B         *int64
	Result    string
	GameID    *int64
	Game      *BlitzGame
	Ruling    string
	DecidedAt *time.Time
}

// BlitzRound is one round of an edition: its number, kind (swiss or knockout), the criteria and seed that paired
// it, when it was published (nil for a draft) and its matches in the order made.
type BlitzRound struct {
	ID               int64
	Number           int
	Kind             string
	Timezone, Rating bool
	Seed             int64
	PublishedAt      *time.Time
	Matches          []BlitzMatch
}

// ErrNoBlitzRound means no round of the edition has that id.
var ErrNoBlitzRound = errors.New("no such round")

// ErrBlitzDraftExists means a round is still a draft: it is published or deleted before the next is made.
var ErrBlitzDraftExists = errors.New("a round is still a draft: publish or delete it first")

// ErrBlitzRoundNotLast means only the latest round can be deleted.
var ErrBlitzRoundNotLast = errors.New("only the latest round can be deleted")

// ErrBadBlitzMatch means a match names someone who is not an active entrant of the edition, or one twice.
var ErrBadBlitzMatch = errors.New("not an active entrant, or paired twice")

// ErrBlitzRepeat means a match pairs two entrants who have met in another round.
var ErrBlitzRepeat = errors.New("they have met before")

// ErrBlitzRoundIncomplete means an active entrant is in no match of the round being published.
var ErrBlitzRoundIncomplete = errors.New("not paired")

// ErrBlitzEntrantInRound means an entrant in a round cannot be removed: drop them instead.
var ErrBlitzEntrantInRound = errors.New("the entrant is in a round: drop them instead of removing them")

// blitzRounds reads an edition's rounds with their matches, every round or the published ones only.
func blitzRounds(ctx context.Context, db conn, public bool, slug, edition string) ([]BlitzRound, error) {
	rows, err := db.Query(ctx, `
		SELECT r.round_id, r.number, r.kind, r.timezone, r.rating, r.seed, r.published_at
		FROM blitz_rounds r JOIN tournaments t USING (tournament_id) JOIN series s USING (series_id)
		WHERE s.slug = $1 AND t.label = $2 AND (r.published_at IS NOT NULL OR NOT $3)
		ORDER BY r.number`, slug, edition, public)
	if err != nil {
		return nil, fmt.Errorf("reading %s %s's rounds: %w", slug, edition, err)
	}
	rounds, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (BlitzRound, error) {
		r := BlitzRound{Matches: []BlitzMatch{}}
		return r, row.Scan(&r.ID, &r.Number, &r.Kind, &r.Timezone, &r.Rating, &r.Seed, &r.PublishedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("reading %s %s's rounds: %w", slug, edition, err)
	}
	if len(rounds) == 0 {
		return []BlitzRound{}, nil
	}
	ids := make([]int64, len(rounds))
	for i, r := range rounds {
		ids[i] = r.ID
	}
	matches, err := db.Query(ctx, `
		SELECT m.round_id, m.match_id, m.entry_a, m.entry_b, coalesce(m.result, ''), m.game_id, m.ruling, m.decided_at,
		       coalesce(g.share_code, ''), g.date, sa.culture, coalesce(sa.outcome, ''), sb.culture, coalesce(sb.outcome, '')
		FROM blitz_matches m
		LEFT JOIN games g ON g.game_id = m.game_id
		LEFT JOIN seats sa ON sa.game_id = m.game_id AND sa.seat = 1
		LEFT JOIN seats sb ON sb.game_id = m.game_id AND sb.seat = 2
		WHERE m.round_id = ANY($1) ORDER BY m.match_id`, ids)
	if err != nil {
		return nil, fmt.Errorf("reading %s %s's matches: %w", slug, edition, err)
	}
	var roundID int64
	var m BlitzMatch
	var g BlitzGame
	var played *time.Time
	var cultureA, cultureB *float64
	if _, err := pgx.ForEachRow(matches, []any{&roundID, &m.ID, &m.A, &m.B, &m.Result, &m.GameID, &m.Ruling, &m.DecidedAt,
		&g.ShareCode, &played, &cultureA, &g.A.Outcome, &cultureB, &g.B.Outcome}, func() error {
		i := slices.IndexFunc(rounds, func(r BlitzRound) bool { return r.ID == roundID })
		got := BlitzMatch{ID: m.ID, A: m.A, Result: m.Result, Ruling: m.Ruling}
		if m.B != nil {
			got.B = new(*m.B)
		}
		if m.GameID != nil {
			got.GameID = new(*m.GameID)
			game := BlitzGame{ShareCode: g.ShareCode, A: BlitzSide{Outcome: g.A.Outcome}, B: BlitzSide{Outcome: g.B.Outcome}}
			if played != nil {
				game.Played = *played
			}
			if cultureA != nil && cultureB != nil {
				game.A.Culture, game.B.Culture = *cultureA, *cultureB
			}
			got.Game = &game
		}
		if m.DecidedAt != nil {
			got.DecidedAt = new(*m.DecidedAt)
		}
		rounds[i].Matches = append(rounds[i].Matches, got)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("reading %s %s's matches: %w", slug, edition, err)
	}
	return rounds, nil
}

// blitzRoundID finds a round of the edition, or returns ErrNoBlitzRound.
func blitzRoundID(ctx context.Context, tx pgx.Tx, edID, roundID int64) (published bool, err error) {
	var at *time.Time
	err = tx.QueryRow(ctx, "SELECT published_at FROM blitz_rounds WHERE round_id = $1 AND tournament_id = $2", roundID, edID).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNoBlitzRound
	}
	if err != nil {
		return false, fmt.Errorf("finding round %d: %w", roundID, err)
	}
	return at != nil, nil
}

// blitzEntrantsOf reads an edition's entrants' names by id, and which are active.
func blitzEntrantsOf(ctx context.Context, tx pgx.Tx, edID int64) (names map[int64]string, active map[int64]bool, err error) {
	rows, err := tx.Query(ctx, "SELECT x.entry_id, m.tta_name, x.status FROM blitz_entries x JOIN members m USING (member_id) WHERE x.tournament_id = $1 ORDER BY x.joined_at, x.entry_id", edID)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the entrants: %w", err)
	}
	names, active = map[int64]string{}, map[int64]bool{}
	var id int64
	var name, status string
	if _, err := pgx.ForEachRow(rows, []any{&id, &name, &status}, func() error {
		names[id], active[id] = name, status == "active"
		return nil
	}); err != nil {
		return nil, nil, fmt.Errorf("reading the entrants: %w", err)
	}
	return names, active, nil
}

// matchKey identifies a pairing by its two sides, the bye's second as 0.
func matchKey(m BlitzMatch) [2]int64 {
	if m.B == nil {
		return [2]int64{m.A, 0}
	}
	return [2]int64{m.A, *m.B}
}

// checkBlitzMatches refuses matches that name anyone but an active entrant once, or a pair met in another round
// of the edition; with complete, also a round that leaves an active entrant out.
func checkBlitzMatches(ctx context.Context, tx pgx.Tx, edID, roundID int64, matches []BlitzMatch, complete bool) error {
	names, active, err := blitzEntrantsOf(ctx, tx, edID)
	if err != nil {
		return fmt.Errorf("checking a round: %w", err)
	}
	who := func(id int64) string {
		if n, ok := names[id]; ok {
			return n
		}
		return fmt.Sprintf("entrant %d", id)
	}
	seen := map[int64]bool{}
	take := func(id int64) error {
		if !active[id] {
			return fmt.Errorf("%w: %s", ErrBadBlitzMatch, who(id))
		}
		if seen[id] {
			return fmt.Errorf("%w: %s twice", ErrBadBlitzMatch, who(id))
		}
		seen[id] = true
		return nil
	}
	met := map[[2]int64]bool{}
	pairs, err := tx.Query(ctx, `
		SELECT least(entry_a, entry_b), greatest(entry_a, entry_b) FROM blitz_matches m JOIN blitz_rounds r USING (round_id)
		WHERE r.tournament_id = $1 AND r.round_id <> $2 AND entry_b IS NOT NULL`, edID, roundID)
	if err != nil {
		return fmt.Errorf("checking a round: %w", err)
	}
	var lo, hi int64
	if _, err := pgx.ForEachRow(pairs, []any{&lo, &hi}, func() error {
		met[[2]int64{lo, hi}] = true
		return nil
	}); err != nil {
		return fmt.Errorf("checking a round: %w", err)
	}
	for _, m := range matches {
		if err := take(m.A); err != nil {
			return err
		}
		if m.B == nil {
			continue
		}
		if err := take(*m.B); err != nil {
			return err
		}
		if met[[2]int64{min(m.A, *m.B), max(m.A, *m.B)}] {
			return fmt.Errorf("%w: %s and %s", ErrBlitzRepeat, who(m.A), who(*m.B))
		}
	}
	if complete {
		var missing []string
		for id, on := range active {
			if on && !seen[id] {
				missing = append(missing, names[id])
			}
		}
		if len(missing) > 0 {
			slices.Sort(missing)
			return fmt.Errorf("%w: %s", ErrBlitzRoundIncomplete, strings.Join(missing, ", "))
		}
	}
	return nil
}

// insertBlitzMatches adds matches to a round and returns them with their ids.
func insertBlitzMatches(ctx context.Context, tx pgx.Tx, roundID int64, matches []BlitzMatch) ([]BlitzMatch, error) {
	out := make([]BlitzMatch, 0, len(matches))
	for _, m := range matches {
		var id int64
		if err := tx.QueryRow(ctx, "INSERT INTO blitz_matches (round_id, entry_a, entry_b) VALUES ($1, $2, $3) RETURNING match_id", roundID, m.A, m.B).Scan(&id); err != nil {
			return nil, fmt.Errorf("adding a match to round %d: %w", roundID, err)
		}
		out = append(out, BlitzMatch{ID: id, A: m.A, B: m.B})
	}
	return out, nil
}

// AddBlitzRound makes the edition's next round as a draft, from r's criteria, seed and matches (which may leave
// entrants unpaired until it is published). It returns the round with its ids and number.
func (a *AdminStore) AddBlitzRound(ctx context.Context, slug, edition string, r BlitzRound) (BlitzRound, error) {
	if r.Kind == "" {
		r.Kind = "swiss"
	}
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		edID, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		var draft bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM blitz_rounds WHERE tournament_id = $1 AND published_at IS NULL)", edID).Scan(&draft); err != nil {
			return fmt.Errorf("adding a round to %s %s: %w", slug, edition, err)
		}
		if draft {
			return ErrBlitzDraftExists
		}
		if err := blitzResultsIn(ctx, tx, edID); err != nil {
			return err
		}
		if err := checkBlitzMatches(ctx, tx, edID, 0, r.Matches, false); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO blitz_rounds (tournament_id, number, kind, timezone, rating, seed)
			VALUES ($1, (SELECT coalesce(max(number), 0) + 1 FROM blitz_rounds WHERE tournament_id = $1), $2, $3, $4, $5)
			RETURNING round_id, number`, edID, r.Kind, r.Timezone, r.Rating, r.Seed).Scan(&r.ID, &r.Number); err != nil {
			return fmt.Errorf("adding a round to %s %s: %w", slug, edition, err)
		}
		r.Matches, err = insertBlitzMatches(ctx, tx, r.ID, r.Matches)
		return err
	})
	if err != nil {
		return BlitzRound{}, err
	}
	return r, nil
}

// SetBlitzRoundMatches replaces a round's matches, checked as AddBlitzRound checks them. A match with a result
// stays as it is: the new matches must include it, else ErrBlitzMatchDecided names it.
func (a *AdminStore) SetBlitzRoundMatches(ctx context.Context, slug, edition string, roundID int64, matches []BlitzMatch) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		edID, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		if _, err := blitzRoundID(ctx, tx, edID, roundID); err != nil {
			return err
		}
		if err := checkBlitzMatches(ctx, tx, edID, roundID, matches, false); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, "SELECT match_id, entry_a, entry_b FROM blitz_matches WHERE round_id = $1 AND result IS NOT NULL ORDER BY match_id", roundID)
		if err != nil {
			return fmt.Errorf("replacing round %d's matches: %w", roundID, err)
		}
		decided, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (BlitzMatch, error) {
			var m BlitzMatch
			return m, row.Scan(&m.ID, &m.A, &m.B)
		})
		if err != nil {
			return fmt.Errorf("replacing round %d's matches: %w", roundID, err)
		}
		given := map[[2]int64]bool{}
		for _, m := range matches {
			given[matchKey(m)] = true
		}
		kept := map[[2]int64]bool{}
		for _, m := range decided {
			if !given[matchKey(m)] {
				names, _, err := blitzEntrantsOf(ctx, tx, edID)
				if err != nil {
					return err
				}
				return fmt.Errorf("%w: %s and %s", ErrBlitzMatchDecided, names[m.A], names[*m.B])
			}
			kept[matchKey(m)] = true
		}
		if _, err := tx.Exec(ctx, "DELETE FROM blitz_matches WHERE round_id = $1 AND result IS NULL", roundID); err != nil {
			return fmt.Errorf("replacing round %d's matches: %w", roundID, err)
		}
		var fresh []BlitzMatch
		for _, m := range matches {
			if !kept[matchKey(m)] {
				fresh = append(fresh, m)
			}
		}
		_, err = insertBlitzMatches(ctx, tx, roundID, fresh)
		return err
	})
}

// PublishBlitzRound makes a round public, once every active entrant is in it exactly once and no pair is a
// repeat. Publishing again changes nothing.
func (a *AdminStore) PublishBlitzRound(ctx context.Context, slug, edition string, roundID int64) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		edID, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		if _, err := blitzRoundID(ctx, tx, edID, roundID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, "SELECT match_id, entry_a, entry_b FROM blitz_matches WHERE round_id = $1 ORDER BY match_id", roundID)
		if err != nil {
			return fmt.Errorf("publishing round %d: %w", roundID, err)
		}
		matches, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (BlitzMatch, error) {
			var m BlitzMatch
			return m, row.Scan(&m.ID, &m.A, &m.B)
		})
		if err != nil {
			return fmt.Errorf("publishing round %d: %w", roundID, err)
		}
		if err := checkBlitzMatches(ctx, tx, edID, roundID, matches, true); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE blitz_rounds SET published_at = now() WHERE round_id = $1 AND published_at IS NULL", roundID); err != nil {
			return fmt.Errorf("publishing round %d: %w", roundID, err)
		}
		return nil
	})
}

// DeleteBlitzRound removes the edition's latest round and its matches, published or not (a wrong pairing is
// replaced before anyone plays), or returns ErrBlitzRoundNotLast for an earlier one and ErrBlitzRoundPlayed
// for one with a result.
func (a *AdminStore) DeleteBlitzRound(ctx context.Context, slug, edition string, roundID int64) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		edID, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		if _, err := blitzRoundID(ctx, tx, edID, roundID); err != nil {
			return err
		}
		var last int64
		if err := tx.QueryRow(ctx, "SELECT round_id FROM blitz_rounds WHERE tournament_id = $1 ORDER BY number DESC LIMIT 1", edID).Scan(&last); err != nil {
			return fmt.Errorf("deleting round %d: %w", roundID, err)
		}
		if last != roundID {
			return ErrBlitzRoundNotLast
		}
		var played bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM blitz_matches WHERE round_id = $1 AND result IS NOT NULL)", roundID).Scan(&played); err != nil {
			return fmt.Errorf("deleting round %d: %w", roundID, err)
		}
		if played {
			return ErrBlitzRoundPlayed
		}
		if _, err := tx.Exec(ctx, "DELETE FROM blitz_rounds WHERE round_id = $1", roundID); err != nil {
			return fmt.Errorf("deleting round %d: %w", roundID, err)
		}
		return nil
	})
}
