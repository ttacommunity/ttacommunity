//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// secondWorlds gives a second series the World Championship's results rules, and returns the error.
func secondWorlds(pool *pgxpool.Pool) error {
	_, err := pool.Exec(context.Background(),
		`INSERT INTO series (slug, name, results_rules) VALUES ('lantern_worlds', 'Lantern Worlds', 'worlds')`)
	return err
}

func TestMigration105LetsOneSeriesHoldEachRuleSet(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO series (slug, name, results_rules) VALUES ('worlds', 'World Championship', 'worlds')`); err != nil {
		t.Fatalf("adding the World Championship: %v", err)
	}
	var pg *pgconn.PgError
	if err := secondWorlds(pool); !errors.As(err, &pg) || pg.Code != "23505" {
		t.Fatalf("a second series on the worlds rules: err = %v, want a unique violation (23505)", err)
	}

	migrateTo(t, pool, 104, true)
	if err := secondWorlds(pool); err != nil {
		t.Fatalf("below 105, a second series on the worlds rules: %v, want it accepted as before", err)
	}
	if err := migrateRatingsTo(ctx, pool, 105); err == nil {
		t.Fatal("migrating up to 105 with two series on the worlds rules succeeded, want it refused")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM series WHERE slug = 'lantern_worlds'`); err != nil {
		t.Fatalf("removing the second series: %v", err)
	}
	migrateTo(t, pool, 105, false)
	if err := secondWorlds(pool); !errors.As(err, &pg) || pg.Code != "23505" {
		t.Errorf("after 105's down and up, a second series on the worlds rules: err = %v, want a unique violation (23505)", err)
	}
}
