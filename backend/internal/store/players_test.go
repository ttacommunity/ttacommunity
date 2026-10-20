//go:build integration

package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
	return pool, a, NewPlayerStore(asPlayerLogin(t, pool))
}

// signedIn signs id in with a session for token, failing the test on an error.
func signedIn(t *testing.T, p *PlayerStore, id Identity, token string) Login {
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
		{"the member already has a Discord login", []Member{{Name: "ann", DiscordName: "player_one"}}, func(t *testing.T, a *AdminStore, p *PlayerStore) {
			other := signedIn(t, p, discordID("100000000000000009", "other_account", ""), "other")
			approved(t, a, p, other.ID, "ann")
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
		"UPDATE members SET banned = false", "DELETE FROM members", "DELETE FROM logins", "SELECT email FROM members",
		"UPDATE admins SET role = 'admin'", "CREATE TABLE x (y int)",
	} {
		if _, err := p.pool.Exec(context.Background(), q); err == nil {
			t.Errorf("ratings_player ran %q, want permission denied", q)
		}
	}
}

func TestThePublicReaderCannotReadSignIns(t *testing.T) {
	reader := asReader(t, migrated(t))
	for _, table := range []string{"logins", "link_requests", "player_sessions"} {
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
		"GRANT UPDATE (banned) ON members TO ratings_player", "GRANT DELETE ON logins TO ratings_player",
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
	var logins int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM logins").Scan(&logins); err != nil || logins != 0 {
		t.Errorf("logins left = %d (%v), want 0", logins, err)
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
	if who != "" || action != "login.delete-own" || target != fmt.Sprint(l.ID) || detail != fmt.Sprintf(`{"member": %d}`, ann) {
		t.Errorf("audit row = %q %q %q %q, want no one, login.delete-own, login %d, member %d", who, action, target, detail, l.ID, ann)
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
		t.Errorf("audit row = %q %q, want login %d with no member", target, detail, l.ID)
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
