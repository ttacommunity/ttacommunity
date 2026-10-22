//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// memberLines writes members as "name cge=<id> player=<name> discord=<name> email=<email> <country> <zone> banned=<reason> in=<n>".
func memberLines(members []Member) []string {
	var out []string
	for _, m := range members {
		cge := "-"
		if m.CGEUser != nil {
			cge = fmt.Sprint(*m.CGEUser)
		}
		banned := "no"
		if m.Banned {
			banned = m.BanReason
		}
		out = append(out, fmt.Sprintf("%s cge=%s player=%q discord=%q email=%q %q %s banned=%s in=%d", m.Name, cge, m.Player, m.DiscordName, m.Email, m.Country, m.Zone, banned, m.Entries))
	}
	return out
}

// withAccount gives the fixture's player Bob a CGE account 501 named "bob" (CGE's own casing), as the owner.
func withAccount(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "INSERT INTO player_accounts (cge_user, player_id, name) SELECT 501, player_id, 'bob' FROM players WHERE name = 'Bob'"); err != nil {
		t.Fatalf("adding Bob's account: %v", err)
	}
}

func TestMembersAreAddedListedChangedBannedAndRemoved(t *testing.T) {
	pool, a := blitzFixture(t)
	ctx := context.Background()
	withAccount(t, pool)
	ann, err := a.AddMember(ctx, Member{Name: " ann ", Zone: "UTC"})
	if err != nil {
		t.Fatalf("AddMember(ann): %v", err)
	}
	bob, err := a.AddMember(ctx, Member{Name: "BOB", Email: "bob@example.test", Country: "US", Zone: "America/New_York"})
	if err != nil {
		t.Fatalf("AddMember(BOB): %v", err)
	}
	got, err := a.Members(ctx)
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	want := []string{
		`ann cge=- player="" discord="" email="" "" UTC banned=no in=0`,
		`bob cge=501 player="Bob" discord="" email="bob@example.test" "US" America/New_York banned=no in=0`,
	}
	if diff := cmp.Diff(want, memberLines(got)); diff != "" {
		t.Errorf("members after adding (-want +got):\n%s", diff)
	}

	for _, tt := range []struct {
		name string
		m    Member
		want error
	}{
		{"a name already a member, in any case", Member{Name: "Bob", Zone: "UTC"}, &MemberExistsError{}},
		{"no name", Member{Name: " ", Zone: "UTC"}, ErrBadMember},
		{"a long name", Member{Name: strings.Repeat("x", 101), Zone: "UTC"}, ErrBadMember},
		{"a country that is not a code", Member{Name: "x", Country: "Germany", Zone: "UTC"}, ErrBadMember},
		{"a code that is no country", Member{Name: "x", Country: "ZZ", Zone: "UTC"}, ErrBadMember},
		{"no zone", Member{Name: "x"}, ErrBadMember},
		{"a long email", Member{Name: "x", Email: strings.Repeat("x", 250) + "@a.test", Zone: "UTC"}, ErrBadMember},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := a.AddMember(ctx, tt.m)
			var exists *MemberExistsError
			if _, ok := tt.want.(*MemberExistsError); ok {
				if !errors.As(err, &exists) || exists.Name != "bob" {
					t.Errorf("AddMember = %v, want a MemberExistsError naming bob", err)
				}
			} else if !errors.Is(err, tt.want) {
				t.Errorf("AddMember = %v, want %v", err, tt.want)
			}
		})
	}

	discord, email, country, zone, name := "ann#1", "ann@example.test", "DE", "Europe/Berlin", "Annie"
	if err := a.SetMember(ctx, ann, MemberChange{DiscordName: &discord, Email: &email, Country: &country, Zone: &zone, Name: &name}); err != nil {
		t.Fatalf("SetMember: %v", err)
	}
	taken := "BOB"
	var exists *MemberExistsError
	if err := a.SetMember(ctx, ann, MemberChange{Name: &taken}); !errors.As(err, &exists) {
		t.Errorf("renaming to a name taken = %v, want a MemberExistsError", err)
	}
	none, bobs := 999, 501
	if err := a.SetMember(ctx, ann, MemberChange{CGEUser: &none}); !errors.Is(err, ErrNoCGEAccount) {
		t.Errorf("a CGE id we do not know = %v, want ErrNoCGEAccount", err)
	}
	if err := a.SetMember(ctx, ann, MemberChange{CGEUser: &bobs}); !errors.Is(err, ErrCGEAccountTaken) {
		t.Errorf("Bob's CGE id on Annie = %v, want ErrCGEAccountTaken", err)
	}
	if err := a.SetMember(ctx, 999, MemberChange{Zone: &zone}); !errors.Is(err, ErrNoMember) {
		t.Errorf("changing an unknown member = %v, want ErrNoMember", err)
	}

	if err := a.BanMember(ctx, ann, " Abusive in Discord "); err != nil {
		t.Fatalf("BanMember: %v", err)
	}
	if err := a.BanMember(ctx, bob, " "); !errors.Is(err, ErrBadMember) {
		t.Errorf("a ban without a reason = %v, want ErrBadMember", err)
	}
	if _, err := a.AddBlitzEntrants(ctx, "us_open", "2026", []BlitzEntrant{{MemberID: ann}}); !errors.Is(err, ErrMemberBanned) {
		t.Errorf("entering a banned member = %v, want ErrMemberBanned", err)
	}
	got, _ = a.Members(ctx)
	want = []string{
		`Annie cge=- player="" discord="ann#1" email="ann@example.test" "DE" Europe/Berlin banned=Abusive in Discord in=0`,
		`bob cge=501 player="Bob" discord="" email="bob@example.test" "US" America/New_York banned=no in=0`,
	}
	if diff := cmp.Diff(want, memberLines(got)); diff != "" {
		t.Errorf("members after the changes (-want +got):\n%s", diff)
	}
	if err := a.UnbanMember(ctx, ann); err != nil {
		t.Fatalf("UnbanMember: %v", err)
	}
	if _, err := a.AddBlitzEntrants(ctx, "us_open", "2026", []BlitzEntrant{{MemberID: ann}}); err != nil {
		t.Fatalf("entering Annie once unbanned: %v", err)
	}
	if got, _ = a.Members(ctx); got[0].Banned || got[0].Entries != 1 {
		t.Errorf("Annie after unban and entry = banned %v in %d, want not banned, in 1", got[0].Banned, got[0].Entries)
	}

	if err := a.RemoveMember(ctx, ann); !errors.Is(err, ErrMemberInUse) {
		t.Errorf("removing a member in an edition = %v, want ErrMemberInUse", err)
	}
	if err := a.RemoveMember(ctx, bob); err != nil {
		t.Errorf("removing a member in no edition = %v, want nil", err)
	}
	if err := a.RemoveMember(ctx, bob); !errors.Is(err, ErrNoMember) {
		t.Errorf("removing a member twice = %v, want ErrNoMember", err)
	}

	reader := asReader(t, pool)
	for _, q := range []string{"SELECT email FROM members", "SELECT discord_name FROM members", "SELECT banned FROM members", "SELECT discord_id FROM members"} {
		if _, err := reader.Exec(ctx, q); err == nil {
			t.Errorf("ratings_read ran %q, want permission denied", q)
		}
	}
	if _, err := reader.Exec(ctx, "SELECT member_id, tta_name, cge_user, player_id, country_code, zone FROM members"); err != nil {
		t.Errorf("ratings_read reading the public columns: %v, want nil", err)
	}
}

