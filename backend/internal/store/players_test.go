//go:build integration

package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

var signInAt = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

// tokenHash stands in for a session token's hash: the store keeps only hashes.
func tokenHash(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// discordID is an invented Discord account with the username and display name given.
func discordID(subject, username, display string) Identity {
	return Identity{Provider: "discord", Subject: subject, Username: username, DisplayName: display}
}

// playersFixture is blitzFixture's database with a player store on the player login.
func playersFixture(t *testing.T) (*pgxpool.Pool, *AdminStore, *PlayerStore) {
	t.Helper()
	pool, a := blitzFixture(t)
	// Accounts' ids start far from logins', so a test mixing the two up fails.
	if _, err := pool.Exec(context.Background(), "SELECT setval(pg_get_serial_sequence('accounts', 'account_id'), 1000)"); err != nil {
		t.Fatalf("moving accounts' ids on: %v", err)
	}
	return pool, a, NewPlayerStore(asPlayerLogin(t, pool))
}

// signedIn signs id in with a session for token, failing the test on an error.
func signedIn(t *testing.T, p *PlayerStore, id Identity, token string) Account {
	t.Helper()
	ctx := context.Background()
	if _, err := p.SignIn(ctx, id, tokenHash(token), signInAt); err != nil {
		t.Fatalf("SignIn(%s): %v", id.Username, err)
	}
	l, err := p.Session(ctx, tokenHash(token), signInAt)
	if err != nil {
		t.Fatalf("Session after signing %s in: %v", id.Username, err)
	}
	return l
}

func TestSignInRecordsALoginAndKeepsItsNamesCurrent(t *testing.T) {
	_, _, p := playersFixture(t)
	ctx := context.Background()
	first := signedIn(t, p, discordID("100000000000000001", "player_one", "Player One"), "token-1")
	if first.Username != "player_one" || first.DisplayName != "Player One" || first.Member != nil {
		t.Errorf("first sign-in = %+v, want player_one, Player One, no member", first)
	}
	again := signedIn(t, p, discordID("100000000000000001", "player_renamed", ""), "token-2")
	if again.ID != first.ID || again.Username != "player_renamed" || again.DisplayName != "" {
		t.Errorf("second sign-in = %+v, want login %d renamed player_renamed with no display name", again, first.ID)
	}
	if _, err := p.Session(ctx, tokenHash("token-1"), signInAt); err != nil {
		t.Errorf("the first session after a second sign-in: %v, want it kept", err)
	}
}

func TestSignInLinksTheOneMemberWhoseDiscordNameMatches(t *testing.T) {
	tests := []struct {
		name    string
		members []Member
		setup   func(t *testing.T, a *AdminStore, p *PlayerStore)
		want    string
	}{
		{"the same name in another case", []Member{{Name: "ann", DiscordName: "Player_One"}}, nil, "ann"},
		{"a recorded name with a leading @", []Member{{Name: "ann", DiscordName: "@player_one"}}, nil, "ann"},
		{"no member with that name", []Member{{Name: "ann", DiscordName: "someone_else"}}, nil, ""},
		{"two members with that name", []Member{{Name: "ann", DiscordName: "player_one"}, {Name: "cat", DiscordName: "player_one"}}, nil, ""},
		{"the member is banned", []Member{{Name: "ann", DiscordName: "player_one"}}, func(t *testing.T, a *AdminStore, _ *PlayerStore) {
			if err := a.BanMember(context.Background(), memberID(t, a, "ann"), "test"); err != nil {
				t.Fatalf("BanMember: %v", err)
			}
		}, "ann"},
		{"another account is linked to the member", []Member{{Name: "ann", DiscordName: "player_one"}}, func(t *testing.T, a *AdminStore, p *PlayerStore) {
			other := signedIn(t, p, discordID("100000000000000009", "other_account", ""), "other")
			approved(t, a, p, other, "ann")
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
			got := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "token")
			linked := ""
			if got.Member != nil {
				linked = got.Member.Name
			}
			if linked != tt.want {
				t.Errorf("signing in player_one linked %q, want %q", linked, tt.want)
			}
		})
	}
}

