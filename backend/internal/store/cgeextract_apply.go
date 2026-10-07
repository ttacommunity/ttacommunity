package store

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

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
}

// The file the extract's rows come from, as games and refusals name it.
const extractFile = "tournaments_export.csv"

var trailingGame = regexp.MustCompile(`\s+game\s*\d+\s*$`)

// ApplyCGEExtract writes the extract as one import batch, in one transaction, by plan (PlanCGEExtract's):
// CGE's facts on each matched game and seat (share code, times, account, starting position, outcome, culture,
// CGE's score and standing, and place as cgeextract.Places gives it), the seats CGE holds and we lack, the
// games only CGE has in their groups (each unlinked CGE tournament a hidden series of its own; a possible
// copy is held back), every account, the catalogue's organisers and moderators, special rules, and the
// settings we lack. Matched games take CGE's finish date.
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
		{"writing accounts", writeAccounts},
		{"writing matched games", writeMatched},
		{"adding games only CGE has", addGames},
		{"writing the catalogue", writeCatalogue},
		{"writing settings", writeSettings},
		{"writing refusals", writeRefusals},
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

// writeAccounts adds every account the extract names and refreshes the ones we hold (name, ban, deletion). A
// new account belongs to the player who played its seats in our games, else the player of its name, else a
// new player of that name.
func writeAccounts(ctx context.Context, tx pgx.Tx, _ CGEPlan, _ CGEInput, got *CGEApplied) error {
	for _, q := range []string{
		`CREATE TEMPORARY TABLE x_accounts ON COMMIT DROP AS
		 SELECT DISTINCT ON (player_id) player_id, player_name, banned FROM x_seats
		 ORDER BY player_id, finished_at DESC NULLS LAST`,
		`CREATE TEMPORARY TABLE x_paired ON COMMIT DROP AS
		 SELECT DISTINCT ON (x.player_id) x.player_id, s.player_id AS our_player
		 FROM x_pairs p JOIN x_seats x USING (session, idx) JOIN seats s ON s.game_id = p.game_id AND s.seat = p.seat
		 GROUP BY x.player_id, s.player_id ORDER BY x.player_id, count(*) DESC`,
	} {
		if _, err := tx.Exec(ctx, q); err != nil {
			return err
		}
	}
	tag, err := tx.Exec(ctx, `
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
		SELECT a.player_id, coalesce(p.our_player, pl.player_id), a.player_name, a.banned, a.player_name ~ '^deleted_user_[0-9]+$'
		FROM x_accounts a LEFT JOIN x_paired p USING (player_id) LEFT JOIN players pl ON pl.name = a.player_name
		ON CONFLICT (cge_user) DO UPDATE SET name = excluded.name, banned = excluded.banned, deleted = excluded.deleted`)
	return err
}

// The culture CGE records for a seat: its score when finished, else none.
const cgeCulture = `CASE x.outcome WHEN 'finished' THEN x.score ELSE 0 END`

// writeMatched writes CGE's facts on our matched games and seats, and adds the seats only CGE holds.
func writeMatched(ctx context.Context, tx pgx.Tx, _ CGEPlan, _ CGEInput, got *CGEApplied) error {
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
	tag, err := tx.Exec(ctx, `
		INSERT INTO seats (game_id, seat, player_id, player_name, cge_user, outcome, culture, place, starting_position,
		                   cge_score, cge_standing)
		SELECT m.game_id,
		       (SELECT max(seat) FROM seats WHERE game_id = m.game_id) + row_number() OVER (PARTITION BY m.game_id ORDER BY x.idx),
		       a.player_id, pl.name, x.player_id, x.outcome, `+cgeCulture+`, x.place, x.starting_position, x.score, x.standing
		FROM x_match m JOIN x_seats x USING (session)
		JOIN player_accounts a ON a.cge_user = x.player_id JOIN players pl ON pl.player_id = a.player_id
		WHERE NOT EXISTS (SELECT FROM x_pairs p WHERE p.session = x.session AND p.idx = x.idx)`)
	if err != nil {
		return err
	}
	got.SeatsAdded = int(tag.RowsAffected())
	// A CGE group that is one of ours, and ours only that one, takes its id.
	_, err = tx.Exec(ctx, `
		WITH pairs AS (
			SELECT DISTINCT x.group_id AS cge_group, g.group_id
			FROM x_match m JOIN games g USING (game_id) JOIN x_seats x ON x.session = m.session AND x.idx = 0
			WHERE g.group_id IS NOT NULL),
		one AS (
			SELECT cge_group, min(group_id) AS group_id FROM pairs GROUP BY cge_group HAVING count(*) = 1)
		UPDATE groups gr SET cge_group = o.cge_group FROM one o
		WHERE gr.group_id = o.group_id AND gr.cge_group IS NULL
		  AND (SELECT count(*) FROM pairs p WHERE p.group_id = o.group_id) = 1
		  AND NOT EXISTS (SELECT FROM groups g2 WHERE g2.cge_group = o.cge_group)`)
	return err
}

