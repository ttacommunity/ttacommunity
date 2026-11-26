// Games changed in admin: a game added to a group, a game removed, or a game replaced by a new one, each kept in
// game_changes with its reason and undone from there.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrNoGame means no game we hold has the share code.
var ErrNoGame = errors.New("no game we hold has that code")

// ErrGameChanged means the game is already removed or replaced.
var ErrGameChanged = errors.New("that game is already removed or replaced")

// ErrNoGroup means the group does not exist.
var ErrNoGroup = errors.New("no such group")

// ErrCodeHeld means a game we hold already has the new game's share code.
var ErrCodeHeld = errors.New("a game we hold already has that code")

// ErrNumberHeld means a game of the group still in play has that game number.
var ErrNumberHeld = errors.New("a game of that group has that number")

// ErrUnknownPlayer means a seat names a player we do not hold.
var ErrUnknownPlayer = errors.New("a seat names a player we do not hold")

// ErrBadGameEntry means a new game or a reason does not fit: a code and a date, 2 to 4 different players, each with
// a place from 1 to 4 and an outcome (finished, resigned or timed_out), culture only when finished and never below
// 0, and a reason of at most 1,000 characters.
var ErrBadGameEntry = errors.New("a game needs a code, a date and 2 to 4 different players, each with a place from 1 to 4, " +
	"an outcome (finished, resigned or timed out) and culture of 0 or more (0 unless finished); a change needs a reason")

// ErrNoGameChange means no change has that id.
var ErrNoGameChange = errors.New("no such game change")

// ErrAddedGameLoaded means a CGE extract has since matched the game the change added: undone, the next load would add it back.
var ErrAddedGameLoaded = errors.New("a CGE extract has matched the game it added since, so the next load would add it back")

// ErrChangedSince means the game a change added has since been changed: that change is undone first.
var ErrChangedSince = errors.New("the game it added has been changed since: undo that change first")

// GameEntry is a game typed in admin: its share code, date, game number (0 for the group's next) and seats.
type GameEntry struct {
	Code   string
	Date   time.Time
	Number int
	Seats  []GameSeatEntry
}

// GameSeatEntry is one seat of a GameEntry: a player we hold, by name, with their place, outcome and culture.
type GameSeatEntry struct {
	Player  string
	Place   int
	Outcome string
	Score   float64
}

// GameChange is one change made in admin: add, remove or replace, the game removed or replaced (nil for add), the
// game added (nil for remove), its reason and when it was made.
type GameChange struct {
	ID     int64
	Action string
	Game   *ChangedGame
	Added  *ChangedGame
	Reason string
	At     time.Time
}

// ChangedGame is a game a change names, as it is now.
type ChangedGame struct {
	ID    int64
	Name  string
	Code  string
	State string
	Date  time.Time
}

