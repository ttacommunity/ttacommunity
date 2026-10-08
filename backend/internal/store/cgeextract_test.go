//go:build integration

package store

import (
	"context"
	"slices"
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
