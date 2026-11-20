//go:build integration

package store

import (
	"context"
	"slices"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// oldMatchGameLines writes each match's game as "<match result> game=<share code> <report> by=<reporter> flag=<reason>
// ruling=<ruling>", read from blitz_matches as the schema before migration 102 holds it, in the order of matches.
const oldMatchGameLines = `
	SELECT concat_ws(' ', coalesce(m.result, '-'), 'game=' || coalesce(g.share_code, '-'), coalesce(m.report, '-'),
	                 'by=' || coalesce(mm.tta_name, '-'), 'flag=' || m.flag_reason, 'ruling=' || m.ruling)
	FROM blitz_matches m LEFT JOIN games g ON g.game_id = m.game_id
	LEFT JOIN blitz_entries x ON x.entry_id = m.reported_by LEFT JOIN members mm ON mm.member_id = x.member_id
	ORDER BY m.match_id`

func TestMigration102MovesMatchesGamesToTheirOwnTableAndBack(t *testing.T) {
	a, p, reader, ids, matches := reportsFixture(t)
	ctx := context.Background()
	if err := p.ReportBlitzGame(ctx, tokenHash("ann"), "us_open", "2026", matches[0], 1, reported("REP-1"), reportsAt); err != nil {
		t.Fatalf("ReportBlitzGame: %v", err)
	}
	if err := p.FlagBlitzResult(ctx, tokenHash("bob"), "us_open", "2026", matches[0], 1, "I won", reportsAt); err != nil {
		t.Fatalf("FlagBlitzResult: %v", err)
	}
	if err := a.RecordBlitzGame(ctx, "us_open", "2026", matches[1], 1, BlitzGame{ShareCode: "ADM-1", A: finished(30, 1), B: finished(60, 2), Finished: finishedAt}); err != nil {
		t.Fatalf("RecordBlitzGame: %v", err)
	}
	if err := a.RecordBlitzRuling(ctx, "us_open", "2026", matches[1], "a", "Dee left early"); err != nil {
		t.Fatalf("RecordBlitzRuling: %v", err)
	}
	cfg := a.pool.(*pgxpool.Pool).Config().Copy()
	cfg.AfterConnect = nil // the owner migrates, not the admin login
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("opening the owner's pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 101); return err }); err != nil {
		t.Fatalf("migrating down to 101: %v", err)
	}
	want := []string{"a game=REP-1 flagged by=Ann flag=I won ruling=", "a game=ADM-1 - by=- flag= ruling=Dee left early"}
	if got := rowsOf(t, pool, oldMatchGameLines); !slices.Equal(got, want) {
		t.Errorf("the matches after the Down = %q, want %q", got, want)
	}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.Up(ctx); return err }); err != nil {
		t.Fatalf("migrating up: %v", err)
	}
	if got, want := rowsOf(t, pool, "SELECT match_id || ' ' || number || ' ' || result FROM blitz_match_games ORDER BY match_id"),
		[]string{strconv.FormatInt(matches[0], 10) + " 1 a", strconv.FormatInt(matches[1], 10) + " 1 b"}; !slices.Equal(got, want) {
		t.Errorf("the match games after the Up = %q, want %q", got, want)
	}
	if got, want := reportLine(matchNow(t, reader, matches[0]), ids), "flagged by=Ann flag=I won"; got != want {
		t.Errorf("Ann v Bob's report after the Up = %q, want %q", got, want)
	}
	if got, want := resultLine(matchNow(t, reader, matches[1])), "a game=ADM-1 30/finished 60/finished on 2026-10-15T03:30:00Z ruling=Dee left early"; got != want {
		t.Errorf("Cy v Dee after the Up = %q, want %q", got, want)
	}
}
