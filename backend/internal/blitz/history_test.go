package blitz_test

import (
	"testing"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/pairing"
)

func TestHistories(t *testing.T) {
	win := func(round int, a, b int64) blitz.Played {
		return blitz.Played{Round: round, A: a, B: b, Result: blitz.WinA}
	}
	tests := []struct {
		name    string
		matches []blitz.Played
		skipped []int
		rounds  []int
		want    blitz.History
	}{
		{"equal scores: no float", []blitz.Played{win(1, 1, 2)}, nil, []int{1}, blitz.History{}},
		{
			"the higher score floats down",
			[]blitz.Played{win(1, 1, 2), win(1, 3, 4), win(2, 1, 4)}, nil, []int{1, 2},
			blitz.History{Floats: [2]pairing.Float{pairing.Down, pairing.NoFloat}},
		},
		{
			"the lower score floats up",
			[]blitz.Played{win(1, 2, 1), win(1, 3, 4), win(2, 1, 3)}, nil, []int{1, 2},
			blitz.History{Floats: [2]pairing.Float{pairing.Up, pairing.NoFloat}},
		},
		{
			"a float moves back a round",
			[]blitz.Played{win(1, 1, 2), win(1, 3, 4), win(2, 1, 4), win(2, 3, 2), win(3, 1, 3)}, nil, []int{1, 2, 3},
			blitz.History{Floats: [2]pairing.Float{pairing.NoFloat, pairing.Down}},
		},
		{
			"the bye: a downfloat, a bye and an unplayed round",
			[]blitz.Played{{Round: 1, A: 1}}, nil, []int{1},
			blitz.History{Floats: [2]pairing.Float{pairing.Down, pairing.NoFloat}, HadBye: true, Unplayed: 1},
		},
		{
			"a forfeit win counts as a bye",
			[]blitz.Played{{Round: 1, A: 1, B: 2, Result: blitz.WinA, Forfeit: true}}, nil, []int{1},
			blitz.History{Floats: [2]pairing.Float{pairing.Down, pairing.NoFloat}, HadBye: true, Unplayed: 1},
		},
		{
			"a forfeit loss is an unplayed round, not a bye",
			[]blitz.Played{{Round: 1, A: 2, B: 1, Result: blitz.WinA, Forfeit: true}}, nil, []int{1},
			blitz.History{Unplayed: 1},
		},
		{"a skipped round is unplayed", nil, []int{1}, []int{1}, blitz.History{Unplayed: 1}},
		{"a round not counted is left out", []blitz.Played{{Round: 2, A: 1}}, nil, []int{1}, blitz.History{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entrants := []blitz.Entrant{{ID: 1, Skipped: tt.skipped}, {ID: 2}, {ID: 3}, {ID: 4}}
			got := blitz.Histories(entrants, tt.matches, tt.rounds, blitz.FIDEScoring)[1]
			if got != tt.want {
				t.Errorf("player 1's history = %+v, want %+v", got, tt.want)
			}
		})
	}
}
