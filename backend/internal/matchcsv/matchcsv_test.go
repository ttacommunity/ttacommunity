package matchcsv

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const header = "Tournament,Date,Player1,Score1,Player2,Score2,Player3,Score3,Player4,Score4,Player_Count\r\n"

func row(fields ...string) Row {
	return Row{File: "s1.csv", Line: 2, Fields: fields}
}

func TestReadKeepsFileAndStartLine(t *testing.T) {
	in := header +
		"Cup,2024-01-02,ann,1,bob,2,,,,,2\r\n" +
		"Cup,2024-01-03,\"multi\nline\",1,bob,2,,,,,2\r\n" +
		"Cup,2024-01-04,ann,3,bob,4,,,,,2\r\n"

	rows, err := Read("s1.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	wantLines := []int{2, 3, 5}
	if len(rows) != len(wantLines) {
		t.Fatalf("got %d rows, want %d", len(rows), len(wantLines))
	}
	for i, r := range rows {
		if r.File != "s1.csv" || r.Line != wantLines[i] {
			t.Errorf("row %d: got %s:%d, want s1.csv:%d", i, r.File, r.Line, wantLines[i])
		}
	}
	if rows[1].Fields[2] != "multi\nline" {
		t.Errorf("quoted field = %q, want %q", rows[1].Fields[2], "multi\nline")
	}
}

func TestReadKeepsRowsWithTheWrongColumnCount(t *testing.T) {
	rows, err := Read("s1.csv", strings.NewReader(header+"Cup,2024-01-02,ann,1\r\n"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(rows) != 1 || len(rows[0].Fields) != 4 {
		t.Fatalf("got %+v, want one row of 4 fields", rows)
	}
}

func TestReadRefusesAFileThatIsNotAMatchCSV(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "empty file", in: ""},
		{name: "other header", in: "Player,Rating\r\nann,1500\r\n"},
		{name: "header in another order", in: strings.Replace(header, "Tournament,Date", "Date,Tournament", 1)},
		{name: "unclosed quote", in: header + "Cup,\"2024-01-02,ann,1,bob,2,,,,,2\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Read("x.csv", strings.NewReader(tt.in))
			if err == nil {
				t.Fatalf("Read(%q): got no error, want one", tt.in)
			}
			if !strings.Contains(err.Error(), "x.csv") {
				t.Errorf("error %q does not name the file", err)
			}
		})
	}
}

func TestCheckAcceptsAMatch(t *testing.T) {
	m, reason := Check(row("Intermezzo S7", "2024-01-02", "ann", "331", "bob", "212", "cat", "-5.0", "", "", "3"))
	if reason != "" {
		t.Fatalf("Check: refused with %q, want accepted", reason)
	}
	want := Match{
		File:       "s1.csv",
		Line:       2,
		Tournament: "Intermezzo S7",
		Date:       time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		Players:    []Player{{"ann", 331}, {"bob", 212}, {"cat", -5}},
	}
	if len(m.Fields) != len(Header) || m.Fields[7] != "-5.0" {
		t.Errorf("fields = %q, want the row as it was read", m.Fields)
	}
	if m.File != want.File || m.Line != want.Line || m.Tournament != want.Tournament || !m.Date.Equal(want.Date) {
		t.Errorf("got %+v, want %+v", m, want)
	}
	if len(m.Players) != len(want.Players) {
		t.Fatalf("players = %v, want %v", m.Players, want.Players)
	}
	for i := range want.Players {
		if m.Players[i] != want.Players[i] {
			t.Errorf("player %d = %v, want %v", i, m.Players[i], want.Players[i])
		}
	}
}

