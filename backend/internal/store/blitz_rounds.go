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

// BlitzMatch is one pairing of a round, by entrant ids; B is nil for the bye. Slot is a knockout match's place in
// the bracket (nil in a Swiss round): the winners of slots 2k and 2k+1 meet in slot k of the next round. SeedA
// and SeedB are a knockout match's players' seeds, their places in the Swiss standings when the knockout began;
// a drawn knockout game sends the higher seed through. Result
// is how it ended ("" for not yet): a, b, draw or double_loss, from the played Game (whose first seat is A) or
// from a Ruling, the organisers' reason, which may overrule a game. A game a player reported names its reporter's
// entry in ReportedBy, and Report says where it stands (reported, confirmed, flagged or resolved; "" for none), with
// the opponent's FlagReason.
type BlitzMatch struct {
	ID         int64
	A          int64
	B          *int64
	Slot       *int
	SeedA      *int
	SeedB      *int
	Result     string
	GameID     *int64
	Game       *BlitzGame
	Ruling     string
	DecidedAt  *time.Time
	ReportedBy *int64
	Report     string
	FlagReason string
}

// BlitzRound is one round of an edition: its number, kind (swiss or knockout), the method ("" for a knockout round)
// and seed that paired it, when it was published (nil for a draft), the deadline to play by and the zone it was set
// in (nil and "" until set) and its matches in the order made.
type BlitzRound struct {
	ID           int64
	Number       int
	Kind, Method string
	Seed         int64
	PublishedAt  *time.Time
	Deadline     *time.Time
	DeadlineZone string
	Matches      []BlitzMatch
}

// ErrNoBlitzRound means no round of the edition has that id.
var ErrNoBlitzRound = errors.New("no such round")

// ErrBlitzDraftExists means a round is still a draft: it is published or deleted before the next is made.
var ErrBlitzDraftExists = errors.New("a round is still a draft: publish or delete it first")

// ErrBlitzRoundNotLast means only the latest round can be deleted.
var ErrBlitzRoundNotLast = errors.New("only the latest round can be deleted")

// ErrBadBlitzMatch means a match names someone who does not play the round (not an entrant of the edition, dropped,
// or skipping it), or one twice.
var ErrBadBlitzMatch = errors.New("not playing this round (dropped or skipping it), or paired twice")

// ErrBlitzRepeat means a match pairs two entrants who have met in another round.
var ErrBlitzRepeat = errors.New("they have met before")

// ErrBlitzRoundIncomplete means an active entrant not skipping the round is in no match of the round being published.
var ErrBlitzRoundIncomplete = errors.New("not paired")

// ErrBlitzKnockoutFixed means a knockout round follows the bracket: it is deleted and paired again, never changed
// by hand.
var ErrBlitzKnockoutFixed = errors.New("a knockout round follows the bracket: delete it and pair again")

// ErrBlitzFinished means the edition's final has a winner: there is no next round.
var ErrBlitzFinished = errors.New("the edition is finished")

// ErrBlitzEntrantPlayed means an entrant who has played cannot be removed: drop them instead.
var ErrBlitzEntrantPlayed = errors.New("the entrant has played: drop them instead of removing them")

// ErrBlitzEntrantInKnockout means an entrant in a knockout match still to play cannot be removed: the bracket needs
// them. The round is deleted and paired again instead.
var ErrBlitzEntrantInKnockout = errors.New("the entrant is in a knockout match: delete that round to take them out")

// ErrBlitzNotKnockout means a player was replaced in a Swiss round: a Swiss round is paired again instead.
var ErrBlitzNotKnockout = errors.New("only a knockout player is replaced: pair a Swiss round again instead")

// ErrBlitzNotDropped means a player was replaced who has not dropped.
var ErrBlitzNotDropped = errors.New("only a player who dropped is replaced: drop them first")

// ErrBlitzEntrantNotPlayed means an entrant who has not played cannot be dropped: remove them instead.
var ErrBlitzEntrantNotPlayed = errors.New("the entrant has not played yet: remove them instead (unpair them first)")

