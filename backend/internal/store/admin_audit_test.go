//go:build integration

package store

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// addAudit writes one audit row at the given minute past a fixed time.
func addAudit(t *testing.T, pool *pgxpool.Pool, m int, discordID, action, target string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "INSERT INTO admin_audit (at, discord_id, action, target) VALUES ($1, $2, $3, $4)",
		minute(m), discordID, action, target); err != nil {
		t.Fatal(err)
	}
}

// minute is the time addAudit writes for minute m.
func minute(m int) *time.Time {
	at := time.Date(2026, 10, 1, 0, m, 0, 0, time.UTC)
	return &at
}

func TestAuditLogPagesNewestFirst(t *testing.T) {
	pool := migrated(t)
	for i := range auditPerPage + 5 {
		addAudit(t, pool, i, "100000000000000001", "badge.add", "")
	}
	a := NewAdminStore(asAdminLogin(t, pool))
	tests := []struct {
		name      string
		page      int
		wantPage  int
		wantCount int
		wantFirst time.Time
	}{
		{name: "the first page", page: 1, wantPage: 1, wantCount: auditPerPage, wantFirst: time.Date(2026, 10, 1, 0, auditPerPage+4, 0, 0, time.UTC)},
		{name: "the last page", page: 2, wantPage: 2, wantCount: 5, wantFirst: time.Date(2026, 10, 1, 0, 4, 0, 0, time.UTC)},
		{name: "a page past the last gives the last", page: 9, wantPage: 2, wantCount: 5, wantFirst: time.Date(2026, 10, 1, 0, 4, 0, 0, time.UTC)},
		{name: "a page before the first gives the first", page: 0, wantPage: 1, wantCount: auditPerPage, wantFirst: time.Date(2026, 10, 1, 0, auditPerPage+4, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := a.AuditLog(context.Background(), AuditFilter{}, tt.page)
			if err != nil {
				t.Fatalf("AuditLog: %v", err)
			}
			if got.Page != tt.wantPage || got.Pages != 2 || got.Total != auditPerPage+5 || len(got.Entries) != tt.wantCount {
				t.Fatalf("page %d: got page %d of %d, total %d, %d entries; want page %d of 2, total %d, %d entries",
					tt.page, got.Page, got.Pages, got.Total, len(got.Entries), tt.wantPage, auditPerPage+5, tt.wantCount)
			}
			if !got.Entries[0].At.Equal(tt.wantFirst) {
				t.Errorf("page %d: first entry at %v, want %v", tt.page, got.Entries[0].At, tt.wantFirst)
			}
		})
	}
}

func TestAuditLogFiltersByActionAndText(t *testing.T) {
	pool := migrated(t)
	if err := AddAdmin(context.Background(), pool, Admin{DiscordID: "100000000000000001", Name: "Owner", Role: RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	addAudit(t, pool, 1, "100000000000000001", "badge.add", "Gold")
	addAudit(t, pool, 2, "100000000000000001", "member.set", "7")
	addAudit(t, pool, 3, "", "blitz.match.report", "12")
	addAudit(t, pool, 4, "100000000000000001", "badge.add", "Silver")
	if _, err := pool.Exec(context.Background(), `UPDATE admin_audit SET detail = '{"set": ["discord_name"]}' WHERE target = '7'`); err != nil {
		t.Fatal(err)
	}
	a := NewAdminStore(asAdminLogin(t, pool))
	tests := []struct {
		name        string
		filter      AuditFilter
		wantTargets []string
	}{
		{name: "everything", wantTargets: []string{"Silver", "12", "7", "Gold"}},
		{name: "one action", filter: AuditFilter{Action: "badge.add"}, wantTargets: []string{"Silver", "Gold"}},
		{name: "an action never taken", filter: AuditFilter{Action: "run.start"}},
		{name: "text in the target, any case", filter: AuditFilter{Text: "gOLD"}, wantTargets: []string{"Gold"}},
		{name: "text in the detail", filter: AuditFilter{Text: "discord_name"}, wantTargets: []string{"7"}},
		{name: "text in the admin's name", filter: AuditFilter{Text: "owner"}, wantTargets: []string{"Silver", "7", "Gold"}},
		{name: "text in the action", filter: AuditFilter{Text: "match.rep"}, wantTargets: []string{"12"}},
		{name: "text with a LIKE wildcard is matched as written", filter: AuditFilter{Text: "%"}},
		{name: "action and text together", filter: AuditFilter{Action: "badge.add", Text: "silver"}, wantTargets: []string{"Silver"}},
		{name: "from a time, inclusive", filter: AuditFilter{From: minute(2)}, wantTargets: []string{"Silver", "12", "7"}},
		{name: "to a time, exclusive", filter: AuditFilter{To: minute(3)}, wantTargets: []string{"7", "Gold"}},
		{name: "between two times", filter: AuditFilter{From: minute(2), To: minute(3)}, wantTargets: []string{"7"}},
		{name: "a range with nothing in it", filter: AuditFilter{From: minute(5)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := a.AuditLog(context.Background(), tt.filter, 1)
			if err != nil {
				t.Fatalf("AuditLog(%+v): %v", tt.filter, err)
			}
			var targets []string
			for _, e := range got.Entries {
				targets = append(targets, e.Target)
			}
			if !slices.Equal(targets, tt.wantTargets) || got.Total != len(tt.wantTargets) {
				t.Errorf("AuditLog(%+v) targets = %q, total %d; want %q", tt.filter, targets, got.Total, tt.wantTargets)
			}
			if want := []string{"badge.add", "blitz.match.report", "member.set"}; !slices.Equal(got.Actions, want) {
				t.Errorf("AuditLog(%+v) actions = %q, want every action taken, %q", tt.filter, got.Actions, want)
			}
		})
	}
}

func TestAuditLogNamesTheAdminWhoMadeEachEntry(t *testing.T) {
	pool := migrated(t)
	if err := AddAdmin(context.Background(), pool, Admin{DiscordID: "100000000000000001", Name: "Owner", Role: RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	addAudit(t, pool, 1, "100000000000000001", "badge.add", "by an admin")
	addAudit(t, pool, 2, "", "blitz.match.report", "by a player")
	addAudit(t, pool, 3, "100000000000000009", "badge.add", "by someone no longer in admin")
	got, err := NewAdminStore(asAdminLogin(t, pool)).AuditLog(context.Background(), AuditFilter{}, 1)
	if err != nil {
		t.Fatalf("AuditLog: %v", err)
	}
	want := map[string]struct {
		admin    string
		byPlayer bool
	}{
		"by an admin":                   {admin: "Owner"},
		"by a player":                   {byPlayer: true},
		"by someone no longer in admin": {},
	}
	if len(got.Entries) != len(want) {
		t.Fatalf("AuditLog: %d entries, want %d", len(got.Entries), len(want))
	}
	for _, e := range got.Entries {
		w := want[e.Target]
		if e.Admin != w.admin || e.ByPlayer != w.byPlayer {
			t.Errorf("%s: admin %q, by a player %v; want %q, %v", e.Target, e.Admin, e.ByPlayer, w.admin, w.byPlayer)
		}
	}
}
