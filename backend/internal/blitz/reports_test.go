package blitz_test

import (
	"slices"
	"testing"

	"ttacommunity/backend/internal/blitz"
)

func TestWhatAPlayerMayDoWithTheirMatch(t *testing.T) {
	tests := []struct {
		name  string
		match blitz.PlayerMatch
		want  []string
	}{
		{"not yet decided", blitz.PlayerMatch{Me: "a"}, []string{"report"}},
		{"their own report", blitz.PlayerMatch{Me: "a", Decided: true, Report: "reported", ReportedBy: "a"}, []string{"report"}},
		{"the opponent's report", blitz.PlayerMatch{Me: "b", Decided: true, Report: "reported", ReportedBy: "a"}, []string{"confirm", "flag"}},
		{"confirmed", blitz.PlayerMatch{Me: "a", Decided: true, Report: "confirmed", ReportedBy: "a"}, nil},
		{"flagged", blitz.PlayerMatch{Me: "b", Decided: true, Report: "flagged", ReportedBy: "a"}, nil},
		{"resolved", blitz.PlayerMatch{Me: "a", Decided: true, Report: "resolved", ReportedBy: "a"}, nil},
		{"an organiser's result", blitz.PlayerMatch{Me: "a", Decided: true}, nil},
		{"a bye", blitz.PlayerMatch{Me: "a", Bye: true}, nil},
		{"the next round paired", blitz.PlayerMatch{Me: "a", Closed: true}, nil},
		{"the opponent's report, the next round paired", blitz.PlayerMatch{Me: "b", Decided: true, Report: "reported", ReportedBy: "a", Closed: true}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := blitz.PlayerActions(tt.match); !slices.Equal(got, tt.want) {
				t.Errorf("PlayerActions(%+v) = %q, want %q", tt.match, got, tt.want)
			}
		})
	}
}
