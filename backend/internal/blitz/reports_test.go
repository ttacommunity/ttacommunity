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

func TestWhatAPlayerMayDoAboutBestOfThree(t *testing.T) {
	tests := []struct {
		name  string
		match blitz.BestOfMatch
		want  []string
	}{
		{"a round of one-game matches", blitz.BestOfMatch{Me: "a", BestOf: 1}, nil},
		{"nothing proposed", blitz.BestOfMatch{Me: "a", Offered: true, BestOf: 1}, []string{"propose"}},
		{"their own proposal", blitz.BestOfMatch{Me: "a", Offered: true, BestOf: 1, ProposedBy: "a"}, []string{"withdraw"}},
		{"the opponent's proposal", blitz.BestOfMatch{Me: "b", Offered: true, BestOf: 1, ProposedBy: "a"}, []string{"accept", "decline"}},
		{"agreed", blitz.BestOfMatch{Me: "a", Offered: true, BestOf: 3}, nil},
		{"game 1 played", blitz.BestOfMatch{Me: "a", Offered: true, BestOf: 1, Played: 1, Decided: true}, nil},
		{"an organiser's ruling", blitz.BestOfMatch{Me: "a", Offered: true, BestOf: 1, Decided: true}, nil},
		{"a bye", blitz.BestOfMatch{Me: "a", Offered: true, BestOf: 1, Bye: true}, nil},
		{"the next round paired", blitz.BestOfMatch{Me: "a", Offered: true, BestOf: 1, Closed: true}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := blitz.BestOfActions(tt.match); !slices.Equal(got, tt.want) {
				t.Errorf("BestOfActions(%+v) = %q, want %q", tt.match, got, tt.want)
			}
		})
	}
}

func TestWhichGameAPlayerMayReportNext(t *testing.T) {
	tests := []struct {
		name  string
		match blitz.BestOfMatch
		want  int
	}{
		{"a one-game match not played", blitz.BestOfMatch{Me: "a", BestOf: 1}, 1},
		{"a one-game match played", blitz.BestOfMatch{Me: "a", BestOf: 1, Played: 1, Decided: true}, 0},
		{"a best of 3 after game 1", blitz.BestOfMatch{Me: "a", BestOf: 3, Played: 1}, 2},
		{"a best of 3 at one each", blitz.BestOfMatch{Me: "a", BestOf: 3, Played: 2}, 3},
		{"a best of 3 decided in two", blitz.BestOfMatch{Me: "a", BestOf: 3, Played: 2, Decided: true}, 0},
		{"a best of 3 played out", blitz.BestOfMatch{Me: "a", BestOf: 3, Played: 3, Decided: true}, 0},
		{"an organiser's ruling", blitz.BestOfMatch{Me: "a", BestOf: 3, Decided: true}, 0},
		{"a bye", blitz.BestOfMatch{Me: "a", BestOf: 1, Bye: true}, 0},
		{"the next round paired", blitz.BestOfMatch{Me: "a", BestOf: 3, Played: 1, Closed: true}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := blitz.NextGame(tt.match); got != tt.want {
				t.Errorf("NextGame(%+v) = %d, want %d", tt.match, got, tt.want)
			}
		})
	}
}