func TestSignInSaysWhetherItIsTheFirstForThatAccount(t *testing.T) {
	_, _, p := playersFixture(t)
	ctx := context.Background()
	for i, want := range []bool{true, false, false} {
		at := signInAt.Add(time.Duration(i) * time.Hour)
		first, err := p.SignIn(ctx, discordID("100000000000000001", "player_one", ""), tokenHash(fmt.Sprint("token-", i)), at)
		if err != nil {
			t.Fatalf("sign-in %d: %v", i+1, err)
		}
		if first != want {
			t.Errorf("sign-in %d: first = %v, want %v", i+1, first, want)
		}
	}
	if first, err := p.SignIn(ctx, discordID("100000000000000002", "player_two", ""), tokenHash("other"), signInAt); err != nil || !first {
		t.Errorf("another account's first sign-in = %v (err %v), want first", first, err)
	}
}

func TestABannedMemberSignsInAndIsToldOfTheBan(t *testing.T) {
	_, a, p := playersFixture(t)
	ctx := context.Background()
	if _, err := a.AddMember(ctx, Member{Name: "ann", DiscordName: "player_one", Zone: "UTC"}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	id := discordID("100000000000000001", "player_one", "")
	if l := signedIn(t, p, id, "before"); l.Banned || l.BanReason != "" {
		t.Errorf("before the ban: banned %v, reason %q, want neither", l.Banned, l.BanReason)
	}
	if err := a.BanMember(ctx, memberID(t, a, "ann"), "Left games unfinished"); err != nil {
		t.Fatalf("BanMember: %v", err)
	}
	signedIn(t, p, id, "after")
	for _, token := range []string{"before", "after"} {
		l, err := p.Session(ctx, tokenHash(token), signInAt)
		if err != nil {
			t.Fatalf("session %q after the ban: %v", token, err)
		}
		if l.Member == nil || !l.Banned || l.BanReason != "Left games unfinished" {
			t.Errorf("session %q after the ban: member %+v, banned %v, reason %q, want ann, banned, the reason", token, l.Member, l.Banned, l.BanReason)
		}
	}
}

func TestSessionsEndAfterAMonthUnusedAndAreRenewedByUse(t *testing.T) {
	tests := []struct {
		name  string
		at    []time.Duration // when the session is used, after signing in
		check time.Duration
		ended bool
	}{
		{"used the next day", nil, 24 * time.Hour, false},
		{"unused for longer than the session lasts", nil, SessionLife + time.Minute, true},
		{"used every three weeks", []time.Duration{21 * 24 * time.Hour, 42 * 24 * time.Hour}, 63 * 24 * time.Hour, false},
		{"used once, then unused for longer than it lasts", []time.Duration{2 * 24 * time.Hour}, 2*24*time.Hour + SessionLife + time.Minute, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, p := playersFixture(t)
			ctx := context.Background()
			signedIn(t, p, discordID("100000000000000001", "player_one", ""), "token")
			for _, d := range tt.at {
				if _, err := p.Session(ctx, tokenHash("token"), signInAt.Add(d)); err != nil {
					t.Fatalf("Session after %v: %v", d, err)
				}
			}
			_, err := p.Session(ctx, tokenHash("token"), signInAt.Add(tt.check))
			if ended := errors.Is(err, ErrNoSession); ended != tt.ended {
				t.Errorf("Session after %v = %v, want ended %v", tt.check, err, tt.ended)
			}
		})
	}
}

