//go:build integration

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// filled returns a migrated database holding a little of every kind of input.
func filled(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1":      day("2024-01-02"),
		"Wimbledon 2024 Stage 7 - Final game 1": day("2025-02-24"),
	})
	for _, q := range []string{
		`INSERT INTO match_seats (match_id, seat, player_name, outcome, cge_user)
		 SELECT match_id, 1, player1, 'finished', 42 FROM matches ORDER BY match_id LIMIT 1`,
		`INSERT INTO tournament_official_placements (series, season, division, player_name, place, points, reason, source)
		 VALUES ('nations_league', 10, 'Apple', 'DANeo', 1, 36, 'r', 's, with a comma and "quotes"')`,
		`INSERT INTO tournament_season_notes (tournament, season, stage, note, source)
		 VALUES ('worlds', '2023 Play Off', '', 'A line
		 and another', 's')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("filling: %v", err)
		}
	}
	return pool
}

func readDir(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	got := map[string][]byte{}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		got[e.Name()] = b
	}
	return got
}

func TestRestoreGivesBackExactlyWhatWasExported(t *testing.T) {
	ctx := context.Background()
	first := filepath.Join(t.TempDir(), "first")
	if _, err := Export(ctx, filled(t), first); err != nil {
		t.Fatalf("Export: %v", err)
	}

	fresh := migrated(t)
	if err := Restore(ctx, fresh, first); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	second := filepath.Join(t.TempDir(), "second")
	if _, err := Export(ctx, fresh, second); err != nil {
		t.Fatalf("Export after restore: %v", err)
	}

	a, b := readDir(t, first), readDir(t, second)
	if len(a) != len(ExportTables)+1 {
		t.Errorf("export wrote %d files, want one per table and a manifest (%d)", len(a), len(ExportTables)+1)
	}
	for name, want := range a {
		if !bytes.Equal(b[name], want) {
			t.Errorf("%s differs after a round trip", name)
		}
	}
	if _, err := fresh.Exec(ctx, `INSERT INTO import_batches (source) VALUES ('load')`); err != nil {
		t.Errorf("a new row after restore: %v (identity not moved past the restored ids)", err)
	}
}

func TestRestoreRefusesADatabaseThatHoldsData(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "x")
	if _, err := Export(ctx, filled(t), dir); err != nil {
		t.Fatalf("Export: %v", err)
	}
	busy := filled(t)
	var before int
	if err := busy.QueryRow(ctx, "SELECT count(*) FROM matches").Scan(&before); err != nil {
		t.Fatal(err)
	}

	if err := Restore(ctx, busy, dir); err == nil {
		t.Fatal("Restore into a database with matches: no error, want a refusal")
	}
	var after int
	if err := busy.QueryRow(ctx, "SELECT count(*) FROM matches").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("matches = %d after a refused restore, want %d", after, before)
	}
}

func TestRestoreRefusesAChangedFile(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "x")
	if _, err := Export(ctx, filled(t), dir); err != nil {
		t.Fatalf("Export: %v", err)
	}
	f := filepath.Join(dir, "matches.csv")
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f, bytes.Replace(b, []byte("Intermezzo"), []byte("Intermezzx"), 1), 0o600); err != nil {
		t.Fatal(err)
	}

	fresh := migrated(t)
	if err := Restore(ctx, fresh, dir); err == nil || !strings.Contains(err.Error(), "matches.csv") {
		t.Errorf("Restore of a changed file: err = %v, want a refusal naming matches.csv", err)
	}
	var n int
	if err := fresh.QueryRow(ctx, "SELECT count(*) FROM matches").Scan(&n); err != nil || n != 0 {
		t.Errorf("matches = %d (%v) after a refused restore, want 0", n, err)
	}
}

func TestRestoreRefusesAnotherSchemaVersion(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "x")
	if _, err := Export(ctx, filled(t), dir); err != nil {
		t.Fatalf("Export: %v", err)
	}
	f := filepath.Join(dir, "manifest.json")
	var m Manifest
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	m.SchemaVersion--
	b, _ = json.Marshal(m)
	if err := os.WriteFile(f, b, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Restore(ctx, migrated(t), dir); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Errorf("Restore of an older schema: err = %v, want a refusal naming the schema", err)
	}
}

func TestExportRefusesAFolderThatIsNotEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "old.csv"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Export(context.Background(), filled(t), dir); err == nil {
		t.Error("Export into a folder with files: no error, want a refusal")
	}
}

func TestExportRefusesAnInputTableItDoesNotKnow(t *testing.T) {
	pool := filled(t)
	if _, err := pool.Exec(context.Background(), "CREATE TABLE player_aliases (name text)"); err != nil {
		t.Fatal(err)
	}

	_, err := Export(context.Background(), pool, filepath.Join(t.TempDir(), "x"))
	if err == nil || !strings.Contains(err.Error(), "player_aliases") {
		t.Errorf("Export with an unlisted table: err = %v, want a refusal naming player_aliases", err)
	}
}