// blitzRounds reads an edition's rounds with their matches, every round or the published ones only.
func blitzRounds(ctx context.Context, db conn, public bool, slug, edition string) ([]BlitzRound, error) {
	rows, err := db.Query(ctx, `
		SELECT r.round_id, r.number, r.kind, coalesce(r.method, ''), r.seed, r.published_at, r.deadline, coalesce(r.deadline_zone, '')
		FROM blitz_rounds r JOIN tournaments t USING (tournament_id) JOIN series s USING (series_id)
		WHERE s.slug = $1 AND t.label = $2 AND (r.published_at IS NOT NULL OR NOT $3)
		ORDER BY r.number`, slug, edition, public)
	if err != nil {
		return nil, fmt.Errorf("reading %s %s's rounds: %w", slug, edition, err)
	}
	rounds, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (BlitzRound, error) {
		r := BlitzRound{Matches: []BlitzMatch{}}
		return r, row.Scan(&r.ID, &r.Number, &r.Kind, &r.Method, &r.Seed, &r.PublishedAt, &r.Deadline, &r.DeadlineZone)
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
		SELECT m.round_id, m.match_id, m.entry_a, m.entry_b, m.slot, m.seed_a, m.seed_b, coalesce(m.result, ''), m.game_id, m.ruling, m.decided_at,
		       coalesce(g.share_code, ''), coalesce(g.finished_at, g.date::timestamp AT TIME ZONE 'UTC'), sa.culture, coalesce(sa.outcome, ''),
		       coalesce(sa.starting_position + 1, 0), sb.culture, coalesce(sb.outcome, ''), coalesce(sb.starting_position + 1, 0),
		       m.reported_by, coalesce(m.report, ''), m.flag_reason
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
	var finished *time.Time
	var cultureA, cultureB *float64
	if _, err := pgx.ForEachRow(matches, []any{&roundID, &m.ID, &m.A, &m.B, &m.Slot, &m.SeedA, &m.SeedB, &m.Result, &m.GameID, &m.Ruling, &m.DecidedAt,
		&g.ShareCode, &finished, &cultureA, &g.A.Outcome, &g.A.Turn, &cultureB, &g.B.Outcome, &g.B.Turn, &m.ReportedBy, &m.Report, &m.FlagReason}, func() error {
		i := slices.IndexFunc(rounds, func(r BlitzRound) bool { return r.ID == roundID })
		got := BlitzMatch{ID: m.ID, A: m.A, Result: m.Result, Ruling: m.Ruling, Report: m.Report, FlagReason: m.FlagReason}
		if m.ReportedBy != nil {
			got.ReportedBy = new(*m.ReportedBy)
		}
		if m.B != nil {
			got.B = new(*m.B)
		}
		if m.Slot != nil {
			got.Slot, got.SeedA, got.SeedB = new(*m.Slot), new(*m.SeedA), new(*m.SeedB)
		}
		if m.GameID != nil {
			got.GameID = new(*m.GameID)
			game := BlitzGame{ShareCode: g.ShareCode, A: BlitzSide{Outcome: g.A.Outcome, Turn: g.A.Turn}, B: BlitzSide{Outcome: g.B.Outcome, Turn: g.B.Turn}}
			if finished != nil {
				game.Finished = *finished
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

// blitzRoundOf finds a round of the edition and returns its kind (swiss or knockout) and number, or ErrNoBlitzRound.
func blitzRoundOf(ctx context.Context, tx pgx.Tx, edID, roundID int64) (kind string, number int, err error) {
	err = tx.QueryRow(ctx, "SELECT kind, number FROM blitz_rounds WHERE round_id = $1 AND tournament_id = $2", roundID, edID).Scan(&kind, &number)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, ErrNoBlitzRound
	}
	if err != nil {
		return "", 0, fmt.Errorf("finding round %d: %w", roundID, err)
	}
	return kind, number, nil
}

// blitzEntrantsOf reads an edition's entrants' names by id, and which play the Swiss round skipped (0 for none):
// the active ones not skipping it.
func blitzEntrantsOf(ctx context.Context, tx pgx.Tx, edID int64, skipped int) (names map[int64]string, plays map[int64]bool, err error) {
	rows, err := tx.Query(ctx, `
		SELECT x.entry_id, m.tta_name, x.status = 'active' AND NOT EXISTS (SELECT 1 FROM blitz_skips s WHERE s.entry_id = x.entry_id AND s.number = $2)
		FROM blitz_entries x JOIN members m USING (member_id) WHERE x.tournament_id = $1 ORDER BY x.joined_at, x.entry_id`, edID, skipped)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the entrants: %w", err)
	}
	names, plays = map[int64]string{}, map[int64]bool{}
	var id int64
	var name string
	var playing bool
	if _, err := pgx.ForEachRow(rows, []any{&id, &name, &playing}, func() error {
		names[id], plays[id] = name, playing
		return nil
	}); err != nil {
		return nil, nil, fmt.Errorf("reading the entrants: %w", err)
	}
	return names, plays, nil
}

// matchKey identifies a pairing by its two sides, the bye's second as 0.
func matchKey(m BlitzMatch) [2]int64 {
	if m.B == nil {
		return [2]int64{m.A, 0}
	}
	return [2]int64{m.A, *m.B}
}

// checkBlitzMatches refuses matches of round number that name anyone but an active entrant once. A Swiss round (kind
// swiss) is also refused an entrant skipping it, a pair met in another round of the edition and, with complete, an
// active entrant left out who is not skipping it; a knockout round's matches each need two players, a slot and both
// seeds, and may repeat a pairing.
func checkBlitzMatches(ctx context.Context, tx pgx.Tx, edID, roundID int64, number int, kind string, matches []BlitzMatch, complete bool) error {
	knockout := kind == "knockout"
	skipped := number
	if knockout {
		skipped = 0
	}
	names, active, err := blitzEntrantsOf(ctx, tx, edID, skipped)
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
		if knockout && (m.B == nil || m.Slot == nil || m.SeedA == nil || m.SeedB == nil) {
			return fmt.Errorf("%w: a knockout match needs two players, a slot and their seeds: %s", ErrBadBlitzMatch, who(m.A))
		}
		if m.B == nil {
			continue
		}
		if err := take(*m.B); err != nil {
			return err
		}
		if !knockout && met[[2]int64{min(m.A, *m.B), max(m.A, *m.B)}] {
			return fmt.Errorf("%w: %s and %s", ErrBlitzRepeat, who(m.A), who(*m.B))
		}
	}
	if complete && !knockout {
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
		if err := tx.QueryRow(ctx, "INSERT INTO blitz_matches (round_id, entry_a, entry_b, slot, seed_a, seed_b) VALUES ($1, $2, $3, $4, $5, $6) RETURNING match_id",
			roundID, m.A, m.B, m.Slot, m.SeedA, m.SeedB).Scan(&id); err != nil {
			return nil, fmt.Errorf("adding a match to round %d: %w", roundID, err)
		}
		out = append(out, BlitzMatch{ID: id, A: m.A, B: m.B, Slot: m.Slot, SeedA: m.SeedA, SeedB: m.SeedB})
	}
	return out, nil
}

// AddBlitzRound makes the edition's next round as a draft, from r's kind (swiss when empty), criteria, seed and
// matches (a Swiss round may leave entrants unpaired until it is published). It returns the round with its ids and
// number, or ErrBlitzDraftExists, ErrBlitzRoundUndecided, ErrBlitzFinished, ErrBadBlitzMatch or ErrBlitzRepeat.
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
		if champion, err := blitzChampion(ctx, tx, edID); err != nil {
			return err
		} else if champion != nil {
			return ErrBlitzFinished
		}
		if err := tx.QueryRow(ctx, "SELECT coalesce(max(number), 0) + 1 FROM blitz_rounds WHERE tournament_id = $1", edID).Scan(&r.Number); err != nil {
			return fmt.Errorf("adding a round to %s %s: %w", slug, edition, err)
		}
		if err := checkBlitzMatches(ctx, tx, edID, 0, r.Number, r.Kind, r.Matches, false); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO blitz_rounds (tournament_id, number, kind, method, seed) VALUES ($1, $2, $3, nullif($4, ''), $5)
			RETURNING round_id`, edID, r.Number, r.Kind, r.Method, r.Seed).Scan(&r.ID); err != nil {
			return fmt.Errorf("adding a round to %s %s: %w", slug, edition, err)
		}
		if r.Kind == "knockout" {
			// Only Swiss rounds are skipped: a skip of a round the format has since made the knockout's is void.
			if _, err := tx.Exec(ctx, `
				DELETE FROM blitz_skips s USING blitz_entries x
				WHERE s.entry_id = x.entry_id AND x.tournament_id = $1 AND s.number >= $2`, edID, r.Number); err != nil {
				return fmt.Errorf("adding a round to %s %s: %w", slug, edition, err)
			}
		}
		r.Matches, err = insertBlitzMatches(ctx, tx, r.ID, r.Matches)
		return err
	})
	if err != nil {
		return BlitzRound{}, err
	}
	return r, nil
}

// SetBlitzRoundMatches replaces a Swiss round's matches, checked as AddBlitzRound checks them. A match with a
// result stays as it is: the new matches must include it, else ErrBlitzMatchDecided names it. A knockout round
// is ErrBlitzKnockoutFixed.
func (a *AdminStore) SetBlitzRoundMatches(ctx context.Context, slug, edition string, roundID int64, matches []BlitzMatch) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		edID, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		kind, number, err := blitzRoundOf(ctx, tx, edID, roundID)
		if err != nil {
			return err
		}
		if kind == "knockout" {
			return ErrBlitzKnockoutFixed
		}
		if err := checkBlitzMatches(ctx, tx, edID, roundID, number, kind, matches, false); err != nil {
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
				names, _, err := blitzEntrantsOf(ctx, tx, edID, 0)
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

// PublishBlitzRound makes a round public: a Swiss round once every active entrant not skipping it is in it exactly
// once and no pair is a repeat; a knockout round as the bracket made it. Publishing again changes nothing.
func (a *AdminStore) PublishBlitzRound(ctx context.Context, slug, edition string, roundID int64) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		edID, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		kind, number, err := blitzRoundOf(ctx, tx, edID, roundID)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, "SELECT match_id, entry_a, entry_b, slot, seed_a, seed_b FROM blitz_matches WHERE round_id = $1 ORDER BY match_id", roundID)
		if err != nil {
			return fmt.Errorf("publishing round %d: %w", roundID, err)
		}
		matches, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (BlitzMatch, error) {
			var m BlitzMatch
			return m, row.Scan(&m.ID, &m.A, &m.B, &m.Slot, &m.SeedA, &m.SeedB)
		})
		if err != nil {
			return fmt.Errorf("publishing round %d: %w", roundID, err)
		}
		if err := checkBlitzMatches(ctx, tx, edID, roundID, number, kind, matches, true); err != nil {
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
		if _, _, err := blitzRoundOf(ctx, tx, edID, roundID); err != nil {
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
		// The starting ranks are drawn with round 1, so they go with the last round.
		if _, err := tx.Exec(ctx, `
			UPDATE blitz_entries SET starting_rank = NULL
			WHERE tournament_id = $1 AND NOT EXISTS (SELECT 1 FROM blitz_rounds WHERE tournament_id = $1)`, edID); err != nil {
			return fmt.Errorf("deleting round %d: %w", roundID, err)
		}
		return nil
	})
}

// ReplaceBlitzPlayer puts in, an active entrant not in the round, in the place of out, a player who dropped, in a
// knockout match without a result, keeping out's slot and seed. It returns ErrNoBlitzEdition, ErrNoBlitzRound,
// ErrNoBlitzMatch, ErrBlitzNotKnockout, ErrBlitzMatchDecided, ErrBlitzNotDropped, or ErrBadBlitzMatch naming
// whom it cannot take.
func (a *AdminStore) ReplaceBlitzPlayer(ctx context.Context, slug, edition string, roundID, matchID, out, in int64) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		edID, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		kind, _, err := blitzRoundOf(ctx, tx, edID, roundID)
		if err != nil {
			return err
		}
		if kind != "knockout" {
			return ErrBlitzNotKnockout
		}
		var entryA int64
		var entryB *int64
		var result *string
		err = tx.QueryRow(ctx, "SELECT entry_a, entry_b, result FROM blitz_matches WHERE match_id = $1 AND round_id = $2 FOR UPDATE",
			matchID, roundID).Scan(&entryA, &entryB, &result)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoBlitzMatch
		}
		if err != nil {
			return fmt.Errorf("replacing a player of match %d: %w", matchID, err)
		}
		if result != nil {
			return ErrBlitzMatchDecided
		}
		if out != entryA && (entryB == nil || out != *entryB) {
			return fmt.Errorf("%w: the player replaced is not in the match", ErrBadBlitzMatch)
		}
		var outStatus string
		if err := tx.QueryRow(ctx, "SELECT status FROM blitz_entries WHERE entry_id = $1", out).Scan(&outStatus); err != nil {
			return fmt.Errorf("replacing a player of match %d: %w", matchID, err)
		}
		if outStatus != "dropped" {
			return ErrBlitzNotDropped
		}
		var name string
		var takes bool
		err = tx.QueryRow(ctx, `
			SELECT m.tta_name, e.status = 'active' AND NOT EXISTS (SELECT 1 FROM blitz_matches x WHERE x.round_id = $3 AND (x.entry_a = $1 OR x.entry_b = $1))
			FROM blitz_entries e JOIN members m USING (member_id) WHERE e.entry_id = $1 AND e.tournament_id = $2`, in, edID, roundID).Scan(&name, &takes)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: entrant %d", ErrBadBlitzMatch, in)
		}
		if err != nil {
			return fmt.Errorf("replacing a player of match %d: %w", matchID, err)
		}
		if !takes {
			return fmt.Errorf("%w: %s", ErrBadBlitzMatch, name)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE blitz_matches SET entry_a = CASE WHEN entry_a = $2 THEN $3 ELSE entry_a END,
			                         entry_b = CASE WHEN entry_b = $2 THEN $3 ELSE entry_b END
			WHERE match_id = $1`, matchID, out, in); err != nil {
			return fmt.Errorf("replacing a player of match %d: %w", matchID, err)
		}
		return nil
	})
}
