// Player sign-in through the api's player login: an account for each player, a login for each way they sign in,
// its sessions, the account's requests to be linked to a member, and a linked member's own country and zone. Admin's
// side is in logins.go.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionLife is how long a session lasts unused; using it starts the time again.
const SessionLife = 30 * 24 * time.Hour

// sessionRenewal is how often a session in use is renewed, so that a page view is not a write.
const sessionRenewal = 24 * time.Hour

// Identity is who a provider says signed in: their id there, and their names there.
type Identity struct {
	Provider, Subject, Username, DisplayName string
}

// Account is a signed-in player as the site shows them: their account, their names where this session signed in,
// the member they are linked to, their latest request to be linked, their role in admin, if any, their country and
// zone (the member's once linked, until then those they gave, Unknown for no zone), and their member's ban and its
// reason.
type Account struct {
	ID int64
	// LoginID is the login this session signed in by.
	LoginID     int64
	Provider    string
	Username    string
	DisplayName string
	Member      *LinkedMember
	Request     *LinkRequest
	AdminRole   string
	Country     string
	Zone        string
	Banned      bool
	BanReason   string
}

// LinkedMember is the member an account is linked to: their TTA username, and the ratings' spelling when they have
// played.
type LinkedMember struct {
	ID     int64
	Name   string
	Player string
}

// LinkRequest is an account's request to be linked to a TTA username: pending, approved or refused with a reason.
type LinkRequest struct {
	Name        string
	Status      string
	Reason      string
	RequestedAt time.Time
}

// ErrNoSession means the session is unknown or has ended.
var ErrNoSession = errors.New("no such session")

// ErrAlreadyLinked means the account is linked to a member already.
var ErrAlreadyLinked = errors.New("the account is linked to a member already")

// ErrBadLinkName means a TTA username asked for is empty or over 100 characters.
var ErrBadLinkName = errors.New("a TTA username has 1 to 100 characters")

// ErrPlayerLoginTooStrong means the player login can change more than sign-in needs.
var ErrPlayerLoginTooStrong = errors.New("the player login can change more than sign-in needs")

// PlayerStore signs players in, as the ratings_player login.
type PlayerStore struct {
	pool *pgxpool.Pool
}

// NewPlayerStore returns a store on the player login's pool.
func NewPlayerStore(pool *pgxpool.Pool) *PlayerStore {
	return &PlayerStore{pool: pool}
}

// CheckLimits returns ErrPlayerLoginTooStrong if the pool's role is a superuser, can create tables, can write
// games, players, admins or more of a member than their country and zone, can read a member's email, or can delete
// a login or an account. The api calls it at start, so a misconfigured login refuses to serve.
func (s *PlayerStore) CheckLimits(ctx context.Context) error {
	var tooStrong bool
	err := s.pool.QueryRow(ctx, `
		SELECT (SELECT rolsuper FROM pg_roles WHERE rolname = current_user)
			OR has_schema_privilege('public', 'CREATE')
			OR has_table_privilege('games', 'INSERT, UPDATE, DELETE, TRUNCATE')
			OR has_table_privilege('seats', 'INSERT, UPDATE, DELETE, TRUNCATE')
			OR has_table_privilege('players', 'INSERT, UPDATE, DELETE, TRUNCATE')
			OR has_table_privilege('admins', 'INSERT, UPDATE, DELETE, TRUNCATE')
			OR has_table_privilege('members', 'INSERT, DELETE, TRUNCATE')
			OR EXISTS (SELECT FROM pg_attribute a
			           WHERE a.attrelid = 'members'::regclass AND a.attnum > 0 AND NOT a.attisdropped
			             AND a.attname NOT IN ('country_code', 'zone') AND has_column_privilege('members', a.attname, 'UPDATE'))
			OR has_column_privilege('members', 'email', 'SELECT')
			OR has_table_privilege('logins', 'DELETE, TRUNCATE')
			OR has_table_privilege('accounts', 'DELETE, TRUNCATE')`).Scan(&tooStrong)
	if err != nil {
		return fmt.Errorf("checking the player login's privileges: %w", err)
	}
	if tooStrong {
		return ErrPlayerLoginTooStrong
	}
	return nil
}

// SignIn records id's sign-in at now: a new login with an account of its own, or a known one with its names brought
// up to date, and reports whether the login is new. An account not linked yet, and not unlinked by an admin, is
// linked to the one member whose recorded Discord name is id's username (ignoring case and a leading @), when no
// other account is linked to that member. It then starts a session whose token hashes to tokenHash.
func (s *PlayerStore) SignIn(ctx context.Context, id Identity, tokenHash []byte, now time.Time) (bool, error) {
	var first bool
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		login, account, err := knownLogin(ctx, tx, id, now)
		if err != nil {
			return err
		}
		if login == 0 {
			first = true
			if login, account, err = newLogin(ctx, tx, id, now); err != nil {
				return err
			}
		}
		if id.Provider == "discord" {
			if err := linkByDiscordName(ctx, tx, account, id.Username, now); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, "INSERT INTO player_sessions (token_hash, login_id, created_at, expires_at) VALUES ($1, $2, $3, $4)",
			tokenHash, login, now, now.Add(SessionLife)); err != nil {
			return fmt.Errorf("starting a session: %w", err)
		}
		return nil
	})
	return first, err
}

