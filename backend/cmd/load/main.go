// Command load seeds the ratings database from a directory of match CSVs.
// It checks every row first, and writes nothing if the database has matches.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/matchcsv"
	"ttacommunity/backend/internal/store"
)

func main() {
	reset := flag.Bool("reset", false, "delete every match, refusal and rating run first")
	migrate := flag.Bool("migrate", false, "only apply the ratings migrations, and load nothing")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: load [-reset] <directory of match CSVs>")
		fmt.Fprintln(os.Stderr, "       load -migrate")
		fmt.Fprintln(os.Stderr, "The database is RATINGS_DATABASE_URL.")
		flag.PrintDefaults()
	}
	flag.Parse()
	var err error
	switch {
	case *migrate && !*reset && flag.NArg() == 0:
		err = migrateOnly()
	case !*migrate && flag.NArg() == 1:
		err = run(*reset, flag.Arg(0))
	default:
		flag.Usage()
		os.Exit(64)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
}

// migrateOnly applies every ratings migration not yet applied.
func migrateOnly() error {
	dbURL := os.Getenv("RATINGS_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("RATINGS_DATABASE_URL is not set")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("opening the ratings database: %w", err)
	}
	defer pool.Close()
	if err := store.MigrateRatings(ctx, pool); err != nil {
		return err
	}
	fmt.Println("ratings migrations applied")
	return nil
}

func run(reset bool, dir string) error {
	dbURL := os.Getenv("RATINGS_DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("RATINGS_DATABASE_URL is not set")
	}
	res, err := matchcsv.CheckFiles(os.DirFS(dir))
	if err != nil {
		return fmt.Errorf("checking %s: %w", dir, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("opening the ratings database: %w", err)
	}
	defer pool.Close()

	if reset {
		err = store.ResetRatings(ctx, pool)
	} else {
		err = store.MigrateRatings(ctx, pool)
	}
	if err != nil {
		return err
	}
	loaded, err := store.LoadMatches(ctx, pool, res.Matches, res.Refused)
	if err != nil {
		return fmt.Errorf("loading: %w (to start again from empty: -reset, or --reset with scripts/ratings-db.sh)", err)
	}
	report(os.Stdout, res, loaded)
	return nil
}

// report writes what was read, loaded and refused. It names files and
// reasons, never a player.
func report(w io.Writer, res matchcsv.Result, loaded store.Loaded) {
	fmt.Fprintf(w, "files: %d (%s)\n", len(res.Files), strings.Join(res.Files, ", "))
	fmt.Fprintf(w, "rows read: %d\n", res.Rows)
	fmt.Fprintf(w, "loaded: %d matches, %d players (batch %d)\n", loaded.Matches, loaded.Players, loaded.BatchID)
	fmt.Fprintf(w, "refused: %d\n", loaded.Refused)

	byReason := map[matchcsv.Reason]int{}
	for _, r := range res.Refused {
		byReason[r.Reason]++
	}
	reasons := make([]string, 0, len(byReason))
	for r := range byReason {
		reasons = append(reasons, string(r))
	}
	slices.Sort(reasons)
	for _, r := range reasons {
		fmt.Fprintf(w, "  %-16s %d\n", r, byReason[matchcsv.Reason(r)])
	}
	if loaded.Refused > 0 {
		fmt.Fprintln(w, "Each refused row, with its file, line and reason, is in the refused_rows table.")
	}
}
