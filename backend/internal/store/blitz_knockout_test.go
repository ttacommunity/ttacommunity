//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestABlitzKnockoutIsPlayedToAChampion(t *testing.T) {
	a, reader, ids := blitzWithEntrants(t)
	ctx := context.Background()
	ann, bob, cy, dee := ids[0], ids[1], ids[2], ids[3]
	if err := a.SetBlitzFormat(ctx, "us_open", "2026", BlitzFormat{SwissRounds: 1, KnockoutSize: 4}); err != nil {
		t.Fatalf("SetBlitzFormat: %v", err)
	}
	swiss, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Seed: 1, Matches: []BlitzMatch{{A: ann, B: &bob}, {A: cy, B: &dee}}})
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

	// The semi-finals repeat both Swiss pairings: a repeat is allowed in the knockout.
	ko := func(a, b int64, slot, seedA, seedB int) BlitzMatch {
		return BlitzMatch{A: a, B: &b, Slot: new(slot), SeedA: new(seedA), SeedB: new(seedB)}
	}
	if _, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Kind: "knockout", Seed: 2, Matches: []BlitzMatch{{A: ann, B: &bob, Slot: new(0)}}}); !errors.Is(err, ErrBadBlitzMatch) {
		t.Errorf("a knockout match without seeds = %v, want ErrBadBlitzMatch", err)
	}
	semi, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Kind: "knockout", Seed: 2, Matches: []BlitzMatch{ko(ann, bob, 0, 1, 4), ko(cy, dee, 1, 2, 3)}})
	if err != nil {
		t.Fatalf("AddBlitzRound(semi-finals): %v", err)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", semi.ID); err != nil {
		t.Fatalf("PublishBlitzRound(semi-finals): %v", err)
	}
	if err := a.SetBlitzRoundMatches(ctx, "us_open", "2026", semi.ID, []BlitzMatch{{A: ann, B: &cy}, {A: bob, B: &dee}}); !errors.Is(err, ErrBlitzKnockoutFixed) {
		t.Errorf("changing a knockout round by hand = %v, want ErrBlitzKnockoutFixed", err)
	}
	if err := a.SetBlitzFormat(ctx, "us_open", "2026", BlitzFormat{SwissRounds: 2, KnockoutSize: 4}); !errors.Is(err, ErrBlitzFormatLocked) {
		t.Errorf("changing the format once the knockout started = %v, want ErrBlitzFormatLocked", err)
	}
	for _, r := range []string{"draw", "double_loss"} {
		if err := a.RecordBlitzRuling(ctx, "us_open", "2026", semi.Matches[0].ID, r, "ruled"); !errors.Is(err, ErrBlitzKnockoutWinner) {
			t.Errorf("a knockout ruling of %s = %v, want ErrBlitzKnockoutWinner", r, err)
		}
	}
	if err := a.RecordBlitzRuling(ctx, "us_open", "2026", semi.Matches[0].ID, "b", "Ann did not show"); err != nil {
		t.Fatalf("RecordBlitzRuling(an upset): %v", err)
	}
	drawn := func(code string) BlitzGame {
		return BlitzGame{ShareCode: code, A: BlitzSide{Culture: 120, Outcome: "finished"}, B: BlitzSide{Culture: 120, Outcome: "finished"},
			Finished: time.Date(2026, 12, 9, 20, 0, 0, 0, time.UTC)}
	}
	if err := a.RecordBlitzGame(ctx, "us_open", "2026", semi.Matches[1].ID, drawn("KO-DRAW-1")); err != nil {
		t.Fatalf("RecordBlitzGame(a drawn semi-final): %v", err)
	}

	// The final names two of the four active entrants: a knockout round is published without the others. Bob,
	// seeded 4th, came through slot 0, so the higher seed sits on side b.
	final, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Kind: "knockout", Seed: 3, Matches: []BlitzMatch{ko(bob, cy, 0, 4, 2)}})
	if err != nil {
		t.Fatalf("AddBlitzRound(final), the drawn semi-final already decided: %v", err)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", final.ID); err != nil {
		t.Fatalf("PublishBlitzRound(final): %v", err)
	}
	if ed, err := reader.BlitzEdition(ctx, "us_open", "2026"); err != nil || ed.Champion != nil {
		t.Errorf("the champion before the final's result = %v, %v; want none", ed.Champion, err)
	}
	if err := a.RecordBlitzGame(ctx, "us_open", "2026", final.Matches[0].ID, drawn("KO-DRAW-2")); err != nil {
		t.Fatalf("RecordBlitzGame(a drawn final): %v", err)
	}

	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	for _, tt := range []struct {
		name  string
		match BlitzMatch
		want  string
	}{
		{"the drawn semi-final, seeds 2 v 3", ed.Rounds[1].Matches[1], "a"},
		{"the drawn final, seeds 4 v 2", ed.Rounds[2].Matches[0], "b"},
	} {
		if tt.match.Result != tt.want || tt.match.Game == nil || tt.match.Ruling != "" {
			t.Errorf("%s: result %q, game %v, ruling %q; want %q from the game: the higher seed goes through", tt.name, tt.match.Result, tt.match.Game, tt.match.Ruling, tt.want)
		}
	}
	if ed.Champion == nil || *ed.Champion != "Cy" {
		t.Errorf("admin's champion = %v, want Cy", ed.Champion)
	}
	if got := ed.Rounds[1].Matches; len(got) != 2 || got[0].Slot == nil || *got[0].Slot != 0 || got[1].Slot == nil || *got[1].Slot != 1 ||
		got[1].SeedA == nil || *got[1].SeedA != 2 || got[1].SeedB == nil || *got[1].SeedB != 3 {
		t.Errorf("the semi-finals = %+v, want slots 0 and 1, the second seeded 2 v 3", got)
	}
	if m := ed.Rounds[0].Matches[0]; m.Slot != nil || m.SeedA != nil {
		t.Errorf("a Swiss match = %+v, want no slot and no seeds", m)
	}
	list, err := reader.BlitzEditions(ctx)
	if err != nil {
		t.Fatalf("BlitzEditions: %v", err)
	}
	for _, r := range list {
		if r.Edition == "2026" && (r.Champion == nil || *r.Champion != "Cy") {
			t.Errorf("the site's list names champion %v for 2026, want Cy", r.Champion)
		}
		if r.Edition != "2026" && r.Champion != nil {
			t.Errorf("the site's list names champion %s for %s, want none", *r.Champion, r.Edition)
		}
	}
	if _, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Kind: "knockout", Seed: 4, Matches: []BlitzMatch{ko(ann, bob, 0, 1, 4)}}); !errors.Is(err, ErrBlitzFinished) {
		t.Errorf("a round after the final = %v, want ErrBlitzFinished", err)
	}
}

