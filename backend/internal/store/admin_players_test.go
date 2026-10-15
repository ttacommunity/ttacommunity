//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSetTitleReplacesAndClearTitleRemoves(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "Ann", "Bob")
	a := NewAdminStore(asAdminLogin(t, pool))

	for _, o := range []TitleOverride{{Name: "Ann", Title: "GM", Reason: "first"}, {Name: "Ann", Title: "SGM", Reason: " won 2019 "}, {Name: "Bob", Title: "", Reason: "a false Super GM"}} {
		if err := a.SetTitle(ctx, o); err != nil {
			t.Fatalf("SetTitle(%+v): %v", o, err)
		}
	}
	got, err := a.TitleOverrides(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []TitleOverride{{Name: "Ann", Title: "SGM", Reason: "won 2019"}, {Name: "Bob", Title: "", Reason: "a false Super GM"}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("overrides (-want +got):\n%s", diff)
	}

	if err := a.ClearTitle(ctx, "Ann"); err != nil {
		t.Fatalf("ClearTitle: %v", err)
	}
	if err := a.ClearTitle(ctx, "Ann"); !errors.Is(err, ErrNoTitle) {
		t.Errorf("ClearTitle twice = %v, want ErrNoTitle", err)
	}
	if got, _ = a.TitleOverrides(ctx); len(got) != 1 || got[0].Name != "Bob" {
		t.Errorf("overrides after clearing Ann = %+v", got)
	}
}

func TestSetTitleRefusesWhatAdminCannotSet(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "Ann")
	a := NewAdminStore(asAdminLogin(t, pool))
	for _, tt := range []struct {
		name string
		o    TitleOverride
		want error
	}{
		{"no such player", TitleOverride{Name: "Nobody", Title: "GM", Reason: "r"}, ErrNoPlayer},
		{"the World Champion's title", TitleOverride{Name: "Ann", Title: "WC", Reason: "r"}, ErrBadTitle},
		{"a title the engine does not give", TitleOverride{Name: "Ann", Title: "Legend", Reason: "r"}, ErrBadTitle},
		{"no reason", TitleOverride{Name: "Ann", Title: "GM", Reason: "  "}, ErrBadTitle},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := a.SetTitle(ctx, tt.o); !errors.Is(err, tt.want) {
				t.Errorf("SetTitle = %v, want %v", err, tt.want)
			}
		})
	}
	if got, _ := a.TitleOverrides(ctx); len(got) != 0 {
		t.Errorf("overrides after only refusals = %+v", got)
	}
}

func TestSetTitleSavesEveryTitleAdminOffers(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	addPlayers(t, pool, "Ann")
	a := NewAdminStore(asAdminLogin(t, pool))
	for _, title := range AdminTitles {
		t.Run("title "+title, func(t *testing.T) {
			if err := a.SetTitle(ctx, TitleOverride{Name: "Ann", Title: title, Reason: "r"}); err != nil {
				t.Fatalf("SetTitle(%q): %v", title, err)
			}
			if got, err := a.TitleOverrides(ctx); err != nil || len(got) != 1 || got[0].Title != title {
				t.Errorf("overrides after SetTitle(%q) = %+v, %v", title, got, err)
			}
		})
	}
}
