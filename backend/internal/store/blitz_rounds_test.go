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

// blitzWithEntrants is blitzFixture with Ann, Bob, Cy and Dee entered in us_open 2026, returning their ids.
func blitzWithEntrants(t *testing.T) (*AdminStore, *RatingsReader, []int64) {
	t.Helper()
	pool, a := blitzFixture(t)
	ids, err := a.AddBlitzEntrants(context.Background(), "us_open", "2026", []BlitzEntrant{
		{Name: "Ann", Zone: "UTC"}, {Name: "Bob", Zone: "UTC"}, {Name: "Cy", Zone: "UTC"}, {Name: "Dee", Zone: "UTC"},
	})
	if err != nil {
		t.Fatalf("AddBlitzEntrants: %v", err)
	}
	shown := true
	if err := a.SetTournament(context.Background(), "us_open", TournamentSettings{Visible: &shown}); err != nil {
		t.Fatalf("SetTournament(visible): %v", err)
	}
	return a, NewRatingsReader(asReader(t, pool)), ids
}

// markDropped marks an entrant dropped directly, for a test of what a dropped entrant does rather than of dropping.
func markDropped(t *testing.T, a *AdminStore, id int64) {
	t.Helper()
	if _, err := a.pool.Exec(context.Background(), "UPDATE blitz_entries SET status = 'dropped' WHERE entry_id = $1", id); err != nil {
		t.Fatalf("marking entrant %d dropped: %v", id, err)
	}
}

func TestOnlyAnEntrantWhoHasPlayedIsDropped(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	more, err := a.AddBlitzEntrants(ctx, "us_open", "2026", []BlitzEntrant{{Name: "Eve", Zone: "UTC"}, {Name: "Fay", Zone: "UTC", Status: "pending"}})
	if err != nil {
		t.Fatalf("AddBlitzEntrants: %v", err)
	}
	ann, bob, cy, dee, eve, fay := ids[0], ids[1], ids[2], ids[3], more[0], more[1]
	r1, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Seed: 1, Matches: []BlitzMatch{{A: ann, B: &bob}, {A: cy, B: &dee}, {A: eve}}})
	if err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	drop := func(id int64) error {
		dropped := "dropped"
		return a.SetBlitzEntrant(ctx, "us_open", "2026", id, BlitzEntrantChange{Status: &dropped})
	}
	if err := drop(eve); !errors.Is(err, ErrBlitzEntrantNotPlayed) {
		t.Errorf("dropping Eve, a bye in a draft = %v, want ErrBlitzEntrantNotPlayed", err)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", r1.ID); err != nil {
		t.Fatalf("PublishBlitzRound: %v", err)
	}
	if err := a.RecordBlitzRuling(ctx, "us_open", "2026", r1.Matches[0].ID, "a", "Bob did not show"); err != nil {
		t.Fatalf("RecordBlitzRuling: %v", err)
	}
	tests := []struct {
		name string
		id   int64
		want error
	}{
		{"Bob, whose match has a result", bob, nil},
		{"Eve, with a bye in a published round", eve, nil},
		{"Cy, paired but not yet played", cy, ErrBlitzEntrantNotPlayed},
		{"Fay, not confirmed and in no round", fay, ErrBlitzEntrantNotPlayed},
	}
	for _, tt := range tests {
		if err := drop(tt.id); !errors.Is(err, tt.want) {
			t.Errorf("dropping %s = %v, want %v", tt.name, err, tt.want)
		}
	}
	active := "active"
	if err := a.SetBlitzEntrant(ctx, "us_open", "2026", bob, BlitzEntrantChange{Status: &active}); err != nil {
		t.Errorf("reinstating Bob: %v", err)
	}
}

