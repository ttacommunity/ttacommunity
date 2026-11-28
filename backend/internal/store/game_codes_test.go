//go:build integration

package store

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestGameCodesAreStoredInCapitals(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	placedOn(t, pool, map[string]time.Time{"Intermezzo S24 - Bronze 1 game 1": day("2024-09-16")})
	tests := []struct {
		name  string
		typed *string
		want  []string
	}{
		{"lower case", new("abcdefgh"), []string{"ABCDEFGH"}},
		{"mixed case", new("Vipafibuma"), []string{"VIPAFIBUMA"}},
		{"already capitals", new("ZESODEFEMA"), []string{"ZESODEFEMA"}},
		{"none", nil, []string{"none"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, "UPDATE games SET share_code = $1", tt.typed); err != nil {
				t.Fatalf("coding the game: %v", err)
			}
			if got := rowsOf(t, pool, "SELECT coalesce(share_code, 'none') FROM games"); !slices.Equal(got, tt.want) {
				t.Errorf("share_code = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMigration108CapitalisesTheCodesAlreadyStored(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1": day("2024-09-16"),
		"Intermezzo S24 - Bronze 1 game 2": day("2024-09-17"),
	})
	codes := "SELECT share_code FROM games ORDER BY name"
	code := func(name, code string) {
		t.Helper()
		if _, err := pool.Exec(ctx, "UPDATE games SET share_code = $2 WHERE name LIKE $1", name, code); err != nil {
			t.Fatalf("coding %s: %v", name, err)
		}
	}
	migrateTo(t, pool, 107, true)
	code("%game 1", "jecenuduma")
	code("%game 2", "BBBBBBBB")
	migrateTo(t, pool, 108, false)
	if got, want := rowsOf(t, pool, codes), []string{"JECENUDUMA", "BBBBBBBB"}; !slices.Equal(got, want) {
		t.Errorf("codes at 108 = %q, want %q", got, want)
	}
	migrateTo(t, pool, 107, true)
	code("%game 2", "bbbbbbbb")
	if got, want := rowsOf(t, pool, codes), []string{"JECENUDUMA", "bbbbbbbb"}; !slices.Equal(got, want) {
		t.Errorf("codes at 107 = %q, want %q: stored as given again", got, want)
	}
}
