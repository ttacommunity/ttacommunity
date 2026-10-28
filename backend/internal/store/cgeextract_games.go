// CGE's extract, its games only CGE has: each placed in our group, stage and tournament for it, made when needed,
// then named and written with its seats.
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

	"ttacommunity/backend/internal/cgeextract"
)

var trailingGame = regexp.MustCompile(`\s+game\s*\d+\s*$`)

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
	prefixes := map[int64]string{}
	stageOf := map[[2]int]int64{}
	stages := map[int64]*string{}
	var rows []added
	for _, session := range plan.Matching.Added {
		g := bySession[session]
		group, ok := groups[g.GroupID]
		if !ok {
			var err error
			if group, err = placeGroup(ctx, tx, plan, in, g, got, stageOf); err != nil {
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
		prefix, ok := prefixes[group]
		if !ok {
			var err error
			if prefix, err = groupPrefix(ctx, tx, group, g, stages); err != nil {
				return err
			}
			prefixes[group] = prefix
		}
		name := fmt.Sprintf("%s game %d", prefix, numbers[group])
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
		 SELECT g.game_id, x.idx + 1, ` + seatPlayer + `, ` + seatPlayerName + `, nullif(x.player_id, 0), x.outcome, ` + cgeCulture + `,
		        x.place, x.starting_position, x.score, x.standing
		 FROM x_added ad JOIN games g ON g.cge_session = ad.session JOIN x_seats x ON x.session = ad.session
		 LEFT JOIN player_accounts a ON a.cge_user = x.player_id LEFT JOIN players pl ON pl.player_id = a.player_id`,
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

// mergeHiddenSeries moves the stages of each hidden series' other tournaments under its tournament "1", naming
// those left unnamed by their CGE stage, and deletes the tournaments it empties.
func mergeHiddenSeries(ctx context.Context, tx pgx.Tx, _ CGEPlan, _ CGEInput, got *CGEApplied) error {
	for _, q := range []string{
		`INSERT INTO tournaments (series_id, label, original_name)
		 SELECT DISTINCT se.series_id, '1', '1' FROM series se JOIN tournaments t USING (series_id)
		 WHERE se.slug ~ '^cge_[0-9]+$' AND NOT se.visible AND t.label <> '1'
		 ON CONFLICT (series_id, label) DO NOTHING`,
		`UPDATE stages st SET tournament_id = one.tournament_id,
		     label = CASE st.label WHEN '' THEN 'Stage ' || st.cge_stage_number ELSE st.label END,
		     original_name = CASE st.original_name WHEN '' THEN 'Stage ' || st.cge_stage_number ELSE st.original_name END
		 FROM tournaments t JOIN series se USING (series_id) JOIN tournaments one ON one.series_id = se.series_id AND one.label = '1'
		 WHERE st.tournament_id = t.tournament_id AND se.slug ~ '^cge_[0-9]+$' AND NOT se.visible AND t.label <> '1'`,
	} {
		if _, err := tx.Exec(ctx, q); err != nil {
			return err
		}
	}
	tag, err := tx.Exec(ctx, `DELETE FROM tournaments t USING series se
		WHERE se.series_id = t.series_id AND se.slug ~ '^cge_[0-9]+$' AND NOT se.visible AND t.label <> '1'
		  AND NOT EXISTS (SELECT FROM stages st WHERE st.tournament_id = t.tournament_id)`)
	got.TournamentsMerged = int(tag.RowsAffected())
	return err
}

// placeGroup returns our group for a CGE game's group (stageOf caches our stage per CGE stage): the one that took its id, else a new group in our stage
// for its CGE stage, making that stage (and a league's season, or an unlinked tournament's hidden series) when
// there is none. It returns 0 when a league's season cannot be told from its other stages.
func placeGroup(ctx context.Context, tx pgx.Tx, plan CGEPlan, in CGEInput, g cgeextract.Game, got *CGEApplied, stageOf map[[2]int]int64) (int64, error) {
	var group int64
	err := tx.QueryRow(ctx, "SELECT group_id FROM groups WHERE cge_group = $1", g.GroupID).Scan(&group)
	if err == nil {
		return group, nil
	}
	if err != pgx.ErrNoRows {
		return 0, err
	}
	key := [2]int{g.TournamentID, g.StageNumber}
	stage, ok := stageOf[key]
	if !ok {
		if stage, err = stageFor(ctx, tx, plan, in, g, got); err != nil {
			return 0, err
		}
		stageOf[key] = stage
	}
	if stage == 0 {
		return 0, nil
	}
	var unlinked int64
	err = tx.QueryRow(ctx, `SELECT group_id FROM groups WHERE stage_id = $1 AND name = $2 AND cge_tournament = $3
		AND cge_stage_number = $4 AND cge_group IS NULL LIMIT 1`, stage, g.GroupName, g.TournamentID, g.StageNumber).Scan(&unlinked)
	if err == nil {
		return 0, fmt.Errorf("group %d is CGE tournament %d stage %d's %q but has no CGE group: link it in the data first",
			unlinked, g.TournamentID, g.StageNumber, g.GroupName)
	}
	if err != pgx.ErrNoRows {
		return 0, err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO groups (stage_id, name, cge_tournament, cge_stage_number, cge_group) VALUES ($1, $2, $3, $4, $5)
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
	slug := fmt.Sprintf("cge_%d", g.TournamentID)
	series := plan.Series[g.TournamentID]
	var own int64
	if err := tx.QueryRow(ctx, "SELECT series_id FROM series WHERE slug = $1", slug).Scan(&own); err != nil && err != pgx.ErrNoRows {
		return 0, err
	}
	hidden := series == 0 || series == own
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
			ON CONFLICT (slug) DO NOTHING`, slug, name)
		if err != nil {
			return 0, err
		}
		got.SeriesAdded += int(tag.RowsAffected())
		if err := tx.QueryRow(ctx, "SELECT series_id FROM series WHERE slug = $1", slug).Scan(&series); err != nil {
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

// groupPrefix returns how a group's games are named before " game N": as its games are ("Intermezzo S24 - Bronze 1"),
// else as its stage's games with its group's name, else its CGE tournament, stage and group. stages caches each
// stage's first game name (nil for none).
func groupPrefix(ctx context.Context, tx pgx.Tx, group int64, g cgeextract.Game, stages map[int64]*string) (string, error) {
	var sibling string
	err := tx.QueryRow(ctx, `SELECT name FROM games WHERE group_id = $1 ORDER BY game_id LIMIT 1`, group).Scan(&sibling)
	if err == nil {
		return trailingGame.ReplaceAllString(sibling, ""), nil
	}
	if err != pgx.ErrNoRows {
		return "", err
	}
	var stage int64
	if err := tx.QueryRow(ctx, "SELECT stage_id FROM groups WHERE group_id = $1", group).Scan(&stage); err != nil {
		return "", err
	}
	first, ok := stages[stage]
	if !ok {
		var name string
		err := tx.QueryRow(ctx, `SELECT g.name FROM games g JOIN groups gr USING (group_id)
			WHERE gr.stage_id = $1 ORDER BY g.game_id LIMIT 1`, stage).Scan(&name)
		if err != nil && err != pgx.ErrNoRows {
			return "", err
		}
		if err == nil {
			first = &name
		}
		stages[stage] = first
	}
	if first != nil {
		if prefix, _, ok := strings.Cut(*first, " - "); ok {
			return prefix + " - " + g.GroupName, nil
		}
	}
	name := fmt.Sprintf("%s Stage %d", g.Seats[0].TournamentName, g.StageNumber)
	if g.GroupName != "" {
		name += " - " + g.GroupName
	}
	return name, nil
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
