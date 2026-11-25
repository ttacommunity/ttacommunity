//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// shareSecond sets a podium whose second place two players share, and returns the error.
func shareSecond(pool *pgxpool.Pool) error {
	_, err := pool.Exec(context.Background(), `INSERT INTO tournament_podium_overrides (tournament, season, place, player_id, note)
		SELECT 'royal_league', '1', place, player_id, '' FROM players JOIN (VALUES ('alder440', 1), ('kittiwake', 2), ('Eider', 2)) v(name, place) USING (name)`)
	return err
}

func TestMigration106LetsPlayersShareAPodiumPlace(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "alder440", "kittiwake", "Eider")
	if err := shareSecond(pool); err != nil {
		t.Fatalf("a shared second place: %v, want it kept", err)
	}
	var pg *pgconn.PgError
	if _, err := pool.Exec(ctx, `INSERT INTO tournament_podium_overrides (tournament, season, place, player_id, note)
		SELECT 'royal_league', '1', 3, player_id, '' FROM players WHERE name = 'kittiwake'`); !errors.As(err, &pg) || pg.Code != "23505" {
		t.Errorf("a player on one podium twice: err = %v, want a unique violation (23505)", err)
	}

	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 105); return err }); err == nil {
		t.Fatal("migrating down past 106 with a shared place succeeded, want it refused")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM tournament_podium_overrides`); err != nil {
		t.Fatalf("removing the podium: %v", err)
	}
	migrateTo(t, pool, 105, true)
	if err := shareSecond(pool); !errors.As(err, &pg) || pg.Code != "23505" {
		t.Errorf("below 106, a shared second place: err = %v, want a unique violation (23505)", err)
	}
	migrateTo(t, pool, 106, false)
	if err := shareSecond(pool); err != nil {
		t.Errorf("after 106's down and up, a shared second place: %v, want it kept", err)
	}
}
