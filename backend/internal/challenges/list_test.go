package challenges_test

import (
	"strings"
	"testing"

	"ttacommunity/backend/internal/challenges"
)

const twoChallenges = `[
  {"id": "1ai", "set": "original", "group": "classic", "slot": 0, "difficulty": 1, "players": 2, "name": "First Steps",
   "description": [], "rules": ["Win against one AI."], "opponents": [{"level": "easy", "name": null, "flags": {}}], "agesToPlay": 3},
  {"id": "epic1_exp", "set": "expansion", "group": "epic", "slot": 1, "difficulty": 4, "players": 3, "name": "Long Road",
   "description": ["A longer game."], "rules": [], "opponents": []}
]`

func TestReadListKeepsEachChallengeInOrder(t *testing.T) {
	got, err := challenges.ReadList(strings.NewReader(twoChallenges))
	if err != nil {
		t.Fatalf("ReadList: %v", err)
	}
	if len(got) != 2 || got[0].ID != "1ai" || got[1].ID != "epic1_exp" {
		t.Fatalf("ReadList = %+v, want 1ai then epic1_exp", got)
	}
	second := got[1]
	if second.Set != "expansion" || second.Group != "epic" || second.Slot != 1 || second.Difficulty != 4 || second.Players != 3 ||
		second.Name != "Long Road" || strings.Join(second.Description, "|") != "A longer game." {
		t.Errorf("second challenge = %+v", second)
	}
	if string(got[0].Opponents) != `[{"level": "easy", "name": null, "flags": {}}]` {
		t.Errorf("opponents = %s, want them as written", got[0].Opponents)
	}
}

func TestReadListRefusesAListItCannotStore(t *testing.T) {
	valid := `{"id": "1ai", "set": "original", "group": "classic", "slot": 0, "difficulty": 1, "players": 2, "name": "A", "opponents": []}`
	tests := []struct{ name, file, want string }{
		{"not a list", `{"id": "1ai"}`, "not a JSON list"},
		{"an empty list", `[]`, "no challenges"},
		{"an id twice", "[" + valid + "," + valid + "]", `"1ai" is listed twice`},
		{"an unknown set", strings.Replace("["+valid+"]", "original", "base", 1), "set"},
		{"an id with capitals", strings.Replace("["+valid+"]", "1ai", "1AI", 1), "id"},
		{"five players", strings.Replace("["+valid+"]", `"players": 2`, `"players": 5`, 1), "players"},
		{"no name", strings.Replace("["+valid+"]", `"name": "A"`, `"name": " "`, 1), "name"},
		{"opponents that are not a list", strings.Replace("["+valid+"]", `"opponents": []`, `"opponents": {}`, 1), "opponents"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := challenges.ReadList(strings.NewReader(tt.file))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ReadList(%s) = %v, want an error naming %q", tt.name, err, tt.want)
			}
		})
	}
}
