//go:build integration

package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// organisersFixture is blitzFixture with the members Carol (Discord carol_tta) and Dave (no Discord name).
func organisersFixture(t *testing.T) (*AdminStore, int64, int64) {
	t.Helper()
	_, a := blitzFixture(t)
	ctx := context.Background()
	carol, err := a.AddMember(ctx, Member{Name: "Carol", DiscordName: "carol_tta", Zone: "UTC"})
	if err != nil {
		t.Fatalf("AddMember(Carol): %v", err)
	}
	dave, err := a.AddMember(ctx, Member{Name: "Dave", Zone: "UTC"})
	if err != nil {
		t.Fatalf("AddMember(Dave): %v", err)
	}
	return a, carol, dave
}

// organiserLines reads us_open 2026's organisers for admin as "name @discord", in order.
func organiserLines(t *testing.T, a *AdminStore) []string {
	t.Helper()
	ed, err := a.BlitzEdition(context.Background(), "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	var out []string
	for _, o := range ed.Organisers {
		out = append(out, o.Name+" @"+o.DiscordName)
	}
	return out
}

func TestBlitzOrganisersAreAddedOnceInOrderAndRemoved(t *testing.T) {
	a, carol, dave := organisersFixture(t)
	ctx := context.Background()
	for _, id := range []int64{carol, dave, carol} {
		if err := a.AddBlitzOrganiser(ctx, "us_open", "2026", id); err != nil {
			t.Fatalf("AddBlitzOrganiser(%d): %v", id, err)
		}
	}
	if diff := cmp.Diff([]string{"Carol @carol_tta", "Dave @"}, organiserLines(t, a)); diff != "" {
		t.Errorf("organisers after adding Carol, Dave and Carol again (-want +got):\n%s", diff)
	}
	if err := a.RemoveBlitzOrganiser(ctx, "us_open", "2026", carol); err != nil {
		t.Fatalf("RemoveBlitzOrganiser(Carol): %v", err)
	}
	if diff := cmp.Diff([]string{"Dave @"}, organiserLines(t, a)); diff != "" {
		t.Errorf("organisers after removing Carol (-want +got):\n%s", diff)
	}
}

func TestBlitzOrganisersRefuseWhatTheyCannotTake(t *testing.T) {
	a, carol, dave := organisersFixture(t)
	ctx := context.Background()
	if err := a.AddBlitzOrganiser(ctx, "us_open", "2026", carol); err != nil {
		t.Fatalf("AddBlitzOrganiser(Carol): %v", err)
	}
	add := a.AddBlitzOrganiser
	remove := a.RemoveBlitzOrganiser
	tests := []struct {
		name    string
		op      func(context.Context, string, string, int64) error
		edition string
		member  int64
		want    error
	}{
		{"adding an unknown member", add, "2026", dave + 100, ErrNoMember},
		{"adding to an unknown edition", add, "2027", dave, ErrNoBlitzEdition},
		{"removing someone who does not organise it", remove, "2026", dave, ErrNoBlitzOrganiser},
		{"removing from an unknown edition", remove, "2027", carol, ErrNoBlitzEdition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.op(ctx, "us_open", tt.edition, tt.member); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestAnOrganiserIsNotRemovedAsAMember(t *testing.T) {
	a, carol, _ := organisersFixture(t)
	ctx := context.Background()
	if err := a.AddBlitzOrganiser(ctx, "us_open", "2026", carol); err != nil {
		t.Fatalf("AddBlitzOrganiser(Carol): %v", err)
	}
	if err := a.RemoveMember(ctx, carol); !errors.Is(err, ErrMemberInUse) {
		t.Errorf("RemoveMember(an organiser) = %v, want ErrMemberInUse", err)
	}
}

func TestThePublicRoleCannotReadWhoOrganises(t *testing.T) {
	pool, _ := blitzFixture(t)
	if _, err := asReader(t, pool).Exec(context.Background(), "SELECT member_id FROM blitz_organisers"); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("ratings_read reading blitz_organisers = %v, want permission denied", err)
	}
}

func TestARoundKeepsTheDeadlineLastSetAndItsZone(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	r, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Seed: 1, Matches: []BlitzMatch{{A: ids[0], B: &ids[1]}, {A: ids[2], B: &ids[3]}}})
	if err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	first, last := time.Date(2026, 10, 19, 11, 0, 0, 0, time.UTC), time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	if err := a.SetBlitzRoundDeadline(ctx, "us_open", "2026", r.ID, first, "Europe/London"); err != nil {
		t.Fatalf("SetBlitzRoundDeadline on a draft: %v", err)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", r.ID); err != nil {
		t.Fatalf("PublishBlitzRound: %v", err)
	}
	if err := a.SetBlitzRoundDeadline(ctx, "us_open", "2026", r.ID, last, "US East"); err != nil {
		t.Fatalf("SetBlitzRoundDeadline once published: %v", err)
	}
	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	got := ed.Rounds[0]
	if got.Deadline == nil || !got.Deadline.Equal(last) || got.DeadlineZone != "US East" {
		t.Errorf("deadline = %v in %q, want %v in %q", got.Deadline, got.DeadlineZone, last, "US East")
	}
}

func TestAFinishedRoundKeepsItsDeadline(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	r, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Seed: 1, Matches: []BlitzMatch{{A: ids[0], B: &ids[1]}, {A: ids[2], B: &ids[3]}}})
	if err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", r.ID); err != nil {
		t.Fatalf("PublishBlitzRound: %v", err)
	}
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	if err := a.SetBlitzRoundDeadline(ctx, "us_open", "2026", r.ID, at, "UTC"); err != nil {
		t.Fatalf("SetBlitzRoundDeadline: %v", err)
	}
	if err := a.RecordBlitzRuling(ctx, "us_open", "2026", r.Matches[0].ID, "draw", "Neither could find a time"); err != nil {
		t.Fatalf("RecordBlitzRuling: %v", err)
	}
	if err := a.SetBlitzRoundDeadline(ctx, "us_open", "2026", r.ID, at.Add(time.Hour), "UTC"); err != nil {
		t.Errorf("SetBlitzRoundDeadline with a game still to play = %v, want nil", err)
	}
	if err := a.RecordBlitzRuling(ctx, "us_open", "2026", r.Matches[1].ID, "a", "Dee did not turn up"); err != nil {
		t.Fatalf("RecordBlitzRuling: %v", err)
	}
	if err := a.SetBlitzRoundDeadline(ctx, "us_open", "2026", r.ID, at.Add(2*time.Hour), "UTC"); !errors.Is(err, ErrBlitzRoundFinished) {
		t.Errorf("SetBlitzRoundDeadline once every game has a result = %v, want ErrBlitzRoundFinished", err)
	}
}

