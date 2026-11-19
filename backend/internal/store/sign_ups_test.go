//go:build integration

package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// signUpAt is when the tests' players sign up, inside the sessions signedIn starts.
var signUpAt = signInAt.Add(time.Hour)

// signUpsFixture is blitzWithEntrants (Ann, Bob, Cy and Dee in us_open 2026, shown) with Eve signed in as "eve"
// and linked to her member, Uma signed in by Discord as "uma" and not linked, and a player store on the player login.
func signUpsFixture(t *testing.T) (*AdminStore, *PlayerStore, *RatingsReader, []int64) {
	t.Helper()
	a, reader, ids := blitzWithEntrants(t)
	p := NewPlayerStore(asPlayerLogin(t, a.pool.(*pgxpool.Pool)))
	approved(t, a, p, signedIn(t, p, discordID("100000000000000005", "eve", "Eve"), "eve"), "Eve")
	signedIn(t, p, discordID("100000000000000006", "uma", "Uma"), "uma")
	return a, p, reader, ids
}

// openSignUps opens us_open 2026's sign-ups at signInAt, closing at closes (nil for no closing time).
func openSignUps(t *testing.T, a *AdminStore, closes *time.Time) {
	t.Helper()
	if err := a.SetSignUps(context.Background(), "us_open", "2026", SignUps{Open: true, ClosesAt: closes}, signInAt); err != nil {
		t.Fatalf("SetSignUps(open): %v", err)
	}
}

// pairRoundOne pairs us_open 2026's round 1: Ann v Bob, Cy v Dee.
func pairRoundOne(t *testing.T, a *AdminStore, ids []int64) {
	t.Helper()
	if _, err := a.AddBlitzRound(context.Background(), "us_open", "2026", BlitzRound{Method: "dutch", Order: "rating", Seed: 7,
		Matches: []BlitzMatch{{A: ids[0], B: &ids[1]}, {A: ids[2], B: &ids[3]}}}); err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
}

// entered says whether name is an entrant of us_open 2026 as the site reads it.
func entered(t *testing.T, reader *RatingsReader, name string) bool {
	t.Helper()
	ed, err := reader.BlitzEdition(context.Background(), "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	for _, e := range ed.Entrants {
		if e.Name == name {
			return true
		}
	}
	return false
}

// pendingNames lists the TTA usernames of us_open 2026's pending sign-ups, in sign-up order.
func pendingNames(t *testing.T, a *AdminStore) []string {
	t.Helper()
	pending, err := a.PendingSignUps(context.Background(), "us_open", "2026")
	if err != nil {
		t.Fatalf("PendingSignUps: %v", err)
	}
	names := []string{}
	for _, s := range pending {
		names = append(names, s.Name)
	}
	return names
}

// onlyPending is us_open 2026's one pending sign-up.
func onlyPending(t *testing.T, a *AdminStore) PendingSignUp {
	t.Helper()
	pending, err := a.PendingSignUps(context.Background(), "us_open", "2026")
	if err != nil {
		t.Fatalf("PendingSignUps: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending sign-ups = %+v, want one", pending)
	}
	return pending[0]
}

