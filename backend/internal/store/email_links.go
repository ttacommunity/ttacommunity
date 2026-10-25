// Signing in with an emailed link: sending one, within the limits, and opening it to sign in or to add the address
// to an account as another way in. A link is kept two days, for the limits, then forgotten.
package store

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// EmailLinkLife is how long an emailed link may be opened.
const EmailLinkLife = time.Hour

// The sending limits: per address and in all, each over a day; and how long a link is remembered.
const (
	linksPerAddress = 5
	linksPerDay     = 90
	linkKeep        = 48 * time.Hour
)

// ErrBadAddress means an email address is not one plain address of at most 254 characters.
var ErrBadAddress = errors.New("an email address, such as name@example.org")

// ErrTooManyLinks means a limit on sending links is reached: try later.
var ErrTooManyLinks = errors.New("too many sign-in links sent: try again later")

// ErrLinkNotValid means a link is unknown, used, replaced by a newer one, or more than an hour old.
var ErrLinkNotValid = errors.New("the link has expired or has been used")

// ErrLoginTaken means the way in belongs to another account.
var ErrLoginTaken = errors.New("that way in has an account of its own")

// ErrHasLoginOfKind means the account has another Discord account already.
var ErrHasLoginOfKind = errors.New("the account has a way in of that kind already")

// EmailSignIn is what opening a link did: a first sign-in, or an address added to an account, and the site path
// to go back to.
type EmailSignIn struct {
	First, Added bool
	Back         string
}

// EmailAddress returns s trimmed and lower-cased, or ErrBadAddress when it is not one plain address.
func EmailAddress(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	a, err := mail.ParseAddress(s)
	if err != nil || a.Name != "" || a.Address != s || len(s) > 254 || strings.Count(s, "@") != 1 {
		return "", ErrBadAddress
	}
	return s, nil
}

// NewEmailLink records a link to address, whose token hashes to tokenHash, sent at now and opened within the hour,
// coming back to back: to sign in, or with account set to add the address to it, in place of any it has. It
// replaces any link to the address not yet used, and forgets links over two days old. It returns ErrBadAddress,
// ErrTooManyLinks, or, to add, ErrLoginTaken.
func (s *PlayerStore) NewEmailLink(ctx context.Context, address string, account *int64, back string, tokenHash []byte, now time.Time) error {
	address, err := EmailAddress(address)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// One sender at a time across the api's replicas, so a limit cannot be passed by two at once.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('email_links'))"); err != nil {
			return fmt.Errorf("waiting to send a link: %w", err)
		}
		if account != nil {
			if err := canAddLogin(ctx, tx, *account, Identity{Provider: "email", Subject: address}); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, "DELETE FROM email_links WHERE created_at <= $1", now.Add(-linkKeep)); err != nil {
			return fmt.Errorf("forgetting old links: %w", err)
		}
		var mine, all int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FILTER (WHERE address = $1), count(*) FROM email_links WHERE created_at > $2`,
			address, now.Add(-24*time.Hour)).Scan(&mine, &all); err != nil {
			return fmt.Errorf("counting links sent: %w", err)
		}
		if mine >= linksPerAddress || all >= linksPerDay {
			return ErrTooManyLinks
		}
		if _, err := tx.Exec(ctx, "UPDATE email_links SET expires_at = $2 WHERE address = $1 AND used_at IS NULL AND expires_at > $2", address, now); err != nil {
			return fmt.Errorf("replacing older links: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO email_links (token_hash, address, account_id, return_path, created_at, expires_at) VALUES ($1, $2, $3, $4, $5, $6)`,
			tokenHash, address, account, back, now, now.Add(EmailLinkLife)); err != nil {
			return fmt.Errorf("recording a link: %w", err)
		}
		return nil
	})
}

// UseEmailLink opens the link whose token hashes to linkHash at now, once: it signs in by its address, or gives the
// address to the account it was sent for, and starts a session whose token hashes to sessionHash. It returns
// ErrLinkNotValid, or, to add, ErrLoginTaken.
func (s *PlayerStore) UseEmailLink(ctx context.Context, linkHash, sessionHash []byte, now time.Time) (EmailSignIn, error) {
	var out EmailSignIn
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var address string
		var account *int64
		err := tx.QueryRow(ctx, `
			UPDATE email_links SET used_at = $2 WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $2
			RETURNING address, account_id, return_path`, linkHash, now).Scan(&address, &account, &out.Back)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrLinkNotValid
		}
		if err != nil {
			return fmt.Errorf("opening a link: %w", err)
		}
		id := Identity{Provider: "email", Subject: address, Username: address}
		if account == nil {
			out.First, err = signIn(ctx, tx, id, sessionHash, now)
			return err
		}
		login, err := addLogin(ctx, tx, *account, id, now)
		if err != nil {
			return err
		}
		out.Added = true
		if err := startSession(ctx, tx, login, sessionHash, now); err != nil {
			return err
		}
		return autoLink(ctx, tx, *account, id, sessionHash, now)
	})
	return out, err
}
