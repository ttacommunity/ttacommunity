package blitz_test

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"ttacommunity/backend/internal/blitz"
)

var deadline = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func TestRoundTitleNamesKnockoutStages(t *testing.T) {
	tests := []struct {
		name                      string
		round, swiss, knockoutOf8 int
		want                      string
	}{
		{"a Swiss round", 3, 8, 8, "Round 3"},
		{"the knockout's first round of 8", 9, 8, 8, "Round 9 · Quarter-finals"},
		{"the semi-finals", 10, 8, 8, "Round 10 · Semi-finals"},
		{"the final", 11, 8, 8, "Round 11 · Final"},
		{"a knockout of 16", 9, 8, 16, "Round 9 · Round of 16"},
		{"no knockout", 9, 8, 0, "Round 9"},
		{"past the final", 12, 8, 8, "Round 12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := blitz.RoundTitle(tt.round, tt.swiss, tt.knockoutOf8); got != tt.want {
				t.Errorf("RoundTitle(%d, %d, %d) = %q, want %q", tt.round, tt.swiss, tt.knockoutOf8, got, tt.want)
			}
		})
	}
}

func TestThreadsTextWritesEachGamesTitleThenItsMessage(t *testing.T) {
	got := blitz.ThreadsText(blitz.Threads{
		Round:      "Round 3",
		Deadline:   deadline,
		Organisers: []blitz.Person{{Name: "Carol", Discord: "carol_tta"}, {Name: "Dave", Discord: "dave.plays"}},
		Games: []blitz.ThreadGame{
			{Number: 1, A: blitz.Person{Name: "Ann", Discord: "ann_tta"}, B: blitz.Person{Name: "Bob", Discord: "bobplays"}},
			{Number: 3, A: blitz.Person{Name: "Eve", Discord: "eve"}, B: blitz.Person{Name: "Fay", Discord: "fay"}},
		},
	})
	message := func(a, b string) string {
		return "Welcome " + a + ", " + b + ". In this thread you can schedule your game with your opponent. " +
			"Please schedule and play your game by <t:1792411200:F>, then post the replay code or the outcome here. " +
			"If the game did not finish, or there are any issues with scheduling or otherwise, please ping the moderators Carol (@carol_tta) and/or Dave (@dave.plays).\n"
	}
	want := "Round 3 · Game 1 · Ann vs Bob\n" + message("@ann_tta", "@bobplays") + "\n" +
		"Round 3 · Game 3 · Eve vs Fay\n" + message("@eve", "@fay")
	if got != want {
		t.Errorf("ThreadsText =\n%s\nwant\n%s", got, want)
	}
}

func TestThreadsTextMentionsAPlayerByUsernameOrMarksThemMissing(t *testing.T) {
	tests := []struct {
		name, discord, want string
	}{
		{"a username", "ann_tta", "@ann_tta"},
		{"a username typed with capitals", "Ann_TTA", "@ann_tta"},
		{"a username typed with its @", "@ann.tta", "@ann.tta"},
		{"spaces around it", " ann_tta ", "@ann_tta"},
		{"no Discord name", "", "@??? (Ann)"},
		{"a display name with a space", "Ann Smith", "@??? (Ann)"},
		{"an old tag", "ann#1234", "@??? (Ann)"},
		{"one character", "a", "@??? (Ann)"},
		{"two dots together", "ann..tta", "@??? (Ann)"},
		{"over 32 characters", strings.Repeat("a", 33), "@??? (Ann)"},
		{"a letter outside a to z", "anné", "@??? (Ann)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blitz.ThreadsText(blitz.Threads{Round: "Round 1", Deadline: deadline, Organisers: []blitz.Person{{Name: "Carol", Discord: "carol"}},
				Games: []blitz.ThreadGame{{Number: 1, A: blitz.Person{Name: "Ann", Discord: tt.discord}, B: blitz.Person{Name: "Bob", Discord: "bob"}}}})
			if want := "Welcome " + tt.want + ", @bob."; !strings.Contains(got, want) {
				t.Errorf("Discord name %q: ThreadsText =\n%s\nwant it to hold %q", tt.discord, got, want)
			}
		})
	}
}

func TestThreadsTextNamesOrganisersWithTheirUsernames(t *testing.T) {
	tests := []struct {
		name       string
		organisers []blitz.Person
		want       string
	}{
		{"one", []blitz.Person{{Name: "Carol", Discord: "carol"}}, "please ping the moderator Carol (@carol).\n"},
		{"two, in order", []blitz.Person{{Name: "Dave", Discord: "dave"}, {Name: "Carol", Discord: "carol"}}, "please ping the moderators Dave (@dave) and/or Carol (@carol).\n"},
		{"three", []blitz.Person{{Name: "Dave", Discord: "dave"}, {Name: "Carol", Discord: "carol"}, {Name: "Eve", Discord: "eve"}},
			"please ping the moderators Dave (@dave), Carol (@carol), and Eve (@eve).\n"},
		{"four", []blitz.Person{{Name: "Dave", Discord: "dave"}, {Name: "Carol", Discord: "carol"}, {Name: "Eve", Discord: "eve"}, {Name: "Fay", Discord: "fay"}},
			"please ping the moderators Dave (@dave), Carol (@carol), Eve (@eve), and Fay (@fay).\n"},
		{"one without a username", []blitz.Person{{Name: "Carol", Discord: "Carol Smith"}}, "please ping the moderator Carol (@???).\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blitz.ThreadsText(blitz.Threads{Round: "Round 1", Deadline: deadline, Organisers: tt.organisers,
				Games: []blitz.ThreadGame{{Number: 1, A: blitz.Person{Name: "Ann", Discord: "ann"}, B: blitz.Person{Name: "Bob", Discord: "bob"}}}})
			if !strings.Contains(got, tt.want) {
				t.Errorf("organisers %v: ThreadsText =\n%s\nwant it to hold %q", tt.organisers, got, tt.want)
			}
		})
	}
}

func TestThreadsTextShortensATitleToDiscordsLimit(t *testing.T) {
	long := strings.Repeat("x", 60)
	got := blitz.ThreadsText(blitz.Threads{Round: "Round 9 · Quarter-finals", Deadline: deadline, Organisers: []blitz.Person{{Name: "Carol", Discord: "carol"}},
		Games: []blitz.ThreadGame{{Number: 12, A: blitz.Person{Name: "Ann" + long, Discord: "ann"}, B: blitz.Person{Name: "Bob" + long, Discord: "bob"}}}})
	title, _, _ := strings.Cut(got, "\n")
	if n := utf8.RuneCountInString(title); n > 100 {
		t.Errorf("title %q has %d characters, want at most 100", title, n)
	}
	if !strings.HasPrefix(title, "Round 9 · Quarter-finals · Game 12 · Annxx") || !strings.Contains(title, "… vs Bobxx") || !strings.HasSuffix(title, "…") {
		t.Errorf("title = %q, want the round and game whole, and both names shortened with …", title)
	}
}
