//go:build integration

package store

import (
	"context"
	"testing"
)

func TestDiscordMemberSaysWhatADiscordAccountIsToUs(t *testing.T) {
	const asked = "100000000000000001"
	tests := []struct {
		name      string
		setup     func(t *testing.T, a *AdminStore, p *PlayerStore)
		wantState string
		wantName  string
	}{
		{"never signed in", nil, DiscordNoLogin, ""},
		{"signed in with another provider under the same id", func(t *testing.T, _ *AdminStore, p *PlayerStore) {
			signedIn(t, p, Identity{Provider: "microsoft", Subject: asked, Username: "Player One"}, "token")
		}, DiscordNoLogin, ""},
		{"another Discord account signed in", func(t *testing.T, _ *AdminStore, p *PlayerStore) {
			signedIn(t, p, discordID("100000000000000002", "player_two", ""), "token")
		}, DiscordNoLogin, ""},
		{"signed in, not linked", func(t *testing.T, _ *AdminStore, p *PlayerStore) {
			signedIn(t, p, discordID(asked, "player_one", ""), "token")
		}, DiscordNotLinked, ""},
		{"linked, then unlinked by an admin", func(t *testing.T, a *AdminStore, p *PlayerStore) {
			l := signedIn(t, p, discordID(asked, "player_one", ""), "token")
			approved(t, a, p, l, "Bob")
			if err := a.UnlinkAccount(context.Background(), l.ID); err != nil {
				t.Fatalf("UnlinkAccount: %v", err)
			}
		}, DiscordNotLinked, ""},
		{"linked to a member the ratings do not know", func(t *testing.T, a *AdminStore, p *PlayerStore) {
			if _, err := a.AddMember(context.Background(), Member{Name: "ann", Zone: "UTC"}); err != nil {
				t.Fatalf("AddMember: %v", err)
			}
			approved(t, a, p, signedIn(t, p, discordID(asked, "player_one", ""), "token"), "ann")
		}, DiscordNoPlayer, ""},
		{"linked to a rated player", func(t *testing.T, a *AdminStore, p *PlayerStore) {
			approved(t, a, p, signedIn(t, p, discordID(asked, "player_one", ""), "token"), "bob")
		}, DiscordFound, "Bob"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, a, p := playersFixture(t)
			if tt.setup != nil {
				tt.setup(t, a, p)
			}
			got, err := p.DiscordMember(context.Background(), asked)
			if err != nil {
				t.Fatalf("DiscordMember: %v", err)
			}
			if got.State != tt.wantState || got.Name != tt.wantName {
				t.Errorf("DiscordMember(%s) = %+v, want state %q, name %q", asked, got, tt.wantState, tt.wantName)
			}
		})
	}
}
