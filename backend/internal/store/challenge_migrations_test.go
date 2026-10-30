//go:build integration

package store

import (
	"context"
	"slices"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestChallengeMigrationsGoDownAndUp(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	present := func() []string {
		return rowsOf(t, pool, `
			SELECT relname FROM pg_class WHERE relkind = 'r' AND relname LIKE 'challenge%'
			UNION SELECT proname FROM pg_proc WHERE proname = 'submit_challenge_entries' ORDER BY 1`)
	}
	want := []string{"challenge_entries", "challenge_week_places", "challenge_weeks", "challenges", "submit_challenge_entries"}
	if got := present(); !slices.Equal(got, want) {
		t.Fatalf("migrated: %q, want %q", got, want)
	}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 68); return err }); err != nil {
		t.Fatalf("migrating down to 68: %v", err)
	}
	if got := present(); len(got) != 0 {
		t.Errorf("at 68, %q remain, want none", got)
	}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.Up(ctx); return err }); err != nil {
		t.Fatalf("migrating up again: %v", err)
	}
	if got := present(); !slices.Equal(got, want) {
		t.Errorf("up again: %q, want %q", got, want)
	}
}