func TestLinkMembersAttachesAccountsAndFollowsRenames(t *testing.T) {
	pool, a := blitzFixture(t)
	ctx := context.Background()
	dee, err := a.AddMember(ctx, Member{Name: "dee", Zone: "UTC"})
	if err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if _, err := a.AddMember(ctx, Member{Name: "eve", Zone: "UTC"}); err != nil {
		t.Fatalf("AddMember(eve): %v", err)
	}
	// A load brings Dee's first game: a player and a CGE account under the name as CGE spells it.
	for _, q := range []string{
		"INSERT INTO players (name) VALUES ('Dee')",
		"INSERT INTO player_accounts (cge_user, player_id, name) SELECT 503, player_id, 'Dee' FROM players WHERE name = 'Dee'",
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	linked, err := LinkMembers(ctx, pool)
	if err != nil {
		t.Fatalf("LinkMembers: %v", err)
	}
	if linked.Accounts != 1 || linked.Players != 1 || linked.Renamed != 1 {
		t.Errorf("LinkMembers = %+v, want 1 account, 1 player and 1 rename (dee to Dee)", linked)
	}
	got, _ := a.Members(ctx)
	if want := `Dee cge=503 player="Dee" discord="" email="" "" UTC banned=no in=0`; memberLines(got)[0] != want {
		t.Errorf("Dee after the load = %q, want %q", memberLines(got)[0], want)
	}

	// CGE renames the account: the member follows, being linked by id now.
	if _, err := pool.Exec(ctx, "UPDATE player_accounts SET name = 'DeeDee' WHERE cge_user = 503"); err != nil {
		t.Fatal(err)
	}
	if linked, err = LinkMembers(ctx, pool); err != nil || linked.Renamed != 1 || linked.Accounts != 0 {
		t.Errorf("LinkMembers after a rename = %+v, %v; want 1 rename and nothing else", linked, err)
	}
	if got, _ = a.Members(ctx); got[0].ID != dee || got[0].Name != "DeeDee" {
		t.Errorf("Dee after CGE's rename = %q, want DeeDee", got[0].Name)
	}
	if linked, err = LinkMembers(ctx, pool); err != nil || linked != (Linked{}) {
		t.Errorf("LinkMembers again = %+v, %v; want nothing to do", linked, err)
	}
}

func TestMigration43MovesEachPlayersCountryToTheirMemberAndItsDownCopiesItBack(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 42); return err }); err != nil {
		t.Fatalf("migrating down to 42: %v", err)
	}
	for _, q := range []string{
		"INSERT INTO players (name, country_code) VALUES ('ann', 'PL'), ('bob', 'DE'), ('cat', NULL), ('dan', 'FR'), ('eve', 'AU')",
		"INSERT INTO members (tta_name, player_id, zone) SELECT 'ann', player_id, 'UTC' FROM players WHERE name = 'ann'",
		"INSERT INTO members (tta_name, discord_name, zone) VALUES ('BOB', 'player_two', 'UTC')",
		"INSERT INTO members (tta_name, player_id, country_code, zone) SELECT 'eve', player_id, 'NZ', 'UTC' FROM players WHERE name = 'eve'",
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("building the old shape: %v\n%s", err, q)
		}
	}
	if err := migrateRatingsTo(ctx, pool, 43); err != nil {
		t.Fatalf("migrateRatingsTo(43): %v", err)
	}
	members := func() []string {
		return rowsOf(t, pool, `SELECT concat_ws('|', m.tta_name, coalesce(p.name, '-'), coalesce(m.country_code, '-'), m.zone, m.discord_name)
			FROM members m LEFT JOIN players p ON p.player_id = m.player_id ORDER BY lower(m.tta_name)`)
	}
	want := []string{"ann|ann|PL|UTC|", "BOB|bob|DE|UTC|player_two", "dan|dan|FR|Unknown|", "eve|eve|NZ|UTC|"}
	if diff := cmp.Diff(want, members()); diff != "" {
		t.Errorf("members after the migration (-want +got):\n%s", diff)
	}
	var column bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT FROM information_schema.columns WHERE table_name = 'players' AND column_name = 'country_code')").Scan(&column); err != nil || column {
		t.Errorf("players.country_code after the migration: exists = %v (%v), want gone", column, err)
	}

	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 42); return err }); err != nil {
		t.Fatalf("migrating down to 42 again: %v", err)
	}
	players := rowsOf(t, pool, "SELECT concat_ws('|', name, coalesce(country_code, '-')) FROM players ORDER BY name")
	if diff := cmp.Diff([]string{"ann|PL", "bob|DE", "cat|-", "dan|FR", "eve|NZ"}, players); diff != "" {
		t.Errorf("players after the Down, each with their member's country (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(want, members()); diff != "" {
		t.Errorf("members after the Down, all kept (-want +got):\n%s", diff)
	}
}

