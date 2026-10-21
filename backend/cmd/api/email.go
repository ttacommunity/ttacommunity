// Signing in by email: Resend sends the links, Turnstile guards the form, and on this machine alone a dev mailer
// prints each link instead, with Cloudflare's test keys.
package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"ttacommunity/backend/internal/resend"
	"ttacommunity/backend/internal/store"
	"ttacommunity/backend/internal/turnstile"
)

// emailSignIn sends sign-in links and checks the form's Turnstile token; its zero value is email sign-in off.
type emailSignIn struct {
	send     func(ctx context.Context, m resend.Message) error
	verify   *turnstile.Client
	support  string
	hostname string
}

func (e emailSignIn) ready() bool { return e.send != nil }

// isTestSecret reports whether s is one of Turnstile's published test secrets, which pass or fail every token.
func isTestSecret(s string) bool {
	return len(s) == 35 && strings.HasSuffix(s, "x0000000000000000000000000000000AA")
}

// openEmail builds email sign-in for the site at site from getenv's RESEND_API_KEY, EMAIL_FROM, EMAIL_SUPPORT (the
// address the email says to write to) and TURNSTILE_SECRET, or EMAIL_DEV_MAIL, which prints links instead of
// sending them, on this machine alone. With none of them set, email sign-in is off; half a config, or a Turnstile
// test secret outside local development, is an error.
func openEmail(getenv func(string) string, site string) (emailSignIn, error) {
	key, from, support, secret := getenv("RESEND_API_KEY"), getenv("EMAIL_FROM"), getenv("EMAIL_SUPPORT"), getenv("TURNSTILE_SECRET")
	dev := getenv("EMAIL_DEV_MAIL") != ""
	if !dev && key == "" && from == "" && support == "" && secret == "" {
		return emailSignIn{}, nil
	}
	if _, err := store.EmailAddress(support); err != nil {
		return emailSignIn{}, errors.New("EMAIL_SUPPORT must be the address players write to with sign-in trouble")
	}
	if secret == "" {
		return emailSignIn{}, errors.New("TURNSTILE_SECRET is not set: the email form needs Turnstile")
	}
	e := emailSignIn{support: support}
	if dev {
		if !strings.HasPrefix(getenv("API_ADDR"), "127.0.0.1:") || getenv("KUBERNETES_SERVICE_HOST") != "" ||
			!strings.HasPrefix(site, "http://127.0.0.1:") || key != "" {
			return emailSignIn{}, errors.New("EMAIL_DEV_MAIL is for this machine alone: API_ADDR and SITE_ORIGIN on 127.0.0.1, " +
				"outside Kubernetes, and no RESEND_API_KEY")
		}
		e.send = printLink
	} else {
		if isTestSecret(secret) {
			return emailSignIn{}, errors.New("TURNSTILE_SECRET is one of Turnstile's test secrets, which are for local development alone")
		}
		u, err := url.Parse(site)
		if err != nil {
			return emailSignIn{}, errors.New("SITE_ORIGIN is not a URL")
		}
		e.hostname = u.Hostname()
		c, err := resend.New(resend.Config{APIKey: key, From: from}, &http.Client{Timeout: 10 * time.Second})
		if err != nil {
			return emailSignIn{}, fmt.Errorf("RESEND_API_KEY and EMAIL_FROM: %w", err)
		}
		e.send = c.Send
	}
	v, err := turnstile.New(turnstile.Config{Secret: secret, Hostname: e.hostname}, &http.Client{Timeout: 10 * time.Second})
	if err != nil {
		return emailSignIn{}, fmt.Errorf("TURNSTILE_SECRET: %w", err)
	}
	e.verify = v
	return e, nil
}

// printLink is the dev mailer: it prints the link, not the address, for the developer to open.
func printLink(_ context.Context, m resend.Message) error {
	for _, line := range strings.Split(m.Text, "\n") {
		if strings.Contains(line, "://") {
			fmt.Fprintln(os.Stderr, "EMAIL_DEV_MAIL sign-in link:", strings.TrimSpace(line))
		}
	}
	return nil
}

// signInMail is the email carrying link to address, saying where to write with trouble: plain text alone.
func signInMail(address, link, support string) resend.Message {
	return resend.Message{
		To:      address,
		Subject: "Your sign-in link for TTA Community",
		Text: "Open this link within an hour to sign in to TTA Community:\n\n" +
			link + "\n\n" +
			"It works once. If you did not ask for it, you can ignore this email: nobody signs in without the link.\n\n" +
			"Trouble signing in? Write to " + support + "\n",
		IdempotencyKey: fmt.Sprintf("%x", sha256.Sum256([]byte(link))),
	}
}
