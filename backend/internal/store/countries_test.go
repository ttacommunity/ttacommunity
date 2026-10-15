package store

import (
	"os"
	"path/filepath"
	"slices"
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

func TestCountryCodesListsEveryCountryOnceInOrder(t *testing.T) {
	got := CountryCodes()
	if len(got) != len(countries) || !slices.IsSorted(got) {
		t.Errorf("CountryCodes() = %d codes, sorted %v; want all %d, sorted", len(got), slices.IsSorted(got), len(countries))
	}
	for _, code := range got {
		if !countries[code] {
			t.Errorf("CountryCodes() has %q, which a member may not have", code)
		}
	}
}
