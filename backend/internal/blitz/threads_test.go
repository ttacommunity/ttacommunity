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

func TestThreadsTextWritesEachGameAsABlockOfItsOwn(t *testing.T) {
	got := blitz.ThreadsText(blitz.Threads{
		Round:      "Round 3",
		Deadline:   deadline,
		Text:       blitz.DefaultThreadText,
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
	want := "---\nRound 3 · Game 1 · Ann vs Bob\n---\n" + message("@ann_tta", "@bobplays") + "---\n\n" +
		"---\nRound 3 · Game 3 · Eve vs Fay\n---\n" + message("@eve", "@fay") + "---\n"
	if got != want {
		t.Errorf("ThreadsText =\n%s\nwant\n%s", got, want)
	}
}

// oneGame is round 1's threads for Ann against Bob, with Carol to ping and the edition's text.
func oneGame(a, b blitz.Person, text blitz.ThreadText) blitz.Threads {
	return blitz.Threads{Round: "Round 1", Deadline: deadline, Text: text, Organisers: []blitz.Person{{Name: "Carol", Discord: "carol"}},
		Games: []blitz.ThreadGame{{Number: 1, A: a, B: b}}}
}

func TestThreadsTextFillsInTheEditionsOwnText(t *testing.T) {
	text := blitz.ThreadText{Message: "Hi {players}! Play by {deadline}: https://example.org/sheet\nGood luck {players}.", NoDiscord: "{name}."}
	got := blitz.ThreadsText(oneGame(blitz.Person{Name: "Ann", Discord: "ann"}, blitz.Person{Name: "Bob", Discord: "bob"}, text))
	want := "---\nRound 1 · Game 1 · Ann vs Bob\n---\nHi @ann, @bob! Play by <t:1792411200:F>: https://example.org/sheet\nGood luck @ann, @bob. " +
		"If the game did not finish, or there are any issues with scheduling or otherwise, please ping the moderator Carol (@carol).\n---\n"
	if got != want {
		t.Errorf("ThreadsText =\n%s\nwant\n%s", got, want)
	}
}

func TestThreadsTextMentionsAPlayerByUsernameOrWelcomesThemByName(t *testing.T) {
	tests := []struct {
		name, discord, want string
	}{
		{"a username", "ann_tta", "@ann_tta"},
		{"a username typed with capitals", "Ann_TTA", "@ann_tta"},
		{"a username typed with its @", "@ann.tta", "@ann.tta"},
		{"spaces around it", " ann_tta ", "@ann_tta"},
		{"no Discord name", "", "Ann"},
		{"a display name with a space", "Ann Smith", "Ann"},
		{"an old tag", "ann#1234", "Ann"},
		{"one character", "a", "Ann"},
		{"two dots together", "ann..tta", "Ann"},
		{"over 32 characters", strings.Repeat("a", 33), "Ann"},
		{"a letter outside a to z", "anné", "Ann"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blitz.ThreadsText(oneGame(blitz.Person{Name: "Ann", Discord: tt.discord}, blitz.Person{Name: "Bob", Discord: "bob"}, blitz.DefaultThreadText))
			if want := "\nWelcome " + tt.want + ", @bob."; !strings.Contains(got, want) {
				t.Errorf("Discord name %q: ThreadsText =\n%s\nwant it to hold %q", tt.discord, got, want)
			}
		})
	}
}

func TestThreadsTextTellsHowToReachEachPlayerWithoutAUsername(t *testing.T) {
	text := blitz.ThreadText{Message: "Welcome {players}.", NoDiscord: "{name} has no Discord: friend {name} in the app."}
	tests := []struct {
		name               string
		discordA, discordB string
		want               string
	}{
		{"both have one", "ann", "bob", ""},
		{"the first has none", "", "bob", "Ann has no Discord: friend Ann in the app.\n"},
		{"the second has a display name", "ann", "Bob Smith", "Bob has no Discord: friend Bob in the app.\n"},
		{"neither has one, in game order", "", "", "Ann has no Discord: friend Ann in the app.\nBob has no Discord: friend Bob in the app.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blitz.ThreadsText(oneGame(blitz.Person{Name: "Ann", Discord: tt.discordA}, blitz.Person{Name: "Bob", Discord: tt.discordB}, text))
			_, after, _ := strings.Cut(got, "the moderator Carol (@carol).\n")
			if extra := strings.TrimSuffix(after, "---\n"); extra != tt.want {
				t.Errorf("Discord names %q and %q: after the message = %q, want %q", tt.discordA, tt.discordB, extra, tt.want)
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
			threads := oneGame(blitz.Person{Name: "Ann", Discord: "ann"}, blitz.Person{Name: "Bob", Discord: "bob"}, blitz.DefaultThreadText)
			threads.Organisers = tt.organisers
			if got := blitz.ThreadsText(threads); !strings.Contains(got, tt.want) {
				t.Errorf("organisers %v: ThreadsText =\n%s\nwant it to hold %q", tt.organisers, got, tt.want)
			}
		})
	}
}

func TestThreadsTextShortensATitleToDiscordsLimit(t *testing.T) {
	long := strings.Repeat("x", 60)
	threads := oneGame(blitz.Person{Name: "Ann" + long, Discord: "ann"}, blitz.Person{Name: "Bob" + long, Discord: "bob"}, blitz.DefaultThreadText)
	threads.Round, threads.Games[0].Number = "Round 9 · Quarter-finals", 12
	title := strings.Split(blitz.ThreadsText(threads), "\n")[1]
	if n := utf8.RuneCountInString(title); n > 100 {
		t.Errorf("title %q has %d characters, want at most 100", title, n)
	}
	if !strings.HasPrefix(title, "Round 9 · Quarter-finals · Game 12 · Annxx") || !strings.Contains(title, "… vs Bobxx") || !strings.HasSuffix(title, "…") {
		t.Errorf("title = %q, want the round and game whole, and both names shortened with …", title)
	}
}

func TestCheckThreadTextRefusesATextThatCannotBeFilledIn(t *testing.T) {
	sentence := blitz.DefaultThreadText.NoDiscord
	tests := []struct {
		name      string
		text      blitz.ThreadText
		wantError string
	}{
		{"the default", blitz.DefaultThreadText, ""},
		{"players twice, no deadline", blitz.ThreadText{Message: "Hi {players}. Bye {players}.", NoDiscord: sentence}, ""},
		{"lone braces", blitz.ThreadText{Message: "Hi } {players} :-{", NoDiscord: sentence}, ""},
		{"a message of 1,500 characters", blitz.ThreadText{Message: strings.Repeat("é", 1491) + "{players}", NoDiscord: sentence}, ""},
		{"a sentence of 300 characters", blitz.ThreadText{Message: "Hi {players}.", NoDiscord: strings.Repeat("é", 294) + "{name}"}, ""},
		{"no message", blitz.ThreadText{NoDiscord: sentence}, "{players}"},
		{"a message without {players}", blitz.ThreadText{Message: "Play by {deadline}.", NoDiscord: sentence}, "{players}"},
		{"a misspelt placeholder", blitz.ThreadText{Message: "Hi {players}, by {deadine}.", NoDiscord: sentence}, "{deadine}"},
		{"a placeholder with spaces", blitz.ThreadText{Message: "Hi { players }{players}.", NoDiscord: sentence}, "{ players }"},
		{"{name} in the message", blitz.ThreadText{Message: "Hi {players} and {name}.", NoDiscord: sentence}, "{name}"},
		{"a message of 1,501 characters", blitz.ThreadText{Message: strings.Repeat("é", 1492) + "{players}", NoDiscord: sentence}, "1500"},
		{"no sentence", blitz.ThreadText{Message: "Hi {players}."}, "{name}"},
		{"a sentence without {name}", blitz.ThreadText{Message: "Hi {players}.", NoDiscord: "Friend them in the app."}, "{name}"},
		{"{players} in the sentence", blitz.ThreadText{Message: "Hi {players}.", NoDiscord: "{name} and {players}"}, "{players}"},
		{"a sentence of 301 characters", blitz.ThreadText{Message: "Hi {players}.", NoDiscord: strings.Repeat("é", 295) + "{name}"}, "300"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := blitz.CheckThreadText(tt.text)
			switch {
			case tt.wantError == "" && err != nil:
				t.Errorf("CheckThreadText(%q) = %v, want nil", tt.text, err)
			case tt.wantError != "" && (err == nil || !strings.Contains(err.Error(), tt.wantError)):
				t.Errorf("CheckThreadText(%q) = %v, want an error naming %q", tt.text, err, tt.wantError)
			}
		})
	}
}
