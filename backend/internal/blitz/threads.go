// Discord threads for a round: each game's thread title and the edition's welcome message mentioning its players
// and organisers, as text an organiser pastes into Discord by hand.
package blitz

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Person is someone a thread names: their TTA username and their Discord username ("" when not known).
type Person struct{ Name, Discord string }

// ThreadGame is one game of a round, numbered as the site's games table numbers it, and its two players.
type ThreadGame struct {
	Number int
	A, B   Person
}

// ThreadText is what an edition's thread messages say. Message welcomes a game's players, {players}, and may give
// the deadline, {deadline}; NoDiscord follows it for each player without a Discord username, {name}.
type ThreadText struct{ Message, NoDiscord string }

// DefaultThreadText is an edition's thread text until admin sets one.
var DefaultThreadText = ThreadText{
	Message: "Welcome {players}. In this thread you can schedule your game with your opponent. " +
		"Please schedule and play your game by {deadline}, then post the replay code or the outcome here.",
	NoDiscord: "{name} is one of the few people that doesn't have access to Discord but wanted to play, " +
		"you can send them a friend request in the TTA app to set up a game instead.",
}

// Threads is what a round's threads say: the round's title (RoundTitle), the deadline to play by, the edition's
// text, the organisers to ping and the games, byes left out.
type Threads struct {
	Round      string
	Deadline   time.Time
	Text       ThreadText
	Organisers []Person
	Games      []ThreadGame
}

// Thread texts' longest, in characters, leaving room in Discord's 2,000 for the names and the moderators.
const threadMessageMax, noDiscordMax = 1500, 300

// placeholder finds anything in braces, so a misspelt placeholder is refused rather than pasted as typed.
var placeholder = regexp.MustCompile(`\{[^{}]*\}`)

// CheckThreadText returns why a thread text cannot be used, nil when it can: a part too long, without the
// placeholder it needs, or with one it cannot use.
func CheckThreadText(t ThreadText) error {
	if err := checkTextPart("the message", t.Message, threadMessageMax, "{players}", "{deadline}"); err != nil {
		return err
	}
	return checkTextPart("the no-Discord sentence", t.NoDiscord, noDiscordMax, "{name}")
}

// checkTextPart refuses a part of a thread text over max characters, without its first placeholder, or with one
// not among them.
func checkTextPart(part, text string, max int, placeholders ...string) error {
	if n := utf8.RuneCountInString(text); n > max {
		return fmt.Errorf("%s is at most %d characters, not %d", part, max, n)
	}
	if !strings.Contains(text, placeholders[0]) {
		return fmt.Errorf("%s must hold %s", part, placeholders[0])
	}
	for _, p := range placeholder.FindAllString(text, -1) {
		if !slices.Contains(placeholders, p) {
			return fmt.Errorf("%s can use only %s, not %s", part, strings.Join(placeholders, " and "), p)
		}
	}
	return nil
}

// threadTitleMax is the longest thread name Discord takes, in characters.
const threadTitleMax = 100

// missingUsername stands in for an organiser's Discord username nobody gave, for them to fill in before pasting.
const missingUsername = "@???"

// usernameChars is what a Discord username is made of: 2 to 32 lowercase letters, digits, _ and . (never two
// dots together, checked apart).
var usernameChars = regexp.MustCompile(`^[a-z0-9_.]{2,32}$`)

// RoundTitle names a round as the site does: "Round 3", or "Round 9 · Quarter-finals" in the knockout.
func RoundTitle(round, swissRounds, knockoutSize int) string {
	title := fmt.Sprintf("Round %d", round)
	k := round - swissRounds
	if k < 1 || knockoutSize == 0 || knockoutSize>>(k-1) < 2 {
		return title
	}
	players := knockoutSize >> (k - 1)
	stage, ok := map[int]string{2: "Final", 4: "Semi-finals", 8: "Quarter-finals"}[players]
	if !ok {
		stage = fmt.Sprintf("Round of %d", players)
	}
	return title + " · " + stage
}

// username returns a stored Discord name as a username (lowercase, without a leading @), and false when it
// cannot be one: a display name, an old name#1234 tag, or nothing.
func username(discord string) (string, bool) {
	u := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(discord), "@"))
	return u, usernameChars.MatchString(u) && !strings.Contains(u, "..")
}

// mention writes a player as Discord mentions them, or by their TTA name without a username.
func mention(p Person) string {
	if u, ok := username(p.Discord); ok {
		return "@" + u
	}
	return p.Name
}

// organiser writes an organiser as "Carol (@carol)", the placeholder in the brackets without a username.
func organiser(p Person) string {
	if u, ok := username(p.Discord); ok {
		return p.Name + " (@" + u + ")"
	}
	return p.Name + " (" + missingUsername + ")"
}

// moderators lists whom to ping: "the moderator Carol (@carol)", "the moderators Carol (@carol) and/or Dave (@dave)", or
// with more, "the moderators A, B, and C".
func moderators(organisers []Person) string {
	names := make([]string, len(organisers))
	for i, o := range organisers {
		names[i] = organiser(o)
	}
	switch len(names) {
	case 0:
		return "the moderators"
	case 1:
		return "the moderator " + names[0]
	case 2:
		return "the moderators " + names[0] + " and/or " + names[1]
	}
	return "the moderators " + strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1]
}

// threadTitle names a game's thread, "Round 3 · Game 1 · Ann vs Bob", shortening the longer name, with …, until
// it fits Discord's limit.
func threadTitle(round string, g ThreadGame) string {
	head := fmt.Sprintf("%s · Game %d · ", round, g.Number)
	room := threadTitleMax - utf8.RuneCountInString(head) - len(" vs ")
	a, b := []rune(g.A.Name), []rune(g.B.Name)
	cutA, cutB := false, false
	over := func() bool {
		n := len(a) + len(b)
		if cutA {
			n++
		}
		if cutB {
			n++
		}
		return n > room
	}
	for over() && len(a)+len(b) > 0 {
		if len(a) >= len(b) {
			a, cutA = a[:len(a)-1], true
		} else {
			b, cutB = b[:len(b)-1], true
		}
	}
	name := func(r []rune, cut bool) string {
		if cut {
			return string(r) + "…"
		}
		return string(r)
	}
	return head + name(a, cutA) + " vs " + name(b, cutB)
}

// ThreadsText writes a round's threads, a block per game between --- lines: its title, then the edition's
// message and whom to ping, then the no-Discord sentence for each player without a username. A blank line parts
// the blocks. The deadline is a Discord timestamp, shown to each reader on their own clock.
func ThreadsText(t Threads) string {
	deadline := fmt.Sprintf("<t:%d:F>", t.Deadline.Unix())
	ping := " If the game did not finish, or there are any issues with scheduling or otherwise, please ping " + moderators(t.Organisers) + ".\n"
	var out strings.Builder
	for i, g := range t.Games {
		if i > 0 {
			out.WriteString("\n")
		}
		message := strings.NewReplacer("{players}", mention(g.A)+", "+mention(g.B), "{deadline}", deadline).Replace(t.Text.Message)
		fmt.Fprintf(&out, "---\n%s\n---\n%s%s", threadTitle(t.Round, g), message, ping)
		for _, p := range []Person{g.A, g.B} {
			if _, ok := username(p.Discord); !ok {
				out.WriteString(strings.ReplaceAll(t.Text.NoDiscord, "{name}", p.Name) + "\n")
			}
		}
		out.WriteString("---\n")
	}
	return out.String()
}
