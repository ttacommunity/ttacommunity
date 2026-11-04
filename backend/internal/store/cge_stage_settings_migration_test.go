//go:build integration

package store

import (
	"context"
	"slices"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestMigrationSeventyFiveKeepsCGETournamentTimes(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO cge_tournaments (cge_tournament, name, status, created)
		VALUES (7001, 'Lantern Open', 'running', '2026-05-13 23:42:50+00')`); err != nil {
		t.Fatal(err)
	}
	created := "SELECT created::text FROM cge_tournaments WHERE cge_tournament = 7001"
	migrate := func(down bool) {
		t.Helper()
		if err := withRatingsMigrations(pool, func(p *goose.Provider) error {
			if down {
				_, err := p.DownTo(ctx, 74)
				return err
			}
			_, err := p.Up(ctx)
			return err
		}); err != nil {
			t.Fatalf("migrating (down %t): %v", down, err)
		}
	}
	steps := []struct {
		name string
		down bool
		want string
	}{
		{"at 75 the time", false, "2026-05-13 23:42:50+00"},
		{"at 74 its UTC day", true, "2026-05-13"},
		{"up again that day's start", false, "2026-05-13 00:00:00+00"},
	}
	for i, s := range steps {
		if i > 0 {
			migrate(s.down)
		}
		if got := rowsOf(t, pool, created); !slices.Equal(got, []string{s.want}) {
			t.Errorf("%s: created = %q, want %q", s.name, got, s.want)
		}
	}
}

func TestMigrationSeventyFiveKeepsEachPlaysPerPlayerCount(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO series (slug, name) VALUES ('lantern_open', 'Lantern Open')`,
		`INSERT INTO tournaments (series_id, label) SELECT series_id, '2031' FROM series WHERE slug = 'lantern_open'`,
		`INSERT INTO stages (tournament_id, label) SELECT tournament_id, l FROM tournaments, (VALUES ('Round 1'), ('Round 2')) v(l)
		 WHERE label = '2031'`,
		`INSERT INTO settings (series_id, each_plays_2p, each_plays_3p, each_plays_4p, winner_score_minimum, source)
		 SELECT series_id, 0, 3, 3, 50, 'cge' FROM series WHERE slug = 'lantern_open'`,
		`INSERT INTO settings (stage_id, each_plays_4p, source) SELECT stage_id, 4, 'cge' FROM stages WHERE label = 'Round 2'`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	resolved := `SELECT st.label || ' ' || concat_ws('/', ss.each_plays_2p, ss.each_plays_3p, ss.each_plays_4p) || ' '
		|| coalesce(ss.winner_score_minimum::text, 'none')
		FROM stage_settings ss JOIN stages st USING (stage_id) JOIN tournaments t USING (tournament_id)
		WHERE t.label = '2031' ORDER BY 1`
	if got, want := rowsOf(t, pool, resolved), []string{"Round 1 0/3/3 50", "Round 2 0/3/4 50"}; !slices.Equal(got, want) {
		t.Errorf("each stage's games per player count and least winner score = %q, want %q (its own, else its series')", got, want)
	}

	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 74); return err }); err != nil {
		t.Fatalf("migrating down to 74: %v", err)
	}
	old := `SELECT coalesce(s.each_plays::text, 'none') FROM settings s
		LEFT JOIN series se USING (series_id) LEFT JOIN stages st USING (stage_id)
		WHERE se.slug = 'lantern_open' OR st.label = 'Round 2' ORDER BY s.series_id NULLS LAST`
	if got, want := rowsOf(t, pool, old), []string{"3", "4"}; !slices.Equal(got, want) {
		t.Errorf("at 74, each_plays = %q, want %q: the count every player count with games shares", got, want)
	}

	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.Up(ctx); return err }); err != nil {
		t.Fatalf("migrating up again: %v", err)
	}
	if got, want := rowsOf(t, pool, resolved), []string{"Round 1  none", "Round 2  none"}; !slices.Equal(got, want) {
		t.Errorf("up again, each stage's games per player count and least winner score = %q, want %q: none until the "+
			"next extract load", got, want)
	}
}