func TestSignOutEndsOnlyThatSession(t *testing.T) {
	_, _, p := playersFixture(t)
	ctx := context.Background()
	id := discordID("100000000000000001", "player_one", "")
	signedIn(t, p, id, "phone")
	signedIn(t, p, id, "laptop")
	if err := p.SignOut(ctx, tokenHash("phone")); err != nil {
		t.Fatalf("SignOut: %v", err)
	}
	if _, err := p.Session(ctx, tokenHash("phone"), signInAt); !errors.Is(err, ErrNoSession) {
		t.Errorf("the signed-out session = %v, want ErrNoSession", err)
	}
	if _, err := p.Session(ctx, tokenHash("laptop"), signInAt); err != nil {
		t.Errorf("the other session = %v, want it kept", err)
	}
	if err := p.SignOut(ctx, tokenHash("phone")); err != nil {
		t.Errorf("signing out again = %v, want nil", err)
	}
}

func TestAnUnlinkedLoginRequestsAName(t *testing.T) {
	_, _, p := playersFixture(t)
	ctx := context.Background()
	l := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "token")
	if err := p.RequestLink(ctx, l.ID, " Bob ", signInAt); err != nil {
		t.Fatalf("RequestLink: %v", err)
	}
	if err := p.RequestLink(ctx, l.ID, "Robert", signInAt.Add(time.Hour)); err != nil {
		t.Fatalf("RequestLink again: %v", err)
	}
	got, err := p.Session(ctx, tokenHash("token"), signInAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if got.Request == nil || got.Request.Name != "Robert" || got.Request.Status != "pending" {
		t.Errorf("request = %+v, want Robert pending (the second replacing the first)", got.Request)
	}
	for _, name := range []string{" ", string(make([]rune, 101))} {
		if err := p.RequestLink(ctx, l.ID, name, signInAt); !errors.Is(err, ErrBadLinkName) {
			t.Errorf("RequestLink(%q) = %v, want ErrBadLinkName", name, err)
		}
	}
}

func TestALinkedLoginCannotRequestAName(t *testing.T) {
	_, a, p := playersFixture(t)
	ctx := context.Background()
	if _, err := a.AddMember(ctx, Member{Name: "ann", DiscordName: "player_one", Zone: "UTC"}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	l := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "token")
	if err := p.RequestLink(ctx, l.ID, "Bob", signInAt); !errors.Is(err, ErrAlreadyLinked) {
		t.Errorf("RequestLink when linked = %v, want ErrAlreadyLinked", err)
	}
}

func TestALinkedPlayerSetsTheirOwnCountryAndZone(t *testing.T) {
	de, berlin, none, bad := "de", "Europe/Berlin", "", "Germany"
	tests := []struct {
		name          string
		country, zone *string
		want          error
		wantC, wantZ  string
	}{
		{"a country and a zone", &de, &berlin, nil, "DE", "Europe/Berlin"},
		{"the country cleared, the zone kept", &none, nil, nil, "", "UTC"},
		{"a country that is not a code", &bad, nil, ErrBadMember, "NZ", "UTC"},
		{"an empty zone", nil, &none, ErrBadMember, "NZ", "UTC"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, a, p := playersFixture(t)
			ctx := context.Background()
			if _, err := a.AddMember(ctx, Member{Name: "ann", DiscordName: "player_one", Country: "NZ", Zone: "UTC"}); err != nil {
				t.Fatalf("AddMember: %v", err)
			}
			login := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "linked")
			if err := p.SetOwnDetails(ctx, login.ID, tt.country, tt.zone); !errors.Is(err, tt.want) {
				t.Fatalf("SetOwnDetails = %v, want %v", err, tt.want)
			}
			got, err := p.Session(ctx, tokenHash("linked"), signInAt)
			if err != nil {
				t.Fatalf("Session: %v", err)
			}
			if got.Country != tt.wantC || got.Zone != tt.wantZ {
				t.Errorf("ann's country and zone = %q %q, want %q %q", got.Country, got.Zone, tt.wantC, tt.wantZ)
			}
			if members, _ := a.Members(ctx); members[0].Country != tt.wantC || members[0].Zone != tt.wantZ {
				t.Errorf("the member's country and zone = %q %q, want %q %q", members[0].Country, members[0].Zone, tt.wantC, tt.wantZ)
			}
		})
	}
}

