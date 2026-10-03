// Export and restore of the ratings database's inputs: one CSV per table and a
// manifest, so a lost database is rebuilt by a restore and an engine run.

package store

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/matchcsv"
)

// ExportTables are the tables that hold inputs, in an order a restore can
// load them (a table after those it refers to). Every other table holds a
// run's results, which the engine computes again.
var ExportTables = []string{
	"import_batches", "refused_rows", "players",
	"tournaments", "tournament_seasons", "tournament_stages",
	"matches", "match_seats",
	"rankings", "ranking_tournaments", "cge_tournaments",
	"community_scores", "community_standings",
	"tournament_career_honours", "tournament_cup_podiums", "tournament_finish_dates",
	"tournament_group_divisions", "tournament_ladder_wins", "tournament_official_placements",
	"tournament_season_corrections", "tournament_season_notes", "tournament_season_rules",
	"tournament_podium_overrides",
	"admins", "admin_audit",
}

// Manifest describes an export: the schema it was taken at and each table's file.
type Manifest struct {
	SchemaVersion int64           `json:"schema_version"`
	Tables        []ExportedTable `json:"tables"`
}

// ExportedTable is one table's file in an export, with its row count and SHA-256.
type ExportedTable struct {
	Table  string `json:"table"`
	File   string `json:"file"`
	Rows   int64  `json:"rows"`
	SHA256 string `json:"sha256"`
}

const manifestFile = "manifest.json"

