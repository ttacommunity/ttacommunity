// Applying CGE's extract: one transaction, step by step, writing CGE's facts on our games, its accounts and
// catalogue, and the refusals. The games only CGE has are in cgeextract_games.go, its settings in cgeextract_settings.go.
package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/cgeextract"
)

// CGEInput is what the extract load reads: the extract, its special rules by session, and the settings files'
// rows (tournament settings, then stage overrides).
type CGEInput struct {
	Extract  cgeextract.Extract
	Special  map[int]cgeextract.Special
	Settings []cgeextract.Settings
}

// CGEApplied counts what an extract load wrote.
type CGEApplied struct {
	BatchID                                 int64
	Matched, Added, HeldBack, Unplaced      int
	SeatsAdded, PlayersAdded, AccountsAdded int
	SeriesAdded, SettingsWritten, Refused   int
	// Relinked counts accounts an earlier load linked to one player that their paired seats show is another's.
	Relinked int
	// TournamentsMerged counts the tournaments an earlier load split off an unlinked CGE tournament's hidden series.
	TournamentsMerged int
	// CopiesRemoved counts the games an earlier load added that the pairs file now matches to a game of ours.
	CopiesRemoved int
	// SeatsRemoved counts our seats dropped because CGE shows their player was removed from the game.
	SeatsRemoved int
	// GamesMoved counts the matched games moved into their CGE group.
	GamesMoved int
}

// UnknownPlayer is the name the site leaves out: an earlier load gave it the seats CGE holds no player for. No
// account may take it (engine data/pairwise.py knows the name too).
const UnknownPlayer = "Unknown player"

// The file the extract's rows come from, as games and refusals name it.
const extractFile = "tournaments_export.csv"

// The source of the games an earlier CGE scrape loaded: their seats are named as CGE named them then.
const scrapeFile = "cge"