func TestAPlayerNotYetLinkedKeepsTheirOwnDetailsOnTheSignIn(t *testing.T) {
	_, _, p := playersFixture(t)
	ctx := context.Background()
	l := signedIn(t, p, discordID("100000000000000002", "player_two", ""), "token")
	if l.Country != "" || l.Zone != "Unknown" {
		t.Errorf("before any details: country and zone %q %q, want none and Unknown", l.Country, l.Zone)
	}
	nz, auckland, bad := "nz", "Pacific/Auckland", "Germany"
	if err := p.SetOwnDetails(ctx, l.ID, &nz, &auckland); err != nil {
		t.Fatalf("SetOwnDetails: %v", err)
	}
	if err := p.SetOwnDetails(ctx, l.ID, &bad, nil); !errors.Is(err, ErrBadMember) {
		t.Errorf("a country that is not a code = %v, want ErrBadMember", err)
	}
	got, err := p.Session(ctx, tokenHash("token"), signInAt)
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if got.Member != nil || got.Country != "NZ" || got.Zone != "Pacific/Auckland" {
		t.Errorf("after giving details: member %+v, country and zone %q %q, want no member, NZ and Pacific/Auckland", got.Member, got.Country, got.Zone)
	}
}

func TestALinkByDiscordNameMovesTheDetailsGivenToTheMember(t *testing.T) {
	_, a, p := playersFixture(t)
	ctx := context.Background()
	l := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "first")
	nz, auckland := "NZ", "Pacific/Auckland"
	if err := p.SetOwnDetails(ctx, l.ID, &nz, &auckland); err != nil {
		t.Fatalf("SetOwnDetails: %v", err)
	}
	if _, err := a.AddMember(ctx, Member{Name: "ann", DiscordName: "player_one", Country: "DE", Zone: "UTC"}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if got := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "again"); got.Member == nil || got.Country != "NZ" || got.Zone != "Pacific/Auckland" {
		t.Errorf("after the link: member %+v, country and zone %q %q, want ann with NZ and Pacific/Auckland", got.Member, got.Country, got.Zone)
	}
	if members, _ := a.Members(ctx); members[0].Country != "NZ" || members[0].Zone != "Pacific/Auckland" {
		t.Errorf("ann's country and zone = %q %q, want the details the player gave", members[0].Country, members[0].Zone)
	}
}

func TestASignedInAdminIsToldTheirRole(t *testing.T) {
	pool, _, p := playersFixture(t)
	if err := AddAdmin(context.Background(), pool, Admin{DiscordID: "100000000000000001", Name: "An admin", Role: RoleOrganiser}); err != nil {
		t.Fatalf("AddAdmin: %v", err)
	}
	if got := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "admin").AdminRole; got != RoleOrganiser {
		t.Errorf("an organiser's role = %q, want %q", got, RoleOrganiser)
	}
	if got := signedIn(t, p, discordID("100000000000000002", "player_two", ""), "player").AdminRole; got != "" {
		t.Errorf("a player's role = %q, want none", got)
	}
}

func TestThePlayerLoginCannotChangeWhatIsNotItsOwn(t *testing.T) {
	_, _, p := playersFixture(t)
	for _, q := range []string{
		"DELETE FROM games", "INSERT INTO players (name) VALUES ('x')", "UPDATE members SET tta_name = 'x'",
		"UPDATE members SET banned = false", "DELETE FROM members", "DELETE FROM logins", "DELETE FROM accounts", "SELECT email FROM members",
		"UPDATE admins SET role = 'admin'", "CREATE TABLE x (y int)",
	} {
		if _, err := p.pool.Exec(context.Background(), q); err == nil {
			t.Errorf("ratings_player ran %q, want permission denied", q)
		}
	}
}

