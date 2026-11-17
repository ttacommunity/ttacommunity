package discordbot_test

import (
	"fmt"
	"strings"
	"testing"

	"ttacommunity/backend/internal/discordbot"
)

func TestRivalsTextShowsTheMostPlayedItIsAskedFor(t *testing.T) {
	var rivals []discordbot.Rival
	for i := range 12 {
		rivals = append(rivals, discordbot.Rival{Name: fmt.Sprintf("Rival%02d_%s", i, strings.Repeat("x", 12)), Games: 70 - i, WinRate: 50})
	}
	for _, n := range []int{1, 5, 10} {
		text := discordbot.RivalsText("Quill", 0, rivals, n, "https://site.example")
		last, next := fmt.Sprintf("Rival%02d", n-1), fmt.Sprintf("Rival%02d", n)
		if !strings.Contains(text, last) || strings.Contains(text, next) || len([]rune(text)) > discordbot.MaxReply {
			t.Errorf("RivalsText for %d = %q, want up to %s and no more, within Discord's limit", n, text, last)
		}
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
			if got := discordbot.RivalsText("Quill", 2, tt.rivals, 5, "https://site.example"); !strings.Contains(got, tt.want) {
				t.Errorf("RivalsText = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}
