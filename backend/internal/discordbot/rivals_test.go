package discordbot_test

import (
	"fmt"
	"strings"
	"testing"

	"ttacommunity/backend/internal/discordbot"
)

func TestRivalsTextShowsTheFiveMostPlayed(t *testing.T) {
	var rivals []discordbot.Rival
	for i := range 7 {
		rivals = append(rivals, discordbot.Rival{Name: fmt.Sprintf("Rival%d", i), Games: 70 - i})
	}
	text := discordbot.RivalsText("Quill", 0, rivals, "https://site.example")
	if !strings.Contains(text, "Rival4") || strings.Contains(text, "Rival5") {
		t.Errorf("RivalsText = %q, want Rival0 to Rival4 and no more", text)
	}
}

func TestRivalsTextEscapesNamesAndSaysWhenThereAreNone(t *testing.T) {
	tests := []struct {
		name   string
		rivals []discordbot.Rival
		want   string
	}{
		{"a rival's name escaped", []discordbot.Rival{{Name: "Rook_1", Games: 3}}, `Rook\_1`},
		{"none", nil, "no rated 2-player games"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := discordbot.RivalsText("Quill", 2, tt.rivals, "https://site.example"); !strings.Contains(got, tt.want) {
				t.Errorf("RivalsText = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}