func TestEnteringAMemberSavesTheDetailsGiven(t *testing.T) {
	_, a := blitzFixture(t)
	ctx := context.Background()
	ann, err := a.AddMember(ctx, Member{Name: "Ann", DiscordName: "ann#1", Zone: "Unknown"})
	if err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	long := BlitzEntrant{MemberID: ann, DiscordName: strings.Repeat("x", 101)}
	if _, err := a.AddBlitzEntrants(ctx, "us_open", "2026", []BlitzEntrant{long}); !errors.Is(err, ErrBadMember) {
		t.Fatalf("entering Ann with a long Discord name = %v, want ErrBadMember", err)
	}
	given := BlitzEntrant{MemberID: ann, Email: "ann@example.test", Country: "de", Zone: "Europe Central"}
	if _, err := a.AddBlitzEntrants(ctx, "us_open", "2026", []BlitzEntrant{given}); err != nil {
		t.Fatalf("entering Ann with her details: %v", err)
	}
	got, err := a.Members(ctx)
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	want := []string{`Ann cge=- player="" discord="ann#1" email="ann@example.test" "DE" Europe Central banned=no in=1`}
	if diff := cmp.Diff(want, memberLines(got)); diff != "" {
		t.Errorf("Ann once entered, the details given saved and the rest kept (-want +got):\n%s", diff)
	}
}
