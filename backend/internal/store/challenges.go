// Challenges: the committed challenge list, loaded by the load job, and the entries players made for them, imported
// once from the old site. The site reads the list and every entry not hidden; admin pages through every entry.
package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/challenges"
)

// ErrChallengeInUse means a load would remove a challenge that has entries.
var ErrChallengeInUse = errors.New("a challenge with entries cannot be removed from the list")

// ChallengesReplaced is what ReplaceChallenges did: challenges kept (and updated), added, and removed.
type ChallengesReplaced struct {
	Kept, Added, Removed int
}

// ChallengeEntriesImported is what ImportChallengeEntries did: imported entries it replaced, entries it imported,
// how many of those it linked to a member, and those it refused.
type ChallengeEntriesImported struct {
	Replaced, Imported, Linked int
	Refused                    []challenges.Refusal
}

// ChallengeRow is a challenge as the site lists it.
type ChallengeRow struct {
	ID, Set, Group, Name      string
	Slot, Difficulty, Players int
}

// ChallengeEntry is an entry the site may show: Player is the linked member's name, or the name it was entered under.
type ChallengeEntry struct {
	ID         int64
	Challenge  string
	Member     *int64
	Player     string
	Domination int
	ProofURL   string
	Bugged     bool
	At         time.Time
}

// AdminChallengeEntry is an entry as admin sees it: also the challenge's name, the name it was entered under, the
// account that submitted it, how it came, and admins' flags.
type AdminChallengeEntry struct {
	ChallengeEntry
	ChallengeName string
	EnteredAs     string
	Account       *int64
	Source        string
	Hidden        bool
	HiddenReason  string
	Checked       bool
}

// AdminChallengeEntries is one page of entries and how many there are in all.
type AdminChallengeEntries struct {
	Total   int
	Entries []AdminChallengeEntry
}

// ReplaceChallenges makes the challenges list, in its order, in one transaction. It refuses to remove a
// challenge that has entries.
func ReplaceChallenges(ctx context.Context, pool *pgxpool.Pool, list []challenges.Challenge) (ChallengesReplaced, error) {
	var got ChallengesReplaced
	ids := make([]string, len(list))
	for i, c := range list {
		ids[i] = c.ID
	}
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT challenge_id FROM challenges")
		if err != nil {
			return err
		}
		had, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		var used string
		err = tx.QueryRow(ctx, "SELECT challenge_id FROM challenge_entries WHERE challenge_id <> ALL($1) LIMIT 1", ids).Scan(&used)
		if err == nil {
			return fmt.Errorf("challenge %q: %w", used, ErrChallengeInUse)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		for i, c := range list {
			if _, err := tx.Exec(ctx, `
				INSERT INTO challenges (challenge_id, position, challenge_set, challenge_group, slot, difficulty, players, name,
				                        description, rules, opponents)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
				ON CONFLICT (challenge_id) DO UPDATE SET position = $2, challenge_set = $3, challenge_group = $4, slot = $5,
				    difficulty = $6, players = $7, name = $8, description = $9, rules = $10, opponents = $11`,
				c.ID, i, c.Set, c.Group, c.Slot, c.Difficulty, c.Players, c.Name, nonNil(c.Description), nonNil(c.Rules),
				string(c.Opponents)); err != nil {
				return fmt.Errorf("challenge %q: %w", c.ID, err)
			}
			if slices.Contains(had, c.ID) {
				got.Kept++
			} else {
				got.Added++
			}
		}
		tag, err := tx.Exec(ctx, "DELETE FROM challenges WHERE challenge_id <> ALL($1)", ids)
		got.Removed = int(tag.RowsAffected())
		return err
	})
	if err != nil {
		return ChallengesReplaced{}, fmt.Errorf("replacing the challenges: %w", err)
	}
	return got, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ImportChallengeEntries replaces every imported entry (from the sheet or the old site) with entries, in one
// transaction, linking each to the member whose TTA username is its name, ignoring case. An entry for a challenge
// not in the list is refused.
func ImportChallengeEntries(ctx context.Context, pool *pgxpool.Pool, entries []challenges.ImportedEntry) (ChallengeEntriesImported, error) {
	var got ChallengeEntriesImported
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT challenge_id FROM challenges")
		if err != nil {
			return err
		}
		known, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		var c columns
		for _, e := range entries {
			if !slices.Contains(known, e.Challenge) {
				got.Refused = append(got.Refused, challenges.Refusal{OldID: e.OldID, Reason: fmt.Sprintf("no challenge %q", e.Challenge)})
				continue
			}
			c.add(e)
		}
		tag, err := tx.Exec(ctx, "DELETE FROM challenge_entries WHERE source IN ('sheet', 'web')")
		if err != nil {
			return err
		}
		got.Replaced = int(tag.RowsAffected())
		return tx.QueryRow(ctx, `
			WITH added AS (
			    INSERT INTO challenge_entries (challenge_id, member_id, name, domination, proof_url, created_at, source,
			                                   hidden, hidden_reason, bugged, checked)
			    SELECT i.challenge_id, m.member_id, i.name, i.domination, nullif(i.proof_url, ''), i.created_at, i.source,
			           i.hidden, i.hidden_reason, i.bugged, i.checked
			    FROM unnest($1::bigint[], $2::text[], $3::text[], $4::int[], $5::text[], $6::timestamptz[], $7::text[],
			                $8::bool[], $9::text[], $10::bool[], $11::bool[])
			         AS i(old_id, challenge_id, name, domination, proof_url, created_at, source, hidden, hidden_reason, bugged, checked)
			    LEFT JOIN members m ON lower(m.tta_name) = lower(i.name)
			    ORDER BY i.old_id
			    RETURNING member_id)
			SELECT count(*), count(member_id) FROM added`,
			c.oldID, c.challenge, c.name, c.domination, c.proof, c.at, c.source, c.hidden, c.reason, c.bugged, c.checked,
		).Scan(&got.Imported, &got.Linked)
	})
	if err != nil {
		return ChallengeEntriesImported{}, fmt.Errorf("importing challenge entries: %w", err)
	}
	return got, nil
}

