// Package store reads and writes the databases. Tests that touch shared
// tables live here, so no other package can empty them mid-test.
package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"ttacommunity/backend/internal/matchcsv"
	"ttacommunity/backend/migrations"
)

// ErrNotEmpty is returned when a load would add to matches already loaded.
var ErrNotEmpty = errors.New("the ratings database already has matches")

// Loaded counts what one load wrote.
type Loaded struct {
	BatchID int64
	Matches int
	Refused int
	Players int
}

// withRatingsMigrations runs fn with a goose provider for the ratings migrations.
func withRatingsMigrations(pool *pgxpool.Pool, fn func(*goose.Provider) error) error {
	dir, err := fs.Sub(migrations.Ratings, "ratings")
	if err != nil {
		return fmt.Errorf("opening the ratings migrations: %w", err)
	}
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, dir)
	if err != nil {
		return fmt.Errorf("reading the ratings migrations: %w", err)
	}
	return fn(p)
}

// MigrateRatings applies every ratings migration not yet applied.
func MigrateRatings(ctx context.Context, pool *pgxpool.Pool) error {
	return withRatingsMigrations(pool, func(p *goose.Provider) error {
		if _, err := p.Up(ctx); err != nil {
			return fmt.Errorf("migrating the ratings database up: %w", err)
		}
		return nil
	})
}

func migrateRatingsDown(ctx context.Context, pool *pgxpool.Pool) error {
	return withRatingsMigrations(pool, func(p *goose.Provider) error {
		if _, err := p.DownTo(ctx, 0); err != nil {
			return fmt.Errorf("migrating the ratings database down: %w", err)
		}
		return nil
	})
}

// ResetRatings rolls every ratings migration back, then applies them all
// again. Every match, refusal and run is deleted.
func ResetRatings(ctx context.Context, pool *pgxpool.Pool) error {
	if err := migrateRatingsDown(ctx, pool); err != nil {
		return err
	}
	return MigrateRatings(ctx, pool)
}

// LoadMatches writes one load's matches and refusals, and adds their players,
// in one transaction. It returns ErrNotEmpty if matches are already loaded.
func LoadMatches(ctx context.Context, pool *pgxpool.Pool, matches []matchcsv.Match, refused []matchcsv.Refusal) (Loaded, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Loaded{}, fmt.Errorf("starting the load: %w", err)
	}
	defer tx.Rollback(ctx)

	var filled bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM matches)").Scan(&filled); err != nil {
		return Loaded{}, fmt.Errorf("checking for loaded matches: %w", err)
	}
	if filled {
		return Loaded{}, ErrNotEmpty
	}

	var got Loaded
	if err := tx.QueryRow(ctx, "INSERT INTO import_batches (source) VALUES ('load') RETURNING batch_id").Scan(&got.BatchID); err != nil {
		return Loaded{}, fmt.Errorf("starting an import batch: %w", err)
	}

	n, err := tx.CopyFrom(ctx, pgx.Identifier{"matches"},
		[]string{"batch_id", "source_file", "source_line", "tournament", "date", "player_count",
			"player1", "score1", "player2", "score2", "player3", "score3", "player4", "score4"},
		pgx.CopyFromSlice(len(matches), func(i int) ([]any, error) {
			m := matches[i]
			row := []any{got.BatchID, m.File, m.Line, m.Tournament, m.Date, len(m.Players)}
			for seat := range 4 {
				if seat < len(m.Players) {
					row = append(row, m.Players[seat].Name, m.Players[seat].Score)
				} else {
					row = append(row, nil, nil)
				}
			}
			return row, nil
		}))
	if err != nil {
		return Loaded{}, fmt.Errorf("writing matches: %w", err)
	}
	got.Matches = int(n)

	n, err = tx.CopyFrom(ctx, pgx.Identifier{"refused_rows"},
		[]string{"batch_id", "source_file", "source_line", "reason", "fields"},
		pgx.CopyFromSlice(len(refused), func(i int) ([]any, error) {
			r := refused[i]
			return []any{got.BatchID, r.File, r.Line, string(r.Reason), r.Fields}, nil
		}))
	if err != nil {
		return Loaded{}, fmt.Errorf("writing refused rows: %w", err)
	}
	got.Refused = int(n)

	tag, err := tx.Exec(ctx, `
		INSERT INTO players (name)
		SELECT DISTINCT name
		FROM matches, unnest(ARRAY[player1, player2, player3, player4]) AS name
		WHERE batch_id = $1 AND name IS NOT NULL
		ORDER BY name
		ON CONFLICT (name) DO NOTHING`, got.BatchID)
	if err != nil {
		return Loaded{}, fmt.Errorf("adding players: %w", err)
	}
	got.Players = int(tag.RowsAffected())

	if err := tx.Commit(ctx); err != nil {
		return Loaded{}, fmt.Errorf("committing the load: %w", err)
	}
	return got, nil
}
