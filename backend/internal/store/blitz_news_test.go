//go:build integration

package store

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// blitzSent sends what is due and returns each post as "endpoint kind slug edition round", with " again" for a
// round posted before with other games.
func blitzSent(t *testing.T, a *AdminStore) []string {
	t.Helper()
	var got []string
	if _, err := a.SendNotifications(context.Background(), time.Now().Add(time.Hour), 50, func(_ context.Context, n Notification, e NotificationEndpoint) error {
		line := fmt.Sprintf("%s %s %s %s %d", e.Name, n.Kind, n.Slug, n.Edition, n.Round)
		if n.Blitz.Again {
			line += " again"
		}
		got = append(got, line)
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	slices.Sort(got)
	return got
}

// publishedRound adds the edition's next Swiss round with pairs, each two entrants (the second 0 for the bye),
// and publishes it, returning its id.
func publishedRound(t *testing.T, a *AdminStore, pairs ...[2]int64) int64 {
	t.Helper()
	ctx := context.Background()
	var matches []BlitzMatch
	for _, p := range pairs {
		m := BlitzMatch{A: p[0]}
		if p[1] != 0 {
			m.B = &p[1]
		}
		matches = append(matches, m)
	}
	r, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Method: "dutch", Order: "rating", Seed: 1, Matches: matches})
	if err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", r.ID); err != nil {
		t.Fatalf("PublishBlitzRound: %v", err)
	}
	return r.ID
}

func deleteRound(t *testing.T, a *AdminStore, id int64) {
	t.Helper()
	if err := a.DeleteBlitzRound(context.Background(), "us_open", "2026", id); err != nil {
		t.Fatalf("DeleteBlitzRound(%d): %v", id, err)
	}
}

func TestPublishingABlitzRoundQueuesItsNewsOnce(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ann, bob, cy, dee := ids[0], ids[1], ids[2], ids[3]
	endpoint(t, a, "Blitz", 1, "blitz_round")
	endpoint(t, a, "Admins", 2, "blitz_report")
	r1 := publishedRound(t, a, [2]int64{ann, bob}, [2]int64{cy, dee})
	if got, want := blitzSent(t, a), []string{"Blitz blitz_round us_open 2026 1"}; !slices.Equal(got, want) {
		t.Errorf("round 1 published = %q, want %q", got, want)
	}
	if err := a.PublishBlitzRound(context.Background(), "us_open", "2026", r1); err != nil {
		t.Fatalf("PublishBlitzRound again: %v", err)
	}
	if got := blitzSent(t, a); len(got) != 0 {
		t.Errorf("publishing round 1 again = %q, want nothing", got)
	}
}

func TestARoundPairedAgainIsPostedOnlyWhenItsGamesChanged(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ann, bob, cy, dee := ids[0], ids[1], ids[2], ids[3]
	endpoint(t, a, "Blitz", 1, "blitz_round")
	first := publishedRound(t, a, [2]int64{ann, bob}, [2]int64{cy, dee})
	blitzSent(t, a)
	deleteRound(t, a, first)
	steps := []struct {
		name  string
		pairs [][2]int64
		want  []string
	}{
		{"the same games", [][2]int64{{cy, dee}, {bob, ann}}, nil},
		{"other games", [][2]int64{{ann, cy}, {bob, dee}}, []string{"Blitz blitz_round us_open 2026 1 again"}},
		{"those games again", [][2]int64{{ann, cy}, {bob, dee}}, nil},
		{"the first games back", [][2]int64{{ann, bob}, {cy, dee}}, []string{"Blitz blitz_round us_open 2026 1 again"}},
	}
	for _, s := range steps {
		id := publishedRound(t, a, s.pairs...)
		if got := blitzSent(t, a); !slices.Equal(got, s.want) {
			t.Errorf("round 1 paired again with %s = %q, want %q", s.name, got, s.want)
		}
		deleteRound(t, a, id)
	}
}

