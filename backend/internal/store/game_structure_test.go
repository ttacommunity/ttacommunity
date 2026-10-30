//go:build integration

package store

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// beforeGameStructure is a database at schema 3 holding a few games in the old flat shape:
// a coded 3-player game (finished, resigned, timed out), an uncoded one with a tie, and their catalogue.
func beforeGameStructure(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := newDB(t)
	ctx := context.Background()
	if err := migrateRatingsTo(ctx, pool, 3); err != nil {
		t.Fatalf("migrateRatingsTo(3): %v", err)
	}
	for _, q := range []string{
		`INSERT INTO import_batches (source) VALUES ('load')`,
		`INSERT INTO tournaments (slug, name, kind, points_3p) VALUES ('international', 'International', 'league', '{5,2,0}')`,
		`INSERT INTO tournament_seasons (tournament_id, label) SELECT tournament_id, '10' FROM tournaments`,
		`INSERT INTO tournament_stages (season_id, label, cge_tournament, cge_season) SELECT season_id, '', 21, 1 FROM tournament_seasons`,
		`INSERT INTO tournament_season_rules (series, season, timeout_penalty, scoring_3p, tiebreaks, source)
		 VALUES ('international', 10, -1, '5-2-0', 'duelPositionsComparison, winnerScore / winnerScore', 'CGE season 1')`,
		`INSERT INTO cge_tournaments (cge_tournament, name, status, tournament_id, season)
		 SELECT 21, 'International Championship', 'running', tournament_id, '10' FROM tournaments`,
		`INSERT INTO cge_tournaments (cge_tournament, name, status, tournament_id, season)
		 SELECT 22, 'International Championship 11', 'prepared', tournament_id, '11' FROM tournaments`,
		// The seeded stories name players this database lacks, which would stop the migration to players by id.
		`DELETE FROM tournament_stories`,
		`INSERT INTO players (name) VALUES ('ann'), ('bob'), ('cy'), ('deleted_user_1')`,
		`INSERT INTO matches (batch_id, source_file, source_line, tournament, date, player_count,
		     player1, score1, player2, score2, player3, score3, replay_code, stage_id, group_label, game_number,
		     cge_tournament, cge_season, cge_finished)
		 SELECT 1, 'cge', 1, 'International S10 - Gold 1 game 1', '2020-06-02', 3,
		     'ann', 200, 'bob', -2, 'cy', -5, 'ABCDEFGH', stage_id, 'Gold 1', 1, 21, 1, '2020-06-02' FROM tournament_stages`,
		`INSERT INTO match_seats (match_id, seat, player_name, outcome, cge_user, banned, cge_name) VALUES
		     (1, 1, 'ann', 'finished', 101, false, 'ann'),
		     (1, 2, 'bob', 'resigned', 102, true, 'bob'),
		     (1, 3, 'cy', 'timed_out', 103, false, 'deleted_user_1')`,
		`INSERT INTO matches (batch_id, source_file, source_line, tournament, date, player_count,
		     player1, score1, player2, score2, player3, score3, stage_id, group_label, game_number)
		 SELECT 1, 's10.csv', 2, 'International S10 - Gold 1 game 2', '2020-06-03', 3,
		     'cy', 150, 'ann', 150, 'bob', -1, stage_id, 'Gold 1', 2 FROM tournament_stages`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("setting up schema 3: %v\n%s", err, q)
		}
	}
	return pool
}

