// /seats: a player's win rate from each starting seat, against their own in that format.
package discordbot

import (
	"fmt"
	"strings"
)

// SeatGroup is a player's rated games in one format, and their record from each seat.
type SeatGroup struct {
	PlayerCount, Games int
	WinRate            float64
	Seats              []SeatRow
}

// SeatRow is a player's record from one seat; Delta is its win rate less the format's.
type SeatRow struct {
	Seat, Games    int
	WinRate, Delta float64
}

// SeatsText is the reply for name's seats in format (0 is every format), linking name's page on site.
func SeatsText(name string, format int, groups []SeatGroup, site string) string {
	who := "**" + EscapeMarkdown(name) + "**"
	lines := []string{who + "'s win rate by starting seat · rated games"}
	for _, g := range groups {
		if format != 0 && g.PlayerCount != format {
			continue
		}
		seats := make([]string, 0, len(g.Seats))
		for _, s := range g.Seats {
			seats = append(seats, fmt.Sprintf("seat %d %.1f%% (%s)", s.Seat, s.WinRate, signed(s.Delta)))
		}
		lines = append(lines, fmt.Sprintf("%d-player · %d games · %.1f%% won", g.PlayerCount, g.Games, g.WinRate), strings.Join(seats, " · "))
	}
	if len(lines) == 1 {
		return fmt.Sprintf("%s has no %s.", who, ratedGames(format))
	}
	return strings.Join(lines, "\n") + "\n<" + PlayerURL(site, name) + ">"
}

// signed writes x to a tenth with its sign, a minus sign for below zero.
func signed(x float64) string {
	if x < 0 {
		return fmt.Sprintf("−%.1f", -x)
	}
	return fmt.Sprintf("+%.1f", x)
}
