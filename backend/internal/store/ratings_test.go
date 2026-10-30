//go:build integration

package store

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	"ttacommunity/backend/internal/matchcsv"
)

// migrateRatingsTo applies the ratings migrations up to and including version.
func migrateRatingsTo(ctx context.Context, pool *pgxpool.Pool, version int64) error {
	return withRatingsMigrations(pool, func(p *goose.Provider) error {
		if _, err := p.UpTo(ctx, version); err != nil {
			return fmt.Errorf("migrating the ratings database up to %d: %w", version, err)
		}
		return nil
	})
}

// newDB creates an empty database for one test and drops it afterwards.
func newDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return newDBFrom(t, "")
}

// newDBFrom creates a database for one test, a copy of template when it is named, and drops it afterwards.
func newDBFrom(t *testing.T, template string) *pgxpool.Pool {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Fatal("TEST_DATABASE_URL is not set: integration tests need a database")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connecting to the test server: %v", err)
	}
	t.Cleanup(func() { admin.Close(context.Background()) })

	name := "ratings_test_" + rand.Text()[:12]
	create := "CREATE DATABASE " + pgx.Identifier{name}.Sanitize()
	if template != "" {
		create += " TEMPLATE " + pgx.Identifier{template}.Sanitize()
	}
	if _, err := admin.Exec(ctx, create); err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parsing TEST_DATABASE_URL: %v", err)
	}
	u.Path = "/" + name
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatalf("opening %s: %v", name, err)
	}
	t.Cleanup(func() {
		pool.Close()
		if _, err := admin.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("dropping %s: %v", name, err)
		}
	})
	return pool
}

func tables(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		"SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_name <> 'goose_db_version' ORDER BY 1")
	if err != nil {
		t.Fatalf("listing tables: %v", err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("listing tables: %v", err)
	}
	return names
}

func TestMigrationsGoDownAndUpAgain(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	up := tables(t, pool)
	if !slices.Contains(up, "games") || !slices.Contains(up, "rating_runs") {
		t.Fatalf("after up: tables = %q, want games and rating_runs among them", up)
	}

	if err := ResetRatings(ctx, pool); err != nil {
		t.Fatalf("ResetRatings: %v", err)
	}
	if got := tables(t, pool); !slices.Equal(got, up) {
		t.Errorf("after reset: tables = %q, want those after up, %q", got, up)
	}

	if err := migrateRatingsDown(ctx, pool); err != nil {
		t.Fatalf("migrating down: %v", err)
	}
	if got := tables(t, pool); len(got) != 0 {
		t.Errorf("after down: tables = %q, want none", got)
	}
}

func TestMigrationsStartedTogetherApplyOnce(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	want := tables(t, pool)
	if err := migrateRatingsDown(ctx, pool); err != nil {
		t.Fatalf("migrating down: %v", err)
	}

	errs := make(chan error, 4)
	for range 4 {
		go func() { errs <- MigrateRatings(ctx, pool) }()
	}
	for range 4 {
		if err := <-errs; err != nil {
			t.Errorf("a migration started alongside others: %v", err)
		}
	}
	if got := tables(t, pool); !slices.Equal(got, want) {
		t.Errorf("tables = %q, want %q", got, want)
	}
}

func TestResetEmptiesTheTables(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	if _, err := LoadMatches(ctx, pool, []matchcsv.Match{match("s1.csv", 2, "ann", "bob")}, nil); err != nil {
		t.Fatalf("LoadMatches: %v", err)
	}
	if err := ResetRatings(ctx, pool); err != nil {
		t.Fatalf("ResetRatings: %v", err)
	}
	if n := count(t, pool, "games"); n != 0 {
		t.Errorf("games after reset = %d, want 0", n)
	}
}