func TestCheckRefuses(t *testing.T) {
	tests := []struct {
		name   string
		fields []string
		want   Reason
	}{
		{name: "too few columns", fields: []string{"Cup", "2024-01-02", "ann", "1"}, want: ColumnCount},
		{name: "empty tournament", fields: []string{" ", "2024-01-02", "ann", "1", "bob", "2", "", "", "", "", "2"}, want: NoTournament},
		{name: "date in another format", fields: []string{"Cup", "02/01/2024", "ann", "1", "bob", "2", "", "", "", "", "2"}, want: BadDate},
		{name: "date that does not exist", fields: []string{"Cup", "2024-02-30", "ann", "1", "bob", "2", "", "", "", "", "2"}, want: BadDate},
		{name: "score with no player", fields: []string{"Cup", "2024-01-02", "ann", "1", "bob", "2", "", "3", "", "", "2"}, want: ScoreWithoutPlayer},
		{name: "player with no score", fields: []string{"Cup", "2024-01-02", "ann", "1", "bob", "", "", "", "", "", "2"}, want: PlayerWithoutScore},
		{name: "gap before a player", fields: []string{"Cup", "2024-01-02", "ann", "1", "", "", "cat", "2", "", "", "2"}, want: PlayerGap},
		{name: "score not a number", fields: []string{"Cup", "2024-01-02", "ann", "one", "bob", "2", "", "", "", "", "2"}, want: BadScore},
		{name: "score not finite", fields: []string{"Cup", "2024-01-02", "ann", "NaN", "bob", "2", "", "", "", "", "2"}, want: BadScore},
		{name: "player count not a number", fields: []string{"Cup", "2024-01-02", "ann", "1", "bob", "2", "", "", "", "", "two"}, want: PlayerCount},
		{name: "player count disagrees", fields: []string{"Cup", "2024-01-02", "ann", "1", "bob", "2", "", "", "", "", "3"}, want: PlayerCount},
		{name: "one player", fields: []string{"Cup", "2024-01-02", "ann", "1", "", "", "", "", "", "", "1"}, want: PlayerCount},
		{name: "same player twice", fields: []string{"Cup", "2024-01-02", "ann", "1", "ann", "2", "", "", "", "", "2"}, want: SamePlayerTwice},
		{name: "standings rank prefix", fields: []string{"Cup", "2024-01-02", "1.45ann", "1", "bob", "2", "", "", "", "", "2"}, want: StandingsRow},
		{name: "standings no finished games", fields: []string{"Cup", "2024-01-02", "annNo finished games", "1", "bob", "2", "", "", "", "", "2"}, want: StandingsRow},
		{name: "name with a line break", fields: []string{"Cup", "2024-01-02", "ann", "1", "0 \n 0", "2", "", "", "", "", "2"}, want: BadName},
		{name: "name with a non-breaking space at the end", fields: []string{"Cup", "2024-01-02", "ann ", "1", "bob", "2", "", "", "", "", "2"}, want: BadName},
		{name: "name with a leading space", fields: []string{"Cup", "2024-01-02", " ann", "1", "bob", "2", "", "", "", "", "2"}, want: BadName},
		{name: "every score zero is a draw, as upstream rates it", fields: []string{"Cup", "2024-01-02", "ann", "0", "bob", "0.0", "cat", "0", "", "", "3"}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := Check(row(tt.fields...))
			if got != tt.want {
				t.Errorf("Check(%q) = %q, want %q", tt.fields, got, tt.want)
			}
		})
	}
}

func TestCheckKeepsPlayersWhoDifferOnlyByCase(t *testing.T) {
	_, reason := Check(row("Cup", "2024-01-02", "ann", "1", "Ann", "2", "", "", "", "", "2"))
	if reason != "" {
		t.Errorf("Check: refused with %q, want accepted", reason)
	}
}

func TestDedupeRefusesTheSecondCopyAcrossFiles(t *testing.T) {
	a := Match{File: "a.csv", Line: 2, Tournament: "Cup", Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), Players: []Player{{"ann", 1}, {"bob", 2}}}
	a.Fields = []string{"Cup", "2024-01-02", "ann", "1", "bob", "2.0", "", "", "", "", "2"}
	sameGame := a
	sameGame.File, sameGame.Line = "b.csv", 9
	otherScore := a
	otherScore.Players = []Player{{"ann", 1}, {"bob", 3}}
	swapped := a
	swapped.Players = []Player{{"bob", 2}, {"ann", 1}}
	spaceMoved := a
	spaceMoved.Players = []Player{{"ann 1", 1}, {"bob", 2}}
	spaceMoved2 := a
	spaceMoved2.Players = []Player{{"ann", 1}, {"1 bob", 2}}

	kept, dups := Dedupe([]Match{a, otherScore, sameGame, swapped, spaceMoved, spaceMoved2})

	if len(kept) != 5 || kept[0].File != "a.csv" || kept[1].Players[1].Score != 3 || kept[2].Players[0].Name != "bob" {
		t.Errorf("kept = %+v, want a.csv:2, the other score, and the swapped order", kept)
	}
	if len(dups) != 1 || dups[0].File != "b.csv" || dups[0].Line != 9 || dups[0].Reason != Duplicate {
		t.Fatalf("dups = %+v, want b.csv:9 refused as %q", dups, Duplicate)
	}
	if dups[0].Fields[5] != "2.0" {
		t.Errorf("refused fields = %q, want the row as it was read", dups[0].Fields)
	}
}

func TestReadErrorsWrapTheCause(t *testing.T) {
	_, err := Read("x.csv", errReader{})
	if !errors.Is(err, errBoom) {
		t.Errorf("Read: err = %v, want it to wrap %v", err, errBoom)
	}
}

var errBoom = errors.New("boom")

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errBoom }
