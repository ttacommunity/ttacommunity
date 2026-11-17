package discordbot_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/discordbot"
)

var upsetDay = time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)

// pair is name's pair with other in game, expected to score expected and scoring score.
func pair(game int64, players int, name, other string, expected, score float64) discordbot.Pair {
	return discordbot.Pair{GameID: game, Date: upsetDay, Tournament: "Cup", PlayerCount: players, Name: name, Other: other,
		Rating: 1800, OtherRating: 1500, Expected: expected, Score: score}
}

// shown writes each upset as "game name expected/actual of opponents, behind whom".
func shown(upsets []discordbot.Upset) []string {
	var got []string
	for _, u := range upsets {
		var ahead []string
		for _, o := range u.Ahead {
			ahead = append(ahead, o.Name)
		}
		got = append(got, fmt.Sprintf("%d %s %.2f/%.1f of %d, behind %s", u.GameID, u.Name, u.Expected, u.Actual, u.Opponents, strings.Join(ahead, ",")))
	}
	return got
}

func TestUpsetsRankByShortfallPerOpponent(t *testing.T) {
	pairs := []discordbot.Pair{
		pair(1, 2, "fav", "dog", 0.96, 0), pair(1, 2, "dog", "fav", 0.04, 1),
		pair(2, 4, "big", "a", 0.9, 0), pair(2, 4, "big", "b", 0.85, 0), pair(2, 4, "big", "c", 0.85, 0),
		pair(3, 3, "mid", "x", 0.7, 1), pair(3, 3, "mid", "y", 0.8, 0),
	}
	want := []string{"1 fav 0.96/0.0 of 1, behind dog", "2 big 2.60/0.0 of 3, behind a,b,c", "3 mid 1.50/1.0 of 2, behind y"}
	if got := shown(discordbot.Upsets(pairs, 5)); !slices.Equal(got, want) {
		t.Errorf("Upsets = %q, want %q", got, want)
	}
}

func TestUpsetsLeaveOut(t *testing.T) {
	timedOut := pair(1, 2, "fav", "dog", 0.96, 0)
	timedOut.TimedOut = true
	tests := []struct {
		name  string
		pairs []discordbot.Pair
	}{
		{"a favourite who timed out", []discordbot.Pair{timedOut}},
		{"a favourite who won, and the underdog who lost", []discordbot.Pair{pair(1, 2, "fav", "dog", 0.6, 1), pair(1, 2, "dog", "fav", 0.4, 0)}},
		{"a player beaten only by those they were expected to lose to", []discordbot.Pair{pair(1, 3, "mid", "top", 0.3, 0), pair(1, 3, "mid", "low", 0.7, 1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := discordbot.Upsets(tt.pairs, 5); len(got) != 0 {
				t.Errorf("Upsets = %q, want none", shown(got))
			}
		})
	}
}

func TestUpsetsCountAFavouritesDraw(t *testing.T) {
	got := discordbot.Upsets([]discordbot.Pair{pair(1, 2, "fav", "dog", 0.9, 0.5), pair(1, 2, "dog", "fav", 0.1, 0.5)}, 5)
	if len(got) != 1 || got[0].Name != "fav" || len(got[0].Level) != 1 {
		t.Errorf("Upsets = %+v, want fav alone, level with dog", got)
	}
	if text := discordbot.UpsetsText(upsetDay, upsetDay, 0, got); !strings.Contains(text, "drew with **dog**") {
		t.Errorf("UpsetsText = %q, want it to say fav drew with dog", text)
	}
}

func TestUpsetsKeepTheTopN(t *testing.T) {
	var pairs []discordbot.Pair
	for g := range int64(7) {
		pairs = append(pairs, pair(g, 2, fmt.Sprintf("fav%d", g), "dog", 0.5+float64(g)/20, 0))
	}
	got := discordbot.Upsets(pairs, 5)
	if len(got) != 5 || got[0].Name != "fav6" || got[4].Name != "fav2" {
		t.Errorf("Upsets = %q, want fav6 down to fav2", shown(got))
	}
}

func TestUpsetsText(t *testing.T) {
	from, to := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	duel := discordbot.Upsets([]discordbot.Pair{pair(1, 2, "Fav_1", "Dog", 0.96, 0)}, 5)
	four := discordbot.Upsets([]discordbot.Pair{pair(2, 4, "Big", "a", 0.9, 0), pair(2, 4, "Big", "b", 0.85, 0), pair(2, 4, "Big", "c", 0.85, 1)}, 5)
	tests := []struct {
		name   string
		upsets []discordbot.Upset
		format int
		want   []string
	}{
		{"a duel: the loser's chance", duel, 0, []string{`**Fav\_1** (1800) lost to **Dog** (1500)`, "96% to win", "2026-10-04 to 2026-11-03"}},
		{"a table: opponents beaten against expected", four, 0, []string{"beat 1 of 3, expected 2.6", "**a** (1500), **b** (1500)"}},
		{"a table with opponents not counted", discordbot.Upsets([]discordbot.Pair{pair(3, 4, "Big", "a", 0.93, 0)}, 5), 0,
			[]string{"beat 0 of 1, expected 0.9 (2 not counted)", "behind **a** (1500)"}},
		{"none in a format", nil, 3, []string{"No upsets in rated 3-player games"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := discordbot.UpsetsText(from, to, tt.format, tt.upsets)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("UpsetsText = %q, want it to contain %q", got, w)
				}
			}
		})
	}
}

func TestUpsetsTextLeavesOutWhatDoesNotFitInAMessage(t *testing.T) {
	long := strings.Repeat("N", 20)
	var upsets []discordbot.Upset
	for range 10 {
		o := discordbot.Opponent{Name: long, Rating: 1800}
		upsets = append(upsets, discordbot.Upset{Tournament: strings.Repeat("T", 40), PlayerCount: 4, Name: long, Rating: 1500,
			Expected: 2.5, Opponents: 3, Ahead: []discordbot.Opponent{o, o, o}, Date: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)})
	}
	from, to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	text := discordbot.UpsetsText(from, to, 0, upsets)
	if n := len([]rune(text)); n > discordbot.MaxReply || !strings.Contains(text, "more that do not fit") {
		t.Errorf("UpsetsText of 10 long upsets is %d characters, want at most %d, saying how many it left out", n, discordbot.MaxReply)
	}
	if short := discordbot.UpsetsText(from, to, 0, upsets[:2]); strings.Contains(short, "more that do not fit") {
		t.Errorf("UpsetsText of 2 upsets = %q, want all of them", short)
	}
}
