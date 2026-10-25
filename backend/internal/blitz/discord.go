// A round's Discord text: a line per game naming its players as Discord mentions them, for an organiser to paste
// into Discord so everyone playing is notified.
package blitz

import (
	"regexp"
	"strings"
)

// Person is a player as Discord text names them: their TTA username and their Discord name ("" when not known).
type Person struct{ Name, Discord string }

// usernameChars is what a Discord username is made of: 2 to 32 lowercase letters, digits, _ and . (never two
// dots together, checked apart).
var usernameChars = regexp.MustCompile(`^[a-z0-9_.]{2,32}$`)

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

// DiscordText writes a round's games, a line each as "@ann vs @bob", every line but the last ending in a comma.
func DiscordText(games [][2]Person) string {
	lines := make([]string, len(games))
	for i, g := range games {
		lines[i] = mention(g[0]) + " vs " + mention(g[1])
	}
	return strings.Join(lines, ",\n")
}
