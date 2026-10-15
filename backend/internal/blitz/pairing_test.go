package blitz_test

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"testing"

	"ttacommunity/backend/internal/blitz"
)

// placed gives a player their place in the standings.
func placed(p blitz.Player, place int) blitz.Player {
	p.Place = place
	return p
}

// rated makes a player with an offset in hours, a rating and points.
func rated(id int64, hours float64, rating, points float64) blitz.Player {
	return blitz.Player{ID: id, Offset: int(hours * 60), Rating: rating, Rated: true, Points: points}
}

// pairs lists a round's matches as "a-b" with the lower id first, sorted, and the bye as "bye:n".
func pairs(r blitz.Round) []string {
	var out []string
	for _, m := range r.Matches {
		a, b := m.A, m.B
		if a > b {
			a, b = b, a
		}
		out = append(out, fmt.Sprintf("%d-%d", a, b))
	}
	sort.Strings(out)
	if r.Bye != nil {
		out = append(out, fmt.Sprintf("bye:%d", *r.Bye))
	}
	return out
}

func TestZoneGapIsCircular(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{120, 120, 0}, {120, -240, 360}, {-600, 780, 60}, {840, -720, 120}, {0, 720, 720}, {-720, 720, 0},
	}
	for _, tt := range tests {
		if got := blitz.ZoneGap(tt.a, tt.b); got != tt.want {
			t.Errorf("ZoneGap(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestPairPutsPlayersInTheSameZoneTogether(t *testing.T) {
	players := []blitz.Player{rated(1, 2, 1500, 0), rated(2, -4, 1500, 0), rated(3, 2, 1500, 0), rated(4, -4, 1500, 0)}
	for seed := uint64(1); seed <= 3; seed++ {
		got, err := blitz.Pair(players, blitz.Criteria{Timezone: true}, seed)
		if err != nil {
			t.Fatalf("Pair: %v", err)
		}
		if want := []string{"1-3", "2-4"}; !slices.Equal(pairs(got), want) {
			t.Errorf("seed %d: pairs = %v, want %v", seed, pairs(got), want)
		}
		for _, m := range got.Matches {
			if m.ZoneGap != 0 {
				t.Errorf("seed %d: match %d-%d zone gap = %d, want 0", seed, m.A, m.B, m.ZoneGap)
			}
		}
	}
}

func TestPairGivesTheByeToTheLowestRankedWithoutOne(t *testing.T) {
	tests := []struct {
		name    string
		players []blitz.Player
		want    int64
	}{
		{"the lowest by points", []blitz.Player{rated(1, 0, 1500, 2), rated(2, 0, 1500, 1), rated(3, 0, 1500, 0)}, 3},
		{"the lowest by rating on equal points", []blitz.Player{rated(1, 0, 1800, 0), rated(2, 0, 1600, 0), rated(3, 0, 1700, 0)}, 2},
		{"an unrated player below every rated one", []blitz.Player{rated(1, 0, 1800, 0), rated(2, 0, 1600, 0), {ID: 3}}, 3},
		{"the next one up when the lowest had a bye", []blitz.Player{rated(1, 0, 1800, 0), rated(2, 0, 1600, 0), {ID: 3, HadBye: true}}, 2},
		{"the lowest again when everyone had one", []blitz.Player{{ID: 1, HadBye: true, Points: 1}, {ID: 2, HadBye: true, Points: 1}, {ID: 3, HadBye: true}}, 3},
		{"the lowest in the standings, whatever the ratings", []blitz.Player{placed(rated(1, 0, 1800, 0), 3), placed(rated(2, 0, 1600, 0), 1), placed(rated(3, 0, 1700, 0), 2)}, 1},
		{"the next one up in the standings when the lowest had a bye", []blitz.Player{placed(blitz.Player{ID: 1, HadBye: true}, 3), placed(rated(2, 0, 1600, 0), 1), placed(rated(3, 0, 1700, 0), 2)}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := blitz.Pair(tt.players, blitz.Criteria{Timezone: true}, 7)
			if err != nil {
				t.Fatalf("Pair: %v", err)
			}
			if got.Bye == nil || *got.Bye != tt.want {
				t.Errorf("round = %v, want the bye to %d", pairs(got), tt.want)
			}
			if len(got.Matches) != 1 {
				t.Errorf("matches = %v, want one", pairs(got))
			}
		})
	}
}

func TestPairNeverRepeatsAnOpponentWhileItCan(t *testing.T) {
	// 1 and 3 share a zone, as do 2 and 4, but 1 has met 3 and 2 has met 4: only cross-zone pairs are legal.
	players := []blitz.Player{rated(1, 2, 1500, 0), rated(2, -4, 1500, 0), rated(3, 2, 1500, 0), rated(4, -4, 1500, 0)}
	players[0].Opponents, players[2].Opponents = []int64{3}, []int64{1}
	players[1].Opponents, players[3].Opponents = []int64{4}, []int64{2}
	got, err := blitz.Pair(players, blitz.Criteria{Timezone: true}, 1)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}
	if len(got.Matches) != 2 {
		t.Fatalf("matches = %v, want two", pairs(got))
	}
	for _, m := range got.Matches {
		if m.ZoneGap != 360 {
			t.Errorf("match %d-%d: zone gap %d; want a fresh cross-zone pair (gap 360)", m.A, m.B, m.ZoneGap)
		}
	}
	if r := repeats(players, got); len(r) > 0 {
		t.Errorf("repeats %v, want none", r)
	}
}