// GameChanges returns every change made in admin, newest first.
func (a *AdminStore) GameChanges(ctx context.Context) ([]GameChange, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT c.change_id, c.action, c.reason, c.at,
		       g.game_id, g.name, coalesce(g.share_code, ''), g.state, g.date,
		       n.game_id, n.name, coalesce(n.share_code, ''), n.state, n.date
		FROM game_changes c LEFT JOIN games g ON g.game_id = c.game_id LEFT JOIN games n ON n.game_id = c.added_game_id
		ORDER BY c.at DESC, c.change_id DESC`)
	if err != nil {
		return nil, fmt.Errorf("reading game changes: %w", err)
	}
	var got []GameChange
	for rows.Next() {
		var c GameChange
		var gID, nID *int64
		var gName, gCode, gState, nName, nCode, nState *string
		var gDate, nDate *time.Time
		if err := rows.Scan(&c.ID, &c.Action, &c.Reason, &c.At, &gID, &gName, &gCode, &gState, &gDate,
			&nID, &nName, &nCode, &nState, &nDate); err != nil {
			return nil, fmt.Errorf("reading game changes: %w", err)
		}
		if gID != nil {
			c.Game = &ChangedGame{ID: *gID, Name: *gName, Code: *gCode, State: *gState, Date: *gDate}
		}
		if nID != nil {
			c.Added = &ChangedGame{ID: *nID, Name: *nName, Code: *nCode, State: *nState, Date: *nDate}
		}
		got = append(got, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading game changes: %w", err)
	}
	return got, nil
}

// AddGame adds the game to the group, named and numbered as its next game unless the entry gives a number, and
// returns the change's id.
func (a *AdminStore) AddGame(ctx context.Context, group int64, g GameEntry, reason string) (int64, error) {
	return a.changeGames(ctx, func(tx pgx.Tx) (int64, error) {
		if err := tx.QueryRow(ctx, "SELECT group_id FROM groups WHERE group_id = $1", group).Scan(&group); errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNoGroup
		} else if err != nil {
			return 0, err
		}
		number := g.Number
		if number == 0 {
			if err := tx.QueryRow(ctx, "SELECT coalesce(max(game_number), 0) + 1 FROM games WHERE group_id = $1", group).Scan(&number); err != nil {
				return 0, err
			}
		}
		prefix, err := addedPrefix(ctx, tx, group)
		if err != nil {
			return 0, err
		}
		added, err := insertGame(ctx, tx, g, fmt.Sprintf("%s game %d", prefix, number), &group, &number)
		if err != nil {
			return 0, err
		}
		return insertChange(ctx, tx, "add", nil, nil, &added, reason)
	})
}

// RemoveGame takes the game with the share code out of play, kept as removed, and returns the change's id.
func (a *AdminStore) RemoveGame(ctx context.Context, code, reason string) (int64, error) {
	return a.changeGames(ctx, func(tx pgx.Tx) (int64, error) {
		game, state, err := gameToChange(ctx, tx, code)
		if err != nil {
			return 0, err
		}
		id, err := insertChange(ctx, tx, "remove", &game, &state, nil, reason)
		if err != nil {
			return 0, err
		}
		_, err = tx.Exec(ctx, "UPDATE games SET state = 'removed' WHERE game_id = $1", game)
		return id, err
	})
}

// ReplaceGame keeps the game with the share code as replaced and adds the new one in its group, with its name and
// game number (the entry's number is not used), and returns the change's id.
func (a *AdminStore) ReplaceGame(ctx context.Context, code string, g GameEntry, reason string) (int64, error) {
	return a.changeGames(ctx, func(tx pgx.Tx) (int64, error) {
		game, state, err := gameToChange(ctx, tx, code)
		if err != nil {
			return 0, err
		}
		var name string
		var group *int64
		var number *int
		if err := tx.QueryRow(ctx, "UPDATE games SET state = 'replaced' WHERE game_id = $1 RETURNING name, group_id, game_number", game).
			Scan(&name, &group, &number); err != nil {
			return 0, err
		}
		added, err := insertGame(ctx, tx, g, name, group, number)
		if err != nil {
			return 0, err
		}
		return insertChange(ctx, tx, "replace", &game, &state, &added, reason)
	})
}

// UndoGameChange undoes the change: the game it added is deleted, and the game it removed or replaced gets back its
// earlier state. It returns ErrNoGameChange, ErrChangedSince, or ErrNumberHeld when the game's number has been used again.
func (a *AdminStore) UndoGameChange(ctx context.Context, id int64) error {
	_, err := a.pool.Exec(ctx, "SELECT undo_game_change($1)", id)
	var pg *pgconn.PgError
	switch {
	case errors.As(err, &pg) && pg.Code == "P0002":
		return ErrNoGameChange
	case errors.As(err, &pg) && pg.Code == "55006":
		return ErrAddedGameLoaded
	case errors.As(err, &pg) && pg.ConstraintName == "game_changes_game_id_fkey":
		return ErrChangedSince
	case errors.As(err, &pg) && pg.ConstraintName == "games_group_number":
		return ErrNumberHeld
	case err != nil:
		return fmt.Errorf("undoing game change %d: %w", id, err)
	}
	return nil
}

// changeGames runs change in a transaction, turning the database's refusals into the store's errors.
func (a *AdminStore) changeGames(ctx context.Context, change func(pgx.Tx) (int64, error)) (int64, error) {
	var id int64
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) (err error) {
		id, err = change(tx)
		return err
	})
	var pg *pgconn.PgError
	if !errors.As(err, &pg) {
		return id, err // nil, or one of the store's errors
	}
	switch {
	case pg.ConstraintName == "games_share_code_key":
		return 0, ErrCodeHeld
	case pg.ConstraintName == "games_group_number":
		return 0, ErrNumberHeld
	case pg.ConstraintName == "game_changes_game_id_key":
		return 0, ErrGameChanged
	case pg.Code == "23514":
		return 0, ErrBadGameEntry
	}
	return 0, fmt.Errorf("changing games: %w", err)
}

// gameToChange returns the id and state of the game with the share code, locked, refusing one already removed or
// replaced.
func gameToChange(ctx context.Context, tx pgx.Tx, code string) (int64, string, error) {
	var id int64
	var state string
	err := tx.QueryRow(ctx, "SELECT game_id, state FROM games WHERE share_code = $1 FOR UPDATE", strings.TrimSpace(code)).Scan(&id, &state)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return 0, "", ErrNoGame
	case err != nil:
		return 0, "", err
	case state == "removed" || state == "replaced":
		return 0, "", ErrGameChanged
	}
	return id, state, nil
}

// addedPrefix returns how a game added to the group is named before " game N": as the load names one, else from
// its place in the catalogue.
func addedPrefix(ctx context.Context, tx pgx.Tx, group int64) (string, error) {
	var groupName, fallback string
	if err := tx.QueryRow(ctx, `
		SELECT gr.name, concat_ws(' ', t.name, s.label, st.label)
		FROM groups gr JOIN stages st USING (stage_id) JOIN tournaments s USING (tournament_id) JOIN series t USING (series_id)
		WHERE gr.group_id = $1`, group).Scan(&groupName, &fallback); err != nil {
		return "", err
	}
	prefix, ok, err := heldPrefix(ctx, tx, group, groupName, map[int64]*string{})
	if err != nil || ok {
		return prefix, err
	}
	if groupName != "" {
		fallback += " - " + groupName
	}
	return fallback, nil
}

// insertGame adds the entry as a played game, named name, in the group with the game number, its players members
// of the group, and returns its id.
func insertGame(ctx context.Context, tx pgx.Tx, g GameEntry, name string, group *int64, number *int) (int64, error) {
	code := strings.TrimSpace(g.Code)
	if code == "" || g.Date.IsZero() || len(g.Seats) < 2 {
		return 0, ErrBadGameEntry
	}
	players := make([]int64, len(g.Seats))
	for i, s := range g.Seats {
		err := tx.QueryRow(ctx, "SELECT player_id FROM players WHERE lower(name) = lower($1)", strings.TrimSpace(s.Player)).Scan(&players[i])
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrUnknownPlayer
		}
		if err != nil {
			return 0, err
		}
		for _, p := range players[:i] {
			if p == players[i] {
				return 0, ErrBadGameEntry
			}
		}
	}
	var batch, game int64
	if err := tx.QueryRow(ctx, "INSERT INTO import_batches (source) VALUES ('admin') RETURNING batch_id").Scan(&batch); err != nil {
		return 0, err
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO games (batch_id, source_file, source_line, name, date, share_code, state, group_id, game_number)
		VALUES ($1, 'admin', 0, $2, $3, $4, 'played', $5, $6) RETURNING game_id`,
		batch, name, g.Date, code, group, number).Scan(&game); err != nil {
		return 0, err
	}
	for i, s := range g.Seats {
		if _, err := tx.Exec(ctx, "INSERT INTO seats (game_id, seat, player_id, outcome, culture, place) VALUES ($1, $2, $3, $4, $5, $6)",
			game, i+1, players[i], s.Outcome, s.Score, s.Place); err != nil {
			return 0, err
		}
	}
	if group != nil {
		if _, err := tx.Exec(ctx, "INSERT INTO group_members (group_id, player_id) SELECT $1, unnest($2::bigint[]) ON CONFLICT DO NOTHING",
			*group, players); err != nil {
			return 0, err
		}
	}
	return game, nil
}

// insertChange records a change and returns its id.
func insertChange(ctx context.Context, tx pgx.Tx, action string, game *int64, earlier *string, added *int64, reason string) (int64, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || utf8.RuneCountInString(reason) > 1000 {
		return 0, ErrBadGameEntry
	}
	var id int64
	err := tx.QueryRow(ctx, `INSERT INTO game_changes (action, game_id, earlier_state, added_game_id, reason)
		VALUES ($1, $2, $3, $4, $5) RETURNING change_id`, action, game, earlier, added, reason).Scan(&id)
	return id, err
}

// addedInAdmin is SQL true when the game named by column (such as "g.game_id") was added by a change in admin.
func addedInAdmin(column string) string {
	return "EXISTS (SELECT FROM game_changes c WHERE c.added_game_id = " + column + ")"
}

// changedInAdmin is SQL true when a change in admin names the game in column: added, removed or replaced.
func changedInAdmin(column string) string {
	return "EXISTS (SELECT FROM game_changes c WHERE " + column + " IN (c.game_id, c.added_game_id))"
}
