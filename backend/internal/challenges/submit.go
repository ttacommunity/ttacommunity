package challenges

import (
	"fmt"
	"regexp"
	"strings"
)

// DiscordGuild is the TtA Discord server: an entry is verified when it links a message there.
const DiscordGuild = "358698681672925195"

// MaxSubmissions is the most entries one submission holds: one for each challenge, and some room.
const MaxSubmissions = 120

var discordMessage = regexp.MustCompile(`^https?://(?:www\.|ptb\.|canary\.)?discord(?:app)?\.com/channels/(\d+)/(\d+)/(\d+)/?$`)

// ProofURL returns raw as the one way a Discord message link in the TtA server is written, and true; "" and true for
// no proof; and false for anything else.
func ProofURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true
	}
	m := discordMessage.FindStringSubmatch(raw)
	if m == nil || m[1] != DiscordGuild {
		return "", false
	}
	return fmt.Sprintf("https://discord.com/channels/%s/%s/%s", m[1], m[2], m[3]), true
}

// Submission is one entry a player submits: the challenge, their domination, and the proof link ("" for none).
type Submission struct {
	Challenge  string
	Domination int
	ProofURL   string
}

// CheckSubmissions returns entries with their proof links written the one way, or a message saying what is wrong:
// none or too many, a challenge twice, an id no challenge can have, a domination out of bounds, or a proof that is not
// a message in the TtA Discord. Whether each challenge exists is the database's check.
func CheckSubmissions(entries []Submission) ([]Submission, string) {
	if len(entries) == 0 || len(entries) > MaxSubmissions {
		return nil, fmt.Sprintf("submit 1 to %d entries", MaxSubmissions)
	}
	seen := map[string]bool{}
	got := make([]Submission, len(entries))
	for i, e := range entries {
		proof, ok := ProofURL(e.ProofURL)
		switch {
		case !challengeID.MatchString(e.Challenge):
			return nil, fmt.Sprintf("entry %d: no challenge has the id %q", i+1, e.Challenge)
		case seen[e.Challenge]:
			return nil, fmt.Sprintf("entry %d: %s is entered twice", i+1, e.Challenge)
		case e.Domination < MinDomination || e.Domination > MaxDomination:
			return nil, fmt.Sprintf("%s: the domination is not %d to %d", e.Challenge, MinDomination, MaxDomination)
		case !ok:
			return nil, fmt.Sprintf("%s: the proof must be a message link in the TtA Discord", e.Challenge)
		}
		seen[e.Challenge] = true
		got[i] = Submission{Challenge: e.Challenge, Domination: e.Domination, ProofURL: proof}
	}
	return got, ""
}
