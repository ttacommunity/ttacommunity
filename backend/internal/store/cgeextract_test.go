//go:build integration

package store

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/cgeextract"
)

func extractSeat(session, tournament, player int, name, code string, culture float64) cgeextract.Seat {
	return cgeextract.Seat{SessionID: session, TournamentID: tournament, PlayerID: player, PlayerName: name,
		ShareCode: code, State: "finished", FinishStatus: cgeextract.Finished, Score: &culture}
}

func TestPlanCGEExtractLeavesOutOurGamesFromBeforeCGEsData(t *testing.T) {
	tests := []struct {
		name, date  string
		wantMatched int
	}{
		{"a game from CGE's first day may be CGE's", "2020-04-15", 1},
		{"a game from before it is not", "2020-04-14", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			ctx := context.Background()
			placedOn(t, pool, map[string]time.Time{"Intermezzo S24 - Bronze 1 game 1": day(tt.date)})
			setSeats(t, pool, "%game 1", Seat{"ann", 200}, Seat{"bob", 150})
			x := cgeextract.Extract{Seats: []cgeextract.Seat{
				extractSeat(100, 19, 1, "ann", "CODEAAAA", 200), extractSeat(100, 19, 2, "bob", "CODEAAAA", 150)}}

			plan, err := PlanCGEExtract(ctx, pool, x, nil)
			if err != nil {
				t.Fatalf("PlanCGEExtract: %v", err)
			}
			if len(plan.Matching.Matched) != tt.wantMatched || len(plan.Matching.HeldBack) != 0 {
				t.Errorf("our game of %s: matched %v, held back %v; want %d matched, none held back", tt.date,
					plan.Matching.Matched, plan.Matching.HeldBack, tt.wantMatched)
			}
		})
	}
}

func TestPlanCGEExtractMatchesOurGamesByCodeAndByPlayers(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02"),
		"Intermezzo S24 - Bronze 1 game 2": day("2024-01-09"),
	})
	setSeats(t, pool, "%game 1", Seat{"ann", 200}, Seat{"bob", 150})
	setSeats(t, pool, "%game 2", Seat{"ann", 120}, Seat{"cy", 90})
	if _, err := pool.Exec(ctx, "UPDATE games SET share_code = 'CODEAAAA' WHERE name LIKE '%game 1'"); err != nil {
		t.Fatal(err)
	}
	x := cgeextract.Extract{Seats: []cgeextract.Seat{
		extractSeat(100, 19, 1, "ann", "CODEAAAA", 200), extractSeat(100, 19, 2, "bob", "CODEAAAA", 150),
		extractSeat(101, 19, 1, "ann", "CODEBBBB", 120), extractSeat(101, 19, 3, "cy", "CODEBBBB", 90),
		extractSeat(102, 19, 2, "bob", "CODECCCC", 70), extractSeat(102, 19, 3, "cy", "CODECCCC", 60),
	}}

	plan, err := PlanCGEExtract(ctx, pool, x, nil)
	if err != nil {
		t.Fatalf("PlanCGEExtract: %v", err)
	}
	var sessions []int
	for _, s := range plan.Matching.Matched {
		sessions = append(sessions, s)
	}
	slices.Sort(sessions)
	if !slices.Equal(sessions, []int{100, 101}) || !slices.Equal(plan.Matching.Added, []int{102}) || len(plan.Matching.OursOnly) != 0 {
		t.Errorf("matched %v, added %v, ours only %v; want 100 and 101 matched, 102 added", sessions, plan.Matching.Added, plan.Matching.OursOnly)
	}
}

// extractOf builds an extract's seats: CGE tournament t, stage 5 (number 19), group 70 "Bronze 1" unless t is
// another tournament; standing 0 is none.
func extractOf(t int, session int, code string, seats ...cgeextract.Seat) []cgeextract.Seat {
	finished := time.Date(2024, 2, 3, 21, 0, 0, 0, time.UTC)
	for i := range seats {
		seats[i].TournamentID, seats[i].SessionID, seats[i].ShareCode, seats[i].State = t, session, code, "finished"
		seats[i].StageID, seats[i].StageNumber, seats[i].GroupID, seats[i].GroupName = 5, 19, 70, "Bronze 1"
		seats[i].FinishedAt, seats[i].PlayersInGame, seats[i].StartingPosition = &finished, len(seats), i
		if t != 19 {
			seats[i].StageID, seats[i].StageNumber, seats[i].GroupID, seats[i].GroupName = 9, 1, 90, "Final"
		}
	}
	return seats
}

func cgeDone(player int, name string, standing int, culture float64) cgeextract.Seat {
	return cgeextract.Seat{PlayerID: player, PlayerName: name, FinishStatus: cgeextract.Finished, Standing: &standing, Score: &culture}
}

func cgeQuit(player int, name string, standing int, mark float64) cgeextract.Seat {
	return cgeextract.Seat{PlayerID: player, PlayerName: name, FinishStatus: cgeextract.Resigned, Standing: &standing, Score: &mark}
}

func cgeGone(player int, name string) cgeextract.Seat {
	return cgeextract.Seat{PlayerID: player, PlayerName: name, FinishStatus: cgeextract.TimedOut}
}

