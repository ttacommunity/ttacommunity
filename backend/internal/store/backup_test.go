//go:build integration

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// withPeople adds a row to every input table an export leaves out, sign-in secrets included, and returns pool.
func withPeople(t *testing.T, pool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO admins (discord_id, name, role) VALUES ('100000000000000001', 'An Admin', 'admin')`,
		`INSERT INTO admin_audit (discord_id, action, target) VALUES ('100000000000000001', 'run.start', 'community')`,
		`INSERT INTO refused_rows (batch_id, source_file, source_line, reason, fields)
		 SELECT batch_id, 'm.csv', 9, 'standings_row', '{"a refused line"}' FROM import_batches LIMIT 1`,
		`INSERT INTO members (tta_name, discord_name, email) VALUES ('Ann', 'ann#1111', 'ann@example.test'), ('Bea', 'bea#2222', '')`,
		`INSERT INTO accounts (created_at) VALUES (now())`,
		`INSERT INTO logins (provider, subject, username, created_at, signed_in_at, account_id)
		 SELECT 'discord', '100000000000000002', 'ann', now(), now(), account_id FROM accounts`,
		`INSERT INTO link_requests (tta_name, requested_at, account_id) SELECT 'Ann', now(), account_id FROM accounts`,
		`INSERT INTO player_sessions (token_hash, login_id, created_at, expires_at) SELECT sha256('session'), login_id, now(), now() FROM logins`,
		`INSERT INTO email_links (token_hash, address, return_path, created_at, expires_at)
		 VALUES (sha256('link'), 'ann@example.test', '/', now(), now())`,
		`INSERT INTO blitz_editions (tournament_id, swiss_rounds, knockout_size) SELECT tournament_id, 8, 8 FROM tournaments LIMIT 1`,
		`INSERT INTO blitz_entries (tournament_id, member_id) SELECT tournament_id, member_id FROM blitz_editions, members`,
		`INSERT INTO blitz_organisers (tournament_id, member_id) SELECT tournament_id, min(member_id) FROM blitz_entries GROUP BY 1`,
		`INSERT INTO blitz_thread_texts (tournament_id, message, no_discord) SELECT tournament_id, 'Round {round}', 'none' FROM blitz_editions`,
		`INSERT INTO blitz_rounds (tournament_id, number, seed) SELECT tournament_id, 1, 7 FROM blitz_editions`,
		`INSERT INTO blitz_matches (round_id, entry_a, entry_b)
		 SELECT round_id, min(entry_id), max(entry_id) FROM blitz_rounds, blitz_entries GROUP BY round_id`,
		`INSERT INTO blitz_skips (entry_id, number) SELECT min(entry_id), 2 FROM blitz_entries`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("filling %q: %v", q, err)
		}
	}
	return pool
}

func TestRestoreGivesBackExactlyWhatWasBackedUp(t *testing.T) {
	ctx := context.Background()
	first := filepath.Join(t.TempDir(), "first")
	if _, err := Backup(ctx, withPeople(t, filled(t)), first); err != nil {
		t.Fatalf("Backup: %v", err)
	}

	fresh := migrated(t)
	if err := Restore(ctx, fresh, first); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	second := filepath.Join(t.TempDir(), "second")
	if _, err := Backup(ctx, fresh, second); err != nil {
		t.Fatalf("Backup after restore: %v", err)
	}

	a, b := readDir(t, first), readDir(t, second)
	if len(a) != len(BackupTables)+1 {
		t.Errorf("backup wrote %d files, want one per table and a manifest (%d)", len(a), len(BackupTables)+1)
	}
	for name, want := range a {
		if !bytes.Equal(b[name], want) {
			t.Errorf("%s differs after a round trip", name)
		}
	}
	for _, q := range []string{
		`INSERT INTO members (tta_name) VALUES ('Cid')`,
		`INSERT INTO accounts (created_at) VALUES (now())`,
		`INSERT INTO blitz_rounds (tournament_id, number, seed) SELECT tournament_id, 2, 7 FROM blitz_editions`,
	} {
		if _, err := fresh.Exec(ctx, q); err != nil {
			t.Errorf("%s after restore: %v (identity not moved past the restored ids)", q, err)
		}
	}
}

func TestBackupHoldsEveryInputTableButSignInSecrets(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "backup")
	m, err := Backup(context.Background(), withPeople(t, filled(t)), dir)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	inputs := append(slices.Clone(ExportTables), NotExported...)
	for _, table := range inputs {
		kept := slices.Contains(tableNames(m), table)
		if secret := table == "player_sessions" || table == "email_links"; kept == secret {
			t.Errorf("backup holds %s = %v, want %v", table, kept, !secret)
		}
	}
	for _, f := range []string{"player_sessions.csv", "email_links.csv"} {
		if _, err := os.Stat(filepath.Join(dir, f)); !os.IsNotExist(err) {
			t.Errorf("backup wrote %s (stat: %v)", f, err)
		}
	}
}

func TestRestoreOfABackupRefusesADatabaseWithMembers(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "backup")
	if _, err := Backup(ctx, withPeople(t, filled(t)), dir); err != nil {
		t.Fatalf("Backup: %v", err)
	}
	busy := migrated(t)
	if _, err := busy.Exec(ctx, `INSERT INTO members (tta_name) VALUES ('Dee')`); err != nil {
		t.Fatal(err)
	}

	if err := Restore(ctx, busy, dir); err == nil || !strings.Contains(err.Error(), "members already holds rows") {
		t.Errorf("Restore of a backup into a database with a member = %v, want a refusal naming members", err)
	}
	var n int
	if err := busy.QueryRow(ctx, "SELECT count(*) FROM members").Scan(&n); err != nil || n != 1 {
		t.Errorf("members = %d (%v) after a refused restore, want 1", n, err)
	}
}

func TestRestoreRefusesAManifestOfOtherTables(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "backup")
	if _, err := Backup(ctx, withPeople(t, filled(t)), dir); err != nil {
		t.Fatalf("Backup: %v", err)
	}
	f := filepath.Join(dir, "manifest.json")
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	m.Tables = slices.DeleteFunc(m.Tables, func(t ExportedTable) bool { return t.Table == "members" })
	b, _ = json.Marshal(m)
	if err := os.WriteFile(f, b, 0o600); err != nil {
		t.Fatal(err)
	}

	fresh := migrated(t)
	if err := Restore(ctx, fresh, dir); err == nil {
		t.Error("Restore of a backup without members: no error, want a refusal")
	}
	var n int
	if err := fresh.QueryRow(ctx, "SELECT count(*) FROM games").Scan(&n); err != nil || n != 0 {
		t.Errorf("games = %d (%v) after a refused restore, want 0", n, err)
	}
}
