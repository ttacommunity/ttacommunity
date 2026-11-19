// Tournament sign-ups: admin opens and closes a tournament's, and a player with an account signs up or withdraws
// while they take entries. A linked player is entered at once; an unlinked one's sign-up waits, with a link request,
// until the account is linked or admin accepts it. Only blitz editions offer sign-ups so far.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// SignUps is a tournament's sign-ups: Open is admin's switch and ClosesAt the closing time (nil for none). Read
// only: Taking, whether they take entries now (sign_ups_taking: open, before ClosesAt, and before round 1), and
// Waiting, how many wait for an account to be linked.
type SignUps struct {
	Open     bool
	ClosesAt *time.Time
	Taking   bool
	Waiting  int
}

// PendingSignUp is a sign-up waiting for its account to be linked: the account, the TTA username it gave, when, its
// pending link request (0 for none) and the member the account is linked to by now ("" while it is not).
type PendingSignUp struct {
	ID, Account int64
	Name        string
	SignedUpAt  time.Time
	Request     int64
	Linked      string
}

// ErrSignUpsClosed means the tournament does not take entries now.
var ErrSignUpsClosed = errors.New("sign-ups are closed")

// ErrSignUpsNotOffered means sign-ups for a tournament that does not offer them: only blitz editions do.
var ErrSignUpsNotOffered = errors.New("only blitz editions take sign-ups")

// ErrSignUpsHidden means opening the sign-ups of a tournament the site does not show.
var ErrSignUpsHidden = errors.New("show the tournament on the site before opening its sign-ups")

// ErrSignUpsStarted means opening the sign-ups of an edition with a round paired.
var ErrSignUpsStarted = errors.New("sign-ups cannot open once round 1 is paired")

// ErrBadSignUps means a closing time that is not after now.
var ErrBadSignUps = errors.New("the closing time must be in the future")

// ErrSignUpNameNeeded means an unlinked player signing up without their TTA username, or with one over 100
// characters.
var ErrSignUpNameNeeded = errors.New("give your TTA username, of at most 100 characters")

// ErrNoSignUp means no pending sign-up of the tournament has that id.
var ErrNoSignUp = errors.New("no such sign-up")

// signUpError maps a sign-up function's refusal to its error, or nil for none it knows.
func signUpError(refusal string) error {
	switch refusal {
	case "signed-out":
		return ErrNoSession
	case "banned":
		return ErrMemberBanned
	case "no-edition":
		return ErrNoBlitzEdition
	case "closed":
		return ErrSignUpsClosed
	case "name-needed":
		return ErrSignUpNameNeeded
	}
	return nil
}

// SetSignUps opens or closes an edition's sign-ups at now, with s's closing time. Opening them the first time queues
// their news (blitz_sign_ups), once. Opening returns ErrSignUpsHidden, ErrSignUpsStarted or ErrBadSignUps; either
// returns ErrNoBlitzEdition or ErrSignUpsNotOffered.
func (a *AdminStore) SetSignUps(ctx context.Context, slug, edition string, s SignUps, now time.Time) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var id int64
		var blitz, shown, posted, started bool
		err := tx.QueryRow(ctx, `
			SELECT t.tournament_id, s.blitz, s.visible, t.sign_ups_posted,
			       EXISTS (SELECT 1 FROM blitz_rounds r WHERE r.tournament_id = t.tournament_id)
			FROM tournaments t JOIN series s USING (series_id)
			WHERE s.slug = $1 AND t.label = $2
			FOR UPDATE OF t`, slug, edition).Scan(&id, &blitz, &shown, &posted, &started)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return ErrNoBlitzEdition
		case err != nil:
			return fmt.Errorf("reading %s %s's sign-ups: %w", slug, edition, err)
		case !blitz:
			return ErrSignUpsNotOffered
		case s.Open && !shown:
			return ErrSignUpsHidden
		case s.Open && started:
			return ErrSignUpsStarted
		case s.Open && s.ClosesAt != nil && !s.ClosesAt.After(now):
			return ErrBadSignUps
		}
		if _, err := blitzEditionID(ctx, tx, slug, edition); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE tournaments SET sign_ups_open = $2, sign_ups_close_at = $3 WHERE tournament_id = $1",
			id, s.Open, s.ClosesAt); err != nil {
			return fmt.Errorf("setting %s %s's sign-ups: %w", slug, edition, err)
		}
		if !s.Open || posted {
			return nil
		}
		if _, err := tx.Exec(ctx, "UPDATE tournaments SET sign_ups_posted = true WHERE tournament_id = $1", id); err != nil {
			return fmt.Errorf("marking %s %s's sign-ups posted: %w", slug, edition, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO notification_outbox (kind, edition_id, created_at, next_at) VALUES ('blitz_sign_ups', $1, $2, $2)",
			id, now); err != nil {
			return fmt.Errorf("queueing %s %s's sign-ups: %w", slug, edition, err)
		}
		return nil
	})
}

