// /versus: two players' record in the games they played together, and its reply.
package discordbot

import (
	"fmt"
	"time"
)

// Meeting is one game two players shared, with where each finished.
type Meeting struct {
	Date                  time.Time
	Tournament            string
	PlayerCount           int
	NamePlace, OtherPlace int
}

// Tally is the first player's record against the second: wins are games they finished ahead in.
type Tally struct {
	Games, Wins, Losses, Draws int
	Last                       *Meeting
}

// TallyVersus counts meetings, newest first, in format (0 is all) as the site's head-to-head does.
func TallyVersus(meetings []Meeting, format int) Tally {
	var t Tally
	for _, m := range meetings {
		if format != 0 && m.PlayerCount != format {
			continue
		}
		if t.Last == nil {
			t.Last = &m
		}
		t.Games++
		switch {
		case m.NamePlace < m.OtherPlace:
			t.Wins++
		case m.NamePlace > m.OtherPlace:
			t.Losses++
		default:
			t.Draws++
		}
	}
	return t
}

// VersusText is the reply for name's tally against other in format, linking name's page on site.
func VersusText(name, other string, format int, t Tally, site string) string {
	a, b := "**"+EscapeMarkdown(name)+"**", "**"+EscapeMarkdown(other)+"**"
	if t.Games == 0 {
		return fmt.Sprintf("%s and %s have no rated %s games together.", a, b, FormatLabel(format))
	}
	text := fmt.Sprintf("%s vs %s · %s, rated games\n%s %d · %s %d", a, b, FormatLabel(format), a, t.Wins, b, t.Losses)
	if t.Draws > 0 {
		text += fmt.Sprintf(" · %d tied", t.Draws)
	}
	text += fmt.Sprintf(" · %d games", t.Games)
	if l := t.Last; l != nil {
		text += fmt.Sprintf("\nLast met %s, %s (%d-player): %s %s, %s %s", l.Date.Format(time.DateOnly),
			EscapeMarkdown(l.Tournament), l.PlayerCount, a, ordinal(l.NamePlace), b, ordinal(l.OtherPlace))
	}
	return text + "\n<" + PlayerURL(site, name) + ">"
}

// ordinal writes a place as 1st, 2nd, 3rd, 4th…
func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}
