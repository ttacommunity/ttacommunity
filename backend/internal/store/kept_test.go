//go:build integration

package store

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// keptColumns is every column of a table about a player: what "Data we keep" on the account page shows it as, or
// why it is left out. A new column fails TestEveryColumnAboutAPlayerIsShownOrLeftOut until it is listed here.
var keptColumns = map[string]string{
	"accounts.account_id":             "omitted: our own number for the account",
	"accounts.member_id":              "shown: as the player record it is linked to",
	"accounts.linked_how":             "shown: how the account was linked",
	"accounts.linked_at":              "shown: when the account was linked",
	"accounts.auto_link":              "shown: whether a sign-in may link it by a matching name or email",
	"accounts.country_code":           "shown: the country given before a link",
	"accounts.zone":                   "shown: the time zone given before a link",
	"accounts.created_at":             "shown: when the account was made",
	"logins.login_id":                 "omitted: our own number for the way in",
	"logins.account_id":               "omitted: the account it belongs to, theirs",
	"logins.provider":                 "shown: each way in",
	"logins.subject":                  "shown: each way in's id",
	"logins.username":                 "shown: each way in's name",
	"logins.display_name":             "shown: each way in's display name",
	"logins.email":                    "shown: the email each way in gave us",
	"logins.created_at":               "shown: when each way in was added",
	"logins.signed_in_at":             "shown: when each way in was last used",
	"members.member_id":               "omitted: our own number for the record",
	"members.tta_name":                "shown: the TTA username",
	"members.cge_user":                "shown: the CGE user number",
	"members.player_id":               "shown: as the name on the ratings pages",
	"members.discord_name":            "shown: the recorded Discord name",
	"members.email":                   "shown: the recorded email",
	"members.country_code":            "shown: the record's country",
	"members.zone":                    "shown: the record's time zone",
	"members.banned":                  "shown: a ban",
	"members.ban_reason":              "shown: the ban's reason",
	"members.banned_at":               "shown: when the ban began",
	"members.created_at":              "shown: when the record was made",
	"link_requests.request_id":        "omitted: our own number for the request",
	"link_requests.account_id":        "omitted: the account that asked, theirs",
	"link_requests.tta_name":          "shown: the name asked for",
	"link_requests.status":            "shown: the answer",
	"link_requests.reason":            "shown: the reason for a refusal",
	"link_requests.requested_at":      "shown: when it was asked",
	"link_requests.decided_at":        "shown: when it was answered",
	"player_sessions.token_hash":      "omitted: a scrambled copy of the session's secret, which only proves a session",
	"player_sessions.login_id":        "shown: as the way in the session signed in by",
	"player_sessions.created_at":      "shown: when the session started",
	"player_sessions.expires_at":      "shown: when the session ends and is removed",
	"email_links.token_hash":          "omitted: a scrambled copy of the link's secret, which only proves a link",
	"email_links.account_id":          "omitted: the account the address is added to, theirs",
	"email_links.address":             "shown: where the link was sent",
	"email_links.return_path":         "shown: the page it returns to",
	"email_links.created_at":          "shown: when it was sent",
	"email_links.expires_at":          "shown: until when it works",
	"email_links.used_at":             "shown: when it was used",
	"blitz_entries.entry_id":          "omitted: public on the tournament's page",
	"blitz_entries.tournament_id":     "omitted: public on the tournament's page",
	"blitz_entries.member_id":         "omitted: public on the tournament's page",
	"blitz_entries.status":            "omitted: public on the tournament's page",
	"blitz_entries.joined_at":         "omitted: public on the tournament's page",
	"blitz_entries.starting_rank":     "omitted: the tournament's pairing order, not about the player; its effect is public in the pairings",
	"challenge_entries.entry_id":      "omitted: public on the Challenges pages",
	"challenge_entries.challenge_id":  "omitted: public on the Challenges pages",
	"challenge_entries.member_id":     "omitted: public on the Challenges pages",
	"challenge_entries.name":          "omitted: public on the Challenges pages",
	"challenge_entries.domination":    "omitted: public on the Challenges pages",
	"challenge_entries.proof_url":     "omitted: public on the Challenges pages",
	"challenge_entries.created_at":    "omitted: public on the Challenges pages",
	"challenge_entries.bugged":        "omitted: public on the Challenges pages",
	"challenge_entries.hidden":        "omitted: an admin's mark on the entry",
	"challenge_entries.hidden_reason": "omitted: an admin's note on the entry",
	"challenge_entries.checked":       "omitted: an admin's mark that they looked at the proof",
	"challenge_entries.source":        "omitted: where an imported entry came from",
	"challenge_entries.account_id":    "omitted: the account that submitted it, theirs",
}

