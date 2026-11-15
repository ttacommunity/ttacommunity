package store

import (
	"slices"
	"testing"
)

func TestBotAnswerFollowsItsRule(t *testing.T) {
	choices := []NostradamusChoice{{Label: "Ann", Seed: 2, LeaderboardRank: 30}, {Label: "Bea", Seed: 1}, {Label: "Cy", Seed: 3, LeaderboardRank: 10}}
	glicko := map[string]float64{"ann": 1900, "bea": 1700, "cy": 1800}
	pick := NostradamusQuestion{Kind: "pick", Picks: 1, Choices: choices}
	last := NostradamusQuestion{Kind: "pick", Picks: 1, AsksLast: true, Choices: choices}
	top2 := NostradamusQuestion{Kind: "set", Picks: 2, Choices: choices}
	tests := []struct {
		name, rule string
		question   NostradamusQuestion
		want       []string
	}{
		{"the best seed", "seed", pick, []string{"Bea"}},
		{"the best leaderboard rank, of those with one", "leaderboard", pick, []string{"Cy"}},
		{"the best Glicko", "glicko", pick, []string{"Ann"}},
		{"the worst seed when it asks who is last", "seed", last, []string{"Cy"}},
		{"the worst Glicko when it asks who is last", "glicko", last, []string{"Bea"}},
		{"the best two seeds for a set of two", "seed", top2, []string{"Bea", "Ann"}},
		{"none when too few have a rank for a set", "leaderboard", NostradamusQuestion{Kind: "set", Picks: 3, Choices: choices}, nil},
		{"none when no choice is rated", "glicko", NostradamusQuestion{Kind: "pick", Picks: 1, Choices: []NostradamusChoice{{Label: "Group A"}}}, nil},
		{"none for a number", "seed", NostradamusQuestion{Kind: "number", Picks: 1}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := botAnswer(tt.rule, tt.question, glicko); !slices.Equal(got, tt.want) {
				t.Errorf("botAnswer(%s) = %q, want %q", tt.rule, got, tt.want)
			}
		})
	}
}
