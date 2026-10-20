//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// emailSignedIn sends address a sign-in link and opens it with a session for token, failing the test on an error.
func emailSignedIn(t *testing.T, p *PlayerStore, address, token string) Account {
	t.Helper()
	ctx := context.Background()
	if err := p.NewEmailLink(ctx, address, nil, "/account/", tokenHash("link-"+token), signInAt); err != nil {
		t.Fatalf("NewEmailLink(%s): %v", address, err)
	}
	if _, err := p.UseEmailLink(ctx, tokenHash("link-"+token), tokenHash(token), signInAt); err != nil {
		t.Fatalf("UseEmailLink(%s): %v", address, err)
	}
	l, err := p.Session(ctx, tokenHash(token), signInAt)
	if err != nil {
		t.Fatalf("Session after signing %s in: %v", address, err)
	}
	return l
}

func TestEmailAddressIsTrimmedAndLowerCasedOrRefused(t *testing.T) {
	tests := []struct {
		in, want string
		err      error
	}{
		{" Player.One@Example.TEST ", "player.one@example.test", nil},
		{"player+tta@example.test", "player+tta@example.test", nil},
		{"", "", ErrBadAddress},
		{"not an address", "", ErrBadAddress},
		{"Player One <player@example.test>", "", ErrBadAddress},
		{"player@one@example.test", "", ErrBadAddress},
		{strings.Repeat("a", 250) + "@example.test", "", ErrBadAddress},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := EmailAddress(tt.in)
			if got != tt.want || !errors.Is(err, tt.err) {
				t.Errorf("EmailAddress(%q) = %q, %v; want %q, %v", tt.in, got, err, tt.want, tt.err)
			}
		})
	}
}

func TestAnEmailedLinkSignsInOnceWithinTheHour(t *testing.T) {
	_, _, p := playersFixture(t)
	ctx := context.Background()
	if err := p.NewEmailLink(ctx, "player@example.test", nil, "/ratings/player/?name=ann", tokenHash("link"), signInAt); err != nil {
		t.Fatalf("NewEmailLink: %v", err)
	}
	got, err := p.UseEmailLink(ctx, tokenHash("link"), tokenHash("session"), signInAt.Add(59*time.Minute))
	if err != nil || !got.First || got.Added || got.Back != "/ratings/player/?name=ann" {
		t.Errorf("UseEmailLink = %+v, %v; want a first sign-in back to the player page", got, err)
	}
	l, err := p.Session(ctx, tokenHash("session"), signInAt.Add(time.Hour))
	if err != nil || l.Provider != "email" || l.Username != "player@example.test" || l.Member != nil {
		t.Errorf("the session = %+v, %v; want an unlinked email sign-in as player@example.test", l, err)
	}
	if _, err := p.UseEmailLink(ctx, tokenHash("link"), tokenHash("again"), signInAt.Add(time.Hour)); !errors.Is(err, ErrLinkNotValid) {
		t.Errorf("using the link again = %v, want ErrLinkNotValid", err)
	}
	again := emailSignedIn(t, p, "Player@Example.test", "later")
	if again.ID != l.ID {
		t.Errorf("signing in again by email: account %d, want %d", again.ID, l.ID)
	}
}

func TestAnEmailedLinkIsRefused(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, p *PlayerStore)
		token string
		at    time.Time
	}{
		{"after an hour", nil, "link", signInAt.Add(time.Hour)},
		{"once a newer one is sent", func(t *testing.T, p *PlayerStore) {
			if err := p.NewEmailLink(context.Background(), "player@example.test", nil, "/account/", tokenHash("newer"), signInAt.Add(time.Minute)); err != nil {
				t.Fatalf("NewEmailLink: %v", err)
			}
		}, "link", signInAt.Add(2 * time.Minute)},
		{"that was never sent", nil, "unknown", signInAt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, p := playersFixture(t)
			if err := p.NewEmailLink(context.Background(), "player@example.test", nil, "/account/", tokenHash("link"), signInAt); err != nil {
				t.Fatalf("NewEmailLink: %v", err)
			}
			if tt.setup != nil {
				tt.setup(t, p)
			}
			if _, err := p.UseEmailLink(context.Background(), tokenHash(tt.token), tokenHash("session"), tt.at); !errors.Is(err, ErrLinkNotValid) {
				t.Errorf("UseEmailLink = %v, want ErrLinkNotValid", err)
			}
		})
	}
}

