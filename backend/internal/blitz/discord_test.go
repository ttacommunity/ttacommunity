package blitz_test

import (
	"testing"

	"ttacommunity/backend/internal/blitz"
)

func TestDiscordText(t *testing.T) {
	ann, bob := blitz.Person{Name: "Ann", Discord: "ann"}, blitz.Person{Name: "Bob", Discord: "bob"}
	cy, dee := blitz.Person{Name: "Cy", Discord: "cy_1"}, blitz.Person{Name: "Dee", Discord: "dee.d"}
	tests := []struct {
		name  string
		games [][2]blitz.Person
		byes  []blitz.Person
		want  string
	}{
		{"a line per game, a comma after all but the last", [][2]blitz.Person{{ann, bob}, {cy, dee}}, nil, "@ann vs @bob,\n@cy_1 vs @dee.d"},
		{"one game, no comma", [][2]blitz.Person{{ann, bob}}, nil, "@ann vs @bob"},
		{"no games", nil, nil, ""},
		{"a username written with @ and capitals", [][2]blitz.Person{{{Name: "Ann", Discord: " @Ann "}, bob}}, nil, "@ann vs @bob"},
		{"a display name: the TTA name instead", [][2]blitz.Person{{{Name: "Ann", Discord: "Ann Smith"}, bob}}, nil, "Ann vs @bob"},
		{"an old tag: the TTA name instead", [][2]blitz.Person{{ann, {Name: "Bob", Discord: "bob#1234"}}}, nil, "@ann vs Bob"},
		{"no Discord name: the TTA name", [][2]blitz.Person{{{Name: "Ann"}, bob}}, nil, "Ann vs @bob"},
		{"two dots together is no username", [][2]blitz.Person{{{Name: "Ann", Discord: "a..nn"}, bob}}, nil, "Ann vs @bob"},
		{"a Discord id: a mention by it", [][2]blitz.Person{{{Name: "Ann", Discord: "ann", DiscordID: "100000000000000031"}, bob}}, nil,
			"<@100000000000000031> vs @bob"},
		{"an id that is not one: the username", [][2]blitz.Person{{{Name: "Ann", Discord: "ann", DiscordID: "12"}, bob}}, nil, "@ann vs @bob"},
		{"the bye on a line of its own, last", [][2]blitz.Person{{ann, bob}}, []blitz.Person{cy}, "@ann vs @bob,\nBye: @cy_1"},
		{"only a bye", nil, []blitz.Person{{Name: "Cy"}}, "Bye: Cy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := blitz.DiscordText(tt.games, tt.byes); got != tt.want {
				t.Errorf("DiscordText = %q, want %q", got, tt.want)
			}
		})
	}
}