func TestSetBlitzRoundDeadlineRefusesARoundItCannotFind(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	r, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Seed: 1, Matches: []BlitzMatch{{A: ids[0], B: &ids[1]}}})
	if err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		edition string
		round   int64
		want    error
	}{
		{"an unknown round", "2026", r.ID + 100, ErrNoBlitzRound},
		{"a round of another edition", "2025", r.ID, ErrNoBlitzRound},
		{"an unknown edition", "2027", r.ID, ErrNoBlitzEdition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := a.SetBlitzRoundDeadline(ctx, "us_open", tt.edition, tt.round, at, "UTC"); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

// threadText reads us_open 2026's own thread text for admin, nil while it has none.
func threadText(t *testing.T, a *AdminStore) *BlitzThreadText {
	t.Helper()
	ed, err := a.BlitzEdition(context.Background(), "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	return ed.ThreadText
}

func TestAnEditionKeepsTheThreadTextLastSetUntilReset(t *testing.T) {
	_, a := blitzFixture(t)
	ctx := context.Background()
	if got := threadText(t, a); got != nil {
		t.Fatalf("thread text of a new edition = %+v, want none", got)
	}
	first, last := BlitzThreadText{Message: "Hi {players}.", NoDiscord: "{name}."}, BlitzThreadText{}
	for _, text := range []BlitzThreadText{first, last} {
		if err := a.SetBlitzThreadText(ctx, "us_open", "2026", text); err != nil {
			t.Fatalf("SetBlitzThreadText(%+v): %v", text, err)
		}
	}
	if got := threadText(t, a); got == nil || *got != last {
		t.Errorf("thread text after setting it, then empty = %+v, want %+v", got, last)
	}
	for range 2 {
		if err := a.ResetBlitzThreadText(ctx, "us_open", "2026"); err != nil {
			t.Fatalf("ResetBlitzThreadText: %v", err)
		}
	}
	if got := threadText(t, a); got != nil {
		t.Errorf("thread text after a reset = %+v, want none", got)
	}
}

func TestTheThreadTextOfAnUnknownEditionIsRefused(t *testing.T) {
	_, a := blitzFixture(t)
	ctx := context.Background()
	tests := []struct {
		name string
		op   func() error
	}{
		{"setting it", func() error { return a.SetBlitzThreadText(ctx, "us_open", "2027", BlitzThreadText{Message: "Hi."}) }},
		{"resetting it", func() error { return a.ResetBlitzThreadText(ctx, "us_open", "2027") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.op(); !errors.Is(err, ErrNoBlitzEdition) {
				t.Errorf("err = %v, want ErrNoBlitzEdition", err)
			}
		})
	}
}

func TestTheDatabaseRefusesAThreadTextTooLong(t *testing.T) {
	_, a := blitzFixture(t)
	tests := []struct {
		name, constraint string
		text             BlitzThreadText
	}{
		{"a message over 1,500 characters", "blitz_thread_texts_message_check", BlitzThreadText{Message: strings.Repeat("é", 1501)}},
		{"a no-Discord sentence over 300 characters", "blitz_thread_texts_no_discord_check", BlitzThreadText{NoDiscord: strings.Repeat("é", 301)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := a.SetBlitzThreadText(context.Background(), "us_open", "2026", tt.text); err == nil || !strings.Contains(err.Error(), tt.constraint) {
				t.Errorf("err = %v, want %s to refuse it", err, tt.constraint)
			}
			if got := threadText(t, a); got != nil {
				t.Errorf("thread text after a refused save = %+v, want none", got)
			}
		})
	}
}

func TestThePublicRoleCannotReadTheThreadText(t *testing.T) {
	pool, _ := blitzFixture(t)
	if _, err := asReader(t, pool).Exec(context.Background(), "SELECT message FROM blitz_thread_texts"); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("ratings_read reading blitz_thread_texts = %v, want permission denied", err)
	}
}
