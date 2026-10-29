package challenges_test

import (
	"math"
	"testing"
	"time"

	"ttacommunity/backend/internal/challenges"
)

// list is two original challenges and one mix challenge.
var list = []challenges.Listed{{ID: "a", Set: "original"}, {ID: "b", Set: "original"}, {ID: "m", Set: "mix"}}

func day(d int) time.Time { return time.Date(2026, 1, d, 12, 0, 0, 0, time.UTC) }

// scored is an entry by name, verified when it has a proof link.
func scored(id int64, challenge, name string, domination int, proof string, at time.Time) challenges.Entry {
	return challenges.Entry{ID: id, Challenge: challenge, Player: challenges.Player{Name: name}.Key(), Name: name,
		Domination: domination, ProofURL: proof, Verified: proof != "", At: at}
}

func TestBoardRanks(t *testing.T) {
	entries := []challenges.Entry{
		scored(1, "a", "Ann", 100, "p1", day(1)),
		scored(2, "a", "Bob", 100, "p2", day(2)),
		scored(3, "a", "Cy", 50, "p3", day(3)),
		scored(4, "a", "Ann", 80, "p4", day(4)),
		scored(5, "a", "Dee", 300, "", day(5)),
	}
	tests := []struct {
		name         string
		verifiedOnly bool
		want         []string
		ranks        []int
	}{
		{"verified only: each player's best, ties sharing a rank", true, []string{"Ann", "Bob", "Cy"}, []int{1, 1, 3}},
		{"all entries count the unproven one", false, []string{"Dee", "Ann", "Bob", "Cy"}, []int{1, 2, 2, 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			board := challenges.Compute(list, entries, tt.verifiedOnly, "").Boards["a"]
			if len(board) != len(tt.want) {
				t.Fatalf("board has %d players, want %d", len(board), len(tt.want))
			}
			for i, b := range board {
				if b.Name != tt.want[i] || b.Rank != tt.ranks[i] {
					t.Errorf("place %d = %s #%d, want %s #%d", i, b.Name, b.Rank, tt.want[i], tt.ranks[i])
				}
			}
		})
	}
}

func TestPlayerSummary(t *testing.T) {
	entries := []challenges.Entry{
		scored(1, "a", "Ann", 100, "p1", day(1)),
		scored(2, "a", "Bob", 50, "p2", day(2)),
		scored(3, "b", "Bob", -20, "p3", day(3)),
		scored(4, "m", "Bob", 40, "", day(4)),
		scored(5, "b", "bob", 10, "", day(5)),
	}
	tests := []struct {
		name         string
		verifiedOnly bool
		player       string
		played       int
		records      int
		total        int
		rating       float64
		original     float64
	}{
		{"a record rates 100% of its challenge", true, "ann", 1, 1, 100, 100.0 / 3, 50},
		{"a negative domination counts as 0, and holds a record nobody beat", true, "bob", 2, 1, 50, 50.0 / 3, 25},
		{"all entries: names match ignoring case, the best counting", false, "bob", 3, 2, 100, (50 + 100 + 100) / 3.0, 75},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := challenges.Compute(list, entries, tt.verifiedOnly, "").Players["name:"+tt.player]
			if p == nil {
				t.Fatalf("no player %q", tt.player)
			}
			if p.Played != tt.played || p.Records != tt.records || p.Total != tt.total {
				t.Errorf("played, records, total = %d, %d, %d, want %d, %d, %d", p.Played, p.Records, p.Total, tt.played, tt.records, tt.total)
			}
			if math.Abs(p.Rating-tt.rating) > 1e-9 || math.Abs(p.Ratings["original"]-tt.original) > 1e-9 {
				t.Errorf("rating, original = %v, %v, want %v, %v", p.Rating, p.Ratings["original"], tt.rating, tt.original)
			}
		})
	}
}

func TestNewestNameAndLastEntry(t *testing.T) {
	entries := []challenges.Entry{
		scored(1, "a", "ann", 100, "p1", day(1)),
		scored(2, "b", "Ann", 10, "", day(9)),
	}
	p := challenges.Compute(list, entries, true, "").Players["name:ann"]
	if p.Name != "Ann" {
		t.Errorf("name = %q, want the newest entry's %q", p.Name, "Ann")
	}
	if !p.LastEntry.Equal(day(9)) {
		t.Errorf("last entry = %v, want %v: an unproven entry is still an entry", p.LastEntry, day(9))
	}
}