// SignUpsOf reads a tournament's sign-ups, or returns ErrNoBlitzEdition.
func (a *AdminStore) SignUpsOf(ctx context.Context, slug, edition string) (SignUps, error) {
	var s SignUps
	err := a.pool.QueryRow(ctx, `
		SELECT t.sign_ups_open, t.sign_ups_close_at, coalesce(sign_ups_taking(t.tournament_id, now()), false), sign_ups_waiting(t.tournament_id)
		FROM tournaments t JOIN series s USING (series_id) WHERE s.slug = $1 AND t.label = $2`, slug, edition).Scan(&s.Open, &s.ClosesAt, &s.Taking, &s.Waiting)
	if errors.Is(err, pgx.ErrNoRows) {
		return SignUps{}, ErrNoBlitzEdition
	}
	if err != nil {
		return SignUps{}, fmt.Errorf("reading %s %s's sign-ups: %w", slug, edition, err)
	}
	return s, nil
}

// SignUp signs the player whose session hashes to tokenHash up for the shown edition at now, and returns "entered"
// for a linked player, now an entrant, or "pending" for an unlinked one: their sign-up waits under name, and asks to
// link their account to it. Signing up again is the same. It returns ErrNoSession, ErrMemberBanned,
// ErrNoBlitzEdition, ErrSignUpsClosed, ErrSignUpNameNeeded, or RequestLink's errors.
func (s *PlayerStore) SignUp(ctx context.Context, tokenHash []byte, slug, edition, name string, now time.Time) (string, error) {
	var got string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var account *int64
		if err := tx.QueryRow(ctx, "SELECT status, account FROM sign_up($1, $2, $3, $4, $5)", tokenHash, now, slug, edition, name).Scan(&got, &account); err != nil {
			return fmt.Errorf("signing up: %w", err)
		}
		if err := signUpError(got); err != nil {
			return err
		}
		if got != "pending" {
			return nil
		}
		return requestLink(ctx, tx, *account, name, now)
	})
	if err != nil {
		return "", err
	}
	return got, nil
}

// WithdrawSignUp takes the player whose session hashes to tokenHash out of the shown edition at now: their entry
// and their pending sign-up, if any; a link request it asked for stays. It returns ErrNoSession, ErrMemberBanned,
// ErrNoBlitzEdition or ErrSignUpsClosed.
func (s *PlayerStore) WithdrawSignUp(ctx context.Context, tokenHash []byte, slug, edition string, now time.Time) error {
	var got string
	if err := s.pool.QueryRow(ctx, "SELECT withdraw_sign_up($1, $2, $3, $4)", tokenHash, now, slug, edition).Scan(&got); err != nil {
		return fmt.Errorf("withdrawing: %w", err)
	}
	return signUpError(got)
}

// MySignUp returns the pending sign-up for the edition of the player whose session hashes to tokenHash at now, and
// whether they have one; none for a player signed out.
func (s *PlayerStore) MySignUp(ctx context.Context, tokenHash []byte, slug, edition string, now time.Time) (PendingSignUp, bool, error) {
	var p PendingSignUp
	err := s.pool.QueryRow(ctx, "SELECT sign_up_id, account_id, tta_name, signed_up_at FROM my_sign_ups($1, $2) WHERE slug = $3 AND label = $4",
		tokenHash, now, slug, edition).Scan(&p.ID, &p.Account, &p.Name, &p.SignedUpAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PendingSignUp{}, false, nil
	}
	if err != nil {
		return PendingSignUp{}, false, fmt.Errorf("reading your sign-up: %w", err)
	}
	return p, true, nil
}

