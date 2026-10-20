//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// memberID returns the id of the member named name, failing the test when there is none.
func memberID(t *testing.T, a *AdminStore, name string) int64 {
	t.Helper()
	members, err := a.Members(context.Background())
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	for _, m := range members {
		if m.Name == name {
			return m.ID
		}
	}
	t.Fatalf("no member named %s", name)
	return 0
}

// pendingRequest returns the id of the pending request of l's account, failing the test when it has none.
func pendingRequest(t *testing.T, a *AdminStore, l Account) int64 {
	t.Helper()
	rows, err := a.Accounts(context.Background())
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	for _, r := range rows {
		if r.ID == l.ID && r.Request != nil {
			return r.Request.ID
		}
	}
	t.Fatalf("account %d has no pending request", l.ID)
	return 0
}

// approved has l's account ask for name, and approves it.
func approved(t *testing.T, a *AdminStore, p *PlayerStore, l Account, name string) {
	t.Helper()
	if err := p.RequestLink(context.Background(), l.ID, name, signInAt); err != nil {
		t.Fatalf("RequestLink(%s): %v", name, err)
	}
	if _, err := a.ApproveLink(context.Background(), pendingRequest(t, a, l), signInAt); err != nil {
		t.Fatalf("ApproveLink(%s): %v", name, err)
	}
}

func TestApprovingARequestLinksTheMemberItNames(t *testing.T) {
	tests := []struct {
		name, asked string
		wantMember  string
		wantCreated bool
	}{
		{"a member, in another case", "ANN", "ann", false},
		{"a name only the ratings know", "bob", "Bob", true},
		{"a name nobody knows yet", "Newcomer", "Newcomer", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, a, p := playersFixture(t)
			ctx := context.Background()
			if _, err := a.AddMember(ctx, Member{Name: "ann", Zone: "UTC"}); err != nil {
				t.Fatalf("AddMember: %v", err)
			}
			l := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "token")
			if err := p.RequestLink(ctx, l.ID, tt.asked, signInAt); err != nil {
				t.Fatalf("RequestLink: %v", err)
			}
			got, err := a.ApproveLink(ctx, pendingRequest(t, a, l), signInAt)
			if err != nil {
				t.Fatalf("ApproveLink: %v", err)
			}
			if got.Member != tt.wantMember || got.Created != tt.wantCreated {
				t.Errorf("ApproveLink(%s) = %+v, want member %s, created %v", tt.asked, got, tt.wantMember, tt.wantCreated)
			}
			now, err := p.Session(ctx, tokenHash("token"), signInAt)
			if err != nil {
				t.Fatalf("Session: %v", err)
			}
			if now.Member == nil || now.Member.Name != tt.wantMember || now.Request == nil || now.Request.Status != "approved" {
				t.Errorf("after approval, member %+v and request %+v, want %s and approved", now.Member, now.Request, tt.wantMember)
			}
		})
	}
}

func TestApprovingKeepsTheRequestersDiscordNameOnANewMember(t *testing.T) {
	_, a, p := playersFixture(t)
	l := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "token")
	approved(t, a, p, l, "Newcomer")
	members, err := a.Members(context.Background())
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	if len(members) != 1 || members[0].DiscordName != "player_one" || members[0].Zone != "Unknown" {
		t.Errorf("members = %+v, want Newcomer with Discord name player_one and zone Unknown", members)
	}
}

func TestApprovingIsRefused(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, a *AdminStore, p *PlayerStore, l Account) int64 // returns the request to approve
		want  error
	}{
		{"a request already decided", func(t *testing.T, a *AdminStore, p *PlayerStore, l Account) int64 {
			r := pendingRequest(t, a, l)
			if err := a.RefuseLink(context.Background(), r, "", signInAt); err != nil {
				t.Fatalf("RefuseLink: %v", err)
			}
			return r
		}, ErrNoRequest},
		{"a request that does not exist", func(*testing.T, *AdminStore, *PlayerStore, Account) int64 { return 999 }, ErrNoRequest},
		{"a member linked to another account", func(t *testing.T, a *AdminStore, p *PlayerStore, l Account) int64 {
			other := signedIn(t, p, discordID("100000000000000009", "other_account", ""), "other")
			approved(t, a, p, other, "ann")
			return pendingRequest(t, a, l)
		}, ErrMemberHasAccount},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, a, p := playersFixture(t)
			ctx := context.Background()
			if _, err := a.AddMember(ctx, Member{Name: "ann", Zone: "UTC"}); err != nil {
				t.Fatalf("AddMember: %v", err)
			}
			l := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "token")
			if err := p.RequestLink(ctx, l.ID, "ann", signInAt); err != nil {
				t.Fatalf("RequestLink: %v", err)
			}
			if _, err := a.ApproveLink(ctx, tt.setup(t, a, p, l), signInAt); !errors.Is(err, tt.want) {
				t.Errorf("ApproveLink = %v, want %v", err, tt.want)
			}
			if now, err := p.Session(ctx, tokenHash("token"), signInAt); err != nil || now.Member != nil {
				t.Errorf("after a refused approval, member %+v (err %v), want none", now.Member, err)
			}
		})
	}
}