func TestBuggedNeverCounts(t *testing.T) {
	bugged := scored(2, "a", "Bob", 900, "p2", day(2))
	bugged.Bugged = true
	stats := challenges.Compute(list, []challenges.Entry{scored(1, "a", "Ann", 100, "p1", day(1)), bugged}, false, "")
	if board := stats.Boards["a"]; len(board) != 1 || board[0].Name != "Ann" {
		t.Errorf("board = %v, want Ann alone", board)
	}
	if _, ok := stats.Players["name:bob"]; ok {
		t.Error("Bob is listed with only a bugged entry")
	}
}

func TestAsIfVerified(t *testing.T) {
	entries := []challenges.Entry{
		scored(1, "a", "Ann", 100, "p1", day(1)),
		scored(2, "a", "Bob", 50, "p2", day(2)),
		scored(3, "a", "Bob", 150, "", day(3)),
		scored(4, "m", "Bob", 30, "", day(4)),
	}
	stats := challenges.Compute(list, entries, true, "name:bob")
	bob := stats.Players["name:bob"]
	tests := []struct {
		challenge string
		rank      int
		share     float64
	}{
		{"a", 1, 150},
		{"m", 1, 100},
	}
	for _, tt := range tests {
		b := bob.Bests[tt.challenge]
		if b.Rank != tt.rank || math.Abs(b.Share-tt.share) > 1e-9 {
			t.Errorf("%s: rank %d share %v, want %d, %v", tt.challenge, b.Rank, b.Share, tt.rank, tt.share)
		}
	}
	if board := stats.Boards["a"]; board[0].Name != "Ann" || stats.Players["name:ann"].Records != 1 {
		t.Error("the official board changed: Ann should still hold the record")
	}
}

func TestScope(t *testing.T) {
	entries := []challenges.Entry{
		scored(1, "a", "Ann", 100, "p1", day(1)),
		scored(2, "b", "Ann", 10, "p2", day(2)),
		scored(3, "a", "Bob", 60, "p3", day(3)),
		scored(4, "b", "Bob", 60, "p4", day(4)),
	}
	s := challenges.Compute(list, entries, true, "").Scope([]string{"a", "b", "m"})
	if s.Pooled != 160 || s.Covered != 2 || s.Size != 3 {
		t.Errorf("pooled, covered, size = %d, %d, %d, want 160, 2, 3", s.Pooled, s.Covered, s.Size)
	}
	if s.ByTotal[0].Name != "Bob" || s.ByRating[0].Name != "Bob" {
		t.Errorf("best by total %s, by rating %s, want Bob both", s.ByTotal[0].Name, s.ByRating[0].Name)
	}
	if got := challenges.Compute(list, entries, true, "").Scope([]string{"a"}); got.ByTotal[0].Name != "Ann" {
		t.Errorf("best on a = %s, want Ann", got.ByTotal[0].Name)
	}
}

func TestRecordSteps(t *testing.T) {
	entries := []challenges.Entry{
		scored(1, "a", "Ann", 40, "p1", day(1)),
		scored(2, "a", "Bob", 30, "p2", day(2)),
		scored(3, "a", "Cy", 90, "", day(3)),
		scored(4, "a", "Bob", 70, "p4", day(4)),
	}
	tests := []struct {
		name         string
		verifiedOnly bool
		want         []int
		gains        []int
	}{
		{"verified only", true, []int{40, 70}, []int{0, 30}},
		{"all entries", false, []int{40, 90}, []int{0, 50}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := challenges.Steps(entries, tt.verifiedOnly)
			if len(steps) != len(tt.want) {
				t.Fatalf("%d steps, want %d", len(steps), len(tt.want))
			}
			if steps[0].Gain != nil {
				t.Errorf("the first step has a gain of %d, want none", *steps[0].Gain)
			}
			for i, s := range steps {
				if s.Domination != tt.want[i] || (i > 0 && *s.Gain != tt.gains[i]) {
					t.Errorf("step %d = %d, want %d (+%d)", i, s.Domination, tt.want[i], tt.gains[i])
				}
			}
		})
	}
}
