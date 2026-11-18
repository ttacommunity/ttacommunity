package discordbot_test

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"ttacommunity/backend/internal/discordbot"
)

func TestParseItems(t *testing.T) {
	tests := []struct {
		name, in string
		want     []string
	}{
		{"trimmed", "Alex, Beta ,Claus", []string{"Alex", "Beta", "Claus"}},
		{"empty entries dropped", ",Alex,, ,Beta,", []string{"Alex", "Beta"}},
		{"duplicates kept", "Alex, Alex", []string{"Alex", "Alex"}},
		{"one", "Alex", []string{"Alex"}},
		{"the most allowed", strings.Repeat("a,", 99) + "a", slices.Repeat([]string{"a"}, 100)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := discordbot.ParseItems(tt.in)
			if err != nil || !slices.Equal(got, tt.want) {
				t.Errorf("ParseItems(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
			}
		})
	}
}

func TestParseItemsRefuses(t *testing.T) {
	tests := []struct{ name, in string }{
		{"nothing", ""},
		{"only commas and spaces", " , ,, "},
		{"over 100 entries", strings.Repeat("a,", 100) + "a"},
		{"over 1000 characters", strings.Repeat("a", 1001)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := discordbot.ParseItems(tt.in); err == nil {
				t.Errorf("ParseItems(%q) = %q, want it refused", tt.in, got)
			}
		})
	}
}

// firstPick always picks the first of the entries left, so a draw takes the entries in their order.
func firstPick(int) int { return 0 }

// lastPick always picks the last of the entries left.
func lastPick(n int) int { return n - 1 }

func TestChooseKeepsTheGivenOrder(t *testing.T) {
	items := []string{"Alex", "Beta", "Claus", "Dennis"}
	got, err := discordbot.Choose(items, 2, lastPick)
	if err != nil || !slices.Equal(got, []string{"Alex", "Dennis"}) {
		t.Errorf("Choose(%q, 2, last) = %q, %v; want [Alex Dennis] in the order given", items, got, err)
	}
}

func TestPermuteShowsTheDrawsOrder(t *testing.T) {
	items := []string{"Alex", "Beta", "Claus", "Dennis"}
	tests := []struct {
		name  string
		count int
		pick  func(int) int
		want  []string
	}{
		{"all, first each time", 4, firstPick, []string{"Alex", "Beta", "Claus", "Dennis"}},
		{"all, last each time", 4, lastPick, []string{"Dennis", "Alex", "Beta", "Claus"}},
		{"two, last each time", 2, lastPick, []string{"Dennis", "Alex"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := discordbot.Permute(items, tt.count, tt.pick)
			if err != nil || !slices.Equal(got, tt.want) {
				t.Errorf("Permute(%q, %d) = %q, %v; want %q", items, tt.count, got, err, tt.want)
			}
		})
	}
}

func TestADrawPicksAmongTheEntriesLeft(t *testing.T) {
	var asked []int
	pick := func(n int) int { asked = append(asked, n); return 0 }
	if _, err := discordbot.Permute([]string{"a", "b", "c", "d", "e"}, 3, pick); err != nil {
		t.Fatalf("Permute: %v", err)
	}
	if !slices.Equal(asked, []int{5, 4, 3}) {
		t.Errorf("picks asked among %v, want [5 4 3]", asked)
	}
}

func TestADrawRefusesACountOutsideTheEntries(t *testing.T) {
	items := []string{"Alex", "Beta", "Claus"}
	for _, count := range []int{0, -1, 4} {
		if got, err := discordbot.Choose(items, count, firstPick); err == nil {
			t.Errorf("Choose(%d of 3) = %q, want it refused", count, got)
		}
		if got, err := discordbot.Permute(items, count, firstPick); err == nil {
			t.Errorf("Permute(%d of 3) = %q, want it refused", count, got)
		}
	}
}

func TestEveryOrderOfFourCanBeDrawn(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	seen := map[string]bool{}
	for range 2000 {
		got, err := discordbot.Permute([]string{"a", "b", "c", "d"}, 4, r.IntN)
		if err != nil {
			t.Fatalf("Permute: %v", err)
		}
		seen[strings.Join(got, "")] = true
	}
	if len(seen) != 24 {
		t.Errorf("drew %d of the 24 orders of four, want all", len(seen))
	}
}

func TestDrawTextShowsTheResultThenWhatWasGiven(t *testing.T) {
	many := strings.Split(strings.Repeat("********,", 100)[:899], ",")
	tests := []struct {
		name, want   string
		drawn, given []string
	}{
		{"a draw", "**Result:** Beta, Dennis\nOriginal: Alex, Beta, Claus, Dennis",
			[]string{"Beta", "Dennis"}, []string{"Alex", "Beta", "Claus", "Dennis"}},
		{"escaped as written", `**Result:** \*Dennis\*` + "\nOriginal: Beta, " + `\*Dennis\*`,
			[]string{"*Dennis*"}, []string{"Beta", "*Dennis*"}},
		{"too long to repeat the original", "\nOriginal: 100 names, too long to repeat", many, many},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := discordbot.DrawText(tt.drawn, tt.given)
			if !strings.HasSuffix(got, tt.want) || utf8.RuneCountInString(got) > 2000 {
				t.Errorf("DrawText = %q, want it to end %q within 2000 characters", got, tt.want)
			}
		})
	}
}