// ApplyCGEExtract writes the extract as one import batch, in one transaction, by plan (PlanCGEExtract's):
// CGE's facts on each matched game and seat (share code, times, account, starting position, outcome, culture,
// CGE's score and standing, and place as cgeextract.Places gives it), the seats CGE holds and we lack, the
// games only CGE has in their groups (each unlinked CGE tournament a hidden series of its own; a possible
// copy is held back), every account, the catalogue's organisers and moderators, special rules, and the
// settings we lack. Matched games take CGE's finish date and group. A seat CGE holds no player for (player id 0)
// is a player removed from the game: it is never added, and our seat paired with it is dropped (removeCopies says
// what a pair by hand removes).
func ApplyCGEExtract(ctx context.Context, pool *pgxpool.Pool, plan CGEPlan, in CGEInput) (CGEApplied, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return CGEApplied{}, fmt.Errorf("starting the extract load: %w", err)
	}
	defer tx.Rollback(ctx)
	got := CGEApplied{Matched: len(plan.Matching.Matched), HeldBack: len(plan.Matching.HeldBack)}
	if err := tx.QueryRow(ctx, "INSERT INTO import_batches (source) VALUES ('load') RETURNING batch_id").Scan(&got.BatchID); err != nil {
		return CGEApplied{}, fmt.Errorf("starting an import batch: %w", err)
	}
	steps := []struct {
		what string
		run  func(context.Context, pgx.Tx, CGEPlan, CGEInput, *CGEApplied) error
	}{
		{"staging the extract", stageExtract},
		{"removing the copies an earlier load added of games paired by hand", removeCopies},
		{"merging the tournaments an earlier load split off hidden series", mergeHiddenSeries},
		{"refusing an account named like the name the site leaves out", keepUnknownPlayer},
		{"writing accounts", writeAccounts},
		{"writing matched games", writeMatched},
		{"naming the seats of games only CGE has by their accounts", followAccounts},
		{"adding games only CGE has", addGames},
		{"writing the catalogue", writeCatalogue},
		{"writing settings", writeSettings},
		{"writing refusals", writeRefusals},
		{"reading outcomes from the files' scores", fileOutcomes},
	}
	for _, s := range steps {
		if err := s.run(ctx, tx, plan, in, &got); err != nil {
			return CGEApplied{}, fmt.Errorf("%s: %w", s.what, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return CGEApplied{}, fmt.Errorf("committing the extract load: %w", err)
	}
	return got, nil
}

// stageExtract copies the extract's seats (with their places), the matches and the seat pairs into temporary
// tables the other steps join.
func stageExtract(ctx context.Context, tx pgx.Tx, plan CGEPlan, in CGEInput, _ *CGEApplied) error {
	for _, q := range []string{
		`CREATE TEMPORARY TABLE x_seats (session int, idx int, tournament int, tournament_name text, stage_number int,
		    group_id int, group_name text, share_code text, state text, started_at timestamptz, finished_at timestamptz,
		    speed text, timer text, players_in_game int, player_id int, player_name text, banned bool,
		    starting_position int, standing int, score float8, outcome text, place int, PRIMARY KEY (session, idx))
		 ON COMMIT DROP`,
		`CREATE TEMPORARY TABLE x_match (game_id bigint PRIMARY KEY, session int) ON COMMIT DROP`,
		`CREATE TEMPORARY TABLE x_pairs (game_id bigint, seat int, session int, idx int) ON COMMIT DROP`,
	} {
		if _, err := tx.Exec(ctx, q); err != nil {
			return err
		}
	}
	var rows [][]any
	for _, g := range plan.Games {
		places := cgeextract.Places(g.Seats)
		for i, s := range g.Seats {
			var outcome *string
			if s.FinishStatus != "" {
				outcome = &s.FinishStatus
			}
			rows = append(rows, []any{g.SessionID, i, g.TournamentID, s.TournamentName, g.StageNumber, g.GroupID, g.GroupName,
				g.ShareCode, g.State, g.StartedAt, g.FinishedAt, g.Speed, g.Timer, g.PlayersInGame, s.PlayerID, s.PlayerName,
				s.Banned, s.StartingPosition, s.Standing, s.Score, outcome, places[i]})
		}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"x_seats"}, []string{"session", "idx", "tournament", "tournament_name",
		"stage_number", "group_id", "group_name", "share_code", "state", "started_at", "finished_at", "speed", "timer",
		"players_in_game", "player_id", "player_name", "banned", "starting_position", "standing", "score", "outcome", "place"},
		pgx.CopyFromRows(rows)); err != nil {
		return err
	}
	bySession := map[int]cgeextract.Game{}
	for _, g := range plan.Games {
		bySession[g.SessionID] = g
	}
	ours := map[int64]cgeextract.OurGame{}
	for _, o := range plan.Ours {
		ours[o.ID] = o
	}
	var matches, pairs [][]any
	for id, session := range plan.Matching.Matched {
		matches = append(matches, []any{id, session})
		seatPairs, _ := cgeextract.MatchSeats(bySession[session], ours[id].Seats, plan.Names)
		for seat, idx := range seatPairs {
			pairs = append(pairs, []any{id, seat, session, idx})
		}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"x_match"}, []string{"game_id", "session"}, pgx.CopyFromRows(matches)); err != nil {
		return err
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"x_pairs"}, []string{"game_id", "seat", "session", "idx"}, pgx.CopyFromRows(pairs))
	return err
}

// removeCopies deletes each game an earlier CGE load added whose session the pairs file now gives to another
// game of ours: its seats go with it, its group when that leaves it empty, and its rows in every run's
// per-game tables (the runs themselves stay).
func removeCopies(ctx context.Context, tx pgx.Tx, _ CGEPlan, _ CGEInput, got *CGEApplied) error {
	for i, q := range []string{
		`CREATE TEMPORARY TABLE x_copies ON COMMIT DROP AS
		 SELECT g.game_id, g.group_id FROM games g
		 JOIN x_seats x ON x.share_code = g.share_code AND x.idx = 0 JOIN x_match m ON m.session = x.session
		 WHERE g.game_id <> m.game_id AND g.source_file IN ('` + extractFile + `', '` + scrapeFile + `')`,
		`DELETE FROM pairwise_matches WHERE match_id IN (SELECT game_id FROM x_copies)`,
		`DELETE FROM tournament_games WHERE match_id IN (SELECT game_id FROM x_copies)`,
		`DELETE FROM games WHERE game_id IN (SELECT game_id FROM x_copies)`,
		`DELETE FROM groups gr WHERE gr.group_id IN (SELECT group_id FROM x_copies)
		   AND NOT EXISTS (SELECT FROM games g WHERE g.group_id = gr.group_id)`,
	} {
		tag, err := tx.Exec(ctx, q)
		if err != nil {
			return err
		}
		if i == 3 {
			got.CopiesRemoved = int(tag.RowsAffected())
		}
	}
	return nil
}

