// Challenges: the committed challenge list, loaded by the load job, and the entries players made for them, imported
// once from the old site. The site reads the list and every entry not hidden; admin pages through every entry.
package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
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
	Replaced, Imported, Linked, Members int
	Refused                             []challenges.Refusal
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
// transaction, linking each to the member whose TTA username is its name, ignoring case. A name no member has gets
// one, spelt as the player of that name is when there is one and that player is no one's yet. An entry for a
// challenge not in the list is refused.
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
		tag, err = tx.Exec(ctx, `
			INSERT INTO members (tta_name, player_id)
			SELECT DISTINCT ON (lower(i.name)) coalesce(p.name, i.name), p.player_id
			FROM unnest($1::text[]) AS i(name)
			LEFT JOIN LATERAL (SELECT player_id, name FROM players WHERE lower(name) = lower(i.name)
			                   AND NOT EXISTS (SELECT FROM members WHERE members.player_id = players.player_id)
			                   ORDER BY player_id LIMIT 1) p ON true
			WHERE NOT EXISTS (SELECT FROM members m WHERE lower(m.tta_name) = lower(i.name))
			ORDER BY lower(i.name), i.name`, c.name)
		if err != nil {
			return fmt.Errorf("adding members for new names: %w", err)
		}
		got.Members = int(tag.RowsAffected())
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

// ChallengeEntriesForAdmin lists every entry, hidden ones too, newest first.
func (a *AdminStore) ChallengeEntriesForAdmin(ctx context.Context) ([]AdminChallengeEntry, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT e.entry_id, e.challenge_id, e.member_id, coalesce(m.tta_name, e.name), e.domination, coalesce(e.proof_url, ''),
		       e.bugged, e.created_at, c.name, e.name, e.account_id, e.source, e.hidden, e.hidden_reason, e.checked
		FROM challenge_entries e JOIN challenges c USING (challenge_id) LEFT JOIN members m USING (member_id)
		ORDER BY e.created_at DESC, e.entry_id DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing challenge entries: %w", err)
	}
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (AdminChallengeEntry, error) {
		var e AdminChallengeEntry
		err := row.Scan(&e.ID, &e.Challenge, &e.Member, &e.Player, &e.Domination, &e.ProofURL, &e.Bugged, &e.At,
			&e.ChallengeName, &e.EnteredAs, &e.Account, &e.Source, &e.Hidden, &e.HiddenReason, &e.Checked)
		return e, err
	})
	if err != nil {
		return nil, fmt.Errorf("listing challenge entries: %w", err)
	}
	return got, nil
}

// ErrNoChallengeEntry means no entry has the id, or no unlinked entry has the name.
var ErrNoChallengeEntry = errors.New("no such challenge entry")

// ErrBadChallengeEntry means a change the entry cannot take: a domination out of bounds, a proof that is not an https
// link of at most 500 characters, or hiding it without a reason.
var ErrBadChallengeEntry = errors.New("a domination is -9999 to 9999, a proof an https link of at most 500 characters, and hiding needs a reason")

// ChallengeEntryChange is what an admin changes on an entry; nil leaves a field as it is. An empty ProofURL removes
// the proof. Showing a hidden entry again clears its reason.
type ChallengeEntryChange struct {
	Domination              *int
	ProofURL, HiddenReason  *string
	Checked, Bugged, Hidden *bool
}

