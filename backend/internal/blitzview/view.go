// Package blitzview reads a blitz edition as the site shows it: its Swiss standings, a published round's games and
// the podium once the edition is finished. The API and Discord's posts share it.
package blitzview

import (
	"slices"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

// Played is a match of round as the blitz package sees it; B is 0 for a bye, and a result with no game is a
// forfeit.
func Played(round int, m store.BlitzMatch) blitz.Played {
	p := blitz.Played{Round: round, A: m.A, Result: blitz.Result(m.Result), Forfeit: m.Result != "" && m.GameID == nil}
	if m.B != nil {
		p.B = *m.B
	}
	return p
}

// SwissPlayed is an edition's published Swiss rounds as the blitz package sees them: the entrants with the rounds
// they skipped, the matches, and the rounds' numbers.
func SwissPlayed(ed store.BlitzEdition) ([]blitz.Entrant, []blitz.Played, []int) {
	var matches []blitz.Played
	var numbers []int
	for _, r := range ed.Rounds {
		if r.Kind == "knockout" || r.PublishedAt == nil {
			continue
		}
		numbers = append(numbers, r.Number)
		for _, m := range r.Matches {
			matches = append(matches, Played(r.Number, m))
		}
	}
	var entrants []blitz.Entrant
	for _, e := range ed.Entrants {
		var skipped []int
		for _, n := range e.Skips {
			if slices.Contains(numbers, n) {
				skipped = append(skipped, n)
			}
		}
		entrants = append(entrants, blitz.Entrant{ID: e.ID, Dropped: e.Status == "dropped", Skipped: skipped, StartingRank: e.StartingRank})
	}
	return entrants, matches, numbers
}

// SwissStandings ranks an edition's entrants by its published Swiss rounds' results and skips, as its points score
// them, and their starting ranks. The knockout's results never count: the standings after the last Swiss round are
// its seeds.
func SwissStandings(ed store.BlitzEdition) []blitz.Standing {
	entrants, matches, _ := SwissPlayed(ed)
	return blitz.Standings(entrants, matches, ed.Format.SwissRounds, ed.Points)
}

// people maps an edition's entrants to themselves as Discord can name them: their Discord name and id only when the
// edition was read for admin.
func people(ed store.BlitzEdition) map[int64]blitz.Person {
	out := make(map[int64]blitz.Person, len(ed.Entrants))
	for _, e := range ed.Entrants {
		out[e.ID] = blitz.Person{Name: e.Name, Discord: e.DiscordName, DiscordID: e.DiscordID}
	}
	return out
}

// published returns the edition's published round of number, and false for none.
func published(ed store.BlitzEdition, number int) (store.BlitzRound, bool) {
	i := slices.IndexFunc(ed.Rounds, func(r store.BlitzRound) bool { return r.Number == number && r.PublishedAt != nil })
	if i < 0 {
		return store.BlitzRound{}, false
	}
	return ed.Rounds[i], true
}

// Round is published round number's news: its title, its games and byes in the order made, and its deadline; false
// when the edition has no such round published.
func Round(ed store.BlitzEdition, number int) (store.BlitzNews, bool) {
	r, ok := published(ed, number)
	if !ok {
		return store.BlitzNews{}, false
	}
	who := people(ed)
	news := store.BlitzNews{Title: blitz.RoundTitle(r.Number, ed.Format.SwissRounds, ed.Format.KnockoutSize), Deadline: r.Deadline}
	for _, m := range r.Matches {
		if m.B == nil {
			news.Byes = append(news.Byes, who[m.A])
			continue
		}
		news.Games = append(news.Games, [2]blitz.Person{who[m.A], who[*m.B]})
	}
	return news, true
}

// Podium is a finished edition's top three, and false while it is not finished. With a knockout, it is finished
// once its final has a winner: the champion, the final's loser, and the semi-finals' losers sharing third. Without
// one, once every game of its last Swiss round has a result: the standings' first three places, shared as they share
// them.
func Podium(ed store.BlitzEdition) ([]store.BlitzPlace, bool) {
	if ed.Format.KnockoutSize == 0 {
		return swissPodium(ed)
	}
	who := people(ed)
	final := ed.Format.SwissRounds + knockoutRounds(ed.Format.KnockoutSize)
	r, ok := published(ed, final)
	if !ok || len(r.Matches) != 1 {
		return nil, false
	}
	winner, loser, ok := decided(r.Matches[0])
	if !ok {
		return nil, false
	}
	podium := []store.BlitzPlace{{Place: 1, People: []blitz.Person{who[winner]}}, {Place: 2, People: []blitz.Person{who[loser]}}}
	if semis, ok := published(ed, final-1); ok && semis.Kind == "knockout" {
		var third []blitz.Person
		for _, m := range semis.Matches {
			if _, l, ok := decided(m); ok {
				third = append(third, who[l])
			}
		}
		if len(third) > 0 {
			podium = append(podium, store.BlitzPlace{Place: 3, People: third})
		}
	}
	return podium, true
}

// knockoutRounds is how many rounds a knockout of size players takes: 3 for 8.
func knockoutRounds(size int) int {
	n := 0
	for ; size > 1; size >>= 1 {
		n++
	}
	return n
}

// decided returns a knockout match's winner and loser, and false while it has neither.
func decided(m store.BlitzMatch) (winner, loser int64, ok bool) {
	if m.B == nil {
		return 0, 0, false
	}
	switch m.Result {
	case "a":
		return m.A, *m.B, true
	case "b":
		return *m.B, m.A, true
	}
	return 0, 0, false
}

// swissPodium is a Swiss-only edition's first three places once every game of its last round has a result.
func swissPodium(ed store.BlitzEdition) ([]store.BlitzPlace, bool) {
	last, ok := published(ed, ed.Format.SwissRounds)
	if !ok || slices.ContainsFunc(last.Matches, func(m store.BlitzMatch) bool { return m.B != nil && m.Result == "" }) {
		return nil, false
	}
	who := people(ed)
	var podium []store.BlitzPlace
	for _, s := range SwissStandings(ed) {
		if s.PlaceFrom > 3 {
			break
		}
		if n := len(podium); n > 0 && podium[n-1].Place == s.PlaceFrom {
			podium[n-1].People = append(podium[n-1].People, who[s.ID])
			continue
		}
		podium = append(podium, store.BlitzPlace{Place: s.PlaceFrom, People: []blitz.Person{who[s.ID]}})
	}
	return podium, true
}