func TestOnlyACompleteRunCanBeCurrent(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	newRun := func(status string) int64 {
		var id int64
		err := pool.QueryRow(ctx,
			"INSERT INTO rating_runs (engine_version, input_fingerprint, status) VALUES ('v1', 'f1', $1) RETURNING run_id",
			status).Scan(&id)
		if err != nil {
			t.Fatalf("inserting a %s run: %v", status, err)
		}
		return id
	}
	running, failed, complete, other := newRun("running"), newRun("failed"), newRun("complete"), newRun("complete")

	for _, id := range []int64{running, failed} {
		if _, err := pool.Exec(ctx, "INSERT INTO current_run (run_id) VALUES ($1)", id); err == nil {
			t.Errorf("run %d, not complete: made current, want refused", id)
		}
	}
	if _, err := pool.Exec(ctx, "INSERT INTO current_run (run_id) VALUES ($1)", complete); err != nil {
		t.Fatalf("making complete run %d current: %v", complete, err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO current_run (run_id) VALUES ($1)", other); err == nil {
		t.Errorf("a second current_run row was accepted, want refused")
	}
	if _, err := pool.Exec(ctx, "UPDATE rating_runs SET status = 'failed' WHERE run_id = $1", complete); err == nil {
		t.Errorf("the current run left 'complete', want refused")
	}
	if _, err := pool.Exec(ctx, "UPDATE current_run SET run_id = $1", other); err != nil {
		t.Errorf("moving current_run to run %d: %v", other, err)
	}
}

func match(file string, line int, names ...string) matchcsv.Match {
	m := matchcsv.Match{File: file, Line: line, Tournament: "Cup", Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)}
	for i, n := range names {
		m.Players = append(m.Players, matchcsv.Player{Name: n, Score: float64(i + 1)})
	}
	return m
}

func count(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return n
}

func TestLoadMatchesWritesMatchesRefusalsAndPlayers(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	matches := []matchcsv.Match{
		match("s1.csv", 2, "ann", "bob"),
		match("s1.csv", 3, "bob", "Ann", "cat", "dan"),
	}
	refused := []matchcsv.Refusal{
		{File: "s1.csv", Line: 4, Reason: matchcsv.Duplicate, Fields: []string{"Cup", "2024-01-02", "ann", "0", "bob", "0", "", "", "", "", "2"}},
	}

	got, err := LoadMatches(ctx, pool, matches, refused)
	if err != nil {
		t.Fatalf("LoadMatches: %v", err)
	}
	want := Loaded{BatchID: got.BatchID, Matches: 2, Refused: 1, Players: 5}
	if got != want {
		t.Errorf("LoadMatches = %+v, want %+v", got, want)
	}

	var p3, file string
	var line, players int
	var s4 float64
	err = pool.QueryRow(ctx,
		"SELECT l.players[3], l.cultures[4], l.player_count, g.source_file, g.source_line FROM game_lines l JOIN games g USING (game_id) WHERE g.source_line = 3").
		Scan(&p3, &s4, &players, &file, &line)
	if err != nil {
		t.Fatalf("reading back the 4-player match: %v", err)
	}
	if p3 != "cat" || s4 != 4 || players != 4 || file != "s1.csv" || line != 3 {
		t.Errorf("4-player match = %q %v %d %s:%d, want cat 4 4 s1.csv:3", p3, s4, players, file, line)
	}

	var reason string
	var fields []string
	if err := pool.QueryRow(ctx, "SELECT reason, fields FROM refused_rows").Scan(&reason, &fields); err != nil {
		t.Fatalf("reading back the refusal: %v", err)
	}
	if reason != string(matchcsv.Duplicate) || !slices.Equal(fields, refused[0].Fields) {
		t.Errorf("refusal = %q %q, want %q %q", reason, fields, matchcsv.Duplicate, refused[0].Fields)
	}
}

func TestLoadMatchesRefusesAFilledDatabase(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	if _, err := LoadMatches(ctx, pool, []matchcsv.Match{match("s1.csv", 2, "ann", "bob")}, nil); err != nil {
		t.Fatalf("first LoadMatches: %v", err)
	}

	_, err := LoadMatches(ctx, pool, []matchcsv.Match{match("s2.csv", 2, "cat", "dan")}, nil)
	if !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("second LoadMatches: err = %v, want %v", err, ErrNotEmpty)
	}
	if n := count(t, pool, "import_batches"); n != 1 {
		t.Errorf("import_batches = %d, want 1: the refused load left a batch", n)
	}
}

