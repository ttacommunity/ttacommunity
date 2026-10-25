//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestAdminFindsAnAddedAdminAndNoOneElse(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	if err := AddAdmin(ctx, pool, Admin{DiscordID: "100000000000000001", Name: "Owner", Role: RoleAdmin}); err != nil {
		t.Fatalf("AddAdmin: %v", err)
	}
	a := NewAdminStore(asAdminLogin(t, pool))

	got, err := a.Admin(ctx, "100000000000000001")
	if err != nil || got != (Admin{DiscordID: "100000000000000001", Name: "Owner", Role: RoleAdmin}) {
		t.Errorf("Admin(owner) = %+v, %v; want the owner as admin", got, err)
	}
	if _, err := a.Admin(ctx, "123456789012345678"); !errors.Is(err, ErrNotAdmin) {
		t.Errorf("Admin(stranger) error = %v, want ErrNotAdmin", err)
	}
}

func TestAddAdminRefusesABadRowOrRole(t *testing.T) {
	pool := migrated(t)
	for _, a := range []Admin{
		{DiscordID: "not-an-id", Name: "X", Role: RoleAdmin},
		{DiscordID: "100000000000000001", Name: "", Role: RoleAdmin},
		{DiscordID: "100000000000000001", Name: "X", Role: "superuser"},
	} {
		if err := AddAdmin(context.Background(), pool, a); err == nil {
			t.Errorf("AddAdmin(%+v): nil error, want a refusal", a)
		}
	}
}

func TestAuditRecordsWhoDidWhat(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	a := NewAdminStore(asAdminLogin(t, pool))

	if err := a.Audit(ctx, "100000000000000001", "tournament.visible", "worlds", map[string]any{"visible": true}); err != nil {
		t.Fatalf("Audit: %v", err)
	}
	var who, action, target, detail string
	if err := pool.QueryRow(ctx, "SELECT discord_id, action, target, detail::text FROM admin_audit").Scan(&who, &action, &target, &detail); err != nil {
		t.Fatal(err)
	}
	if who != "100000000000000001" || action != "tournament.visible" || target != "worlds" || detail != `{"visible": true}` {
		t.Errorf("audit row = %s %s %s %s", who, action, target, detail)
	}
}

func TestAChangeStandsOnlyWithItsAudit(t *testing.T) {
	pool := migrated(t)
	a := NewAdminStore(asAdminLogin(t, pool))
	tests := []struct {
		name      string
		label     string
		detail    any
		cancel    bool // the request is given up between the change and its audit
		wantStand bool
	}{
		{name: "the audit is written", label: "Kept", detail: map[string]any{}, wantStand: true},
		{name: "the database refuses the audit row", label: "Refused", detail: map[string]string{"note": "a\x00b"}},
		{name: "the request is given up before the audit", label: "Given up", detail: map[string]any{}, cancel: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := a.InTx(ctx, func(tx *AdminStore) error {
				if _, err := tx.AddBadge(ctx, Badge{Label: tt.label}); err != nil {
					return err
				}
				if tt.cancel {
					cancel()
				}
				return tx.Audit(ctx, "100000000000000001", "badge.add", tt.label, tt.detail)
			})
			if (err == nil) != tt.wantStand {
				t.Errorf("InTx err = %v, want the change to stand %v", err, tt.wantStand)
			}
			var badges, audits int
			if err := pool.QueryRow(context.Background(), `
				SELECT (SELECT count(*) FROM badges WHERE label = $1), (SELECT count(*) FROM admin_audit WHERE target = $1)`,
				tt.label).Scan(&badges, &audits); err != nil {
				t.Fatal(err)
			}
			if want := map[bool]int{true: 1, false: 0}[tt.wantStand]; badges != want || audits != want {
				t.Errorf("badges = %d, audit rows = %d; want %d of each", badges, audits, want)
			}
		})
	}
}

func TestThePublicReaderCannotReadAdminsOrTheAudit(t *testing.T) {
	reader := asReader(t, migrated(t))
	for _, table := range []string{"admins", "admin_audit"} {
		if _, err := reader.Exec(context.Background(), "SELECT * FROM "+table); err == nil {
			t.Errorf("ratings_read read %s, want permission denied", table)
		}
	}
}

func TestTheAdminLoginCannotDeleteGamesPlayersOrRuns(t *testing.T) {
	admin := asAdminLogin(t, migrated(t))
	for _, q := range []string{"DELETE FROM games", "TRUNCATE games", "DELETE FROM players", "DELETE FROM rating_runs", "CREATE TABLE x (y int)"} {
		if _, err := admin.Exec(context.Background(), q); err == nil {
			t.Errorf("ratings_admin ran %q, want permission denied", q)
		}
	}
}

