package challenges

import (
	"fmt"
	"strings"
)

// MaxSubmissions is the most entries one submission holds: one for each challenge, and some room.
const MaxSubmissions = 120

// Submission is one entry a player submits: the challenge, their domination, and the proof link ("" for none).
type Submission struct {
	Challenge  string
	Domination int
	ProofURL   string
}

// CheckSubmissions returns entries with their proof links trimmed, or a message saying what is wrong: none or too
// many, a challenge twice, an id no challenge can have, or a domination out of bounds. Whether each challenge exists
// and whether a proof is a message in the TtA Discord are the database's checks.
func CheckSubmissions(entries []Submission) ([]Submission, string) {
	if len(entries) == 0 || len(entries) > MaxSubmissions {
		return nil, fmt.Sprintf("submit 1 to %d entries", MaxSubmissions)
	}
	seen := map[string]bool{}
	got := make([]Submission, len(entries))
	for i, e := range entries {
		switch {
		case !challengeID.MatchString(e.Challenge):
			return nil, fmt.Sprintf("entry %d: no challenge has the id %q", i+1, e.Challenge)
		case seen[e.Challenge]:
			return nil, fmt.Sprintf("entry %d: %s is entered twice", i+1, e.Challenge)
		case e.Domination < MinDomination || e.Domination > MaxDomination:
			return nil, fmt.Sprintf("%s: the domination is not %d to %d", e.Challenge, MinDomination, MaxDomination)
		}
		seen[e.Challenge] = true
		got[i] = Submission{Challenge: e.Challenge, Domination: e.Domination, ProofURL: strings.TrimSpace(e.ProofURL)}
	}
	return got, ""
}
