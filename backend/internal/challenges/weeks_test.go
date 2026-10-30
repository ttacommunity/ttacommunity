package challenges_test

import (
	"strings"
	"testing"

	"ttacommunity/backend/internal/challenges"
)

func TestReadWeeks(t *testing.T) {
	week := `{"number": 1, "date": "2024-09-22", "challengeId": "cheap1_mix", "winners": [{"name": " Ann  B ", "score": 650}],
		"runnersUp": [{"name": "Bob", "score": 406}, {"name": "Cy", "score": 406}]}`
	got, err := challenges.ReadWeeks(strings.NewReader("[" + week + "]"))
	if err != nil {
		t.Fatalf("ReadWeeks: %v", err)
	}
	w := got[0]
	if w.Number != 1 || w.Of.Format("2006-01-02") != "2024-09-22" || w.Challenge != "cheap1_mix" {
		t.Errorf("week = %d %s %s, want 1 2024-09-22 cheap1_mix", w.Number, w.Of.Format("2006-01-02"), w.Challenge)
	}
	if len(w.Winners) != 1 || w.Winners[0].Name != "Ann B" || w.Winners[0].Domination != 650 {
		t.Errorf("winners = %+v, want Ann B with 650 (spaces collapsed)", w.Winners)
	}
	if len(w.RunnersUp) != 2 {
		t.Errorf("runners-up = %+v, want two sharing second place", w.RunnersUp)
	}
}

func TestReadWeeksRefuses(t *testing.T) {
	tests := []struct {
		name, file string
	}{
		{"not JSON", `nope`},
		{"no weeks", `[]`},
		{"a week numbered twice", `[{"number": 1, "date": "2024-09-22", "challengeId": "a"}, {"number": 1, "date": "2024-09-29", "challengeId": "b"}]`},
		{"a week numbered 0", `[{"number": 0, "date": "2024-09-22", "challengeId": "a"}]`},
		{"a date that is not one", `[{"number": 1, "date": "22/09/2024", "challengeId": "a"}]`},
		{"no challenge", `[{"number": 1, "date": "2024-09-22", "challengeId": ""}]`},
		{"a place with no name", `[{"number": 1, "date": "2024-09-22", "challengeId": "a", "winners": [{"name": " ", "score": 1}]}]`},
		{"a domination out of bounds", `[{"number": 1, "date": "2024-09-22", "challengeId": "a", "winners": [{"name": "Ann", "score": 10000}]}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := challenges.ReadWeeks(strings.NewReader(tt.file)); err == nil {
				t.Error("ReadWeeks took it, want an error")
			}
		})
	}
}