func TestGameStructureMigrationScoresEverySeat(t *testing.T) {
	pool := beforeGameStructure(t)
	if err := MigrateRatings(context.Background(), pool); err != nil {
		t.Fatalf("MigrateRatings: %v", err)
	}
	tests := []struct {
		name          string
		game, seat    int
		outcome       *string
		culture       float64
		recordedScore float64
		place         int
	}{
		{"finished keeps its culture", 1, 1, ptr("finished"), 200, 200, 1},
		{"resigned scores no culture", 1, 2, ptr("resigned"), 0, -2, 2},
		{"timed out scores no culture", 1, 3, ptr("timed_out"), 0, -5, 3},
		{"an uncoded tie shares first place", 2, 1, nil, 150, 150, 1},
		{"its other tied seat", 2, 2, nil, 150, 150, 1},
		{"an uncoded negative score is no culture, outcome unknown", 2, 3, nil, 0, -1, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var outcome *string
			var culture, recorded float64
			var place int
			err := pool.QueryRow(context.Background(),
				"SELECT outcome, culture, recorded_score, place FROM seats WHERE game_id = $1 AND seat = $2",
				tt.game, tt.seat).Scan(&outcome, &culture, &recorded, &place)
			if err != nil {
				t.Fatalf("reading game %d seat %d: %v", tt.game, tt.seat, err)
			}
			if (outcome == nil) != (tt.outcome == nil) || (outcome != nil && *outcome != *tt.outcome) {
				t.Errorf("outcome = %v, want %v", text(outcome), text(tt.outcome))
			}
			if culture != tt.culture || recorded != tt.recordedScore || place != tt.place {
				t.Errorf("culture, recorded score, place = %v, %v, %d; want %v, %v, %d",
					culture, recorded, place, tt.culture, tt.recordedScore, tt.place)
			}
		})
	}
}

func TestGameStructureMigrationKeepsTheCatalogue(t *testing.T) {
	pool := beforeGameStructure(t)
	ctx := context.Background()
	if err := MigrateRatings(ctx, pool); err != nil {
		t.Fatalf("MigrateRatings: %v", err)
	}
	tests := []struct {
		name, query, want string
	}{
		{"a game sits in its group, stage, tournament and series",
			`SELECT se.slug || '/' || t.label || '/' || g.name FROM games ga JOIN groups g USING (group_id)
			 JOIN stages st USING (stage_id) JOIN tournaments t USING (tournament_id) JOIN series se USING (series_id)
			 WHERE ga.game_id = 1`, "international/10/Gold 1"},
		{"games of one group share it", `SELECT count(DISTINCT group_id)::text FROM games`, "1"},
		{"the group's CGE stage comes from its games", `SELECT cge_tournament || '/' || cge_stage_number FROM groups`, "21/1"},
		{"group members are its players", `SELECT count(*)::text FROM group_members`, "3"},
		{"series points become series settings",
			`SELECT points_3p::text FROM settings WHERE series_id IS NOT NULL`, "{5,2,0}"},
		{"season rules become tournament settings",
			`SELECT timeout_penalty || ' ' || tiebreak_2p || ' / ' || tiebreak_3p_plus FROM settings WHERE tournament_id IS NOT NULL`,
			"-1 duelPositionsComparison, winnerScore / winnerScore"},
		{"an account keeps its ban", `SELECT banned::text FROM player_accounts WHERE cge_user = 102`, "true"},
		{"a deleted_user name marks the account deleted", `SELECT deleted::text FROM player_accounts WHERE cge_user = 103`, "true"},
		{"an account belongs to the player who played it",
			`SELECT p.name FROM player_accounts a JOIN players p USING (player_id) WHERE a.cge_user = 101`, "ann"},
		{"a CGE tournament links to its series and tournament",
			`SELECT se.slug || '/' || t.label FROM cge_tournaments c JOIN series se USING (series_id)
			 JOIN tournaments t ON t.tournament_id = c.tournament_id WHERE c.cge_tournament = 21`, "international/10"},
		{"a catalogued season with no games becomes its tournament",
			`SELECT t.label FROM cge_tournaments c JOIN tournaments t USING (tournament_id) WHERE c.cge_tournament = 22`, "11"},
		{"the share code is kept", `SELECT share_code FROM games WHERE game_id = 1`, "ABCDEFGH"},
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

func TestGameStructureMigrationDropsTheFlatShape(t *testing.T) {
	pool := beforeGameStructure(t)
	ctx := context.Background()
	if err := MigrateRatings(ctx, pool); err != nil {
		t.Fatalf("MigrateRatings: %v", err)
	}
	var n int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema = 'public'
		AND (table_name, column_name) IN (('games', 'player1'), ('games', 'score1'), ('games', 'group_label'),
		    ('series', 'points_3p')) OR (table_schema = 'public' AND table_name IN ('matches', 'match_seats', 'tournament_season_rules'))`).Scan(&n)
	if err != nil {
		t.Fatalf("listing columns: %v", err)
	}
	if n != 0 {
		t.Errorf("old columns or tables left = %d, want 0", n)
	}
}

func text(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
