package discordbot_test

import (
	"slices"
	"strings"
	"testing"

	"ttacommunity/backend/internal/discordbot"
)

func TestParseDice(t *testing.T) {
	tests := []struct {
		name, in string
		want     discordbot.Dice
	}{
		{"count and sides", "2d6", discordbot.Dice{Count: 2, Sides: 6}},
		{"one die by default", "d20", discordbot.Dice{Count: 1, Sides: 20}},
		{"capital D and spaces", " 3D8 + 2 ", discordbot.Dice{Count: 3, Sides: 8, Modifier: 2}},
		{"taken off", "1d6-1", discordbot.Dice{Count: 1, Sides: 6, Modifier: -1}},
		{"the most allowed", "100d1000+1000", discordbot.Dice{Count: 100, Sides: 1000, Modifier: 1000}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := discordbot.ParseDice(tt.in)
			if err != nil || got != tt.want {
				t.Errorf("ParseDice(%q) = %+v, %v; want %+v", tt.in, got, err, tt.want)
			}
		})
	}
}

func TestParseDiceRefuses(t *testing.T) {
	for _, in := range []string{"", "6", "d", "0d6", "101d6", "2d1", "2d1001", "2d6+1001", "2d6+", "2d6x", "abc", "-2d6", "2d6+1+1"} {
		t.Run(in, func(t *testing.T) {
			if got, err := discordbot.ParseDice(in); err == nil {
				t.Errorf("ParseDice(%q) = %+v, want it refused", in, got)
			}
		})
	}
}

func TestRollDiceRollsEachDieWithItsSides(t *testing.T) {
	var asked []int
	next := 0
	roll := func(sides int) int { asked = append(asked, sides); next++; return next }
	rolls := discordbot.RollDice(discordbot.Dice{Count: 3, Sides: 8}, roll)
	if !slices.Equal(rolls, []int{1, 2, 3}) || !slices.Equal(asked, []int{8, 8, 8}) {
		t.Errorf("rolls = %v with sides %v, want [1 2 3] with sides [8 8 8]", rolls, asked)
	}
}

func TestDiceTextTotalsTheRollsAndModifier(t *testing.T) {
	tests := []struct {
		name      string
		dice      discordbot.Dice
		rolls     []int
		wantTotal string
	}{
		{"added", discordbot.Dice{Count: 2, Sides: 6, Modifier: 1}, []int{3, 5}, "**9**"},
		{"taken off", discordbot.Dice{Count: 2, Sides: 6, Modifier: -2}, []int{1, 1}, "**0**"},
		{"one die", discordbot.Dice{Count: 1, Sides: 20}, []int{17}, "**17**"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := discordbot.DiceText(tt.dice, tt.rolls); !strings.Contains(got, tt.wantTotal) {
				t.Errorf("DiceText(%+v, %v) = %q, want the total %s", tt.dice, tt.rolls, got, tt.wantTotal)
			}
		})
	}
}