// PendingSignUps lists the edition's pending sign-ups, oldest first.
func (a *AdminStore) PendingSignUps(ctx context.Context, slug, edition string) ([]PendingSignUp, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT u.sign_up_id, u.account_id, u.tta_name, u.signed_up_at, coalesce(r.request_id, 0), coalesce(m.tta_name, '')
		FROM tournament_sign_ups u JOIN tournaments t USING (tournament_id) JOIN series s USING (series_id)
		JOIN accounts a ON a.account_id = u.account_id LEFT JOIN members m ON m.member_id = a.member_id
		LEFT JOIN link_requests r ON r.account_id = u.account_id AND r.status = 'pending'
		WHERE s.slug = $1 AND t.label = $2
		ORDER BY u.signed_up_at, u.sign_up_id`, slug, edition)
	if err != nil {
		return nil, fmt.Errorf("listing %s %s's sign-ups: %w", slug, edition, err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (PendingSignUp, error) {
		var p PendingSignUp
		return p, row.Scan(&p.ID, &p.Account, &p.Name, &p.SignedUpAt, &p.Request, &p.Linked)
	})
	if err != nil {
		return nil, fmt.Errorf("listing %s %s's sign-ups: %w", slug, edition, err)
	}
	return out, nil
}

// AcceptSignUp enters a pending sign-up at now, whether or not sign-ups take entries: an unlinked account is linked
// first by approving its link request, as ApproveLink does. It returns ErrNoBlitzEdition, ErrNoSignUp, ErrNoRequest
// for an unlinked account with no pending request, ErrMemberBanned, or ApproveLink's errors.
func (a *AdminStore) AcceptSignUp(ctx context.Context, slug, edition string, id int64, now time.Time) error {
	return a.inBlitzEdition(ctx, slug, edition, func(tx pgx.Tx, edID int64) error {
		var account int64
		var request *int64
		err := tx.QueryRow(ctx, `
			SELECT u.account_id, (SELECT r.request_id FROM link_requests r WHERE r.account_id = u.account_id AND r.status = 'pending')
			FROM tournament_sign_ups u WHERE u.sign_up_id = $1 AND u.tournament_id = $2`, id, edID).Scan(&account, &request)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoSignUp
		}
		if err != nil {
			return fmt.Errorf("reading a sign-up of %s %s: %w", slug, edition, err)
		}
		var member *int64
		var banned bool
		read := func() error {
			return tx.QueryRow(ctx, "SELECT a.member_id, coalesce(m.banned, false) FROM accounts a LEFT JOIN members m USING (member_id) WHERE a.account_id = $1",
				account).Scan(&member, &banned)
		}
		if err := read(); err != nil {
			return fmt.Errorf("reading a sign-up's account: %w", err)
		}
		if member == nil {
			if request == nil {
				return ErrNoRequest
			}
			if _, err := approveLink(ctx, tx, *request, now); err != nil {
				return err
			}
			if err := read(); err != nil {
				return fmt.Errorf("reading a sign-up's account: %w", err)
			}
		}
		if banned {
			return ErrMemberBanned
		}
		if _, err := tx.Exec(ctx, "INSERT INTO blitz_entries (tournament_id, member_id, joined_at) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING",
			edID, *member, now); err != nil {
			return fmt.Errorf("entering a sign-up of %s %s: %w", slug, edition, err)
		}
		if _, err := tx.Exec(ctx, "DELETE FROM tournament_sign_ups WHERE sign_up_id = $1", id); err != nil {
			return fmt.Errorf("entering a sign-up of %s %s: %w", slug, edition, err)
		}
		return nil
	})
}

// RemoveSignUp deletes a pending sign-up; its link request stays. It returns ErrNoSignUp.
func (a *AdminStore) RemoveSignUp(ctx context.Context, slug, edition string, id int64) error {
	tag, err := a.pool.Exec(ctx, `
		DELETE FROM tournament_sign_ups u USING tournaments t, series s
		WHERE u.sign_up_id = $1 AND t.tournament_id = u.tournament_id AND s.series_id = t.series_id AND s.slug = $2 AND t.label = $3`,
		id, slug, edition)
	if err != nil {
		return fmt.Errorf("removing a sign-up of %s %s: %w", slug, edition, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoSignUp
	}
	return nil
}