// columns are entries to import, a slice per column, for one INSERT from unnest.
type columns struct {
	oldID                                  []int64
	challenge, name, proof, source, reason []string
	domination                             []int
	at                                     []time.Time
	hidden, bugged, checked                []bool
}

func (c *columns) add(e challenges.ImportedEntry) {
	c.oldID, c.challenge, c.name = append(c.oldID, e.OldID), append(c.challenge, e.Challenge), append(c.name, e.Name)
	c.domination, c.proof, c.at = append(c.domination, e.Domination), append(c.proof, e.ProofURL), append(c.at, e.At)
	c.source, c.hidden, c.reason = append(c.source, e.Source), append(c.hidden, e.Hidden), append(c.reason, e.HiddenReason)
	c.bugged, c.checked = append(c.bugged, e.Bugged), append(c.checked, e.Checked)
}

// Challenges lists the challenges in the list's order.
func (r *RatingsReader) Challenges(ctx context.Context) ([]ChallengeRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT challenge_id, challenge_set, challenge_group, name, slot, difficulty, players
		FROM challenges ORDER BY position, challenge_id`)
	if err != nil {
		return nil, fmt.Errorf("listing challenges: %w", err)
	}
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ChallengeRow, error) {
		var c ChallengeRow
		err := row.Scan(&c.ID, &c.Set, &c.Group, &c.Name, &c.Slot, &c.Difficulty, &c.Players)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("listing challenges: %w", err)
	}
	return got, nil
}

// ChallengeEntries lists every entry not hidden, in the order they were entered.
func (r *RatingsReader) ChallengeEntries(ctx context.Context) ([]ChallengeEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT e.entry_id, e.challenge_id, e.member_id, coalesce(m.tta_name, e.name), e.domination, coalesce(e.proof_url, ''),
		       e.bugged, e.created_at
		FROM challenge_entries e LEFT JOIN members m USING (member_id)
		WHERE NOT e.hidden ORDER BY e.entry_id`)
	if err != nil {
		return nil, fmt.Errorf("listing challenge entries: %w", err)
	}
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ChallengeEntry, error) {
		var e ChallengeEntry
		err := row.Scan(&e.ID, &e.Challenge, &e.Member, &e.Player, &e.Domination, &e.ProofURL, &e.Bugged, &e.At)
		return e, err
	})
	if err != nil {
		return nil, fmt.Errorf("listing challenge entries: %w", err)
	}
	return got, nil
}

// ChallengeEntryPage returns page (from 1) of every entry, size to a page, newest first, and how many there are.
func (a *AdminStore) ChallengeEntryPage(ctx context.Context, page, size int) (AdminChallengeEntries, error) {
	var got AdminChallengeEntries
	if err := a.pool.QueryRow(ctx, "SELECT count(*) FROM challenge_entries").Scan(&got.Total); err != nil {
		return AdminChallengeEntries{}, fmt.Errorf("counting challenge entries: %w", err)
	}
	rows, err := a.pool.Query(ctx, `
		SELECT e.entry_id, e.challenge_id, e.member_id, coalesce(m.tta_name, e.name), e.domination, coalesce(e.proof_url, ''),
		       e.bugged, e.created_at, c.name, e.name, e.account_id, e.source, e.hidden, e.hidden_reason, e.checked
		FROM challenge_entries e JOIN challenges c USING (challenge_id) LEFT JOIN members m USING (member_id)
		ORDER BY e.created_at DESC, e.entry_id DESC LIMIT $1 OFFSET $2`, size, (max(page, 1)-1)*size)
	if err != nil {
		return AdminChallengeEntries{}, fmt.Errorf("listing challenge entries: %w", err)
	}
	got.Entries, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (AdminChallengeEntry, error) {
		var e AdminChallengeEntry
		err := row.Scan(&e.ID, &e.Challenge, &e.Member, &e.Player, &e.Domination, &e.ProofURL, &e.Bugged, &e.At,
			&e.ChallengeName, &e.EnteredAs, &e.Account, &e.Source, &e.Hidden, &e.HiddenReason, &e.Checked)
		return e, err
	})
	if err != nil {
		return AdminChallengeEntries{}, fmt.Errorf("listing challenge entries: %w", err)
	}
	return got, nil
}