// added is one game only CGE has, placed: its group, name and number.
type added struct {
	session, gameNumber int
	group               int64
	name, state         string
	date                time.Time
}

// addGames places each game only CGE has (not held back) in our group for its CGE group, making the group,
// stage, tournament or hidden series it needs, then writes the games and their seats.
func addGames(ctx context.Context, tx pgx.Tx, plan CGEPlan, in CGEInput, got *CGEApplied) error {
	bySession := map[int]cgeextract.Game{}
	for _, g := range plan.Games {
		bySession[g.SessionID] = g
	}
	latest := time.Time{}
	for _, g := range plan.Games {
		if g.FinishedAt != nil && g.FinishedAt.After(latest) {
			latest = *g.FinishedAt
		}
	}
	groups := map[int]int64{}
	numbers := map[int64]int{}
	var rows []added
	for _, session := range plan.Matching.Added {
		g := bySession[session]
		group, ok := groups[g.GroupID]
		if !ok {
			var err error
			if group, err = placeGroup(ctx, tx, plan, in, g, got); err != nil {
				return err
			}
			groups[g.GroupID] = group
		}
		if group == 0 {
			got.Unplaced++
			continue
		}
		if _, ok := numbers[group]; !ok {
			var highest int
			if err := tx.QueryRow(ctx, "SELECT coalesce(max(game_number), 0) FROM games WHERE group_id = $1", group).Scan(&highest); err != nil {
				return err
			}
			numbers[group] = highest
		}
		numbers[group]++
		name, err := gameName(ctx, tx, group, g, numbers[group])
		if err != nil {
			return err
		}
		rows = append(rows, added{session: session, gameNumber: numbers[group], group: group, name: name,
			state: gameState(g), date: gameDate(g, in.Extract, latest)})
	}
	got.Added = len(rows)
	if _, err := tx.Exec(ctx, `CREATE TEMPORARY TABLE x_added (session int PRIMARY KEY, group_id bigint, name text,
		game_number int, state text, date date) ON COMMIT DROP`); err != nil {
		return err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"x_added"}, []string{"session", "group_id", "name", "game_number", "state", "date"},
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			r := rows[i]
			return []any{r.session, r.group, r.name, r.gameNumber, r.state, r.date}, nil
		})); err != nil {
		return err
	}
	for _, q := range []string{
		`INSERT INTO games (batch_id, source_file, source_line, name, date, state, group_id, game_number, share_code,
		                    cge_session, started_at, finished_at, speed, timer, cge_players_in_game, cge_finished)
		 SELECT $1, '` + extractFile + `', a.session, a.name, a.date, a.state, a.group_id, a.game_number, x.share_code,
		        x.session, x.started_at, x.finished_at, x.speed, x.timer, x.players_in_game,
		        (x.finished_at AT TIME ZONE 'UTC')::date
		 FROM x_added a JOIN x_seats x ON x.session = a.session AND x.idx = 0`,
		`INSERT INTO seats (game_id, seat, player_id, player_name, cge_user, outcome, culture, place, starting_position,
		                    cge_score, cge_standing)
		 SELECT g.game_id, x.idx + 1, a.player_id, pl.name, x.player_id, x.outcome, ` + cgeCulture + `, x.place,
		        x.starting_position, x.score, x.standing
		 FROM x_added ad JOIN games g ON g.cge_session = ad.session JOIN x_seats x ON x.session = ad.session
		 JOIN player_accounts a ON a.cge_user = x.player_id JOIN players pl ON pl.player_id = a.player_id`,
		`INSERT INTO group_members (group_id, player_id)
		 SELECT DISTINCT g.group_id, s.player_id FROM games g JOIN seats s USING (game_id)
		 WHERE g.cge_session IS NOT NULL AND g.group_id IS NOT NULL
		 ON CONFLICT DO NOTHING`,
	} {
		args := []any{}
		if strings.Contains(q, "$1") {
			args = append(args, got.BatchID)
		}
		if _, err := tx.Exec(ctx, q, args...); err != nil {
			return err
		}
	}
	for session, s := range in.Special {
		if _, err := tx.Exec(ctx, "UPDATE games SET rules = $2, card_set = $3 WHERE cge_session = $1", session, s.Rules, s.CardSet); err != nil {
			return err
		}
	}
	return nil
}

