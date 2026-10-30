//go:build integration

package store

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/cgeextract"
)

func extractSeat(session, tournament, player int, name, code string, culture float64) cgeextract.Seat {
	return cgeextract.Seat{SessionID: session, TournamentID: tournament, PlayerID: player, PlayerName: name,
		ShareCode: code, State: "finished", FinishStatus: cgeextract.Finished, Score: &culture}
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
	penalty := -1.0
	in := CGEInput{
		Extract:  x,
		Special:  map[int]cgeextract.Special{200: {Rules: "Tabletop", CardSet: "Random Mix"}},
		Settings: []cgeextract.Settings{{TournamentID: 19, TimeoutPenalty: &penalty}},
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
			`SELECT outcome || ' ' || culture || ' ' || cge_score || ' ' || place FROM seats WHERE player_name = 'cy' AND cge_score IS NOT NULL
			 AND game_id = (SELECT game_id FROM games WHERE share_code = 'CODEAAAA')`, "resigned 0 -9 3"},
		{"a seat only CGE holds is added, timed out and last",
			`SELECT s.player_name || ' ' || s.outcome || ' ' || s.place FROM seats s JOIN games g USING (game_id)
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
		{"a league's settings sit on its series",
			`SELECT s.timeout_penalty || ' ' || s.source FROM settings s JOIN series se USING (series_id) WHERE se.slug = 'intermezzo'`,
			"-1 cge"},
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

	got := rowsOf(t, pool, `SELECT s.player_name || ' ' || a.cge_user FROM seats s JOIN player_accounts a ON a.cge_user = s.cge_user
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
	got := rowsOf(t, pool, "SELECT player_name || ' ' || outcome || ' ' || culture || ' ' || place FROM seats ORDER BY seat")
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
				`UPDATE seats s SET player_id = (SELECT player_id FROM players WHERE name = 'ann'), player_name = 'ann'
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
				{"the game only CGE has", `SELECT s.player_name FROM seats s JOIN games g USING (game_id)
					WHERE g.share_code = 'CODECCCC' AND s.cge_user = 2`, tt.wantLinked},
				{"the game only CGE has between the two", `SELECT string_agg(s.cge_user || ' ' || s.player_name, ', ' ORDER BY s.cge_user)
					FROM seats s JOIN games g USING (game_id) WHERE g.share_code = 'CODEDDDD'`, tt.wantBetween},
				{"the game an earlier CGE scrape named", `SELECT string_agg(s.cge_user || ' ' || s.player_name, ', ' ORDER BY s.cge_user)
					FROM seats s JOIN games g USING (game_id) WHERE g.share_code = 'CODEBBBB'`, tt.wantBetween},
				{"ann's seat in the shared game", `SELECT s.cge_user || ' ' || s.place || ' ' || s.culture FROM seats s
					JOIN games g USING (game_id) WHERE g.share_code = 'CODEAAAA' AND s.player_name = 'ann'`, "1 1 200"},
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
			`SELECT string_agg(s.player_name || ' ' || s.outcome || ' ' || s.place || ' ' || coalesce(s.cge_user::text, '-'), ', ' ORDER BY s.seat)
			 FROM seats s JOIN games g USING (game_id) WHERE g.share_code = 'CODEDDDD'`,
			"eve finished 1 5, fay finished 2 6"},
		{"our game is the others': cy's seat is gone",
			`SELECT string_agg(s.player_name || ' ' || s.place, ', ' ORDER BY s.seat) || ' ' || g.share_code
			 FROM seats s JOIN games g USING (game_id) WHERE g.game_id = ` + strconv.FormatInt(gameID, 10) + ` GROUP BY g.share_code`,
			"ann 1, bob 2 CODEAAAA"},
		{"no player holds seats for nobody", `SELECT count(*)::text FROM players WHERE name = 'Unknown player'`, "0"},
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

func TestApplyCGEExtractFillsOnlyAnEmptyOrganiserName(t *testing.T) {
	tests := []struct{ name, held, want string }{
		{"an empty name is filled", "", "eve"},
		{"a held name is kept", "Lantern_Keeper", "Lantern_Keeper"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			ctx := context.Background()
			placedOn(t, pool, map[string]time.Time{"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02")})
			setSeats(t, pool, "%game 1", Seat{"ann", 200}, Seat{"bob", 150})
			if _, err := pool.Exec(ctx, `INSERT INTO cge_tournaments (cge_tournament, name, status, organiser)
				VALUES (999, 'Snail Cup', 'running', $1)`, tt.held); err != nil {
				t.Fatal(err)
			}
			seats := extractOf(999, 200, "CODEDDDD", cgeDone(5, "eve", 1, 100), cgeDone(6, "fay", 2, 50))
			in := CGEInput{Extract: cgeextract.Extract{Seats: seats, Tournaments: map[int]cgeextract.Tournament{
				999: {ID: 999, Name: "Snail Cup", OrganiserID: 5, OrganiserName: "eve"},
			}}}
			plan, err := PlanCGEExtract(ctx, pool, in.Extract, nil)
			if err != nil {
				t.Fatalf("PlanCGEExtract: %v", err)
			}
			if _, err := ApplyCGEExtract(ctx, pool, plan, in); err != nil {
				t.Fatalf("ApplyCGEExtract: %v", err)
			}

			var got string
			if err := pool.QueryRow(ctx, "SELECT organiser FROM cge_tournaments WHERE cge_tournament = 999").Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("organiser held %q = %q, want %q", tt.held, got, tt.want)
			}
		})
	}
}

func TestApplyCGEExtractRefusesAnAccountNamedLikeThePlaceholder(t *testing.T) {
	pool := migrated(t)
	ctx := context.Background()
	placedOn(t, pool, map[string]time.Time{"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02")})
	setSeats(t, pool, "%game 1", Seat{"ann", 200}, Seat{"bob", 150})
	seats := extractOf(999, 200, "CODEDDDD", cgeDone(5, "Unknown player", 1, 100), cgeEmpty())
	in := CGEInput{Extract: cgeextract.Extract{Seats: seats, Tournaments: map[int]cgeextract.Tournament{999: {ID: 999, Name: "Snail Cup", OrganiserID: 5}}}}
	plan, err := PlanCGEExtract(ctx, pool, in.Extract, nil)
	if err != nil {
		t.Fatalf("PlanCGEExtract: %v", err)
	}

	if _, err := ApplyCGEExtract(ctx, pool, plan, in); err == nil || !strings.Contains(err.Error(), UnknownPlayer) {
		t.Errorf("ApplyCGEExtract with a CGE account named Unknown player: err = %v, want one naming it", err)
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
		{`INSERT INTO seats (game_id, seat, player_id, player_name, cge_user, outcome, culture, place)
		  SELECT $1, a.cge_user, a.player_id, a.name, a.cge_user, 'finished', 200 - 50 * (a.cge_user - 1), a.cge_user FROM player_accounts a RETURNING game_id`, nil},
		{"INSERT INTO group_members (group_id, player_id) SELECT $1, player_id FROM player_accounts RETURNING group_id", nil},
		{"INSERT INTO rating_runs (engine_version, input_fingerprint) VALUES ('t', 't') RETURNING run_id", &runID},
		{`INSERT INTO pairwise_matches (run_id, match_id, date, tournament, player_count, player_a, player_b, score_a, score_b, outcome_a, weight, glicko_eligible)
		  VALUES ($1, $2, '2024-02-03', 'Intermezzo S24', 2, 'ann', 'bob', 200, 150, 1, 1, true) RETURNING run_id`, nil},
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
			`SELECT g.share_code || ' ' || string_agg(s.player_name, ',' ORDER BY s.seat) FROM games g JOIN seats s USING (game_id)
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
	}{
		{"a season CGE runs takes its stages, games and settings", false, "Round 1, Stage 1", 1, 0, "-1"},
		{"a season run on this site takes CGE's page only", true, "Round 1", 0, 1, "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := migrated(t)
			ctx := context.Background()
			for _, q := range []string{
				`INSERT INTO series (slug, name) VALUES ('lantern_open', 'Lantern Open')`,
				fmt.Sprintf(`INSERT INTO tournaments (series_id, label, run_here) SELECT series_id, '2031', %t FROM series WHERE slug = 'lantern_open'`, tt.runHere),
				`INSERT INTO stages (tournament_id, label) SELECT tournament_id, 'Round 1' FROM tournaments WHERE label = '2031'`,
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
			penalty := -1.0
			in := CGEInput{Extract: x, Settings: []cgeextract.Settings{{TournamentID: 7001, TimeoutPenalty: &penalty}}}
			if _, err := ApplyCGEExtract(ctx, pool, plan, in); err != nil {
				t.Fatalf("ApplyCGEExtract: %v", err)
			}
			var stages, page, setting string
			if err := pool.QueryRow(ctx, `SELECT coalesce((SELECT s.timeout_penalty::text FROM settings s
				JOIN tournaments t USING (tournament_id) WHERE t.label = '2031'), 'none')`).Scan(&setting); err != nil {
				t.Fatal(err)
			}
			if setting != tt.wantPenalty {
				t.Errorf("time-out penalty from CGE = %s, want %s", setting, tt.wantPenalty)
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