func TestARefusalsReasonReachesThePlayerWhoMayAskAgain(t *testing.T) {
	_, a, p := playersFixture(t)
	ctx := context.Background()
	l := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "token")
	if err := p.RequestLink(ctx, l.ID, "ann", signInAt); err != nil {
		t.Fatalf("RequestLink: %v", err)
	}
	r := pendingRequest(t, a, l)
	if err := a.RefuseLink(ctx, r, strings.Repeat("x", 501), signInAt); !errors.Is(err, ErrBadReason) {
		t.Errorf("a 501-character reason = %v, want ErrBadReason", err)
	}
	if err := a.RefuseLink(ctx, r, " Message us on Discord first ", signInAt); err != nil {
		t.Fatalf("RefuseLink: %v", err)
	}
	got, err := p.Session(ctx, tokenHash("token"), signInAt)
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if got.Request == nil || got.Request.Status != "refused" || got.Request.Reason != "Message us on Discord first" {
		t.Errorf("request = %+v, want refused with the reason", got.Request)
	}
	if err := p.RequestLink(ctx, l.ID, "annie", signInAt); err != nil {
		t.Errorf("asking again after a refusal = %v, want nil", err)
	}
}

func TestLoginsShowARefusalUntilAskedAgainOrLinked(t *testing.T) {
	_, a, p := playersFixture(t)
	ctx := context.Background()
	l := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "token")
	refused := func(t *testing.T) *RefusedRequest {
		t.Helper()
		rows, err := a.Accounts(ctx)
		if err != nil {
			t.Fatalf("Accounts: %v", err)
		}
		for _, r := range rows {
			if r.ID == l.ID {
				return r.Refused
			}
		}
		t.Fatalf("account %d not listed", l.ID)
		return nil
	}
	if got := refused(t); got != nil {
		t.Errorf("never asked: refused = %+v, want none", got)
	}
	if err := p.RequestLink(ctx, l.ID, "ann", signInAt); err != nil {
		t.Fatalf("RequestLink: %v", err)
	}
	if err := a.RefuseLink(ctx, pendingRequest(t, a, l), "Not the same player", signInAt); err != nil {
		t.Fatalf("RefuseLink: %v", err)
	}
	if got := refused(t); got == nil || got.Name != "ann" || got.Reason != "Not the same player" {
		t.Errorf("refused: refused = %+v, want ann with the reason", got)
	}
	if err := p.RequestLink(ctx, l.ID, "annie", signInAt); err != nil {
		t.Fatalf("RequestLink again: %v", err)
	}
	if got := refused(t); got != nil {
		t.Errorf("asked again: refused = %+v, want none while the new request waits", got)
	}
	if err := a.RefuseLink(ctx, pendingRequest(t, a, l), "", signInAt); err != nil {
		t.Fatalf("RefuseLink again: %v", err)
	}
	if _, err := a.AddMember(ctx, Member{Name: "annie", DiscordName: "player_one", Zone: "UTC"}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	signedIn(t, p, discordID("100000000000000001", "player_one", ""), "linked")
	if got := refused(t); got != nil {
		t.Errorf("linked by Discord name since: refused = %+v, want none", got)
	}
}

func TestRemovingAMemberWithASignInSaysToUnlinkItFirst(t *testing.T) {
	_, a, p := playersFixture(t)
	ctx := context.Background()
	ann, err := a.AddMember(ctx, Member{Name: "ann", DiscordName: "player_one", Zone: "UTC"})
	if err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	l := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "token")
	if err := a.RemoveMember(ctx, ann); !errors.Is(err, ErrMemberSignedIn) {
		t.Errorf("RemoveMember(linked to a sign-in) = %v, want ErrMemberSignedIn", err)
	}
	if err := a.UnlinkAccount(ctx, l.ID); err != nil {
		t.Fatalf("UnlinkAccount: %v", err)
	}
	if err := a.RemoveMember(ctx, ann); err != nil {
		t.Errorf("RemoveMember(once unlinked) = %v, want nil", err)
	}
}