func TestEveryColumnAboutAPlayerIsShownOrLeftOut(t *testing.T) {
	pool := migrated(t)
	got := rowsOf(t, pool, `
		WITH about AS (
			SELECT unnest(ARRAY['accounts', 'logins', 'members']::regclass[]) AS rel
			UNION
			SELECT conrelid FROM pg_constraint
			WHERE contype = 'f' AND confrelid = ANY (ARRAY['accounts', 'logins', 'members']::regclass[]))
		SELECT c.relname || '.' || a.attname
		FROM about JOIN pg_class c ON c.oid = about.rel
		JOIN pg_attribute a ON a.attrelid = about.rel AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY 1`)
	for _, column := range got {
		if !strings.HasPrefix(keptColumns[column], "shown: ") && !strings.HasPrefix(keptColumns[column], "omitted: ") {
			t.Errorf("%s is kept about a player, but keptColumns = %q, want \"shown: …\" or \"omitted: <why>\"", column, keptColumns[column])
		}
	}
	for column := range keptColumns {
		if !slices.Contains(got, column) {
			t.Errorf("keptColumns lists %s, which is not a column of a table about a player", column)
		}
	}
}

// keptAt is when the tests read what is kept: a day after signInAt.
var keptAt = signInAt.Add(24 * time.Hour)

// keptBy reads what is kept about the player whose session token is token, at keptAt, failing the test on an error.
func keptBy(t *testing.T, p *PlayerStore, token string) KeptData {
	t.Helper()
	k, err := p.KeptData(context.Background(), tokenHash(token), keptAt)
	if err != nil {
		t.Fatalf("KeptData(%s): %v", token, err)
	}
	return k
}

