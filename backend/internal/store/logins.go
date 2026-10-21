// Admin's side of player sign-in: the accounts with their ways in and pending requests, approving or refusing a
// request, and unlinking or removing an account. The player's side is in players.go.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"ttacommunity/backend/internal/blitz"
)

// AccountRow is an account as admin lists it: its ways in, the member it is linked to and how, when it last signed
// in, and its pending request or a refusal nothing has followed.
type AccountRow struct {
	ID         int64
	Logins     []AccountLogin
	MemberID   *int64
	Member     string
	LinkedHow  string
	SignedInAt time.Time
	// AutoLink is off once an admin unlinked it: it is not linked by a recorded name or email again.
	AutoLink bool
	Request  *PendingRequest
	Refused  *RefusedRequest
}

// AccountLogin is a way into an account as admin sees it.
type AccountLogin struct {
	Provider    string    `json:"provider"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	SignedInAt  time.Time `json:"signed_in_at"`
}

// RefusedRequest is an account's last request when it was refused and nothing has followed: no new request, no link.
type RefusedRequest struct {
	Name   string
	Reason string
}

// PendingRequest is a request waiting for an answer: the name asked for, the country and zone the player gave, and
// the member the name names now (empty when approving would add one) with their recorded Discord name, email,
// country and zone, for the approver to compare.
type PendingRequest struct {
	ID            int64
	Name          string
	Country, Zone string
	Member        string
	MemberDiscord string
	MemberEmail   string
	MemberCountry string
	MemberZone    string
	RequestedAt   time.Time
}

// LinkApproved is what approving did: the member linked, and whether it was added for the request.
type LinkApproved struct {
	Member  string
	Created bool
}

// ErrNoRequest means no request with that id is waiting for an answer.
var ErrNoRequest = errors.New("no such request waiting")

// ErrNoAccount means no account has that id.
var ErrNoAccount = errors.New("no such account")

// ErrNoLogin means the account has no way in of that kind.
var ErrNoLogin = errors.New("no such way in")

// ErrMemberHasAccount means another account is linked to the member.
var ErrMemberHasAccount = errors.New("the member is linked to another account")

// ErrBadReason means a refusal's reason is over 500 characters.
var ErrBadReason = errors.New("a reason has at most 500 characters")

// Accounts lists every account: those not linked with a pending request first, oldest request first, then by the
// first way in's username. A request made before its account was linked by a recorded name no longer waits.
func (a *AdminStore) Accounts(ctx context.Context) ([]AccountRow, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT a.account_id, w.logins, w.signed_in_at, a.member_id, coalesce(m.tta_name, ''),
			coalesce(a.linked_how, ''), a.auto_link, r.request_id, r.tta_name, r.requested_at,
			coalesce(a.country_code, ''), coalesce(a.zone, ''), last.tta_name, last.reason
		FROM accounts a
		JOIN LATERAL (SELECT json_agg(json_build_object('provider', provider, 'username', username, 'display_name', display_name,
		                                                'signed_in_at', signed_in_at) ORDER BY provider) AS logins,
		                     max(signed_in_at) AS signed_in_at, min(lower(username)) AS first_name
		              FROM logins WHERE account_id = a.account_id) w ON true
		LEFT JOIN members m ON m.member_id = a.member_id
		LEFT JOIN link_requests r ON r.account_id = a.account_id AND r.status = 'pending' AND a.member_id IS NULL
		LEFT JOIN LATERAL (SELECT tta_name, reason, status FROM link_requests
		                   WHERE account_id = a.account_id ORDER BY request_id DESC LIMIT 1) last
		       ON last.status = 'refused' AND a.member_id IS NULL
		WHERE w.logins IS NOT NULL
		ORDER BY r.requested_at NULLS LAST, w.first_name, a.account_id`)
	if err != nil {
		return nil, fmt.Errorf("listing accounts: %w", err)
	}
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (AccountRow, error) {
		var l AccountRow
		var reqID *int64
		var reqName *string
		var reqAt *time.Time
		var country, zone string
		var refusedName, refusedReason *string
		err := row.Scan(&l.ID, &l.Logins, &l.SignedInAt, &l.MemberID, &l.Member, &l.LinkedHow, &l.AutoLink,
			&reqID, &reqName, &reqAt, &country, &zone, &refusedName, &refusedReason)
		if reqID != nil {
			l.Request = &PendingRequest{ID: *reqID, Name: *reqName, Country: country, Zone: zone, RequestedAt: *reqAt}
		}
		if refusedName != nil {
			l.Refused = &RefusedRequest{Name: *refusedName, Reason: *refusedReason}
		}
		return l, err
	})
	if err != nil {
		return nil, fmt.Errorf("listing accounts: %w", err)
	}
	for _, l := range got {
		if l.Request == nil {
			continue
		}
		m, found, err := namedMember(ctx, a.pool, l.Request.Name)
		if err != nil {
			return nil, err
		}
		if found {
			r := l.Request
			r.Member, r.MemberDiscord, r.MemberEmail, r.MemberCountry, r.MemberZone = m.Name, m.DiscordName, m.Email, m.Country, m.Zone
		}
	}
	return got, nil
}

