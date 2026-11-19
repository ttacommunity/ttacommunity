package blitzview

import (
	"reflect"
	"testing"
	"time"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

var publishedAt = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// edition is an edition of swiss rounds and a knockout of knockout, its entrants Ann (1), Bob (2), Cid (3) and Dee (4).
func edition(swiss, knockout int, rounds ...store.BlitzRound) store.BlitzEdition {
	ed := store.BlitzEdition{Rounds: rounds}
	ed.Format = store.BlitzFormat{SwissRounds: swiss, KnockoutSize: knockout}
	ed.Points = blitz.FIDEScoring
	for i, name := range []string{"Ann", "Bob", "Cid", "Dee"} {
		ed.Entrants = append(ed.Entrants, store.BlitzEntrant{ID: int64(i + 1), Name: name, Status: "active"})
	}
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
	tests := []struct {
		name   string
		ed     store.BlitzEdition
		number int
		want   store.BlitzNews
		ok     bool
	}{
		{"a Swiss round with a bye and a deadline", edition(3, 0, round(1, "swiss", false), swiss), 2,
			store.BlitzNews{Title: "Round 2", Games: [][2]string{{"Ann", "Bob"}}, Byes: []string{"Cid"}, Deadline: &deadline}, true},
		{"a knockout round named by its stage", edition(1, 2, round(1, "swiss", false), round(2, "knockout", false, match(4, 1, ""))), 2,
			store.BlitzNews{Title: "Round 2 · Final", Games: [][2]string{{"Dee", "Ann"}}}, true},
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
			out = append(out, store.BlitzPlace{Place: i + 1, Names: n})
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
			if ok != tt.ok || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Podium = %+v, %v, want %+v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestPodiumSharesAPlaceOnlyTheDrawSeparates(t *testing.T) {
	ed := edition(1, 0, round(1, "swiss", false, match(1, 2, "draw"), match(3, 4, "draw")))
	got, ok := Podium(ed)
	if !ok || len(got) != 1 || got[0].Place != 1 || len(got[0].Names) != 4 {
		t.Errorf("Podium of four draws = %+v, %v, want all four sharing first", got, ok)
	}
}
