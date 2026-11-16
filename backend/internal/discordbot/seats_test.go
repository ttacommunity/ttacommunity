package discordbot_test

import (
	"strings"
	"testing"

	"ttacommunity/backend/internal/discordbot"
)

func TestSeatsText(t *testing.T) {
	groups := []discordbot.SeatGroup{
		{PlayerCount: 2, Games: 170, WinRate: 66, Seats: []discordbot.SeatRow{{Seat: 1, Games: 85, WinRate: 68.24, Delta: 2.24}, {Seat: 2, Games: 85, WinRate: 63.8, Delta: -2.2}}},
		{PlayerCount: 3, Games: 40, WinRate: 40, Seats: []discordbot.SeatRow{{Seat: 1, Games: 14, WinRate: 50, Delta: 10}}},
	}
	tests := []struct {
		name          string
		format        int
		want, notWant string
	}{
		{"a seat above the player's own rate", 0, "seat 1 68.2% (+2.2)", ""},
		{"a seat below it", 0, "seat 2 63.8% (−2.2)", ""},
		{"every format", 0, "3-player", ""},
		{"one format alone", 2, "2-player", "3-player"},
		{"a format with no games", 4, "no rated 4-player games", "2-player"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := discordbot.SeatsText("Quill", tt.format, groups, "https://site.example")
			if !strings.Contains(got, tt.want) || (tt.notWant != "" && strings.Contains(got, tt.notWant)) {
				t.Errorf("SeatsText(format %d) = %q, want %q and no %q", tt.format, got, tt.want, tt.notWant)
			}
		})
	}
}
