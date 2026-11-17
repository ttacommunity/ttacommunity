// /upsets: the players who did furthest below what their ratings expected in a month's rated games.
package discordbot

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// Pair is one player against one opponent in a game: their ratings going in, Name's expected score against Other
// (0 to 1), Name's actual score, and whether Name timed out.
type Pair struct {
	GameID              int64
	Date                time.Time
	Tournament          string
	PlayerCount         int
	Name, Other         string
	Rating, OtherRating float64
	Expected, Score     float64
	TimedOut            bool
}

// Upset is a player who failed to beat an opponent they were favoured against: their expected and actual score
// against the table, and the opponents who finished ahead of or level with them.
type Upset struct {
	GameID           int64
	Date             time.Time
	Tournament       string
	PlayerCount      int
	Name             string
	Rating           float64
	Expected, Actual float64
	Opponents        int
	Ahead, Level     []Opponent
	favouriteBeaten  bool
}

// Opponent is an opponent's name and rating going into the game.
type Opponent struct {
	Name   string
	Rating float64
}

// shortfall is how far below expectation u did per opponent, so a table weighs the same as a duel.
func (u Upset) shortfall() float64 { return (u.Expected - u.Actual) / float64(u.Opponents) }

// Upsets returns the n biggest upsets in pairs, the biggest shortfall per opponent first: players who lost to or
// drew with an opponent they had over 50% against. A player who timed out is left out.
func Upsets(pairs []Pair, n int) []Upset {
	type key struct {
		game int64
		name string
	}
	byPlayer := map[key]*Upset{}
	var order []key
	for _, p := range pairs {
		if p.TimedOut {
			continue
		}
		k := key{p.GameID, p.Name}
		u, ok := byPlayer[k]
		if !ok {
			u = &Upset{GameID: p.GameID, Date: p.Date, Tournament: p.Tournament, PlayerCount: p.PlayerCount, Name: p.Name, Rating: p.Rating}
			byPlayer[k] = u
			order = append(order, k)
		}
		u.Expected += p.Expected
		u.Actual += p.Score
		u.Opponents++
		o := Opponent{Name: p.Other, Rating: p.OtherRating}
		switch {
		case p.Score == 0:
			u.Ahead = append(u.Ahead, o)
		case p.Score == 0.5:
			u.Level = append(u.Level, o)
		}
		if p.Expected > 0.5 && p.Score < 1 {
			u.favouriteBeaten = true
		}
	}
	var upsets []Upset
	for _, k := range order {
		if u := byPlayer[k]; u.favouriteBeaten {
			upsets = append(upsets, *u)
		}
	}
	slices.SortStableFunc(upsets, func(a, b Upset) int {
		return cmp.Or(cmp.Compare(b.shortfall(), a.shortfall()), b.Date.Compare(a.Date), cmp.Compare(a.Name, b.Name))
	})
	return upsets[:min(n, len(upsets))]
}

// UpsetsText is the reply for upsets in the rated games from from to to, in format (0 is every format).
func UpsetsText(from, to time.Time, format int, upsets []Upset) string {
	span := from.Format(time.DateOnly) + " to " + to.Format(time.DateOnly)
	if len(upsets) == 0 {
		return fmt.Sprintf("No upsets in %s from %s.", ratedGames(format), span)
	}
	lines := []string{fmt.Sprintf("Biggest upsets in %s, %s", ratedGames(format), span)}
	for i, u := range upsets {
		who := fmt.Sprintf("**%s** (%.0f)", EscapeMarkdown(u.Name), u.Rating)
		ahead, level := opponentsText(u.Ahead), opponentsText(u.Level)
		var what string
		switch {
		case u.PlayerCount == 2 && ahead != "":
			what = fmt.Sprintf("%s lost to %s, %.0f%% to win", who, ahead, math.Round(u.Expected*100))
		case u.PlayerCount == 2:
			what = fmt.Sprintf("%s drew with %s, %.0f%% to win", who, level, math.Round(u.Expected*100))
		default:
			what = fmt.Sprintf("%s beat %g of %d, expected %.1f", who, u.Actual, u.Opponents, u.Expected)
			if uncounted := u.PlayerCount - 1 - u.Opponents; uncounted > 0 {
				what += fmt.Sprintf(" (%d not counted)", uncounted)
			}
			if ahead != "" {
				what += " · behind " + ahead
			}
			if level != "" {
				what += " · level with " + level
			}
		}
		lines = append(lines, fmt.Sprintf("%d. %s · %d-player, %s, %s", i+1, what, u.PlayerCount, EscapeMarkdown(u.Tournament), u.Date.Format(time.DateOnly)))
	}
	return fitted(lines)
}

// MaxReply is the most characters Discord takes in a reply.
const MaxReply = 2000

// fitted joins a heading and its numbered lines, leaving out the last lines that would take the reply past
// MaxReply and saying how many it left out.
func fitted(lines []string) string {
	for left := 0; ; left++ {
		kept := lines[:len(lines)-left]
		text := strings.Join(kept, "\n")
		if left > 0 {
			text += fmt.Sprintf("\n…and %d more that do not fit in a Discord message", left)
		}
		if len([]rune(text)) <= MaxReply || len(kept) <= 2 {
			return text
		}
	}
}

// opponentsText lists opponents in bold with their ratings, or "" for none.
func opponentsText(opponents []Opponent) string {
	names := make([]string, len(opponents))
	for i, o := range opponents {
		names[i] = fmt.Sprintf("**%s** (%.0f)", EscapeMarkdown(o.Name), o.Rating)
	}
	return strings.Join(names, ", ")
}
