// Discord threads for a round: the round's games as DiscordText lists them, then each game's thread title and the
// edition's welcome message mentioning its players and organisers, as text an organiser pastes into Discord by hand
// or the bot posts in each game's own thread.
package blitz

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// ThreadGame is one game of a round, numbered as the site's games table numbers it, and its two players.
type ThreadGame struct {
	Number int
	A, B   Person
}

// ThreadText is what an edition's thread messages say, each part optional. Message welcomes a game's players: any
// {players} becomes them and any {deadline} the deadline. NoDiscord follows it for each player without a Discord
// username, any {name} becoming theirs. Anything else is written as typed.
type ThreadText struct{ Message, NoDiscord string }

// DefaultThreadText is an edition's thread text until admin sets one.
var DefaultThreadText = ThreadText{
	Message: "Welcome {players}. In this thread you can schedule your game with your opponent. " +
		"Please schedule and play your game by {deadline}, then post the replay code or the outcome here.",
	NoDiscord: "{name} is one of the few people that doesn't have access to Discord but wanted to play, " +
		"you can send them a friend request in the TTA app to set up a game instead.",
}

// Threads is what a round's threads say: the round's title (RoundTitle), the deadline to play by, the edition's
// text, the organisers to ping, the games, and the byes, listed with the games but given no thread.
type Threads struct {
	Round      string
	Deadline   time.Time
	Text       ThreadText
	Organisers []Person
	Games      []ThreadGame
	Byes       []Person
}

// Thread texts' longest, in characters, leaving room in Discord's 2,000 for the names and the moderators.
const threadMessageMax, noDiscordMax = 1500, 300

// CheckThreadText returns why a thread text cannot be used, nil when it can: a part too long.
func CheckThreadText(t ThreadText) error {
	if n := utf8.RuneCountInString(t.Message); n > threadMessageMax {
		return fmt.Errorf("the message is at most %d characters, not %d", threadMessageMax, n)
	}
	if n := utf8.RuneCountInString(t.NoDiscord); n > noDiscordMax {
		return fmt.Errorf("the no-Discord sentence is at most %d characters, not %d", noDiscordMax, n)
	}
	return nil
}

// threadTitleMax is the longest thread name Discord takes, in characters.
const threadTitleMax = 100

// missingUsername stands in for an organiser's Discord username nobody gave, for them to fill in before pasting.
const missingUsername = "@???"

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

// organiser writes an organiser as "Carol (@carol)", by user id when known, the placeholder in the brackets without
// either.
func organiser(p Person) string {
	if !reachable(p) {
		return p.Name + " (" + missingUsername + ")"
	}
	return p.Name + " (" + mention(p) + ")"
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

// ThreadsText writes a round's threads: first the games as DiscordText lists them and a blank line, then a block
// per game between --- lines: its title, then its message as GameThread writes it. A blank line parts the blocks.
func ThreadsText(t Threads) string {
	games := make([][2]Person, len(t.Games))
	for i, g := range t.Games {
		games[i] = [2]Person{g.A, g.B}
	}
	var out strings.Builder
	out.WriteString(DiscordText(games, t.Byes) + "\n")
	for _, g := range t.Games {
		title, message := GameThread(t, g, nil)
		fmt.Fprintf(&out, "\n---\n%s\n---\n%s---\n", title, message)
	}
	return out.String()
}

// GameThread is game g's thread: its title, and its message, the edition's text mentioning its players, then whom
// to ping, the no-Discord sentence for each player without a username, an empty part left out, and a line asking
// the organisers to add each of missing, players the bot could not find. The deadline is a Discord timestamp, shown
// to each reader on their own clock.
func GameThread(t Threads, g ThreadGame, missing []Person) (title, message string) {
	deadline := fmt.Sprintf("<t:%d:F>", t.Deadline.Unix())
	var out strings.Builder
	if t.Text.Message != "" {
		out.WriteString(strings.NewReplacer("{players}", mention(g.A)+", "+mention(g.B), "{deadline}", deadline).Replace(t.Text.Message) + " ")
	}
	out.WriteString("If the game did not finish, or there are any issues with scheduling or otherwise, please ping " + moderators(t.Organisers) + ".\n")
	for _, p := range []Person{g.A, g.B} {
		if !reachable(p) && t.Text.NoDiscord != "" {
			out.WriteString(strings.ReplaceAll(t.Text.NoDiscord, "{name}", p.Name) + "\n")
		}
	}
	for _, p := range missing {
		out.WriteString("Organisers: please add " + p.Name + " (" + mention(p) + ") to this thread; the bot could not find them in this server.\n")
	}
	return threadTitle(t.Round, g), out.String()
}

// ReplacedThread is what the bot posts in a game's thread before closing it, once its round is paired again without
// the game.
const ReplacedThread = "This round was paired again and this game is no longer in it. Please use your new thread; this one is closed."

// DeadlineMoved is the line posted in game g's thread when its round's deadline moves, mentioning its players.
func DeadlineMoved(t Threads, g ThreadGame) string {
	at := t.Deadline.Unix()
	return fmt.Sprintf("%s, %s: the deadline for this game is now <t:%d:F> (<t:%d:R>).", mention(g.A), mention(g.B), at, at)
}

// ThreadReminder is the line posted in game g's thread when its round's deadline nears, mentioning its players.
func ThreadReminder(t Threads, g ThreadGame) string {
	at := t.Deadline.Unix()
	return fmt.Sprintf("%s, %s: a reminder that this game is due <t:%d:R>, by <t:%d:F>.", mention(g.A), mention(g.B), at, at)
}