func TestARoundTakenBackBeforeItsNewsIsSentIsPostedWhenPublishedAgain(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ann, bob, cy, dee := ids[0], ids[1], ids[2], ids[3]
	endpoint(t, a, "Blitz", 1, "blitz_round")
	deleteRound(t, a, publishedRound(t, a, [2]int64{ann, bob}, [2]int64{cy, dee}))
	publishedRound(t, a, [2]int64{ann, bob}, [2]int64{cy, dee})
	if got, want := blitzSent(t, a), []string{"Blitz blitz_round us_open 2026 1"}; !slices.Equal(got, want) {
		t.Errorf("sent = %q, want round 1 once, not again: the first was never posted", got)
	}
}

func TestAByeIsPartOfARoundsGames(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ann, bob, cy := ids[0], ids[1], ids[2]
	markDropped(t, a, ids[3])
	endpoint(t, a, "Blitz", 1, "blitz_round")
	first := publishedRound(t, a, [2]int64{ann, bob}, [2]int64{cy, 0})
	blitzSent(t, a)
	deleteRound(t, a, first)
	publishedRound(t, a, [2]int64{cy, bob}, [2]int64{ann, 0})
	if got, want := blitzSent(t, a), []string{"Blitz blitz_round us_open 2026 1 again"}; !slices.Equal(got, want) {
		t.Errorf("another bye = %q, want %q", got, want)
	}
}

// signedInWithDiscord links the member entered as entrant to a new account that signs in with Discord as id, as
// pool's owner: the admin login cannot.
func signedInWithDiscord(t *testing.T, pool *pgxpool.Pool, entrant int64, id string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `WITH x AS (
			INSERT INTO accounts (created_at, member_id, linked_how, linked_at)
			SELECT now(), member_id, 'approved', now() FROM blitz_entries WHERE entry_id = $1 RETURNING account_id)
		INSERT INTO logins (provider, subject, username, created_at, signed_in_at, account_id)
		SELECT 'discord', $2, 'player', now(), now(), account_id FROM x`, entrant, id); err != nil {
		t.Fatalf("signing entrant %d in with Discord: %v", entrant, err)
	}
}

func TestAnEntrantsDiscordIDComesFromTheirDiscordSignInForAdminOnly(t *testing.T) {
	pool, a := blitzFixture(t)
	ctx := context.Background()
	ids, err := a.AddBlitzEntrants(ctx, "us_open", "2026", []BlitzEntrant{{Name: "Ann", Zone: "UTC"}, {Name: "Bob", Zone: "UTC"}})
	if err != nil {
		t.Fatalf("AddBlitzEntrants: %v", err)
	}
	shown := true
	if err := a.SetTournament(ctx, "us_open", TournamentSettings{Visible: &shown}); err != nil {
		t.Fatalf("SetTournament(visible): %v", err)
	}
	reader := NewRatingsReader(asReader(t, pool))
	signedInWithDiscord(t, pool, ids[0], "100000000000000041")
	if err := a.AddBlitzOrganiser(ctx, "us_open", "2026", memberOfEntrant(t, a, ids[0])); err != nil {
		t.Fatalf("AddBlitzOrganiser: %v", err)
	}
	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	if got := []string{ed.Entrants[0].DiscordID, ed.Entrants[1].DiscordID}; !slices.Equal(got, []string{"100000000000000041", ""}) {
		t.Errorf("admin's entrants' Discord ids = %q, want Ann's from her sign-in and none for Bob", got)
	}
	if len(ed.Organisers) != 1 || ed.Organisers[0].DiscordID != "100000000000000041" {
		t.Errorf("organisers = %+v, want Ann with her Discord id", ed.Organisers)
	}
	public, err := reader.BlitzEdition(ctx, "us_open", "2026")
	if err != nil || public.Entrants[0].DiscordID != "" {
		t.Errorf("the site's Ann = %+v, %v; want no Discord id", public.Entrants[0], err)
	}
}

// memberOfEntrant is entrant's member id.
func memberOfEntrant(t *testing.T, a *AdminStore, entrant int64) int64 {
	t.Helper()
	var id int64
	if err := a.pool.QueryRow(context.Background(), "SELECT member_id FROM blitz_entries WHERE entry_id = $1", entrant).Scan(&id); err != nil {
		t.Fatalf("reading entrant %d's member: %v", entrant, err)
	}
	return id
}

