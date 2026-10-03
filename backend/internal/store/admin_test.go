//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// asAdminLogin returns a pool on the same database acting as ratings_admin, as the API's admin login does.
func asAdminLogin(t *testing.T, pool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	cfg := pool.Config().Copy()
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE ratings_admin")
		return err
	}
	admin, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("opening a ratings_admin pool: %v", err)
	}
	t.Cleanup(admin.Close)
	return admin
}

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

func TestThePublicReaderCannotReadAdminsOrTheAudit(t *testing.T) {
	reader := asReader(t, migrated(t))
	for _, table := range []string{"admins", "admin_audit"} {
		if _, err := reader.Exec(context.Background(), "SELECT * FROM "+table); err == nil {
			t.Errorf("ratings_read read %s, want permission denied", table)
		}
	}
}

func TestTheAdminLoginCannotChangeGamesOrRuns(t *testing.T) {
	admin := asAdminLogin(t, migrated(t))
	for _, q := range []string{"DELETE FROM matches", "DELETE FROM rating_runs", "UPDATE players SET name = name", "CREATE TABLE x (y int)"} {
		if _, err := admin.Exec(context.Background(), q); err == nil {
			t.Errorf("ratings_admin ran %q, want permission denied", q)
		}
	}
}