// knownLogin brings the names of id's login up to date and records its sign-in at now, returning the login and its
// account, or zeros when id has not signed in before.
func knownLogin(ctx context.Context, tx pgx.Tx, id Identity, now time.Time) (login, account int64, err error) {
	err = tx.QueryRow(ctx, `
		UPDATE logins SET username = $3, display_name = $4, signed_in_at = $5
		WHERE provider = $1 AND subject = $2
		RETURNING login_id, account_id`, id.Provider, id.Subject, id.Username, id.DisplayName, now).Scan(&login, &account)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("recording a sign-in: %w", err)
	}
	return login, account, nil
}

// newLogin records id's first sign-in at now, with a new account, returning both.
func newLogin(ctx context.Context, tx pgx.Tx, id Identity, now time.Time) (login, account int64, err error) {
	if err := tx.QueryRow(ctx, "INSERT INTO accounts (created_at) VALUES ($1) RETURNING account_id", now).Scan(&account); err != nil {
		return 0, 0, fmt.Errorf("opening an account: %w", err)
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO logins (account_id, provider, subject, username, display_name, created_at, signed_in_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6) RETURNING login_id`,
		account, id.Provider, id.Subject, id.Username, id.DisplayName, now).Scan(&login)
	if err != nil {
		return 0, 0, fmt.Errorf("recording a first sign-in: %w", err)
	}
	return login, account, nil
}

// linkByDiscordName links account at now to the one member whose recorded Discord name is username, when the
// account is not linked, was not unlinked by an admin, and no other account is linked to that member.
func linkByDiscordName(ctx context.Context, tx pgx.Tx, account int64, username string, now time.Time) error {
	tag, err := tx.Exec(ctx, `
		UPDATE accounts a SET member_id = m.member_id, linked_how = 'match', linked_at = $3
		FROM (SELECT member_id FROM members
		      WHERE lower(ltrim(btrim(discord_name), '@')) = lower($2)) m
		WHERE a.account_id = $1 AND a.member_id IS NULL AND a.auto_link
		  AND (SELECT count(*) FROM members WHERE lower(ltrim(btrim(discord_name), '@')) = lower($2)) = 1
		  AND NOT EXISTS (SELECT FROM accounts o WHERE o.member_id = m.member_id)`, account, username, now)
	if err != nil {
		return fmt.Errorf("linking a sign-in by Discord name: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	return moveAccountDetails(ctx, tx, account)
}

// Session returns the account whose session hashes to tokenHash at now, renewing a session not renewed for a day.
// An unknown or ended session is ErrNoSession.
func (s *PlayerStore) Session(ctx context.Context, tokenHash []byte, now time.Time) (Account, error) {
	var l Account
	var expires time.Time
	var memberID *int64
	var member LinkedMember
	var request LinkRequest
	var requested *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT ac.account_id, l.login_id, l.provider, l.username, l.display_name, s.expires_at,
			m.member_id, coalesce(m.tta_name, ''), coalesce(p.name, ''),
			coalesce(CASE WHEN m.member_id IS NULL THEN ac.country_code ELSE m.country_code END, ''),
			CASE WHEN m.member_id IS NULL THEN coalesce(ac.zone, 'Unknown') ELSE m.zone END,
			coalesce(m.banned, false), coalesce(m.ban_reason, ''), coalesce(r.tta_name, ''), coalesce(r.status, ''), coalesce(r.reason, ''), r.requested_at,
			coalesce(a.role, '')
		FROM player_sessions s
		JOIN logins l USING (login_id)
		JOIN accounts ac USING (account_id)
		LEFT JOIN members m ON m.member_id = ac.member_id
		LEFT JOIN players p ON p.player_id = m.player_id
		LEFT JOIN LATERAL (SELECT tta_name, status, reason, requested_at FROM link_requests
		                   WHERE account_id = ac.account_id ORDER BY request_id DESC LIMIT 1) r ON true
		LEFT JOIN logins d ON d.account_id = ac.account_id AND d.provider = 'discord'
		LEFT JOIN admins a ON a.discord_id = d.subject
		WHERE s.token_hash = $1`, tokenHash).Scan(&l.ID, &l.LoginID, &l.Provider, &l.Username, &l.DisplayName, &expires,
		&memberID, &member.Name, &member.Player, &l.Country, &l.Zone, &l.Banned, &l.BanReason,
		&request.Name, &request.Status, &request.Reason, &requested, &l.AdminRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNoSession
	}
	if err != nil {
		return Account{}, fmt.Errorf("reading a session: %w", err)
	}
	if !now.Before(expires) {
		return Account{}, ErrNoSession
	}
	if expires.Before(now.Add(SessionLife - sessionRenewal)) {
		if _, err := s.pool.Exec(ctx, "UPDATE player_sessions SET expires_at = $2 WHERE token_hash = $1", tokenHash, now.Add(SessionLife)); err != nil {
			return Account{}, fmt.Errorf("renewing a session: %w", err)
		}
	}
	if memberID != nil {
		member.ID = *memberID
		l.Member = &member
	}
	if requested != nil {
		request.RequestedAt = *requested
		l.Request = &request
	}
	return l, nil
}

