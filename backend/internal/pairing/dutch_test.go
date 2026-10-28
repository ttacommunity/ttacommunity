package pairing_test

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"ttacommunity/backend/internal/pairing"
)

// field makes players 1 to n on the given points, in that order, each starting rank their id.
func field(points ...float64) []pairing.Player {
	var out []pairing.Player
	for i, p := range points {
		out = append(out, pairing.Player{ID: int64(i + 1), TPN: i + 1, Points: p})
	}
	return out
}

// meet records that players a and b (ids from 1) have played.
func meet(players []pairing.Player, pairs ...[2]int64) []pairing.Player {
	for _, p := range pairs {
		players[p[0]-1].Opponents = append(players[p[0]-1].Opponents, p[1])
		players[p[1]-1].Opponents = append(players[p[1]-1].Opponents, p[0])
	}
	return players
}

// with changes player id (from 1) in place and returns the players.
func with(players []pairing.Player, id int64, change func(*pairing.Player)) []pairing.Player {
	change(&players[id-1])
	return players
}

// inZones gives players 1 to n the offsets in hours.
func inZones(players []pairing.Player, hours ...float64) []pairing.Player {
	for i, h := range hours {
		players[i].Offset = int(h * 60)
	}
	return players
}

func TestDutchPairs(t *testing.T) {
	tests := []struct {
		name    string
		players []pairing.Player
		want    []string
	}{
		{"round 1: the top half plays the bottom half", field(0, 0, 0, 0, 0, 0, 0, 0), []string{"1-5", "2-6", "3-7", "4-8"}},
		{"round 1, odd: the lowest takes the bye", field(0, 0, 0, 0, 0, 0, 0), []string{"1-4", "2-5", "3-6", "bye:7"}},
		{"a transposition avoids a rematch", meet(field(0, 0, 0, 0), [2]int64{1, 3}), []string{"1-4", "2-3"}},
		{"an exchange when no transposition works", meet(field(0, 0, 0, 0), [2]int64{1, 3}, [2]int64{1, 4}), []string{"1-2", "3-4"}},
		{"the odd one floats down to the top of the next group", field(2, 2, 2, 1, 1, 1, 0, 0), []string{"1-2", "3-4", "5-6", "7-8"}},
		{
			"a player who floated down last round does not float again",
			with(field(2, 2, 2, 1, 1, 1, 0, 0), 3, func(p *pairing.Player) { p.Floats[0] = pairing.Down }),
			[]string{"1-3", "2-4", "5-6", "7-8"},
		},
		{
			"a player who floated up last round is not the floater's opponent",
			with(field(2, 2, 2, 1, 1, 1, 0, 0), 4, func(p *pairing.Player) { p.Floats[0] = pairing.Up }),
			[]string{"1-2", "3-5", "4-6", "7-8"},
		},
		{"leaders float when pairing them strands a rematch below", meet(field(2, 2, 0, 0), [2]int64{3, 4}), []string{"1-3", "2-4"}},
		{
			"the bye skips a player who had one",
			with(field(1, 1, 0), 3, func(p *pairing.Player) { p.HadBye = true }),
			[]string{"1-3", "bye:2"},
		},
		{
			"the bye goes to the lowest score that leaves a full round",
			meet(field(2, 2, 1, 0, 0), [2]int64{3, 4}, [2]int64{3, 5}),
			[]string{"1-3", "2-4", "bye:5"},
		},
		{"starting rank, not id, orders a group", with(with(field(0, 0, 0, 0), 1, func(p *pairing.Player) { p.TPN = 4 }), 4, func(p *pairing.Player) { p.TPN = 1 }), []string{"1-2", "3-4"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pairing.Pair(tt.players, pairing.Dutch, pairing.ByRating, 1)
			if err != nil {
				t.Fatalf("Pair: %v", err)
			}
			if !slices.Equal(pairs(got), tt.want) {
				t.Errorf("pairs = %v, want %v", pairs(got), tt.want)
			}
		})
	}
}