func TestThePublicReaderCannotReadSignIns(t *testing.T) {
	reader := asReader(t, migrated(t))
	for _, table := range []string{"accounts", "logins", "link_requests", "player_sessions"} {
		if _, err := reader.Exec(context.Background(), "SELECT * FROM "+table); err == nil {
			t.Errorf("ratings_read read %s, want permission denied", table)
		}
	}
}

func TestCheckLimitsAcceptsThePlayerLoginAndRefusesStrongerOnes(t *testing.T) {
	pool := migrated(t)
	if err := NewPlayerStore(asPlayerLogin(t, pool)).CheckLimits(context.Background()); err != nil {
		t.Errorf("CheckLimits as ratings_player: %v, want nil", err)
	}
	if err := NewPlayerStore(asAdminLogin(t, pool)).CheckLimits(context.Background()); !errors.Is(err, ErrPlayerLoginTooStrong) {
		t.Errorf("CheckLimits as ratings_admin = %v, want ErrPlayerLoginTooStrong", err)
	}
	if err := NewPlayerStore(pool).CheckLimits(context.Background()); !errors.Is(err, ErrPlayerLoginTooStrong) {
		t.Errorf("CheckLimits as the owner = %v, want ErrPlayerLoginTooStrong", err)
	}
}

func TestCheckLimitsRefusesAPlayerLoginGivenMore(t *testing.T) {
	for _, grant := range []string{
		"GRANT INSERT ON games TO ratings_player", "GRANT UPDATE ON players TO ratings_player",
		"GRANT INSERT ON members TO ratings_player", "GRANT UPDATE (tta_name) ON members TO ratings_player",
		"GRANT UPDATE (banned) ON members TO ratings_player", "GRANT DELETE ON logins TO ratings_player", "GRANT DELETE ON accounts TO ratings_player",
		"GRANT INSERT ON admins TO ratings_player", "GRANT SELECT (email) ON members TO ratings_player",
		"GRANT CREATE ON SCHEMA public TO ratings_player",
	} {
		t.Run(grant, func(t *testing.T) {
			pool := migrated(t)
			if _, err := pool.Exec(context.Background(), grant); err != nil {
				t.Fatalf("%s: %v", grant, err)
			}
			if err := NewPlayerStore(asPlayerLogin(t, pool)).CheckLimits(context.Background()); !errors.Is(err, ErrPlayerLoginTooStrong) {
				t.Errorf("CheckLimits after %q = %v, want ErrPlayerLoginTooStrong", grant, err)
			}
		})
	}
}

func TestDeletingAnAccountRemovesTheSignInAndClearsItsMembersDetails(t *testing.T) {
	pool, a, p := playersFixture(t)
	ctx := context.Background()
	ann, err := a.AddMember(ctx, Member{Name: "ann", DiscordName: "player_one", Country: "NZ", Zone: "Pacific/Auckland"})
	if err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	id := discordID("100000000000000001", "player_one", "")
	l := signedIn(t, p, id, "phone")
	signedIn(t, p, id, "laptop")
	if err := p.DeleteAccount(ctx, tokenHash("phone"), signInAt); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if _, err := p.Session(ctx, tokenHash("laptop"), signInAt); !errors.Is(err, ErrNoSession) {
		t.Errorf("the other session = %v, want ErrNoSession", err)
	}
	var accounts, logins int
	if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM accounts), (SELECT count(*) FROM logins)").Scan(&accounts, &logins); err != nil || accounts != 0 || logins != 0 {
		t.Errorf("accounts and logins left = %d %d (%v), want 0 and 0", accounts, logins, err)
	}
	if members, _ := a.Members(ctx); members[0].Country != "" || members[0].Zone != "Unknown" || members[0].DiscordName != "" {
		t.Errorf("ann's country, zone and Discord name = %q %q %q, want none, Unknown and none", members[0].Country, members[0].Zone, members[0].DiscordName)
	}
	if again := signedIn(t, p, id, "again"); again.Member != nil {
		t.Errorf("signing in again linked %+v, want no member", again.Member)
	}
	var who, action, target, detail string
	if err := pool.QueryRow(ctx, "SELECT discord_id, action, target, detail::text FROM admin_audit").Scan(&who, &action, &target, &detail); err != nil {
		t.Fatalf("reading the audit: %v", err)
	}
	if who != "" || action != "account.delete-own" || target != fmt.Sprint(l.ID) || detail != fmt.Sprintf(`{"member": %d}`, ann) {
		t.Errorf("audit row = %q %q %q %q, want no one, account.delete-own, account %d, member %d", who, action, target, detail, l.ID, ann)
	}
}

