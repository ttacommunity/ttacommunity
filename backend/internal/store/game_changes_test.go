//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// changeFixture is a group of two played games, coded AAAAAAAA and BBBBBBBB, and the players a new game seats.
func changeFixture(t *testing.T) (*pgxpool.Pool, *AdminStore) {
	t.Helper()
	pool := migrated(t)
	placedOn(t, pool, map[string]time.Time{
		"Intermezzo S24 - Bronze 1 game 1": day("2024-09-16"),
		"Intermezzo S24 - Bronze 1 game 2": day("2024-09-17"),
	})
	for name, code := range map[string]string{"Intermezzo S24 - Bronze 1 game 1": "AAAAAAAA", "Intermezzo S24 - Bronze 1 game 2": "BBBBBBBB"} {
		if _, err := pool.Exec(context.Background(), "UPDATE games SET share_code = $2 WHERE name = $1", name, code); err != nil {
			t.Fatalf("coding %s: %v", name, err)
		}
	}
	ensurePlayers(t, pool, "Player One", "Player Two", "Player Three")
	return pool, NewAdminStore(asAdminLogin(t, pool))
}

// newEntry is a two-player game coded code, which Player One wins.
func newEntry(code string) GameEntry {
	return GameEntry{Code: code, Date: day("2024-09-20"), Seats: []GameSeatEntry{
		{Player: "Player One", Place: 1, Outcome: "finished", Score: 120},
		{Player: "Player Two", Place: 2, Outcome: "timed_out", Score: 0},
	}}
}

// gameByCode returns the state, name, group and game number of the game coded code.
func gameByCode(t *testing.T, pool *pgxpool.Pool, code string) (state, name string, group *int64, number *int) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), "SELECT state, name, group_id, game_number FROM games WHERE share_code = $1", code).
		Scan(&state, &name, &group, &number); err != nil {
		t.Fatalf("reading game %s: %v", code, err)
	}
	return state, name, group, number
}

func gameCount(t *testing.T, pool *pgxpool.Pool, code string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM games WHERE share_code = $1", code).Scan(&n); err != nil {
		t.Fatalf("counting games coded %s: %v", code, err)
	}
	return n
}

func TestReplaceGameKeepsTheOriginalAndPutsTheNewGameInItsPlace(t *testing.T) {
	ctx := context.Background()
	pool, a := changeFixture(t)
	_, origName, origGroup, origNumber := gameByCode(t, pool, "AAAAAAAA")

	id, err := a.ReplaceGame(ctx, "AAAAAAAA", newEntry("NEWGAME001"), "The players timed out")
	if err != nil {
		t.Fatalf("ReplaceGame: %v", err)
	}
	if state, _, _, _ := gameByCode(t, pool, "AAAAAAAA"); state != "replaced" {
		t.Errorf("original's state = %q, want replaced", state)
	}
	state, name, group, number := gameByCode(t, pool, "NEWGAME001")
	if state != "played" || name != origName || *group != *origGroup || *number != *origNumber {
		t.Errorf("new game = %s %q group %d number %d, want played %q group %d number %d",
			state, name, *group, *number, origName, *origGroup, *origNumber)
	}
	changes, err := a.GameChanges(ctx)
	if err != nil || len(changes) != 1 {
		t.Fatalf("GameChanges = %+v, %v; want the one change", changes, err)
	}
	c := changes[0]
	if c.ID != id || c.Action != "replace" || c.Game.Code != "AAAAAAAA" || c.Added.Code != "NEWGAME001" || c.Reason != "The players timed out" {
		t.Errorf("change = %+v, want replace AAAAAAAA by NEWGAME001 with its reason", c)
	}

	if err := a.UndoGameChange(ctx, id); err != nil {
		t.Fatalf("UndoGameChange: %v", err)
	}
	if state, _, _, _ := gameByCode(t, pool, "AAAAAAAA"); state != "played" {
		t.Errorf("original's state once undone = %q, want played", state)
	}
	if n := gameCount(t, pool, "NEWGAME001"); n != 0 {
		t.Errorf("new games once undone = %d, want 0", n)
	}
}