func TestApplyCGEExtractWritesWhatCGERecorded(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02"),
		"Intermezzo S24 - Bronze 1 game 2": day("2024-01-09"),
	})
	setSeats(t, pool, "%game 1", Seat{"ann", 200}, Seat{"bob", 150}, Seat{"cy", 0})
	setSeats(t, pool, "%game 2", Seat{"ann", 120}, Seat{"bob", 90})
	for _, q := range []string{
		"UPDATE games SET share_code = 'CODEAAAA' WHERE name LIKE '%game 1'",
		`INSERT INTO cge_tournaments (cge_tournament, name, status, series_id)
		 SELECT 19, 'Intermezzo Championship', 'running', series_id FROM series WHERE slug = 'intermezzo'`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	var seats []cgeextract.Seat
	seats = append(seats, extractOf(19, 100, "CODEAAAA", cgeDone(1, "ann", 1, 200), cgeDone(2, "bob", 2, 150), cgeQuit(3, "cy", 3, -9))...)
	seats = append(seats, extractOf(19, 101, "CODEBBBB", cgeDone(1, "ann", 1, 120), cgeDone(2, "bob", 2, 90), cgeGone(4, "dee"))...)
	seats = append(seats, extractOf(19, 102, "CODECCCC", cgeDone(2, "bob", 1, 80), cgeDone(3, "cy", 2, 70))...)
	seats = append(seats, extractOf(999, 200, "CODEDDDD", cgeDone(5, "eve", 1, 100), cgeDone(6, "fay", 2, 50))...)
	x := cgeextract.Extract{Seats: seats, Tournaments: map[int]cgeextract.Tournament{
		19:  {ID: 19, Name: "Intermezzo Championship", OrganiserID: 1},
		999: {ID: 999, Name: "Snail Cup", OrganiserID: 5, Moderators: []string{"fay"}},
	}}
	in := CGEInput{
		Extract: x,
		Special: map[int]cgeextract.Special{200: {Rules: "Tabletop", CardSet: "Random Mix"}},
	}
	snapshot := func() []string {
		return rowsOf(t, pool, `SELECT 'g' || g::text FROM games g UNION ALL SELECT 's' || s::text FROM seats s
			UNION ALL SELECT 'a' || a::text FROM player_accounts a UNION ALL SELECT 'x' || x::text FROM settings x
			UNION ALL SELECT 'r' || r::text FROM groups r ORDER BY 1`)
	}
	var first []string
	for load := range 2 {
		plan, err := PlanCGEExtract(ctx, pool, x, nil)
		if err != nil {
			t.Fatalf("PlanCGEExtract: %v", err)
		}
		if _, err := ApplyCGEExtract(ctx, pool, plan, in); err != nil {
			t.Fatalf("ApplyCGEExtract: %v", err)
		}
		if load == 0 {
			first = snapshot()
		} else if again := snapshot(); !slices.Equal(again, first) {
			t.Errorf("a second load of the same extract changed the data:\n%q\nwant\n%q", again, first)
		}
	}

	tests := []struct{ name, query, want string }{
		{"a matched game takes CGE's share code, session and finish date",
			`SELECT share_code || ' ' || cge_session || ' ' || date FROM games WHERE name LIKE '%game 2'`, "CODEBBBB 101 2024-02-03"},
		{"a resignation the file scored 0 is CGE's, placed by order",
			`SELECT outcome || ' ' || culture || ' ' || cge_score || ' ' || place FROM seats WHERE player_id = (SELECT player_id FROM players WHERE name = 'cy') AND cge_score IS NOT NULL
			 AND game_id = (SELECT game_id FROM games WHERE share_code = 'CODEAAAA')`, "resigned 0 -9 3"},
		{"a seat only CGE holds is added, timed out and last",
			`SELECT sp.name || ' ' || s.outcome || ' ' || s.place FROM seats s JOIN players sp ON sp.player_id = s.player_id JOIN games g USING (game_id)
			 WHERE g.share_code = 'CODEBBBB' AND s.seat = 3`, "dee timed_out 3"},
		{"an account belongs to the player who played it",
			`SELECT p.name FROM player_accounts a JOIN players p USING (player_id) WHERE a.cge_user = 1`, "ann"},
		{"a game only CGE has joins its group, named as its games are",
			`SELECT g.name || ' ' || gr.name FROM games g JOIN groups gr USING (group_id) WHERE g.share_code = 'CODECCCC'`,
			"Intermezzo S24 - Bronze 1 game 3 Bronze 1"},
		{"an unlinked CGE tournament is a hidden series of its own",
			`SELECT se.slug || ' ' || se.name || ' ' || se.visible FROM games g JOIN game_lines l USING (game_id)
			 JOIN series se ON se.series_id = l.series_id WHERE g.share_code = 'CODEDDDD'`, "cge_999 Snail Cup false"},
		{"a game's special rules", `SELECT rules || ' ' || card_set FROM games WHERE share_code = 'CODEDDDD'`, "Tabletop Random Mix"},
		{"an organiser is an account", `SELECT organiser_user::text FROM cge_tournaments WHERE cge_tournament = 999`, "5"},
		{"a moderator is an account", `SELECT cge_user::text FROM cge_tournament_moderators WHERE cge_tournament = 999`, "6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			if err := pool.QueryRow(ctx, tt.query).Scan(&got); err != nil {
				t.Fatalf("%v\n%s", err, tt.query)
			}
			if got != tt.want {
				t.Errorf("got = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyCGEExtractKeepsAnUnlinkedTournamentsStagesInOneTournament(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	inStage := func(stage int, seats []cgeextract.Seat) []cgeextract.Seat {
		for i := range seats {
			seats[i].StageID, seats[i].StageNumber, seats[i].GroupID, seats[i].GroupName = 10+stage, stage, 90+stage, "Group A"
		}
		return seats
	}
	tournaments := map[int]cgeextract.Tournament{999: {ID: 999, Name: "Snail Cup"}}
	var seats []cgeextract.Seat
	seats = append(seats, inStage(1, extractOf(999, 200, "CODEDDDD", cgeDone(5, "eve", 1, 100), cgeDone(6, "fay", 2, 50)))...)
	seats = append(seats, inStage(2, extractOf(999, 201, "CODEEEEE", cgeDone(5, "eve", 1, 90), cgeDone(6, "fay", 2, 60)))...)
	later := append(slices.Clone(seats), inStage(3, extractOf(999, 202, "CODEFFFF", cgeDone(5, "eve", 2, 80), cgeDone(6, "fay", 1, 85)))...)
	for _, s := range [][]cgeextract.Seat{seats, later} {
		x := cgeextract.Extract{Seats: s, Tournaments: tournaments}
		plan, err := PlanCGEExtract(ctx, pool, x, nil)
		if err != nil {
			t.Fatalf("PlanCGEExtract: %v", err)
		}
		if _, err := ApplyCGEExtract(ctx, pool, plan, CGEInput{Extract: x}); err != nil {
			t.Fatalf("ApplyCGEExtract: %v", err)
		}
	}

	var got string
	err := pool.QueryRow(ctx, `SELECT string_agg(t.label || ': ' || st.label, ', ' ORDER BY st.cge_stage_number)
		FROM series se JOIN tournaments t USING (series_id) JOIN stages st USING (tournament_id) WHERE se.slug = 'cge_999'`).Scan(&got)
	if err != nil {
		t.Fatal(err)
	}
	if want := "1: Stage 1, 1: Stage 2, 1: Stage 3"; got != want {
		t.Errorf("tournament: stage = %q, want %q", got, want)
	}
}

func TestApplyCGEExtractKeepsTwoGroupsOfOneNameApart(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	var seats []cgeextract.Seat
	for i, group := range []int{91, 92} {
		s := extractOf(999, 200+i, fmt.Sprintf("CODE%04d", i), cgeDone(5+2*i, "eve", 1, 100), cgeDone(6+2*i, "fay", 2, 50))
		for j := range s {
			s[j].StageID, s[j].StageNumber, s[j].GroupID, s[j].GroupName = 11, 1, group, "Group A"
			s[j].PlayerName = fmt.Sprintf("%s%d", s[j].PlayerName, i)
		}
		seats = append(seats, s...)
	}
	x := cgeextract.Extract{Seats: seats, Tournaments: map[int]cgeextract.Tournament{999: {ID: 999, Name: "Snail Cup"}}}
	plan, err := PlanCGEExtract(ctx, pool, x, nil)
	if err != nil {
		t.Fatalf("PlanCGEExtract: %v", err)
	}
	if _, err := ApplyCGEExtract(ctx, pool, plan, CGEInput{Extract: x}); err != nil {
		t.Fatalf("ApplyCGEExtract: %v", err)
	}

	var got string
	err = pool.QueryRow(ctx, `SELECT string_agg(gr.cge_group || ': ' || (SELECT count(*) FROM games g WHERE g.group_id = gr.group_id), ', '
		ORDER BY gr.cge_group) FROM groups gr WHERE gr.cge_tournament = 999 AND gr.name = 'Group A'`).Scan(&got)
	if err != nil {
		t.Fatal(err)
	}
	if want := "91: 1, 92: 1"; got != want {
		t.Errorf("CGE group: games = %q, want %q", got, want)
	}
}

func TestApplyCGEExtractMovesAMatchedGameIntoItsCGEGroup(t *testing.T) {
	tests := []struct{ name, ours string }{
		{"from another CGE group's group", "Intermezzo S24 - Bronze 1 game 2"},
		{"from the season's unnamed group", "Intermezzo S24 game 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			ctx := context.Background()
			placedOn(t, pool, map[string]time.Time{"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02"), tt.ours: day("2024-01-09")})
			setSeats(t, pool, "%game 1", Seat{"ann", 200}, Seat{"bob", 150})
			setSeats(t, pool, "%game 2", Seat{"cy", 120}, Seat{"dee", 90})
			setShareCode(t, pool, "Intermezzo S24 - Bronze 1 game 1", "CODEAAAA")
			setShareCode(t, pool, tt.ours, "CODEBBBB")
			seats := extractOf(19, 100, "CODEAAAA", cgeDone(1, "ann", 1, 200), cgeDone(2, "bob", 2, 150))
			other := extractOf(19, 101, "CODEBBBB", cgeDone(3, "cy", 1, 120), cgeDone(4, "dee", 2, 90))
			for i := range other {
				other[i].GroupID, other[i].GroupName = 71, "Bronze 2"
			}
			x := cgeextract.Extract{Seats: append(seats, other...),
				Tournaments: map[int]cgeextract.Tournament{19: {ID: 19, Name: "Intermezzo Championship"}}}
			snapshot := func() []string {
				return rowsOf(t, pool, `SELECT 'g' || g::text FROM games g UNION ALL SELECT 'r' || r::text FROM groups r ORDER BY 1`)
			}
			var first []string
			for load := range 2 {
				plan, err := PlanCGEExtract(ctx, pool, x, nil)
				if err != nil {
					t.Fatalf("PlanCGEExtract: %v", err)
				}
				if _, err := ApplyCGEExtract(ctx, pool, plan, CGEInput{Extract: x}); err != nil {
					t.Fatalf("ApplyCGEExtract: %v", err)
				}
				if load == 0 {
					first = snapshot()
				} else if again := snapshot(); !slices.Equal(again, first) {
					t.Errorf("a second load of the same extract changed the data:\n%q\nwant\n%q", again, first)
				}
			}

			got := rowsOf(t, pool, `SELECT g.share_code || ' ' || gr.name || ' ' || coalesce(gr.cge_group::text, '-') || ' ' || st.label
				FROM games g JOIN groups gr USING (group_id) JOIN stages st USING (stage_id) ORDER BY 1`)
			want := []string{"CODEAAAA Bronze 1 70 ", "CODEBBBB Bronze 2 71 "}
			if !slices.Equal(got, want) {
				t.Errorf("game group CGE group stage = %q, want %q: each game in its CGE group, in the season's stage", got, want)
			}
		})
	}
}

func TestApplyCGEExtractFollowsAGameCGEStartedOrFinished(t *testing.T) {
	tests := []struct {
		name, before, later, ours, want string
	}{
		{"a prepared game CGE finished is played", "prepared", "finished", "", "played"},
		{"a running game CGE finished is played", "running", "finished", "", "played"},
		{"a prepared game CGE started is running", "prepared", "running", "", "running"},
		{"a game we hold as never played stays so", "prepared", "finished", "never_played", "never_played"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			game := func(state string) []cgeextract.Seat {
				seats := inTournament(999, state != "finished", extractOf(999, 400, "CODEPREP", cgeDone(1, "ann", 1, 100), cgeDone(2, "bob", 2, 50)))
				for i := range seats {
					seats[i].State = state
				}
				return seats
			}
			if _, err := loadExtract(t, pool, game(tt.before)); err != nil {
				t.Fatalf("the first load: %v", err)
			}
			if tt.ours != "" {
				if _, err := pool.Exec(context.Background(), "UPDATE games SET state = $1 WHERE share_code = 'CODEPREP'", tt.ours); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := loadExtract(t, pool, game(tt.later)); err != nil {
				t.Fatalf("the second load: %v", err)
			}

			if got := rowsOf(t, pool, "SELECT state FROM games WHERE share_code = 'CODEPREP'"); !slices.Equal(got, []string{tt.want}) {
				t.Errorf("%s, then %s on CGE: state = %q, want %q", tt.before, tt.later, got, tt.want)
			}
		})
	}
}

func TestApplyCGEExtractRecordsALinkedGroupsCGETournamentAndStage(t *testing.T) {
	tests := []struct {
		name   string
		linked bool
	}{
		{"a group the load links by its name", false},
		{"a group an earlier load linked by its CGE group alone", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			ctx := context.Background()
			placedOn(t, pool, map[string]time.Time{"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02")})
			setSeats(t, pool, "%game 1", Seat{"ann", 200}, Seat{"bob", 150})
			setShareCode(t, pool, "Intermezzo S24 - Bronze 1 game 1", "CODEAAAA")
			if tt.linked {
				if _, err := pool.Exec(ctx, "UPDATE groups SET cge_group = 70 WHERE name = 'Bronze 1'"); err != nil {
					t.Fatal(err)
				}
			}
			x := cgeextract.Extract{Seats: extractOf(19, 100, "CODEAAAA", cgeDone(1, "ann", 1, 200), cgeDone(2, "bob", 2, 150)),
				Tournaments: map[int]cgeextract.Tournament{19: {ID: 19, Name: "Intermezzo Championship"}}}
			plan, err := PlanCGEExtract(ctx, pool, x, nil)
			if err != nil {
				t.Fatalf("PlanCGEExtract: %v", err)
			}
			if _, err := ApplyCGEExtract(ctx, pool, plan, CGEInput{Extract: x}); err != nil {
				t.Fatalf("ApplyCGEExtract: %v", err)
			}

			got := rowsOf(t, pool, `SELECT concat_ws(' ', name, cge_group, cge_tournament, cge_stage_number) FROM groups
				WHERE cge_group IS NOT NULL ORDER BY 1`)
			if want := []string{"Bronze 1 70 19 19"}; !slices.Equal(got, want) {
				t.Errorf("group, CGE group, tournament and stage = %q, want %q", got, want)
			}
		})
	}
}

