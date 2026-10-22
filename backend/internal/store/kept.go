// What we keep about a signed-in player, all of it, for "Data we keep" on the account page: their account, each way
// in, their member once linked, their requests to be linked, their sessions and their emailed sign-in links.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// KeptData is everything we keep about one player. Member is nil until the account is linked.
type KeptData struct {
	Account    KeptAccount
	Logins     []KeptLogin
	Member     *Member
	Requests   []KeptRequest
	Sessions   []KeptSession
	EmailLinks []KeptEmailLink
}

// KeptAccount is the account itself: when it was made, the country and zone given before a link (empty once linked,
// when they move to the member), how and when it was linked, and whether a sign-in may still link it by a matching
// name or email.
type KeptAccount struct {
	CreatedAt     time.Time
	Country, Zone string
	LinkedHow     string
	LinkedAt      *time.Time
	AutoLink      bool
}

// KeptLogin is a way in: its provider, its id for the player there (an email login's is the address), its names,
// the email it gave us, and when it was added and last used.
type KeptLogin struct {
	Provider, Subject, Username, DisplayName, Email string
	CreatedAt, SignedInAt                           time.Time
}

// KeptRequest is a request to be linked: the name asked for, its answer and any reason, and when each came.
type KeptRequest struct {
	Name, Status, Reason string
	RequestedAt          time.Time
	DecidedAt            *time.Time
}

// KeptSession is a session: the way in it signed in by, when it started, when it ends and is removed, and whether it
// is the one asking.
type KeptSession struct {
	Provider    string
	CreatedAt   time.Time
	Ends        time.Time
	ThisBrowser bool
}

// KeptEmailLink is an emailed sign-in link: where it went, the page it returns to, when it was sent, until when it
// works, when it was used, and until when it is kept.
type KeptEmailLink struct {
	Address, ReturnPath  string
	CreatedAt, ExpiresAt time.Time
	UsedAt               *time.Time
	KeptUntil            time.Time
}

// KeptData returns everything kept about the player whose session hashes to tokenHash, at now, first removing the
// account's sessions and links past their time, so what it lists is exactly what is kept. An unknown or ended session
// is ErrNoSession.
func (s *PlayerStore) KeptData(ctx context.Context, tokenHash []byte, now time.Time) (KeptData, error) {
	var k KeptData
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var account int64
		var country, zone, linkedHow *string
		err := tx.QueryRow(ctx, `
			SELECT a.account_id, a.created_at, a.country_code, a.zone, a.linked_how, a.linked_at, a.auto_link
			FROM player_sessions s JOIN logins l USING (login_id) JOIN accounts a USING (account_id)
			WHERE s.token_hash = $1 AND s.expires_at > $2`, tokenHash, now).
			Scan(&account, &k.Account.CreatedAt, &country, &zone, &linkedHow, &k.Account.LinkedAt, &k.Account.AutoLink)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoSession
		}
		if err != nil {
			return fmt.Errorf("reading an account: %w", err)
		}
		k.Account.Country, k.Account.Zone, k.Account.LinkedHow = orEmpty(country), orEmpty(zone), orEmpty(linkedHow)
		if err := forgetPast(ctx, tx, account, now); err != nil {
			return err
		}
		if k.Logins, err = keptLogins(ctx, tx, account); err != nil {
			return err
		}
		if k.Member, err = ownMember(ctx, tx, tokenHash, now); err != nil {
			return err
		}
		if k.Requests, err = keptRequests(ctx, tx, account); err != nil {
			return err
		}
		if k.Sessions, err = keptSessions(ctx, tx, account, tokenHash); err != nil {
			return err
		}
		k.EmailLinks, err = keptEmailLinks(ctx, tx, account)
		return err
	})
	return k, err
}

// orEmpty is the string p points to, or "" for nil.
func orEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// forgetPast removes account's ended sessions and its emailed links older than they are kept, at now.
func forgetPast(ctx context.Context, tx pgx.Tx, account int64, now time.Time) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM player_sessions s USING logins l
		WHERE s.login_id = l.login_id AND l.account_id = $1 AND s.expires_at <= $2`, account, now); err != nil {
		return fmt.Errorf("removing ended sessions: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM email_links
		WHERE (account_id = $1 OR address IN (SELECT subject FROM logins WHERE account_id = $1 AND provider = 'email'))
		  AND created_at <= $2`, account, now.Add(-linkKeep)); err != nil {
		return fmt.Errorf("removing old links: %w", err)
	}
	return nil
}

