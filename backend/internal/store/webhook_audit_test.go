//go:build integration

package store

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestMigration91TakesWebhookURLsOutOfTheAuditLog(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 90); return err }); err != nil {
		t.Fatalf("migrating down to 90: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO admin_audit (discord_id, action, target, detail) VALUES
		('100000000000000001', 'notification.endpoint.add', 'Admins', jsonb_build_object('name', 'Admins', 'url', $1::text)),
		('100000000000000001', 'notification.endpoint.set', '3', jsonb_build_object('name', 'Admins', 'url', ''))`, webhook(2)); err != nil {
		t.Fatalf("auditing: %v", err)
	}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.Up(ctx); return err }); err != nil {
		t.Fatalf("migrating up: %v", err)
	}
	got := rowsOf(t, pool, "SELECT action || ' ' || (detail->>'url') FROM admin_audit WHERE action LIKE 'notification.endpoint.%' ORDER BY audit_id")
	want := []string{"notification.endpoint.add webhook 100000000000000002", "notification.endpoint.set "}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("audit urls = %q, want %q", got, want)
	}
}
