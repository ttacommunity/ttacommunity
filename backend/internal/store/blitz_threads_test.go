//go:build integration

package store

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pressly/goose/v3"
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

func TestMigration73sDownTakesTheThreadTablesAwayAndUpBringsThemBack(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	present := func() []string {
		return rowsOf(t, pool, `SELECT relname FROM pg_class WHERE relkind = 'r' AND relname IN ('blitz_organisers', 'blitz_thread_texts') ORDER BY 1`)
	}
	want := []string{"blitz_organisers", "blitz_thread_texts"}
	if got := present(); !slices.Equal(got, want) {
		t.Fatalf("migrated: %q, want %q", got, want)
	}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 72); return err }); err != nil {
		t.Fatalf("migrating down to 72: %v", err)
	}
	if got := present(); len(got) != 0 {
		t.Errorf("at 72, %q remain, want none", got)
	}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.Up(ctx); return err }); err != nil {
		t.Fatalf("migrating up again: %v", err)
	}
	if got := present(); !slices.Equal(got, want) {
		t.Errorf("up again: %q, want %q", got, want)
	}
}