func TestAnEntrantWhoHasNotPlayedIsRemovedFromTheirRound(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	more, err := a.AddBlitzEntrants(ctx, "us_open", "2026", []BlitzEntrant{{Name: "Eve", Zone: "UTC"}})
	if err != nil {
		t.Fatalf("AddBlitzEntrants: %v", err)
	}
	ann, bob, cy, dee, eve := ids[0], ids[1], ids[2], ids[3], more[0]
	if _, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Seed: 1, Matches: []BlitzMatch{{A: ann, B: &bob}, {A: cy, B: &dee}, {A: eve}}}); err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	for _, id := range []int64{bob, eve} {
		if err := a.RemoveBlitzEntrant(ctx, "us_open", "2026", id); err != nil {
			t.Fatalf("removing entrant %d, paired in a draft: %v", id, err)
		}
	}
	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	want := []string{fmt.Sprintf("round 1 swiss published=false: %d-%d", cy, dee)}
	if diff := cmp.Diff(want, roundLines(ed.Rounds)); diff != "" {
		t.Errorf("round 1 once Bob and Eve are removed, Ann left unpaired (-want +got):\n%s", diff)
	}
	if len(ed.Entrants) != 3 {
		t.Errorf("entrants = %d, want Ann, Cy and Dee", len(ed.Entrants))
	}
}

func roundLines(rounds []BlitzRound) []string {
	var out []string
	for _, r := range rounds {
		line := fmt.Sprintf("round %d %s published=%v:", r.Number, r.Kind, r.PublishedAt != nil)
		for _, m := range r.Matches {
			if m.B == nil {
				line += fmt.Sprintf(" bye:%d", m.A)
			} else {
				line += fmt.Sprintf(" %d-%d", m.A, *m.B)
			}
		}
		out = append(out, line)
	}
	return out
}

func TestBlitzRoundsAreMadePublishedAndListed(t *testing.T) {
	a, reader, ids := blitzWithEntrants(t)
	ctx := context.Background()
	ann, bob, cy, dee := ids[0], ids[1], ids[2], ids[3]
	r1, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Timezone: true, Seed: 7, Matches: []BlitzMatch{{A: ann, B: &bob}, {A: cy, B: &dee}}})
	if err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	if r1.ID == 0 || r1.Number != 1 || r1.Kind != "swiss" || len(r1.Matches) != 2 || r1.Matches[0].ID == 0 {
		t.Errorf("round made = %+v, want round 1, swiss, two matches with ids", r1)
	}
	if _, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Seed: 8, Matches: []BlitzMatch{{A: ann, B: &cy}}}); !errors.Is(err, ErrBlitzDraftExists) {
		t.Errorf("a second round while the first is a draft = %v, want ErrBlitzDraftExists", err)
	}

	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	if want := []string{fmt.Sprintf("round 1 swiss published=false: %d-%d %d-%d", ann, bob, cy, dee)}; !cmp.Equal(want, roundLines(ed.Rounds)) {
		t.Errorf("admin's rounds = %v, want %v", roundLines(ed.Rounds), want)
	}
	public, err := reader.BlitzEdition(ctx, "us_open", "2026")
	if err != nil || len(public.Rounds) != 0 || public.Round != nil {
		t.Errorf("the site's rounds while a draft = %v, round reached %v, %v; want none", roundLines(public.Rounds), public.Round, err)
	}

	if err := a.PublishBlitzRound(ctx, "us_open", "2026", r1.ID); err != nil {
		t.Fatalf("PublishBlitzRound: %v", err)
	}
	if public, err = reader.BlitzEdition(ctx, "us_open", "2026"); err != nil || len(public.Rounds) != 1 || public.Rounds[0].PublishedAt == nil || public.Round == nil || *public.Round != 1 {
		t.Errorf("the site's rounds once published = %v, round reached %v, %v; want round 1", roundLines(public.Rounds), public.Round, err)
	}
	if err := a.DeleteBlitzRound(ctx, "us_open", "2026", r1.ID); err != nil {
		t.Errorf("deleting the published round 1, the latest = %v, want nil (nothing played yet)", err)
	}
	if r1, err = a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Timezone: true, Seed: 7, Matches: []BlitzMatch{{A: ann, B: &bob}, {A: cy, B: &dee}}}); err != nil || r1.Number != 1 {
		t.Fatalf("round 1 again = %+v, %v; want round 1", r1, err)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", r1.ID); err != nil {
		t.Fatalf("PublishBlitzRound again: %v", err)
	}
	for _, m := range r1.Matches {
		if err := a.RecordBlitzRuling(ctx, "us_open", "2026", m.ID, "draw", "agreed"); err != nil {
			t.Fatalf("RecordBlitzRuling(%d): %v", m.ID, err)
		}
	}
	r2, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Seed: 9, Matches: []BlitzMatch{{A: ann, B: &cy}, {A: bob, B: &dee}}})
	if err != nil || r2.Number != 2 {
		t.Errorf("the next round = %+v, %v; want round 2", r2, err)
	}
	if err := a.DeleteBlitzRound(ctx, "us_open", "2026", r1.ID); !errors.Is(err, ErrBlitzRoundNotLast) {
		t.Errorf("deleting round 1 behind round 2 = %v, want ErrBlitzRoundNotLast", err)
	}
	if err := a.DeleteBlitzRound(ctx, "us_open", "2026", r2.ID); err != nil {
		t.Errorf("deleting the draft = %v, want nil", err)
	}
	if ed, err = a.BlitzEdition(ctx, "us_open", "2026"); err != nil || len(ed.Rounds) != 1 {
		t.Errorf("rounds after deleting the draft = %v, %v; want round 1 alone", roundLines(ed.Rounds), err)
	}
	if err := a.RemoveBlitzEntrant(ctx, "us_open", "2026", ann); !errors.Is(err, ErrBlitzEntrantPlayed) {
		t.Errorf("removing an entrant who played a round = %v, want ErrBlitzEntrantPlayed", err)
	}
}