func TestDeletingAnUnlinkedAccountRemovesItsRequestsAndDetails(t *testing.T) {
	pool, _, p := playersFixture(t)
	ctx := context.Background()
	l := signedIn(t, p, discordID("100000000000000002", "player_two", ""), "token")
	other := signedIn(t, p, discordID("100000000000000003", "player_three", ""), "other")
	nz, auckland := "NZ", "Pacific/Auckland"
	if err := p.SetOwnDetails(ctx, l.ID, &nz, &auckland); err != nil {
		t.Fatalf("SetOwnDetails: %v", err)
	}
	for _, r := range []struct {
		login int64
		name  string
	}{{l.ID, "Invented Player"}, {other.ID, "Other Player"}} {
		if err := p.RequestLink(ctx, r.login, r.name, signInAt); err != nil {
			t.Fatalf("RequestLink(%s): %v", r.name, err)
		}
	}
	if err := p.DeleteAccount(ctx, tokenHash("token"), signInAt); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	var logins, requests int
	if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM logins), (SELECT count(*) FROM link_requests)").Scan(&logins, &requests); err != nil {
		t.Fatal(err)
	}
	if logins != 1 || requests != 1 {
		t.Errorf("logins and requests left = %d %d, want the other player's 1 and 1", logins, requests)
	}
	var target, detail string
	if err := pool.QueryRow(ctx, "SELECT target, detail::text FROM admin_audit").Scan(&target, &detail); err != nil {
		t.Fatalf("reading the audit: %v", err)
	}
	if target != fmt.Sprint(l.ID) || detail != "{}" {
		t.Errorf("audit row = %q %q, want account %d with no member", target, detail, l.ID)
	}
}

func TestDeletingAnAccountNeedsALiveSession(t *testing.T) {
	tests := []struct {
		name  string
		token string
		at    time.Time
	}{
		{"an unknown session", "nobody", signInAt},
		{"an ended session", "token", signInAt.Add(SessionLife)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, _, p := playersFixture(t)
			ctx := context.Background()
			signedIn(t, p, discordID("100000000000000002", "player_two", ""), "token")
			if err := p.DeleteAccount(ctx, tokenHash(tt.token), tt.at); !errors.Is(err, ErrNoSession) {
				t.Errorf("DeleteAccount = %v, want ErrNoSession", err)
			}
			var logins, audits int
			if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM logins), (SELECT count(*) FROM admin_audit)").Scan(&logins, &audits); err != nil {
				t.Fatal(err)
			}
			if logins != 1 || audits != 0 {
				t.Errorf("logins and audit rows = %d %d, want 1 and 0", logins, audits)
			}
		})
	}
}