// repeats lists a round's matches between players who have met before.
func repeats(players []blitz.Player, r blitz.Round) []string {
	met := map[[2]int64]bool{}
	for _, p := range players {
		for _, o := range p.Opponents {
			met[[2]int64{min(p.ID, o), max(p.ID, o)}] = true
		}
	}
	var out []string
	for _, m := range r.Matches {
		if met[[2]int64{min(m.A, m.B), max(m.A, m.B)}] {
			out = append(out, fmt.Sprintf("%d-%d", m.A, m.B))
		}
	}
	return out
}

func TestPairFindsARoundWithoutRepeatsTheScoreGroupsMiss(t *testing.T) {
	// The leaders 1 and 2 could meet, but then 3 and 4, who have met, would be left: only 1-3 and 2-4 (or 1-4 and
	// 2-3) pair everyone without a repeat.
	players := []blitz.Player{rated(1, 0, 1500, 2), rated(2, 0, 1500, 2), rated(3, 0, 1500, 0), rated(4, 0, 1500, 0)}
	players[2].Opponents, players[3].Opponents = []int64{4}, []int64{3}
	for seed := uint64(1); seed <= 5; seed++ {
		got, err := blitz.Pair(players, blitz.Criteria{Timezone: true}, seed)
		if err != nil {
			t.Fatalf("Pair(seed %d): %v", seed, err)
		}
		if r := repeats(players, got); len(r) > 0 || len(got.Matches) != 2 {
			t.Errorf("seed %d: %v, repeats %v; want two fresh pairs", seed, pairs(got), r)
		}
	}
}

func TestPairRefusesARoundThatWouldRepeatAnOpponent(t *testing.T) {
	// Four players who have all met: every pairing repeats.
	players := []blitz.Player{rated(1, 0, 1500, 0), rated(2, 0, 1500, 0), rated(3, 0, 1500, 0), rated(4, 0, 1500, 0)}
	for i := range players {
		for j := range players {
			if i != j {
				players[i].Opponents = append(players[i].Opponents, players[j].ID)
			}
		}
	}
	if got, err := blitz.Pair(players, blitz.Criteria{}, 1); !errors.Is(err, blitz.ErrNoPairing) {
		t.Errorf("Pair = %v, %v; want ErrNoPairing", pairs(got), err)
	}
}