func TestCheckLimitsAcceptsTheAdminLoginAndRefusesTheOwner(t *testing.T) {
	pool := migrated(t)
	if err := NewAdminStore(asAdminLogin(t, pool)).CheckLimits(context.Background()); err != nil {
		t.Errorf("CheckLimits as ratings_admin: %v, want nil", err)
	}
	if err := NewAdminStore(pool).CheckLimits(context.Background()); !errors.Is(err, ErrAdminTooStrong) {
		t.Errorf("CheckLimits as the owner = %v, want ErrAdminTooStrong", err)
	}
}

func TestCheckLimitsRefusesALoginThatCanDeleteOrCreate(t *testing.T) {
	tests := []struct {
		name, grant string
	}{
		{"delete games", "GRANT DELETE ON games TO ratings_admin"},
		{"truncate games", "GRANT TRUNCATE ON games TO ratings_admin"},
		{"delete players", "GRANT DELETE ON players TO ratings_admin"},
		{"delete runs", "GRANT DELETE ON rating_runs TO ratings_admin"},
		{"create tables", "GRANT CREATE ON SCHEMA public TO ratings_admin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			if _, err := pool.Exec(context.Background(), tt.grant); err != nil {
				t.Fatalf("%s: %v", tt.grant, err)
			}
			if err := NewAdminStore(asAdminLogin(t, pool)).CheckLimits(context.Background()); !errors.Is(err, ErrAdminTooStrong) {
				t.Errorf("CheckLimits after %q = %v, want ErrAdminTooStrong", tt.grant, err)
			}
		})
	}
}

func TestAdminsListsSetsAndRemovesAsTheAdminLogin(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	a := NewAdminStore(asAdminLogin(t, pool))

	for _, x := range []Admin{
		{DiscordID: "222222222222222222", Name: "Org", Role: RoleOrganiser},
		{DiscordID: "111111111111111111", Name: "Owner", Role: RoleAdmin},
	} {
		if err := a.SetAdmin(ctx, x); err != nil {
			t.Fatalf("SetAdmin(%+v): %v", x, err)
		}
	}
	if err := a.SetAdmin(ctx, Admin{DiscordID: "222222222222222222", Name: "Organiser", Role: RoleAdmin}); err != nil {
		t.Fatalf("SetAdmin(promote): %v", err)
	}
	if err := a.SetAdmin(ctx, Admin{DiscordID: "x", Name: "Bad", Role: RoleAdmin}); err == nil {
		t.Error("SetAdmin with a bad Discord id: nil error, want a refusal")
	}

	got, err := a.Admins(ctx)
	if err != nil {
		t.Fatalf("Admins: %v", err)
	}
	want := []Admin{{DiscordID: "222222222222222222", Name: "Organiser", Role: RoleAdmin}, {DiscordID: "111111111111111111", Name: "Owner", Role: RoleAdmin}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Admins, by name (-want +got):\n%s", diff)
	}

	if err := a.RemoveAdmin(ctx, "222222222222222222"); err != nil {
		t.Fatalf("RemoveAdmin: %v", err)
	}
	if err := a.RemoveAdmin(ctx, "222222222222222222"); !errors.Is(err, ErrNotAdmin) {
		t.Errorf("RemoveAdmin twice = %v, want ErrNotAdmin", err)
	}
	if got, _ := a.Admins(ctx); len(got) != 1 {
		t.Errorf("Admins after a removal = %+v, want the owner only", got)
	}
}

func TestAdminAlwaysKeepsOneAdmin(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	a := NewAdminStore(asAdminLogin(t, pool))
	owner := Admin{DiscordID: "111111111111111111", Name: "Owner", Role: RoleAdmin}
	if err := a.SetAdmin(ctx, owner); err != nil {
		t.Fatal(err)
	}

	if err := a.SetAdmin(ctx, Admin{DiscordID: owner.DiscordID, Name: "Owner", Role: RoleOrganiser}); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("demoting the only admin = %v, want ErrLastAdmin", err)
	}
	if err := a.RemoveAdmin(ctx, owner.DiscordID); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("removing the only admin = %v, want ErrLastAdmin", err)
	}
	if err := a.SetAdmin(ctx, Admin{DiscordID: owner.DiscordID, Name: "Owner renamed", Role: RoleAdmin}); err != nil {
		t.Errorf("renaming the only admin: %v, want nil", err)
	}

	if err := a.SetAdmin(ctx, Admin{DiscordID: "222222222222222222", Name: "Second", Role: RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	if err := a.RemoveAdmin(ctx, owner.DiscordID); err != nil {
		t.Errorf("removing one of two admins: %v, want nil", err)
	}
	if got, _ := a.Admins(ctx); len(got) != 1 || got[0].Name != "Second" {
		t.Errorf("admins = %+v, want Second only", got)
	}
}