// keepUnknownPlayer refuses to go on if a CGE account is named UnknownPlayer: the name must stay nobody's.
func keepUnknownPlayer(ctx context.Context, tx pgx.Tx, _ CGEPlan, _ CGEInput, _ *CGEApplied) error {
	var named bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT FROM x_seats WHERE player_name = $1)
		OR EXISTS (SELECT FROM player_accounts a JOIN players p USING (player_id) WHERE p.name = $1)`, UnknownPlayer).Scan(&named); err != nil {
		return err
	}
	if named {
		return fmt.Errorf("a CGE account is named %q, the name the site leaves out", UnknownPlayer)
	}
	return nil
}

// writeAccounts adds every account the extract names and refreshes the ones we hold (name, ban, deletion). A
// new account belongs to the player who played its seats in our games, else the player of its name, else a
// new player of that name.
func writeAccounts(ctx context.Context, tx pgx.Tx, _ CGEPlan, _ CGEInput, got *CGEApplied) error {
	for _, q := range []string{
		`CREATE TEMPORARY TABLE x_accounts ON COMMIT DROP AS
		 SELECT DISTINCT ON (player_id) player_id, player_name, banned FROM x_seats WHERE player_id > 0
		 ORDER BY player_id, finished_at DESC NULLS LAST`,
		// Each account's player is the one most of its seats in the organisers' games show; a game CGE's extract
		// or an earlier scrape named has no say, as its names come from CGE's accounts.
		`CREATE TEMPORARY TABLE x_paired ON COMMIT DROP AS
		 SELECT DISTINCT ON (x.player_id) x.player_id, s.player_id AS our_player
		 FROM x_pairs p JOIN x_seats x USING (session, idx) JOIN seats s ON s.game_id = p.game_id AND s.seat = p.seat
		 JOIN games g ON g.game_id = p.game_id WHERE g.source_file NOT IN ('` + extractFile + `', '` + scrapeFile + `')
		   AND x.player_id > 0
		 GROUP BY x.player_id, s.player_id ORDER BY x.player_id, count(*) DESC`,
	} {
		if _, err := tx.Exec(ctx, q); err != nil {
			return err
		}
	}
	// An account an earlier load linked follows the player most of its paired seats show; one linked by hand stays.
	tag, err := tx.Exec(ctx, `
		UPDATE player_accounts pa SET player_id = p.our_player FROM x_paired p
		WHERE pa.cge_user = p.player_id AND pa.player_id <> p.our_player AND pa.linked_reason = ''`)
	if err != nil {
		return err
	}
	got.Relinked = int(tag.RowsAffected())
	tag, err = tx.Exec(ctx, `
		INSERT INTO players (name)
		SELECT a.player_name FROM x_accounts a
		WHERE NOT EXISTS (SELECT FROM player_accounts pa WHERE pa.cge_user = a.player_id)
		  AND NOT EXISTS (SELECT FROM x_paired p WHERE p.player_id = a.player_id)
		ON CONFLICT (name) DO NOTHING`)
	if err != nil {
		return err
	}
	got.PlayersAdded = int(tag.RowsAffected())
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM x_accounts a
		WHERE NOT EXISTS (SELECT FROM player_accounts pa WHERE pa.cge_user = a.player_id)`).Scan(&got.AccountsAdded); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO player_accounts (cge_user, player_id, name, banned, deleted)
		SELECT a.player_id, coalesce(p.our_player, pa.player_id, pl.player_id), a.player_name, a.banned, a.player_name ~ '^deleted_user_[0-9]+$'
		FROM x_accounts a LEFT JOIN x_paired p USING (player_id) LEFT JOIN player_accounts pa ON pa.cge_user = a.player_id
		LEFT JOIN players pl ON pl.name = a.player_name
		ON CONFLICT (cge_user) DO UPDATE SET name = excluded.name, banned = excluded.banned, deleted = excluded.deleted`)
	return err
}

// followAccounts names each seat of a game CGE named (one only CGE has, or one an earlier scrape loaded) after its
// account's player, so its seats follow a relinked account, once matched seats have their accounts; a group's
// members are the players with a seat in its games.
func followAccounts(ctx context.Context, tx pgx.Tx, _ CGEPlan, _ CGEInput, _ *CGEApplied) error {
	for _, q := range []string{
		`UPDATE seats s SET player_id = a.player_id, player_name = pl.name
		 FROM games g, player_accounts a, players pl
		 WHERE g.game_id = s.game_id AND g.source_file IN ('` + extractFile + `', '` + scrapeFile + `') AND a.cge_user = s.cge_user
		   AND pl.player_id = a.player_id AND s.player_id <> a.player_id`,
		`INSERT INTO group_members (group_id, player_id)
		 SELECT DISTINCT g.group_id, s.player_id FROM games g JOIN seats s USING (game_id)
		 WHERE g.group_id IS NOT NULL ON CONFLICT DO NOTHING`,
		`DELETE FROM group_members m WHERE NOT EXISTS (
		   SELECT FROM games g JOIN seats s USING (game_id) WHERE g.group_id = m.group_id AND s.player_id = m.player_id)`,
	} {
		if _, err := tx.Exec(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

// The culture CGE records for a seat: its score when finished, else none.
const cgeCulture = `CASE x.outcome WHEN 'finished' THEN x.score ELSE 0 END`

// writeMatched writes CGE's facts on our matched games and seats, drops our seats of players CGE removed, adds the
// seats only CGE holds, and moves each matched game into our group for its CGE group (moveMatched).
func writeMatched(ctx context.Context, tx pgx.Tx, plan CGEPlan, in CGEInput, got *CGEApplied) error {
	tag, err := tx.Exec(ctx, `
		DELETE FROM seats s USING x_pairs p JOIN x_seats x USING (session, idx)
		WHERE s.game_id = p.game_id AND s.seat = p.seat AND x.player_id = 0`)
	if err != nil {
		return err
	}
	got.SeatsRemoved = int(tag.RowsAffected())
	for _, q := range []string{
		`UPDATE games g SET share_code = x.share_code, cge_session = x.session, started_at = x.started_at,
		        finished_at = x.finished_at, speed = x.speed, timer = x.timer, cge_players_in_game = x.players_in_game,
		        date = coalesce((x.finished_at AT TIME ZONE 'UTC')::date, g.date),
		        cge_finished = coalesce((x.finished_at AT TIME ZONE 'UTC')::date, g.cge_finished),
		        state = CASE WHEN x.state = 'cancelled' THEN 'never_played' ELSE g.state END
		 FROM x_match m JOIN x_seats x ON x.session = m.session AND x.idx = 0
		 WHERE g.game_id = m.game_id`,
		`UPDATE seats s SET cge_user = x.player_id, starting_position = x.starting_position, cge_score = x.score,
		        cge_standing = x.standing, outcome = coalesce(x.outcome, s.outcome),
		        culture = CASE WHEN x.outcome IS NULL THEN s.culture ELSE ` + cgeCulture + ` END,
		        place = CASE WHEN x.state = 'finished' THEN x.place ELSE s.place END
		 FROM x_pairs p JOIN x_seats x USING (session, idx)
		 WHERE s.game_id = p.game_id AND s.seat = p.seat`,
	} {
		if _, err := tx.Exec(ctx, q); err != nil {
			return err
		}
	}
	// A seat only CGE holds joins its game, never past the fourth (a seat we could not pair stays ours).
	tag, err = tx.Exec(ctx, `
		INSERT INTO seats (game_id, seat, player_id, player_name, cge_user, outcome, culture, place, starting_position,
		                   cge_score, cge_standing)
		SELECT * FROM (
			SELECT m.game_id,
			       (SELECT max(seat) FROM seats WHERE game_id = m.game_id) + row_number() OVER (PARTITION BY m.game_id ORDER BY x.idx) AS seat,
			       a.player_id, pl.name, x.player_id, x.outcome, `+cgeCulture+`, x.place, x.starting_position, x.score, x.standing
			FROM x_match m JOIN x_seats x USING (session)
			JOIN player_accounts a ON a.cge_user = x.player_id JOIN players pl ON pl.player_id = a.player_id
			WHERE NOT EXISTS (SELECT FROM x_pairs p WHERE p.session = x.session AND p.idx = x.idx)) s
		WHERE s.seat <= 4`)
	if err != nil {
		return err
	}
	got.SeatsAdded = int(tag.RowsAffected())
	// A group of ours with a CGE group's name, holding only that CGE group's matched games, takes its id.
	_, err = tx.Exec(ctx, `
		WITH pairs AS (
			SELECT DISTINCT x.group_id AS cge_group, x.group_name, g.group_id
			FROM x_match m JOIN games g USING (game_id) JOIN x_seats x ON x.session = m.session AND x.idx = 0
			WHERE g.group_id IS NOT NULL),
		one AS (
			SELECT cge_group, min(group_id) AS group_id FROM pairs GROUP BY cge_group HAVING count(*) = 1)
		UPDATE groups gr SET cge_group = o.cge_group FROM one o JOIN pairs p USING (cge_group)
		WHERE gr.group_id = o.group_id AND gr.cge_group IS NULL AND gr.name = p.group_name
		  AND (SELECT count(*) FROM pairs q WHERE q.group_id = o.group_id) = 1
		  AND NOT EXISTS (SELECT FROM groups g2 WHERE g2.cge_group = o.cge_group)`)
	if err != nil {
		return err
	}
	return moveMatched(ctx, tx, plan, in, got)
}

// moveMatched moves each matched game that is not in our group for its CGE group into it (placeGroup's, made when
// there is none), and deletes the groups that leaves empty.
func moveMatched(ctx context.Context, tx pgx.Tx, plan CGEPlan, in CGEInput, got *CGEApplied) error {
	rows, err := tx.Query(ctx, `
		SELECT m.game_id, m.session FROM x_match m JOIN games g USING (game_id)
		JOIN x_seats x ON x.session = m.session AND x.idx = 0 LEFT JOIN groups gr ON gr.group_id = g.group_id
		WHERE gr.cge_group IS DISTINCT FROM x.group_id ORDER BY m.session`)
	if err != nil {
		return err
	}
	type move struct {
		game    int64
		session int
	}
	var moves []move
	for rows.Next() {
		var mv move
		if err := rows.Scan(&mv.game, &mv.session); err != nil {
			return err
		}
		moves = append(moves, mv)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	bySession := map[int]cgeextract.Game{}
	for _, g := range plan.Games {
		bySession[g.SessionID] = g
	}
	groups := map[int]int64{}
	stageOf := map[[2]int]int64{}
	for _, mv := range moves {
		g := bySession[mv.session]
		group, ok := groups[g.GroupID]
		if !ok {
			if group, err = placeGroup(ctx, tx, plan, in, g, got, stageOf); err != nil {
				return err
			}
			groups[g.GroupID] = group
		}
		if group == 0 {
			continue
		}
		var from *int64
		if err := tx.QueryRow(ctx, "UPDATE games g SET group_id = $2 FROM games old WHERE old.game_id = g.game_id AND g.game_id = $1 RETURNING old.group_id",
			mv.game, group).Scan(&from); err != nil {
			return err
		}
		got.GamesMoved++
		if from != nil {
			if _, err := tx.Exec(ctx, `DELETE FROM groups gr WHERE gr.group_id = $1
				AND NOT EXISTS (SELECT FROM games g WHERE g.group_id = gr.group_id)`, *from); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeCatalogue adds the CGE tournaments the catalogue lacks and records each one's organiser, moderators
// (named, as the extract gives them) and dates.
func writeCatalogue(ctx context.Context, tx pgx.Tx, plan CGEPlan, in CGEInput, _ *CGEApplied) error {
	for _, t := range in.Extract.Tournaments {
		status := "running"
		if t.Finished != nil {
			status = "finished"
		}
		var series *int64
		if s := plan.Series[t.ID]; s != 0 {
			series = &s
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO cge_tournaments (cge_tournament, name, status, organiser, created, started, finished, series_id, organiser_user)
			VALUES ($1, $2, $3, $4, $5::timestamptz::date, $6::timestamptz::date, $7::timestamptz::date, $8,
			        (SELECT cge_user FROM player_accounts WHERE cge_user = $9))
			ON CONFLICT (cge_tournament) DO UPDATE SET
			    organiser_user = excluded.organiser_user, series_id = coalesce(cge_tournaments.series_id, excluded.series_id),
			    created = coalesce(cge_tournaments.created, excluded.created),
			    started = coalesce(cge_tournaments.started, excluded.started),
			    finished = coalesce(cge_tournaments.finished, excluded.finished)`,
			t.ID, t.Name, status, t.OrganiserName, t.Created, t.Started, t.Finished, series, t.OrganiserID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cge_tournament_moderators (cge_tournament, cge_user)
			SELECT DISTINCT $1::int, a.cge_user FROM unnest($2::text[]) AS m(name)
			JOIN player_accounts a ON lower(a.name) = lower(m.name)
			ON CONFLICT DO NOTHING`, t.ID, t.Moderators); err != nil {
			return err
		}
	}
	return nil
}

// writeRefusals records the rows the extract load refused, by line.
func writeRefusals(ctx context.Context, tx pgx.Tx, _ CGEPlan, in CGEInput, got *CGEApplied) error {
	got.Refused = len(in.Extract.Refused)
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"refused_rows"}, []string{"batch_id", "source_file", "source_line", "reason", "fields"},
		pgx.CopyFromSlice(len(in.Extract.Refused), func(i int) ([]any, error) {
			r := in.Extract.Refused[i]
			return []any{got.BatchID, extractFile, r.Line, string(r.Reason), []string{}}, nil
		}))
	return err
}

// fileOutcomes gives each played seat no source recorded an outcome for the one CGE would: above or at 0
// finished, -1 and -5 (the files' marks) timed out, below that resigned; and places its game's seats as CGE
// does: finishers by score, resigners by their score, timed-out seats tied last.
func fileOutcomes(ctx context.Context, tx pgx.Tx, _ CGEPlan, _ CGEInput, _ *CGEApplied) error {
	for _, q := range []string{
		`CREATE TEMPORARY TABLE x_inferred ON COMMIT DROP AS
		 SELECT DISTINCT s.game_id FROM seats s JOIN games g USING (game_id)
		 WHERE g.state = 'played' AND s.outcome IS NULL AND s.recorded_score IS NOT NULL`,
		`UPDATE seats s SET outcome = CASE WHEN s.recorded_score >= 0 THEN 'finished'
		                                   WHEN s.recorded_score IN (-1, -5) THEN 'timed_out' ELSE 'resigned' END,
		                    culture = greatest(s.recorded_score, 0)
		 FROM x_inferred x WHERE s.game_id = x.game_id AND s.outcome IS NULL AND s.recorded_score IS NOT NULL`,
		`WITH ranked AS (
			SELECT s.game_id, s.seat,
			       CASE WHEN s.outcome = 'timed_out' THEN 1 + count(*) FILTER (WHERE s.outcome <> 'timed_out') OVER (PARTITION BY s.game_id)
			            ELSE rank() OVER (PARTITION BY s.game_id ORDER BY CASE s.outcome WHEN 'finished' THEN 0 WHEN 'resigned' THEN 1 ELSE 2 END, s.recorded_score DESC) END AS place
			FROM seats s JOIN x_inferred USING (game_id))
		 UPDATE seats s SET place = r.place FROM ranked r WHERE s.game_id = r.game_id AND s.seat = r.seat`,
	} {
		if _, err := tx.Exec(ctx, q); err != nil {
			return err
		}
	}
	return nil
}
