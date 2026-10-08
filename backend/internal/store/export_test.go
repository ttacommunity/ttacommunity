//go:build integration

package store

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/matchcsv"
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
		`INSERT INTO player_accounts (cge_user, player_id, name) SELECT 42, player_id, player_name FROM seats ORDER BY game_id, seat LIMIT 1`,
		`UPDATE seats SET cge_user = 42, outcome = 'finished' WHERE (game_id, seat) = (SELECT game_id, seat FROM seats ORDER BY game_id, seat LIMIT 1)`,
		`INSERT INTO tournament_official_placements (series, season, division, player_name, place, points, reason, source)
		 VALUES ('nations_league', 10, 'Apple', 'Dahlia', 1, 36, 'r', 's, with a comma and "quotes"')`,
		`INSERT INTO cge_standings (cge_tournament, cge_season, group_name, row_number, place, player_name, cge_user,
		                           banned, games, points, places_2p, places_3p, places_4p, source)
		 VALUES (4760, 2, '2024 group 1', 4, NULL, 'swift', 65877, true, 3, -1.5, '{1,0}', NULL, '{0,0,0,2}', 'CGE page')`,
		`INSERT INTO community_points (tournament, from_edition, group_name, place, points, source)
		 VALUES ('worlds', 2025, 'Stage 3', 0, 130, 'sheet')`,
		`INSERT INTO community_quarters (tournament, season, year, quarter, source) VALUES ('wimbledon', '2025', 2026, 1, 'tab')`,
		`INSERT INTO community_ladders (tournament, from_edition, ratio, half_steps_from, shares, rounds, source)
		 VALUES ('sodium_ladder', 2025, 0.8, 4, '{1,0.5,0.166,0}', 8, 'tab')`,
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
	if err := busy.QueryRow(ctx, "SELECT count(*) FROM games").Scan(&before); err != nil {
		t.Fatal(err)
	}

	if err := Restore(ctx, busy, dir); err == nil {
		t.Fatal("Restore into a database with games: no error, want a refusal")
	}
	var after int
	if err := busy.QueryRow(ctx, "SELECT count(*) FROM games").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("games = %d after a refused restore, want %d", after, before)
	}
}

