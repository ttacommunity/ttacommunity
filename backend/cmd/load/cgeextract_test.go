package main

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ttacommunity/backend/internal/cgeextract"
	"ttacommunity/backend/internal/store"
)

func TestReportCGEExtractCountsAndNamesNoPlayer(t *testing.T) {
	plan := store.CGEPlan{
		Games: []cgeextract.Game{
			{SessionID: 1, ShareCode: "CODEAAAA", State: "finished", Seats: []cgeextract.Seat{{PlayerName: "secretname"}}},
			{SessionID: 2, ShareCode: "CODEBBBB", State: "running"},
			{SessionID: 3, ShareCode: "CODECCCC", State: "finished"},
		},
		Matching:      cgeextract.Matching{Matched: map[int64]int{10: 1}, Added: []int{2}, HeldBack: []int{3}, OursOnly: []int64{11, 12}},
		RunHere:       4,
		Removed:       []int64{20, 21},
		RemovedPlayed: 1,
	}
	stages := []store.CGEStageWrite{{StageID: 30, Settings: map[string]any{"points_2p": []float64{1, 0}}},
		{StageID: 31, Disagree: []string{"each_plays_2p", "timeout_penalty"}}}
	refused := []cgeextract.Refusal{{Line: 7, Reason: cgeextract.BadID}}
	var b strings.Builder

	reportCGEExtract(&b, plan, stages, refused, nil)

	got := b.String()
	for _, want := range []string{"CGE games: 3", "left out, in seasons run on this site: 4", "matched to ours: 1", "only CGE has: 1 (running 1)",
		"held back as possible copies: 1", "  CODECCCC", "ours only: 2", "removed by CGE: 2 (1 played)", "  game 20", "  game 21",
		"stages of ours with CGE stages now: 2", "settings their CGE stages disagree on, kept: 1", "  stage 31: each_plays_2p, timeout_penalty",
		"refused: 1", "  line 7 bad_id",
		"nothing written: add -apply"} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "secretname") {
		t.Errorf("report names a player:\n%s", got)
	}
}

func TestReportCGEExtractSaysWhatWasWritten(t *testing.T) {
	applied := store.CGEApplied{BatchID: 9, Added: 2, SeatsAdded: 3, CopiesRemoved: 1, SeatsRemoved: 2, GamesMoved: 4, PlayersRenamed: 5,
		GamesRemoved: 6}
	var b strings.Builder

	reportCGEExtract(&b, store.CGEPlan{}, nil, nil, &applied)

	got := b.String()
	for _, want := range []string{"batch 9: 2 games added", "3 seats added", "1 copies an earlier load added removed",
		"2 seats of players CGE removed dropped", "4 games moved into their CGE group", "5 players renamed after their account",
		"6 games CGE removed deleted"} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q:\n%s", want, got)
		}
	}
}

// deliveryRow is one extract row of game 5, seat of account player.
func deliveryRow(player int, name string) string {
	return "19,Cup,101,orga,,2020-04-07 20:42:28.000,2020-04-25 21:19:48.000,,3,1,,,7,Gold 1,5,ABCDEFGH,finished," +
		"2020-04-25 21:19:16.992,2020-05-12 07:34:01.739,Medium Async,48 h," + strconv.Itoa(player) + "," + name + ",0,2,1,100.0,finished"
}

// writeDelivery writes files, by name, into a new directory; a name ending .gz is gzipped.
func writeDelivery(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		var b bytes.Buffer
		if strings.HasSuffix(name, ".gz") {
			z := gzip.NewWriter(&b)
			if _, err := z.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
		} else {
			b.WriteString(body)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestReadDeliveryTakesWhicheverFilesItHolds(t *testing.T) {
	full := strings.Join(cgeextract.Header, ",") + "\n" + deliveryRow(1, "ann") + "\n"
	changes := strings.Join(cgeextract.ChangesHeader, ",") + "\nnew," + deliveryRow(2, "bob") + ",\n"
	stages := "tournament_id,tournament_name,stage_id,stage_number,stage_name,timeout_penalty,each_player_plays_2p,scoring_2p," +
		"each_player_plays_3p,scoring_3p,each_player_plays_4p,scoring_4p,ignore_ai_in_scoring,tiebreakers_2_players_tie," +
		"tiebreakers_3plus_players_tie,winner_score_minimum\n19,Cup,3,1,,0,1,1-0,0,,0,,no,,,\n"
	special := "tournament_id,tournament_name,session_id,share_code,rules,card_set\n19,Cup,5,ABCDEFGH,Tabletop,Random Mix\n"
	tests := []struct {
		name                            string
		files                           map[string]string
		wantSeats                       int
		wantFull, wantStages, wantRules bool
	}{
		{"every file", map[string]string{"tournaments_export.csv": full, "tournaments_export_changes.csv": changes,
			"tournaments_export_stages.csv": stages, "tournaments_export_special_rules.csv": special}, 2, true, true, true},
		{"the full extract gzipped", map[string]string{"tournaments_export.csv.gz": full}, 1, true, false, false},
		{"a changes file alone", map[string]string{"tournaments_export_changes.csv": changes}, 1, false, false, false},
		{"changes and stages", map[string]string{"tournaments_export_changes.csv.gz": changes,
			"tournaments_export_stages.csv": stages}, 1, false, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, err := readDelivery(writeDelivery(t, tt.files))
			if err != nil {
				t.Fatalf("readDelivery: %v", err)
			}
			if len(in.Extract.Seats) != tt.wantSeats || in.Extract.Full != tt.wantFull ||
				(in.Stages != nil) != tt.wantStages || (in.Special != nil) != tt.wantRules {
				t.Errorf("seats %d, full %t, stages %t, rules %t; want %d, %t, %t, %t", len(in.Extract.Seats), in.Extract.Full,
					in.Stages != nil, in.Special != nil, tt.wantSeats, tt.wantFull, tt.wantStages, tt.wantRules)
			}
		})
	}
}

func TestReadDeliveryRefusesAFolderWithNoGames(t *testing.T) {
	_, err := readDelivery(writeDelivery(t, map[string]string{"tournaments_export_stages.csv": "x"}))
	if err == nil || !strings.Contains(err.Error(), "tournaments_export_changes.csv") {
		t.Errorf("readDelivery of stages alone: err = %v, want one naming the files it needs", err)
	}
}
