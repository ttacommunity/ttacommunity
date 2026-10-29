package challenges_test

import (
	"testing"

	"ttacommunity/backend/internal/challenges"
)

func TestProofURL(t *testing.T) {
	const message = "https://discord.com/channels/358698681672925195/111/222"
	tests := []struct {
		name, raw, want string
		ok              bool
	}{
		{"a message in the TtA server", message, message, true},
		{"spaces and a trailing slash", "  " + message + "/ ", message, true},
		{"the old domain and the canary app", "https://canary.discordapp.com/channels/358698681672925195/111/222", message, true},
		{"none", "", "", true},
		{"another server", "https://discord.com/channels/1/111/222", "", false},
		{"a channel, not a message", "https://discord.com/channels/358698681672925195/111", "", false},
		{"another site", "https://example.com/channels/358698681672925195/111/222", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := challenges.ProofURL(tt.raw)
			if got != tt.want || ok != tt.ok {
				t.Errorf("ProofURL(%q) = %q, %v, want %q, %v", tt.raw, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestCheckSubmissions(t *testing.T) {
	ok := challenges.Submission{Challenge: "1ai", Domination: 50}
	tests := []struct {
		name    string
		entries []challenges.Submission
		bad     bool
	}{
		{"one entry", []challenges.Submission{ok}, false},
		{"none", nil, true},
		{"a challenge twice", []challenges.Submission{ok, ok}, true},
		{"a domination out of bounds", []challenges.Submission{{Challenge: "1ai", Domination: 10000}}, true},
		{"a proof from elsewhere", []challenges.Submission{{Challenge: "1ai", Domination: 5, ProofURL: "https://example.com/x"}}, true},
		{"an id no challenge has", []challenges.Submission{{Challenge: "Bad Id", Domination: 5}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, msg := challenges.CheckSubmissions(tt.entries)
			if (msg != "") != tt.bad {
				t.Errorf("CheckSubmissions refused %v (%q), want refused %v", msg != "", msg, tt.bad)
			}
		})
	}
	got, _ := challenges.CheckSubmissions([]challenges.Submission{{Challenge: "1ai", Domination: 5, ProofURL: " https://discordapp.com/channels/358698681672925195/1/2 "}})
	if got[0].ProofURL != "https://discord.com/channels/358698681672925195/1/2" {
		t.Errorf("proof = %q, want it written the one way", got[0].ProofURL)
	}
}
