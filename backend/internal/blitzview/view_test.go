package blitzview

import (
	"reflect"
	"testing"
	"time"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

var publishedAt = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// edition is an edition of swiss rounds and a knockout of knockout, its entrants Ann (1, signed in with Discord),
// Bob (2), Cid (3) and Dee (4).
func edition(swiss, knockout int, rounds ...store.BlitzRound) store.BlitzEdition {
	ed := store.BlitzEdition{Rounds: rounds}
	ed.Format = store.BlitzFormat{SwissRounds: swiss, KnockoutSize: knockout}
	ed.Points = blitz.FIDEScoring
	for i, name := range []string{"Ann", "Bob", "Cid", "Dee"} {
		ed.Entrants = append(ed.Entrants, store.BlitzEntrant{ID: int64(i + 1), Name: name, Status: "active"})
	}
	ed.Entrants[0].DiscordName, ed.Entrants[0].DiscordID = "ann", "100000000000000051"
	return ed
}

// round is round number of kind, published unless draft, with matches.
func round(number int, kind string, draft bool, matches ...store.BlitzMatch) store.BlitzRound {
	r := store.BlitzRound{ID: int64(100 + number), Number: number, Kind: kind, Matches: matches}
	if !draft {
		r.PublishedAt = &publishedAt
	}
	return r
}

// match is a, b (0 for the bye) and the result.
func match(a, b int64, result string) store.BlitzMatch {
	m := store.BlitzMatch{A: a, Result: result}
	if b != 0 {
		m.B = &b
	}
	return m
}

func TestRound(t *testing.T) {
	deadline := time.Date(2026, 3, 8, 18, 0, 0, 0, time.UTC)
	swiss := round(2, "swiss", false, match(1, 2, ""), match(3, 0, "a"))
	swiss.Deadline = &deadline
	ann := blitz.Person{Name: "Ann", Discord: "ann", DiscordID: "100000000000000051"}
	tests := []struct {
		name   string
		ed     store.BlitzEdition
		number int
		want   store.BlitzNews
		ok     bool
	}{
		{"a Swiss round with a bye and a deadline", edition(3, 0, round(1, "swiss", false), swiss), 2,
			store.BlitzNews{Title: "Round 2", Games: [][2]blitz.Person{{ann, {Name: "Bob"}}}, Byes: []blitz.Person{{Name: "Cid"}}, Deadline: &deadline}, true},
		{"a knockout round named by its stage", edition(1, 2, round(1, "swiss", false), round(2, "knockout", false, match(4, 1, ""))), 2,
			store.BlitzNews{Title: "Round 2 · Final", Games: [][2]blitz.Person{{{Name: "Dee"}, ann}}}, true},
		{"a draft round", edition(3, 0, round(1, "swiss", true, match(1, 2, ""))), 1, store.BlitzNews{}, false},
		{"no such round", edition(3, 0, round(1, "swiss", false, match(1, 2, ""))), 2, store.BlitzNews{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Round(tt.ed, tt.number)
			if ok != tt.ok || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Round(%d) = %+v, %v, want %+v, %v", tt.number, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestPodium(t *testing.T) {
	swiss := []store.BlitzRound{
		round(1, "swiss", false, match(1, 2, "a"), match(3, 4, "a")),
		round(2, "swiss", false, match(1, 3, "a"), match(4, 2, "a")),
	}
	semis := round(3, "knockout", false, match(1, 4, "a"), match(2, 3, "b"))
	places := func(names ...[]string) []store.BlitzPlace {
		var out []store.BlitzPlace
		for i, n := range names {
			var who []blitz.Person
			for _, name := range n {
				who = append(who, blitz.Person{Name: name})
			}
			out = append(out, store.BlitzPlace{Place: i + 1, People: who})
		}
		return out
	}
	tests := []struct {
		name string
		ed   store.BlitzEdition
		want []store.BlitzPlace
		ok   bool
	}{
		{"a final won: its loser second, the semi-finals' losers third",
			edition(2, 4, append(swiss, semis, round(4, "knockout", false, match(1, 3, "b")))...),
			places([]string{"Cid"}, []string{"Ann"}, []string{"Dee", "Bob"}), true},
		{"a knockout of two: no third", edition(2, 2, append(swiss, round(3, "knockout", false, match(1, 3, "a")))...),
			places([]string{"Ann"}, []string{"Cid"}), true},
		{"a final not yet won", edition(2, 4, append(swiss, semis, round(4, "knockout", false, match(1, 3, "")))...), nil, false},
		{"a final still a draft", edition(2, 4, append(swiss, semis, round(4, "knockout", true, match(1, 3, "a")))...), nil, false},
		{"the Swiss rounds only, when a knockout follows", edition(2, 4, swiss...), nil, false},
		{"Swiss only, every game played: the standings' top three", edition(2, 0, swiss...),
			places([]string{"Ann"}, []string{"Cid"}, []string{"Dee"}), true},
		{"Swiss only, a game of the last round unplayed",
			edition(2, 0, swiss[0], round(2, "swiss", false, match(1, 3, "a"), match(4, 2, ""))), nil, false},
		{"Swiss only, a round still to come", edition(3, 0, swiss...), nil, false},
		{"Swiss only, its last round a draft", edition(2, 0, swiss[0], round(2, "swiss", true, match(1, 3, "a"), match(4, 2, "a"))), nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Podium(tt.ed)
			if ok != tt.ok || !reflect.DeepEqual(byName(got), tt.want) {
				t.Errorf("Podium = %+v, %v, want %+v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

// byName is podium with each person by name alone.
func byName(podium []store.BlitzPlace) []store.BlitzPlace {
	var out []store.BlitzPlace
	for _, p := range podium {
		var who []blitz.Person
		for _, person := range p.People {
			who = append(who, blitz.Person{Name: person.Name})
		}
		out = append(out, store.BlitzPlace{Place: p.Place, People: who})
	}
	return out
}

func TestAPodiumNamesItsPeopleAsDiscordCan(t *testing.T) {
	ed := edition(1, 2, round(1, "swiss", false, match(1, 2, "a"), match(3, 4, "a")), round(2, "knockout", false, match(3, 1, "a")))
	got, ok := Podium(ed)
	want := blitz.Person{Name: "Ann", Discord: "ann", DiscordID: "100000000000000051"}
	if !ok || len(got) != 2 || len(got[1].People) != 1 || got[1].People[0] != want {
		t.Errorf("Podium = %+v, %v; want Ann second with her Discord name and id", got, ok)
	}
}

func TestPodiumSharesAPlaceOnlyTheDrawSeparates(t *testing.T) {
	ed := edition(1, 0, round(1, "swiss", false, match(1, 2, "draw"), match(3, 4, "draw")))
	got, ok := Podium(ed)
	if !ok || len(got) != 1 || got[0].Place != 1 || len(got[0].People) != 4 {
		t.Errorf("Podium of four draws = %+v, %v, want all four sharing first", got, ok)
	}
}

func TestReminder(t *testing.T) {
	deadline := time.Date(2026, 3, 8, 18, 0, 0, 0, time.UTC)
	open := round(1, "swiss", false, match(1, 2, "a"), match(3, 4, ""), match(2, 0, "a"))
	open.Deadline = &deadline
	tests := []struct {
		name string
		ed   store.BlitzEdition
		want store.BlitzNews
		ok   bool
	}{
		{"only the games without a result, no bye", edition(1, 0, open),
			store.BlitzNews{Title: "Round 1", Games: [][2]blitz.Person{{{Name: "Cid"}, {Name: "Dee"}}}, Deadline: &deadline}, true},
		{"every game played", edition(1, 0, round(1, "swiss", false, match(1, 2, "a"), match(3, 4, "draw"))), store.BlitzNews{}, false},
		{"a draft", edition(1, 0, round(1, "swiss", true, match(1, 2, ""))), store.BlitzNews{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Reminder(tt.ed, 1)
			if ok != tt.ok || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Reminder = %+v, %v, want %+v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestGameThread(t *testing.T) {
	deadline := publishedAt.Add(72 * time.Hour)
	withID := func(id int64, m store.BlitzMatch) store.BlitzMatch { m.ID = id; return m }
	dated := func(r store.BlitzRound) store.BlitzRound { r.Deadline = &deadline; return r }
	tests := []struct {
		name   string
		r      store.BlitzRound
		match  int64
		want   string
		wantOK bool
	}{
		{"a game to play", dated(round(1, "swiss", false, withID(7, match(1, 2, "")), withID(8, match(3, 4, "")))), 8, "Round 1 Game 2 Cid v Dee", true},
		{"a round with no deadline", round(1, "swiss", false, withID(7, match(1, 2, ""))), 7, "", false},
		{"a draft", dated(round(1, "swiss", true, withID(7, match(1, 2, "")))), 7, "", false},
		{"a game played", dated(round(1, "swiss", false, withID(7, match(1, 2, "a")))), 7, "", false},
		{"a bye", dated(round(1, "swiss", false, withID(7, match(1, 0, "")))), 7, "", false},
		{"a game no longer in the round", dated(round(1, "swiss", false, withID(7, match(1, 2, "")))), 9, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ed := edition(3, 0, tt.r)
			threads, g, ok := GameThread(ed, 1, tt.match)
			got := ""
			if ok {
				got = threads.Round + " Game " + string(rune('0'+g.Number)) + " " + g.A.Name + " v " + g.B.Name
				if !threads.Deadline.Equal(deadline) || threads.Text != blitz.DefaultThreadText || g.A.DiscordID != "100000000000000051" && g.A.Name == "Ann" {
					t.Errorf("threads = %+v, want the round's deadline, the default text and Discord ids", threads)
				}
			}
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("GameThread(round 1, match %d) = %q, %t; want %q, %t", tt.match, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestThreadsByMatchKeysEachGameByItsMatch(t *testing.T) {
	deadline := publishedAt.Add(72 * time.Hour)
	withID := func(id int64, m store.BlitzMatch) store.BlitzMatch { m.ID = id; return m }
	r := round(1, "swiss", false, withID(7, match(1, 2, "a")), withID(8, match(3, 4, "")))
	r.Deadline = &deadline
	threads, games := ThreadsByMatch(edition(3, 0, r), 1)
	if !threads.Deadline.Equal(deadline) || len(games) != 2 || games[8].Number != 2 || games[8].A.Name != "Cid" || games[7].B.Name != "Bob" {
		t.Errorf("ThreadsByMatch = %+v, %+v; want both games by match, Cid's second", threads, games)
	}
	if _, games := ThreadsByMatch(edition(3, 0, r), 2); len(games) != 0 {
		t.Errorf("ThreadsByMatch of a round not there = %+v, want none", games)
	}
}