// placeGroup returns our group for a CGE game's group: the one that took its id, else a new group in our stage
// for its CGE stage, making that stage (and a league's season, or an unlinked tournament's hidden series) when
// there is none. It returns 0 when a league's season cannot be told from its other stages.
func placeGroup(ctx context.Context, tx pgx.Tx, plan CGEPlan, in CGEInput, g cgeextract.Game, got *CGEApplied) (int64, error) {
	var group int64
	err := tx.QueryRow(ctx, "SELECT group_id FROM groups WHERE cge_group = $1", g.GroupID).Scan(&group)
	if err == nil {
		return group, nil
	}
	if err != pgx.ErrNoRows {
		return 0, err
	}
	stage, err := stageFor(ctx, tx, plan, in, g, got)
	if err != nil || stage == 0 {
		return 0, err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO groups (stage_id, name, cge_tournament, cge_stage_number, cge_group) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (stage_id, name, cge_tournament, cge_stage_number) DO UPDATE SET cge_group = coalesce(groups.cge_group, excluded.cge_group)
		RETURNING group_id`, stage, g.GroupName, g.TournamentID, g.StageNumber, g.GroupID).Scan(&group)
	return group, err
}

// stageFor returns our stage for a CGE game's stage, making it when needed, or 0 when it cannot be placed.
func stageFor(ctx context.Context, tx pgx.Tx, plan CGEPlan, in CGEInput, g cgeextract.Game, got *CGEApplied) (int64, error) {
	var stage int64
	err := tx.QueryRow(ctx, `
		SELECT stage_id FROM (
			SELECT st.stage_id, count(*) AS n FROM x_match m JOIN games ga USING (game_id) JOIN groups gr USING (group_id)
			JOIN stages st USING (stage_id) JOIN x_seats x ON x.session = m.session AND x.idx = 0
			WHERE x.tournament = $1 AND x.stage_number = $2 GROUP BY st.stage_id
			UNION ALL SELECT stage_id, 0 FROM stages WHERE cge_tournament = $1 AND cge_stage_number = $2
			UNION ALL SELECT stage_id, 0 FROM groups WHERE cge_tournament = $1 AND cge_stage_number = $2) s
		ORDER BY n DESC LIMIT 1`, g.TournamentID, g.StageNumber).Scan(&stage)
	if err == nil {
		return stage, nil
	}
	if err != pgx.ErrNoRows {
		return 0, err
	}
	series := plan.Series[g.TournamentID]
	hidden := series == 0
	var tournament *int64
	if !hidden {
		if err := tx.QueryRow(ctx, "SELECT tournament_id FROM cge_tournaments WHERE cge_tournament = $1", g.TournamentID).Scan(&tournament); err != nil && err != pgx.ErrNoRows {
			return 0, err
		}
	} else {
		name := in.Extract.Tournaments[g.TournamentID].Name
		if name == "" {
			name = g.Seats[0].TournamentName
		}
		tag, err := tx.Exec(ctx, `INSERT INTO series (slug, name, original_name, visible) VALUES ($1, $2, $2, false)
			ON CONFLICT (slug) DO NOTHING`, fmt.Sprintf("cge_%d", g.TournamentID), name)
		if err != nil {
			return 0, err
		}
		got.SeriesAdded += int(tag.RowsAffected())
		if err := tx.QueryRow(ctx, "SELECT series_id FROM series WHERE slug = $1", fmt.Sprintf("cge_%d", g.TournamentID)).Scan(&series); err != nil {
			return 0, err
		}
		plan.Series[g.TournamentID] = series
	}
	label, stageLabel := "", fmt.Sprintf("Stage %d", g.StageNumber)
	switch {
	case tournament != nil:
	case hidden:
		label = "1"
	default:
		// A league: its CGE stage is a season, numbered as its other stages are.
		var offsets []int
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT t.label::int - c.n FROM (
				SELECT stage_id, cge_stage_number AS n FROM stages WHERE cge_tournament = $1
				UNION SELECT stage_id, cge_stage_number FROM groups WHERE cge_tournament = $1) c
			JOIN stages st USING (stage_id) JOIN tournaments t USING (tournament_id) WHERE t.label ~ '^[0-9]+$'`, g.TournamentID)
		if err != nil {
			return 0, err
		}
		offsets, err = pgx.CollectRows(rows, pgx.RowTo[int])
		if err != nil {
			return 0, err
		}
		if len(offsets) != 1 {
			return 0, nil
		}
		label, stageLabel = strconv.Itoa(g.StageNumber+offsets[0]), ""
	}
	if tournament == nil {
		var id int64
		if err := tx.QueryRow(ctx, `INSERT INTO tournaments (series_id, label, original_name) VALUES ($1, $2, $2)
			ON CONFLICT (series_id, label) DO UPDATE SET label = excluded.label RETURNING tournament_id`, series, label).Scan(&id); err != nil {
			return 0, err
		}
		tournament = &id
	}
	err = tx.QueryRow(ctx, `INSERT INTO stages (tournament_id, label, original_name, cge_tournament, cge_stage_number)
		VALUES ($1, $2, $2, $3, $4)
		ON CONFLICT (tournament_id, label) DO UPDATE SET label = excluded.label RETURNING stage_id`,
		*tournament, stageLabel, g.TournamentID, g.StageNumber).Scan(&stage)
	return stage, err
}