// SetChallengeEntry changes the entry id. A proof is kept as the database's challenge_proof_url writes it. It returns
// ErrNoChallengeEntry or ErrBadChallengeEntry.
func (a *AdminStore) SetChallengeEntry(ctx context.Context, id int64, c ChallengeEntryChange) error {
	switch {
	case c.Domination != nil && (*c.Domination < challenges.MinDomination || *c.Domination > challenges.MaxDomination),
		c.Hidden != nil && *c.Hidden && (c.HiddenReason == nil || strings.TrimSpace(*c.HiddenReason) == ""),
		c.HiddenReason != nil && len([]rune(*c.HiddenReason)) > 500:
		return ErrBadChallengeEntry
	}
	proof := c.ProofURL
	if proof != nil && strings.TrimSpace(*proof) != "" {
		var canonical *string
		if err := a.pool.QueryRow(ctx, "SELECT challenge_proof_url($1)", *proof).Scan(&canonical); err != nil {
			return fmt.Errorf("checking a proof: %w", err)
		}
		if canonical == nil {
			return ErrBadChallengeEntry
		}
		proof = canonical
	}
	var reason *string
	if c.HiddenReason != nil {
		r := strings.TrimSpace(*c.HiddenReason)
		reason = &r
	}
	tag, err := a.pool.Exec(ctx, `
		UPDATE challenge_entries SET domination = coalesce($2, domination),
		    proof_url = CASE WHEN $3::text IS NULL THEN proof_url ELSE nullif(btrim($3), '') END,
		    checked = coalesce($4, checked), bugged = coalesce($5, bugged), hidden = coalesce($6, hidden),
		    hidden_reason = CASE WHEN $6 IS FALSE THEN '' ELSE coalesce($7, hidden_reason) END
		WHERE entry_id = $1`, id, c.Domination, proof, c.Checked, c.Bugged, c.Hidden, reason)
	if err != nil {
		return fmt.Errorf("changing challenge entry %d: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoChallengeEntry
	}
	return nil
}

// ChallengeNameMerged is what MergeChallengeName did: how many entries and weekly placings it moved.
type ChallengeNameMerged struct {
	Moved int
}

// MergeChallengeName moves every entry and weekly placing shown under the name from (its member's TTA username, or
// the name it was entered under when unlinked), ignoring case, to the member whose TTA username is to, ignoring
// case; the names they were entered under stay. It returns ErrNoMember when no member is named to,
// ErrNoChallengeEntry when nothing is under from, and ErrBadChallengeEntry for an empty name or one over 100
// characters.
func (a *AdminStore) MergeChallengeName(ctx context.Context, from, to string) (ChallengeNameMerged, error) {
	from, to = strings.TrimSpace(from), strings.Join(strings.Fields(to), " ")
	if from == "" || to == "" || len([]rune(to)) > 100 {
		return ChallengeNameMerged{}, ErrBadChallengeEntry
	}
	var member *int64
	if err := a.pool.QueryRow(ctx, "SELECT min(member_id) FROM members WHERE lower(tta_name) = lower($1)", to).Scan(&member); err != nil {
		return ChallengeNameMerged{}, fmt.Errorf("finding the member to merge into: %w", err)
	}
	if member == nil {
		return ChallengeNameMerged{}, ErrNoMember
	}
	moved := 0
	for _, table := range []string{"challenge_entries", "challenge_week_places"} {
		tag, err := a.pool.Exec(ctx, `
			UPDATE `+table+` AS x SET member_id = $2
			WHERE (x.member_id IS NULL OR x.member_id <> $2)
			  AND lower(coalesce((SELECT m.tta_name FROM members m WHERE m.member_id = x.member_id), x.name)) = lower($1)`, from, *member)
		if err != nil {
			return ChallengeNameMerged{}, fmt.Errorf("merging a challenge name: %w", err)
		}
		moved += int(tag.RowsAffected())
	}
	if moved == 0 {
		return ChallengeNameMerged{}, ErrNoChallengeEntry
	}
	return ChallengeNameMerged{Moved: moved}, nil
}

// ReplaceChallengeWeeks replaces every week of the weekly challenge with weeks, in one transaction, linking each
// placing to the member whose TTA username is its name, ignoring case, and returns how many weeks it stored. A week
// of a challenge not in the list is refused, and so is the whole file.
func ReplaceChallengeWeeks(ctx context.Context, pool *pgxpool.Pool, weeks []challenges.Week) (int, error) {
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM challenge_weeks"); err != nil {
			return err
		}
		for _, w := range weeks {
			if _, err := tx.Exec(ctx, "INSERT INTO challenge_weeks (week, week_of, challenge_id) VALUES ($1, $2, $3)", w.Number, w.Of, w.Challenge); err != nil {
				return fmt.Errorf("week %d: %w", w.Number, err)
			}
			for place, ps := range map[string][]challenges.Placing{"winner": w.Winners, "runner_up": w.RunnersUp} {
				for i, p := range ps {
					if _, err := tx.Exec(ctx, `
						INSERT INTO challenge_week_places (week, place, position, name, member_id, domination)
						VALUES ($1, $2, $3, $4, (SELECT min(member_id) FROM members WHERE lower(tta_name) = lower($4)), $5)`,
						w.Number, place, i, p.Name, p.Domination); err != nil {
						return fmt.Errorf("week %d: %w", w.Number, err)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("replacing the challenge weeks: %w", err)
	}
	return len(weeks), nil
}

// ChallengeWeeks lists the weeks of the weekly challenge in order, each with its winners and runners-up as listed: a
// placing linked to a member is named as the member is, the rest as they were entered.
func (r *RatingsReader) ChallengeWeeks(ctx context.Context) ([]challenges.Week, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT w.week, w.week_of, w.challenge_id, p.place, coalesce(m.tta_name, p.name), p.domination
		FROM challenge_weeks w LEFT JOIN challenge_week_places p USING (week) LEFT JOIN members m ON m.member_id = p.member_id
		ORDER BY w.week, p.place DESC, p.position`)
	if err != nil {
		return nil, fmt.Errorf("listing challenge weeks: %w", err)
	}
	defer rows.Close()
	var weeks []challenges.Week
	for rows.Next() {
		var (
			w           challenges.Week
			place, name *string
			domination  *int
		)
		if err := rows.Scan(&w.Number, &w.Of, &w.Challenge, &place, &name, &domination); err != nil {
			return nil, fmt.Errorf("listing challenge weeks: %w", err)
		}
		if len(weeks) == 0 || weeks[len(weeks)-1].Number != w.Number {
			weeks = append(weeks, w)
		}
		last := &weeks[len(weeks)-1]
		switch {
		case place == nil:
		case *place == "winner":
			last.Winners = append(last.Winners, challenges.Placing{Name: *name, Domination: *domination})
		default:
			last.RunnersUp = append(last.RunnersUp, challenges.Placing{Name: *name, Domination: *domination})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing challenge weeks: %w", err)
	}
	return weeks, nil
}
