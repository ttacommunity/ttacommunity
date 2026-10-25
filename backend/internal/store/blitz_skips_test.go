//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// skipsOf is each entrant's skipped rounds, by name, for those with any.
func skipsOf(ed BlitzEdition) map[string][]int {
	out := map[string][]int{}
	for _, e := range ed.Entrants {
		if len(e.Skips) > 0 {
			out[e.Name] = e.Skips
		}
	}
	return out
}

func TestASkipLeavesAnEntrantOutOfTheNextSwissRound(t *testing.T) {
	a, reader, ids := blitzWithEntrants(t)
	ctx := context.Background()
	ann, bob, cy, dee := ids[0], ids[1], ids[2], ids[3]
	if n, err := a.SkipBlitzRound(ctx, "us_open", "2026", dee); err != nil || n != 1 {
		t.Fatalf("SkipBlitzRound(Dee) before round 1 = %d, %v; want round 1", n, err)
	}
	if _, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Method: "dutch", Seed: 1, Matches: []BlitzMatch{{A: ann, B: &dee}}}); !errors.Is(err, ErrBadBlitzMatch) || !strings.Contains(err.Error(), "Dee") {
		t.Errorf("pairing Dee in the round she skips = %v, want ErrBadBlitzMatch naming Dee", err)
	}
	r1, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Method: "dutch", Seed: 1, Matches: []BlitzMatch{{A: ann, B: &bob}, {A: cy}}})
	if err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", r1.ID); err != nil {
		t.Fatalf("publishing round 1 without Dee, who skips it: %v", err)
	}
	if n, err := a.SkipBlitzRound(ctx, "us_open", "2026", dee); err != nil || n != 2 {
		t.Fatalf("SkipBlitzRound(Dee) once round 1 is published = %d, %v; want round 2", n, err)
	}

	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	if diff := cmp.Diff(map[string][]int{"Dee": {1, 2}}, skipsOf(ed)); diff != "" {
		t.Errorf("admin's skips (-want +got):\n%s", diff)
	}
	public, err := reader.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("the site's BlitzEdition: %v", err)
	}
	if diff := cmp.Diff(map[string][]int{"Dee": {1}}, skipsOf(public)); diff != "" {
		t.Errorf("the site's skips, published rounds only (-want +got):\n%s", diff)
	}

	if err := a.UnskipBlitzRound(ctx, "us_open", "2026", dee); err != nil {
		t.Fatalf("UnskipBlitzRound(Dee): %v", err)
	}
	if ed, err = a.BlitzEdition(ctx, "us_open", "2026"); err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	if diff := cmp.Diff(map[string][]int{"Dee": {1}}, skipsOf(ed)); diff != "" {
		t.Errorf("skips once round 2's is undone, round 1's kept (-want +got):\n%s", diff)
	}
	if err := a.UnskipBlitzRound(ctx, "us_open", "2026", dee); !errors.Is(err, ErrBlitzNotSkipping) {
		t.Errorf("undoing a skip of a published round = %v, want ErrBlitzNotSkipping", err)
	}
}

