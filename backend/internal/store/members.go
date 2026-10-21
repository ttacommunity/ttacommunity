// Community members: everyone who takes part in anything we run, keyed by their TTA username, with the CGE
// account and ratings player attached as they become known, and a community ban. The load job links members to
// the accounts and players each load brings, and follows a linked account's current name.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Member is one person: the TTA username as in the app, the CGE account and ratings player when known (Player is
// the ratings' spelling), how to reach them, their country and zone, a community ban, and how many editions
// they are in.
type Member struct {
	ID                 int64
	Name               string
	CGEUser            *int
	PlayerID           *int64
	Player             string
	DiscordName, Email string
	Country            string
	Zone               string
	Banned             bool
	BanReason          string
	BannedAt           *time.Time
	Entries            int
	CreatedAt          time.Time
}

// MemberChange is what admin may change on a member; a nil field is left as it is. A CGEUser of 0 clears it.
type MemberChange struct {
	Name, DiscordName, Email, Country, Zone *string
	CGEUser                                 *int
}

// Linked is what LinkMembers did: members given a CGE account, a ratings player, or their account's current name.
type Linked struct {
	Accounts, Players, Renamed int
}

// ErrNoMember means no member has that id.
var ErrNoMember = errors.New("no such member")

// ErrBadMember means a member outside what can be stored.
var ErrBadMember = errors.New("a member has a TTA username of at most 100 characters and a time zone, a Discord name of at most 100, " +
	"an email of at most 254, a country as its two-letter ISO code (NZ), and a ban reason of at most 1000")

// ErrMemberBanned means a banned member was entered in an edition.
var ErrMemberBanned = errors.New("the member is banned")

// ErrMemberInUse means a member in an edition, as an entrant or an organiser, cannot be removed: take them out of it instead.
var ErrMemberInUse = errors.New("the member is in an edition and cannot be removed")

// ErrMemberSignedIn means a member a player's sign-in is linked to cannot be removed: unlink the sign-in first.
var ErrMemberSignedIn = errors.New("a player's sign-in is linked to the member: unlink it first")

// ErrNoCGEAccount means no CGE account the data knows has that id.
var ErrNoCGEAccount = errors.New("no CGE account has that id")

// ErrCGEAccountTaken means another member already has that CGE account.
var ErrCGEAccountTaken = errors.New("another member has that CGE account")

// MemberExistsError means a member already has that username, in any case, or that CGE account or player: the
// member's name.
type MemberExistsError struct{ Name string }

func (e *MemberExistsError) Error() string {
	return "already a member: " + e.Name
}

// memberColumns and memberFrom read a member with the ratings' spelling of their name and their entry count.
const memberColumns = `m.member_id, m.tta_name, m.cge_user, m.player_id, coalesce(p.name, ''), m.discord_name, m.email,
	coalesce(m.country_code, ''), m.zone, m.banned, m.ban_reason, m.banned_at,
	(SELECT count(*) FROM blitz_entries e WHERE e.member_id = m.member_id)::int, m.created_at`

const memberFrom = ` FROM members m LEFT JOIN players p ON p.player_id = m.player_id`

func scanMember(row pgx.CollectableRow) (Member, error) {
	var m Member
	return m, row.Scan(&m.ID, &m.Name, &m.CGEUser, &m.PlayerID, &m.Player, &m.DiscordName, &m.Email, &m.Country, &m.Zone,
		&m.Banned, &m.BanReason, &m.BannedAt, &m.Entries, &m.CreatedAt)
}

// Members lists every member, by name without case.
func (a *AdminStore) Members(ctx context.Context) ([]Member, error) {
	rows, err := a.pool.Query(ctx, "SELECT "+memberColumns+memberFrom+" ORDER BY lower(m.tta_name), m.member_id")
	if err != nil {
		return nil, fmt.Errorf("listing members: %w", err)
	}
	got, err := pgx.CollectRows(rows, scanMember)
	if err != nil {
		return nil, fmt.Errorf("listing members: %w", err)
	}
	return got, nil
}

// validMember trims a member's fields and reports whether they are within bounds. The zone's validity is the
// caller's to check: the store only needs one.
func validMember(m *Member) bool {
	m.Name, m.DiscordName, m.Email = strings.TrimSpace(m.Name), strings.TrimSpace(m.DiscordName), strings.TrimSpace(m.Email)
	m.Country, m.Zone = strings.ToUpper(strings.TrimSpace(m.Country)), strings.TrimSpace(m.Zone)
	return m.Name != "" && utf8.RuneCountInString(m.Name) <= 100 && utf8.RuneCountInString(m.DiscordName) <= 100 &&
		utf8.RuneCountInString(m.Email) <= 254 && (m.Country == "" || countries[m.Country]) && m.Zone != ""
}