func TestApplyCGEExtractKeepsAnAccountsPlayerWhoGoesByAnotherName(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	for _, q := range []string{
		"INSERT INTO players (name) VALUES ('ann'), ('bob')",
		"INSERT INTO player_accounts (cge_user, player_id, name, deleted) SELECT 1, player_id, 'deleted_user_1', true FROM players WHERE name = 'ann'",
		"INSERT INTO player_accounts (cge_user, player_id, name) SELECT 2, player_id, 'bob' FROM players WHERE name = 'bob'",
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	seats := extractOf(999, 200, "CODEDDDD", cgeDone(1, "deleted_user_1", 1, 100), cgeDone(2, "bob", 2, 50))
	x := cgeextract.Extract{Seats: seats, Tournaments: map[int]cgeextract.Tournament{999: {ID: 999, Name: "Snail Cup"}}}
	plan, err := PlanCGEExtract(ctx, pool, x, nil)
	if err != nil {
		t.Fatalf("PlanCGEExtract: %v", err)
	}

	if _, err := ApplyCGEExtract(ctx, pool, plan, CGEInput{Extract: x}); err != nil {
		t.Fatalf("ApplyCGEExtract: %v", err)
	}

	got := rowsOf(t, pool, `SELECT sp.name || ' ' || a.cge_user FROM seats s JOIN players sp ON sp.player_id = s.player_id JOIN player_accounts a ON a.cge_user = s.cge_user
		JOIN games g USING (game_id) WHERE g.share_code = 'CODEDDDD' ORDER BY a.cge_user`)
	if want := []string{"ann 1", "bob 2"}; !slices.Equal(got, want) {
		t.Errorf("seat player, account = %q, want %q: a deleted account keeps the player we know by their last name", got, want)
	}
}

func TestApplyCGEExtractRefusesAGroupOfItsNameWithNoCGEGroup(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	seats := extractOf(999, 200, "CODE0000", cgeDone(5, "eve", 1, 100), cgeDone(6, "fay", 2, 50))
	x := cgeextract.Extract{Seats: seats, Tournaments: map[int]cgeextract.Tournament{999: {ID: 999, Name: "Snail Cup"}}}
	load := func() error {
		plan, err := PlanCGEExtract(ctx, pool, x, nil)
		if err != nil {
			t.Fatalf("PlanCGEExtract: %v", err)
		}
		_, err = ApplyCGEExtract(ctx, pool, plan, CGEInput{Extract: x})
		return err
	}
	if err := load(); err != nil {
		t.Fatalf("first load: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO groups (stage_id, name, cge_tournament, cge_stage_number)
		SELECT stage_id, 'Semi', 999, 1 FROM groups WHERE cge_group = 90`); err != nil {
		t.Fatal(err)
	}
	semi := extractOf(999, 201, "CODE0001", cgeDone(5, "eve", 2, 80), cgeDone(6, "fay", 1, 90))
	for i := range semi {
		semi[i].GroupID, semi[i].GroupName = 91, "Semi"
	}
	x.Seats = append(x.Seats, semi...)

	err := load()
	if err == nil || !strings.Contains(err.Error(), "has no CGE group") {
		t.Errorf("second load: err = %v, want a refusal naming the group with no CGE group", err)
	}
}

func TestApplyCGEExtractMovesAnUnlinkedTournamentsStagesBackIntoOneTournament(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	var seats []cgeextract.Seat
	for stage := range 3 {
		s := extractOf(999, 200+stage, fmt.Sprintf("CODE%04d", stage), cgeDone(5, "eve", 1, 100), cgeDone(6, "fay", 2, 50))
		for i := range s {
			s[i].StageID, s[i].StageNumber, s[i].GroupID, s[i].GroupName = 11+stage, 1+stage, 91+stage, "Group A"
		}
		seats = append(seats, s...)
	}
	x := cgeextract.Extract{Seats: seats, Tournaments: map[int]cgeextract.Tournament{999: {ID: 999, Name: "Snail Cup"}}}
	load := func() CGEApplied {
		t.Helper()
		plan, err := PlanCGEExtract(ctx, pool, x, nil)
		if err != nil {
			t.Fatalf("PlanCGEExtract: %v", err)
		}
		got, err := ApplyCGEExtract(ctx, pool, plan, CGEInput{Extract: x})
		if err != nil {
			t.Fatalf("ApplyCGEExtract: %v", err)
		}
		return got
	}
	load()
	for _, q := range []string{
		`INSERT INTO tournaments (series_id, label, original_name)
		 SELECT series_id, n::text, n::text FROM series, generate_series(2, 3) n WHERE slug = 'cge_999'`,
		`UPDATE stages st SET tournament_id = t.tournament_id, label = '', original_name = ''
		 FROM tournaments t JOIN series se USING (series_id)
		 WHERE se.slug = 'cge_999' AND t.label = st.cge_stage_number::text AND st.cge_tournament = 999 AND st.cge_stage_number > 1`,
		`INSERT INTO settings (stage_id, timeout_penalty, source) SELECT stage_id, -2, 'cge' FROM stages WHERE cge_tournament = 999 AND cge_stage_number = 3`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	if got := load().TournamentsMerged; got != 2 {
		t.Errorf("TournamentsMerged = %d, want 2", got)
	}
	tests := []struct{ name, query, want string }{
		{"one tournament holds every stage", `SELECT string_agg(t.label || ': ' || st.label, ', ' ORDER BY st.cge_stage_number)
			FROM series se JOIN tournaments t USING (series_id) JOIN stages st USING (tournament_id) WHERE se.slug = 'cge_999'`,
			"1: Stage 1, 1: Stage 2, 1: Stage 3"},
		{"no tournament is left empty", `SELECT count(*)::text FROM tournaments t JOIN series se USING (series_id) WHERE se.slug = 'cge_999'`, "1"},
		{"a moved stage keeps its settings", `SELECT s.timeout_penalty::text FROM settings s JOIN stages st USING (stage_id)
			WHERE st.cge_tournament = 999 AND st.cge_stage_number = 3`, "-2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			if err := pool.QueryRow(ctx, tt.query).Scan(&got); err != nil {
				t.Fatalf("%v\n%s", err, tt.query)
			}
			if got != tt.want {
				t.Errorf("got = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyCGEExtractReadsOutcomesFromTheFilesScores(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"Intermezzo S4 - Bronze 1 game 1": day("2019-01-02")})
	setSeats(t, pool, "%game 1", Seat{"ann", 120}, Seat{"bob", 0}, Seat{"cy", -3}, Seat{"dee", -1})
	plan, err := PlanCGEExtract(ctx, pool, cgeextract.Extract{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyCGEExtract(ctx, pool, plan, CGEInput{}); err != nil {
		t.Fatalf("ApplyCGEExtract: %v", err)
	}
	got := rowsOf(t, pool, "SELECT p.name || ' ' || s.outcome || ' ' || s.culture || ' ' || s.place FROM seats s JOIN players p USING (player_id) ORDER BY s.seat")
	want := []string{"ann finished 120 1", "bob finished 0 2", "cy resigned 0 3", "dee timed_out 0 4"}
	if !slices.Equal(got, want) {
		t.Errorf("seats = %q, want %q", got, want)
	}
}

func TestApplyCGEExtractCorrectsAnAccountAnEarlierLoadLinkedWrongly(t *testing.T) {
	tests := []struct {
		name         string
		reason       string
		wantLinked   string
		wantRelinked int
		wantBetween  string
	}{
		{"a link the load made follows the seats", "", "bob", 1, "1 ann, 2 bob"},
		{"a link set by hand is kept", "the same person, by their choice", "ann", 0, "1 ann, 2 ann"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			ctx := context.Background()
			placedOn(t, pool, map[string]time.Time{
				"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02"),
				"Intermezzo S24 - Bronze 1 game 2": day("2024-01-09"),
			})
			setSeats(t, pool, "%game 1", Seat{"ann", 200}, Seat{"bob", 150})
			setSeats(t, pool, "%game 2", Seat{"ann", 120}, Seat{"bob", 90})
			setShareCode(t, pool, "Intermezzo S24 - Bronze 1 game 1", "CODEAAAA")
			var seats []cgeextract.Seat
			seats = append(seats, extractOf(19, 100, "CODEAAAA", cgeDone(1, "ann", 1, 200), cgeDone(2, "bob", 2, 150))...)
			seats = append(seats, extractOf(19, 101, "CODEBBBB", cgeDone(1, "ann", 1, 120), cgeDone(2, "bob", 2, 90))...)
			seats = append(seats, extractOf(19, 102, "CODECCCC", cgeDone(2, "bob", 1, 80), cgeDone(3, "cy", 2, 70))...)
			seats = append(seats, extractOf(19, 103, "CODEDDDD", cgeDone(1, "ann", 1, 60), cgeDone(2, "bob", 2, 50))...)
			in := CGEInput{Extract: cgeextract.Extract{Seats: seats}}
			load := func() CGEApplied {
				t.Helper()
				plan, err := PlanCGEExtract(ctx, pool, in.Extract, nil)
				if err != nil {
					t.Fatalf("PlanCGEExtract: %v", err)
				}
				got, err := ApplyCGEExtract(ctx, pool, plan, in)
				if err != nil {
					t.Fatalf("ApplyCGEExtract: %v", err)
				}
				return got
			}
			load()
			// As an earlier load left it: bob's account linked to ann, the two swapped in their shared game,
			// and bob's seats in the games only CGE has, and in one an earlier CGE scrape named, given to ann.
			for _, q := range []string{
				`UPDATE player_accounts SET player_id = (SELECT player_id FROM players WHERE name = 'ann'),
				        linked_reason = '` + tt.reason + `' WHERE cge_user = 2`,
				`UPDATE seats s SET cge_user = 3 - s.cge_user, place = 3 - s.place, culture = CASE s.place WHEN 1 THEN 150 ELSE 200 END
				 FROM games g WHERE g.game_id = s.game_id AND g.share_code = 'CODEAAAA'`,
				`UPDATE seats s SET player_id = (SELECT player_id FROM players WHERE name = 'ann')
				 FROM games g WHERE g.game_id = s.game_id AND g.share_code IN ('CODECCCC', 'CODEDDDD', 'CODEBBBB') AND s.cge_user = 2`,
				`UPDATE games SET source_file = 'cge' WHERE share_code = 'CODEBBBB'`,
			} {
				if _, err := pool.Exec(ctx, q); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
			}

			got := load()

			if got.Relinked != tt.wantRelinked {
				t.Errorf("relinked = %d, want %d", got.Relinked, tt.wantRelinked)
			}
			checks := []struct{ what, query, want string }{
				{"bob's account", `SELECT p.name FROM player_accounts a JOIN players p USING (player_id) WHERE a.cge_user = 2`, tt.wantLinked},
				{"the game only CGE has", `SELECT sp.name FROM seats s JOIN players sp ON sp.player_id = s.player_id JOIN games g USING (game_id)
					WHERE g.share_code = 'CODECCCC' AND s.cge_user = 2`, tt.wantLinked},
				{"the game only CGE has between the two", `SELECT string_agg(s.cge_user || ' ' || sp.name, ', ' ORDER BY s.cge_user)
					FROM seats s JOIN players sp ON sp.player_id = s.player_id JOIN games g USING (game_id) WHERE g.share_code = 'CODEDDDD'`, tt.wantBetween},
				{"the game an earlier CGE scrape named", `SELECT string_agg(s.cge_user || ' ' || sp.name, ', ' ORDER BY s.cge_user)
					FROM seats s JOIN players sp ON sp.player_id = s.player_id JOIN games g USING (game_id) WHERE g.share_code = 'CODEBBBB'`, tt.wantBetween},
				{"ann's seat in the shared game", `SELECT s.cge_user || ' ' || s.place || ' ' || s.culture FROM seats s
					JOIN players sp ON sp.player_id = s.player_id JOIN games g USING (game_id) WHERE g.share_code = 'CODEAAAA' AND sp.name = 'ann'`, "1 1 200"},
			}
			for _, c := range checks {
				var v string
				if err := pool.QueryRow(ctx, c.query).Scan(&v); err != nil {
					t.Fatalf("%s: %v", c.what, err)
				}
				if v != c.want {
					t.Errorf("%s = %q, want %q", c.what, v, c.want)
				}
			}
		})
	}
}

// cgeEmpty is a seat CGE holds no account for: timed out, nobody's.
func cgeEmpty() cgeextract.Seat { return cgeextract.Seat{FinishStatus: cgeextract.TimedOut} }

func TestApplyCGEExtractDropsTheSeatsOfPlayersCGERemoved(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02")})
	// The file scored cy's timed-out seat 0; CGE holds no player for it: cy was removed from the game.
	setSeats(t, pool, "%game 1", Seat{"ann", 200}, Seat{"bob", 150}, Seat{"cy", 0})
	if _, err := pool.Exec(ctx, `INSERT INTO cge_tournaments (cge_tournament, name, status, series_id)
		SELECT 19, 'Intermezzo Championship', 'running', series_id FROM series WHERE slug = 'intermezzo'`); err != nil {
		t.Fatal(err)
	}
	var seats []cgeextract.Seat
	seats = append(seats, extractOf(19, 100, "CODEAAAA", cgeDone(1, "ann", 1, 200), cgeDone(2, "bob", 2, 150), cgeEmpty())...)
	seats = append(seats, extractOf(999, 200, "CODEDDDD", cgeDone(5, "eve", 1, 100), cgeEmpty(), cgeDone(6, "fay", 2, 50))...)
	in := CGEInput{Extract: cgeextract.Extract{Seats: seats, Tournaments: map[int]cgeextract.Tournament{
		19:  {ID: 19, Name: "Intermezzo Championship", OrganiserID: 1},
		999: {ID: 999, Name: "Snail Cup", OrganiserID: 5},
	}}}
	load := func(confirmed map[string]int64) CGEApplied {
		t.Helper()
		plan, err := PlanCGEExtract(ctx, pool, in.Extract, confirmed)
		if err != nil {
			t.Fatalf("PlanCGEExtract: %v", err)
		}
		got, err := ApplyCGEExtract(ctx, pool, plan, in)
		if err != nil {
			t.Fatalf("ApplyCGEExtract: %v", err)
		}
		return got
	}
	snapshot := func() []string {
		return rowsOf(t, pool, `SELECT 'g' || g::text FROM games g UNION ALL SELECT 's' || s::text FROM seats s
			UNION ALL SELECT 'p' || p::text FROM players p UNION ALL SELECT 'a' || a::text FROM player_accounts a ORDER BY 1`)
	}

	players := rowsOf(t, pool, "SELECT name FROM players")
	got := load(nil)
	if got.HeldBack != 1 || got.Added != 1 {
		t.Errorf("held back %d, added %d; want the game with cy's seat held back and Snail Cup's added", got.HeldBack, got.Added)
	}
	var gameID int64
	if err := pool.QueryRow(ctx, "SELECT game_id FROM games WHERE name LIKE '%game 1'").Scan(&gameID); err != nil {
		t.Fatal(err)
	}
	got = load(map[string]int64{"CODEAAAA": gameID})
	if got.SeatsRemoved != 1 {
		t.Errorf("seats removed = %d, want 1: cy's", got.SeatsRemoved)
	}
	first := snapshot()
	if again := load(map[string]int64{"CODEAAAA": gameID}); !slices.Equal(snapshot(), first) {
		t.Errorf("a second load changed the data (%+v)", again)
	}

	tests := []struct{ name, query, want string }{
		{"the game only CGE has is its players' alone",
			`SELECT string_agg(sp.name || ' ' || s.outcome || ' ' || s.place || ' ' || coalesce(s.cge_user::text, '-'), ', ' ORDER BY s.seat)
			 FROM seats s JOIN players sp ON sp.player_id = s.player_id JOIN games g USING (game_id) WHERE g.share_code = 'CODEDDDD'`,
			"eve finished 1 5, fay finished 2 6"},
		{"our game is the others': cy's seat is gone",
			`SELECT string_agg(sp.name || ' ' || s.place, ', ' ORDER BY s.seat) || ' ' || g.share_code
			 FROM seats s JOIN players sp ON sp.player_id = s.player_id JOIN games g USING (game_id) WHERE g.game_id = ` + strconv.FormatInt(gameID, 10) + ` GROUP BY g.share_code`,
			"ann 1, bob 2 CODEAAAA"},
		{"no player is added for an empty seat", `SELECT string_agg(name, ',' ORDER BY name) FROM players
			WHERE name NOT IN ('` + strings.Join(players, "', '") + `')`, "eve,fay"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v string
			if err := pool.QueryRow(ctx, tt.query).Scan(&v); err != nil {
				t.Fatalf("%v\n%s", err, tt.query)
			}
			if v != tt.want {
				t.Errorf("got = %q, want %q", v, tt.want)
			}
		})
	}
}

func TestApplyCGEExtractFollowsCGEsTournamentRecord(t *testing.T) {
	may1, may2 := time.Date(2026, 5, 1, 18, 0, 0, 0, time.UTC), time.Date(2026, 5, 2, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name, heldName, heldOrganiser, heldCreated, heldFinished string
		cge                                                      cgeextract.Tournament
		want                                                     string
		wantUpdated                                              int
	}{
		{"an empty organiser name is filled", "Snail Cup", "", "2026-05-01 18:00", "",
			cgeextract.Tournament{Name: "Snail Cup", OrganiserName: "eve", Created: &may1}, "Snail Cup eve 2026-05-01 18:00 -", 1},
		{"CGE's organiser name replaces ours", "Snail Cup", "eve", "2026-05-01 18:00", "",
			cgeextract.Tournament{Name: "Snail Cup", OrganiserName: "evelyn", Created: &may1}, "Snail Cup evelyn 2026-05-01 18:00 -", 1},
		{"CGE's name and date replace ours", "Snail Cup", "eve", "2026-05-01 18:00", "",
			cgeextract.Tournament{Name: "Snail Cup 2026", OrganiserName: "eve", Created: &may2}, "Snail Cup 2026 eve 2026-05-02 09:00 -", 1},
		{"a date CGE does not give is kept", "Snail Cup", "eve", "2026-05-01 18:00", "2026-06-01 12:00",
			cgeextract.Tournament{Name: "Snail Cup", OrganiserName: "eve", Created: &may1}, "Snail Cup eve 2026-05-01 18:00 2026-06-01 12:00", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			ctx := context.Background()
			if _, err := pool.Exec(ctx, `INSERT INTO cge_tournaments (cge_tournament, name, status, organiser, created, finished)
				VALUES (999, $1, 'running', $2, ($3 || 'Z')::timestamptz, (nullif($4, '') || 'Z')::timestamptz)`, tt.heldName, tt.heldOrganiser, tt.heldCreated, tt.heldFinished); err != nil {
				t.Fatal(err)
			}
			cge := tt.cge
			cge.ID, cge.OrganiserID = 999, 5
			seats := extractOf(999, 200, "CODEDDDD", cgeDone(5, "eve", 1, 100), cgeDone(6, "fay", 2, 50))
			in := CGEInput{Extract: cgeextract.Extract{Seats: seats, Tournaments: map[int]cgeextract.Tournament{999: cge}}}
			plan, err := PlanCGEExtract(ctx, pool, in.Extract, nil)
			if err != nil {
				t.Fatalf("PlanCGEExtract: %v", err)
			}
			got, err := ApplyCGEExtract(ctx, pool, plan, in)
			if err != nil {
				t.Fatalf("ApplyCGEExtract: %v", err)
			}

			row := rowsOf(t, pool, `SELECT concat_ws(' ', name, organiser, to_char(created AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI'),
				coalesce(to_char(finished AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI'), '-')) FROM cge_tournaments
				WHERE cge_tournament = 999`)
			if !slices.Equal(row, []string{tt.want}) || got.TournamentsUpdated != tt.wantUpdated {
				t.Errorf("tournament = %q, %d updated; want %q, %d", row, got.TournamentsUpdated, tt.want, tt.wantUpdated)
			}
		})
	}
}

func TestApplyCGEExtractKeepsAndCountsWhatCGENoLongerLists(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	load := func(moderators []string, special map[int]cgeextract.Special) CGEApplied {
		t.Helper()
		x := cgeextract.Extract{Seats: extractOf(999, 200, "CODEDDDD", cgeDone(5, "eve", 1, 100), cgeDone(6, "fay", 2, 50)),
			Tournaments: map[int]cgeextract.Tournament{999: {ID: 999, Name: "Snail Cup", OrganiserID: 5, Moderators: moderators}}}
		plan, err := PlanCGEExtract(ctx, pool, x, nil)
		if err != nil {
			t.Fatalf("PlanCGEExtract: %v", err)
		}
		got, err := ApplyCGEExtract(ctx, pool, plan, CGEInput{Extract: x, Special: special})
		if err != nil {
			t.Fatalf("ApplyCGEExtract: %v", err)
		}
		return got
	}
	load([]string{"fay"}, map[int]cgeextract.Special{200: {Rules: "Tabletop", CardSet: "Random Mix"}})
	if got := load([]string{"fay"}, nil); len(got.RulesKept) != 0 {
		t.Errorf("with no special rules file, games' rules kept = %v, want none counted", got.RulesKept)
	}
	got := load(nil, map[int]cgeextract.Special{})

	held := rowsOf(t, pool, `SELECT 'moderator ' || cge_user FROM cge_tournament_moderators WHERE cge_tournament = 999
		UNION ALL SELECT 'rules ' || rules || ' ' || card_set FROM games WHERE share_code = 'CODEDDDD' ORDER BY 1`)
	if want := []string{"moderator 6", "rules Tabletop Random Mix"}; !slices.Equal(held, want) {
		t.Errorf("held after CGE stopped listing them = %q, want %q", held, want)
	}
	if want := map[int]int{999: 1}; !maps.Equal(got.ModeratorsKept, want) || !maps.Equal(got.RulesKept, want) {
		t.Errorf("moderators kept %v, games' rules kept %v; want %v each", got.ModeratorsKept, got.RulesKept, want)
	}
}

func TestApplyCGEExtractRemovesTheCopyAnEarlierLoadAddedOfAGameConfirmedByHand(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02")})
	setSeats(t, pool, "%game 1", Seat{"ann", 200}, Seat{"bob", 150}, Seat{"cy", 0})
	// As an earlier load left it: CGE's game, lacking cy's seat, added as a copy in a group of its own, and rated.
	var gameID, copyID, groupID, runID int64
	for _, q := range []struct {
		query string
		into  *int64
	}{
		{`INSERT INTO cge_tournaments (cge_tournament, name, status, series_id)
		  SELECT 19, 'Intermezzo Championship', 'running', series_id FROM series WHERE slug = 'intermezzo' RETURNING cge_tournament`, nil},
		{"SELECT game_id FROM games WHERE name LIKE '%game 1'", &gameID},
		{`INSERT INTO player_accounts (cge_user, player_id, name) SELECT 1, player_id, name FROM players WHERE name = 'ann' RETURNING cge_user`, nil},
		{`INSERT INTO player_accounts (cge_user, player_id, name) SELECT 2, player_id, name FROM players WHERE name = 'bob' RETURNING cge_user`, nil},
		{`INSERT INTO groups (stage_id, name, cge_tournament, cge_stage_number, cge_group)
		  SELECT stage_id, 'Bronze 1', 19, 19, 70 FROM groups WHERE group_id = (SELECT group_id FROM games WHERE name LIKE '%game 1') RETURNING group_id`, &groupID},
		{`INSERT INTO games (batch_id, source_file, source_line, name, date, state, group_id, share_code, cge_session)
		  SELECT batch_id, 'tournaments_export.csv', 1, 'Intermezzo S24 - Bronze 1 game 2', '2024-02-03', 'played', $1, 'CODEAAAA', 100
		  FROM import_batches ORDER BY batch_id LIMIT 1 RETURNING game_id`, &copyID},
		{`INSERT INTO seats (game_id, seat, player_id, cge_user, outcome, culture, place)
		  SELECT $1, a.cge_user, a.player_id, a.cge_user, 'finished', 200 - 50 * (a.cge_user - 1), a.cge_user FROM player_accounts a RETURNING game_id`, nil},
		{"INSERT INTO group_members (group_id, player_id) SELECT $1, player_id FROM player_accounts RETURNING group_id", nil},
		{"INSERT INTO rating_runs (engine_version, input_fingerprint) VALUES ('t', 't') RETURNING run_id", &runID},
		{`INSERT INTO pairwise_matches (run_id, match_id, date, tournament, player_count, player_a, player_b, score_a, score_b, outcome_a, weight, glicko_eligible)
		  SELECT $1, $2, '2024-02-03', 'Intermezzo S24', 2, (SELECT player_id FROM players WHERE name = 'ann'),
		         (SELECT player_id FROM players WHERE name = 'bob'), 200, 150, 1, 1, true RETURNING run_id`, nil},
		{"INSERT INTO tournament_games (run_id, match_id, series, name_season) VALUES ($1, $2, 'intermezzo', 24) RETURNING run_id", nil},
	} {
		var into int64
		if q.into == nil {
			q.into = &into
		}
		args := []any{}
		switch {
		case strings.Contains(q.query, "$2"):
			args = []any{runID, copyID}
		case strings.Contains(q.query, "group_members") || strings.Contains(q.query, "INTO seats"):
			args = []any{groupID}
			if strings.Contains(q.query, "INTO seats") {
				args = []any{copyID}
			}
		case strings.Contains(q.query, "$1"):
			args = []any{groupID}
		}
		if err := pool.QueryRow(ctx, q.query, args...).Scan(q.into); err != nil {
			t.Fatalf("%s: %v", q.query, err)
		}
	}
	seats := extractOf(19, 100, "CODEAAAA", cgeDone(1, "ann", 1, 200), cgeDone(2, "bob", 2, 150), cgeEmpty())
	in := CGEInput{Extract: cgeextract.Extract{Seats: seats, Tournaments: map[int]cgeextract.Tournament{19: {ID: 19, Name: "Intermezzo Championship", OrganiserID: 1}}}}
	plan, err := PlanCGEExtract(ctx, pool, in.Extract, map[string]int64{"CODEAAAA": gameID})
	if err != nil {
		t.Fatalf("PlanCGEExtract: %v", err)
	}

	got, err := ApplyCGEExtract(ctx, pool, plan, in)
	if err != nil {
		t.Fatalf("ApplyCGEExtract: %v", err)
	}

	if got.CopiesRemoved != 1 {
		t.Errorf("copies removed = %d, want 1", got.CopiesRemoved)
	}
	tests := []struct {
		name, query string
		arg         int64
		want        string
	}{
		{"the copy is gone with its seats", "SELECT count(*)::text FROM games WHERE game_id = $1", copyID, "0"},
		{"and its group", "SELECT count(*)::text FROM groups WHERE group_id = $1", groupID, "0"},
		{"and its rows in the run's tables",
			"SELECT (SELECT count(*) FROM pairwise_matches WHERE match_id = $1)::text || ' ' || (SELECT count(*) FROM tournament_games WHERE match_id = $1)", copyID, "0 0"},
		{"the run stays", "SELECT count(*)::text FROM rating_runs WHERE run_id = $1", runID, "1"},
		{"our game takes CGE's code and is ann's and bob's: cy, removed from it, is gone",
			`SELECT g.share_code || ' ' || string_agg(sp.name, ',' ORDER BY s.seat) FROM games g JOIN seats s USING (game_id) JOIN players sp ON sp.player_id = s.player_id
			 WHERE g.game_id = $1 GROUP BY g.share_code`, gameID, "CODEAAAA ann,bob"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v string
			if err := pool.QueryRow(ctx, tt.query, tt.arg).Scan(&v); err != nil {
				t.Fatalf("%v\n%s", err, tt.query)
			}
			if v != tt.want {
				t.Errorf("got = %q, want %q", v, tt.want)
			}
		})
	}
}

func TestPlanCGEExtractLeavesOutTheGamesOfASeasonRunOnThisSite(t *testing.T) {
	tests := []struct {
		name        string
		runHere     bool
		wantStages  string
		wantGames   int
		wantLeftOut int
		wantPenalty string
		wantNew     string
	}{
		{"a season CGE runs takes its stages, games and settings, a stage the load makes included", false, "Round 1, Stage 1", 1, 0, "-1", "-2"},
		{"a season run on this site takes CGE's page only", true, "Round 1", 0, 1, "none", "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			ctx := context.Background()
			for _, q := range []string{
				`INSERT INTO series (slug, name) VALUES ('lantern_open', 'Lantern Open')`,
				fmt.Sprintf(`INSERT INTO tournaments (series_id, label, run_here) SELECT series_id, '2031', %t FROM series WHERE slug = 'lantern_open'`, tt.runHere),
				`INSERT INTO stages (tournament_id, label, cge_tournament, cge_stage_number)
				 SELECT tournament_id, 'Round 1', 7001, 2 FROM tournaments WHERE label = '2031'`,
				`INSERT INTO cge_tournaments (cge_tournament, name, status, series_id, tournament_id)
				 SELECT 7001, 'Lantern Open', 'Stage 1 of 11', series_id, tournament_id FROM tournaments WHERE label = '2031'`,
			} {
				if _, err := pool.Exec(ctx, q); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
			}
			x := cgeextract.Extract{
				Seats:       extractOf(7001, 300, "CODEGGGG", cgeDone(5, "eve", 1, 100), cgeDone(6, "fay", 2, 50)),
				Tournaments: map[int]cgeextract.Tournament{7001: {ID: 7001, Name: "Lantern Open", OrganiserID: 5}},
			}
			plan, err := PlanCGEExtract(ctx, pool, x, nil)
			if err != nil {
				t.Fatalf("PlanCGEExtract: %v", err)
			}
			if plan.RunHere != tt.wantLeftOut {
				t.Errorf("games left out = %d, want %d", plan.RunHere, tt.wantLeftOut)
			}
			in := CGEInput{Extract: x, Stages: []cgeextract.StageSettings{
				{TournamentID: 7001, StageID: 9, StageNumber: 1, TimeoutPenalty: -2},
				{TournamentID: 7001, StageID: 10, StageNumber: 2, TimeoutPenalty: -1}}}
			if _, err := ApplyCGEExtract(ctx, pool, plan, in); err != nil {
				t.Fatalf("ApplyCGEExtract: %v", err)
			}
			var stages, page string
			penalty := func(stage string) string {
				t.Helper()
				var got string
				if err := pool.QueryRow(ctx, `SELECT coalesce((SELECT s.timeout_penalty::text FROM settings s
					JOIN stages st USING (stage_id) WHERE st.label = $1), 'none')`, stage).Scan(&got); err != nil {
					t.Fatal(err)
				}
				return got
			}
			if got := penalty("Round 1"); got != tt.wantPenalty {
				t.Errorf("Round 1's time-out penalty from CGE = %s, want %s", got, tt.wantPenalty)
			}
			if got := penalty("Stage 1"); got != tt.wantNew {
				t.Errorf("the time-out penalty from CGE of the stage the load made = %s, want %s", got, tt.wantNew)
			}
			var games int
			if err := pool.QueryRow(ctx, `SELECT string_agg(st.label, ', ' ORDER BY st.label) FROM stages st
				JOIN tournaments t USING (tournament_id) WHERE t.label = '2031'`).Scan(&stages); err != nil {
				t.Fatal(err)
			}
			if stages != tt.wantStages {
				t.Errorf("stages = %q, want %q", stages, tt.wantStages)
			}
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM games WHERE share_code = 'CODEGGGG'").Scan(&games); err != nil {
				t.Fatal(err)
			}
			if games != tt.wantGames {
				t.Errorf("games added = %d, want %d", games, tt.wantGames)
			}
			if err := pool.QueryRow(ctx, `SELECT c.name || ', ' || c.status || ', ' || t.label FROM cge_tournaments c
				JOIN tournaments t USING (tournament_id) WHERE c.cge_tournament = 7001`).Scan(&page); err != nil {
				t.Fatal(err)
			}
			if page != "Lantern Open, Stage 1 of 11, 2031" {
				t.Errorf("CGE's page = %q, want it kept and linked to 2031 either way", page)
			}
		})
	}
}

// loadExtract plans and applies an extract of Snail Cup (CGE tournament 999) holding seats.
func loadExtract(t *testing.T, pool *pgxpool.Pool, seats []cgeextract.Seat) (CGEApplied, error) {
	t.Helper()
	ctx := context.Background()
	x := cgeextract.Extract{Seats: seats, Tournaments: map[int]cgeextract.Tournament{999: {ID: 999, Name: "Snail Cup", OrganiserID: 2}}}
	plan, err := PlanCGEExtract(ctx, pool, x, nil)
	if err != nil {
		t.Fatalf("PlanCGEExtract: %v", err)
	}
	return ApplyCGEExtract(ctx, pool, plan, CGEInput{Extract: x})
}

// finishedOn moves seats' game to finish at the time given.
func finishedOn(at time.Time, seats []cgeextract.Seat) []cgeextract.Seat {
	for i := range seats {
		seats[i].FinishedAt = &at
	}
	return seats
}

// namesSnapshot is every player, name, account, game and seat, to compare two moments by.
const namesSnapshot = `SELECT 'p' || p::text FROM players p UNION ALL SELECT 'n' || n::text FROM player_names n
	UNION ALL SELECT 'a' || a::text FROM player_accounts a UNION ALL SELECT 'g' || g::text FROM games g
	UNION ALL SELECT 's' || s::text FROM seats s ORDER BY 1`

func TestApplyCGEExtractNamesAPlayerAfterTheirAccount(t *testing.T) {
	later := time.Date(2024, 5, 6, 23, 30, 0, 0, time.UTC)
	tests := []struct {
		name          string
		first, second []cgeextract.Seat
		wantPlayer    string
		wantNames     []string
		wantRenamed   int
	}{
		{"an account renamed renames its player, who keeps the old name",
			extractOf(999, 200, "CODEDDDD", cgeDone(1, "ann", 1, 100), cgeDone(2, "bob", 2, 50)),
			finishedOn(later, extractOf(999, 201, "CODEEEEE", cgeDone(1, "annie", 1, 100), cgeDone(2, "bob", 2, 50))),
			"annie", []string{"ann 2024-02-03 2024-02-03 cge_extract", "annie 2024-05-06 2024-05-06 cge_extract"}, 1},
		{"a deleted account's name renames nobody",
			extractOf(999, 200, "CODEDDDD", cgeDone(1, "ann", 1, 100), cgeDone(2, "bob", 2, 50)),
			finishedOn(later, extractOf(999, 201, "CODEEEEE", cgeDone(1, "deleted_user_1", 1, 100), cgeDone(2, "bob", 2, 50))),
			"ann", []string{"ann 2024-02-03 2024-02-03 cge_extract", "deleted_user_1 2024-05-06 2024-05-06 cge_extract"}, 0},
		{"a player with two live accounts of different names keeps their name",
			extractOf(999, 200, "CODEDDDD", cgeDone(1, "ann", 1, 100), cgeDone(3, "ann", 2, 70), cgeDone(2, "bob", 3, 50)),
			finishedOn(later, extractOf(999, 201, "CODEEEEE", cgeDone(1, "annie", 1, 100), cgeDone(3, "ann", 2, 70), cgeDone(2, "bob", 3, 50))),
			"ann", []string{"ann 2024-02-03 2024-05-06 cge_extract", "annie 2024-05-06 2024-05-06 cge_extract"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			for _, q := range []string{
				"INSERT INTO players (name) VALUES ('ann'), ('bob')",
				"INSERT INTO player_accounts (cge_user, player_id, name) SELECT 1, player_id, 'ann' FROM players WHERE name = 'ann'",
				"INSERT INTO player_accounts (cge_user, player_id, name) SELECT 2, player_id, 'bob' FROM players WHERE name = 'bob'",
			} {
				if _, err := pool.Exec(context.Background(), q); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
			}
			if _, err := loadExtract(t, pool, tt.first); err != nil {
				t.Fatalf("the first load: %v", err)
			}
			got, err := loadExtract(t, pool, tt.second)
			if err != nil {
				t.Fatalf("the second load: %v", err)
			}

			if got.PlayersRenamed != tt.wantRenamed {
				t.Errorf("players renamed = %d, want %d", got.PlayersRenamed, tt.wantRenamed)
			}
			player := rowsOf(t, pool, "SELECT p.name FROM player_accounts a JOIN players p USING (player_id) WHERE a.cge_user = 1")
			if !slices.Equal(player, []string{tt.wantPlayer}) {
				t.Errorf("account 1's player = %q, want %q", player, tt.wantPlayer)
			}
			names := rowsOf(t, pool, `SELECT n.name || ' ' || n.first_seen || ' ' || n.last_seen || ' ' || n.source FROM player_names n
				JOIN player_accounts a USING (player_id) WHERE a.cge_user = 1 ORDER BY n.name COLLATE "C"`)
			if !slices.Equal(names, tt.wantNames) {
				t.Errorf("account 1's player's names = %q, want %q", names, tt.wantNames)
			}

			before := rowsOf(t, pool, namesSnapshot)
			again, err := loadExtract(t, pool, tt.second)
			if err != nil {
				t.Fatalf("the second load again: %v", err)
			}
			if after := rowsOf(t, pool, namesSnapshot); !slices.Equal(after, before) || again.PlayersRenamed != 0 {
				t.Errorf("loading the same extract again renamed %d and changed the data:\n%q\nwant\n%q", again.PlayersRenamed, after, before)
			}
		})
	}
}

func TestApplyCGEExtractRefusesANameAnotherPlayerHas(t *testing.T) {
	pool := migrated(t)
	for _, q := range []string{
		"INSERT INTO players (name) VALUES ('ann'), ('bob'), ('cy')",
		"INSERT INTO player_accounts (cge_user, player_id, name) SELECT 1, player_id, 'ann' FROM players WHERE name = 'ann'",
		"INSERT INTO player_accounts (cge_user, player_id, name) SELECT 2, player_id, 'bob' FROM players WHERE name = 'bob'",
	} {
		if _, err := pool.Exec(context.Background(), q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	before := rowsOf(t, pool, namesSnapshot)

	_, err := loadExtract(t, pool, extractOf(999, 200, "CODEDDDD", cgeDone(1, "Cy", 1, 100), cgeDone(2, "bob", 2, 50)))

	if err == nil || !strings.Contains(err.Error(), "1 player") || !strings.Contains(err.Error(), "CGE accounts 1") || strings.Contains(err.Error(), "Cy") {
		t.Errorf("loading an account renamed to cy's name: err = %v, want one counting 1 player and giving account 1, naming nobody", err)
	}
	if after := rowsOf(t, pool, namesSnapshot); !slices.Equal(after, before) {
		t.Errorf("the refused load changed the data:\n%q\nwant\n%q", after, before)
	}
}

func TestApplyCGEExtractPutsCGEsSettingsOnEachStage(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO series (slug, name) VALUES ('lantern_open', 'Lantern Open')`,
		`INSERT INTO tournaments (series_id, label) SELECT series_id, '2031' FROM series WHERE slug = 'lantern_open'`,
		`INSERT INTO stages (tournament_id, label, cge_tournament, cge_stage_number)
		 SELECT tournament_id, l, 7001, n FROM tournaments, (VALUES ('Stage 1', 1), ('Stage 2', 2)) v(l, n) WHERE label = '2031'`,
		`INSERT INTO cge_tournaments (cge_tournament, name, status, series_id, tournament_id)
		 SELECT 7001, 'Lantern Open', 'finished', series_id, tournament_id FROM tournaments WHERE label = '2031'`,
		`INSERT INTO settings (series_id, timeout_penalty, points_2p, tiebreak_2p, source)
		 SELECT series_id, -3, '{2,0}', 'positions', 'admin' FROM series WHERE slug = 'lantern_open'`,
		`INSERT INTO settings (stage_id, timeout_penalty, ignore_ai_players, tiebreak_2p, source)
		 SELECT stage_id, -5, false, 'winnerScore', 'cge' FROM stages WHERE label = 'Stage 1'`,
		`INSERT INTO groups (stage_id, name, cge_tournament, cge_stage_number) SELECT stage_id, 'Final', 7002, 1 FROM stages WHERE label = 'Stage 1'`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	x := cgeextract.Extract{
		Seats:       extractOf(7001, 300, "CODEGGGG", cgeDone(5, "eve", 1, 100), cgeDone(6, "fay", 2, 50)),
		Tournaments: map[int]cgeextract.Tournament{7001: {ID: 7001, Name: "Lantern Open", OrganiserID: 5}},
	}
	least := 50.0
	first := cgeextract.StageSettings{TournamentID: 7001, StageID: 9, StageNumber: 1, TimeoutPenalty: -1, IgnoreAI: true,
		Points:    map[int][]float64{2: {1, 0}, 3: {}, 4: {6, 3, 1, 0}},
		EachPlays: map[int]int{2: 1, 3: 0, 4: 2}, Tiebreak2p: "duelPositionsComparison, winnerScore",
		Tiebreak3pPlus: "winnerScore", WinnerScoreMinimum: &least}
	final := first
	final.TournamentID, final.StageID, final.TimeoutPenalty, final.EachPlays = 7002, 11, -50, map[int]int{2: 2, 3: 0, 4: 2}
	stages := []cgeextract.StageSettings{first, final,
		{TournamentID: 7001, StageID: 10, StageNumber: 2, Points: map[int][]float64{2: {}, 3: {5, 2, 0}, 4: {}}, EachPlays: map[int]int{2: 0, 3: 3, 4: 0}}}
	writes, err := PlanCGEStages(ctx, pool, stages)
	if err != nil {
		t.Fatalf("PlanCGEStages: %v", err)
	}
	var disagree []string
	for _, w := range writes {
		for _, d := range w.Disagree {
			disagree = append(disagree, w.Stage+": "+d)
		}
	}
	if want := []string{"Lantern Open 2031, Stage 1: each_plays_2p", "Lantern Open 2031, Stage 1: timeout_penalty"}; !slices.Equal(disagree, want) {
		t.Errorf("settings CGE's stages disagree on = %q, want %q", disagree, want)
	}
	for i, want := range []int{2, 0} {
		plan, err := PlanCGEExtract(ctx, pool, x, nil)
		if err != nil {
			t.Fatalf("PlanCGEExtract: %v", err)
		}
		got, err := ApplyCGEExtract(ctx, pool, plan, CGEInput{Extract: x, Stages: stages})
		if err != nil {
			t.Fatalf("ApplyCGEExtract: %v", err)
		}
		if got.SettingsWritten != want {
			t.Errorf("load %d: stages whose settings changed = %d, want %d", i+1, got.SettingsWritten, want)
		}
	}

	resolved := `SELECT concat_ws(' ', ss.timeout_penalty, ss.points_2p, ss.points_3p, ss.points_4p, ss.ignore_ai_players,
		concat_ws('/', ss.each_plays_2p, ss.each_plays_3p, ss.each_plays_4p), coalesce(ss.winner_score_minimum::text, 'none'))
		|| ' [' || ss.tiebreak_2p || '] [' || ss.tiebreak_3p_plus || ']'
		FROM stage_settings ss JOIN stages st USING (stage_id) WHERE st.label = `
	tests := []struct{ name, query, want string }{
		{"a stage takes the settings its CGE stages agree on over an earlier load's and its series', and keeps the rest",
			resolved + "'Stage 1'", "-5 {1,0} {} {6,3,1,0} t 0/2 50 [duelPositionsComparison, winnerScore] [winnerScore]"},
		{"a stage with no settings of its own takes CGE's, none included", resolved + "'Stage 2'",
			"0 {} {5,2,0} {} f 0/3/0 none [] []"},
		{"admin's series settings stay on the series",
			`SELECT s.timeout_penalty || ' ' || s.points_2p::text || ' ' || s.source FROM settings s
			 JOIN series se USING (series_id) WHERE se.slug = 'lantern_open'`, "-3 {2,0} admin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			if err := pool.QueryRow(ctx, tt.query).Scan(&got); err != nil {
				t.Fatalf("%v\n%s", err, tt.query)
			}
			if got != tt.want {
				t.Errorf("got = %q, want %q", got, tt.want)
			}
		})
	}
}

// inTournament puts an extract game's seats in a stage and group of their own for CGE tournament t; unplayed, the
// game is prepared, with no outcomes.
func inTournament(t int, unplayed bool, seats []cgeextract.Seat) []cgeextract.Seat {
	for i := range seats {
		seats[i].StageID, seats[i].StageNumber, seats[i].GroupID, seats[i].GroupName = t, 1, t*10, fmt.Sprintf("Group %d", t)
		if unplayed {
			seats[i].State, seats[i].FinishedAt, seats[i].FinishStatus, seats[i].Standing, seats[i].Score = "prepared", nil, "", nil, nil
		}
	}
	return seats
}

func TestApplyCGEExtractRemovesTheGamesCGERemoved(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	tournaments := map[int]cgeextract.Tournament{
		999: {ID: 999, Name: "Snail Cup", OrganiserID: 1}, 7001: {ID: 7001, Name: "Lantern Open", OrganiserID: 1},
		8000: {ID: 8000, Name: "Ember Cup", OrganiserID: 1},
	}
	game := func(t, session int, code string, unplayed bool) []cgeextract.Seat {
		return inTournament(t, unplayed, extractOf(t, session, code, cgeDone(1, "ann", 1, 100), cgeDone(2, "bob", 2, 50)))
	}
	load := func(seats, removed []cgeextract.Seat, in map[int]cgeextract.Tournament, full bool) (CGEPlan, CGEApplied) {
		t.Helper()
		x := cgeextract.Extract{Seats: seats, Removed: removed, Tournaments: in, Full: full}
		plan, err := PlanCGEExtract(ctx, pool, x, nil)
		if err != nil {
			t.Fatalf("PlanCGEExtract: %v", err)
		}
		got, err := ApplyCGEExtract(ctx, pool, plan, CGEInput{Extract: x})
		if err != nil {
			t.Fatalf("ApplyCGEExtract: %v", err)
		}
		return plan, got
	}
	moved := func(session int) []cgeextract.Seat {
		return inTournament(999, false, extractOf(999, session, "CODEMOVE", cgeDone(1, "ann", 1, 90), cgeDone(2, "bob", 2, 40)))
	}
	var first []cgeextract.Seat
	for _, g := range [][]cgeextract.Seat{
		game(999, 400, "CODEPREP", true), game(999, 401, "CODEPLAY", false), game(999, 402, "CODESTAY", true),
		game(7001, 403, "CODEHERE", true), game(8000, 405, "CODEGONE", true), moved(406),
	} {
		first = append(first, g...)
	}
	load(first, nil, tournaments, true)
	for _, q := range []string{
		`UPDATE cge_tournaments SET tournament_id = (SELECT st.tournament_id FROM groups gr JOIN stages st USING (stage_id)
		 WHERE gr.cge_tournament = 7001) WHERE cge_tournament = 7001`,
		`UPDATE tournaments SET run_here = true WHERE tournament_id = (SELECT tournament_id FROM cge_tournaments WHERE cge_tournament = 7001)`,
	} {
		if tag, err := pool.Exec(ctx, q); err != nil || tag.RowsAffected() != 1 {
			t.Fatalf("%s: %v, %d rows; want 1", q, err, tag.RowsAffected())
		}
	}

	second := slices.Concat(game(999, 402, "CODESTAY", true), game(7001, 404, "CODENEXT", true), moved(407),
		game(999, 411, "CODENEWS", false))
	removed := slices.Concat(game(999, 400, "CODEPREP", true), game(999, 401, "CODEPLAY", false),
		game(7001, 403, "CODEHERE", true), moved(406))
	delete(tournaments, 8000)
	for i, want := range []struct{ removed, played int }{{2, 1}, {0, 0}} {
		plan, got := load(second, removed, tournaments, false)
		if len(plan.Removed) != want.removed || plan.RemovedPlayed != want.played || got.GamesRemoved != want.removed {
			t.Errorf("load %d: planned %d removed, %d of them played, and removed %d; want %d, %d and %d", i+1,
				len(plan.Removed), plan.RemovedPlayed, got.GamesRemoved, want.removed, want.played, want.removed)
		}
		if len(plan.Matching.HeldBack) != 0 {
			t.Errorf("load %d: held back %d games as possible copies; want none, as a game CGE removed is no copy",
				i+1, len(plan.Matching.HeldBack))
		}
	}
	got := rowsOf(t, pool, `SELECT concat_ws(' ', share_code, CASE WHEN share_code = 'CODEMOVE' THEN cge_session END)
		FROM games WHERE share_code LIKE 'CODE%' ORDER BY 1`)
	want := []string{"CODEGONE", "CODEHERE", "CODEMOVE 407", "CODENEWS", "CODESTAY"}
	if !slices.Equal(got, want) {
		t.Errorf("games = %q, want %q: the games the changes file removes gone, played or not; a season run here's, "+
			"one it does not name and one matched to a new session kept; a new game of the removed games' players "+
			"added, not held back as a copy", got, want)
	}
}

func TestPlanCGEExtractOnlyListsTheGamesAFullExtractLacks(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	tournaments := map[int]cgeextract.Tournament{999: {ID: 999, Name: "Snail Cup", OrganiserID: 1}}
	game := func(session int, code string) []cgeextract.Seat {
		return inTournament(999, true, extractOf(999, session, code, cgeDone(1, "ann", 1, 100), cgeDone(2, "bob", 2, 50)))
	}
	if _, err := loadExtract(t, pool, slices.Concat(game(400, "CODEGONE"), game(401, "CODESTAY"))); err != nil {
		t.Fatalf("the first load: %v", err)
	}
	tests := []struct {
		name        string
		full        bool
		wantMissing int
	}{
		{"a full extract lists the game it lacks", true, 1},
		{"a partial delivery lists nothing", false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := cgeextract.Extract{Seats: game(401, "CODESTAY"), Tournaments: tournaments, Full: tt.full}
			plan, err := PlanCGEExtract(ctx, pool, x, nil)
			if err != nil {
				t.Fatalf("PlanCGEExtract: %v", err)
			}
			if _, err := ApplyCGEExtract(ctx, pool, plan, CGEInput{Extract: x}); err != nil {
				t.Fatalf("ApplyCGEExtract: %v", err)
			}
			if len(plan.Missing) != tt.wantMissing || len(plan.Removed) != 0 {
				t.Errorf("missing %v, removed %v; want %d missing, none removed", plan.Missing, plan.Removed, tt.wantMissing)
			}
			if got := rowsOf(t, pool, "SELECT share_code FROM games WHERE share_code LIKE 'CODE%' ORDER BY 1"); len(got) != 2 {
				t.Errorf("games = %q, want both kept: a game missing from a delivery is never deleted", got)
			}
		})
	}
}

func TestApplyCGEExtractCompletesAPartialGameFromTheSeatsWeHold(t *testing.T) {
	full := func() []cgeextract.Seat {
		return extractOf(999, 500, "CODEPART", cgeDone(1, "ann", 1, 100), cgeDone(2, "bob", 2, 80), cgeDone(3, "cy", 3, 60))
	}
	only := func(seats []cgeextract.Seat, players ...int) []cgeextract.Seat {
		return slices.DeleteFunc(seats, func(s cgeextract.Seat) bool { return !slices.Contains(players, s.PlayerID) })
	}
	tests := []struct {
		name             string
		present, removed []cgeextract.Seat
		wantSeats        []string
		wantPartial      int
	}{
		{"one changed seat keeps the others and their places", only(full(), 3), nil,
			[]string{"ann 1", "bob 2", "cy 3"}, 0},
		{"a seat the delivery removes is dropped, not completed back", only(full(), 1, 2), only(full(), 3),
			[]string{"ann 1", "bob 2"}, 0},
		{"a partial game we do not hold is left out", only(extractOf(999, 600, "CODENONE", cgeDone(1, "ann", 1, 100),
			cgeDone(2, "bob", 2, 80)), 1), nil, []string{"ann 1", "bob 2", "cy 3"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			ctx := context.Background()
			if _, err := loadExtract(t, pool, full()); err != nil {
				t.Fatalf("the first load: %v", err)
			}
			x := cgeextract.Extract{Seats: tt.present, Removed: tt.removed,
				Tournaments: map[int]cgeextract.Tournament{999: {ID: 999, Name: "Snail Cup", OrganiserID: 2}}}
			plan, err := PlanCGEExtract(ctx, pool, x, nil)
			if err != nil {
				t.Fatalf("PlanCGEExtract: %v", err)
			}
			if _, err := ApplyCGEExtract(ctx, pool, plan, CGEInput{Extract: x}); err != nil {
				t.Fatalf("ApplyCGEExtract: %v", err)
			}

			got := rowsOf(t, pool, `SELECT p.name || ' ' || s.place FROM seats s JOIN players p USING (player_id)
				JOIN games g USING (game_id) WHERE g.share_code LIKE 'CODE%' ORDER BY 1`)
			if !slices.Equal(got, tt.wantSeats) || plan.Partial != tt.wantPartial {
				t.Errorf("seats %q, %d partial games left out; want %q, %d", got, plan.Partial, tt.wantSeats, tt.wantPartial)
			}
		})
	}
}

func TestApplyCGEExtractRecordsEachStagesCGENameAndTimes(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO series (slug, name) VALUES ('lantern_open', 'Lantern Open')`,
		`INSERT INTO tournaments (series_id, label) SELECT series_id, '2031' FROM series WHERE slug = 'lantern_open'`,
		`INSERT INTO stages (tournament_id, label, cge_tournament, cge_stage_number)
		 SELECT tournament_id, l, 7001, n FROM tournaments, (VALUES ('Stage 1', 1), ('Final', 2)) v(l, n) WHERE label = '2031'`,
		`INSERT INTO groups (stage_id, name, cge_tournament, cge_stage_number) SELECT stage_id, 'Final', 7002, 1 FROM stages WHERE label = 'Final'`,
		`INSERT INTO cge_tournaments (cge_tournament, name, status, series_id, tournament_id)
		 SELECT 7001, 'Lantern Open', 'finished', series_id, tournament_id FROM tournaments WHERE label = '2031'`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	at := func(day int) *time.Time {
		d := time.Date(2031, 5, day, 12, 0, 0, 0, time.UTC)
		return &d
	}
	load := func(stages map[int]cgeextract.Stage, names []cgeextract.StageSettings) {
		t.Helper()
		x := cgeextract.Extract{Seats: extractOf(7001, 300, "CODEGGGG", cgeDone(5, "eve", 1, 100), cgeDone(6, "fay", 2, 50)),
			Tournaments: map[int]cgeextract.Tournament{7001: {ID: 7001, Name: "Lantern Open", OrganiserID: 5}}, Stages: stages}
		plan, err := PlanCGEExtract(ctx, pool, x, nil)
		if err != nil {
			t.Fatalf("PlanCGEExtract: %v", err)
		}
		if _, err := ApplyCGEExtract(ctx, pool, plan, CGEInput{Extract: x, Stages: names}); err != nil {
			t.Fatalf("ApplyCGEExtract: %v", err)
		}
	}
	load(map[int]cgeextract.Stage{
		1:  {ID: 1, TournamentID: 7009, Number: 1, Started: at(1)},
		9:  {ID: 9, TournamentID: 7001, Number: 1, Started: at(1), Finished: at(10)},
		10: {ID: 10, TournamentID: 7001, Number: 2, Started: at(12), Finished: at(14)},
		11: {ID: 11, TournamentID: 7002, Number: 1, Started: at(11), Finished: at(20)},
	}, []cgeextract.StageSettings{
		{TournamentID: 7001, StageID: 9, StageNumber: 1, StageName: "Stage 1 of 2"},
		{TournamentID: 7001, StageID: 10, StageNumber: 2, StageName: "Stage 2 of 2"},
		{TournamentID: 7002, StageID: 11, StageNumber: 1, StageName: ""},
	})
	load(map[int]cgeextract.Stage{10: {ID: 10, TournamentID: 7001, Number: 2, Started: at(12), Finished: at(15)}}, nil)

	got := rowsOf(t, pool, `SELECT concat_ws(' ', label, coalesce(cge_name, '-'), to_char(cge_started_at AT TIME ZONE 'UTC', 'DD'),
		to_char(cge_finished_at AT TIME ZONE 'UTC', 'DD')) FROM stages WHERE cge_tournament = 7001 ORDER BY 1`)
	if want := []string{"Final - 11 20", "Stage 1 Stage 1 of 2 01 10"}; !slices.Equal(got, want) {
		t.Errorf("stage, CGE name, started and finished day = %q, want %q: a stage of ours spans its CGE stages, and a "+
			"later delivery of one of them widens it only", got, want)
	}
}

func TestApplyCGEExtractSummarisesWhatChanged(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	game := func(tournament, session int, code string, unplayed bool) []cgeextract.Seat {
		return inTournament(tournament, unplayed, extractOf(tournament, session, code, cgeDone(1, "ann", 1, 100), cgeDone(2, "bob", 2, 50)))
	}
	cups := func(ids ...int) map[int]cgeextract.Tournament {
		m := map[int]cgeextract.Tournament{}
		for _, id := range ids {
			m[id] = cgeextract.Tournament{ID: id, Name: fmt.Sprintf("Cup %d", id), OrganiserID: 1}
		}
		return m
	}
	first := cgeextract.Extract{Tournaments: cups(999, 1001), Full: true, Seats: slices.Concat(game(999, 400, "CODEDONE", true),
		game(999, 401, "CODESAME", false), game(999, 402, "CODEGONE", true), game(1001, 600, "CODELAST", true))}
	plan, err := PlanCGEExtract(ctx, pool, first, nil)
	if err != nil {
		t.Fatalf("PlanCGEExtract: %v", err)
	}
	if _, err := ApplyCGEExtract(ctx, pool, plan, CGEInput{Extract: first}); err != nil {
		t.Fatalf("the first load: %v", err)
	}
	second := cgeextract.Extract{Tournaments: cups(999, 1000),
		Seats:   slices.Concat(game(999, 400, "CODEDONE", false), game(999, 401, "CODESAME", false), game(1000, 500, "CODENEWT", false)),
		Removed: slices.Concat(game(999, 402, "CODEGONE", true), game(1001, 600, "CODELAST", true))}
	plan, err = PlanCGEExtract(ctx, pool, second, nil)
	if err != nil {
		t.Fatalf("PlanCGEExtract: %v", err)
	}
	before := rowsOf(t, pool, namesSnapshot)
	dry, err := DryRunCGEExtract(ctx, pool, plan, CGEInput{Extract: second})
	if err != nil {
		t.Fatalf("DryRunCGEExtract: %v", err)
	}
	if after := rowsOf(t, pool, namesSnapshot); !slices.Equal(after, before) {
		t.Errorf("the dry run changed the data:\n%q\nwant\n%q", after, before)
	}
	got, err := ApplyCGEExtract(ctx, pool, plan, CGEInput{Extract: second})
	if err != nil {
		t.Fatalf("ApplyCGEExtract: %v", err)
	}
	for _, run := range []struct {
		name string
		got  CGEApplied
	}{{"the dry run", dry}, {"the load", got}} {
		checks := []struct {
			what      string
			got, want any
		}{
			{"games changed", run.got.GamesChanged, 1},
			{"games unchanged", run.got.GamesUnchanged, 1},
			{"state changes", run.got.StateChanges, map[string]int{"prepared → played": 1}},
			{"tournaments added", run.got.TournamentsAdded, 1},
			{"games removed", run.got.GamesRemoved, 2},
		}
		for _, c := range checks {
			if !reflect.DeepEqual(c.got, c.want) {
				t.Errorf("%s: %s = %v, want %v", run.name, c.what, c.got, c.want)
			}
		}
	}
	if !slices.Equal(plan.TournamentsEmptied, []int{1001}) {
		t.Errorf("tournaments every game of which is removed = %v, want [1001]", plan.TournamentsEmptied)
	}
}

// dataLoadSeats are two games CGE finished, the later on 5 March 2024, and one it cancelled after them.
func dataLoadSeats() []cgeextract.Seat {
	played := func(session int, code string, at time.Time) []cgeextract.Seat {
		return finishedOn(at, inTournament(999, false, extractOf(999, session, code, cgeDone(1, "ann", 1, 100), cgeDone(2, "bob", 2, 50))))
	}
	cancelled := finishedOn(time.Date(2024, 3, 9, 12, 0, 0, 0, time.UTC),
		inTournament(999, true, extractOf(999, 402, "CODECANC", cgeDone(1, "ann", 1, 0), cgeDone(2, "bob", 2, 0))))
	for i := range cancelled {
		cancelled[i].State = "cancelled"
	}
	return slices.Concat(played(400, "CODEOLDR", time.Date(2024, 2, 3, 21, 0, 0, 0, time.UTC)),
		played(401, "CODENEWR", time.Date(2024, 3, 5, 23, 30, 0, 0, time.UTC)), cancelled)
}

func TestApplyCGEExtractRecordsADataLoadDatedByItsLastFinishedGame(t *testing.T) {
	pool := migrated(t)
	got, err := loadExtract(t, pool, dataLoadSeats())
	if err != nil {
		t.Fatalf("ApplyCGEExtract: %v", err)
	}

	want := []string{strconv.FormatInt(got.BatchID, 10) + " 2024-03-05"}
	if loads := rowsOf(t, pool, "SELECT batch_id || ' ' || last_game FROM data_loads"); !slices.Equal(loads, want) {
		t.Errorf("data loads = %q, want %q: the last finished game, not the cancelled one", loads, want)
	}
}

func TestApplyCGEExtractRecordsNoDataLoadWhenItChangesNoGame(t *testing.T) {
	pool := migrated(t)
	for load := range 2 {
		if _, err := loadExtract(t, pool, dataLoadSeats()); err != nil {
			t.Fatalf("load %d: %v", load+1, err)
		}
	}

	if loads := rowsOf(t, pool, "SELECT count(*) FROM data_loads"); !slices.Equal(loads, []string{"1"}) {
		t.Errorf("after loading the same extract twice, %q data loads, want 1", loads)
	}
}

func TestDryRunCGEExtractRecordsNoDataLoad(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	x := cgeextract.Extract{Seats: dataLoadSeats(), Tournaments: map[int]cgeextract.Tournament{999: {ID: 999, Name: "Snail Cup", OrganiserID: 2}}}
	plan, err := PlanCGEExtract(ctx, pool, x, nil)
	if err != nil {
		t.Fatalf("PlanCGEExtract: %v", err)
	}
	if _, err := DryRunCGEExtract(ctx, pool, plan, CGEInput{Extract: x}); err != nil {
		t.Fatalf("DryRunCGEExtract: %v", err)
	}

	if loads := rowsOf(t, pool, "SELECT count(*) FROM data_loads"); !slices.Equal(loads, []string{"0"}) {
		t.Errorf("after a dry run, %q data loads, want 0", loads)
	}
}
