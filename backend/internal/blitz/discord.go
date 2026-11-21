// A round's games as Discord text: a line per game, then its bye, naming its players as Discord mentions them, at
// the top of the round's threads so that pasting it into Discord notifies everyone playing.
package blitz

import (
	"regexp"
	"strings"
)

// Person is a player as Discord text names them: their TTA username, their Discord name ("" when not known) and
// their Discord user id, from their Discord sign-in ("" when they have none).
type Person struct{ Name, Discord, DiscordID string }

// usernameChars is what a Discord username is made of: 2 to 32 lowercase letters, digits, _ and . (never two
// dots together, checked apart).
var usernameChars = regexp.MustCompile(`^[a-z0-9_.]{2,32}$`)

// discordID is what a Discord user id is: 17 to 20 digits.
var discordID = regexp.MustCompile(`^[0-9]{17,20}$`)

// Username returns a stored Discord name as a username (lowercase, without a leading @), and false when it
// cannot be one: a display name, an old name#1234 tag, or nothing.
func Username(discord string) (string, bool) {
	u := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(discord), "@"))
	return u, usernameChars.MatchString(u) && !strings.Contains(u, "..")
}

// TagOf returns p's Discord mention by user id, <@id>, and false when p has no id.
func TagOf(p Person) (string, bool) {
	if !discordID.MatchString(p.DiscordID) {
		return "", false
	}
	return "<@" + p.DiscordID + ">", true
}

// reachable says whether Discord text can mention p: by user id or by username.
func reachable(p Person) bool {
	_, tagged := TagOf(p)
	_, named := Username(p.Discord)
	return tagged || named
}

// mention writes a player as Discord mentions them: by user id, else by username, else by their TTA name.
func mention(p Person) string {
	if tag, ok := TagOf(p); ok {
		return tag
	}
	if u, ok := Username(p.Discord); ok {
		return "@" + u
	}
	return p.Name
}

// DiscordText writes a round's games, a line each as "@ann vs @bob", then a line per bye as "Bye: @cy", every
// line but the last ending in a comma.
func DiscordText(games [][2]Person, byes []Person) string {
	var lines []string
	for _, g := range games {
		lines = append(lines, mention(g[0])+" vs "+mention(g[1]))
	}
	for _, p := range byes {
		lines = append(lines, "Bye: "+mention(p))
	}
	return strings.Join(lines, ",\n")
}
