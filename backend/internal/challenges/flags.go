package challenges

import (
	"fmt"
	"slices"
	"strings"
)

// Flags returns what looks wrong about each entry, by id, for admins to look at: an unproven entry, not bugged, over
// twice the best of everyone else on its challenge, and a proof another player's entry also links. Pass the entries
// the site shows: a hidden one should not flag others.
func Flags(entries []Entry) map[int64][]string {
	users := map[string][]Entry{}
	byChallenge := map[string][]Entry{}
	for _, e := range entries {
		byChallenge[e.Challenge] = append(byChallenge[e.Challenge], e)
		if e.ProofURL != "" {
			users[e.ProofURL] = append(users[e.ProofURL], e)
		}
	}
	flags := map[int64][]string{}
	for _, e := range entries {
		if !e.Bugged && !e.Verified {
			var best *Entry
			for i, o := range byChallenge[e.Challenge] {
				if o.Player != e.Player && !o.Bugged && (best == nil || o.Domination > best.Domination) {
					best = &byChallenge[e.Challenge][i]
				}
			}
			if best != nil && best.Domination > 0 && e.Domination > 2*best.Domination {
				flags[e.ID] = append(flags[e.ID], fmt.Sprintf("no proof, over 2× everyone else's best (%d by %s)", best.Domination, best.Name))
			}
		}
		var others []string
		for _, o := range users[e.ProofURL] {
			if o.Player != e.Player && !slices.Contains(others, o.Name) {
				others = append(others, o.Name)
			}
		}
		if len(others) > 0 {
			flags[e.ID] = append(flags[e.ID], "proof also used by "+strings.Join(others, ", "))
		}
	}
	return flags
}