// namedMember finds the member a TTA username names (the database's named_member: by username without case, or by
// the ratings player or CGE account of that name, as adding a member resolves it). It returns the member's id,
// name, Discord name, country, zone, email and ban. The email is read only by admin's login.
func namedMember(ctx context.Context, c conn, name string) (Member, bool, error) {
	var m Member
	err := c.QueryRow(ctx, `
		SELECT m.member_id, m.tta_name, m.discord_name, coalesce(m.country_code, ''), m.zone, m.email, m.banned FROM members m
		WHERE m.member_id = named_member($1)`, name).Scan(&m.ID, &m.Name, &m.DiscordName, &m.Country, &m.Zone, &m.Email, &m.Banned)
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, false, nil
	}
	if err != nil {
		return Member{}, false, fmt.Errorf("finding the member a name names: %w", err)
	}
	return m, true, nil
}

// ApproveLink links a pending request's account, at now, to the member its name names, adding that member (with
// the account's Discord name and email address, zone Unknown) when there is none, and moves the country and zone the player gave to it.
// It returns ErrNoRequest, ErrAlreadyLinked, or ErrMemberHasAccount when another account is linked to the member.
func (a *AdminStore) ApproveLink(ctx context.Context, request int64, now time.Time) (LinkApproved, error) {
	var out LinkApproved
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var account int64
		var name, discordName, email string
		var linked bool
		err := tx.QueryRow(ctx, `
			SELECT r.account_id, r.tta_name, coalesce(d.username, ''), coalesce(e.subject, ''), a.member_id IS NOT NULL
			FROM link_requests r JOIN accounts a USING (account_id)
			LEFT JOIN logins d ON d.account_id = a.account_id AND d.provider = 'discord'
			LEFT JOIN logins e ON e.account_id = a.account_id AND e.provider = 'email'
			WHERE r.request_id = $1 AND r.status = 'pending' FOR UPDATE OF r, a`, request).Scan(&account, &name, &discordName, &email, &linked)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoRequest
		}
		if err != nil {
			return fmt.Errorf("reading a request to be linked: %w", err)
		}
		if linked {
			return ErrAlreadyLinked
		}
		m, found, err := namedMember(ctx, tx, name)
		if err != nil {
			return err
		}
		if !found {
			add := Member{Name: name, DiscordName: discordName, Email: email, Zone: blitz.Unknown}
			if !validMember(&add) {
				return ErrBadMember
			}
			if m.ID, err = addMember(ctx, tx, add); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, "SELECT tta_name FROM members WHERE member_id = $1", m.ID).Scan(&m.Name); err != nil {
				return fmt.Errorf("reading the member added: %w", err)
			}
			out.Created = true
		}
		var taken bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT FROM accounts WHERE member_id = $1)", m.ID).Scan(&taken); err != nil {
			return fmt.Errorf("checking the member's account: %w", err)
		}
		if taken {
			return ErrMemberHasAccount
		}
		if _, err := tx.Exec(ctx, "UPDATE accounts SET member_id = $2, linked_how = 'approved', linked_at = $3 WHERE account_id = $1", account, m.ID, now); err != nil {
			return fmt.Errorf("linking an account: %w", err)
		}
		if err := moveAccountDetails(ctx, tx, account); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE link_requests SET status = 'approved', decided_at = $2 WHERE request_id = $1", request, now); err != nil {
			return fmt.Errorf("approving a request: %w", err)
		}
		out.Member = m.Name
		return nil
	})
	return out, err
}

// RefuseLink refuses a pending request at now, with a reason the player sees (may be empty). It returns
// ErrBadReason or ErrNoRequest.
func (a *AdminStore) RefuseLink(ctx context.Context, request int64, reason string, now time.Time) error {
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > 500 {
		return ErrBadReason
	}
	tag, err := a.pool.Exec(ctx, "UPDATE link_requests SET status = 'refused', reason = $2, decided_at = $3 WHERE request_id = $1 AND status = 'pending'",
		request, reason, now)
	if err != nil {
		return fmt.Errorf("refusing a request: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoRequest
	}
	return nil
}

// UnlinkAccount takes an account's member away, keeping its ways in, and stops it being linked by a recorded name
// or email again. It returns ErrNoAccount.
func (a *AdminStore) UnlinkAccount(ctx context.Context, account int64) error {
	tag, err := a.pool.Exec(ctx, `
		UPDATE accounts SET member_id = NULL, linked_how = NULL, linked_at = NULL, auto_link = false WHERE account_id = $1`, account)
	if err != nil {
		return fmt.Errorf("unlinking an account: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoAccount
	}
	return nil
}

// RemoveAccount deletes an account with its ways in, sessions, requests and links. It returns ErrNoAccount.
func (a *AdminStore) RemoveAccount(ctx context.Context, account int64) error {
	tag, err := a.pool.Exec(ctx, "DELETE FROM accounts WHERE account_id = $1", account)
	if err != nil {
		return fmt.Errorf("removing an account: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoAccount
	}
	return nil
}