func TestLoginsShowNoRefusalForAnApprovedRequestSinceUnlinked(t *testing.T) {
	_, a, p := playersFixture(t)
	ctx := context.Background()
	l := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "token")
	if err := p.RequestLink(ctx, l.ID, "ann", signInAt); err != nil {
		t.Fatalf("RequestLink: %v", err)
	}
	if _, err := a.ApproveLink(ctx, pendingRequest(t, a, l), signInAt); err != nil {
		t.Fatalf("ApproveLink: %v", err)
	}
	if err := a.UnlinkAccount(ctx, l.ID); err != nil {
		t.Fatalf("UnlinkAccount: %v", err)
	}
	rows, err := a.Accounts(ctx)
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	for _, r := range rows {
		if r.ID == l.ID && r.Refused != nil {
			t.Errorf("approved, then unlinked: refused = %+v, want none", r.Refused)
		}
		if r.ID == l.ID && r.AutoLink {
			t.Errorf("unlinked by an admin: auto link = true, want false")
		}
	}
}

func TestLoginsSayASignInIsLinkedByDiscordNameUntilUnlinked(t *testing.T) {
	_, a, p := playersFixture(t)
	l := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "token")
	rows, err := a.Accounts(context.Background())
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != l.ID || !rows[0].AutoLink {
		t.Errorf("a new sign-in: rows = %+v, want it with auto link on", rows)
	}
}

func TestLoginsListsPendingRequestsFirstWithWhatTheyMatch(t *testing.T) {
	_, a, p := playersFixture(t)
	ctx := context.Background()
	for _, m := range []Member{{Name: "ann", DiscordName: "ann_d", Zone: "UTC"}, {Name: "cat", DiscordName: "player_three", Zone: "UTC"}} {
		if _, err := a.AddMember(ctx, m); err != nil {
			t.Fatalf("AddMember(%s): %v", m.Name, err)
		}
	}
	signedIn(t, p, discordID("100000000000000001", "player_one", "Player One"), "one")
	asking := signedIn(t, p, discordID("100000000000000002", "player_two", ""), "two")
	signedIn(t, p, discordID("100000000000000003", "player_three", ""), "three")
	if err := p.RequestLink(ctx, asking.ID, "ANN", signInAt); err != nil {
		t.Fatalf("RequestLink: %v", err)
	}
	nz, auckland := "NZ", "Pacific/Auckland"
	if err := p.SetOwnDetails(ctx, asking.ID, &nz, &auckland); err != nil {
		t.Fatalf("SetOwnDetails: %v", err)
	}
	rows, err := a.Accounts(ctx)
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	var got []string
	for _, r := range rows {
		line := fmt.Sprintf("%s %s member=%q how=%q", r.Logins[0].Provider, r.Logins[0].Username, r.Member, r.LinkedHow)
		if r.Request != nil {
			line += fmt.Sprintf(" asks=%s resolves=%q recorded=%q gave=%q/%q has=%q/%q", r.Request.Name, r.Request.Member, r.Request.MemberDiscord,
				r.Request.Country, r.Request.Zone, r.Request.MemberCountry, r.Request.MemberZone)
		}
		got = append(got, line)
	}
	want := []string{
		`discord player_two member="" how="" asks=ANN resolves="ann" recorded="ann_d" gave="NZ"/"Pacific/Auckland" has=""/"UTC"`,
		`discord player_one member="" how=""`,
		`discord player_three member="cat" how="match"`,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("logins (-want +got):\n%s", diff)
	}
}

