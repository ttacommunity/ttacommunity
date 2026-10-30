//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/challenges"
)

// submitting is a database with challenges 1ai and 2ai, an old entry, Ann signed in and linked (session "ann"), and
// Bob signed in but not linked (session "bob").
func submitting(t *testing.T) (*pgxpool.Pool, *PlayerStore, *RatingsReader) {
	t.Helper()
	pool, a, p := playersFixture(t)
	withChallengeEntries(t, pool, imported(1, "Old", 40, 1))
	approved(t, a, p, signedIn(t, p, discordID("100000000000000001", "ann", "Ann"), "ann"), "Ann")
	signedIn(t, p, discordID("100000000000000002", "bob", "Bob"), "bob")
	return pool, p, NewRatingsReader(asReader(t, pool))
}

func TestALinkedPlayerSubmitsEntries(t *testing.T) {
	_, p, r := submitting(t)
	ctx := context.Background()
	proof := "https://discord.com/channels/358698681672925195/1/2"
	saved, err := p.SubmitChallengeEntries(ctx, tokenHash("ann"), []challenges.Submission{{Challenge: "1ai", Domination: 90, ProofURL: proof}, {Challenge: "2ai", Domination: -5}}, signInAt)
	if err != nil || saved != 2 {
		t.Fatalf("SubmitChallengeEntries = %d, %v, want 2 saved", saved, err)
	}
	entries, err := r.ChallengeEntries(ctx)
	if err != nil {
		t.Fatalf("ChallengeEntries: %v", err)
	}
	var mine []ChallengeEntry
	for _, e := range entries {
		if e.Member != nil {
			mine = append(mine, e)
		}
	}
	if len(mine) != 2 || mine[0].Player != "Ann" || mine[0].ProofURL != proof || mine[1].ProofURL != "" || !mine[0].At.Equal(signInAt) {
		t.Errorf("Ann's entries = %+v, want 1ai with its proof and 2ai without, under her member, at the submission", mine)
	}
}

func TestSubmittingIsRefusedWhenItShouldBe(t *testing.T) {
	ok := []challenges.Submission{{Challenge: "1ai", Domination: 90}}
	tests := []struct {
		name    string
		session string
		entries []challenges.Submission
		want    error
	}{
		{"no session", "nobody", ok, ErrNoSession},
		{"not linked", "bob", ok, ErrNotLinked},
		{"an unknown challenge", "ann", []challenges.Submission{{Challenge: "9ai", Domination: 1}}, ErrBadChallengeEntries},
		{"a challenge twice", "ann", []challenges.Submission{ok[0], ok[0]}, ErrBadChallengeEntries},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, p, r := submitting(t)
			_, err := p.SubmitChallengeEntries(context.Background(), tokenHash(tt.session), tt.entries, signInAt)
			if !errors.Is(err, tt.want) {
				t.Errorf("SubmitChallengeEntries = %v, want %v", err, tt.want)
			}
			if entries, _ := r.ChallengeEntries(context.Background()); len(entries) != 1 {
				t.Errorf("%d entries kept, want only the old one", len(entries))
			}
		})
	}
}

func TestReimportingKeepsEntriesMadeHere(t *testing.T) {
	pool, p, r := submitting(t)
	ctx := context.Background()
	if _, err := p.SubmitChallengeEntries(ctx, tokenHash("ann"), []challenges.Submission{{Challenge: "1ai", Domination: 90}}, signInAt); err != nil {
		t.Fatalf("SubmitChallengeEntries: %v", err)
	}
	if _, err := ImportChallengeEntries(ctx, pool, []challenges.ImportedEntry{imported(2, "Old", 45, 2)}); err != nil {
		t.Fatalf("ImportChallengeEntries: %v", err)
	}
	entries, err := r.ChallengeEntries(ctx)
	if err != nil {
		t.Fatalf("ChallengeEntries: %v", err)
	}
	if len(entries) != 2 || entries[0].Player != "Ann" || entries[1].Domination != 45 {
		t.Errorf("entries = %+v, want Ann's kept and the old one replaced", entries)
	}
}

func TestAProofIsKeptAsTheOneLinkOrRefused(t *testing.T) {
	const message = "https://discord.com/channels/358698681672925195/111/222"
	tests := []struct {
		name, raw, want string
		err             error
	}{
		{"a message in the TtA server", message, message, nil},
		{"spaces and a trailing slash", "  " + message + "/ ", message, nil},
		{"the old domain and the canary app", "https://canary.discordapp.com/channels/358698681672925195/111/222", message, nil},
		{"none", "", "", nil},
		{"another server", "https://discord.com/channels/1/111/222", "", ErrBadProofURL},
		{"a channel, not a message", "https://discord.com/channels/358698681672925195/111", "", ErrBadProofURL},
		{"another site", "https://example.com/channels/358698681672925195/111/222", "", ErrBadProofURL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, p, r := submitting(t)
			ctx := context.Background()
			_, err := p.SubmitChallengeEntries(ctx, tokenHash("ann"), []challenges.Submission{{Challenge: "1ai", Domination: 90, ProofURL: tt.raw}}, signInAt)
			if !errors.Is(err, tt.err) {
				t.Fatalf("SubmitChallengeEntries(%q) = %v, want %v", tt.raw, err, tt.err)
			}
			entries, _ := r.ChallengeEntries(ctx)
			if tt.err != nil {
				if len(entries) != 1 {
					t.Errorf("%d entries kept, want only the old one", len(entries))
				}
				return
			}
			if len(entries) != 2 || entries[1].ProofURL != tt.want {
				t.Errorf("the proof kept for %q = %q, want %q", tt.raw, entries[len(entries)-1].ProofURL, tt.want)
			}
		})
	}
}
