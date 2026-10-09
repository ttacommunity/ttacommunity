//go:build integration

package store

import (
	"context"
	"errors"
	"slices"
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
	for _, b := range []Badge{
		{ID: org, Label: "Organiser", Description: "Runs a league", Players: []string{"Ann", "Bob"}},
		{ID: td, Label: "TD", Players: []string{"Ann"}},
		{ID: org, Label: "Organiser", Description: "Runs a league", Players: []string{"Ann"}},
	} {
		if err := a.SetBadge(ctx, b); err != nil {
			t.Fatalf("SetBadge(%+v): %v", b, err)
		}
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
	if err := a.SetBadge(ctx, Badge{ID: id, Label: "Organiser", Players: []string{"Ann"}}); err != nil {
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
		{"giving a badge that is gone", func() error { return a.SetBadge(ctx, Badge{ID: 9999, Label: "X", Players: []string{"Ann"}}) }, ErrNoBadge},
		{"giving to nobody", func() error {
			return a.SetBadge(ctx, Badge{ID: id, Label: "Organiser", Players: []string{"Ann", "Nobody"}})
		}, ErrNoPlayer},
		{"removing a badge that is gone", func() error { return a.RemoveBadge(ctx, 9999) }, ErrNoBadge},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.do(); !errors.Is(err, tt.want) {
				t.Errorf("got %v, want %v", err, tt.want)
			}
		})
	}
	if err := a.SetBadge(ctx, Badge{ID: id, Label: "Organiser", Players: []string{"Ann", "Ann"}}); err != nil {
		t.Errorf("naming a player twice = %v, want no error", err)
	}
	if got, _ := a.Badges(ctx); len(got) != 1 || len(got[0].Players) != 1 {
		t.Errorf("after a refused change the badge's players = %+v, want only Ann", got)
	}
}

func TestABadgeChangeRefusedForOnePartChangesNoPart(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "Ann", "Bob")
	a := NewAdminStore(asAdminLogin(t, pool))
	if _, err := a.AddBadge(ctx, Badge{Label: "Organiser"}); err != nil {
		t.Fatal(err)
	}
	td, err := a.AddBadge(ctx, Badge{Label: "TD"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetBadge(ctx, Badge{ID: td, Label: "TD", Players: []string{"Ann"}}); err != nil {
		t.Fatalf("SetBadge with players: %v", err)
	}
	for _, tt := range []struct {
		name string
		b    Badge
		want error
	}{
		{"a label taken", Badge{ID: td, Label: "Organiser", Players: []string{"Bob"}}, ErrBadgeExists},
		{"a player who is not one", Badge{ID: td, Label: "Director", Players: []string{"Nobody"}}, ErrNoPlayer},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := a.SetBadge(ctx, tt.b); !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
			got, _ := a.Badges(ctx)
			if got[1].ID != td || got[1].Label != "TD" || !slices.Equal(got[1].Players, []string{"Ann"}) {
				t.Errorf("after a refused change the badge = %+v, want TD held by Ann", got[1])
			}
		})
	}
}