func TestAddGameNamesAndNumbersItAsItsGroupsNextGame(t *testing.T) {
	ctx := context.Background()
	pool, a := changeFixture(t)
	_, _, group, _ := gameByCode(t, pool, "AAAAAAAA")

	id, err := a.AddGame(ctx, *group, newEntry("NEWGAME001"), "Played outside the tournament")
	if err != nil {
		t.Fatalf("AddGame: %v", err)
	}
	if state, name, _, number := gameByCode(t, pool, "NEWGAME001"); state != "played" || name != "Intermezzo S24 - Bronze 1 game 3" || *number != 3 {
		t.Errorf("added game = %s %q number %d, want played \"Intermezzo S24 - Bronze 1 game 3\" number 3", state, name, *number)
	}
	if err := a.UndoGameChange(ctx, id); err != nil {
		t.Fatalf("UndoGameChange: %v", err)
	}
	if n := gameCount(t, pool, "NEWGAME001"); n != 0 {
		t.Errorf("added games once undone = %d, want 0", n)
	}
}

func TestRemoveGameKeepsItAndUndoGivesBackItsEarlierState(t *testing.T) {
	ctx := context.Background()
	pool, a := changeFixture(t)
	setGameState(t, pool, "Intermezzo S24 - Bronze 1 game 2", "never_played")

	id, err := a.RemoveGame(ctx, "BBBBBBBB", "Not a tournament game")
	if err != nil {
		t.Fatalf("RemoveGame: %v", err)
	}
	if state, _, _, _ := gameByCode(t, pool, "BBBBBBBB"); state != "removed" {
		t.Errorf("state = %q, want removed", state)
	}
	if err := a.UndoGameChange(ctx, id); err != nil {
		t.Fatalf("UndoGameChange: %v", err)
	}
	if state, _, _, _ := gameByCode(t, pool, "BBBBBBBB"); state != "never_played" {
		t.Errorf("state once undone = %q, want never_played", state)
	}
}

func TestGameChangesRefuseBadInput(t *testing.T) {
	ctx := context.Background()
	with := func(change func(*GameEntry)) GameEntry {
		e := newEntry("NEWGAME001")
		change(&e)
		return e
	}
	tests := []struct {
		name  string
		code  string
		entry GameEntry
		want  error
	}{
		{"an unknown original", "ZZZZZZZZ", newEntry("NEWGAME001"), ErrNoGame},
		{"a code we hold", "AAAAAAAA", newEntry("BBBBBBBB"), ErrCodeHeld},
		{"no code", "AAAAAAAA", newEntry(" "), ErrBadGameEntry},
		{"no date", "AAAAAAAA", with(func(e *GameEntry) { e.Date = time.Time{} }), ErrBadGameEntry},
		{"one seat", "AAAAAAAA", with(func(e *GameEntry) { e.Seats = e.Seats[:1] }), ErrBadGameEntry},
		{"a player twice", "AAAAAAAA", with(func(e *GameEntry) { e.Seats[1].Player = "player one" }), ErrBadGameEntry},
		{"an unknown player", "AAAAAAAA", with(func(e *GameEntry) { e.Seats[1].Player = "Nobody Here" }), ErrUnknownPlayer},
		{"a fifth place", "AAAAAAAA", with(func(e *GameEntry) { e.Seats[1].Place = 5 }), ErrBadGameEntry},
		{"an unknown outcome", "AAAAAAAA", with(func(e *GameEntry) { e.Seats[1].Outcome = "left" }), ErrBadGameEntry},
		{"culture after a timeout", "AAAAAAAA", with(func(e *GameEntry) { e.Seats[1].Score = 40 }), ErrBadGameEntry},
		{"negative culture", "AAAAAAAA", with(func(e *GameEntry) { e.Seats[0].Score = -1 }), ErrBadGameEntry},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, a := changeFixture(t)
			if _, err := a.ReplaceGame(ctx, tt.code, tt.entry, "A reason"); !errors.Is(err, tt.want) {
				t.Errorf("ReplaceGame(%s) = %v, want %v", tt.code, err, tt.want)
			}
			if state, _, _, _ := gameByCode(t, pool, "AAAAAAAA"); state != "played" {
				t.Errorf("original's state after a refusal = %q, want played", state)
			}
		})
	}
}

