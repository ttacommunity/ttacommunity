// /compare: several players' Glicko-D ratings side by side, every format each.
package discordbot

import (
	"fmt"
	"strings"
)

// Compared is one player in /compare: their ratings by format (0 is every format), only those they have.
type Compared struct {
	Name, Title string
	Games       int
	Ratings     map[int]Summary
}

// compareFormats are the formats /compare shows, in order, with their short labels.
var compareFormats = []struct {
	format int
	label  string
}{{0, "All"}, {2, "2p"}, {3, "3p"}, {4, "4p"}}

// CompareText is the reply for players, two lines each: name, title and games, then a rating per format.
func CompareText(players []Compared) string {
	lines := []string{"Glicko-D, rating ± deviation (rank)"}
	for _, p := range players {
		head := "**" + EscapeMarkdown(p.Name) + "**"
		if p.Title != "" {
			head += " [" + p.Title + "]"
		}
		cells := make([]string, 0, len(compareFormats))
		for _, f := range compareFormats {
			r, ok := p.Ratings[f.format]
			if !ok {
				cells = append(cells, f.label+" —")
				continue
			}
			cell := fmt.Sprintf("%s %.1f ± %.1f", f.label, r.Rating, r.RD)
			if r.Rank != nil {
				cell += fmt.Sprintf(" (#%d)", *r.Rank)
			}
			cells = append(cells, cell)
		}
		lines = append(lines, fmt.Sprintf("%s · %d games", head, p.Games), strings.Join(cells, " · "))
	}
	return strings.Join(lines, "\n")
}