func TestBlitzRoundMatchesAreCheckedAndChanged(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	ann, bob, cy, dee := ids[0], ids[1], ids[2], ids[3]
	markDropped(t, a, dee)
	r1, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Seed: 1, Matches: []BlitzMatch{{A: ann, B: &bob}, {A: cy}}})
	if err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", r1.ID); err != nil {
		t.Fatalf("PublishBlitzRound: %v", err)
	}
	if err := a.RecordBlitzRuling(ctx, "us_open", "2026", r1.Matches[0].ID, "a", "Bob did not show"); err != nil {
		t.Fatalf("RecordBlitzRuling: %v", err)
	}
	tests := []struct {
		name    string
		matches []BlitzMatch
		want    error
		named   string
	}{
		{"a dropped entrant", []BlitzMatch{{A: ann, B: &dee}, {A: bob, B: &cy}}, ErrBadBlitzMatch, "Dee"},
		{"an entrant twice", []BlitzMatch{{A: cy, B: &bob}, {A: cy}}, ErrBadBlitzMatch, "Cy"},
		{"an unknown entrant", []BlitzMatch{{A: ann, B: new(int64)}}, ErrBadBlitzMatch, ""},
		{"a repeat of round 1", []BlitzMatch{{A: bob, B: &ann}, {A: cy}}, ErrBlitzRepeat, "Ann"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Seed: 2, Matches: tt.matches})
			if !errors.Is(err, tt.want) || !strings.Contains(err.Error(), tt.named) {
				t.Errorf("AddBlitzRound = %v, want %v naming %q", err, tt.want, tt.named)
			}
		})
	}

	r2, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Seed: 3, Matches: []BlitzMatch{{A: ann, B: &cy}}})
	if err != nil {
		t.Fatalf("AddBlitzRound(round 2, incomplete): %v", err)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", r2.ID); !errors.Is(err, ErrBlitzRoundIncomplete) || !strings.Contains(err.Error(), "Bob") {
		t.Errorf("publishing with Bob unpaired = %v, want ErrBlitzRoundIncomplete naming Bob", err)
	}
	if err := a.SetBlitzRoundMatches(ctx, "us_open", "2026", r2.ID, []BlitzMatch{{A: ann, B: &cy}, {A: bob}}); err != nil {
		t.Fatalf("SetBlitzRoundMatches: %v", err)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", r2.ID); err != nil {
		t.Errorf("publishing the completed round = %v, want nil", err)
	}
	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	want := []string{
		fmt.Sprintf("round 1 swiss published=true: %d-%d bye:%d", ann, bob, cy),
		fmt.Sprintf("round 2 swiss published=true: %d-%d bye:%d", ann, cy, bob),
	}
	if diff := cmp.Diff(want, roundLines(ed.Rounds)); diff != "" {
		t.Errorf("rounds (-want +got):\n%s", diff)
	}
	if err := a.SetBlitzRoundMatches(ctx, "us_open", "2026", r2.ID+100, nil); !errors.Is(err, ErrNoBlitzRound) {
		t.Errorf("changing an unknown round = %v, want ErrNoBlitzRound", err)
	}
}
