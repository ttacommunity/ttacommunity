//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgxpool"
)

// blitzFixture gives a migrated database with the blitz series us_open (editions 2026 and 2025), the series worlds
// (edition 2025, not blitz) and the player Bob, and an admin store on it.
func blitzFixture(t *testing.T) (*pgxpool.Pool, *AdminStore) {
	t.Helper()
	pool := migrated(t)
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO series (slug, name) VALUES ('us_open', 'US Open'), ('worlds', 'World Championship')`,
		`INSERT INTO tournaments (series_id, label) SELECT series_id, '2026' FROM series WHERE slug = 'us_open'`,
		`INSERT INTO tournaments (series_id, label) SELECT series_id, '2025' FROM series WHERE slug = 'us_open'`,
		`INSERT INTO tournaments (series_id, label) SELECT series_id, '2025' FROM series WHERE slug = 'worlds'`,
		`INSERT INTO players (name) VALUES ('Bob')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("filling: %v", err)
		}
	}
	a := NewAdminStore(asAdminLogin(t, pool))
	on := true
	if err := a.SetTournament(ctx, "us_open", TournamentSettings{Blitz: &on}); err != nil {
		t.Fatalf("SetTournament(blitz): %v", err)
	}
	return pool, a
}

func editionLines(rows []BlitzEditionRow) []string {
	var out []string
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%s %s %d/%d in=%d out=%d", r.Slug, r.Edition, r.Format.SwissRounds, r.Format.KnockoutSize, r.Entrants, r.Dropped))
	}
	return out
}

func TestBlitzEditionsListEveryEditionOfABlitzSeries(t *testing.T) {
	pool, a := blitzFixture(t)
	ctx := context.Background()
	got, err := a.BlitzEditions(ctx)
	if err != nil {
		t.Fatalf("BlitzEditions: %v", err)
	}
	want := []string{"us_open 2026 8/8 in=0 out=0", "us_open 2025 8/8 in=0 out=0"}
	if diff := cmp.Diff(want, editionLines(got)); diff != "" {
		t.Errorf("admin's blitz editions (-want +got):\n%s", diff)
	}
	rows, err := a.TournamentRows(ctx)
	if err != nil {
		t.Fatalf("TournamentRows: %v", err)
	}
	for _, r := range rows {
		if r.Blitz != (r.Slug == "us_open") {
			t.Errorf("TournamentRows: %s blitz = %v, want %v", r.Slug, r.Blitz, r.Slug == "us_open")
		}
	}
	if tt := adminTournament(t, a, "us_open"); !tt.Blitz {
		t.Errorf("Tournament(us_open).Blitz = false, want true")
	}

	reader := NewRatingsReader(asReader(t, pool))
	public, err := reader.BlitzEditions(ctx)
	if err != nil || len(public) != 0 {
		t.Errorf("the site's blitz editions while us_open is hidden = %v, %v; want none", editionLines(public), err)
	}
	shown := true
	if err := a.SetTournament(ctx, "us_open", TournamentSettings{Visible: &shown}); err != nil {
		t.Fatalf("SetTournament(visible): %v", err)
	}
	if public, err = reader.BlitzEditions(ctx); err != nil {
		t.Fatalf("the site's BlitzEditions: %v", err)
	}
	if diff := cmp.Diff(want, editionLines(public)); diff != "" {
		t.Errorf("the site's blitz editions once shown (-want +got):\n%s", diff)
	}
}