func TestUnlinkingKeepsTheSignInAndRemovingEndsIt(t *testing.T) {
	pool, a, p := playersFixture(t)
	ctx := context.Background()
	if _, err := a.AddMember(ctx, Member{Name: "ann", DiscordName: "player_one", Zone: "UTC"}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	l := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "token")
	if err := a.UnlinkAccount(ctx, l.ID); err != nil {
		t.Fatalf("UnlinkAccount: %v", err)
	}
	if got, err := p.Session(ctx, tokenHash("token"), signInAt); err != nil || got.Member != nil {
		t.Errorf("after unlinking, member %+v (err %v), want signed in with none", got.Member, err)
	}
	if again := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "again"); again.Member != nil {
		t.Errorf("signing in after an unlink linked %+v again by Discord name, want none", again.Member)
	}
	if err := a.RemoveAccount(ctx, l.ID); err != nil {
		t.Fatalf("RemoveAccount: %v", err)
	}
	if _, err := p.Session(ctx, tokenHash("token"), signInAt); !errors.Is(err, ErrNoSession) {
		t.Errorf("after removing, Session = %v, want ErrNoSession", err)
	}
	var accounts int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM accounts").Scan(&accounts); err != nil || accounts != 0 {
		t.Errorf("accounts left after removing it = %d (%v), want 0", accounts, err)
	}
	for name, err := range map[string]error{"unlink": a.UnlinkAccount(ctx, l.ID), "remove": a.RemoveAccount(ctx, l.ID)} {
		if !errors.Is(err, ErrNoAccount) {
			t.Errorf("%s a removed account = %v, want ErrNoAccount", name, err)
		}
	}
}

func TestARequestNoLongerWaitsOnceItsSignInIsLinkedByName(t *testing.T) {
	_, a, p := playersFixture(t)
	ctx := context.Background()
	l := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "token")
	if err := p.RequestLink(ctx, l.ID, "Newcomer", signInAt); err != nil {
		t.Fatalf("RequestLink: %v", err)
	}
	if _, err := a.AddMember(ctx, Member{Name: "ann", DiscordName: "player_one", Zone: "UTC"}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	signedIn(t, p, discordID("100000000000000001", "player_one", ""), "again")
	rows, err := a.Accounts(ctx)
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	if len(rows) != 1 || rows[0].Member != "ann" || rows[0].Request != nil {
		t.Errorf("logins = %+v, want player_one linked to ann with nothing waiting", rows)
	}
}

func TestApprovingMovesTheDetailsGivenToTheMember(t *testing.T) {
	tests := []struct {
		name, asked        string
		give               bool
		wantMember         string
		wantCountry, wantZ string
	}{
		{"to a new member", "Newcomer", true, "Newcomer", "NZ", "Pacific/Auckland"},
		{"to a new member, nothing given", "Newcomer", false, "Newcomer", "", "Unknown"},
		{"replacing an existing member's", "ann", true, "ann", "NZ", "Pacific/Auckland"},
		{"an existing member's kept when nothing was given", "ann", false, "ann", "DE", "UTC"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, a, p := playersFixture(t)
			ctx := context.Background()
			if _, err := a.AddMember(ctx, Member{Name: "ann", Country: "DE", Zone: "UTC"}); err != nil {
				t.Fatalf("AddMember: %v", err)
			}
			l := signedIn(t, p, discordID("100000000000000001", "player_one", ""), "token")
			if tt.give {
				nz, auckland := "NZ", "Pacific/Auckland"
				if err := p.SetOwnDetails(ctx, l.ID, &nz, &auckland); err != nil {
					t.Fatalf("SetOwnDetails: %v", err)
				}
			}
			approved(t, a, p, l, tt.asked)
			members, err := a.Members(ctx)
			if err != nil {
				t.Fatalf("Members: %v", err)
			}
			for _, m := range members {
				if m.Name == tt.wantMember && (m.Country != tt.wantCountry || m.Zone != tt.wantZ) {
					t.Errorf("%s's country and zone = %q %q, want %q %q", m.Name, m.Country, m.Zone, tt.wantCountry, tt.wantZ)
				}
			}
			got, err := p.Session(ctx, tokenHash("token"), signInAt)
			if err != nil {
				t.Fatalf("Session: %v", err)
			}
			if got.Country != tt.wantCountry || got.Zone != tt.wantZ {
				t.Errorf("the player's country and zone = %q %q, want the member's %q %q", got.Country, got.Zone, tt.wantCountry, tt.wantZ)
			}
		})
	}
}

func TestApprovingLinksABannedMember(t *testing.T) {
	_, a, p := playersFixture(t)
	ctx := context.Background()
	ann, err := a.AddMember(ctx, Member{Name: "ann", Zone: "UTC"})
	if err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if err := a.BanMember(ctx, ann, "test"); err != nil {
		t.Fatalf("BanMember: %v", err)
	}
	l := signedIn(t, p, discordID("100000000000000002", "player_two", ""), "token")
	approved(t, a, p, l, "ann")
	if got := signedIn(t, p, discordID("100000000000000002", "player_two", ""), "again"); got.Member == nil || got.Member.ID != ann || !got.Banned {
		t.Errorf("after approving: member %+v, banned %v, want ann, banned", got.Member, got.Banned)
	}
}
