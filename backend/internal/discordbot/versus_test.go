package discordbot_test

import (
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/discordbot"
)

func TestTallyVersus(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	newestFirst := []discordbot.Meeting{
		{Date: day(9), PlayerCount: 3, NamePlace: 2, OtherPlace: 1},
		{Date: day(5), PlayerCount: 2, NamePlace: 1, OtherPlace: 2},
		{Date: day(3), PlayerCount: 2, NamePlace: 1, OtherPlace: 2},
		{Date: day(1), PlayerCount: 4, NamePlace: 2, OtherPlace: 2},
	}
	tests := []struct {
		name                       string
		format                     int
		games, wins, losses, draws int
		last                       time.Time
	}{
		{"every format", 0, 4, 2, 1, 1, day(9)},
		{"2-player only", 2, 2, 2, 0, 0, day(5)},
		{"a format they never met in", 3, 1, 0, 1, 0, day(9)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := discordbot.TallyVersus(newestFirst, tt.format)
			if got.Games != tt.games || got.Wins != tt.wins || got.Losses != tt.losses || got.Draws != tt.draws {
				t.Errorf("TallyVersus(format %d) = %d games %d-%d-%d, want %d games %d-%d-%d", tt.format,
					got.Games, got.Wins, got.Losses, got.Draws, tt.games, tt.wins, tt.losses, tt.draws)
			}
			if got.Last == nil || !got.Last.Date.Equal(tt.last) {
				t.Errorf("TallyVersus(format %d).Last = %+v, want the game of %s", tt.format, got.Last, tt.last.Format(time.DateOnly))
			}
		})
	}
}

func TestTallyVersusWithNoSharedGame(t *testing.T) {
	if got := discordbot.TallyVersus(nil, 0); got.Games != 0 || got.Last != nil {
		t.Errorf("TallyVersus(none) = %+v, want no games and no last meeting", got)
	}
}

func TestNoGamesTextNamesAFormatOnlyWhenOneIsAsked(t *testing.T) {
	tests := []struct {
		name   string
		format int
		want   string
	}{
		{"every format", 0, "have no rated games together"},
		{"one format", 3, "have no rated 3-player games together"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := discordbot.VersusText("Quill", "Rook", tt.format, discordbot.Tally{}, "https://site.example"); !strings.Contains(got, tt.want) {
				t.Errorf("VersusText(no games, format %d) = %q, want %q", tt.format, got, tt.want)
			}
		})
	}
}

func TestVersusTextNamesBothSafelyAndLinksTheFirst(t *testing.T) {
	tally := discordbot.TallyVersus([]discordbot.Meeting{{PlayerCount: 2, NamePlace: 1, OtherPlace: 2}}, 0)
	text := discordbot.VersusText("Player_One", "Two*", 0, tally, "https://site.example")
	for _, want := range []string{`Player\_One`, `Two\*`, "https://site.example/ratings/player/?name=Player_One"} {
		if !strings.Contains(text, want) {
			t.Errorf("VersusText = %q, want it to contain %q", text, want)
		}
	}
}
