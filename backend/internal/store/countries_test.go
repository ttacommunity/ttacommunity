package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryCountryAMemberMayHaveHasAFlag(t *testing.T) {
	if len(countries) < 249 {
		t.Fatalf("countries has %d codes, want ISO 3166-1's 249 at least", len(countries))
	}
	for code := range countries {
		path := filepath.Join("..", "..", "..", "web", "site", "public", "flags", strings.ToLower(code)+".svg")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("country %s: no flag at %s", code, path)
		}
	}
}
