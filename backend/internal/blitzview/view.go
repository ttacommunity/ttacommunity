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

// Round is published round number's news: its title, its games and byes in the order made, its third-place match
// apart, and its deadline; false when the edition has no such round published.
func Round(ed store.BlitzEdition, number int) (store.BlitzNews, bool) {
	r, ok := published(ed, number)
	if !ok {
		return store.BlitzNews{}, false
	}
	who := people(ed)
	news := store.BlitzNews{Title: blitz.RoundTitle(r.Number, ed.Format.SwissRounds, ed.Format.KnockoutSize), Deadline: r.Deadline}
	for _, m := range r.Matches {
		switch {
		case m.B == nil:
			news.Byes = append(news.Byes, who[m.A])
			continue
		case m.ThirdPlace:
			news.ThirdPlace = &[2]blitz.Person{who[m.A], who[*m.B]}
			continue
		}
		news.Games = append(news.Games, [2]blitz.Person{who[m.A], who[*m.B]})
	}
	return news, true
}

// Podium is a finished edition's top three, and false while it is not finished. With a knockout, it is finished
// once its final, and any third-place match, has a winner: the champion, the final's loser, and the third-place
// match's winner, else the semi-finals' losers sharing third. Without one, once every game of its last Swiss round
// has a result: the standings' first three places, shared as they share them.
func Podium(ed store.BlitzEdition) ([]store.BlitzPlace, bool) {
	if ed.Format.KnockoutSize == 0 {
		return swissPodium(ed)
	}
	who := people(ed)
	final := ed.Format.SwissRounds + blitz.KnockoutRounds(ed.Format.KnockoutSize)
	r, ok := published(ed, final)
	if !ok {
		return nil, false
	}
	var finals, third []store.BlitzMatch
	for _, m := range r.Matches {
		if m.ThirdPlace {
			third = append(third, m)
		} else {
			finals = append(finals, m)
		}
	}
	if len(finals) != 1 || len(third) > 1 {
		return nil, false
	}
	winner, loser, ok := decided(finals[0])
	if !ok {
		return nil, false
	}
	podium := []store.BlitzPlace{{Place: 1, People: []blitz.Person{who[winner]}}, {Place: 2, People: []blitz.Person{who[loser]}}}
	if len(third) == 1 {
		bronze, _, ok := decided(third[0])
		if !ok {
			return nil, false
		}
		return append(podium, store.BlitzPlace{Place: 3, People: []blitz.Person{who[bronze]}}), true
	}
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

// Reminder is published round number's games still without a result, with its title and deadline, and false when
// every game has one or the edition has no such round published. Byes are left out: they play no game.
func Reminder(ed store.BlitzEdition, number int) (store.BlitzNews, bool) {
	r, ok := published(ed, number)
	if !ok {
		return store.BlitzNews{}, false
	}
	who := people(ed)
	news := store.BlitzNews{Title: blitz.RoundTitle(r.Number, ed.Format.SwissRounds, ed.Format.KnockoutSize), Deadline: r.Deadline}
	for _, m := range r.Matches {
		switch {
		case m.B == nil || m.Result != "":
		case m.ThirdPlace:
			news.ThirdPlace = &[2]blitz.Person{who[m.A], who[*m.B]}
		default:
			news.Games = append(news.Games, [2]blitz.Person{who[m.A], who[*m.B]})
		}
	}
	if len(news.Games) == 0 && news.ThirdPlace == nil {
		return store.BlitzNews{}, false
	}
	return news, true
}

// ThreadText is what an edition's thread messages say, and true when it is the edition's own rather than the default.
func ThreadText(ed store.BlitzEdition) (blitz.ThreadText, bool) {
	if ed.ThreadText == nil {
		return blitz.DefaultThreadText, false
	}
	return blitz.ThreadText{Message: ed.ThreadText.Message, NoDiscord: ed.ThreadText.NoDiscord}, true
}

// Threads is what round r's threads say, read for admin: its title, deadline (zero while unset), the edition's text
// and organisers, every game with two players, numbered by its place in the round as the site numbers it, and the
// byes.
func Threads(ed store.BlitzEdition, r store.BlitzRound) blitz.Threads {
	who := people(ed)
	text, _ := ThreadText(ed)
	t := blitz.Threads{Round: blitz.RoundTitle(r.Number, ed.Format.SwissRounds, ed.Format.KnockoutSize), Text: text}
	if r.Deadline != nil {
		t.Deadline = *r.Deadline
	}
	for _, o := range ed.Organisers {
		t.Organisers = append(t.Organisers, blitz.Person{Name: o.Name, Discord: o.DiscordName, DiscordID: o.DiscordID})
	}
	for i, m := range r.Matches {
		if m.B == nil {
			t.Byes = append(t.Byes, who[m.A])
			continue
		}
		g := blitz.ThreadGame{Number: i + 1, A: who[m.A], B: who[*m.B]}
		if m.ThirdPlace {
			g.Title = blitz.ThirdPlaceTitle(r.Number)
		}
		t.Games = append(t.Games, g)
	}
	return t
}

// GameThread is published round number's threads and the game of match in it, for that game's own thread; false
// when the round has no deadline or the match is no longer an unplayed game of it.
func GameThread(ed store.BlitzEdition, number int, match int64) (blitz.Threads, blitz.ThreadGame, bool) {
	r, ok := published(ed, number)
	if !ok || r.Deadline == nil {
		return blitz.Threads{}, blitz.ThreadGame{}, false
	}
	i := slices.IndexFunc(r.Matches, func(m store.BlitzMatch) bool { return m.ID == match })
	if i < 0 || r.Matches[i].B == nil || r.Matches[i].Result != "" {
		return blitz.Threads{}, blitz.ThreadGame{}, false
	}
	t := Threads(ed, r)
	g := slices.IndexFunc(t.Games, func(g blitz.ThreadGame) bool { return g.Number == i+1 })
	return t, t.Games[g], true
}

// ThreadsByMatch is published round number's threads and each of its games by its match's id, for its reminders;
// no games when the edition has no such round published.
func ThreadsByMatch(ed store.BlitzEdition, number int) (blitz.Threads, map[int64]blitz.ThreadGame) {
	r, ok := published(ed, number)
	if !ok {
		return blitz.Threads{}, nil
	}
	t := Threads(ed, r)
	games := map[int64]blitz.ThreadGame{}
	for i, m := range r.Matches {
		if g := slices.IndexFunc(t.Games, func(g blitz.ThreadGame) bool { return g.Number == i+1 }); g >= 0 {
			games[m.ID] = t.Games[g]
		}
	}
	return t, games
}
