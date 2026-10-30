package challenges

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Challenge is one of the game's challenges as the committed list has it. Opponents are kept as written, a JSON list.
type Challenge struct {
	ID          string          `json:"id"`
	Set         string          `json:"set"`
	Group       string          `json:"group"`
	Slot        int             `json:"slot"`
	Difficulty  int             `json:"difficulty"`
	Players     int             `json:"players"`
	Name        string          `json:"name"`
	Description []string        `json:"description"`
	Rules       []string        `json:"rules"`
	Opponents   json.RawMessage `json:"opponents"`
}

var (
	challengeID    = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)
	challengeGroup = regexp.MustCompile(`^[a-z_]{1,40}$`)
	sets           = map[string]bool{"original": true, "expansion": true, "mix": true}
)

// ReadList reads the committed challenge list, in its order, or refuses it whole when a challenge could not be
// stored: the load job replaces the table with it, so a partial list would remove challenges.
func ReadList(r io.Reader) ([]Challenge, error) {
	var list []Challenge
	if err := json.NewDecoder(r).Decode(&list); err != nil {
		return nil, fmt.Errorf("the challenge list is not a JSON list of challenges: %w", err)
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("the challenge list has no challenges")
	}
	seen := map[string]bool{}
	for i, c := range list {
		if problem := checkChallenge(c); problem != "" {
			return nil, fmt.Errorf("challenge %d (%q): %s", i+1, c.ID, problem)
		}
		if seen[c.ID] {
			return nil, fmt.Errorf("challenge %d: %q is listed twice", i+1, c.ID)
		}
		seen[c.ID] = true
	}
	return list, nil
}

// checkChallenge says what about c the database would refuse, or "".
func checkChallenge(c Challenge) string {
	switch {
	case !challengeID.MatchString(c.ID):
		return "the id is not 1 to 40 lowercase letters, digits and underscores"
	case !sets[c.Set]:
		return "the set is not original, expansion or mix"
	case !challengeGroup.MatchString(c.Group):
		return "the group is not 1 to 40 lowercase letters and underscores"
	case c.Slot < 0:
		return "the slot is negative"
	case c.Difficulty < 1 || c.Difficulty > 10:
		return "the difficulty is not 1 to 10"
	case c.Players < 2 || c.Players > 4:
		return "the players are not 2 to 4"
	case strings.TrimSpace(c.Name) == "" || utf8.RuneCountInString(c.Name) > 100:
		return "the name is empty or over 100 characters"
	case !bytes.HasPrefix(bytes.TrimSpace(c.Opponents), []byte("[")):
		return "the opponents are not a list"
	}
	return ""
}