func TestMigration32MakesEachLoginAnAccountAndItsDownUndoesIt(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 31); return err }); err != nil {
		t.Fatalf("migrating down to 31: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO members (tta_name, discord_name, zone) VALUES ('ann', 'player_one', 'UTC')`,
		`INSERT INTO logins (login_id, provider, subject, username, member_id, linked_how, linked_at, created_at, signed_in_at) OVERRIDING SYSTEM VALUE
		 SELECT 40, 'discord', '100000000000000001', 'player_one', member_id, 'match', '2026-10-18T01:00:00Z', '2026-10-18T01:00:00Z', '2026-10-18T01:00:00Z' FROM members`,
		`INSERT INTO logins (login_id, provider, subject, username, auto_link, country_code, zone, created_at, signed_in_at) OVERRIDING SYSTEM VALUE
		 VALUES (41, 'discord', '100000000000000002', 'player_two', false, 'NZ', 'Pacific/Auckland', '2026-10-18T02:00:00Z', '2026-10-18T02:00:00Z')`,
		`INSERT INTO link_requests (login_id, tta_name, requested_at) VALUES (41, 'Newcomer', '2026-10-18T02:05:00Z')`,
		`INSERT INTO player_sessions (token_hash, login_id, created_at, expires_at) VALUES
		 (sha256('one'), 40, '2026-10-18T01:00:00Z', '2026-11-17T01:00:00Z'), (sha256('two'), 41, '2026-10-18T02:00:00Z', '2026-11-17T02:00:00Z')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("building the old shape: %v\n%s", err, q)
		}
	}
	if err := migrateRatingsTo(ctx, pool, 32); err != nil {
		t.Fatalf("migrateRatingsTo(32): %v", err)
	}
	// The store reads today's schema, so the migrations since 32 run before it does.
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.Up(ctx); return err }); err != nil {
		t.Fatalf("migrating up from 32: %v", err)
	}
	p := NewPlayerStore(asPlayerLogin(t, pool))
	one, err := p.Session(ctx, tokenHash("one"), signInAt)
	if err != nil || one.ID != 40 || one.LoginID != 40 || one.Member == nil || one.Member.Name != "ann" {
		t.Errorf("player_one's session = %+v (%v), want account 40 linked to ann", one, err)
	}
	two, err := p.Session(ctx, tokenHash("two"), signInAt)
	if err != nil || two.ID != 41 || two.Country != "NZ" || two.Zone != "Pacific/Auckland" || two.Request == nil || two.Request.Name != "Newcomer" {
		t.Errorf("player_two's session = %+v (%v), want account 41 from NZ asking for Newcomer", two, err)
	}
	if got := rowsOf(t, pool, "SELECT concat_ws('|', account_id, auto_link) FROM accounts ORDER BY 1"); !slices.Equal(got, []string{"40|t", "41|f"}) {
		t.Errorf("accounts = %q, want 40 linking by name and 41 not", got)
	}
	var third int64
	if err := pool.QueryRow(ctx, `
		WITH a AS (INSERT INTO accounts (created_at) VALUES ('2026-10-18T03:00:00Z') RETURNING account_id)
		INSERT INTO logins (account_id, provider, subject, username, created_at, signed_in_at)
		SELECT account_id, 'discord', '100000000000000003', 'player_three', '2026-10-18T03:00:00Z', '2026-10-18T03:00:00Z' FROM a
		RETURNING account_id`).Scan(&third); err != nil {
		t.Fatalf("opening a new account: %v", err)
	}
	if third <= 41 {
		t.Errorf("a new account's id = %d, want one after the migrated accounts'", third)
	}

	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 31); return err }); err != nil {
		t.Fatalf("migrating down to 31 again: %v", err)
	}
	got := rowsOf(t, pool, `SELECT concat_ws('|', l.username, coalesce(m.tta_name, '-'), l.auto_link, coalesce(l.country_code, '-'), coalesce(r.tta_name, '-'))
		FROM logins l LEFT JOIN members m USING (member_id) LEFT JOIN link_requests r USING (login_id) ORDER BY 1`)
	want := []string{"player_one|ann|t|-|-", "player_three|-|t|-|-", "player_two|-|f|NZ|Newcomer"}
	if !slices.Equal(got, want) {
		t.Errorf("after the Down, logins = %q, want %q", got, want)
	}
}

// microsoftID is an invented Microsoft account with the name and verified email given.
func microsoftID(subject, name, email string) Identity {
	return Identity{Provider: "microsoft", Subject: subject, Username: name, Email: email}
}

func TestAMicrosoftSignInKeepsItsNameAndVerifiedEmailCurrent(t *testing.T) {
	tests := []struct {
		name, email, wantEmail string
	}{
		{"an email", "Player@Example.test ", "player@example.test"},
		{"no email", "", ""},
		{"something not an email", "player at example", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, p := playersFixture(t)
			first := signedIn(t, p, microsoftID("AAAAAAAAAAAAAAAAAAAAAIkzqFVrSaSaFHy782bbtaQ", "Player One", "first@example.test"), "token-1")
			again := signedIn(t, p, microsoftID("AAAAAAAAAAAAAAAAAAAAAIkzqFVrSaSaFHy782bbtaQ", "Player Renamed", tt.email), "token-2")
			if again.ID != first.ID || len(again.Logins) != 1 {
				t.Fatalf("second sign-in = %+v, want account %d with one way in", again, first.ID)
			}
			if got := again.Logins[0]; got.Provider != "microsoft" || got.Username != "Player Renamed" || got.Email != tt.wantEmail {
				t.Errorf("the way in = %+v, want microsoft, Player Renamed, email %q", got, tt.wantEmail)
			}
		})
	}
}

func TestAMicrosoftEmailNeverLinksAMember(t *testing.T) {
	_, a, p := playersFixture(t)
	if _, err := a.AddMember(context.Background(), Member{Name: "ann", Email: "player@example.test", Zone: "UTC"}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if l := signedIn(t, p, microsoftID("AAAAAAAAAAAAAAAAAAAAAIkzqFVrSaSaFHy782bbtaQ", "Player One", "player@example.test"), "token"); l.Member != nil {
		t.Errorf("a Microsoft sign-in linked %+v, want no member", l.Member)
	}
}

func TestDeletingAnAccountClearsTheMembersEmailWhenItCameFromMicrosoft(t *testing.T) {
	tests := []struct {
		name, recorded, want string
	}{
		{"filled from Microsoft", "", ""},
		{"recorded before", "recorded@example.test", "recorded@example.test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, a, p := playersFixture(t)
			ctx := context.Background()
			ann, err := a.AddMember(ctx, Member{Name: "ann", Email: tt.recorded, Zone: "UTC"})
			if err != nil {
				t.Fatalf("AddMember: %v", err)
			}
			approved(t, a, p, signedIn(t, p, microsoftID("AAAAAAAAAAAAAAAAAAAAAIkzqFVrSaSaFHy782bbtaQ", "Player One", "player@example.test"), "token"), "ann")
			if err := p.DeleteAccount(ctx, tokenHash("token"), signInAt); err != nil {
				t.Fatalf("DeleteAccount: %v", err)
			}
			if got := rowsOf(t, pool, fmt.Sprintf("SELECT email FROM members WHERE member_id = %d", ann)); len(got) != 1 || got[0] != tt.want {
				t.Errorf("ann's email = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMigration39sDownRemovesMicrosoftSignIns(t *testing.T) {
	pool, _, p := playersFixture(t)
	ctx := context.Background()
	signedIn(t, p, microsoftID("AAAAAAAAAAAAAAAAAAAAAIkzqFVrSaSaFHy782bbtaQ", "Player One", "player@example.test"), "microsoft")
	signedIn(t, p, discordID("100000000000000001", "player_two", ""), "discord")
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 38); return err }); err != nil {
		t.Fatalf("migrating down to 38: %v", err)
	}
	if got := rowsOf(t, pool, "SELECT l.username FROM accounts a JOIN logins l USING (account_id)"); !slices.Equal(got, []string{"player_two"}) {
		t.Errorf("after the Down, accounts' ways in = %q, want player_two alone", got)
	}
}
