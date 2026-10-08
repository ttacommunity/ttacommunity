package cgeextract

import (
	"slices"
	"strings"
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
		{"a timed-out seat our file lacks", cgeGame(100, 19, "CODEAAAA", cgeSeat(1, 200), cgeSeat(2, 150), cgeSeat(3, -1)),
			[]OurGame{ours(1, 7, "", seat("ann", 200), seat("bob", 150))}, map[int64]int{1: 100}},
		{"not when a finisher is missing", cgeGame(100, 19, "CODEAAAA", cgeSeat(1, 200), cgeSeat(2, 150), cgeSeat(3, 90)),
			[]OurGame{ours(1, 7, "", seat("ann", 200), seat("bob", 150))}, map[int64]int{}},
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
			got := Match([]Game{tt.cge}, tt.ours, names, series, nil)
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
	got := Match(cge, []OurGame{ours(1, 7, "", seat("ann", 200), seat("bob", 15))}, names, series, nil)

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
		{"a renamed player by their score", []OurSeat{{Seat: 1, Name: "ann"}, {Seat: 2, Name: "old name", Score: 150}, {Seat: 3, Name: "cy"}},
			map[int]int{1: 0, 2: 1, 3: 2}, nil},
		{"the one seat left on each side", []OurSeat{{Seat: 1, Name: "ann"}, {Seat: 2, Name: "bob"}, {Seat: 3, Name: "old name", Score: -5}},
			map[int]int{1: 0, 2: 1, 3: 2}, nil},
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

func TestMatchSeatsTrustsCGEsNameBeforeAnAccountAnEarlierLoadStored(t *testing.T) {
	ann, bob := cgeSeat(1, 200), cgeSeat(2, 150)
	ann.PlayerName, bob.PlayerName = "ann", "bob"
	// An earlier load swapped the two seats' accounts; CGE's own seats name who sat where.
	stored := []OurSeat{{Seat: 1, Name: "ann", Account: 2}, {Seat: 2, Name: "Bob", Account: 1}}

	pairs, missing := MatchSeats(cgeGame(100, 19, "CODEAAAA", ann, bob), stored, names)

	if want := map[int]int{1: 0, 2: 1}; !mapsEqual(pairs, want) || len(missing) != 0 {
		t.Errorf("pairs %v, missing %v; want %v, none", pairs, missing, want)
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

func TestMatchTakesAConfirmedPairBeforeAnythingElse(t *testing.T) {
	series := map[int]int64{19: 7}
	cge := []Game{cgeGame(100, 19, "CODEAAAA", cgeSeat(1, 200), cgeSeat(2, 150))}
	// The file typed bob's score differently: held back, unless the pair is confirmed.
	ours := []OurGame{ours(1, 7, "", seat("ann", 200), seat("bob", 15))}

	got := Match(cge, ours, names, series, map[string]int64{"CODEAAAA": 1})

	if !mapsEqual(got.Matched, map[int64]int{1: 100}) || len(got.HeldBack) != 0 {
		t.Errorf("matched %v, held back %v; want our game 1 matched to 100", got.Matched, got.HeldBack)
	}
}

func TestReadConfirmedPairs(t *testing.T) {
	got, err := ReadConfirmedPairs("p.csv", strings.NewReader("share_code,game_id,reason\nCODEAAAA,12,r\nCODEBBBB,x,r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pairs) != 1 || got.Pairs["CODEAAAA"] != 12 || len(got.Refused) != 1 || got.Refused[0].Line != 3 {
		t.Errorf("pairs %v, refused %+v; want CODEAAAA→12 and line 3 refused", got.Pairs, got.Refused)
	}
}