func TestDutchWithTimezonesKeepsTheHalves(t *testing.T) {
	tests := []struct {
		name    string
		players []pairing.Player
		want    []string
	}{
		{"zones choose the opponent in the bottom half", inZones(field(0, 0, 0, 0, 0, 0, 0, 0), 10, -5, 0, 0, -5, 10, 0, 0), []string{"1-6", "2-5", "3-7", "4-8"}},
		{"the top half never meets itself", inZones(field(0, 0, 0, 0, 0, 0, 0, 0), 10, 10, 0, 0, 0, 0, 0, 0), []string{"1-5", "2-6", "3-7", "4-8"}},
		{"a rematch is still avoided", meet(inZones(field(0, 0, 0, 0), 5, 0, 5, 0), [2]int64{1, 3}), []string{"1-4", "2-3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pairing.Pair(tt.players, pairing.DutchTimezone, pairing.ByRating, 1)
			if err != nil {
				t.Fatalf("Pair: %v", err)
			}
			if !slices.Equal(pairs(got), tt.want) {
				t.Errorf("pairs = %v, want %v", pairs(got), tt.want)
			}
		})
	}
}

func TestDutchIgnoresTimezones(t *testing.T) {
	players := inZones(field(0, 0, 0, 0, 0, 0, 0, 0), 10, -5, 0, 0, -5, 10, 0, 0)
	got, err := pairing.Pair(players, pairing.Dutch, pairing.ByRating, 1)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}
	if want := []string{"1-5", "2-6", "3-7", "4-8"}; !slices.Equal(pairs(got), want) {
		t.Errorf("pairs = %v, want %v", pairs(got), want)
	}
}

// TestDutchPlaysWholeTournaments pairs simulated tournaments round after round and checks every round is legal:
// everyone paired once, no rematch, no second bye.
func TestDutchPlaysWholeTournaments(t *testing.T) {
	for _, m := range []pairing.Method{pairing.Dutch, pairing.DutchTimezone} {
		for _, size := range []int{9, 24, 41} {
			t.Run(fmt.Sprintf("%s/%d players", m, size), func(t *testing.T) {
				rng := rand.New(rand.NewPCG(uint64(size), 7))
				players := make([]pairing.Player, size)
				for i := range players {
					players[i] = pairing.Player{ID: int64(i + 1), TPN: i + 1, Offset: 60 * (rng.IntN(24) - 11)}
				}
				for round := 1; round <= min(8, size/2); round++ {
					got, err := pairing.Pair(players, m, pairing.ByRating, uint64(round))
					if err != nil {
						t.Fatalf("round %d: %v", round, err)
					}
					players = play(t, players, got, rng)
				}
			})
		}
	}
}

// play checks a round is legal for the players and returns them after it, the higher starting rank winning
// two games in three.
func play(t *testing.T, players []pairing.Player, r pairing.Round, rng *rand.Rand) []pairing.Player {
	t.Helper()
	byID := map[int64]*pairing.Player{}
	for i := range players {
		byID[players[i].ID] = &players[i]
	}
	seen := map[int64]bool{}
	take := func(id int64) *pairing.Player {
		p, ok := byID[id]
		if !ok || seen[id] {
			t.Fatalf("player %d unknown or paired twice in %v", id, pairs(r))
		}
		seen[id] = true
		return p
	}
	for _, m := range r.Matches {
		a, b := take(m.A), take(m.B)
		if slices.Contains(a.Opponents, b.ID) {
			t.Fatalf("%d and %d meet again in %v", a.ID, b.ID, pairs(r))
		}
		a.Opponents, b.Opponents = append(a.Opponents, b.ID), append(b.Opponents, a.ID)
		if (a.TPN < b.TPN) == (rng.IntN(3) < 2) {
			a.Points++
		} else {
			b.Points++
		}
	}
	if r.Bye != nil {
		p := take(*r.Bye)
		if p.HadBye {
			t.Fatalf("player %d has a second bye", p.ID)
		}
		p.HadBye, p.Points = true, p.Points+1
	}
	if len(seen) != len(players) {
		t.Fatalf("%d of %d players paired: %v", len(seen), len(players), pairs(r))
	}
	return players
}
