//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBadgesAreMadeGivenTakenAndRemoved(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "Ann", "Bob")
	a := NewAdminStore(asAdminLogin(t, pool))

	org, err := a.AddBadge(ctx, Badge{Label: " Organiser ", Description: "Runs a league"})
	if err != nil {
		t.Fatalf("AddBadge: %v", err)
	}
	td, err := a.AddBadge(ctx, Badge{Label: "TD", Description: ""})
	if err != nil {
		t.Fatalf("AddBadge(TD): %v", err)
	}
	for _, give := range []struct {
		badge int64
		name  string
	}{{org, "Ann"}, {org, "Bob"}, {td, "Ann"}} {
		if err := a.GiveBadge(ctx, give.badge, give.name); err != nil {
			t.Fatalf("GiveBadge(%d, %s): %v", give.badge, give.name, err)
		}
	}
	if err := a.TakeBadge(ctx, org, "Bob"); err != nil {
		t.Fatalf("TakeBadge: %v", err)
	}
	if err := a.SetBadge(ctx, Badge{ID: td, Label: "Director", Description: "Tournament director"}); err != nil {
		t.Fatalf("SetBadge: %v", err)
	}

	got, err := a.Badges(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []Badge{
		{ID: td, Label: "Director", Description: "Tournament director", Players: []string{"Ann"}},
		{ID: org, Label: "Organiser", Description: "Runs a league", Players: []string{"Ann"}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("badges (-want +got):\n%s", diff)
	}

	if err := a.RemoveBadge(ctx, org); err != nil {
		t.Fatalf("RemoveBadge: %v", err)
	}
	if got, _ = a.Badges(ctx); len(got) != 1 || got[0].ID != td {
		t.Errorf("badges after removing Organiser = %+v", got)
	}
}

func TestBadgeChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "Ann")
	a := NewAdminStore(asAdminLogin(t, pool))
	id, err := a.AddBadge(ctx, Badge{Label: "Organiser"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.GiveBadge(ctx, id, "Ann"); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		do   func() error
		want error
	}{
		{"an empty label", func() error { _, err := a.AddBadge(ctx, Badge{Label: " "}); return err }, ErrBadBadge},
		{"a long label", func() error { _, err := a.AddBadge(ctx, Badge{Label: "Seventeen letters"}); return err }, ErrBadBadge},
		{"a label taken", func() error { _, err := a.AddBadge(ctx, Badge{Label: "Organiser"}); return err }, ErrBadgeExists},
		{"renaming a badge that is gone", func() error { return a.SetBadge(ctx, Badge{ID: 9999, Label: "X"}) }, ErrNoBadge},
		{"giving a badge that is gone", func() error { return a.GiveBadge(ctx, 9999, "Ann") }, ErrNoBadge},
		{"giving to nobody", func() error { return a.GiveBadge(ctx, id, "Nobody") }, ErrNoPlayer},
		{"taking one not given", func() error { return a.TakeBadge(ctx, id, "Bob") }, ErrNoBadge},
		{"removing a badge that is gone", func() error { return a.RemoveBadge(ctx, 9999) }, ErrNoBadge},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.do(); !errors.Is(err, tt.want) {
				t.Errorf("got %v, want %v", err, tt.want)
			}
		})
	}
	if err := a.GiveBadge(ctx, id, "Ann"); err != nil {
		t.Errorf("giving a badge twice = %v, want no error", err)
	}
}