// finishedIfPublished stands in for the podium's rule: an edition is finished once it has a published round.
func finishedIfPublished(ed BlitzEdition) bool { return ed.Round != nil }

func queueFinished(t *testing.T, a *AdminStore) int {
	t.Helper()
	n, err := a.QueueFinishedBlitz(context.Background(), time.Now(), finishedIfPublished)
	if err != nil {
		t.Fatalf("QueueFinishedBlitz: %v", err)
	}
	return n
}

func TestAFinishedEditionIsQueuedOnce(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	endpoint(t, a, "Blitz", 1, "blitz_finished")
	if n := queueFinished(t, a); n != 0 {
		t.Errorf("queued before any round = %d, want 0", n)
	}
	publishedRound(t, a, [2]int64{ids[0], ids[1]}, [2]int64{ids[2], ids[3]})
	if n := queueFinished(t, a); n != 1 {
		t.Errorf("queued once finished = %d, want 1", n)
	}
	if n := queueFinished(t, a); n != 0 {
		t.Errorf("queued again = %d, want 0", n)
	}
	if got, want := blitzSent(t, a), []string{"Blitz blitz_finished us_open 2026 0"}; !slices.Equal(got, want) {
		t.Errorf("sent = %q, want %q", got, want)
	}
}

func TestResettingAnEditionLetsItsNewsBePostedAgain(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ann, bob, cy, dee := ids[0], ids[1], ids[2], ids[3]
	endpoint(t, a, "Blitz", 1, "blitz_round", "blitz_finished")
	publishedRound(t, a, [2]int64{ann, bob}, [2]int64{cy, dee})
	queueFinished(t, a)
	blitzSent(t, a)
	if err := a.ResetBlitzEdition(context.Background(), "us_open", "2026"); err != nil {
		t.Fatalf("ResetBlitzEdition: %v", err)
	}
	publishedRound(t, a, [2]int64{ann, bob}, [2]int64{cy, dee})
	queueFinished(t, a)
	want := []string{"Blitz blitz_finished us_open 2026 0", "Blitz blitz_round us_open 2026 1"}
	if got := blitzSent(t, a); !slices.Equal(got, want) {
		t.Errorf("after a reset = %q, want %q", got, want)
	}
}

func TestMigration98DropsOnlyBlitzNewsGoingDown(t *testing.T) {
	pool, a := blitzFixture(t)
	endpoint(t, a, "Both", 1, "blitz_report", "blitz_round", "blitz_finished")
	migrateTo(t, pool, 97, true)
	if got := rowsOf(t, pool, "SELECT array_to_string(events, ',') FROM notification_endpoints"); !slices.Equal(got, []string{"blitz_report"}) {
		t.Errorf("events at 97 = %q, want only blitz_report", got)
	}
	migrateTo(t, pool, 98, false)
}

func TestMigration98MarksEditionsFinishedBeforeItAsPosted(t *testing.T) {
	pool, a := blitzFixture(t)
	ctx := context.Background()
	ids, err := a.AddBlitzEntrants(ctx, "us_open", "2026", []BlitzEntrant{{Name: "Ann", Zone: "UTC"}, {Name: "Bob", Zone: "UTC"}})
	if err != nil {
		t.Fatalf("AddBlitzEntrants: %v", err)
	}
	if err := a.SetBlitzFormat(ctx, "us_open", "2026", BlitzFormat{SwissRounds: 1, KnockoutSize: 0}); err != nil {
		t.Fatalf("SetBlitzFormat: %v", err)
	}
	r := publishedRound(t, a, [2]int64{ids[0], ids[1]})
	ed, err := a.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	if err := a.RecordBlitzRuling(ctx, "us_open", "2026", ed.Rounds[0].Matches[0].ID, "a", "Bob did not show"); err != nil {
		t.Fatalf("RecordBlitzRuling(round %d): %v", r, err)
	}
	migrateTo(t, pool, 97, true)
	migrateTo(t, pool, 98, false)
	if n, err := a.QueueFinishedBlitz(ctx, time.Now(), func(BlitzEdition) bool { return true }); err != nil || n != 0 {
		t.Errorf("an edition finished before the migration = %d queued, %v; want 0: it is not news", n, err)
	}
}
