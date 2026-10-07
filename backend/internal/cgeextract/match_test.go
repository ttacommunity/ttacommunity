package cgeextract

import (
	"slices"
	"testing"
)

// cgeSeat is a seat of a finished game, timed out when culture is below 0.
func cgeSeat(player int, culture float64) Seat {
	if culture < 0 {
		return Seat{PlayerID: player, FinishStatus: TimedOut}
	}
	return Seat{PlayerID: player, FinishStatus: Finished, Score: &culture}
}

func resignedSeat(player int, mark float64) Seat {
	return Seat{PlayerID: player, FinishStatus: Resigned, Score: &mark}
}

func cgeGame(session, tournament int, code string, seats ...Seat) Game {
	return Game{SessionID: session, TournamentID: tournament, ShareCode: code, State: "finished", Seats: seats}
}

func ours(id int64, series int64, code string, seats ...OurSeat) OurGame {
	return OurGame{ID: id, Series: series, ShareCode: code, Seats: seats}
}

func seat(name string, score float64) OurSeat { return OurSeat{Name: name, Score: score} }

// names are the accounts each name has played under: ann is 1, bob 2, cy 3, dee 4.
var names = map[string][]int{"ann": {1}, "bob": {2}, "cy": {3}, "dee": {4}}

func TestMatchFindsEachCGEGamesCopyOfOurs(t *testing.T) {
	series := map[int]int64{19: 7, 21: 8}
	tests := []struct {
		name string
		cge  Game
		ours []OurGame
		want map[int64]int
	}{
		{"by share code", cgeGame(100, 19, "CODEAAAA", cgeSeat(1, 200), cgeSeat(2, 150)),
			[]OurGame{ours(1, 7, "CODEAAAA", seat("zed", 1), seat("yan", 2))}, map[int64]int{1: 100}},
		{"by players and scores", cgeGame(100, 19, "CODEAAAA", cgeSeat(1, 200), cgeSeat(2, 150)),
			[]OurGame{ours(1, 7, "", seat("bob", 150), seat("ann", 200))}, map[int64]int{1: 100}},
		{"leaving out a timed-out seat", cgeGame(100, 19, "CODEAAAA", cgeSeat(1, 200), cgeSeat(2, 150), cgeSeat(3, -1)),
			[]OurGame{ours(1, 7, "", seat("ann", 200), seat("bob", 150), seat("cy", -5))}, map[int64]int{1: 100}},
		{"a resignation our file scored 0", cgeGame(100, 19, "CODEAAAA", cgeSeat(1, 200), cgeSeat(2, 150), resignedSeat(3, -9)),
			[]OurGame{ours(1, 7, "", seat("ann", 200), seat("bob", 150), seat("cy", 0))}, map[int64]int{1: 100}},
		{"not when the scores differ", cgeGame(100, 19, "CODEAAAA", cgeSeat(1, 200), cgeSeat(2, 150)),
			[]OurGame{ours(1, 7, "", seat("ann", 200), seat("bob", 151))}, map[int64]int{}},
		{"not when a player differs", cgeGame(100, 19, "CODEAAAA", cgeSeat(1, 200), cgeSeat(2, 150)),
			[]OurGame{ours(1, 7, "", seat("ann", 200), seat("dee", 150))}, map[int64]int{}},
		{"not across series", cgeGame(100, 19, "CODEAAAA", cgeSeat(1, 200), cgeSeat(2, 150)),
			[]OurGame{ours(1, 8, "", seat("ann", 200), seat("bob", 150))}, map[int64]int{}},
		{"not when two of ours fit", cgeGame(100, 19, "CODEAAAA", cgeSeat(1, 200), cgeSeat(2, 150)),
			[]OurGame{ours(1, 7, "", seat("ann", 200), seat("bob", 150)), ours(2, 7, "", seat("ann", 200), seat("bob", 150))},
			map[int64]int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Match([]Game{tt.cge}, tt.ours, names, series)
			if !mapsEqual(got.Matched, tt.want) {
				t.Errorf("matched = %v, want %v", got.Matched, tt.want)
			}
		})
	}
}

func TestMatchAddsWhatOnlyCGEHasAndHoldsBackPossibleCopies(t *testing.T) {
	series := map[int]int64{19: 7}
	cge := []Game{
		cgeGame(100, 19, "CODEAAAA", cgeSeat(1, 200), cgeSeat(2, 150)),
		cgeGame(101, 19, "CODEBBBB", cgeSeat(3, 120), cgeSeat(4, 110)),
	}
	// Ours holds ann and bob's game with a score typed differently: a possible copy, not a match.
	got := Match(cge, []OurGame{ours(1, 7, "", seat("ann", 200), seat("bob", 15))}, names, series)

	if !slices.Equal(got.Added, []int{101}) || !slices.Equal(got.HeldBack, []int{100}) || !slices.Equal(got.OursOnly, []int64{1}) {
		t.Errorf("added %v, held back %v, ours only %v; want 101, 100 and our game 1", got.Added, got.HeldBack, got.OursOnly)
	}
}

func TestMatchSeatsPairsEachCGESeatWithOurs(t *testing.T) {
	g := cgeGame(100, 19, "CODEAAAA", cgeSeat(1, 200), cgeSeat(2, 150), cgeSeat(3, -1))
	tests := []struct {
		name        string
		ours        []OurSeat
		wantPairs   map[int]int
		wantMissing []int
	}{
		{"by account", []OurSeat{{Seat: 1, Name: "x", Account: 2}, {Seat: 2, Name: "y", Account: 1}, {Seat: 3, Name: "z", Account: 3}},
			map[int]int{1: 1, 2: 0, 3: 2}, nil},
		{"by a name the account played under", []OurSeat{{Seat: 1, Name: "bob"}, {Seat: 2, Name: "ann"}, {Seat: 3, Name: "cy"}},
			map[int]int{1: 1, 2: 0, 3: 2}, nil},
		{"a seat only CGE holds", []OurSeat{{Seat: 1, Name: "ann"}, {Seat: 2, Name: "bob"}},
			map[int]int{1: 0, 2: 1}, []int{2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pairs, missing := MatchSeats(g, tt.ours, names)
			if !mapsEqual(pairs, tt.wantPairs) || !slices.Equal(missing, tt.wantMissing) {
				t.Errorf("pairs %v, missing %v; want %v, %v", pairs, missing, tt.wantPairs, tt.wantMissing)
			}
		})
	}
}

func mapsEqual[K comparable, V comparable](a, b map[K]V) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}