func TestSkippingADraftRoundTakesThePlayerOutOfIt(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	ann, bob, cy, dee := ids[0], ids[1], ids[2], ids[3]
	if _, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Method: "dutch", Seed: 1, Matches: []BlitzMatch{{A: ann, B: &bob}, {A: cy, B: &dee}}}); err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	if n, err := a.SkipBlitzRound(ctx, "us_open", "2026", bob); err != nil || n != 1 {
		t.Fatalf("SkipBlitzRound(Bob) with round 1 a draft = %d, %v; want round 1", n, err)
	}
	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	if diff := cmp.Diff([]string{fmt.Sprintf("round 1 swiss published=false: %d-%d", cy, dee)}, roundLines(ed.Rounds)); diff != "" {
		t.Errorf("the draft once Bob skips it, Ann left unpaired (-want +got):\n%s", diff)
	}
	err = a.PublishBlitzRound(ctx, "us_open", "2026", roundID(t, a, 1))
	if !errors.Is(err, ErrBlitzRoundIncomplete) || !strings.Contains(err.Error(), "Ann") || strings.Contains(err.Error(), "Bob") {
		t.Errorf("publishing with Ann unpaired = %v, want ErrBlitzRoundIncomplete naming Ann, not Bob", err)
	}
	if err := a.UnskipBlitzRound(ctx, "us_open", "2026", bob); err != nil {
		t.Fatalf("UnskipBlitzRound(Bob) while the round is a draft: %v", err)
	}
	if err := a.SetBlitzRoundMatches(ctx, "us_open", "2026", roundID(t, a, 1), []BlitzMatch{{A: cy, B: &dee}, {A: ann, B: &bob}}); err != nil {
		t.Errorf("pairing Bob again once his skip is undone: %v", err)
	}
}