func TestRestoreRefusesAChangedFile(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "x")
	if _, err := Export(ctx, filled(t), dir); err != nil {
		t.Fatalf("Export: %v", err)
	}
	f := filepath.Join(dir, "games.csv")
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f, bytes.Replace(b, []byte("Intermezzo"), []byte("Intermezzx"), 1), 0o600); err != nil {
		t.Fatal(err)
	}

	fresh := migrated(t)
	if err := Restore(ctx, fresh, dir); err == nil || !strings.Contains(err.Error(), "games.csv") {
		t.Errorf("Restore of a changed file: err = %v, want a refusal naming games.csv", err)
	}
	var n int
	if err := fresh.QueryRow(ctx, "SELECT count(*) FROM games").Scan(&n); err != nil || n != 0 {
		t.Errorf("games = %d (%v) after a refused restore, want 0", n, err)
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

	dir := filepath.Join(t.TempDir(), "x")
	_, err := Export(context.Background(), pool, dir)
	if err == nil || !strings.Contains(err.Error(), "player_aliases") {
		t.Errorf("Export with an unlisted table: err = %v, want a refusal naming player_aliases", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("a refused export left %s behind (stat: %v)", dir, err)
	}
}

func TestExportForUpstreamWritesEveryPlayedGameInItsMatchFormat(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	ms := []matchcsv.Match{
		named(2, "Intermezzo S24 - Bronze 1 game 1"),
		named(3, "Intermezzo S24 - Bronze 1 game 2"),
		named(4, "Intermezzo S24 - Bronze 1 game 3"),
		named(5, "RL_s09 - Duke game 1"),
	}
	ms[0].Players = append(ms[0].Players, matchcsv.Player{Name: "cat", Score: 70.5})
	if _, err := LoadMatches(ctx, pool, ms, nil); err != nil {
		t.Fatalf("LoadMatches: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE games SET state = CASE name
		WHEN 'Intermezzo S24 - Bronze 1 game 2' THEN 'never_played'
		WHEN 'Intermezzo S24 - Bronze 1 game 3' THEN 'unconfirmed' ELSE 'played' END`); err != nil {
		t.Fatalf("setting states: %v", err)
	}

	var buf bytes.Buffer
	n, err := ExportForUpstream(ctx, pool, &buf)
	if err != nil {
		t.Fatalf("ExportForUpstream: %v", err)
	}

	rows, err := matchcsv.Read("all_matches.csv", &buf)
	if err != nil {
		t.Fatalf("reading the export back: %v", err)
	}
	var got []string
	for _, r := range rows {
		m, reason := matchcsv.Check(r)
		if reason != "" {
			t.Fatalf("row %v refused: %s", r.Fields, reason)
		}
		got = append(got, fmt.Sprintf("%s|%s|%v", m.Tournament, m.Date.Format("2006-01-02"), m.Players))
	}
	want := []string{
		fmt.Sprintf("%s|%s|%v", ms[0].Tournament, ms[0].Date.Format("2006-01-02"), ms[0].Players),
		fmt.Sprintf("%s|%s|%v", ms[3].Tournament, ms[3].Date.Format("2006-01-02"), ms[3].Players),
	}
	if n != 2 || !slices.Equal(got, want) {
		t.Errorf("export = %d rows %q, want the two played games %q", n, got, want)
	}
}

func TestExportZipHoldsTheSameExportAndTheAdminLoginCanTakeIt(t *testing.T) {
	ctx := context.Background()
	pool := filled(t)
	dir := filepath.Join(t.TempDir(), "dir")
	if _, err := Export(ctx, pool, dir); err != nil {
		t.Fatalf("Export: %v", err)
	}
	var buf bytes.Buffer
	if _, err := ExportZip(ctx, asAdminLogin(t, pool), &buf); err != nil {
		t.Fatalf("ExportZip as the admin login: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("reading the zip: %v", err)
	}
	unzipped := filepath.Join(t.TempDir(), "unzipped")
	if err := os.MkdirAll(unzipped, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(unzipped, f.Name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	a, b := readDir(t, dir), readDir(t, unzipped)
	if len(b) != len(a) {
		t.Fatalf("the zip holds %d files, the export %d", len(b), len(a))
	}
	for name, want := range a {
		if !bytes.Equal(b[name], want) {
			t.Errorf("%s differs between the zip and the export", name)
		}
	}
	if err := Restore(ctx, migrated(t), unzipped); err != nil {
		t.Errorf("restoring the unzipped export: %v", err)
	}
}

// personalColumns are the columns that hold personal data, which the export never holds.
var personalColumns = []string{"discord_id", "email", "ip", "ip_address", "session_id"}

func TestExportHoldsNoPersonalDataOrLoadHistory(t *testing.T) {
	ctx := context.Background()
	pool := filled(t)
	for _, q := range []string{
		`INSERT INTO admins (discord_id, name, role) VALUES ('111111111111111111', 'An Admin', 'admin')`,
		`INSERT INTO admin_audit (discord_id, action, target) VALUES ('111111111111111111', 'run.start', 'community')`,
		`INSERT INTO refused_rows (batch_id, source_file, source_line, reason, fields)
		 SELECT batch_id, 'm.csv', 9, 'standings_row', '{"a refused line"}' FROM import_batches LIMIT 1`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("filling: %v", err)
		}
	}
	dir := filepath.Join(t.TempDir(), "export")
	if _, err := Export(ctx, pool, dir); err != nil {
		t.Fatalf("Export: %v", err)
	}
	for name, body := range readDir(t, dir) {
		for _, left := range []string{"admins.csv", "admin_audit.csv", "refused_rows.csv"} {
			if name == left {
				t.Errorf("the export holds %s", name)
			}
		}
		header := strings.SplitN(string(body), "\n", 2)[0]
		for _, col := range strings.Split(header, ",") {
			if slices.Contains(personalColumns, strings.Trim(col, `"`)) {
				t.Errorf("%s has the personal column %s", name, col)
			}
		}
		if strings.Contains(string(body), "111111111111111111") {
			t.Errorf("%s holds an admin's Discord id", name)
		}
	}
}