// gameName names a game as its group's games are named ("Intermezzo S24 - Bronze 1 game 3"), else as its
// stage's games with its group's name, else its CGE tournament, stage and group.
func gameName(ctx context.Context, tx pgx.Tx, group int64, g cgeextract.Game, number int) (string, error) {
	var sibling string
	err := tx.QueryRow(ctx, `SELECT name FROM games WHERE group_id = $1 ORDER BY game_id LIMIT 1`, group).Scan(&sibling)
	if err == nil {
		return fmt.Sprintf("%s game %d", trailingGame.ReplaceAllString(sibling, ""), number), nil
	}
	if err != pgx.ErrNoRows {
		return "", err
	}
	err = tx.QueryRow(ctx, `SELECT g.name FROM games g JOIN groups gr USING (group_id)
		WHERE gr.stage_id = (SELECT stage_id FROM groups WHERE group_id = $1) ORDER BY g.game_id LIMIT 1`, group).Scan(&sibling)
	if err == nil {
		if prefix, _, ok := strings.Cut(sibling, " - "); ok {
			return fmt.Sprintf("%s - %s game %d", prefix, g.GroupName, number), nil
		}
	} else if err != pgx.ErrNoRows {
		return "", err
	}
	name := fmt.Sprintf("%s Stage %d", g.Seats[0].TournamentName, g.StageNumber)
	if g.GroupName != "" {
		name += " - " + g.GroupName
	}
	return fmt.Sprintf("%s game %d", name, number), nil
}

