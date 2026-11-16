// /rivals: the opponents a player met most, and the record against each.
package discordbot

import (
	"fmt"
	"strings"
)

// Rival is a player's record against one opponent, with the win rate the site shows.
type Rival struct {
	Name                       string
	Games, Wins, Losses, Draws int
	WinRate                    float64
}

// shownRivals is how many opponents /rivals lists.
const shownRivals = 5

// RivalsText is the reply for name's rivals in format, most games first, linking name's page on site.
func RivalsText(name string, format int, rivals []Rival, site string) string {
	who := "**" + EscapeMarkdown(name) + "**"
	if len(rivals) == 0 {
		return fmt.Sprintf("%s has no rated %s games.", who, FormatLabel(format))
	}
	lines := []string{fmt.Sprintf("%s's most played opponents · %s, rated games", who, FormatLabel(format))}
	for i, r := range rivals[:min(shownRivals, len(rivals))] {
		lines = append(lines, fmt.Sprintf("%d. %s · %d games · %d–%d–%d · %.1f%% won",
			i+1, EscapeMarkdown(r.Name), r.Games, r.Wins, r.Losses, r.Draws, r.WinRate))
	}
	return strings.Join(lines, "\n") + "\n<" + PlayerURL(site, name) + ">"
}