func TestKeptDataHoldsEveryDetailOfALinkedAccount(t *testing.T) {
	pool, a, p := playersFixture(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "INSERT INTO player_accounts (cge_user, player_id, name) SELECT 501, player_id, 'Bob' FROM players WHERE name = 'Bob'"); err != nil {
		t.Fatalf("adding a CGE account: %v", err)
	}
	bob, err := a.AddMember(ctx, Member{Name: "bob", Email: "bob@example.test", Country: "US", Zone: "America/New_York"})
	if err != nil {
		t.Fatalf("AddMember(bob): %v", err)
	}
	if _, err := a.AddMember(ctx, Member{Name: "ann", DiscordName: "ann_d", Email: "ann@example.test", Zone: "UTC"}); err != nil {
		t.Fatalf("AddMember(ann): %v", err)
	}
	l := signedIn(t, p, discordID("100000000000000001", "player_one", "Player One"), "discord")
	if err := p.RequestLink(ctx, l.ID, "bob", signInAt.Add(time.Hour)); err != nil {
		t.Fatalf("RequestLink: %v", err)
	}
	if _, err := a.ApproveLink(ctx, pendingRequest(t, a, l), signInAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("ApproveLink: %v", err)
	}
	if err := p.AddLogin(ctx, tokenHash("discord"), googleID("100000000000000000002", "Bob B", "bob@gmail.example"), signInAt.Add(3*time.Hour)); err != nil {
		t.Fatalf("AddLogin(google): %v", err)
	}
	if err := a.BanMember(ctx, bob, "Abusive"); err != nil {
		t.Fatalf("BanMember: %v", err)
	}

	k := keptBy(t, p, "discord")
	linked := signInAt.Add(2 * time.Hour)
	if diff := cmp.Diff(KeptAccount{CreatedAt: signInAt, LinkedHow: "approved", LinkedAt: &linked, AutoLink: true}, k.Account); diff != "" {
		t.Errorf("account (-want +got):\n%s", diff)
	}
	wantLogins := []KeptLogin{
		{Provider: "discord", Subject: "100000000000000001", Username: "player_one", DisplayName: "Player One", CreatedAt: signInAt, SignedInAt: signInAt},
		{Provider: "google", Subject: "100000000000000000002", Username: "Bob B", Email: "bob@gmail.example",
			CreatedAt: signInAt.Add(3 * time.Hour), SignedInAt: signInAt.Add(3 * time.Hour)},
	}
	if diff := cmp.Diff(wantLogins, k.Logins); diff != "" {
		t.Errorf("logins (-want +got):\n%s", diff)
	}
	cge := 501
	wantMember := &Member{Name: "Bob", CGEUser: &cge, Player: "Bob", DiscordName: "player_one", Email: "bob@example.test",
		Country: "US", Zone: "America/New_York", Banned: true, BanReason: "Abusive"}
	if diff := cmp.Diff(wantMember, k.Member, cmpopts.IgnoreFields(Member{}, "ID", "PlayerID", "BannedAt", "CreatedAt")); diff != "" {
		t.Errorf("member (-want +got):\n%s", diff)
	}
	if k.Member != nil && (k.Member.BannedAt == nil || k.Member.CreatedAt.IsZero()) {
		t.Errorf("member's banned_at = %v and created_at = %v, want both kept", k.Member.BannedAt, k.Member.CreatedAt)
	}
	want := []KeptRequest{{Name: "bob", Status: "approved", RequestedAt: signInAt.Add(time.Hour), DecidedAt: &linked}}
	if diff := cmp.Diff(want, k.Requests); diff != "" {
		t.Errorf("requests (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]KeptSession{{Provider: "discord", CreatedAt: signInAt, Ends: signInAt.Add(SessionLife), ThisBrowser: true}}, k.Sessions); diff != "" {
		t.Errorf("sessions (-want +got):\n%s", diff)
	}
}

func TestKeptDataSaysWhetherDeletingTheAccountClearsTheMembersEmail(t *testing.T) {
	tests := []struct {
		name   string
		email  string // the member's, as an admin recorded it
		add    Identity
		addVia string // "link" adds add's address as an email way in, "login" adds add as a way in
		want   bool
	}{
		{"an email an admin recorded", "ann@example.test", googleID("100000000000000000002", "Ann", "ann@gmail.example"), "login", false},
		{"the email way in's address", "", Identity{Provider: "email", Subject: "ann@example.test"}, "link", true},
		{"the address Google gave", "", googleID("100000000000000000002", "Ann", "ann@gmail.example"), "login", true},
		{"an admin's, the same as Google's", "ann@gmail.example", googleID("100000000000000000002", "Ann", "ann@gmail.example"), "login", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, a, p := playersFixture(t)
			ctx := context.Background()
			ann, err := a.AddMember(ctx, Member{Name: "ann", DiscordName: "player_one", Email: tt.email, Zone: "UTC"})
			if err != nil {
				t.Fatalf("AddMember: %v", err)
			}
			l := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "discord")
			if tt.addVia == "link" {
				if err := p.NewEmailLink(ctx, tt.add.Subject, &l.ID, "/account", tokenHash("link"), signInAt); err != nil {
					t.Fatalf("NewEmailLink: %v", err)
				}
				if _, err := p.UseEmailLink(ctx, tokenHash("link"), tokenHash("email"), signInAt); err != nil {
					t.Fatalf("UseEmailLink: %v", err)
				}
			} else if err := p.AddLogin(ctx, tokenHash("discord"), tt.add, signInAt); err != nil {
				t.Fatalf("AddLogin: %v", err)
			}
			if got := keptBy(t, p, "discord").MemberEmailCleared; got != tt.want {
				t.Errorf("MemberEmailCleared = %v, want %v", got, tt.want)
			}
			if err := p.DeleteAccount(ctx, tokenHash("discord"), keptAt); err != nil {
				t.Fatalf("DeleteAccount: %v", err)
			}
			var email string
			if err := pool.QueryRow(ctx, "SELECT email FROM members WHERE member_id = $1", ann).Scan(&email); err != nil {
				t.Fatalf("reading the member: %v", err)
			}
			if cleared := email == ""; cleared != tt.want {
				t.Errorf("after Delete account, the member's email = %q; cleared %v, want %v as KeptData said", email, cleared, tt.want)
			}
		})
	}
}