func TestLoadMatchesIsAllOrNothing(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	twice := match("s1.csv", 2, "ann", "bob")

	_, err := LoadMatches(ctx, pool, []matchcsv.Match{twice, twice}, nil)
	if err == nil {
		t.Fatal("LoadMatches with the same game twice: got no error, want the unique constraint")
	}
	for _, table := range []string{"import_batches", "games", "seats", "players"} {
		if n := count(t, pool, table); n != 0 {
			t.Errorf("%s = %d after a failed load, want 0", table, n)
		}
	}
}

func TestNewDBIsolatesTests(t *testing.T) {
	a, b := newDB(t), newDB(t)
	var nameA, nameB string
	if err := a.QueryRow(context.Background(), "SELECT current_database()").Scan(&nameA); err != nil {
		t.Fatal(err)
	}
	if err := b.QueryRow(context.Background(), "SELECT current_database()").Scan(&nameB); err != nil {
		t.Fatal(err)
	}
	if nameA == nameB {
		t.Errorf("two tests share database %s", fmt.Sprint(nameA))
	}
}

// countryOf reads the country of the member linked to the player named: nil for no member or no country.
func countryOf(t *testing.T, pool *pgxpool.Pool, name string) *string {
	t.Helper()
	var c *string
	err := pool.QueryRow(context.Background(),
		"SELECT m.country_code FROM members m JOIN players p ON p.player_id = m.player_id WHERE p.name = $1", name).Scan(&c)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		t.Fatalf("reading %s's country: %v", name, err)
	}
	return c
}

func TestSetCountriesWritesMembersThatHaveNotSignedIn(t *testing.T) {
	const linkedAnn = "INSERT INTO members (tta_name, player_id, country_code, zone) SELECT 'ann', player_id, 'DE', 'UTC' FROM players WHERE name = 'ann'"
	tests := []struct {
		name      string
		before    []string
		countries map[string]string
		want      CountriesSet
		members   []string
	}{
		{"a player with no member gets one, in the ratings' spelling, with nothing else known",
			nil, map[string]string{"ann": "PL", "ghost": "DE"}, CountriesSet{Set: 1, Unknown: 1},
			[]string{"ann|ann|PL|Unknown||"}},
		{"the member linked to the player takes the country, replacing an earlier one",
			[]string{linkedAnn}, map[string]string{"ann": "PL"}, CountriesSet{Set: 1},
			[]string{"ann|ann|PL|UTC||"}},
		{"a member of that name in any case is linked to the player and takes it",
			[]string{"INSERT INTO members (tta_name, discord_name, zone) VALUES ('ANN', 'player_one', 'UTC')"},
			map[string]string{"ann": "PL"}, CountriesSet{Set: 1},
			[]string{"ANN|ann|PL|UTC|player_one|"}},
		{"a member a sign-in is linked to keeps their own country",
			[]string{linkedAnn, "INSERT INTO accounts (member_id, linked_how, linked_at, created_at) SELECT member_id, 'match', now(), now() FROM members"},
			map[string]string{"ann": "PL"}, CountriesSet{Kept: 1},
			[]string{"ann|ann|DE|UTC||"}},
		{"a name that is a player in another case only is not that player",
			nil, map[string]string{"Ann": "PL"}, CountriesSet{Unknown: 1}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			ctx := context.Background()
			addPlayers(t, pool, "ann", "bob")
			for _, q := range tt.before {
				if _, err := pool.Exec(ctx, q); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
			}
			got, err := SetCountries(ctx, pool, tt.countries)
			if err != nil {
				t.Fatalf("SetCountries(%v): %v", tt.countries, err)
			}
			if got != tt.want {
				t.Errorf("SetCountries(%v) = %+v, want %+v", tt.countries, got, tt.want)
			}
			members := rowsOf(t, pool, `SELECT concat_ws('|', m.tta_name, coalesce(p.name, ''), coalesce(m.country_code, ''), m.zone, m.discord_name, m.email)
				FROM members m LEFT JOIN players p ON p.player_id = m.player_id ORDER BY 1`)
			if !slices.Equal(members, tt.members) {
				t.Errorf("members after SetCountries(%v) = %q, want %q", tt.countries, members, tt.members)
			}
		})
	}
}