func TestGameChangesRefuseWhatCannotBeDone(t *testing.T) {
	ctx := context.Background()
	pool, a := changeFixture(t)
	_, _, group, _ := gameByCode(t, pool, "AAAAAAAA")

	if _, err := a.RemoveGame(ctx, "AAAAAAAA", " "); !errors.Is(err, ErrBadGameEntry) {
		t.Errorf("RemoveGame without a reason = %v, want %v", err, ErrBadGameEntry)
	}
	if _, err := a.AddGame(ctx, 999999, newEntry("NEWGAME001"), "A reason"); !errors.Is(err, ErrNoGroup) {
		t.Errorf("AddGame to an unknown group = %v, want %v", err, ErrNoGroup)
	}
	numbered := newEntry("NEWGAME001")
	numbered.Number = 2
	if _, err := a.AddGame(ctx, *group, numbered, "A reason"); !errors.Is(err, ErrNumberHeld) {
		t.Errorf("AddGame with game 2's number = %v, want %v", err, ErrNumberHeld)
	}
	removed, err := a.RemoveGame(ctx, "BBBBBBBB", "Not a tournament game")
	if err != nil {
		t.Fatalf("RemoveGame: %v", err)
	}
	if _, err := a.RemoveGame(ctx, "BBBBBBBB", "Again"); !errors.Is(err, ErrGameChanged) {
		t.Errorf("RemoveGame of a removed game = %v, want %v", err, ErrGameChanged)
	}
	if _, err := a.ReplaceGame(ctx, "BBBBBBBB", newEntry("NEWGAME002"), "Again"); !errors.Is(err, ErrGameChanged) {
		t.Errorf("ReplaceGame of a removed game = %v, want %v", err, ErrGameChanged)
	}
	if _, err := a.AddGame(ctx, *group, numbered, "Game 2 again"); err != nil {
		t.Fatalf("AddGame with a removed game's number: %v", err)
	}
	if err := a.UndoGameChange(ctx, removed); !errors.Is(err, ErrNumberHeld) {
		t.Errorf("UndoGameChange of a removal whose number is used again = %v, want %v", err, ErrNumberHeld)
	}

	replaced, err := a.ReplaceGame(ctx, "AAAAAAAA", newEntry("NEWGAME003"), "Timed out")
	if err != nil {
		t.Fatalf("ReplaceGame: %v", err)
	}
	if _, err := a.RemoveGame(ctx, "NEWGAME003", "Timed out too"); err != nil {
		t.Fatalf("RemoveGame of the replacement: %v", err)
	}
	if err := a.UndoGameChange(ctx, replaced); !errors.Is(err, ErrChangedSince) {
		t.Errorf("UndoGameChange of a replacement since removed = %v, want %v", err, ErrChangedSince)
	}
	if err := a.UndoGameChange(ctx, 999999); !errors.Is(err, ErrNoGameChange) {
		t.Errorf("UndoGameChange of an unknown change = %v, want %v", err, ErrNoGameChange)
	}
}

func TestChangedGamesAreNotCountedAsNeverPlayed(t *testing.T) {
	ctx := context.Background()
	_, a := changeFixture(t)
	if _, err := a.ReplaceGame(ctx, "AAAAAAAA", newEntry("NEWGAME001"), "Timed out"); err != nil {
		t.Fatalf("ReplaceGame: %v", err)
	}
	if _, err := a.RemoveGame(ctx, "BBBBBBBB", "Not a tournament game"); err != nil {
		t.Fatalf("RemoveGame: %v", err)
	}
	d, err := a.DataSummary(ctx)
	if err != nil {
		t.Fatalf("DataSummary: %v", err)
	}
	if d.Games != 1 || d.NeverPlayed != 0 {
		t.Errorf("summary = %d played, %d never played; want 1 played (the replacement), 0 never played", d.Games, d.NeverPlayed)
	}
}

func TestMigration107IsRefusedGoingDownWhileAGameIsChanged(t *testing.T) {
	ctx := context.Background()
	pool, a := changeFixture(t)
	id, err := a.RemoveGame(ctx, "BBBBBBBB", "Not a tournament game")
	if err != nil {
		t.Fatalf("RemoveGame: %v", err)
	}
	if err := withRatingsMigrations(pool, func(p *goose.Provider) error { _, err := p.DownTo(ctx, 106); return err }); err == nil {
		t.Fatal("migrating down past 107 with a removed game: no error, want it refused")
	}
	if err := a.UndoGameChange(ctx, id); err != nil {
		t.Fatalf("UndoGameChange: %v", err)
	}
	migrateTo(t, pool, 106, true)
	migrateTo(t, pool, 107, false)
}