func TestSkipsThatCannotBe(t *testing.T) {
	skip := func(a *AdminStore, edition string, id int64) error {
		_, err := a.SkipBlitzRound(context.Background(), "us_open", edition, id)
		return err
	}
	tests := []struct {
		name  string
		setup func(t *testing.T, a *AdminStore, ids []int64)
		do    func(a *AdminStore, ids []int64) error
		want  error
	}{
		{"skipping for an unknown edition", nil, func(a *AdminStore, ids []int64) error { return skip(a, "2027", ids[0]) }, ErrNoBlitzEdition},
		{"skipping for an entrant of another edition", nil, func(a *AdminStore, ids []int64) error { return skip(a, "2025", ids[0]) }, ErrNoBlitzEntrant},
		{"skipping for a dropped entrant", func(t *testing.T, a *AdminStore, ids []int64) { markDropped(t, a, ids[0]) },
			func(a *AdminStore, ids []int64) error { return skip(a, "2026", ids[0]) }, ErrBlitzEntrantDropped},
		{"skipping when the next round is the knockout", func(t *testing.T, a *AdminStore, ids []int64) {
			if err := a.SetBlitzFormat(context.Background(), "us_open", "2026", BlitzFormat{SwissRounds: 1, KnockoutSize: 2}); err != nil {
				t.Fatalf("SetBlitzFormat: %v", err)
			}
			roundOnePublished(t, a, ids)
		}, func(a *AdminStore, ids []int64) error { return skip(a, "2026", ids[0]) }, ErrBlitzSkipNotSwiss},
		{"skipping a draft round with a result already", func(t *testing.T, a *AdminStore, ids []int64) {
			r, err := a.AddBlitzRound(context.Background(), "us_open", "2026", BlitzRound{Method: "dutch", Seed: 1, Matches: []BlitzMatch{{A: ids[0], B: &ids[1]}, {A: ids[2], B: &ids[3]}}})
			if err != nil {
				t.Fatalf("AddBlitzRound: %v", err)
			}
			if err := a.RecordBlitzRuling(context.Background(), "us_open", "2026", r.Matches[0].ID, "a", "Bob did not show"); err != nil {
				t.Fatalf("RecordBlitzRuling: %v", err)
			}
		}, func(a *AdminStore, ids []int64) error { return skip(a, "2026", ids[1]) }, ErrBlitzMatchDecided},
		{"undoing no skip", nil, func(a *AdminStore, ids []int64) error {
			return a.UnskipBlitzRound(context.Background(), "us_open", "2026", ids[0])
		}, ErrBlitzNotSkipping},
		{"undoing a skip of an unknown entrant", nil, func(a *AdminStore, ids []int64) error {
			return a.UnskipBlitzRound(context.Background(), "us_open", "2026", ids[0]+100)
		}, ErrNoBlitzEntrant},
		{"dropping an entrant skipping the next round", func(t *testing.T, a *AdminStore, ids []int64) {
			roundOnePublished(t, a, ids)
			if err := a.RecordBlitzRuling(context.Background(), "us_open", "2026", matchWith(t, a, ids[0]), "a", "a forfeit"); err != nil {
				t.Fatalf("RecordBlitzRuling: %v", err)
			}
			if err := skip(a, "2026", ids[0]); err != nil {
				t.Fatalf("SkipBlitzRound: %v", err)
			}
		}, func(a *AdminStore, ids []int64) error {
			dropped := "dropped"
			return a.SetBlitzEntrant(context.Background(), "us_open", "2026", ids[0], BlitzEntrantChange{Status: &dropped})
		}, ErrBlitzEntrantSkipping},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _, ids := blitzWithEntrants(t)
			if tt.setup != nil {
				tt.setup(t, a, ids)
			}
			if err := tt.do(a, ids); !errors.Is(err, tt.want) {
				t.Errorf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestTheKnockoutClearsSkipsOfRoundsNoLongerSwiss(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	if err := a.SetBlitzFormat(ctx, "us_open", "2026", BlitzFormat{SwissRounds: 2, KnockoutSize: 2}); err != nil {
		t.Fatalf("SetBlitzFormat: %v", err)
	}
	roundOnePublished(t, a, ids)
	ann, dee := ids[0], ids[3]
	if err := a.RecordBlitzRuling(ctx, "us_open", "2026", matchWith(t, a, ann), "a", "a forfeit"); err != nil {
		t.Fatalf("RecordBlitzRuling: %v", err)
	}
	if err := a.RecordBlitzRuling(ctx, "us_open", "2026", matchWith(t, a, dee), "b", "a forfeit"); err != nil {
		t.Fatalf("RecordBlitzRuling: %v", err)
	}
	if _, err := a.SkipBlitzRound(ctx, "us_open", "2026", dee); err != nil {
		t.Fatalf("SkipBlitzRound(Dee) for round 2: %v", err)
	}
	if err := a.SetBlitzFormat(ctx, "us_open", "2026", BlitzFormat{SwissRounds: 1, KnockoutSize: 2}); err != nil {
		t.Fatalf("SetBlitzFormat(one Swiss round): %v", err)
	}
	final := BlitzMatch{A: ann, B: &dee, Slot: new(0), SeedA: new(1), SeedB: new(2)}
	if _, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Kind: "knockout", Seed: 1, Matches: []BlitzMatch{final}}); err != nil {
		t.Fatalf("AddBlitzRound(knockout) with Dee, whose skip was for round 2: %v", err)
	}
	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	if got := skipsOf(ed); len(got) != 0 {
		t.Errorf("skips once round 2 is the knockout = %v, want none", got)
	}
}

// roundOnePublished publishes round 1 of blitzWithEntrants' four: Ann v Bob, Cy v Dee.
func roundOnePublished(t *testing.T, a *AdminStore, ids []int64) {
	t.Helper()
	r, err := a.AddBlitzRound(context.Background(), "us_open", "2026", BlitzRound{Method: "dutch", Seed: 1, Matches: []BlitzMatch{{A: ids[0], B: &ids[1]}, {A: ids[2], B: &ids[3]}}})
	if err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	if err := a.PublishBlitzRound(context.Background(), "us_open", "2026", r.ID); err != nil {
		t.Fatalf("PublishBlitzRound: %v", err)
	}
}

// matchWith is the id of the latest match an entrant is in.
func matchWith(t *testing.T, a *AdminStore, id int64) int64 {
	t.Helper()
	ed, err := a.BlitzEdition(context.Background(), "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	var found int64
	for _, r := range ed.Rounds {
		for _, m := range r.Matches {
			if m.A == id || (m.B != nil && *m.B == id) {
				found = m.ID
			}
		}
	}
	if found == 0 {
		t.Fatalf("entrant %d is in no match", id)
	}
	return found
}
