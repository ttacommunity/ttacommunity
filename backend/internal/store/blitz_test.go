//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/blitz"
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
		{Name: "Cy", Zone: "UTC"},
	})
	if err != nil || len(ids) != 3 {
		t.Fatalf("AddBlitzEntrants = %v, %v; want three ids", ids, err)
	}
	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	want := []BlitzEntrant{
		{ID: ids[0], Name: "Ann", Player: "", DiscordName: "ann#1", Zone: "Europe/Berlin", Status: "active"},
		{ID: ids[1], Name: "Bob", Player: "Bob", DiscordName: "", Zone: "America/New_York", Status: "active"},
		{ID: ids[2], Name: "Cy", Player: "", DiscordName: "", Zone: "UTC", Status: "active"},
	}
	if diff := cmp.Diff(want, ed.Entrants, cmpopts.IgnoreFields(BlitzEntrant{}, "MemberID")); diff != "" {
		t.Errorf("admin's entrants (-want +got):\n%s", diff)
	}
	for _, e := range ed.Entrants {
		if e.MemberID == 0 {
			t.Errorf("entrant %s has no member", e.Name)
		}
	}
	if ed.BlitzEditionRow.Entrants != 3 || ed.Dropped != 0 {
		t.Errorf("counts = in %d out %d, want 3 and 0", ed.BlitzEditionRow.Entrants, ed.Dropped)
	}

	zone := "Europe/London"
	markDropped(t, a, ids[1])
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
	wantPublic := []BlitzEntrant{
		{ID: ids[0], Name: "Ann", Zone: "Europe/London", Status: "active"},
		{ID: ids[1], Name: "Bob", Player: "Bob", Zone: "America/New_York", Status: "dropped"},
		{ID: ids[2], Name: "Cy", Zone: "UTC", Status: "active"},
	}
	if diff := cmp.Diff(wantPublic, public.Entrants, cmpopts.IgnoreFields(BlitzEntrant{}, "MemberID")); diff != "" {
		t.Errorf("the site's entrants, everyone entered, without Discord names (-want +got):\n%s", diff)
	}
	if public.BlitzEditionRow.Entrants != 2 || public.Dropped != 1 {
		t.Errorf("the site's counts = in %d out %d, want 2 and 1", public.BlitzEditionRow.Entrants, public.Dropped)
	}

	if err := a.RemoveBlitzEntrant(ctx, "us_open", "2026", ids[0]); err != nil {
		t.Fatalf("RemoveBlitzEntrant: %v", err)
	}
	if ed, err = a.BlitzEdition(ctx, "us_open", "2026"); err != nil || len(ed.Entrants) != 2 || ed.Entrants[0].Name != "Bob" {
		t.Errorf("entrants after removing Ann = %+v, %v; want Bob and Cy", ed.Entrants, err)
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
		{"a long name", "us_open", "2026", nil, []BlitzEntrant{{Name: strings.Repeat("x", 101), Zone: "UTC"}}, ErrBadMember, ""},
		{"a long Discord name", "us_open", "2026", nil, []BlitzEntrant{{Name: "Ann", DiscordName: strings.Repeat("x", 101), Zone: "UTC"}}, ErrBadMember, ""},
		{"no zone", "us_open", "2026", nil, []BlitzEntrant{{Name: "Ann"}}, ErrBadMember, ""},
		{"an unknown member", "us_open", "2026", nil, []BlitzEntrant{{MemberID: 999}}, ErrNoMember, ""},
		{"a name twice in the list", "us_open", "2026", nil, []BlitzEntrant{{Name: "Ann", Zone: "UTC"}, {Name: "ann", Zone: "UTC"}}, &BlitzEntrantExistsError{}, "Ann"},
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
	gone, pending, blank, long := "gone", "pending", " ", strings.Repeat("x", 101)
	tests := []struct {
		name    string
		edition string
		id      int64
		change  BlitzEntrantChange
		want    error
	}{
		{"a status that is not active or dropped", "2026", ids[0], BlitzEntrantChange{Status: &gone}, ErrBadBlitzStatus},
		{"pending, no longer a status", "2026", ids[0], BlitzEntrantChange{Status: &pending}, ErrBadBlitzStatus},
		{"an empty zone", "2026", ids[0], BlitzEntrantChange{Zone: &blank}, ErrBadMember},
		{"a long Discord name", "2026", ids[0], BlitzEntrantChange{DiscordName: &long}, ErrBadMember},
		{"a country that is not a code", "2026", ids[0], BlitzEntrantChange{Country: &long}, ErrBadMember},
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
	if _, err := reader.Exec(ctx, "SELECT discord_name FROM members"); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("ratings_read reading discord_name = %v, want permission denied", err)
	}
	if _, err := reader.Exec(ctx, "SELECT entry_id, tournament_id, member_id, status, joined_at FROM blitz_entries"); err != nil {
		t.Errorf("ratings_read reading an entry's columns: %v, want nil", err)
	}
}

