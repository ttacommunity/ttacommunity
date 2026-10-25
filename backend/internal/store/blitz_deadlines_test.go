//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestARoundKeepsTheDeadlineLastSetAndItsZone(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	r, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Method: "dutch", Seed: 1, Matches: []BlitzMatch{{A: ids[0], B: &ids[1]}, {A: ids[2], B: &ids[3]}}})
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
	r, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Method: "dutch", Seed: 1, Matches: []BlitzMatch{{A: ids[0], B: &ids[1]}, {A: ids[2], B: &ids[3]}}})
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
	r, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Method: "dutch", Seed: 1, Matches: []BlitzMatch{{A: ids[0], B: &ids[1]}}})
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