func TestPairKeepsScoreGroupsTogetherAndFloatsTheOddOneDown(t *testing.T) {
	players := []blitz.Player{
		rated(1, 0, 1500, 2), rated(2, 0, 1500, 2),
		rated(3, 0, 1500, 1), rated(4, 0, 1500, 1), rated(5, 0, 1500, 1),
		rated(6, 0, 1500, 0), rated(7, 0, 1500, 0), rated(8, 0, 1500, 0),
	}
	got, err := blitz.Pair(players, blitz.Criteria{Timezone: true}, 3)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}
	if !slices.Contains(pairs(got), "1-2") {
		t.Errorf("the two leaders are not paired: %v", pairs(got))
	}
	if got.Bye != nil || len(got.Matches) != 4 {
		t.Errorf("eight players gave %d matches and bye %v, want 4 and none", len(got.Matches), got.Bye)
	}
	// 5, the lowest of the odd 1-point group, floats down: exactly one match crosses score groups.
	byID := map[int64]blitz.Player{}
	for _, p := range players {
		byID[p.ID] = p
	}
	cross := 0
	for _, m := range got.Matches {
		if byID[m.A].Points != byID[m.B].Points {
			cross++
			if m.A != 5 && m.B != 5 {
				t.Errorf("the floater is not 5: %d-%d", m.A, m.B)
			}
		}
	}
	if cross != 1 {
		t.Errorf("%d matches cross score groups, want 1 (the floater): %v", cross, pairs(got))
	}
}

func TestPairUsesRatingsWithinAZoneWhenAsked(t *testing.T) {
	players := []blitz.Player{rated(1, 1, 2000, 0), rated(2, 1, 1400, 0), rated(3, 1, 1900, 0), rated(4, 1, 1500, 0)}
	got, err := blitz.Pair(players, blitz.Criteria{Timezone: true, Rating: true}, 5)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}
	if want := []string{"1-3", "2-4"}; !slices.Equal(pairs(got), want) {
		t.Errorf("pairs = %v, want %v", pairs(got), want)
	}
}

func TestPairIsTheSameForASeedAndVariesAcrossSeeds(t *testing.T) {
	var players []blitz.Player
	for i := int64(1); i <= 8; i++ {
		players = append(players, rated(i, 0, 1500, 0))
	}
	first, err := blitz.Pair(players, blitz.Criteria{}, 11)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}
	again, _ := blitz.Pair(players, blitz.Criteria{}, 11)
	if !slices.Equal(pairs(first), pairs(again)) {
		t.Errorf("seed 11 twice = %v and %v", pairs(first), pairs(again))
	}
	seen := map[string]bool{}
	for seed := uint64(1); seed <= 20; seed++ {
		r, _ := blitz.Pair(players, blitz.Criteria{}, seed)
		seen[fmt.Sprint(pairs(r))] = true
	}
	if len(seen) < 2 {
		t.Errorf("20 seeds gave %d pairing(s), want chance to vary them", len(seen))
	}
}

func TestPairHandlesATwentyFourPlayerRoundOne(t *testing.T) {
	var players []blitz.Player
	for i := int64(1); i <= 24; i++ {
		zone := []float64{2, -4, 8}[i%3]
		players = append(players, rated(i, zone, 1500+float64(i)*10, 0))
	}
	got, err := blitz.Pair(players, blitz.Criteria{Timezone: true}, 2)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}
	if len(got.Matches) != 12 || got.Bye != nil {
		t.Fatalf("matches = %d, bye = %v; want 12 and none", len(got.Matches), got.Bye)
	}
	for _, m := range got.Matches {
		if m.ZoneGap != 0 {
			t.Errorf("match %d-%d crosses zones (gap %d)", m.A, m.B, m.ZoneGap)
		}
	}
}

func TestPairRefusesFewerThanTwoPlayers(t *testing.T) {
	if _, err := blitz.Pair([]blitz.Player{rated(1, 0, 1500, 0)}, blitz.Criteria{}, 1); !errors.Is(err, blitz.ErrTooFew) {
		t.Errorf("Pair(one player) = %v, want ErrTooFew", err)
	}
}
