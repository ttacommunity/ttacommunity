//go:build integration

package store

import (
	"context"
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5"
)

func TestAnEditionIsPairedByTheMethodsLastSet(t *testing.T) {
	a, reader, _ := blitzWithEntrants(t)
	ctx := context.Background()
	ed, err := reader.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	if ed.Pairing != DefaultBlitzPairing {
		t.Errorf("a new edition's pairing = %+v, want %+v", ed.Pairing, DefaultBlitzPairing)
	}
	for _, first := range BlitzFirstMethods {
		for _, later := range BlitzLaterMethods {
			set := BlitzPairing{First: first, Later: later, Ranks: BlitzRankOrders[(len(first)+len(later))%len(BlitzRankOrders)]}
			if err := a.SetBlitzPairing(ctx, "us_open", "2026", set); err != nil {
				t.Fatalf("SetBlitzPairing(%+v): %v", set, err)
			}
			if ed, err = reader.BlitzEdition(ctx, "us_open", "2026"); err != nil || ed.Pairing != set {
				t.Errorf("pairing = %+v, %v; want %+v", ed.Pairing, err, set)
			}
		}
	}
	for _, bad := range []BlitzPairing{{First: "rating", Later: "dutch", Ranks: "rating"}, {First: "dutch", Later: "dutch", Ranks: "random"}, {First: "", Later: "", Ranks: ""}} {
		if err := a.SetBlitzPairing(ctx, "us_open", "2026", bad); !errors.Is(err, ErrBadBlitzPairing) {
			t.Errorf("SetBlitzPairing(%+v) = %v, want ErrBadBlitzPairing", bad, err)
		}
	}
	if err := a.SetBlitzPairing(ctx, "us_open", "2027", DefaultBlitzPairing); !errors.Is(err, ErrNoBlitzEdition) {
		t.Errorf("SetBlitzPairing(an unknown edition) = %v, want ErrNoBlitzEdition", err)
	}
}

func TestARoundKeepsTheMethodItWasPairedWith(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	ann, bob, cy, dee := ids[0], ids[1], ids[2], ids[3]
	for _, method := range []string{"dutch", "dutch-timezone", "timezone", "rating", "random"} {
		r, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Method: method, Seed: 1, Matches: []BlitzMatch{{A: ann, B: &bob}, {A: cy, B: &dee}}})
		if err != nil {
			t.Fatalf("AddBlitzRound(%s): %v", method, err)
		}
		ed, err := a.BlitzEdition(ctx, "us_open", "2026")
		if err != nil {
			t.Fatalf("BlitzEdition: %v", err)
		}
		if got := ed.Rounds[len(ed.Rounds)-1].Method; got != method {
			t.Errorf("round method = %q, want %q", got, method)
		}
		if err := a.DeleteBlitzRound(ctx, "us_open", "2026", r.ID); err != nil {
			t.Fatalf("DeleteBlitzRound: %v", err)
		}
	}
}

func TestTheDatabaseRefusesAPairingMethodOutOfPlace(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	if _, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Method: "dutch", Seed: 1, Matches: []BlitzMatch{{A: ids[0], B: &ids[1]}, {A: ids[2], B: &ids[3]}}}); err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	tests := []struct{ name, sql, constraint string }{
		{"round 1 by rating", "UPDATE blitz_editions SET pairing_first = 'rating'", "blitz_editions_pairing_first_check"},
		{"later rounds by a method nobody knows", "UPDATE blitz_editions SET pairing_later = 'swiss-ish'", "blitz_editions_pairing_later_check"},
		{"starting ranks by something else", "UPDATE blitz_editions SET pairing_ranks = 'random'", "blitz_editions_pairing_ranks_check"},
		{"a starting rank of 0", "UPDATE blitz_entries SET starting_rank = 0", "blitz_entries_starting_rank_check"},
		{"a method nobody knows", "UPDATE blitz_rounds SET method = 'swiss-ish'", "blitz_rounds_method_check"},
		{"a Swiss round without one", "UPDATE blitz_rounds SET method = NULL", "blitz_rounds_method_kind_check"},
		{"a knockout round with one", "UPDATE blitz_rounds SET kind = 'knockout'", "blitz_rounds_method_kind_check"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := a.pool.Exec(ctx, tt.sql); err == nil || !strings.Contains(err.Error(), tt.constraint) {
				t.Errorf("%s = %v, want %s to refuse it", tt.sql, err, tt.constraint)
			}
		})
	}
}

