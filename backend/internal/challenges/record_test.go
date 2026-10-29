package challenges_test

import (
	"testing"
	"time"

	"ttacommunity/backend/internal/challenges"
)

func TestRecord(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 1, d, 12, 0, 0, 0, time.UTC) }
	entry := func(id int64, player string, domination int, verified bool, at time.Time) challenges.Entry {
		return challenges.Entry{ID: id, Challenge: "c", Player: player, Domination: domination, Verified: verified, At: at}
	}
	bugged := func(e challenges.Entry) challenges.Entry {
		e.Bugged = true
		return e
	}
	tests := []struct {
		name         string
		entries      []challenges.Entry
		verifiedOnly bool
		want         int64
	}{
		{"no entries: no record", nil, false, 0},
		{"the highest domination", []challenges.Entry{entry(1, "a", 90, true, day(1)), entry(2, "b", 120, true, day(2))}, true, 2},
		{"a bugged entry never counts", []challenges.Entry{bugged(entry(1, "a", 900, true, day(1))), entry(2, "b", 120, true, day(2))}, true, 2},
		{"only bugged entries: no record", []challenges.Entry{bugged(entry(1, "a", 900, true, day(1)))}, false, 0},
		{"verified only leaves out an unproven entry", []challenges.Entry{entry(1, "a", 150, false, day(1)), entry(2, "b", 120, true, day(2))}, true, 2},
		{"verified only, none verified: no record", []challenges.Entry{entry(1, "a", 150, false, day(1))}, true, 0},
		{"all entries count an unproven one", []challenges.Entry{entry(1, "a", 150, false, day(1)), entry(2, "b", 120, true, day(2))}, false, 1},
		{"a tie between players goes to the earlier", []challenges.Entry{entry(1, "a", 100, true, day(3)), entry(2, "b", 100, true, day(2))}, true, 2},
		{"a tie at the same time goes to the first entered", []challenges.Entry{entry(7, "a", 100, true, day(2)), entry(3, "b", 100, true, day(2))}, true, 3},
		{"a player's verified entry wins their own tie, even when later",
			[]challenges.Entry{entry(1, "a", 100, false, day(1)), entry(2, "b", 100, true, day(2)), entry(3, "a", 100, true, day(3))}, false, 2},
		{"a player's earlier entry wins their own tie when both are verified",
			[]challenges.Entry{entry(4, "a", 100, true, day(4)), entry(2, "a", 100, true, day(1))}, true, 2},
		{"a negative domination holds the record when nothing higher counts", []challenges.Entry{entry(1, "a", -20, true, day(1)), entry(2, "b", -5, true, day(2))}, true, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := int64(0)
			if board := challenges.Compute([]challenges.Listed{{ID: "c", Set: "mix"}}, tt.entries, tt.verifiedOnly, "").Boards["c"]; len(board) > 0 {
				got = board[0].ID
			}
			if got != tt.want {
				t.Errorf("record (verified only %v) holder = entry %d, want entry %d", tt.verifiedOnly, got, tt.want)
			}
		})
	}
}

func TestPlayerKey(t *testing.T) {
	five, six := int64(5), int64(6)
	tests := []struct {
		name   string
		a, b   challenges.Player
		wantEq bool
	}{
		{"one member under two names", challenges.Player{Member: &five, Name: "Ann"}, challenges.Player{Member: &five, Name: "Annie"}, true},
		{"two members of one name", challenges.Player{Member: &five, Name: "Ann"}, challenges.Player{Member: &six, Name: "Ann"}, false},
		{"unlinked names differing in case", challenges.Player{Name: "Ann"}, challenges.Player{Name: "aNN"}, true},
		{"unlinked names differing in spelling", challenges.Player{Name: "Ann"}, challenges.Player{Name: "Anne"}, false},
		{"a member and an unlinked name", challenges.Player{Member: &five, Name: "Ann"}, challenges.Player{Name: "Ann"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Key() == tt.b.Key(); got != tt.wantEq {
				t.Errorf("%+v and %+v same key = %v, want %v", tt.a, tt.b, got, tt.wantEq)
			}
		})
	}
}