// SignOut ends the session that hashes to tokenHash. One already ended is no error.
func (s *PlayerStore) SignOut(ctx context.Context, tokenHash []byte) error {
	if _, err := s.pool.Exec(ctx, "DELETE FROM player_sessions WHERE token_hash = $1", tokenHash); err != nil {
		return fmt.Errorf("ending a session: %w", err)
	}
	return nil
}

// DeleteAccount deletes the account whose session hashes to tokenHash at now, with every login, session and
// request, clears its member's Discord name, country and zone, and audits it by the account's and member's ids alone.
// An unknown or ended session is ErrNoSession.
func (s *PlayerStore) DeleteAccount(ctx context.Context, tokenHash []byte, now time.Time) error {
	var deleted bool
	if err := s.pool.QueryRow(ctx, "SELECT delete_own_account($1, $2)", tokenHash, now).Scan(&deleted); err != nil {
		return fmt.Errorf("deleting a player's own account: %w", err)
	}
	if !deleted {
		return ErrNoSession
	}
	return nil
}

// RequestLink asks at now for account to be linked to the TTA username name, replacing a request still pending.
// It returns ErrBadLinkName, or ErrAlreadyLinked when the account has a member.
func (s *PlayerStore) RequestLink(ctx context.Context, account int64, name string, now time.Time) error {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return ErrBadLinkName
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var linked bool
		if err := tx.QueryRow(ctx, "SELECT member_id IS NOT NULL FROM accounts WHERE account_id = $1 FOR UPDATE", account).Scan(&linked); err != nil {
			return fmt.Errorf("reading an account: %w", err)
		}
		if linked {
			return ErrAlreadyLinked
		}
		tag, err := tx.Exec(ctx, "UPDATE link_requests SET tta_name = $2, requested_at = $3 WHERE account_id = $1 AND status = 'pending'", account, name, now)
		if err != nil {
			return fmt.Errorf("changing a request to be linked: %w", err)
		}
		if tag.RowsAffected() == 0 {
			if _, err := tx.Exec(ctx, "INSERT INTO link_requests (account_id, tta_name, requested_at) VALUES ($1, $2, $3)", account, name, now); err != nil {
				return fmt.Errorf("asking to be linked: %w", err)
			}
		}
		return nil
	})
}

// SetOwnDetails changes a player's own country and zone: their member's once linked, until then the account's,
// which a link moves to the member. nil leaves one as it is, and an empty country clears it. It returns ErrBadMember for
// a country that is not a known two-letter code or an empty zone. Whether the zone is a real one is the caller's to
// check.
func (s *PlayerStore) SetOwnDetails(ctx context.Context, account int64, country, zone *string) error {
	if country != nil {
		c := strings.ToUpper(strings.TrimSpace(*country))
		if c != "" && !countries[c] {
			return ErrBadMember
		}
		country = &c
	}
	if zone != nil {
		z := strings.TrimSpace(*zone)
		if z == "" {
			return ErrBadMember
		}
		zone = &z
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE members SET country_code = CASE WHEN $2::text IS NULL THEN country_code ELSE nullif($2, '') END,
			zone = coalesce($3, zone)
		WHERE member_id = (SELECT member_id FROM accounts WHERE account_id = $1)`, account, country, zone)
	if err != nil {
		return fmt.Errorf("changing a member's own details: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE accounts SET country_code = CASE WHEN $2::text IS NULL THEN country_code ELSE nullif($2, '') END,
			zone = coalesce($3, zone)
		WHERE account_id = $1 AND member_id IS NULL`, account, country, zone); err != nil {
		return fmt.Errorf("keeping a player's own details: %w", err)
	}
	return nil
}

// moveAccountDetails gives an account's member the country and zone the player gave before the link, replacing the
// member's where one was given, and clears them from the account.
func moveAccountDetails(ctx context.Context, c conn, account int64) error {
	if _, err := c.Exec(ctx, `
		UPDATE members m SET country_code = coalesce(a.country_code, m.country_code), zone = coalesce(a.zone, m.zone)
		FROM accounts a
		WHERE a.account_id = $1 AND m.member_id = a.member_id AND (a.country_code IS NOT NULL OR a.zone IS NOT NULL)`, account); err != nil {
		return fmt.Errorf("moving a player's details to their member: %w", err)
	}
	if _, err := c.Exec(ctx, "UPDATE accounts SET country_code = NULL, zone = NULL WHERE account_id = $1 AND member_id IS NOT NULL", account); err != nil {
		return fmt.Errorf("clearing a linked account's details: %w", err)
	}
	return nil
}