// keptLogins is every way into account, by provider.
func keptLogins(ctx context.Context, tx pgx.Tx, account int64) ([]KeptLogin, error) {
	rows, err := tx.Query(ctx, `
		SELECT provider, subject, username, display_name, email, created_at, signed_in_at
		FROM logins WHERE account_id = $1 ORDER BY provider`, account)
	if err != nil {
		return nil, fmt.Errorf("reading ways in: %w", err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (KeptLogin, error) {
		var l KeptLogin
		err := r.Scan(&l.Provider, &l.Subject, &l.Username, &l.DisplayName, &l.Email, &l.CreatedAt, &l.SignedInAt)
		return l, err
	})
}

// ownMember is the whole member the session hashing to tokenHash is linked to at now, read through own_member, since
// the player login cannot read members' emails; nil when it is not linked.
func ownMember(ctx context.Context, tx pgx.Tx, tokenHash []byte, now time.Time) (*Member, error) {
	var m Member
	var country *string
	err := tx.QueryRow(ctx, `
		SELECT m.tta_name, m.cge_user, m.player_id, coalesce(p.name, ''), m.discord_name, m.email, m.country_code, m.zone,
			m.banned, m.ban_reason, m.banned_at, m.created_at
		FROM own_member($1, $2) m LEFT JOIN players p USING (player_id)`, tokenHash, now).
		Scan(&m.Name, &m.CGEUser, &m.PlayerID, &m.Player, &m.DiscordName, &m.Email, &country, &m.Zone,
			&m.Banned, &m.BanReason, &m.BannedAt, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the player's own member: %w", err)
	}
	m.Country = orEmpty(country)
	return &m, nil
}

// keptRequests is every request account made to be linked, latest first.
func keptRequests(ctx context.Context, tx pgx.Tx, account int64) ([]KeptRequest, error) {
	rows, err := tx.Query(ctx, `
		SELECT tta_name, status, reason, requested_at, decided_at
		FROM link_requests WHERE account_id = $1 ORDER BY request_id DESC`, account)
	if err != nil {
		return nil, fmt.Errorf("reading requests to be linked: %w", err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (KeptRequest, error) {
		var q KeptRequest
		err := r.Scan(&q.Name, &q.Status, &q.Reason, &q.RequestedAt, &q.DecidedAt)
		return q, err
	})
}

// keptSessions is account's sessions, oldest first, the one hashing to tokenHash marked.
func keptSessions(ctx context.Context, tx pgx.Tx, account int64, tokenHash []byte) ([]KeptSession, error) {
	rows, err := tx.Query(ctx, `
		SELECT l.provider, s.created_at, s.expires_at, s.token_hash = $2
		FROM player_sessions s JOIN logins l USING (login_id)
		WHERE l.account_id = $1 ORDER BY s.created_at, l.provider`, account, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("reading sessions: %w", err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (KeptSession, error) {
		var s KeptSession
		err := r.Scan(&s.Provider, &s.CreatedAt, &s.Ends, &s.ThisBrowser)
		return s, err
	})
}

// keptEmailLinks is the links sent for account or to its email login's address, oldest first.
func keptEmailLinks(ctx context.Context, tx pgx.Tx, account int64) ([]KeptEmailLink, error) {
	rows, err := tx.Query(ctx, `
		SELECT address, return_path, created_at, expires_at, used_at
		FROM email_links
		WHERE account_id = $1 OR address IN (SELECT subject FROM logins WHERE account_id = $1 AND provider = 'email')
		ORDER BY created_at`, account)
	if err != nil {
		return nil, fmt.Errorf("reading emailed links: %w", err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (KeptEmailLink, error) {
		var l KeptEmailLink
		err := r.Scan(&l.Address, &l.ReturnPath, &l.CreatedAt, &l.ExpiresAt, &l.UsedAt)
		l.KeptUntil = l.CreatedAt.Add(linkKeep)
		return l, err
	})
}