// signUpPosts counts the sign-up news queued for us_open 2026.
func signUpPosts(t *testing.T, a *AdminStore) int {
	t.Helper()
	var n int
	if err := a.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM notification_outbox o JOIN tournaments t ON t.tournament_id = o.edition_id
		JOIN series s USING (series_id) WHERE o.kind = 'blitz_sign_ups' AND s.slug = 'us_open' AND t.label = '2026'`).Scan(&n); err != nil {
		t.Fatalf("counting sign-up news: %v", err)
	}
	return n
}

func TestALinkedPlayerEntersAndWithdrawsWhileSignUpsAreOpen(t *testing.T) {
	a, p, reader, _ := signUpsFixture(t)
	ctx := context.Background()
	openSignUps(t, a, nil)
	signUp := func() error {
		got, err := p.SignUp(ctx, tokenHash("eve"), "us_open", "2026", "", signUpAt)
		if err == nil && got != "entered" {
			t.Errorf("SignUp = %q, want entered", got)
		}
		return err
	}
	withdraw := func() error { return p.WithdrawSignUp(ctx, tokenHash("eve"), "us_open", "2026", signUpAt) }
	steps := []struct {
		name string
		do   func() error
		want bool
	}{
		{"entering", signUp, true},
		{"entering again", signUp, true},
		{"withdrawing", withdraw, false},
		{"withdrawing again", withdraw, false},
		{"entering after withdrawing", signUp, true},
	}
	for _, s := range steps {
		if err := s.do(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if got := entered(t, reader, "Eve"); got != s.want {
			t.Errorf("after %s, Eve entered = %v, want %v", s.name, got, s.want)
		}
	}
}

func TestAnUnlinkedPlayersSignUpWaitsWithALinkRequest(t *testing.T) {
	a, p, reader, _ := signUpsFixture(t)
	ctx := context.Background()
	openSignUps(t, a, nil)
	if _, err := p.SignUp(ctx, tokenHash("uma"), "us_open", "2026", "  ", signUpAt); !errors.Is(err, ErrSignUpNameNeeded) {
		t.Errorf("SignUp without a name = %v, want %v", err, ErrSignUpNameNeeded)
	}
	got, err := p.SignUp(ctx, tokenHash("uma"), "us_open", "2026", " Uma ", signUpAt)
	if err != nil || got != "pending" {
		t.Fatalf("SignUp = %q, %v, want pending", got, err)
	}
	s := onlyPending(t, a)
	if s.Name != "Uma" || s.Request == 0 || s.Linked != "" {
		t.Errorf("the pending sign-up = %+v, want Uma with a link request, not linked", s)
	}
	var asked string
	if err := a.pool.QueryRow(ctx, "SELECT tta_name FROM link_requests WHERE request_id = $1 AND status = 'pending'", s.Request).Scan(&asked); err != nil || asked != "Uma" {
		t.Errorf("the link request asks for %q (%v), want Uma", asked, err)
	}
	if entered(t, reader, "Uma") {
		t.Errorf("Uma is entered, want only a pending sign-up")
	}
	mine, ok, err := p.MySignUp(ctx, tokenHash("uma"), "us_open", "2026", signUpAt)
	if err != nil || !ok || mine.Name != "Uma" {
		t.Errorf("MySignUp = %+v, %v, %v, want Uma's", mine, ok, err)
	}
	kept, err := p.KeptData(ctx, tokenHash("uma"), signUpAt)
	if err != nil {
		t.Fatalf("KeptData: %v", err)
	}
	if len(kept.SignUps) != 1 || kept.SignUps[0].Tournament != "US Open 2026" || kept.SignUps[0].Name != "Uma" {
		t.Errorf("the sign-ups kept = %+v, want Uma's for US Open 2026", kept.SignUps)
	}
}

func TestWithdrawingAPendingSignUpLeavesItsLinkRequest(t *testing.T) {
	a, p, _, _ := signUpsFixture(t)
	ctx := context.Background()
	openSignUps(t, a, nil)
	if _, err := p.SignUp(ctx, tokenHash("uma"), "us_open", "2026", "Uma", signUpAt); err != nil {
		t.Fatalf("SignUp: %v", err)
	}
	request := onlyPending(t, a).Request
	if err := p.WithdrawSignUp(ctx, tokenHash("uma"), "us_open", "2026", signUpAt); err != nil {
		t.Fatalf("WithdrawSignUp: %v", err)
	}
	if got := pendingNames(t, a); len(got) != 0 {
		t.Errorf("pending sign-ups = %v, want none", got)
	}
	var status string
	if err := a.pool.QueryRow(ctx, "SELECT status FROM link_requests WHERE request_id = $1", request).Scan(&status); err != nil || status != "pending" {
		t.Errorf("the link request is %q (%v), want pending", status, err)
	}
}

func TestAPlayerCannotSignUpOrWithdrawWhenTheyShouldNot(t *testing.T) {
	closes := signInAt.Add(30 * time.Minute)
	tests := []struct {
		name  string
		token string
		setUp func(t *testing.T, a *AdminStore, ids []int64)
		want  error
	}{
		{"sign-ups never opened", "eve", func(*testing.T, *AdminStore, []int64) {}, ErrSignUpsClosed},
		{"sign-ups closed by admin", "eve", func(t *testing.T, a *AdminStore, _ []int64) {
			openSignUps(t, a, nil)
			if err := a.SetSignUps(context.Background(), "us_open", "2026", SignUps{}, signInAt); err != nil {
				t.Fatalf("SetSignUps(closed): %v", err)
			}
		}, ErrSignUpsClosed},
		{"past the closing time", "eve", func(t *testing.T, a *AdminStore, _ []int64) { openSignUps(t, a, &closes) }, ErrSignUpsClosed},
		{"round 1 paired", "eve", func(t *testing.T, a *AdminStore, ids []int64) {
			openSignUps(t, a, nil)
			pairRoundOne(t, a, ids)
		}, ErrSignUpsClosed},
		{"the edition hidden", "eve", func(t *testing.T, a *AdminStore, _ []int64) {
			openSignUps(t, a, nil)
			hidden := false
			if err := a.SetTournament(context.Background(), "us_open", TournamentSettings{Visible: &hidden}); err != nil {
				t.Fatalf("SetTournament(hidden): %v", err)
			}
		}, ErrNoBlitzEdition},
		{"not signed in", "nobody", func(t *testing.T, a *AdminStore, _ []int64) { openSignUps(t, a, nil) }, ErrNoSession},
		{"banned", "eve", func(t *testing.T, a *AdminStore, _ []int64) {
			openSignUps(t, a, nil)
			if _, err := a.pool.Exec(context.Background(), "UPDATE members SET banned = true, banned_at = $1 WHERE tta_name = 'Eve'", signInAt); err != nil {
				t.Fatalf("banning Eve: %v", err)
			}
		}, ErrMemberBanned},
	}
	for _, tt := range tests {
		for _, action := range []string{"sign up", "withdraw"} {
			t.Run(tt.name+", "+action, func(t *testing.T) {
				a, p, reader, ids := signUpsFixture(t)
				ctx := context.Background()
				if action == "withdraw" {
					if _, err := a.AddBlitzEntrants(ctx, "us_open", "2026", []BlitzEntrant{{Name: "Eve"}}); err != nil {
						t.Fatalf("AddBlitzEntrants(Eve): %v", err)
					}
				}
				tt.setUp(t, a, ids)
				var err error
				if action == "withdraw" {
					err = p.WithdrawSignUp(ctx, tokenHash(tt.token), "us_open", "2026", signUpAt)
				} else {
					_, err = p.SignUp(ctx, tokenHash(tt.token), "us_open", "2026", "", signUpAt)
				}
				if !errors.Is(err, tt.want) {
					t.Errorf("%s = %v, want %v", action, err, tt.want)
				}
				if errors.Is(tt.want, ErrNoBlitzEdition) {
					return
				}
				if got, want := entered(t, reader, "Eve"), action == "withdraw"; got != want {
					t.Errorf("Eve entered = %v, want %v as before", got, want)
				}
			})
		}
	}
}

func TestLinkingEntersAPendingSignUpOnlyWhileSignUpsTakeEntries(t *testing.T) {
	closes := signUpAt.Add(time.Hour)
	tests := []struct {
		name        string
		closeFirst  bool
		approveAt   time.Time
		wantEntered bool
	}{
		{"while they take entries", false, signUpAt, true},
		{"after the closing time", false, closes.Add(time.Minute), false},
		{"after admin closed them", true, signUpAt, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, p, reader, _ := signUpsFixture(t)
			ctx := context.Background()
			openSignUps(t, a, &closes)
			if _, err := p.SignUp(ctx, tokenHash("uma"), "us_open", "2026", "Uma", signUpAt); err != nil {
				t.Fatalf("SignUp: %v", err)
			}
			if tt.closeFirst {
				if err := a.SetSignUps(ctx, "us_open", "2026", SignUps{}, signUpAt); err != nil {
					t.Fatalf("SetSignUps(closed): %v", err)
				}
			}
			if _, err := a.ApproveLink(ctx, onlyPending(t, a).Request, tt.approveAt); err != nil {
				t.Fatalf("ApproveLink: %v", err)
			}
			if got := entered(t, reader, "Uma"); got != tt.wantEntered {
				t.Errorf("Uma entered = %v, want %v", got, tt.wantEntered)
			}
			if got, want := len(pendingNames(t, a)), map[bool]int{true: 0, false: 1}[tt.wantEntered]; got != want {
				t.Errorf("pending sign-ups = %d, want %d", got, want)
			}
		})
	}
}

func TestSigningInLinkedByDiscordNameEntersAPendingSignUp(t *testing.T) {
	a, p, reader, _ := signUpsFixture(t)
	ctx := context.Background()
	openSignUps(t, a, nil)
	if _, err := p.SignUp(ctx, tokenHash("uma"), "us_open", "2026", "Uma", signUpAt); err != nil {
		t.Fatalf("SignUp: %v", err)
	}
	if _, err := a.AddMember(ctx, Member{Name: "Uma", DiscordName: "uma", Zone: "UTC"}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if _, err := p.SignIn(ctx, discordID("100000000000000006", "uma", "Uma"), tokenHash("uma-again"), signUpAt); err != nil {
		t.Fatalf("SignIn: %v", err)
	}
	if !entered(t, reader, "Uma") {
		t.Errorf("Uma is not entered after her sign-in linked her, want entered")
	}
}

func TestAcceptingASignUpLinksAndEntersEvenOnceClosed(t *testing.T) {
	a, p, reader, _ := signUpsFixture(t)
	ctx := context.Background()
	openSignUps(t, a, nil)
	if _, err := p.SignUp(ctx, tokenHash("uma"), "us_open", "2026", "Uma", signUpAt); err != nil {
		t.Fatalf("SignUp: %v", err)
	}
	if err := a.SetSignUps(ctx, "us_open", "2026", SignUps{}, signUpAt); err != nil {
		t.Fatalf("SetSignUps(closed): %v", err)
	}
	s := onlyPending(t, a)
	if err := a.AcceptSignUp(ctx, "us_open", "2026", s.ID, signUpAt); err != nil {
		t.Fatalf("AcceptSignUp: %v", err)
	}
	if !entered(t, reader, "Uma") {
		t.Errorf("Uma is not entered, want entered")
	}
	var status string
	if err := a.pool.QueryRow(ctx, "SELECT status FROM link_requests WHERE request_id = $1", s.Request).Scan(&status); err != nil || status != "approved" {
		t.Errorf("the link request is %q (%v), want approved", status, err)
	}
	if got := pendingNames(t, a); len(got) != 0 {
		t.Errorf("pending sign-ups = %v, want none", got)
	}
}

func TestAcceptingASignUpOfALinkedAccountEntersIt(t *testing.T) {
	a, p, reader, _ := signUpsFixture(t)
	ctx := context.Background()
	openSignUps(t, a, nil)
	if _, err := p.SignUp(ctx, tokenHash("uma"), "us_open", "2026", "Uma", signUpAt); err != nil {
		t.Fatalf("SignUp: %v", err)
	}
	if err := a.SetSignUps(ctx, "us_open", "2026", SignUps{}, signUpAt); err != nil {
		t.Fatalf("SetSignUps(closed): %v", err)
	}
	s := onlyPending(t, a)
	if _, err := a.ApproveLink(ctx, s.Request, signUpAt); err != nil {
		t.Fatalf("ApproveLink: %v", err)
	}
	if got := onlyPending(t, a).Linked; got != "Uma" {
		t.Errorf("the pending sign-up is linked to %q, want Uma", got)
	}
	if err := a.AcceptSignUp(ctx, "us_open", "2026", s.ID, signUpAt); err != nil {
		t.Fatalf("AcceptSignUp: %v", err)
	}
	if !entered(t, reader, "Uma") {
		t.Errorf("Uma is not entered, want entered")
	}
}

func TestAcceptingASignUpIsRefusedWhenItShouldBe(t *testing.T) {
	tests := []struct {
		name  string
		setUp func(t *testing.T, a *AdminStore, s PendingSignUp) int64
		want  error
	}{
		{"its link request refused", func(t *testing.T, a *AdminStore, s PendingSignUp) int64 {
			if err := a.RefuseLink(context.Background(), s.Request, "", signUpAt); err != nil {
				t.Fatalf("RefuseLink: %v", err)
			}
			return s.ID
		}, ErrNoRequest},
		{"no such sign-up", func(_ *testing.T, _ *AdminStore, s PendingSignUp) int64 { return s.ID + 1000 }, ErrNoSignUp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, p, reader, _ := signUpsFixture(t)
			ctx := context.Background()
			openSignUps(t, a, nil)
			if _, err := p.SignUp(ctx, tokenHash("uma"), "us_open", "2026", "Uma", signUpAt); err != nil {
				t.Fatalf("SignUp: %v", err)
			}
			id := tt.setUp(t, a, onlyPending(t, a))
			if err := a.AcceptSignUp(ctx, "us_open", "2026", id, signUpAt); !errors.Is(err, tt.want) {
				t.Errorf("AcceptSignUp = %v, want %v", err, tt.want)
			}
			if entered(t, reader, "Uma") {
				t.Errorf("Uma is entered, want not")
			}
		})
	}
}

func TestRemovingASignUpDeletesIt(t *testing.T) {
	a, p, _, _ := signUpsFixture(t)
	ctx := context.Background()
	openSignUps(t, a, nil)
	if _, err := p.SignUp(ctx, tokenHash("uma"), "us_open", "2026", "Uma", signUpAt); err != nil {
		t.Fatalf("SignUp: %v", err)
	}
	id := onlyPending(t, a).ID
	if err := a.RemoveSignUp(ctx, "us_open", "2026", id); err != nil {
		t.Fatalf("RemoveSignUp: %v", err)
	}
	if got := pendingNames(t, a); len(got) != 0 {
		t.Errorf("pending sign-ups = %v, want none", got)
	}
	if err := a.RemoveSignUp(ctx, "us_open", "2026", id); !errors.Is(err, ErrNoSignUp) {
		t.Errorf("removing it again = %v, want %v", err, ErrNoSignUp)
	}
}

func TestOpeningSignUpsIsRefusedWhenItShouldBe(t *testing.T) {
	before := signInAt
	tests := []struct {
		name          string
		slug, edition string
		setUp         func(t *testing.T, a *AdminStore, ids []int64)
		closes        *time.Time
		want          error
	}{
		{"a tournament that is not blitz", "worlds", "2025", func(*testing.T, *AdminStore, []int64) {}, nil, ErrSignUpsNotOffered},
		{"the edition hidden", "us_open", "2026", func(t *testing.T, a *AdminStore, _ []int64) {
			hidden := false
			if err := a.SetTournament(context.Background(), "us_open", TournamentSettings{Visible: &hidden}); err != nil {
				t.Fatalf("SetTournament(hidden): %v", err)
			}
		}, nil, ErrSignUpsHidden},
		{"round 1 paired", "us_open", "2026", pairRoundOne, nil, ErrSignUpsStarted},
		{"a closing time not after now", "us_open", "2026", func(*testing.T, *AdminStore, []int64) {}, &before, ErrBadSignUps},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _, _, ids := signUpsFixture(t)
			tt.setUp(t, a, ids)
			err := a.SetSignUps(context.Background(), tt.slug, tt.edition, SignUps{Open: true, ClosesAt: tt.closes}, signInAt)
			if !errors.Is(err, tt.want) {
				t.Errorf("SetSignUps = %v, want %v", err, tt.want)
			}
			if got := signUpPosts(t, a); got != 0 {
				t.Errorf("sign-up news queued = %d, want 0", got)
			}
		})
	}
}

func TestClosingSignUpsIsNeverRefused(t *testing.T) {
	a, _, _, ids := signUpsFixture(t)
	openSignUps(t, a, nil)
	pairRoundOne(t, a, ids)
	if err := a.SetSignUps(context.Background(), "us_open", "2026", SignUps{}, signInAt); err != nil {
		t.Errorf("closing after round 1 = %v, want nil", err)
	}
}

func TestSignUpsArePostedOnceWhenFirstOpened(t *testing.T) {
	a, _, _, _ := signUpsFixture(t)
	ctx := context.Background()
	later := signInAt.Add(48 * time.Hour)
	steps := []struct {
		name string
		s    SignUps
		want int
	}{
		{"opening", SignUps{Open: true}, 1},
		{"moving the closing time", SignUps{Open: true, ClosesAt: &later}, 1},
		{"closing", SignUps{}, 1},
		{"opening again", SignUps{Open: true, ClosesAt: &later}, 1},
	}
	for _, s := range steps {
		if err := a.SetSignUps(ctx, "us_open", "2026", s.s, signInAt); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if got := signUpPosts(t, a); got != s.want {
			t.Errorf("after %s, sign-up news queued = %d, want %d", s.name, got, s.want)
		}
	}
}

func TestTheSiteReadsWhetherSignUpsTakeEntriesNow(t *testing.T) {
	future := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		set        *SignUps
		wantOpen   bool
		wantTaking bool
	}{
		{"never opened", nil, false, false},
		{"open, no closing time", &SignUps{Open: true}, true, true},
		{"open, closing in the future", &SignUps{Open: true, ClosesAt: &future}, true, true},
		{"closed", &SignUps{}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _, reader, _ := signUpsFixture(t)
			ctx := context.Background()
			if tt.set != nil {
				if err := a.SetSignUps(ctx, "us_open", "2026", *tt.set, signInAt); err != nil {
					t.Fatalf("SetSignUps: %v", err)
				}
			}
			ed, err := reader.BlitzEdition(ctx, "us_open", "2026")
			if err != nil {
				t.Fatalf("BlitzEdition: %v", err)
			}
			if got := ed.SignUps; got.Open != tt.wantOpen || got.Taking != tt.wantTaking {
				t.Errorf("sign-ups = open %v taking %v, want open %v taking %v", got.Open, got.Taking, tt.wantOpen, tt.wantTaking)
			}
		})
	}
}

func TestTheSiteSeesSignUpsStopTakingEntriesAtTheClosingTime(t *testing.T) {
	a, _, reader, _ := signUpsFixture(t)
	ctx := context.Background()
	closes := signInAt.Add(time.Hour)
	openSignUps(t, a, &closes)
	ed, err := reader.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	if !ed.SignUps.Open || ed.SignUps.Taking || ed.SignUps.ClosesAt == nil || !ed.SignUps.ClosesAt.Equal(closes) {
		t.Errorf("sign-ups = %+v, want open, closing at %v, no longer taking entries", ed.SignUps, closes)
	}
}

func TestMigration101sDownTakesSignUpsAwayAndUpBringsThemBack(t *testing.T) {
	pool, a := blitzFixture(t)
	ctx := context.Background()
	shown := true
	if err := a.SetTournament(ctx, "us_open", TournamentSettings{Visible: &shown}); err != nil {
		t.Fatalf("SetTournament(visible): %v", err)
	}
	openSignUps(t, a, nil)
	present := func() []string {
		return rowsOf(t, pool, `SELECT table_name::text FROM information_schema.tables WHERE table_name = 'tournament_sign_ups'
			UNION ALL SELECT column_name::text FROM information_schema.columns WHERE table_name = 'tournaments' AND column_name LIKE 'sign_ups_%'
			UNION ALL SELECT proname::text FROM pg_proc WHERE proname IN ('sign_up', 'withdraw_sign_up', 'sign_ups_taking')`)
	}
	if got := present(); len(got) != 7 {
		t.Fatalf("migrated: %q, want the table, 3 columns and 3 functions", got)
	}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 100); return err }); err != nil {
		t.Fatalf("migrating down to 100: %v", err)
	}
	if got := present(); len(got) != 0 {
		t.Errorf("at 100, %q remain, want none", got)
	}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.Up(ctx); return err }); err != nil {
		t.Fatalf("migrating up again: %v", err)
	}
	if got := present(); len(got) != 7 {
		t.Errorf("up again: %q, want the table, 3 columns and 3 functions", got)
	}
}

func TestAdminReadsATournamentsSignUps(t *testing.T) {
	a, _, _, _ := signUpsFixture(t)
	ctx := context.Background()
	future := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	openSignUps(t, a, &future)
	got, err := a.SignUpsOf(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("SignUpsOf: %v", err)
	}
	if !got.Open || !got.Taking || got.ClosesAt == nil || !got.ClosesAt.Equal(future) {
		t.Errorf("SignUpsOf = %+v, want open and taking entries until %v", got, future)
	}
	if got, err := a.SignUpsOf(ctx, "worlds", "2025"); err != nil || got.Open || got.Taking {
		t.Errorf("SignUpsOf(worlds 2025) = %+v, %v, want closed", got, err)
	}
	if _, err := a.SignUpsOf(ctx, "us_open", "1999"); !errors.Is(err, ErrNoBlitzEdition) {
		t.Errorf("SignUpsOf(an unknown edition) = %v, want %v", err, ErrNoBlitzEdition)
	}
}

func TestOpenSignUpsAreSentToTheEndpointsTakingThem(t *testing.T) {
	a, _, _, _ := signUpsFixture(t)
	endpoint(t, a, "Blitz", 1, "blitz_sign_ups")
	endpoint(t, a, "Rounds", 2, "blitz_round")
	openSignUps(t, a, nil)
	if got, want := blitzSent(t, a), []string{"Blitz blitz_sign_ups us_open 2026 0"}; !slices.Equal(got, want) {
		t.Errorf("sent = %q, want %q", got, want)
	}
	if got := blitzSent(t, a); len(got) != 0 {
		t.Errorf("sent again = %q, want nothing", got)
	}
}

func TestTheSiteCountsSignUpsWaitingForALink(t *testing.T) {
	a, p, reader, _ := signUpsFixture(t)
	ctx := context.Background()
	openSignUps(t, a, nil)
	if _, err := p.SignUp(ctx, tokenHash("uma"), "us_open", "2026", "Uma", signUpAt); err != nil {
		t.Fatalf("SignUp: %v", err)
	}
	ed, err := reader.BlitzEdition(ctx, "us_open", "2026")
	if err != nil {
		t.Fatalf("BlitzEdition: %v", err)
	}
	if ed.SignUps.Waiting != 1 {
		t.Errorf("the site's waiting sign-ups = %d, want 1", ed.SignUps.Waiting)
	}
	if got, err := a.SignUpsOf(ctx, "us_open", "2026"); err != nil || got.Waiting != 1 {
		t.Errorf("admin's waiting sign-ups = %d, %v, want 1", got.Waiting, err)
	}
}
