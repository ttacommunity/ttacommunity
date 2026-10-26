package discordbot_test

import (
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/discordbot"
)

func TestEscapeMarkdown(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"plain", "Grozz", "Grozz"},
		{"an underscore", "Martin_Pecheur", `Martin\_Pecheur`},
		{"stars and tildes", "*x*~~y~~", `\*x\*\~\~y\~\~`},
		{"a backslash", `a\b`, `a\\b`},
		{"code and spoilers", "`a`||b||", "\\`a\\`\\|\\|b\\|\\|"},
		{"a link", "[a](b)", `\[a\]\(b\)`},
		{"a mention", "@everyone <@1>", `@everyone \<@1\>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := discordbot.EscapeMarkdown(tt.in); got != tt.want {
				t.Errorf("EscapeMarkdown(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPlayerURL(t *testing.T) {
	tests := []struct{ name, site, player, want string }{
		{"a plain name", "https://site.example", "Grozz", "https://site.example/ratings/player/?name=Grozz"},
		{"a name needing escapes", "https://site.example", "a b/c", "https://site.example/ratings/player/?name=a+b%2Fc"},
		{"a site with a trailing slash", "https://site.example/", "Grozz", "https://site.example/ratings/player/?name=Grozz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := discordbot.PlayerURL(tt.site, tt.player); got != tt.want {
				t.Errorf("PlayerURL(%q, %q) = %q, want %q", tt.site, tt.player, got, tt.want)
			}
		})
	}
}

func TestSearchTerm(t *testing.T) {
	tests := []struct{ name, typed, want string }{
		{"as typed", "groz", "groz"},
		{"without the spaces around it", "  groz ", "groz"},
		{"a comma kept, as a name may hold one", "a,b", "a,b"},
		{"cut to 100 characters", strings.Repeat("x", 150), strings.Repeat("x", 100)},
		{"cut between characters, not bytes", strings.Repeat("é", 150), strings.Repeat("é", 100)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := discordbot.SearchTerm(tt.typed); got != tt.want {
				t.Errorf("SearchTerm(%q) = %q, want %q", tt.typed, got, tt.want)
			}
		})
	}
}

func TestRatingTextNamesThePlayerSafelyAndLinksTheirPage(t *testing.T) {
	last := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	text := discordbot.RatingText(discordbot.Summary{Name: "Player_One", Format: 0, Games: 10, LastPlayed: &last}, "https://site.example")
	for _, want := range []string{`Player\_One`, "https://site.example/ratings/player/?name=Player_One"} {
		if !strings.Contains(text, want) {
			t.Errorf("RatingText = %q, want it to contain %q", text, want)
		}
	}
}

func TestOwnRatingProblem(t *testing.T) {
	const site = "https://site.example"
	tests := []struct {
		state       string
		wantProblem bool
		wantAccount bool
	}{
		{"found", false, false},
		{"no_player", true, false},
		{"not_linked", true, true},
		{"no_login", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := discordbot.OwnRatingProblem(tt.state, site)
			if (got != "") != tt.wantProblem {
				t.Errorf("OwnRatingProblem(%q) = %q, want a problem: %v", tt.state, got, tt.wantProblem)
			}
			if tt.wantAccount && !strings.Contains(got, site+"/account/") {
				t.Errorf("OwnRatingProblem(%q) = %q, want it to point to the account page", tt.state, got)
			}
		})
	}
}
