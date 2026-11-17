package discordbot_test

import (
	"strings"
	"testing"

	"ttacommunity/backend/internal/discordbot"
)

func TestCompareTextShowsEachFormatOrADash(t *testing.T) {
	third := 3
	players := []discordbot.Compared{
		{Name: "Quill_1", Title: "SGM", Games: 481, Ratings: map[int]discordbot.Summary{
			0: {Rating: 1972, RD: 20, Rank: &third},
			2: {Rating: 1900.04, RD: 25.06},
		}},
		{Name: "Rook", Games: 12, Ratings: map[int]discordbot.Summary{0: {Rating: 1500, RD: 150}}},
	}
	text := discordbot.CompareText(players)
	lines := strings.Split(text, "\n")
	quill, rook := lineWith(lines, `Quill\_1`), lineWith(lines, "Rook")
	tests := []struct{ name, line, want string }{
		{"a ranked rating with its rank", quill, "1972.0 ± 20.0 (#3)"},
		{"an unranked rating", quill, "2p 1900.0 ± 25.1 ·"},
		{"a format with no rating", quill, "3p —"},
		{"the title", quill, "[SGM]"},
		{"the second player's own line", rook, "2p —"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.line, tt.want) {
				t.Errorf("CompareText = %q, want a line containing %q", text, tt.want)
			}
		})
	}
}

// lineWith is the first line holding s joined with the line after it (a player's ratings), or "".
func lineWith(lines []string, s string) string {
	for i, l := range lines {
		if strings.Contains(l, s) {
			return strings.Join(lines[i:min(i+2, len(lines))], " ")
		}
	}
	return ""
}
