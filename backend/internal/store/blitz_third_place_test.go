//go:build integration

package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// semiFinalsPlayed plays the entrants (Ann, Bob, Cy, Dee) through a 1-round Swiss and a top-4 knockout's semi-finals,
// published and won by side a, and returns the semi-finals.
func semiFinalsPlayed(t *testing.T, a *AdminStore, ids []int64) BlitzRound {
	t.Helper()
	ctx := context.Background()
	ann, bob, cy, dee := ids[0], ids[1], ids[2], ids[3]
	if err := a.SetBlitzFormat(ctx, "us_open", "2026", BlitzFormat{SwissRounds: 1, KnockoutSize: 4}); err != nil {
		t.Fatalf("SetBlitzFormat: %v", err)
	}
	swiss, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Method: "dutch", Order: "rating", Seed: 1, Matches: []BlitzMatch{{A: ann, B: &bob}, {A: cy, B: &dee}}})
	if err != nil {
		t.Fatalf("AddBlitzRound(swiss): %v", err)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", swiss.ID); err != nil {
		t.Fatalf("PublishBlitzRound(swiss): %v", err)
	}
	for _, m := range swiss.Matches {
		if err := a.RecordBlitzRuling(ctx, "us_open", "2026", m.ID, "a", "ruled"); err != nil {
			t.Fatalf("RecordBlitzRuling(swiss): %v", err)
		}
	}
	semi, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Kind: "knockout", Seed: 2, Matches: []BlitzMatch{ko(ann, dee, 0, 1, 4), ko(cy, bob, 1, 2, 3)}})
	if err != nil {
		t.Fatalf("AddBlitzRound(semi-finals): %v", err)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", semi.ID); err != nil {
		t.Fatalf("PublishBlitzRound(semi-finals): %v", err)
	}
	for _, m := range semi.Matches {
		if err := a.RecordBlitzRuling(ctx, "us_open", "2026", m.ID, "a", "ruled"); err != nil {
			t.Fatalf("RecordBlitzRuling(semi-finals): %v", err)
		}
	}
	return semi
}

// ko is a knockout match of a v b in slot, with their seeds.
func ko(a, b int64, slot, seedA, seedB int) BlitzMatch {
	return BlitzMatch{A: a, B: &b, Slot: new(slot), SeedA: new(seedA), SeedB: new(seedB)}
}

func TestTheThirdPlaceMatchCanBeSetUntilTheFinalIsPaired(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	semiFinalsPlayed(t, a, ids)
	on := BlitzFormat{SwissRounds: 1, KnockoutSize: 4, ThirdPlace: true}
	if err := a.SetBlitzFormat(ctx, "us_open", "2026", on); err != nil {
		t.Fatalf("turning the third-place match on after the semi-finals = %v, want it set", err)
	}
	if ed, err := a.BlitzEdition(ctx, "us_open", "2026"); err != nil || !ed.Format.ThirdPlace {
		t.Errorf("the format read back = %+v, %v; want the third-place match on", ed.Format, err)
	}
	if err := a.SetBlitzFormat(ctx, "us_open", "2026", BlitzFormat{SwissRounds: 2, KnockoutSize: 4, ThirdPlace: true}); !errors.Is(err, ErrBlitzFormatLocked) {
		t.Errorf("changing the Swiss rounds with it once the knockout started = %v, want ErrBlitzFormatLocked", err)
	}
	ann, bob, cy, dee := ids[0], ids[1], ids[2], ids[3]
	third := ko(dee, bob, 1, 4, 3)
	third.ThirdPlace = true
	if _, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Kind: "knockout", Seed: 3, Matches: []BlitzMatch{ko(ann, cy, 0, 1, 2), third}}); err != nil {
		t.Fatalf("AddBlitzRound(final): %v", err)
	}
	if err := a.SetBlitzFormat(ctx, "us_open", "2026", BlitzFormat{SwissRounds: 1, KnockoutSize: 4}); !errors.Is(err, ErrBlitzThirdPlaceLocked) {
		t.Errorf("turning the third-place match off once the final is paired = %v, want ErrBlitzThirdPlaceLocked", err)
	}
	if err := a.SetBlitzFormat(ctx, "us_open", "2026", on); err != nil {
		t.Errorf("saving the format unchanged once the final is paired = %v, want no error", err)
	}
}

func TestTheChampionWaitsForTheThirdPlaceMatch(t *testing.T) {
	tests := []struct {
		name    string
		decided []int
		want    string
	}{
		{"only the third-place match decided", []int{1}, ""},
		{"only the final decided", []int{0}, ""},
		{"both decided: the final's winner", []int{0, 1}, "Cy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, reader, ids := blitzWithEntrants(t)
			ctx := context.Background()
			semiFinalsPlayed(t, a, ids)
			ann, bob, cy, dee := ids[0], ids[1], ids[2], ids[3]
			third := ko(dee, bob, 1, 4, 3)
			third.ThirdPlace = true
			final, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Kind: "knockout", Seed: 3, Matches: []BlitzMatch{ko(ann, cy, 0, 1, 2), third}})
			if err != nil {
				t.Fatalf("AddBlitzRound(final): %v", err)
			}
			if err := a.PublishBlitzRound(ctx, "us_open", "2026", final.ID); err != nil {
				t.Fatalf("PublishBlitzRound(final): %v", err)
			}
			for _, i := range tt.decided {
				if err := a.RecordBlitzRuling(ctx, "us_open", "2026", final.Matches[i].ID, "b", "ruled"); err != nil {
					t.Fatalf("RecordBlitzRuling(match %d): %v", i, err)
				}
			}
			ed, err := reader.BlitzEdition(ctx, "us_open", "2026")
			if err != nil {
				t.Fatalf("BlitzEdition: %v", err)
			}
			if got := nameOr(ed.Champion); got != tt.want {
				t.Errorf("the champion = %q, want %q", got, tt.want)
			}
			if got := ed.Rounds[2].Matches; len(got) != 2 || got[0].ThirdPlace || !got[1].ThirdPlace {
				t.Errorf("the final round's matches = %+v, want the final and then the third-place match", got)
			}
		})
	}
}

// nameOr is what s points to, or "" for nil.
func nameOr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func TestMigration104sDownRefusesAThirdPlaceMatch(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	semiFinalsPlayed(t, a, ids)
	third := ko(ids[3], ids[1], 1, 4, 3)
	third.ThirdPlace = true
	if _, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Kind: "knockout", Seed: 3, Matches: []BlitzMatch{ko(ids[0], ids[2], 0, 1, 2), third}}); err != nil {
		t.Fatalf("AddBlitzRound(final): %v", err)
	}
	cfg := a.pool.(*pgxpool.Pool).Config().Copy()
	cfg.AfterConnect = nil // the owner migrates, not the admin login
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("opening the owner's pool: %v", err)
	}
	t.Cleanup(pool.Close)
	err = withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 103); return err })
	if err == nil || !strings.Contains(err.Error(), "third-place match") {
		t.Errorf("migrating down to 103 with a third-place match = %v, want a refusal naming it", err)
	}
	if got := rowsOf(t, pool, "SELECT count(*)::text FROM blitz_matches WHERE third_place"); len(got) != 1 || got[0] != "1" {
		t.Errorf("the third-place matches after the refused Down = %q, want it kept", got)
	}
}