func TestTheMigrationKeepsTimezonePairingForEditionsSoFar(t *testing.T) {
	pool := newDB(t)
	ctx := context.Background()
	if err := migrateRatingsTo(ctx, pool, 53); err != nil {
		t.Fatalf("migrateRatingsTo(53): %v", err)
	}
	for _, q := range []string{
		`INSERT INTO series (slug, name, blitz) VALUES ('us_open', 'US Open', true)`,
		`INSERT INTO tournaments (series_id, label) SELECT series_id, '2026' FROM series`,
		`INSERT INTO tournaments (series_id, label) SELECT series_id, '2027' FROM series`,
		`INSERT INTO blitz_editions (tournament_id, swiss_rounds, knockout_size) SELECT tournament_id, 8, 8 FROM tournaments WHERE label = '2026'`,
		`INSERT INTO blitz_rounds (tournament_id, number, timezone, rating, seed)
		 SELECT tournament_id, n, n = 1, n = 2, 1 FROM blitz_editions, generate_series(1, 3) n`,
		`INSERT INTO blitz_rounds (tournament_id, number, kind, seed) SELECT tournament_id, 4, 'knockout', 1 FROM blitz_editions`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("building the old shape: %v\n%s", err, q)
		}
	}
	if err := migrateRatingsTo(ctx, pool, 54); err != nil {
		t.Fatalf("migrateRatingsTo(54): %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO blitz_editions (tournament_id, swiss_rounds, knockout_size)
		SELECT tournament_id, 8, 8 FROM tournaments WHERE label = '2027'`); err != nil {
		t.Fatalf("adding a new edition: %v", err)
	}
	rows, err := pool.Query(ctx, `
		SELECT t.label || ' ' || e.pairing_first || '/' || e.pairing_later FROM blitz_editions e JOIN tournaments t USING (tournament_id)
		UNION ALL
		SELECT 'round ' || number || ' ' || coalesce(method, 'none') FROM blitz_rounds
		ORDER BY 1`)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	want := []string{"2026 timezone/timezone", "2027 dutch/dutch", "round 1 timezone", "round 2 rating", "round 3 random", "round 4 none"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("after the migration (-want +got):\n%s", diff)
	}
}

func TestStartingRanksStayUntilTheEditionHasNoRound(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	ann, bob, cy, dee := ids[0], ids[1], ids[2], ids[3]
	ranksOf := func() map[int64]int {
		t.Helper()
		ed, err := a.BlitzEdition(ctx, "us_open", "2026")
		if err != nil {
			t.Fatalf("BlitzEdition: %v", err)
		}
		got := map[int64]int{}
		for _, e := range ed.Entrants {
			got[e.ID] = e.StartingRank
		}
		return got
	}
	if got := ranksOf(); !maps.Equal(got, map[int64]int{ann: 0, bob: 0, cy: 0, dee: 0}) {
		t.Errorf("starting ranks before any are set = %v, want none", got)
	}
	set := map[int64]int{ann: 2, bob: 4, cy: 1, dee: 3}
	if err := a.SetBlitzStartingRanks(ctx, "us_open", "2026", set); err != nil {
		t.Fatalf("SetBlitzStartingRanks: %v", err)
	}
	if got := ranksOf(); !maps.Equal(got, set) {
		t.Errorf("starting ranks = %v, want %v", got, set)
	}
	if err := a.SetBlitzStartingRanks(ctx, "us_open", "2026", map[int64]int{ann: 0}); err == nil {
		t.Errorf("SetBlitzStartingRanks(rank 0) = nil, want an error: a rank starts at 1")
	}
	if err := a.SetBlitzStartingRanks(ctx, "us_open", "2027", set); !errors.Is(err, ErrNoBlitzEdition) {
		t.Errorf("SetBlitzStartingRanks(an unknown edition) = %v, want ErrNoBlitzEdition", err)
	}
	r1, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Method: "dutch", Seed: 1, Matches: []BlitzMatch{{A: ann, B: &bob}, {A: cy, B: &dee}}})
	if err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	if err := a.DeleteBlitzRound(ctx, "us_open", "2026", r1.ID); err != nil {
		t.Fatalf("DeleteBlitzRound(round 1): %v", err)
	}
	if got := ranksOf(); !maps.Equal(got, map[int64]int{ann: 0, bob: 0, cy: 0, dee: 0}) {
		t.Errorf("starting ranks after deleting the last round = %v, want none", got)
	}
	if err := a.SetBlitzStartingRanks(ctx, "us_open", "2026", set); err != nil {
		t.Fatalf("SetBlitzStartingRanks: %v", err)
	}
	if _, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Method: "dutch", Seed: 1, Matches: []BlitzMatch{{A: ann, B: &bob}, {A: cy, B: &dee}}}); err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	if err := a.ResetBlitzEdition(ctx, "us_open", "2026"); err != nil {
		t.Fatalf("ResetBlitzEdition: %v", err)
	}
	if got := ranksOf(); !maps.Equal(got, map[int64]int{ann: 0, bob: 0, cy: 0, dee: 0}) {
		t.Errorf("starting ranks after a reset = %v, want none", got)
	}
}
