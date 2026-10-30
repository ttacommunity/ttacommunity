// The load job's challenges: the committed challenge list, and the old site's entries and weekly challenge, imported once.

package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/challenges"
	"ttacommunity/backend/internal/store"
)

// loadChallenges replaces the challenges with the list in file, after applying any pending migration.
func loadChallenges(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("opening %s: %w", file, err)
	}
	list, err := challenges.ReadList(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	return withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
		if err := store.MigrateRatings(ctx, pool); err != nil {
			return err
		}
		replaced, err := store.ReplaceChallenges(ctx, pool, list)
		if err != nil {
			return err
		}
		reportChallenges(os.Stdout, file, len(list), replaced)
		return nil
	})
}

// importChallengeEntries replaces the imported entries with the old site's entries in file, after applying any
// pending migration.
func importChallengeEntries(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("opening %s: %w", file, err)
	}
	read, err := challenges.ReadEntries(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	return withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
		if err := store.MigrateRatings(ctx, pool); err != nil {
			return err
		}
		imported, err := store.ImportChallengeEntries(ctx, pool, read.Entries)
		if err != nil {
			return err
		}
		reportChallengeEntries(os.Stdout, file, read, imported)
		return nil
	})
}

// loadChallengeWeeks replaces the weekly challenge's weeks with the old site's weekly.json in file, after applying any
// pending migration.
func loadChallengeWeeks(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("opening %s: %w", file, err)
	}
	weeks, err := challenges.ReadWeeks(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	return withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
		if err := store.MigrateRatings(ctx, pool); err != nil {
			return err
		}
		stored, err := store.ReplaceChallengeWeeks(ctx, pool, weeks)
		if err != nil {
			return err
		}
		fmt.Printf("file: %s\nweeks stored: %d\n", file, stored)
		return nil
	})
}

// reportChallenges writes what a load of the challenge list did.
func reportChallenges(w io.Writer, file string, read int, replaced store.ChallengesReplaced) {
	fmt.Fprintf(w, "file: %s\n", file)
	fmt.Fprintf(w, "challenges read: %d\n", read)
	fmt.Fprintf(w, "kept: %d, added: %d, removed: %d\n", replaced.Kept, replaced.Added, replaced.Removed)
}

// reportChallengeEntries writes what an import did, each refused entry by its id on the old site, never a player.
func reportChallengeEntries(w io.Writer, file string, read challenges.EntriesRead, imported store.ChallengeEntriesImported) {
	refused := append(read.Refused, imported.Refused...)
	fmt.Fprintf(w, "file: %s\n", file)
	fmt.Fprintf(w, "entries read: %d\n", read.Rows)
	fmt.Fprintf(w, "imported entries replaced: %d\n", imported.Replaced)
	fmt.Fprintf(w, "imported: %d, linked to a member: %d, members added for new names: %d\n", imported.Imported, imported.Linked, imported.Members)
	fmt.Fprintf(w, "refused: %d\n", len(refused))
	for _, r := range refused {
		fmt.Fprintf(w, "  entry %d: %s\n", r.OldID, r.Reason)
	}
}