// candidatesFixture adds to blitzFixture the players Ann (GB), Annabel, Annette, Annika, Hannah and Joanna, the
// members anna, Annette (that player), Annika (entered in us_open 2026) and Bea (banned).
func candidatesFixture(t *testing.T) (*pgxpool.Pool, *AdminStore) {
	t.Helper()
	pool, a := blitzFixture(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO players (name, country_code) VALUES ('Ann', 'GB'), ('Annabel', NULL), ('Annette', NULL),
		('Annika', NULL), ('Hannah', NULL), ('Joanna', NULL)`); err != nil {
		t.Fatalf("adding players: %v", err)
	}
	for _, m := range []Member{{Name: "anna", Zone: "Unknown"}, {Name: "annette", Email: "annette@example.test", Zone: "US East"}, {Name: "Bea", Zone: "UTC"}} {
		if _, err := a.AddMember(ctx, m); err != nil {
			t.Fatalf("AddMember(%s): %v", m.Name, err)
		}
	}
	if _, err := a.AddBlitzEntrants(ctx, "us_open", "2026", []BlitzEntrant{{Name: "Annika", Zone: "Unknown"}}); err != nil {
		t.Fatalf("entering Annika: %v", err)
	}
	members, err := a.Members(ctx)
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	for _, m := range members {
		if m.Name == "Bea" {
			if err := a.BanMember(ctx, m.ID, "Abusive in Discord"); err != nil {
				t.Fatalf("BanMember: %v", err)
			}
		}
	}
	return pool, a
}

func candidateLines(found []Candidate) []string {
	var out []string
	for _, c := range found {
		line := c.Name
		if c.MemberID > 0 {
			line += " (member)"
		}
		if c.Banned {
			line += " banned"
		}
		out = append(out, line)
	}
	return out
}

func TestBlitzCandidatesAreTheBestFiveNotYetEntered(t *testing.T) {
	_, a := candidatesFixture(t)
	tests := []struct {
		name, edition, q string
		want             []string
	}{
		{"the name itself, then names starting with it, members first, then the rest", "2026", "ann", []string{"Ann", "anna (member)", "Annette (member)", "Annabel", "Hannah"}},
		{"any case", "2026", "ANNA", []string{"anna (member)", "Annabel", "Hannah", "Joanna"}},
		{"a banned member, marked", "2026", "bea", []string{"Bea (member) banned"}},
		{"a member entered in another edition", "2025", "annik", []string{"Annika (member)"}},
		{"a member entered in this one", "2026", "annik", nil},
		{"nobody", "2026", "zz", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := a.BlitzCandidates(context.Background(), "us_open", tt.edition, tt.q, 5)
			if err != nil {
				t.Fatalf("BlitzCandidates(%q): %v", tt.q, err)
			}
			if diff := cmp.Diff(tt.want, candidateLines(got)); diff != "" {
				t.Errorf("BlitzCandidates(%q) (-want +got):\n%s", tt.q, diff)
			}
		})
	}
}

func TestBlitzCandidatesCarryWhatIsKnownOfThem(t *testing.T) {
	_, a := candidatesFixture(t)
	ctx := context.Background()
	got, err := a.BlitzCandidates(ctx, "us_open", "2026", "Ann", 2)
	if err != nil {
		t.Fatalf("BlitzCandidates: %v", err)
	}
	want := []Candidate{
		{Name: "Ann", Player: "Ann", Country: "GB"},
		{Name: "anna", Zone: "Unknown"},
	}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(Candidate{}, "MemberID")); diff != "" {
		t.Errorf("a player and a member found (-want +got):\n%s", diff)
	}
	if got, err = a.BlitzCandidates(ctx, "us_open", "2026", "annette", 1); err != nil || len(got) != 1 || got[0].Player != "Annette" || got[0].Email != "annette@example.test" || got[0].Zone != "US East" {
		t.Errorf("BlitzCandidates(annette) = %+v, %v; want the member with the player's spelling, email and zone", got, err)
	}
	if _, err := a.BlitzCandidates(ctx, "us_open", "2027", "ann", 5); !errors.Is(err, ErrNoBlitzEdition) {
		t.Errorf("BlitzCandidates for an unknown edition = %v, want ErrNoBlitzEdition", err)
	}
}

func TestPeopleAreTheBestFiveOfEveryMemberAndPlayer(t *testing.T) {
	_, a := candidatesFixture(t)
	tests := []struct {
		q    string
		want []string
	}{
		{"annik", []string{"Annika (member)"}},
		{"ann", []string{"Ann", "anna (member)", "Annette (member)", "Annika (member)", "Annabel"}},
		{"bea", []string{"Bea (member) banned"}},
	}
	for _, tt := range tests {
		got, err := a.People(context.Background(), tt.q, 5)
		if err != nil {
			t.Fatalf("People(%q): %v", tt.q, err)
		}
		if diff := cmp.Diff(tt.want, candidateLines(got)); diff != "" {
			t.Errorf("People(%q), members entered anywhere included (-want +got):\n%s", tt.q, diff)
		}
	}
}

func TestAnEditionScoresByThePointsLastSet(t *testing.T) {
	a, reader, _ := blitzWithEntrants(t)
	ctx := context.Background()
	points := func(edition string) blitz.Scoring {
		t.Helper()
		ed, err := a.BlitzEdition(ctx, "us_open", edition)
		if err != nil {
			t.Fatalf("BlitzEdition(%s): %v", edition, err)
		}
		return ed.Points
	}
	if got := points("2025"); got != blitz.FIDEScoring {
		t.Errorf("points of an edition never set up = %+v, want FIDE's", got)
	}
	if got := points("2026"); got != blitz.FIDEScoring {
		t.Errorf("points of an edition with entrants = %+v, want FIDE's", got)
	}
	ours := blitz.Scoring{Win: 3, Draw: 1.5, Loss: 0.5, ForfeitWin: 2, ForfeitLoss: 0, Bye: 2.5, Skip: 10}
	if err := a.SetBlitzPoints(ctx, "us_open", "2026", ours); err != nil {
		t.Fatalf("SetBlitzPoints: %v", err)
	}
	if got := points("2026"); got != ours {
		t.Errorf("points for admin = %+v, want %+v", got, ours)
	}
	if ed, err := reader.BlitzEdition(ctx, "us_open", "2026"); err != nil || ed.Points != ours {
		t.Errorf("points for the site = %+v, %v; want %+v", ed.Points, err, ours)
	}
	if err := a.SetBlitzPoints(ctx, "us_open", "2027", ours); !errors.Is(err, ErrNoBlitzEdition) {
		t.Errorf("SetBlitzPoints(an unknown edition) = %v, want ErrNoBlitzEdition", err)
	}
}

func TestThePointsLockOnceTheKnockoutHasARound(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	if err := a.SetBlitzFormat(ctx, "us_open", "2026", BlitzFormat{SwissRounds: 1, KnockoutSize: 4}); err != nil {
		t.Fatalf("SetBlitzFormat: %v", err)
	}
	swiss, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Seed: 1, Matches: []BlitzMatch{{A: ids[0], B: &ids[1]}, {A: ids[2], B: &ids[3]}}})
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
	ko := func(a, b int64, slot, seedA, seedB int) BlitzMatch {
		return BlitzMatch{A: a, B: &b, Slot: new(slot), SeedA: new(seedA), SeedB: new(seedB)}
	}
	semi, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Kind: "knockout", Seed: 2, Matches: []BlitzMatch{ko(ids[0], ids[3], 0, 1, 4), ko(ids[2], ids[1], 1, 2, 3)}})
	if err != nil {
		t.Fatalf("AddBlitzRound(semi-finals): %v", err)
	}
	ours := blitz.Scoring{Win: 3, Draw: 1, ForfeitWin: 3, Bye: 3}
	if err := a.SetBlitzPoints(ctx, "us_open", "2026", ours); !errors.Is(err, ErrBlitzPointsLocked) {
		t.Errorf("SetBlitzPoints with the knockout paired = %v, want ErrBlitzPointsLocked", err)
	}
	if err := a.DeleteBlitzRound(ctx, "us_open", "2026", semi.ID); err != nil {
		t.Fatalf("DeleteBlitzRound(semi-finals): %v", err)
	}
	if err := a.SetBlitzPoints(ctx, "us_open", "2026", ours); err != nil {
		t.Errorf("SetBlitzPoints once the knockout is deleted = %v, want nil", err)
	}
}

func TestTheDatabaseRefusesPointsOutOfRangeOrOrder(t *testing.T) {
	_, a := blitzFixture(t)
	with := func(change func(*blitz.Scoring)) blitz.Scoring {
		s := blitz.FIDEScoring
		change(&s)
		return s
	}
	tests := []struct {
		name, constraint string
		points           blitz.Scoring
	}{
		{"a win over 10", "blitz_editions_points_win_check", with(func(s *blitz.Scoring) { s.Win = 10.5 })},
		{"a skip below 0", "blitz_editions_points_skip_check", with(func(s *blitz.Scoring) { s.Skip = -1 })},
		{"a bye of a quarter", "blitz_editions_points_bye_check", with(func(s *blitz.Scoring) { s.Bye = 0.25 })},
		{"a forfeit loss of a quarter", "blitz_editions_points_forfeit_loss_check", with(func(s *blitz.Scoring) { s.ForfeitLoss = 0.25 })},
		{"a draw above a win", "blitz_editions_points_order_check", with(func(s *blitz.Scoring) { s.Draw = 1.5 })},
		{"a loss above a draw", "blitz_editions_points_order_check", with(func(s *blitz.Scoring) { s.Loss = 1 })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := a.SetBlitzPoints(context.Background(), "us_open", "2026", tt.points); err == nil || !strings.Contains(err.Error(), tt.constraint) {
				t.Errorf("err = %v, want %s to refuse it", err, tt.constraint)
			}
		})
	}
}

func TestTheMigrationTakesPendingEntrantsOut(t *testing.T) {
	pool := newDB(t)
	ctx := context.Background()
	if err := migrateRatingsTo(ctx, pool, 23); err != nil {
		t.Fatalf("migrateRatingsTo(23): %v", err)
	}
	for _, q := range []string{
		`INSERT INTO series (slug, name, blitz) VALUES ('us_open', 'US Open', true)`,
		`INSERT INTO tournaments (series_id, label) SELECT series_id, '2026' FROM series`,
		`INSERT INTO blitz_editions (tournament_id, swiss_rounds, knockout_size) SELECT tournament_id, 8, 8 FROM tournaments`,
		`INSERT INTO members (tta_name) VALUES ('Ann'), ('Bob'), ('Cy')`,
		`INSERT INTO blitz_entries (tournament_id, member_id, status)
		 SELECT tournament_id, member_id, CASE tta_name WHEN 'Ann' THEN 'active' ELSE 'pending' END FROM tournaments, members`,
		`INSERT INTO blitz_rounds (tournament_id, number, seed) SELECT tournament_id, 1, 1 FROM tournaments`,
		`INSERT INTO blitz_matches (round_id, entry_a, entry_b)
		 SELECT round_id, (SELECT entry_id FROM blitz_entries JOIN members USING (member_id) WHERE tta_name = 'Ann'),
		        (SELECT entry_id FROM blitz_entries JOIN members USING (member_id) WHERE tta_name = 'Bob') FROM blitz_rounds`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("building the old shape: %v\n%s", err, q)
		}
	}
	if err := migrateRatingsTo(ctx, pool, 24); err != nil {
		t.Fatalf("migrateRatingsTo(24): %v", err)
	}
	rows, err := pool.Query(ctx, "SELECT m.tta_name || ' ' || x.status FROM blitz_entries x JOIN members m USING (member_id) ORDER BY 1")
	if err != nil {
		t.Fatalf("reading the entrants: %v", err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("reading the entrants: %v", err)
	}
	if diff := cmp.Diff([]string{"Ann active", "Bob active"}, got); diff != "" {
		t.Errorf("entrants once pending goes: Cy, in no match, deleted; Bob, paired, in (-want +got):\n%s", diff)
	}
	if _, err := pool.Exec(ctx, "UPDATE blitz_entries SET status = 'pending'"); err == nil {
		t.Errorf("the database took an entrant back to pending")
	}
}