func TestSetCountriesIsAllOrNothing(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "ann", "bob")

	if _, err := SetCountries(ctx, pool, map[string]string{"ann": "PL", "bob": "Poland"}); err == nil {
		t.Fatal("SetCountries with a bad code: no error, want one")
	}
	if c := countryOf(t, pool, "ann"); c != nil {
		t.Errorf("ann's country after a refused load = %q, want none", *c)
	}
}

func TestEverySecurityDefinerFunctionSearchesTheTemporarySchemaLast(t *testing.T) {
	pool := migrated(t)
	got := rowsOf(t, pool, `
		SELECT p.oid::regprocedure::text || ' ' || coalesce(array_to_string(p.proconfig, ','), 'no search_path')
		FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'public' AND p.prosecdef AND NOT coalesce('search_path=public, pg_temp' = ANY (p.proconfig), false)
		ORDER BY 1`)
	if len(got) > 0 {
		t.Errorf("SECURITY DEFINER functions without search_path public, pg_temp: %q", got)
	}
}

// runTables lists the tables whose rows are a run's results: those deleted with their run, partitions left out.
func runTables(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT conrelid::regclass::text FROM pg_constraint
		WHERE confrelid = 'rating_runs'::regclass AND contype = 'f' AND confdeltype = 'c' AND conparentid = 0
		ORDER BY 1`)
	if err != nil {
		t.Fatalf("listing run tables: %v", err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("listing run tables: %v", err)
	}
	return names
}

func TestEveryRunTableIsPartitionedByRun(t *testing.T) {
	pool := migrated(t)
	tables := runTables(t, pool)
	if len(tables) < 20 {
		t.Fatalf("run tables = %v, want the 21 or more a run writes", tables)
	}
	for _, table := range tables {
		var key string
		err := pool.QueryRow(context.Background(), `
			SELECT p.partstrat::text || ' ' || a.attname FROM pg_partitioned_table p
			JOIN pg_attribute a ON a.attrelid = p.partrelid AND a.attnum = p.partattrs[0]
			WHERE p.partrelid = $1::regclass`, table).Scan(&key)
		if err != nil || key != "l run_id" {
			t.Errorf("%s: partition key %q, %v; want a list of run_id", table, key, err)
		}
	}
}

func TestARunsRowsGoInItsOwnPartition(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	run := addRun(t, pool, "complete", time.Time{})
	withRatings(t, pool, run, 1)

	partitions := len(runPartitions(t, pool, run))
	if want := len(runTables(t, pool)); partitions != want {
		t.Errorf("run %d has %d partitions, want one in each of the %d run tables", run, partitions, want)
	}
	var in string
	if err := pool.QueryRow(ctx, "SELECT tableoid::regclass::text FROM player_ratings WHERE run_id = $1", run).Scan(&in); err != nil {
		t.Fatalf("finding the run's rating: %v", err)
	}
	if want := fmt.Sprintf("player_ratings_run%d", run); in != want {
		t.Errorf("run %d's rating is in %s, want %s", run, in, want)
	}
}

func TestLoadMatchesRecordsTheNamesTheFileGives(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	early, late := match("s1.csv", 2, "ann", "bob"), match("s1.csv", 3, "ann", "Cat")
	early.Date = time.Date(2023, 5, 6, 0, 0, 0, 0, time.UTC)
	late.Date = time.Date(2024, 7, 8, 0, 0, 0, 0, time.UTC)
	if _, err := LoadMatches(ctx, pool, []matchcsv.Match{early, late}, nil); err != nil {
		t.Fatalf("LoadMatches: %v", err)
	}

	got := rowsOf(t, pool, `SELECT n.name || ' ' || n.first_seen || ' ' || n.last_seen || ' ' || n.source
		FROM player_names n JOIN players p USING (player_id) WHERE p.name = n.name ORDER BY n.name COLLATE "C"`)
	want := []string{"Cat 2024-07-08 2024-07-08 match_file", "ann 2023-05-06 2024-07-08 match_file", "bob 2023-05-06 2023-05-06 match_file"}
	if !slices.Equal(got, want) {
		t.Errorf("player_names after loading a match file = %q, want %q", got, want)
	}
	if n := count(t, pool, "player_names"); n != 3 {
		t.Errorf("player_names = %d rows, want 3: one per player and name", n)
	}
}
