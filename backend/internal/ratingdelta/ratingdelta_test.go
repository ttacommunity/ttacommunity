package ratingdelta

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

type engineCase struct {
	Name    string       `json:"name"`
	Kind    string       `json:"kind"`
	Tau     float64      `json:"tau"`
	Player  [3]float64   `json:"player"`
	Matches [][6]float64 `json:"matches"`
	Want    [3]float64   `json:"want"`
}

func TestUpdateMatchesTheEngine(t *testing.T) {
	raw, err := os.ReadFile("testdata/engine_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []engineCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no cases in testdata/engine_cases.json")
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			player := Rating{Rating: tt.Player[0], RD: tt.Player[1], Sigma: tt.Player[2]}
			games := make([]Game, 0, len(tt.Matches))
			for _, m := range tt.Matches {
				games = append(games, Game{
					Opponent: Rating{Rating: m[0], RD: m[1], Sigma: m[2]},
					Outcome:  m[3], Weight: m[4], PlayerCount: int(m[5]),
				})
			}
			var got Rating
			switch tt.Kind {
			case "standard":
				got = Update(player, games, tt.Tau, false)
			case "weighted":
				got = Update(player, games, tt.Tau, true)
			case "adaptive":
				got = UpdateAdaptive(player, games, tt.Tau)
			default:
				t.Fatalf("unknown kind %q", tt.Kind)
			}
			for i, g := range []float64{got.Rating, got.RD, got.Sigma} {
				if math.Abs(g-tt.Want[i]) > 1e-9 {
					t.Errorf("got = %v, want %v", got, tt.Want)
					break
				}
			}
		})
	}
}

func TestDelta(t *testing.T) {
	ratings := map[string]Rating{
		"me":    {Rating: 1800, RD: 45},
		"alice": {Rating: 1900, RD: 35},
		"bob":   {Rating: 1600, RD: 80},
	}
	lookup := func(name string) (Rating, bool) {
		r, ok := ratings[name]
		return r, ok
	}
	at := func(name string) Rating {
		r, ok := ratings[name]
		if !ok {
			r = Rating{Rating: 1500, RD: 350}
		}
		r.Sigma = 0.03
		return r
	}
	threeSeats := []Seat{{"alice", 150}, {"me", 120}, {"bob", 90}}
	golden3 := (1 + math.Sqrt(5)) / 2 / 2

	tests := []struct {
		name   string
		engine string
		me     string
		seats  []Seat
		want   Rating
	}{
		{
			name: "GlickoD weights a 3P game by the golden weight", engine: "glicko2_daneo", me: "me", seats: threeSeats,
			want: Update(at("me"), []Game{{at("alice"), 0, golden3, 3}, {at("bob"), 1, golden3, 3}}, 0.3, true),
		},
		{
			name: "WHR is shown with GlickoD's update", engine: "whr", me: "me", seats: threeSeats,
			want: Update(at("me"), []Game{{at("alice"), 0, golden3, 3}, {at("bob"), 1, golden3, 3}}, 0.3, true),
		},
		{
			name: "MP weights by one over the opponents", engine: "glicko2_mp", me: "me", seats: threeSeats,
			want: Update(at("me"), []Game{{at("alice"), 0, 0.5, 3}, {at("bob"), 1, 0.5, 3}}, 0.3, true),
		},
		{
			name: "Standard ignores weights", engine: "glicko2_std", me: "me", seats: threeSeats,
			want: Update(at("me"), []Game{{at("alice"), 0, 1, 3}, {at("bob"), 1, 1, 3}}, 0.3, false),
		},
		{
			name: "Adaptive-T uses the adaptive update", engine: "glicko2_adapt", me: "me", seats: threeSeats,
			want: UpdateAdaptive(at("me"), []Game{{at("alice"), 0, 0.5, 3}, {at("bob"), 1, 0.5, 3}}, 0.3),
		},
		{
			name: "an equal score is a draw", engine: "glicko2_std", me: "me", seats: []Seat{{"me", 100}, {"alice", 100}},
			want: Update(at("me"), []Game{{at("alice"), 0.5, 1, 2}}, 0.3, false),
		},
		{
			name: "unrated players count as 1500 ± 350", engine: "glicko2_std", me: "newcomer",
			seats: []Seat{{"newcomer", 80}, {"stranger", 60}},
			want:  Update(at("newcomer"), []Game{{at("stranger"), 1, 1, 2}}, 0.3, false),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Delta(tt.engine, tt.me, len(tt.seats), tt.seats, lookup)
			want := round1(tt.want.Rating - at(tt.me).Rating)
			if !ok || got != want {
				t.Errorf("Delta(%q) = %v, %v; want %v, true", tt.engine, got, ok, want)
			}
		})
	}
}

func TestDeltaRefusesAGameWithoutOpponents(t *testing.T) {
	none := func(string) (Rating, bool) { return Rating{}, false }
	for _, seats := range [][]Seat{{{"me", 10}}, {{"alice", 10}, {"bob", 5}}} {
		if got, ok := Delta("glicko2_daneo", "me", len(seats), seats, none); ok {
			t.Errorf("Delta(%v) = %v, true; want no delta", seats, got)
		}
	}
}

func TestDeltaWeighsByTheMatchsPlayerCount(t *testing.T) {
	none := func(string) (Rating, bool) { return Rating{}, false }
	seats := []Seat{{"me", 10}, {"alice", 5}}
	fresh := Rating{Rating: 1500, RD: 350, Sigma: 0.03}
	want := round1(Update(fresh, []Game{{fresh, 1, math.Phi / 2, 3}}, 0.3, true).Rating - 1500)
	if got, ok := Delta("glicko2_daneo", "me", 3, seats, none); !ok || got != want {
		t.Errorf("Delta(3 players, 2 seats) = %v, %v; want %v, true", got, ok, want)
	}
}

func TestRound1MatchesPython(t *testing.T) {
	tests := []struct{ in, want float64 }{
		{2.14, 2.1}, {2.15000001, 2.2}, {0.25, 0.2}, {0.35, 0.3}, {-0.04, 0}, {-1.26, -1.3},
	}
	for _, tt := range tests {
		if got := round1(tt.in); got != tt.want {
			t.Errorf("round1(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