// gameState is a CGE game's state as ours: never played when cancelled or no one finished or resigned.
func gameState(g cgeextract.Game) string {
	switch g.State {
	case "cancelled":
		return "never_played"
	case "running", "prepared":
		return g.State
	}
	if !slices.ContainsFunc(g.Seats, func(s cgeextract.Seat) bool {
		return s.FinishStatus == cgeextract.Finished || s.FinishStatus == cgeextract.Resigned
	}) {
		return "never_played"
	}
	return "played"
}

// gameDate is the UTC day a CGE game finished, else started, else its stage or tournament began, else latest.
func gameDate(g cgeextract.Game, x cgeextract.Extract, latest time.Time) time.Time {
	for _, at := range []*time.Time{g.FinishedAt, g.StartedAt, x.Stages[g.StageID].Started, x.Tournaments[g.TournamentID].Started,
		x.Tournaments[g.TournamentID].Created} {
		if at != nil {
			return at.UTC().Truncate(24 * time.Hour)
		}
	}
	return latest.UTC().Truncate(24 * time.Hour)
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

// writeSettings fills the settings we lack from CGE's: a stage's on our stage for it, a cup year's on its
// tournament, a league's on its series. A setting we hold is kept.
func writeSettings(ctx context.Context, tx pgx.Tx, plan CGEPlan, in CGEInput, got *CGEApplied) error {
	for _, s := range in.Settings {
		target, id, err := settingsTarget(ctx, tx, plan, in, s)
		if err != nil {
			return err
		}
		if id == 0 {
			continue
		}
		points := func(n int) any {
			if p := s.Points[n]; p != nil {
				return *p
			}
			return nil
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO settings (`+target+`, timeout_penalty, points_2p, points_3p, points_4p, tiebreak_2p, tiebreak_3p_plus,
			                      ignore_ai_players, each_plays, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'cge')
			ON CONFLICT (`+target+`) DO UPDATE SET
			    timeout_penalty = coalesce(settings.timeout_penalty, excluded.timeout_penalty),
			    points_2p = coalesce(settings.points_2p, excluded.points_2p),
			    points_3p = coalesce(settings.points_3p, excluded.points_3p),
			    points_4p = coalesce(settings.points_4p, excluded.points_4p),
			    tiebreak_2p = coalesce(settings.tiebreak_2p, excluded.tiebreak_2p),
			    tiebreak_3p_plus = coalesce(settings.tiebreak_3p_plus, excluded.tiebreak_3p_plus),
			    ignore_ai_players = coalesce(settings.ignore_ai_players, excluded.ignore_ai_players),
			    each_plays = coalesce(settings.each_plays, excluded.each_plays),
			    source = CASE WHEN settings.source IN ('', 'catalogue') THEN 'cge' ELSE settings.source END`,
			id, s.TimeoutPenalty, points(2), points(3), points(4), s.Tiebreak2p, s.Tiebreak3pPlus, s.IgnoreAI, s.EachPlays)
		if err != nil {
			return err
		}
		got.SettingsWritten += int(tag.RowsAffected())
	}
	return nil
}

// settingsTarget returns the settings column and our id CGE's settings row belongs to, or 0 for none.
func settingsTarget(ctx context.Context, tx pgx.Tx, plan CGEPlan, in CGEInput, s cgeextract.Settings) (string, int64, error) {
	var id int64
	if s.StageID != 0 {
		stage := in.Extract.Stages[s.StageID]
		err := tx.QueryRow(ctx, `
			SELECT stage_id FROM stages WHERE cge_tournament = $1 AND cge_stage_number = $2
			UNION SELECT stage_id FROM groups WHERE cge_tournament = $1 AND cge_stage_number = $2 LIMIT 1`,
			s.TournamentID, stage.Number).Scan(&id)
		if err == pgx.ErrNoRows {
			return "", 0, nil
		}
		return "stage_id", id, err
	}
	var tournament *int64
	err := tx.QueryRow(ctx, "SELECT tournament_id FROM cge_tournaments WHERE cge_tournament = $1", s.TournamentID).Scan(&tournament)
	if err != nil && err != pgx.ErrNoRows {
		return "", 0, err
	}
	if tournament != nil {
		return "tournament_id", *tournament, nil
	}
	return "series_id", plan.Series[s.TournamentID], nil
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