// Export writes every input table to dir (created, or empty) as <table>.csv
// with a header, rows in a fixed order, from one snapshot, and the manifest.
// It refuses a database with an input table ExportTables does not name.
func Export(ctx context.Context, pool *pgxpool.Pool, dir string) (Manifest, error) {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return Manifest{}, fmt.Errorf("exporting: %s is not empty", dir)
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Manifest{}, fmt.Errorf("exporting: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := checkInputTables(ctx, tx); err != nil {
		return Manifest{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Manifest{}, fmt.Errorf("exporting: %w", err)
	}
	m := Manifest{}
	if m.SchemaVersion, err = schemaVersion(ctx, tx); err != nil {
		return Manifest{}, err
	}
	for _, table := range ExportTables {
		t, err := exportTable(ctx, tx, dir, table)
		if err != nil {
			return Manifest{}, err
		}
		m.Tables = append(m.Tables, t)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return Manifest{}, fmt.Errorf("exporting: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestFile), append(b, '\n'), 0o600); err != nil {
		return Manifest{}, fmt.Errorf("exporting: %w", err)
	}
	return m, nil
}

func exportTable(ctx context.Context, tx pgx.Tx, dir, table string) (ExportedTable, error) {
	t := ExportedTable{Table: table, File: table + ".csv"}
	f, err := os.OpenFile(filepath.Join(dir, t.File), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return t, fmt.Errorf("exporting %s: %w", table, err)
	}
	defer f.Close()
	h := sha256.New()
	name := pgx.Identifier{table}.Sanitize()
	tag, err := tx.Conn().PgConn().CopyTo(ctx, io.MultiWriter(f, h),
		fmt.Sprintf(`COPY (SELECT * FROM %s t ORDER BY t::text COLLATE "C") TO STDOUT WITH (FORMAT csv, HEADER)`, name))
	if err != nil {
		return t, fmt.Errorf("exporting %s: %w", table, err)
	}
	if err := f.Close(); err != nil {
		return t, fmt.Errorf("exporting %s: %w", table, err)
	}
	t.Rows, t.SHA256 = tag.RowsAffected(), hex.EncodeToString(h.Sum(nil))
	return t, nil
}

// Restore loads an export into a migrated database that holds no inputs and no
// runs, in one transaction. It refuses another schema version, a file whose
// checksum or row count differs from the manifest, or a database with data.
func Restore(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	b, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		return fmt.Errorf("restoring: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return fmt.Errorf("restoring: reading the manifest: %w", err)
	}
	if got := tableNames(m); !slices.Equal(got, ExportTables) {
		return fmt.Errorf("restoring: the manifest names tables %v, want %v", got, ExportTables)
	}
	for _, t := range m.Tables {
		if err := checkFile(dir, t); err != nil {
			return err
		}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("restoring: %w", err)
	}
	defer tx.Rollback(ctx)
	if v, err := schemaVersion(ctx, tx); err != nil {
		return err
	} else if v != m.SchemaVersion {
		return fmt.Errorf("restoring: the export is at schema version %d, the database at %d", m.SchemaVersion, v)
	}
	if err := checkInputTables(ctx, tx); err != nil {
		return err
	}
	if err := checkEmpty(ctx, tx); err != nil {
		return err
	}
	// The migrations seed the community ranking; the export holds it too.
	if _, err := tx.Exec(ctx, "DELETE FROM rankings"); err != nil {
		return fmt.Errorf("restoring: %w", err)
	}
	for _, t := range m.Tables {
		if err := restoreTable(ctx, tx, dir, t); err != nil {
			return err
		}
	}
	if err := resetIdentities(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("restoring: %w", err)
	}
	return nil
}

func tableNames(m Manifest) []string {
	names := make([]string, len(m.Tables))
	for i, t := range m.Tables {
		names[i] = t.Table
	}
	return names
}

// checkFile refuses a file whose name is not the table's or whose SHA-256 is not the manifest's.
func checkFile(dir string, t ExportedTable) error {
	if t.File != t.Table+".csv" {
		return fmt.Errorf("restoring: %s names file %q, want %s.csv", t.Table, t.File, t.Table)
	}
	f, err := os.Open(filepath.Join(dir, t.File))
	if err != nil {
		return fmt.Errorf("restoring: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("restoring %s: %w", t.File, err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != t.SHA256 {
		return fmt.Errorf("restoring: %s has changed since the export (SHA-256 %s, manifest %s)", t.File, got, t.SHA256)
	}
	return nil
}

func restoreTable(ctx context.Context, tx pgx.Tx, dir string, t ExportedTable) error {
	f, err := os.Open(filepath.Join(dir, t.File))
	if err != nil {
		return fmt.Errorf("restoring: %w", err)
	}
	defer f.Close()
	tag, err := tx.Conn().PgConn().CopyFrom(ctx, f,
		fmt.Sprintf("COPY %s FROM STDIN WITH (FORMAT csv, HEADER MATCH)", pgx.Identifier{t.Table}.Sanitize()))
	if err != nil {
		return fmt.Errorf("restoring %s: %w", t.File, err)
	}
	if tag.RowsAffected() != t.Rows {
		return fmt.Errorf("restoring %s: %d rows, manifest %d", t.File, tag.RowsAffected(), t.Rows)
	}
	return nil
}

// checkInputTables refuses a database whose input tables (those without a
// run_id, other than goose's and rating_runs) are not exactly ExportTables.
func checkInputTables(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `
		SELECT t.table_name FROM information_schema.tables t
		WHERE t.table_schema = 'public' AND t.table_type = 'BASE TABLE'
		  AND t.table_name NOT IN ('goose_db_version', 'rating_runs')
		  AND NOT EXISTS (SELECT 1 FROM information_schema.columns c
		                  WHERE c.table_schema = 'public' AND c.table_name = t.table_name AND c.column_name = 'run_id')
		ORDER BY 1`)
	if err != nil {
		return fmt.Errorf("listing input tables: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("listing input tables: %w", err)
	}
	var problems []error
	for _, t := range found {
		if !slices.Contains(ExportTables, t) {
			problems = append(problems, fmt.Errorf("input table %s is not in ExportTables", t))
		}
	}
	for _, t := range ExportTables {
		if !slices.Contains(found, t) {
			problems = append(problems, fmt.Errorf("table %s in ExportTables is not in the database", t))
		}
	}
	return errors.Join(problems...)
}

// checkEmpty refuses a database holding any run or input, other than the rankings the migrations seed.
func checkEmpty(ctx context.Context, tx pgx.Tx) error {
	for _, t := range append([]string{"rating_runs"}, ExportTables...) {
		if t == "rankings" {
			continue
		}
		var any bool
		if err := tx.QueryRow(ctx, fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM %s)", pgx.Identifier{t}.Sanitize())).Scan(&any); err != nil {
			return fmt.Errorf("restoring: checking %s: %w", t, err)
		}
		if any {
			return fmt.Errorf("restoring: %s already holds rows; restore only into a new, migrated database", t)
		}
	}
	return nil
}

// resetIdentities moves each input table's identity past the restored ids.
func resetIdentities(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `
		SELECT table_name, column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND is_identity = 'YES' AND table_name = ANY($1)`, ExportTables)
	if err != nil {
		return fmt.Errorf("restoring: listing identities: %w", err)
	}
	type identity struct{ Table, Column string }
	ids, err := pgx.CollectRows(rows, pgx.RowToStructByPos[identity])
	if err != nil {
		return fmt.Errorf("restoring: listing identities: %w", err)
	}
	for _, id := range ids {
		q := fmt.Sprintf("SELECT setval(pg_get_serial_sequence($1, $2), coalesce(max(%s), 0) + 1, false) FROM %s",
			pgx.Identifier{id.Column}.Sanitize(), pgx.Identifier{id.Table}.Sanitize())
		if _, err := tx.Exec(ctx, q, id.Table, id.Column); err != nil {
			return fmt.Errorf("restoring: moving %s.%s past its ids: %w", id.Table, id.Column, err)
		}
	}
	return nil
}

// schemaVersion returns the latest migration goose has applied.
func schemaVersion(ctx context.Context, tx pgx.Tx) (int64, error) {
	var v int64
	if err := tx.QueryRow(ctx, "SELECT coalesce(max(version_id), 0) FROM goose_db_version WHERE is_applied").Scan(&v); err != nil {
		return 0, fmt.Errorf("reading the schema version: %w", err)
	}
	return v, nil
}

// ExportForUpstream writes every played game as one match CSV in the format
// upstream's loader reads (matchcsv.Header), oldest first, and returns how
// many it wrote. Games never played or unconfirmed are left out: they are not
// rated here, and upstream would rate them.
func ExportForUpstream(ctx context.Context, pool *pgxpool.Pool, w io.Writer) (int, error) {
	rows, err := pool.Query(ctx, `
		SELECT tournament, to_char(date, 'YYYY-MM-DD'), player_count,
		       player1, score1, player2, score2, player3, score3, player4, score4
		FROM matches WHERE state = 'played' ORDER BY date, match_id`)
	if err != nil {
		return 0, fmt.Errorf("exporting for upstream: %w", err)
	}
	defer rows.Close()
	cw := csv.NewWriter(w)
	if err := cw.Write(matchcsv.Header); err != nil {
		return 0, fmt.Errorf("exporting for upstream: %w", err)
	}
	n := 0
	for rows.Next() {
		var tournament, date string
		var count int
		var names [4]*string
		var scores [4]*float64
		if err := rows.Scan(&tournament, &date, &count,
			&names[0], &scores[0], &names[1], &scores[1], &names[2], &scores[2], &names[3], &scores[3]); err != nil {
			return n, fmt.Errorf("exporting for upstream: %w", err)
		}
		record := []string{tournament, date}
		for i := range names {
			name, score := "", ""
			if names[i] != nil && scores[i] != nil {
				name, score = *names[i], strconv.FormatFloat(*scores[i], 'f', -1, 64)
			}
			record = append(record, name, score)
		}
		if err := cw.Write(append(record, strconv.Itoa(count))); err != nil {
			return n, fmt.Errorf("exporting for upstream: %w", err)
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return n, fmt.Errorf("exporting for upstream: %w", err)
	}
	cw.Flush()
	return n, cw.Error()
}