func TestEmailedLinksAreLimitedToFiveADayPerAddress(t *testing.T) {
	_, _, p := playersFixture(t)
	ctx := context.Background()
	send := func(n int, at time.Time) error {
		return p.NewEmailLink(ctx, "player@example.test", nil, "/account/", tokenHash(fmt.Sprint("link-", n)), at)
	}
	for i := range 5 {
		if err := send(i, signInAt.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("link %d: %v", i+1, err)
		}
	}
	if err := send(5, signInAt.Add(time.Hour)); !errors.Is(err, ErrTooManyLinks) {
		t.Errorf("a sixth link within a day = %v, want ErrTooManyLinks", err)
	}
	if err := send(6, signInAt.Add(24*time.Hour+time.Second)); err != nil {
		t.Errorf("a link a day after the first = %v, want nil", err)
	}
}

func TestEmailedLinksAreLimitedToNinetyADay(t *testing.T) {
	_, _, p := playersFixture(t)
	ctx := context.Background()
	send := func(n int) error {
		return p.NewEmailLink(ctx, fmt.Sprintf("player%d@example.test", n), nil, "/account/", tokenHash(fmt.Sprint("link-", n)), signInAt)
	}
	for i := range 90 {
		if err := send(i); err != nil {
			t.Fatalf("link %d: %v", i+1, err)
		}
	}
	if err := send(90); !errors.Is(err, ErrTooManyLinks) {
		t.Errorf("the 91st link in a day = %v, want ErrTooManyLinks", err)
	}
}

func TestOldEmailedLinksAreForgottenAfterTwoDays(t *testing.T) {
	pool, _, p := playersFixture(t)
	ctx := context.Background()
	if err := p.NewEmailLink(ctx, "player@example.test", nil, "/account/", tokenHash("old"), signInAt); err != nil {
		t.Fatalf("NewEmailLink: %v", err)
	}
	if err := p.NewEmailLink(ctx, "other@example.test", nil, "/account/", tokenHash("new"), signInAt.Add(48*time.Hour)); err != nil {
		t.Fatalf("NewEmailLink: %v", err)
	}
	if got := rowsOf(t, pool, "SELECT address FROM email_links"); len(got) != 1 || got[0] != "other@example.test" {
		t.Errorf("links kept = %q, want only the newer address", got)
	}
}

func TestAnEmailSignInLinksTheOneMemberWhoseEmailMatches(t *testing.T) {
	tests := []struct {
		name    string
		members []Member
		setup   func(t *testing.T, a *AdminStore, p *PlayerStore)
		want    string
	}{
		{"the same address in another case", []Member{{Name: "ann", Email: " Player@Example.test"}}, nil, "ann"},
		{"no member with that address", []Member{{Name: "ann", Email: "someone@example.test"}}, nil, ""},
		{"two members with that address", []Member{{Name: "ann", Email: "player@example.test"}, {Name: "cat", Email: "player@example.test"}}, nil, ""},
		{"another account is linked to the member", []Member{{Name: "ann", Email: "player@example.test"}}, func(t *testing.T, a *AdminStore, p *PlayerStore) {
			approved(t, a, p, signedIn(t, p, discordID("100000000000000009", "other_account", ""), "other"), "ann")
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, a, p := playersFixture(t)
			for _, m := range tt.members {
				m.Zone = "UTC"
				if _, err := a.AddMember(context.Background(), m); err != nil {
					t.Fatalf("AddMember(%s): %v", m.Name, err)
				}
			}
			if tt.setup != nil {
				tt.setup(t, a, p)
			}
			got := emailSignedIn(t, p, "player@example.test", "token")
			linked := ""
			if got.Member != nil {
				linked = got.Member.Name
			}
			if linked != tt.want {
				t.Errorf("signing in as player@example.test linked %q, want %q", linked, tt.want)
			}
		})
	}
}

func TestAnUnlinkedEmailSignInIsNotLinkedByEmailAgain(t *testing.T) {
	_, a, p := playersFixture(t)
	ctx := context.Background()
	if _, err := a.AddMember(ctx, Member{Name: "ann", Email: "player@example.test", Zone: "UTC"}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	l := emailSignedIn(t, p, "player@example.test", "token")
	if err := a.UnlinkAccount(ctx, l.ID); err != nil {
		t.Fatalf("UnlinkAccount: %v", err)
	}
	if again := emailSignedIn(t, p, "player@example.test", "again"); again.Member != nil {
		t.Errorf("signing in after an unlink linked %+v again by email, want none", again.Member)
	}
}

func TestAPlayerAddsEmailAsASecondWayIn(t *testing.T) {
	_, _, p := playersFixture(t)
	ctx := context.Background()
	d := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "discord")
	if err := p.NewEmailLink(ctx, "player@example.test", &d.ID, "/account/", tokenHash("add"), signInAt); err != nil {
		t.Fatalf("NewEmailLink to add: %v", err)
	}
	got, err := p.UseEmailLink(ctx, tokenHash("add"), tokenHash("email"), signInAt)
	if err != nil || !got.Added || got.First {
		t.Errorf("UseEmailLink to add = %+v, %v; want added", got, err)
	}
	e, err := p.Session(ctx, tokenHash("email"), signInAt)
	if err != nil || e.ID != d.ID || e.Provider != "email" {
		t.Errorf("the email session = %+v, %v; want account %d by email", e, err, d.ID)
	}
	if again := emailSignedIn(t, p, "player@example.test", "later"); again.ID != d.ID {
		t.Errorf("a later email sign-in: account %d, want %d", again.ID, d.ID)
	}
	if ways := loginNames(e); ways != "discord:player_one email:player@example.test" {
		t.Errorf("ways in = %q, want Discord and email", ways)
	}
}