func TestBlitzEntrantsAreAddedChangedAndRemoved(t *testing.T) {
	pool, a := blitzFixture(t)
	ctx := context.Background()
	ids, err := a.AddBlitzEntrants(ctx, "us_open", "2026", []BlitzEntrant{
		{Name: " Ann ", DiscordName: "ann#1", Zone: "Europe/Berlin"},
		{Name: "bob", Zone: "America/New_York"},
	})
	if err != nil || len(ids) != 2 {
		t.Fatalf("AddBlitzEntrants = %v, %v; want two ids", ids, err)
	}
	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	want := []BlitzEntrant{
		{ID: ids[0], Name: "Ann", Player: "", DiscordName: "ann#1", Zone: "Europe/Berlin", Status: "active"},
		{ID: ids[1], Name: "Bob", Player: "Bob", DiscordName: "", Zone: "America/New_York", Status: "active"},
	}
	if diff := cmp.Diff(want, ed.Entrants); diff != "" {
		t.Errorf("admin's entrants (-want +got):\n%s", diff)
	}
	if ed.BlitzEditionRow.Entrants != 2 || ed.Dropped != 0 {
		t.Errorf("counts = in %d out %d, want 2 and 0", ed.BlitzEditionRow.Entrants, ed.Dropped)
	}

	dropped, zone := "dropped", "Europe/London"
	if err := a.SetBlitzEntrant(ctx, "us_open", "2026", ids[1], BlitzEntrantChange{Status: &dropped}); err != nil {
		t.Fatalf("SetBlitzEntrant(dropped): %v", err)
	}
	if err := a.SetBlitzEntrant(ctx, "us_open", "2026", ids[0], BlitzEntrantChange{Zone: &zone}); err != nil {
		t.Fatalf("SetBlitzEntrant(zone): %v", err)
	}
	shown := true
	if err := a.SetTournament(ctx, "us_open", TournamentSettings{Visible: &shown}); err != nil {
		t.Fatalf("SetTournament(visible): %v", err)
	}
	public, err := NewRatingsReader(asReader(t, pool)).BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("the site's BlitzEdition: %v", err)
	}
	wantPublic := []BlitzEntrant{{ID: ids[0], Name: "Ann", Zone: "Europe/London", Status: "active"}}
	if diff := cmp.Diff(wantPublic, public.Entrants); diff != "" {
		t.Errorf("the site's entrants (-want +got):\n%s", diff)
	}
	if public.BlitzEditionRow.Entrants != 1 || public.Dropped != 1 {
		t.Errorf("the site's counts = in %d out %d, want 1 and 1", public.BlitzEditionRow.Entrants, public.Dropped)
	}

	if err := a.RemoveBlitzEntrant(ctx, "us_open", "2026", ids[0]); err != nil {
		t.Fatalf("RemoveBlitzEntrant: %v", err)
	}
	if ed, err = a.BlitzEdition(ctx, "us_open", "2026"); err != nil || len(ed.Entrants) != 1 || ed.Entrants[0].Name != "Bob" {
		t.Errorf("entrants after removing Ann = %+v, %v; want Bob alone", ed.Entrants, err)
	}
}

func TestBlitzEntrantsAreRefusedWhenTheyShouldBe(t *testing.T) {
	tests := []struct {
		name          string
		slug, edition string
		stored, added []BlitzEntrant
		want          error
		wantNamed     string
	}{
		{"an unknown edition", "us_open", "2027", nil, []BlitzEntrant{{Name: "Ann", Zone: "UTC"}}, ErrNoBlitzEdition, ""},
		{"a series not marked blitz", "worlds", "2025", nil, []BlitzEntrant{{Name: "Ann", Zone: "UTC"}}, ErrNoBlitzEdition, ""},
		{"no name", "us_open", "2026", nil, []BlitzEntrant{{Name: " ", Zone: "UTC"}}, ErrBadBlitzEntrant, ""},
		{"a long name", "us_open", "2026", nil, []BlitzEntrant{{Name: strings.Repeat("x", 101), Zone: "UTC"}}, ErrBadBlitzEntrant, ""},
		{"a long Discord name", "us_open", "2026", nil, []BlitzEntrant{{Name: "Ann", DiscordName: strings.Repeat("x", 101), Zone: "UTC"}}, ErrBadBlitzEntrant, ""},
		{"no zone", "us_open", "2026", nil, []BlitzEntrant{{Name: "Ann"}}, ErrBadBlitzEntrant, ""},
		{"a name twice in the list", "us_open", "2026", nil, []BlitzEntrant{{Name: "Ann", Zone: "UTC"}, {Name: "ann", Zone: "UTC"}}, &BlitzEntrantExistsError{}, "ann"},
		{"a name already entered", "us_open", "2026", []BlitzEntrant{{Name: "Ann", Zone: "UTC"}}, []BlitzEntrant{{Name: "ANN", Zone: "UTC"}, {Name: "Cy", Zone: "UTC"}}, &BlitzEntrantExistsError{}, "Ann"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, a := blitzFixture(t)
			ctx := context.Background()
			if len(tt.stored) > 0 {
				if _, err := a.AddBlitzEntrants(ctx, tt.slug, tt.edition, tt.stored); err != nil {
					t.Fatalf("storing first: %v", err)
				}
			}
			_, err := a.AddBlitzEntrants(ctx, tt.slug, tt.edition, tt.added)
			var exists *BlitzEntrantExistsError
			if _, wantExists := tt.want.(*BlitzEntrantExistsError); wantExists {
				if !errors.As(err, &exists) || !strings.Contains(err.Error(), tt.wantNamed) {
					t.Fatalf("AddBlitzEntrants = %v, want a BlitzEntrantExistsError naming %s", err, tt.wantNamed)
				}
			} else if !errors.Is(err, tt.want) {
				t.Fatalf("AddBlitzEntrants = %v, want %v", err, tt.want)
			}
			if tt.slug == "us_open" && tt.edition == "2026" {
				ed, err := a.BlitzEdition(ctx, tt.slug, tt.edition)
				if err != nil || len(ed.Entrants) != len(tt.stored) {
					t.Errorf("a refused add still added: %d entrants, %v; want %d", len(ed.Entrants), err, len(tt.stored))
				}
			}
		})
	}
}

func TestBlitzEntrantChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	_, a := blitzFixture(t)
	ctx := context.Background()
	ids, err := a.AddBlitzEntrants(ctx, "us_open", "2026", []BlitzEntrant{{Name: "Ann", Zone: "UTC"}})
	if err != nil {
		t.Fatalf("AddBlitzEntrants: %v", err)
	}
	gone, blank, long := "gone", " ", strings.Repeat("x", 101)
	tests := []struct {
		name    string
		edition string
		id      int64
		change  BlitzEntrantChange
		want    error
	}{
		{"a status that is not active or dropped", "2026", ids[0], BlitzEntrantChange{Status: &gone}, ErrBadBlitzStatus},
		{"an empty zone", "2026", ids[0], BlitzEntrantChange{Zone: &blank}, ErrBadBlitzEntrant},
		{"a long Discord name", "2026", ids[0], BlitzEntrantChange{DiscordName: &long}, ErrBadBlitzEntrant},
		{"an entrant of another edition", "2025", ids[0], BlitzEntrantChange{DiscordName: &blank}, ErrNoBlitzEntrant},
		{"an unknown entrant", "2026", ids[0] + 100, BlitzEntrantChange{DiscordName: &blank}, ErrNoBlitzEntrant},
		{"an unknown edition", "2027", ids[0], BlitzEntrantChange{DiscordName: &blank}, ErrNoBlitzEdition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := a.SetBlitzEntrant(ctx, "us_open", tt.edition, tt.id, tt.change); !errors.Is(err, tt.want) {
				t.Errorf("SetBlitzEntrant = %v, want %v", err, tt.want)
			}
		})
	}
	if err := a.RemoveBlitzEntrant(ctx, "us_open", "2026", ids[0]+100); !errors.Is(err, ErrNoBlitzEntrant) {
		t.Errorf("RemoveBlitzEntrant(unknown) = %v, want ErrNoBlitzEntrant", err)
	}
	if err := a.RemoveBlitzEntrant(ctx, "us_open", "2025", ids[0]); !errors.Is(err, ErrNoBlitzEntrant) {
		t.Errorf("RemoveBlitzEntrant(another edition) = %v, want ErrNoBlitzEntrant", err)
	}
	if _, err := a.BlitzEdition(ctx, "worlds", "2025"); !errors.Is(err, ErrNoBlitzEdition) {
		t.Errorf("BlitzEdition(worlds) = %v, want ErrNoBlitzEdition", err)
	}
}

func TestBlitzFormatIsSetAndRefused(t *testing.T) {
	_, a := blitzFixture(t)
	ctx := context.Background()
	if err := a.SetBlitzFormat(ctx, "us_open", "2026", BlitzFormat{SwissRounds: 5, KnockoutSize: 4}); err != nil {
		t.Fatalf("SetBlitzFormat: %v", err)
	}
	got, err := a.BlitzEditions(ctx)
	if err != nil {
		t.Fatalf("BlitzEditions: %v", err)
	}
	if want := []string{"us_open 2026 5/4 in=0 out=0", "us_open 2025 8/8 in=0 out=0"}; !cmp.Equal(want, editionLines(got)) {
		t.Errorf("editions after setting a format = %v, want %v", editionLines(got), want)
	}
	tests := []struct {
		name    string
		edition string
		format  BlitzFormat
		want    error
	}{
		{"no Swiss round", "2026", BlitzFormat{SwissRounds: 0, KnockoutSize: 8}, ErrBadBlitzFormat},
		{"too many Swiss rounds", "2026", BlitzFormat{SwissRounds: 31, KnockoutSize: 8}, ErrBadBlitzFormat},
		{"a knockout of 3", "2026", BlitzFormat{SwissRounds: 8, KnockoutSize: 3}, ErrBadBlitzFormat},
		{"an unknown edition", "2027", BlitzFormat{SwissRounds: 8, KnockoutSize: 8}, ErrNoBlitzEdition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := a.SetBlitzFormat(ctx, "us_open", tt.edition, tt.format); !errors.Is(err, tt.want) {
				t.Errorf("SetBlitzFormat = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestThePublicRoleCannotReadADiscordName(t *testing.T) {
	pool, _ := blitzFixture(t)
	ctx := context.Background()
	reader := asReader(t, pool)
	if _, err := reader.Exec(ctx, "SELECT discord_name FROM blitz_entries"); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("ratings_read reading discord_name = %v, want permission denied", err)
	}
	if _, err := reader.Exec(ctx, "SELECT entry_id, tournament_id, name, zone, status, joined_at FROM blitz_entries"); err != nil {
		t.Errorf("ratings_read reading the other columns: %v, want nil", err)
	}
}