// memberClash finds a member who already is the person named: by username without case, or by the CGE account
// or ratings player that name resolves to.
func memberClash(ctx context.Context, tx pgx.Tx, name string, except int64) (string, bool, error) {
	var who string
	err := tx.QueryRow(ctx, `
		SELECT m.tta_name FROM members m
		WHERE m.member_id <> $2 AND (lower(m.tta_name) = lower($1)
		   OR m.player_id = (SELECT player_id FROM players WHERE lower(name) = lower($1) ORDER BY player_id LIMIT 1)
		   OR m.cge_user = (SELECT cge_user FROM player_accounts WHERE lower(name) = lower($1) AND NOT deleted ORDER BY cge_user LIMIT 1))
		ORDER BY m.member_id LIMIT 1`, name, except).Scan(&who)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("checking for a member: %w", err)
	}
	return who, true, nil
}

// addMember adds a validated member: a name a CGE account or the ratings know takes their spelling and their
// ids. A person already a member is a *MemberExistsError.
func addMember(ctx context.Context, tx pgx.Tx, m Member) (int64, error) {
	if who, taken, err := memberClash(ctx, tx, m.Name, 0); err != nil {
		return 0, err
	} else if taken {
		return 0, &MemberExistsError{Name: who}
	}
	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO members (tta_name, cge_user, player_id, discord_name, email, country_code, zone)
		SELECT coalesce(a.name, p.name, $1), a.cge_user, p.player_id, $2, $3, nullif($4, ''), $5
		FROM (SELECT 1) x
		LEFT JOIN LATERAL (SELECT player_id, name FROM players WHERE lower(name) = lower($1) ORDER BY player_id LIMIT 1) p ON true
		LEFT JOIN LATERAL (SELECT cge_user, name FROM player_accounts WHERE lower(name) = lower($1) AND NOT deleted ORDER BY cge_user LIMIT 1) a ON true
		RETURNING member_id`, m.Name, m.DiscordName, m.Email, m.Country, m.Zone).Scan(&id); err != nil {
		return 0, fmt.Errorf("adding a member: %w", err)
	}
	return id, nil
}

// AddMember adds a member and returns their id, or returns ErrBadMember or a *MemberExistsError.
func (a *AdminStore) AddMember(ctx context.Context, m Member) (int64, error) {
	if !validMember(&m) {
		return 0, ErrBadMember
	}
	var id int64
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) (err error) {
		id, err = addMember(ctx, tx, m)
		return err
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// setMember applies a change to a member, or returns ErrNoMember, ErrBadMember, a *MemberExistsError for a name
// another member has, ErrNoCGEAccount or ErrCGEAccountTaken.
func setMember(ctx context.Context, tx pgx.Tx, id int64, c MemberChange) error {
	probe := Member{Name: "x", Zone: "x"}
	for field, given := range map[*string]*string{&probe.Name: c.Name, &probe.DiscordName: c.DiscordName, &probe.Email: c.Email, &probe.Country: c.Country, &probe.Zone: c.Zone} {
		if given != nil {
			*field = *given
		}
	}
	if !validMember(&probe) {
		return ErrBadMember
	}
	if c.Name != nil {
		c.Name = &probe.Name
		var who string
		err := tx.QueryRow(ctx, "SELECT tta_name FROM members WHERE lower(tta_name) = lower($1) AND member_id <> $2", probe.Name, id).Scan(&who)
		if err == nil {
			return &MemberExistsError{Name: who}
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("checking a member's name: %w", err)
		}
	}
	if c.Country != nil {
		c.Country = &probe.Country
	}
	if c.CGEUser != nil && *c.CGEUser != 0 {
		var player *int64
		switch err := tx.QueryRow(ctx, "SELECT player_id FROM player_accounts WHERE cge_user = $1", *c.CGEUser).Scan(&player); {
		case errors.Is(err, pgx.ErrNoRows):
			return ErrNoCGEAccount
		case err != nil:
			return fmt.Errorf("finding a CGE account: %w", err)
		}
		var taken bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM members WHERE member_id <> $1 AND (cge_user = $2 OR player_id = $3))", id, *c.CGEUser, player).Scan(&taken); err != nil {
			return fmt.Errorf("checking a CGE account: %w", err)
		}
		if taken {
			return ErrCGEAccountTaken
		}
		if _, err := tx.Exec(ctx, "UPDATE members SET cge_user = $2, player_id = $3 WHERE member_id = $1", id, *c.CGEUser, player); err != nil {
			return fmt.Errorf("linking a member's CGE account: %w", err)
		}
	} else if c.CGEUser != nil {
		if _, err := tx.Exec(ctx, "UPDATE members SET cge_user = NULL WHERE member_id = $1", id); err != nil {
			return fmt.Errorf("unlinking a member's CGE account: %w", err)
		}
	}
	tag, err := tx.Exec(ctx, `
		UPDATE members SET tta_name = coalesce($2, tta_name), discord_name = coalesce($3, discord_name), email = coalesce($4, email),
		       country_code = CASE WHEN $5::text IS NULL THEN country_code ELSE nullif($5, '') END, zone = coalesce($6, zone)
		WHERE member_id = $1`, id, c.Name, c.DiscordName, c.Email, c.Country, c.Zone)
	if err != nil {
		return fmt.Errorf("changing a member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoMember
	}
	return nil
}

// SetMember changes a member's details; see setMember for what it returns.
func (a *AdminStore) SetMember(ctx context.Context, id int64, c MemberChange) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		return setMember(ctx, tx, id, c)
	})
}

// BanMember bans a member with a reason, or returns ErrBadMember for no reason, or ErrNoMember.
func (a *AdminStore) BanMember(ctx context.Context, id int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" || utf8.RuneCountInString(reason) > 1000 {
		return ErrBadMember
	}
	tag, err := a.pool.Exec(ctx, "UPDATE members SET banned = true, ban_reason = $2, banned_at = now() WHERE member_id = $1", id, reason)
	if err != nil {
		return fmt.Errorf("banning a member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoMember
	}
	return nil
}

// UnbanMember lifts a member's ban, or returns ErrNoMember.
func (a *AdminStore) UnbanMember(ctx context.Context, id int64) error {
	tag, err := a.pool.Exec(ctx, "UPDATE members SET banned = false, ban_reason = '', banned_at = NULL WHERE member_id = $1", id)
	if err != nil {
		return fmt.Errorf("unbanning a member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoMember
	}
	return nil
}

// RemoveMember removes a member added by mistake, or returns ErrMemberInUse for one who enters or organises an edition,
// ErrMemberSignedIn for one a sign-in is linked to, or ErrNoMember.
func (a *AdminStore) RemoveMember(ctx context.Context, id int64) error {
	tag, err := a.pool.Exec(ctx, "DELETE FROM members WHERE member_id = $1", id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" { // foreign_key_violation: a sign-in, an entry or an organiser names them
		if pgErr.ConstraintName == "accounts_member_id_fkey" {
			return ErrMemberSignedIn
		}
		return ErrMemberInUse
	}
	if err != nil {
		return fmt.Errorf("removing a member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoMember
	}
	return nil
}

// LinkMembers, run by the load job after a load as the database owner, gives members without a CGE account the
// one their name has, members without a ratings player the one their account or name has, and members with an
// account that account's current name. A link another member already holds is left alone.
func LinkMembers(ctx context.Context, pool *pgxpool.Pool) (Linked, error) {
	var got Linked
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		for _, step := range []struct {
			what  string
			count *int
			query string
		}{
			{"accounts", &got.Accounts, `
				UPDATE members m SET cge_user = a.cge_user
				FROM (SELECT DISTINCT ON (lower(name)) cge_user, name FROM player_accounts WHERE NOT deleted ORDER BY lower(name), cge_user) a
				WHERE m.cge_user IS NULL AND lower(a.name) = lower(m.tta_name)
				  AND NOT EXISTS (SELECT 1 FROM members o WHERE o.cge_user = a.cge_user)`},
			{"players", &got.Players, `
				UPDATE members m SET player_id = x.player_id
				FROM (SELECT m2.member_id, coalesce(a.player_id, p.player_id) AS player_id
				      FROM members m2
				      LEFT JOIN player_accounts a ON a.cge_user = m2.cge_user
				      LEFT JOIN LATERAL (SELECT player_id FROM players WHERE lower(name) = lower(m2.tta_name) ORDER BY player_id LIMIT 1) p ON true
				      WHERE m2.player_id IS NULL) x
				WHERE m.member_id = x.member_id AND x.player_id IS NOT NULL
				  AND NOT EXISTS (SELECT 1 FROM members o WHERE o.player_id = x.player_id)`},
			{"names", &got.Renamed, `
				UPDATE members m SET tta_name = a.name
				FROM player_accounts a
				WHERE a.cge_user = m.cge_user AND NOT a.deleted AND a.name <> m.tta_name
				  AND NOT EXISTS (SELECT 1 FROM members o WHERE o.member_id <> m.member_id AND lower(o.tta_name) = lower(a.name))`},
		} {
			tag, err := tx.Exec(ctx, step.query)
			if err != nil {
				return fmt.Errorf("linking members' %s: %w", step.what, err)
			}
			*step.count = int(tag.RowsAffected())
		}
		return nil
	})
	if err != nil {
		return Linked{}, err
	}
	return got, nil
}
