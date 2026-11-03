package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ttacommunity/backend/internal/cgeextract"
	"ttacommunity/backend/internal/store"
)

func TestCGESummaryTellsTheDiffAndNamesNoPlayer(t *testing.T) {
	plan := store.CGEPlan{
		Games: []cgeextract.Game{
			{SessionID: 1, ShareCode: "CODEAAAA", State: "finished", Seats: []cgeextract.Seat{{PlayerName: "secretname"}}},
			{SessionID: 2, ShareCode: "CODEBBBB", State: "running"},
			{SessionID: 3, ShareCode: "CODECCCC", State: "finished"},
		},
		Matching: cgeextract.Matching{Matched: map[int64]int{10: 1}, Added: []int{2}, HeldBack: []int{3}},
		RunHere:  4, Removed: []int64{20, 21}, RemovedPlayed: 1, Missing: []int64{30}, Partial: 1, TournamentsEmptied: []int{1001},
	}
	d := delivery{Dir: "/data/cge/extract/2026-11-03", Files: []deliveryFile{{Name: "tournaments_export.csv", Rows: 3},
		{Name: "tournaments_export_changes.csv.gz", Rows: 4, Changes: map[string]int{"new": 2, "changed": 1, "removed": 1}}},
		In: store.CGEInput{Extract: cgeextract.Extract{Tournaments: map[int]cgeextract.Tournament{999: {}, 1001: {}},
			Refused: []cgeextract.Refusal{{File: "/data/cge/extract/2026-11-03/tournaments_export.csv", Line: 7, Reason: cgeextract.BadID}}}}}
	stages := []store.CGEStageWrite{{StageID: 30, Settings: map[string]any{"points_2p": []float64{1, 0}}},
		{StageID: 31, Stage: "Royal League 6, Tiebreak", Disagree: []string{"each_plays_2p", "timeout_penalty"}}}
	applied := store.CGEApplied{BatchID: 9, TournamentsAdded: 1, TournamentsUpdated: 1, SettingsWritten: 1, GamesChanged: 5,
		GamesUnchanged: 6, StateChanges: map[string]int{"prepared → played": 3}, SeatsAdded: 3, SeatsRemoved: 2, AccountsAdded: 7,
		Relinked: 8, PlayersAdded: 9, PlayersRenamed: 5, Members: store.Linked{Accounts: 1, Players: 2, Renamed: 3}, GroupsLinked: 4,
		GamesMoved: 4, ModeratorsKept: map[int]int{999: 1}}
	summary := newCGESummary(d, plan, stages, applied, false)
	var text, js strings.Builder
	summary.writeText(&text)
	if err := json.NewEncoder(&js).Encode(summary); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"CGE delivery 2026-11-03 (a dry run: nothing written)",
		"files: tournaments_export.csv 3 rows; tournaments_export_changes.csv.gz 4 rows (2 new, 1 changed, 1 removed)",
		"refused rows: 1", "  tournaments_export.csv line 7: bad_id",
		"tournaments: 2 in the delivery; 1 added; 1 changed", "  every game removed (made private?): 1001",
		"stages: 2 take CGE's settings; 1 changed", "  settings their CGE stages disagree on, kept: 1",
		"    stage 31 (Royal League 6, Tiebreak): each_plays_2p, timeout_penalty",
		"games: 3 in the delivery", "  added: 1 (running 1)", "  changed: 5 (prepared → played 3)", "  unchanged: 6",
		"  removed by the changes file: 2 (1 played): games 20, 21", "  missing from the full file, kept: 1: games 30",
		"  held back as possible copies: 1: CODECCCC", "  partial games we don't hold, left out: 1",
		"  in seasons run on this site, left out: 4", "seats: 3 added; 2 dropped", "accounts: 7 added; 8 relinked",
		"players: 9 added; 5 renamed after their account", "members linked: 1 to an account, 2 to a player, 3 renamed",
		"groups: 4 linked to their CGE stage; 4 games moved into their CGE group",
		"moderators CGE no longer lists, kept: 1 (tournament 999: 1)", "special rules CGE no longer lists, kept: none"} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("summary lacks %q:\n%s", want, text.String())
		}
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(js.String()), &parsed); err != nil {
		t.Fatalf("the summary's JSON: %v", err)
	}
	if parsed["dry_run"] != true || parsed["games"].(map[string]any)["changed"] != 5.0 {
		t.Errorf("JSON = %s, want dry_run true and games.changed 5", js.String())
	}
	for _, out := range []string{text.String(), js.String()} {
		if strings.Contains(out, "secretname") {
			t.Errorf("summary names a player:\n%s", out)
		}
	}
}

func TestCGESummarySaysWhenItWrote(t *testing.T) {
	var text strings.Builder
	newCGESummary(delivery{Dir: "d/2026-11-03"}, store.CGEPlan{}, nil, store.CGEApplied{BatchID: 9}, true).writeText(&text)
	if !strings.Contains(text.String(), "CGE delivery 2026-11-03 (written as batch 9)") {
		t.Errorf("summary = %s, want it to say the batch it wrote", text.String())
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
			d, err := readDelivery(writeDelivery(t, tt.files))
			if err != nil {
				t.Fatalf("readDelivery: %v", err)
			}
			in := d.In
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
