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
	"github.com/pressly/goose/v3/lock"

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

// withRatingsMigrations runs fn with a goose provider for the ratings migrations, holding
// goose's advisory lock so migrations started together (two api pods) run one after another.
func withRatingsMigrations(pool *pgxpool.Pool, fn func(*goose.Provider) error) error {
	dir, err := fs.Sub(migrations.Ratings, "ratings")
	if err != nil {
		return fmt.Errorf("opening the ratings migrations: %w", err)
	}
	locker, err := lock.NewPostgresSessionLocker(lock.WithLockTimeout(1, 300))
	if err != nil {
		return fmt.Errorf("making the migration lock: %w", err)
	}
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, dir, goose.WithSessionLocker(locker))
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

// migrateRatingsTo applies the ratings migrations up to and including version.
func migrateRatingsTo(ctx context.Context, pool *pgxpool.Pool, version int64) error {
	return withRatingsMigrations(pool, func(p *goose.Provider) error {
		if _, err := p.UpTo(ctx, version); err != nil {
			return fmt.Errorf("migrating the ratings database up to %d: %w", version, err)
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

// CountriesSet counts what SetCountries did: players given a country, and
// names that are not players.
type CountriesSet struct {
	Set     int
	Unknown int
}

// SetCountries sets each named player's country, in one transaction. A name
// that is not a player is counted, not added.
func SetCountries(ctx context.Context, pool *pgxpool.Pool, countries map[string]string) (CountriesSet, error) {
	names := make([]string, 0, len(countries))
	codes := make([]string, 0, len(countries))
	for n, c := range countries {
		names = append(names, n)
		codes = append(codes, c)
	}
	tag, err := pool.Exec(ctx, `
		UPDATE players p SET country_code = c.code
		FROM unnest($1::text[], $2::text[]) AS c(name, code)
		WHERE p.name = c.name`, names, codes)
	if err != nil {
		return CountriesSet{}, fmt.Errorf("setting countries: %w", err)
	}
	set := int(tag.RowsAffected())
	return CountriesSet{Set: set, Unknown: len(countries) - set}, nil
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
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM games)").Scan(&filled); err != nil {
		return Loaded{}, fmt.Errorf("checking for loaded games: %w", err)
	}
	if filled {
		return Loaded{}, ErrNotEmpty
	}

	var got Loaded
	if err := tx.QueryRow(ctx, "INSERT INTO import_batches (source) VALUES ('load') RETURNING batch_id").Scan(&got.BatchID); err != nil {
		return Loaded{}, fmt.Errorf("starting an import batch: %w", err)
	}

	n, err := tx.CopyFrom(ctx, pgx.Identifier{"games"},
		[]string{"batch_id", "source_file", "source_line", "name", "date"},
		pgx.CopyFromSlice(len(matches), func(i int) ([]any, error) {
			m := matches[i]
			return []any{got.BatchID, m.File, m.Line, m.Tournament, m.Date}, nil
		}))
	if err != nil {
		return Loaded{}, fmt.Errorf("writing games: %w", err)
	}
	got.Matches = int(n)
	var files, names []string
	var lines, seatNos []int32
	var scores []float64
	for _, m := range matches {
		for i, p := range m.Players {
			files, lines, seatNos = append(files, m.File), append(lines, int32(m.Line)), append(seatNos, int32(i+1))
			names, scores = append(names, p.Name), append(scores, p.Score)
		}
	}

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
		SELECT DISTINCT name FROM unnest($1::text[]) AS name ORDER BY name
		ON CONFLICT (name) DO NOTHING`, names)
	if err != nil {
		return Loaded{}, fmt.Errorf("adding players: %w", err)
	}
	got.Players = int(tag.RowsAffected())

	// The file says only the score: culture is never negative, and place keeps the score's order.
	if _, err := tx.Exec(ctx, `
		INSERT INTO seats (game_id, seat, player_id, player_name, culture, recorded_score, place)
		SELECT g.game_id, s.seat, p.player_id, s.name, greatest(s.score, 0), s.score,
		       rank() OVER (PARTITION BY g.game_id ORDER BY s.score DESC)
		FROM unnest($2::text[], $3::int[], $4::int[], $5::text[], $6::float8[]) AS s(file, line, seat, name, score)
		JOIN games g ON g.batch_id = $1 AND g.source_file = s.file AND g.source_line = s.line
		JOIN players p ON p.name = s.name`,
		got.BatchID, files, lines, seatNos, names, scores); err != nil {
		return Loaded{}, fmt.Errorf("writing seats: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Loaded{}, fmt.Errorf("committing the load: %w", err)
	}
	return got, nil
}
