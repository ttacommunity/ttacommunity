package challenges_test

import (
	"testing"

	"ttacommunity/backend/internal/challenges"
)

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
	if got[0].ProofURL != "https://discordapp.com/channels/358698681672925195/1/2" {
		t.Errorf("proof = %q, want it trimmed and otherwise as typed: the database checks it", got[0].ProofURL)
	}
}
