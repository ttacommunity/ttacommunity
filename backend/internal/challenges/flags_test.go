package challenges_test

import (
	"strings"
	"testing"

	"ttacommunity/backend/internal/challenges"
)

func TestFlags(t *testing.T) {
	bugged := scored(5, "a", "Dee", 900, "", day(5))
	bugged.Bugged = true
	entries := []challenges.Entry{
		scored(1, "a", "Ann", 100, "p1", day(1)),
		scored(2, "a", "Bob", 250, "", day(2)),
		scored(3, "a", "Cy", 60, "p1", day(3)),
		scored(4, "b", "ann", 190, "", day(4)),
		bugged,
	}
	flags := challenges.Flags(entries)
	tests := []struct {
		id   int64
		want string
	}{
		{1, "proof also used by Cy"},
		{2, "no proof, over 2× everyone else's best (100 by Ann)"},
		{3, "proof also used by Ann"},
		{4, ""},
		{5, ""},
	}
	for _, tt := range tests {
		if got := strings.Join(flags[tt.id], "; "); got != tt.want {
			t.Errorf("entry %d flags = %q, want %q", tt.id, got, tt.want)
		}
	}
}
