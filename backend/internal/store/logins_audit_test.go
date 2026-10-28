//go:build integration

package store

import (
	"context"
	"slices"
	"strconv"
	"testing"

	"github.com/pressly/goose/v3"
)

// auditRows returns the audit's action, target and detail, oldest first.
func auditRows(t *testing.T, a *AdminStore) []string {
	t.Helper()
	log, err := a.AuditLog(context.Background(), AuditFilter{}, 1)
	if err != nil {
		t.Fatalf("AuditLog: %v", err)
	}
	var got []string
	for _, e := range slices.Backward(log.Entries) {
		got = append(got, e.Action+" "+e.Target+" "+string(e.Detail))
	}
	return got
}

func TestAddingAndRemovingAWayInIsAuditedByIds(t *testing.T) {
	_, a, p := playersFixture(t)
	ctx := context.Background()
	l := emailSignedIn(t, p, "player@example.test", "email")
	if err := p.AddLogin(ctx, tokenHash("email"), discordID("100000000000000001", "player_one", ""), signInAt); err != nil {
		t.Fatalf("AddLogin: %v", err)
	}
	if err := p.NewEmailLink(ctx, "new@example.test", &l.ID, "/account/", tokenHash("add-new"), signInAt); err != nil {
		t.Fatalf("NewEmailLink: %v", err)
	}
	if _, err := p.UseEmailLink(ctx, tokenHash("add-new"), tokenHash("added-new"), signInAt); err != nil {
		t.Fatalf("UseEmailLink: %v", err)
	}
	if err := p.RemoveOwnLogin(ctx, tokenHash("email"), "discord", signInAt); err != nil {
		t.Fatalf("RemoveOwnLogin: %v", err)
	}
	account := strconv.FormatInt(l.ID, 10)
	want := []string{
		"login.add " + account + ` {"provider": "discord"}`,
		"login.add " + account + ` {"provider": "email"}`,
		"login.remove " + account + ` {"provider": "discord"}`,
	}
	if got := auditRows(t, a); !slices.Equal(got, want) {
		t.Errorf("audit = %q, want %q", got, want)
	}
}

func TestASignInChangingAMembersDiscordNameIsAuditedByIds(t *testing.T) {
	_, a, p := playersFixture(t)
	ctx := context.Background()
	ann, err := a.AddMember(ctx, Member{Name: "ann", DiscordName: "player_one", Zone: "UTC"})
	if err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	l := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "token")
	signedIn(t, p, discordID("100000000000000001", "player_one", ""), "same")
	signedIn(t, p, discordID("100000000000000001", "player_renamed", ""), "renamed")
	want := []string{"member.discord " + strconv.FormatInt(ann, 10) + ` {"account": ` + strconv.FormatInt(l.ID, 10) + `}`}
	if got := auditRows(t, a); !slices.Equal(got, want) {
		t.Errorf("audit = %q, want one entry, for the rename: %q", got, want)
	}
}

func TestMigration58sDownKeepsAnOrganisersDiscordNameAgain(t *testing.T) {
	pool, a, p := playersFixture(t)
	ctx := context.Background()
	if _, err := a.AddMember(ctx, Member{Name: "ann", DiscordName: "player_one", Zone: "UTC"}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if err := withRatingsMigrations(pool, func(g *goose.Provider) error { _, err := g.DownTo(ctx, 57); return err }); err != nil {
		t.Fatalf("migrating down to 57: %v", err)
	}
	signedIn(t, p, discordID("100000000000000001", "player_one", ""), "token")
	signedIn(t, p, discordID("100000000000000001", "player_renamed", ""), "renamed")
	if got := rowsOf(t, pool, "SELECT discord_name FROM members"); !slices.Equal(got, []string{"player_one"}) {
		t.Errorf("after the Down, ann's Discord name = %q, want the one she had, player_one", got)
	}
	var left bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT FROM pg_proc WHERE proname = 'audit_own_login_added')").Scan(&left); err != nil || left {
		t.Errorf("audit_own_login_added after the Down: %t, %v; want gone", left, err)
	}
}