func TestADroppedKnockoutPlayerIsReplaced(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	more, err := a.AddBlitzEntrants(ctx, "us_open", "2026", []BlitzEntrant{{Name: "Eve", Zone: "UTC"}})
	if err != nil {
		t.Fatalf("AddBlitzEntrants: %v", err)
	}
	ann, bob, cy, dee, eve := ids[0], ids[1], ids[2], ids[3], more[0]
	if err := a.SetBlitzFormat(ctx, "us_open", "2026", BlitzFormat{SwissRounds: 1, KnockoutSize: 2}); err != nil {
		t.Fatalf("SetBlitzFormat: %v", err)
	}
	swiss, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Seed: 1, Matches: []BlitzMatch{{A: ann, B: &bob}, {A: cy, B: &dee}, {A: eve}}})
	if err != nil {
		t.Fatalf("AddBlitzRound(swiss): %v", err)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", swiss.ID); err != nil {
		t.Fatalf("PublishBlitzRound(swiss): %v", err)
	}
	for _, m := range swiss.Matches[:2] {
		if err := a.RecordBlitzRuling(ctx, "us_open", "2026", m.ID, "a", "ruled"); err != nil {
			t.Fatalf("RecordBlitzRuling(swiss): %v", err)
		}
	}
	final, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Kind: "knockout", Seed: 2, Matches: []BlitzMatch{{A: ann, B: &cy, Slot: new(0), SeedA: new(1), SeedB: new(2)}}})
	if err != nil {
		t.Fatalf("AddBlitzRound(final): %v", err)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", final.ID); err != nil {
		t.Fatalf("PublishBlitzRound(final): %v", err)
	}
	markDropped(t, a, cy)
	markDropped(t, a, dee)
	replace := func(roundID, matchID, out, in int64) error {
		return a.ReplaceBlitzPlayer(ctx, "us_open", "2026", roundID, matchID, out, in)
	}
	match := final.Matches[0].ID
	tests := []struct {
		name                  string
		round, match, out, in int64
		want                  error
	}{
		{"in a Swiss round", swiss.ID, swiss.Matches[0].ID, bob, eve, ErrBlitzNotKnockout},
		{"in a match of another round", final.ID, swiss.Matches[0].ID, cy, eve, ErrNoBlitzMatch},
		{"someone not in the match", final.ID, match, bob, eve, ErrBadBlitzMatch},
		{"someone who has not dropped", final.ID, match, ann, eve, ErrBlitzNotDropped},
		{"with someone in the round", final.ID, match, cy, ann, ErrBadBlitzMatch},
		{"with someone who dropped", final.ID, match, cy, dee, ErrBadBlitzMatch},
	}
	for _, tt := range tests {
		if err := replace(tt.round, tt.match, tt.out, tt.in); !errors.Is(err, tt.want) {
			t.Errorf("replacing %s = %v, want %v", tt.name, err, tt.want)
		}
	}
	if err := replace(final.ID, match, cy, eve); err != nil {
		t.Fatalf("replacing Cy, who dropped, with Eve: %v", err)
	}
	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	if m := matchOf(t, ed, match); m.A != ann || m.B == nil || *m.B != eve || *m.Slot != 0 || *m.SeedA != 1 || *m.SeedB != 2 {
		t.Errorf("the final = %d v %v (slot %d, seeds %d v %d), want Ann v Eve in Cy's slot and seed", m.A, m.B, *m.Slot, *m.SeedA, *m.SeedB)
	}
	gus, err := a.AddBlitzEntrants(ctx, "us_open", "2026", []BlitzEntrant{{Name: "Gus", Zone: "UTC"}})
	if err != nil {
		t.Fatalf("AddBlitzEntrants(Gus): %v", err)
	}
	markDropped(t, a, eve)
	if err := replace(final.ID, match, eve, gus[0]); err != nil {
		t.Fatalf("replacing Eve with Gus, who has not played: %v", err)
	}
	if err := a.RemoveBlitzEntrant(ctx, "us_open", "2026", gus[0]); !errors.Is(err, ErrBlitzEntrantInKnockout) {
		t.Errorf("removing Gus from the final = %v, want ErrBlitzEntrantInKnockout", err)
	}
	if err := a.RecordBlitzRuling(ctx, "us_open", "2026", match, "a", "ruled"); err != nil {
		t.Fatalf("RecordBlitzRuling(final): %v", err)
	}
	markDropped(t, a, ann)
	if err := replace(final.ID, match, ann, bob); !errors.Is(err, ErrBlitzMatchDecided) {
		t.Errorf("replacing in a match with a result = %v, want ErrBlitzMatchDecided", err)
	}
}