// loginNames is an account's ways in as provider:username, in order.
func loginNames(a Account) string {
	var out []string
	for _, l := range a.Logins {
		out = append(out, l.Provider+":"+l.Username)
	}
	return strings.Join(out, " ")
}

func TestAddingAnEmailIsRefused(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, p *PlayerStore, account int64)
		want  error
	}{
		{"an address with an account of its own", func(t *testing.T, p *PlayerStore, _ int64) {
			emailSignedIn(t, p, "player@example.test", "other")
		}, ErrLoginTaken},
		{"an account that has an email already", func(t *testing.T, p *PlayerStore, account int64) {
			ctx := context.Background()
			if err := p.NewEmailLink(ctx, "first@example.test", &account, "/account/", tokenHash("first"), signInAt); err != nil {
				t.Fatalf("NewEmailLink(first): %v", err)
			}
			if _, err := p.UseEmailLink(ctx, tokenHash("first"), tokenHash("first-session"), signInAt); err != nil {
				t.Fatalf("UseEmailLink(first): %v", err)
			}
		}, ErrHasLoginOfKind},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, p := playersFixture(t)
			d := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "discord")
			tt.setup(t, p, d.ID)
			if err := p.NewEmailLink(context.Background(), "player@example.test", &d.ID, "/account/", tokenHash("add"), signInAt); !errors.Is(err, tt.want) {
				t.Errorf("NewEmailLink to add = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestAddingAnEmailTakenSinceTheLinkWasSentIsRefused(t *testing.T) {
	pool, _, p := playersFixture(t)
	ctx := context.Background()
	d := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "discord")
	if err := p.NewEmailLink(ctx, "player@example.test", &d.ID, "/account/", tokenHash("add"), signInAt); err != nil {
		t.Fatalf("NewEmailLink to add: %v", err)
	}
	// A newer link would cancel this one, so the address is taken behind the links' back.
	if _, err := pool.Exec(ctx, `WITH a AS (INSERT INTO accounts (created_at) VALUES ($1) RETURNING account_id)
		INSERT INTO logins (account_id, provider, subject, username, created_at, signed_in_at)
		SELECT account_id, 'email', 'player@example.test', 'player@example.test', $1, $1 FROM a`, signInAt); err != nil {
		t.Fatalf("taking the address: %v", err)
	}
	if _, err := p.UseEmailLink(ctx, tokenHash("add"), tokenHash("email"), signInAt); !errors.Is(err, ErrLoginTaken) {
		t.Errorf("UseEmailLink to add a taken address = %v, want ErrLoginTaken", err)
	}
}

func TestAPlayerAddsDiscordAsASecondWayIn(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, a *AdminStore, p *PlayerStore)
		want  error
	}{
		{"a Discord account new here", nil, nil},
		{"a Discord account with an account of its own", func(t *testing.T, _ *AdminStore, p *PlayerStore) {
			signedIn(t, p, discordID("100000000000000001", "player_one", ""), "other")
		}, ErrLoginTaken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, a, p := playersFixture(t)
			ctx := context.Background()
			e := emailSignedIn(t, p, "player@example.test", "email")
			if tt.setup != nil {
				tt.setup(t, a, p)
			}
			if err := p.AddDiscord(ctx, tokenHash("email"), discordID("100000000000000001", "player_one", "One"), signInAt); !errors.Is(err, tt.want) {
				t.Fatalf("AddDiscord = %v, want %v", err, tt.want)
			}
			if tt.want != nil {
				return
			}
			if again := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "discord"); again.ID != e.ID {
				t.Errorf("a Discord sign-in after adding it: account %d, want %d", again.ID, e.ID)
			}
		})
	}
}