func TestKeptDataShowsTheDetailsGivenBeforeALink(t *testing.T) {
	_, _, p := playersFixture(t)
	ctx := context.Background()
	l := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "discord")
	nz, auckland := "NZ", "Pacific/Auckland"
	if err := p.SetOwnDetails(ctx, l.ID, &nz, &auckland); err != nil {
		t.Fatalf("SetOwnDetails: %v", err)
	}
	if err := p.RequestLink(ctx, l.ID, "carol", signInAt.Add(time.Hour)); err != nil {
		t.Fatalf("RequestLink: %v", err)
	}
	k := keptBy(t, p, "discord")
	if diff := cmp.Diff(KeptAccount{CreatedAt: signInAt, Country: "NZ", Zone: "Pacific/Auckland", AutoLink: true}, k.Account); diff != "" {
		t.Errorf("account (-want +got):\n%s", diff)
	}
	if k.Member != nil {
		t.Errorf("member = %+v, want none before a link", k.Member)
	}
	if diff := cmp.Diff([]KeptRequest{{Name: "carol", Status: "pending", RequestedAt: signInAt.Add(time.Hour)}}, k.Requests); diff != "" {
		t.Errorf("requests (-want +got):\n%s", diff)
	}
}

func TestKeptDataListsLiveSessionsAndRemovesEndedOnes(t *testing.T) {
	pool, _, p := playersFixture(t)
	ctx := context.Background()
	one := discordID("100000000000000001", "player_one", "")
	for _, s := range []struct {
		id    Identity
		token string
		at    time.Time
	}{
		{one, "phone", signInAt.Add(-10 * 24 * time.Hour)},
		{discordID("100000000000000002", "player_two", ""), "other", signInAt.Add(-10 * 24 * time.Hour)},
		{one, "laptop", signInAt},
	} {
		if _, err := p.SignIn(ctx, s.id, tokenHash(s.token), s.at); err != nil {
			t.Fatalf("SignIn(%s): %v", s.token, err)
		}
	}
	at := signInAt.Add(SessionLife - 5*24*time.Hour)
	k, err := p.KeptData(ctx, tokenHash("laptop"), at)
	if err != nil {
		t.Fatalf("KeptData: %v", err)
	}
	if diff := cmp.Diff([]KeptSession{{Provider: "discord", CreatedAt: signInAt, Ends: signInAt.Add(SessionLife), ThisBrowser: true}}, k.Sessions); diff != "" {
		t.Errorf("sessions (-want +got):\n%s", diff)
	}
	for token, want := range map[string]int{"phone": 0, "other": 1} {
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM player_sessions WHERE token_hash = $1", tokenHash(token)).Scan(&n); err != nil {
			t.Fatalf("counting %s: %v", token, err)
		}
		if n != want {
			t.Errorf("after reading what is kept, sessions %q = %d, want %d", token, n, want)
		}
	}
}

func TestKeptDataListsTheAccountsRecentEmailLinks(t *testing.T) {
	_, _, p := playersFixture(t)
	ctx := context.Background()
	l := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "discord")
	send := func(address string, account *int64, back, token string, at time.Time) {
		t.Helper()
		if err := p.NewEmailLink(ctx, address, account, back, tokenHash(token), at); err != nil {
			t.Fatalf("NewEmailLink(%s): %v", token, err)
		}
	}
	t0 := keptAt.Add(-49 * time.Hour)
	send("one@example.test", &l.ID, "/account", "old", t0)
	send("one@example.test", &l.ID, "/account", "added", t0.Add(2*time.Hour))
	if _, err := p.UseEmailLink(ctx, tokenHash("added"), tokenHash("email"), t0.Add(2*time.Hour+time.Minute)); err != nil {
		t.Fatalf("UseEmailLink: %v", err)
	}
	send("one@example.test", nil, "/tournaments", "sign-in", t0.Add(47*time.Hour))
	send("two@example.test", nil, "/", "someone else's", t0.Add(47*time.Hour))

	used := t0.Add(2*time.Hour + time.Minute)
	want := []KeptEmailLink{
		{Address: "one@example.test", ReturnPath: "/account", CreatedAt: t0.Add(2 * time.Hour), ExpiresAt: t0.Add(2*time.Hour + EmailLinkLife),
			UsedAt: &used, KeptUntil: t0.Add(50 * time.Hour)},
		{Address: "one@example.test", ReturnPath: "/tournaments", CreatedAt: t0.Add(47 * time.Hour), ExpiresAt: t0.Add(47*time.Hour + EmailLinkLife),
			KeptUntil: t0.Add(95 * time.Hour)},
	}
	if diff := cmp.Diff(want, keptBy(t, p, "discord").EmailLinks); diff != "" {
		t.Errorf("email links (-want +got):\n%s", diff)
	}
}

func TestKeptDataNeedsALiveSession(t *testing.T) {
	tests := []struct {
		name  string
		token string
		at    time.Time
	}{
		{"an unknown session", "nobody", keptAt},
		{"an ended session", "discord", signInAt.Add(SessionLife + time.Minute)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, p := playersFixture(t)
			signedIn(t, p, discordID("100000000000000001", "player_one", ""), "discord")
			if _, err := p.KeptData(context.Background(), tokenHash(tt.token), tt.at); !errors.Is(err, ErrNoSession) {
				t.Errorf("KeptData = %v, want ErrNoSession", err)
			}
		})
	}
}

func TestOnlyThePlayerLoginReadsItsOwnMember(t *testing.T) {
	pool := migrated(t)
	for name, login := range map[string]*pgxpool.Pool{"ratings_read": asReader(t, pool), "ratings_admin": asAdminLogin(t, pool)} {
		_, err := login.Exec(context.Background(), "SELECT * FROM own_member('\\x00', now())")
		if pgErr := (*pgconn.PgError)(nil); !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Errorf("own_member as %s = %v, want permission denied (42501)", name, err)
		}
	}
}

func TestOwnMemberAnswersOnlyALiveSession(t *testing.T) {
	_, a, p := playersFixture(t)
	ctx := context.Background()
	if _, err := a.AddMember(ctx, Member{Name: "ann", DiscordName: "player_one", Zone: "UTC"}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	signedIn(t, p, discordID("100000000000000001", "player_one", ""), "discord")
	tests := []struct {
		name  string
		token string
		at    time.Time
		want  int
	}{
		{"its live session", "discord", keptAt, 1},
		{"its ended session", "discord", signInAt.Add(SessionLife), 0},
		{"no session", "nobody", keptAt, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var n int
			if err := p.pool.QueryRow(ctx, "SELECT count(*) FROM own_member($1, $2)", tokenHash(tt.token), tt.at).Scan(&n); err != nil {
				t.Fatalf("own_member: %v", err)
			}
			if n != tt.want {
				t.Errorf("own_member rows = %d, want %d", n, tt.want)
			}
		})
	}
}

func TestMigration42KeepsOnlyFieldNamesInMemberAudits(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 41); return err }); err != nil {
		t.Fatalf("migrating down to 41: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO members (tta_name, zone) VALUES ('Ann', 'UTC')`,
		`INSERT INTO admin_audit (discord_id, action, target, detail) VALUES
			('111111111111111111', 'member.add', '7', '{"name": "Ann", "discord_name": "ann_d", "email": "ann@example.test", "country": "NZ", "zone": "UTC"}'),
			('111111111111111111', 'member.set', '7', '{"email": "new@example.test", "cge_user": 501}'),
			('111111111111111111', 'blitz.entrants.add', 'us_open/2026', '{"entrants": [{"name": "Ann", "email": "ann@example.test"}, {"member": 7, "zone": "UTC"}]}'),
			('111111111111111111', 'blitz.entrant.set', '3', '{"discord_name": "ann_d", "status": "dropped"}'),
			('111111111111111111', 'link.approve', '4', '{"Member": "ann", "Created": false}'),
			('111111111111111111', 'link.approve', '5', '{"Member": "Removed Since", "Created": true}'),
			('111111111111111111', 'member.ban', '7', '{"reason": "Abusive"}')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("building the old shape: %v\n%s", err, q)
		}
	}
	if err := migrateRatingsTo(ctx, pool, 42); err != nil {
		t.Fatalf("migrateRatingsTo(42): %v", err)
	}
	ann := rowsOf(t, pool, "SELECT member_id::text FROM members WHERE tta_name = 'Ann'")
	got := rowsOf(t, pool, "SELECT action || ' ' || target || ' ' || detail::text FROM admin_audit ORDER BY audit_id")
	want := []string{
		`member.add 7 {"set": ["country", "discord_name", "email", "name", "zone"]}`,
		`member.set 7 {"set": ["cge_user", "email"]}`,
		`blitz.entrants.add us_open/2026 {"set": ["email", "member", "name", "zone"], "entrants": 2}`,
		`blitz.entrant.set 3 {"set": ["discord_name", "status"]}`,
		`link.approve 4 {"member": ` + strings.Join(ann, "") + `, "created": false}`,
		`link.approve 5 {"member": null, "created": true}`,
		`member.ban 7 {"reason": "Abusive"}`,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("audit after the migration (-want +got):\n%s", diff)
	}
}