func TestAddingDiscordLinksByDiscordName(t *testing.T) {
	_, a, p := playersFixture(t)
	ctx := context.Background()
	if _, err := a.AddMember(ctx, Member{Name: "ann", DiscordName: "player_one", Zone: "UTC"}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	emailSignedIn(t, p, "player@example.test", "email")
	if err := p.AddDiscord(ctx, tokenHash("email"), discordID("100000000000000001", "player_one", ""), signInAt); err != nil {
		t.Fatalf("AddDiscord: %v", err)
	}
	if l, err := p.Session(ctx, tokenHash("email"), signInAt); err != nil || l.Member == nil || l.Member.Name != "ann" {
		t.Errorf("after adding Discord: member %+v (%v), want ann", l.Member, err)
	}
}

func TestRemovingAWayIn(t *testing.T) {
	_, _, p := playersFixture(t)
	ctx := context.Background()
	d := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "discord")
	signedIn(t, p, discordID("100000000000000001", "player_one", ""), "discord-elsewhere")
	if err := p.RemoveOwnLogin(ctx, tokenHash("discord"), "discord", signInAt); !errors.Is(err, ErrLastLogin) {
		t.Errorf("removing the only way in = %v, want ErrLastLogin", err)
	}
	if err := p.RemoveOwnLogin(ctx, tokenHash("discord"), "email", signInAt); !errors.Is(err, ErrNoLogin) {
		t.Errorf("removing a way in the account has not got = %v, want ErrNoLogin", err)
	}
	if err := p.NewEmailLink(ctx, "player@example.test", &d.ID, "/account/", tokenHash("add"), signInAt); err != nil {
		t.Fatalf("NewEmailLink to add: %v", err)
	}
	if _, err := p.UseEmailLink(ctx, tokenHash("add"), tokenHash("email"), signInAt); err != nil {
		t.Fatalf("UseEmailLink to add: %v", err)
	}
	if err := p.RemoveOwnLogin(ctx, tokenHash("discord"), "discord", signInAt); err != nil {
		t.Fatalf("removing Discord with email left: %v", err)
	}
	now, err := p.Session(ctx, tokenHash("discord"), signInAt)
	if err != nil || now.ID != d.ID || now.Provider != "email" || loginNames(now) != "email:player@example.test" {
		t.Errorf("this browser after removing Discord = %+v, %v; want signed in by email to account %d", now, err, d.ID)
	}
	if _, err := p.Session(ctx, tokenHash("discord-elsewhere"), signInAt); !errors.Is(err, ErrNoSession) {
		t.Errorf("another Discord session after removing Discord = %v, want ErrNoSession", err)
	}
	if err := p.RemoveOwnLogin(ctx, tokenHash("nobody"), "email", signInAt); !errors.Is(err, ErrNoSession) {
		t.Errorf("removing with no session = %v, want ErrNoSession", err)
	}
}

func TestDeletingAnAccountClearsTheMembersEmailOnlyWhenItSignedInWithIt(t *testing.T) {
	tests := []struct {
		name, recorded, signIn, want string
	}{
		{"the address it signed in with", "Player@example.test", "player@example.test", ""},
		{"another address", "recorded@example.test", "player@example.test", "recorded@example.test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, a, p := playersFixture(t)
			ctx := context.Background()
			ann, err := a.AddMember(ctx, Member{Name: "ann", Email: tt.recorded, Zone: "UTC"})
			if err != nil {
				t.Fatalf("AddMember: %v", err)
			}
			l := emailSignedIn(t, p, tt.signIn, "token")
			if l.Member == nil {
				approved(t, a, p, l, "ann")
			}
			if err := p.DeleteAccount(ctx, tokenHash("token"), signInAt); err != nil {
				t.Fatalf("DeleteAccount: %v", err)
			}
			if got := rowsOf(t, pool, fmt.Sprintf("SELECT email FROM members WHERE member_id = %d", ann)); len(got) != 1 || got[0] != tt.want {
				t.Errorf("ann's email = %q, want %q", got, tt.want)
			}
			if got := rowsOf(t, pool, "SELECT address FROM email_links"); len(got) != 0 {
				t.Errorf("links left = %q, want none", got)
			}
		})
	}
}

func TestApprovingAnEmailSignInsRequestRecordsTheAddressOnANewMember(t *testing.T) {
	_, a, p := playersFixture(t)
	ctx := context.Background()
	approved(t, a, p, emailSignedIn(t, p, "player@example.test", "token"), "Newcomer")
	members, err := a.Members(ctx)
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	if len(members) != 1 || members[0].Email != "player@example.test" || members[0].DiscordName != "" {
		t.Errorf("members = %+v, want Newcomer with the address and no Discord name", members)
	}
}
