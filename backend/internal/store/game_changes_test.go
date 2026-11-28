//go:build integration

package store

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	"ttacommunity/backend/internal/cgeextract"
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

// idOf returns the id of the game coded code.
func idOf(t *testing.T, pool *pgxpool.Pool, code string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), "SELECT game_id FROM games WHERE share_code = $1", code).Scan(&id); err != nil {
		t.Fatalf("finding game %s: %v", code, err)
	}
	return id
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

	id, err := a.ReplaceGame(ctx, idOf(t, pool, "AAAAAAAA"), newEntry("NEWGAME001"), "The players timed out")
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

	id, err := a.RemoveGame(ctx, idOf(t, pool, "BBBBBBBB"), "Not a tournament game")
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
		entry GameEntry
		want  error
	}{
		{"a code we hold", newEntry("BBBBBBBB"), ErrCodeHeld},
		{"no code", newEntry(" "), ErrBadGameEntry},
		{"no date", with(func(e *GameEntry) { e.Date = time.Time{} }), ErrBadGameEntry},
		{"one seat", with(func(e *GameEntry) { e.Seats = e.Seats[:1] }), ErrBadGameEntry},
		{"a player twice", with(func(e *GameEntry) { e.Seats[1].Player = "player one" }), ErrBadGameEntry},
		{"an unknown player", with(func(e *GameEntry) { e.Seats[1].Player = "Nobody Here" }), ErrUnknownPlayer},
		{"a fifth place", with(func(e *GameEntry) { e.Seats[1].Place = 5 }), ErrBadGameEntry},
		{"an unknown outcome", with(func(e *GameEntry) { e.Seats[1].Outcome = "left" }), ErrBadGameEntry},
		{"culture after a timeout", with(func(e *GameEntry) { e.Seats[1].Score = 40 }), ErrBadGameEntry},
		{"negative culture", with(func(e *GameEntry) { e.Seats[0].Score = -1 }), ErrBadGameEntry},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, a := changeFixture(t)
			if _, err := a.ReplaceGame(ctx, idOf(t, pool, "AAAAAAAA"), tt.entry, "A reason"); !errors.Is(err, tt.want) {
				t.Errorf("ReplaceGame(%+v) = %v, want %v", tt.entry, err, tt.want)
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

	if _, err := a.RemoveGame(ctx, idOf(t, pool, "AAAAAAAA"), " "); !errors.Is(err, ErrBadGameEntry) {
		t.Errorf("RemoveGame without a reason = %v, want %v", err, ErrBadGameEntry)
	}
	if _, err := a.RemoveGame(ctx, 999999, "A reason"); !errors.Is(err, ErrNoGame) {
		t.Errorf("RemoveGame of an unknown game = %v, want %v", err, ErrNoGame)
	}
	if _, err := a.AddGame(ctx, 999999, newEntry("NEWGAME001"), "A reason"); !errors.Is(err, ErrNoGroup) {
		t.Errorf("AddGame to an unknown group = %v, want %v", err, ErrNoGroup)
	}
	numbered := newEntry("NEWGAME001")
	numbered.Number = 2
	if _, err := a.AddGame(ctx, *group, numbered, "A reason"); !errors.Is(err, ErrNumberHeld) {
		t.Errorf("AddGame with game 2's number = %v, want %v", err, ErrNumberHeld)
	}
	removed, err := a.RemoveGame(ctx, idOf(t, pool, "BBBBBBBB"), "Not a tournament game")
	if err != nil {
		t.Fatalf("RemoveGame: %v", err)
	}
	if _, err := a.RemoveGame(ctx, idOf(t, pool, "BBBBBBBB"), "Again"); !errors.Is(err, ErrGameChanged) {
		t.Errorf("RemoveGame of a removed game = %v, want %v", err, ErrGameChanged)
	}
	if _, err := a.ReplaceGame(ctx, idOf(t, pool, "BBBBBBBB"), newEntry("NEWGAME002"), "Again"); !errors.Is(err, ErrGameChanged) {
		t.Errorf("ReplaceGame of a removed game = %v, want %v", err, ErrGameChanged)
	}
	if _, err := a.AddGame(ctx, *group, numbered, "Game 2 again"); err != nil {
		t.Fatalf("AddGame with a removed game's number: %v", err)
	}
	if err := a.UndoGameChange(ctx, removed); !errors.Is(err, ErrNumberHeld) {
		t.Errorf("UndoGameChange of a removal whose number is used again = %v, want %v", err, ErrNumberHeld)
	}

	replaced, err := a.ReplaceGame(ctx, idOf(t, pool, "AAAAAAAA"), newEntry("NEWGAME003"), "Timed out")
	if err != nil {
		t.Fatalf("ReplaceGame: %v", err)
	}
	if _, err := a.RemoveGame(ctx, idOf(t, pool, "NEWGAME003"), "Timed out too"); err != nil {
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
	pool, a := changeFixture(t)
	if _, err := a.ReplaceGame(ctx, idOf(t, pool, "AAAAAAAA"), newEntry("NEWGAME001"), "Timed out"); err != nil {
		t.Fatalf("ReplaceGame: %v", err)
	}
	if _, err := a.RemoveGame(ctx, idOf(t, pool, "BBBBBBBB"), "Not a tournament game"); err != nil {
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

func TestApplyCGEExtractKeepsWhatAdminChanged(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	placedOn(t, pool, map[string]time.Time{"Intermezzo S24 - Bronze 1 game 1": day("2024-01-02"), "Intermezzo S24 - Bronze 1 game 2": day("2024-01-09")})
	setSeats(t, pool, "%game 1", Seat{"ann", 200}, Seat{"bob", 150})
	setSeats(t, pool, "%game 2", Seat{"cy", 120}, Seat{"dee", 90})
	setShareCode(t, pool, "Intermezzo S24 - Bronze 1 game 1", "CODEAAAA")
	setShareCode(t, pool, "Intermezzo S24 - Bronze 1 game 2", "CODEBBBB")
	a := NewAdminStore(asAdminLogin(t, pool))
	entry := GameEntry{Code: "CODENEWW", Date: day("2024-01-05"), Seats: []GameSeatEntry{
		{Player: "ann", Place: 1, Outcome: "finished", Score: 180}, {Player: "bob", Place: 2, Outcome: "finished", Score: 170}}}
	replaced, err := a.ReplaceGame(ctx, idOf(t, pool, "CODEAAAA"), entry, "Timed out")
	if err != nil {
		t.Fatalf("ReplaceGame: %v", err)
	}
	if _, err := a.RemoveGame(ctx, idOf(t, pool, "CODEBBBB"), "Not a tournament game"); err != nil {
		t.Fatalf("RemoveGame: %v", err)
	}

	cancelled := extractOf(19, 100, "CODEAAAA", cgeGone(1, "ann"), cgeGone(2, "bob"))
	for i := range cancelled {
		cancelled[i].State = "cancelled"
	}
	other := extractOf(19, 102, "CODENEWW", cgeDone(1, "ann", 1, 180), cgeDone(2, "bob", 2, 170))
	for i := range other {
		other[i].GroupID, other[i].GroupName = 72, "Bronze 3"
	}
	kept := extractOf(19, 101, "CODEBBBB", cgeDone(3, "cy", 1, 120), cgeDone(4, "dee", 2, 90))
	tournaments := map[int]cgeextract.Tournament{19: {ID: 19, Name: "Intermezzo Championship"}}
	for _, x := range []cgeextract.Extract{
		{Seats: slices.Concat(cancelled, other, kept), Tournaments: tournaments},
		{Seats: kept, Removed: slices.Concat(cancelled, other), Tournaments: tournaments},
	} {
		plan, err := PlanCGEExtract(ctx, pool, x, nil)
		if err != nil {
			t.Fatalf("PlanCGEExtract: %v", err)
		}
		if _, err := ApplyCGEExtract(ctx, pool, plan, CGEInput{Extract: x}); err != nil {
			t.Fatalf("ApplyCGEExtract: %v", err)
		}
	}

	got := rowsOf(t, pool, `SELECT g.share_code || ' ' || g.state || ' ' || coalesce(gr.name, '-') || ' ' || coalesce(gr.cge_group::text, '-')
		FROM games g LEFT JOIN groups gr USING (group_id) ORDER BY 1`)
	want := []string{"CODEAAAA replaced Bronze 1 70", "CODEBBBB removed Bronze 1 70", "CODENEWW played Bronze 1 70"}
	if !slices.Equal(got, want) {
		t.Errorf("games = %q, want %q: each change kept, the replacement in its original's group, nothing CGE removed deleted", got, want)
	}
	if err := a.UndoGameChange(ctx, replaced); !errors.Is(err, ErrAddedGameLoaded) {
		t.Errorf("UndoGameChange of a replacement an extract has matched = %v, want %v", err, ErrAddedGameLoaded)
	}
}

func TestMigration107IsRefusedGoingDownWhileAGameIsChanged(t *testing.T) {
	ctx := context.Background()
	pool, a := changeFixture(t)
	id, err := a.RemoveGame(ctx, idOf(t, pool, "BBBBBBBB"), "Not a tournament game")
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

func TestGameChangesNameWhoMadeThem(t *testing.T) {
	ctx := context.Background()
	pool, a := changeFixture(t)
	if _, err := pool.Exec(ctx, `INSERT INTO admins (discord_id, name, role) VALUES ('100000000000000001', 'An Organiser', 'organiser')`); err != nil {
		t.Fatalf("adding an admin: %v", err)
	}
	removed, err := a.RemoveGame(ctx, idOf(t, pool, "BBBBBBBB"), "Not a tournament game")
	if err != nil {
		t.Fatalf("RemoveGame: %v", err)
	}
	if err := a.Audit(ctx, "100000000000000001", "game.remove", strconv.FormatInt(removed, 10), map[string]any{}); err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if _, err := a.RemoveGame(ctx, idOf(t, pool, "AAAAAAAA"), "Made by the load job"); err != nil {
		t.Fatalf("RemoveGame: %v", err)
	}
	changes, err := a.GameChanges(ctx)
	if err != nil {
		t.Fatalf("GameChanges: %v", err)
	}
	got := map[string]string{}
	for _, c := range changes {
		got[c.Game.Code] = c.By
	}
	if got["BBBBBBBB"] != "An Organiser" || got["AAAAAAAA"] != "" {
		t.Errorf("made by = %q, want BBBBBBBB by An Organiser and AAAAAAAA by no one", got)
	}
}

func TestGameByCodeFindsTheGameWithItsSeats(t *testing.T) {
	ctx := context.Background()
	pool, a := changeFixture(t)
	setSeats(t, pool, "%Bronze 1 game 1", Seat{"Player One", 200}, Seat{"Player Two", 150})
	_, _, group, _ := gameByCode(t, pool, "AAAAAAAA")
	if _, err := a.RemoveGame(ctx, idOf(t, pool, "BBBBBBBB"), "Not a tournament game"); err != nil {
		t.Fatalf("RemoveGame: %v", err)
	}

	g, err := a.GameByCode(ctx, " AAAAAAAA ")
	if err != nil {
		t.Fatalf("GameByCode: %v", err)
	}
	if g.ID != idOf(t, pool, "AAAAAAAA") || g.Group != *group || g.Number == nil || *g.Number != 1 || g.State != "played" {
		t.Errorf("game = %+v, want game 1 of group %d, played", g, *group)
	}
	if len(g.Seats) != 2 || g.Seats[0].Player != "Player One" || g.Seats[0].Place != 1 || g.Seats[1].Player != "Player Two" {
		t.Errorf("seats = %+v, want Player One first, then Player Two", g.Seats)
	}
	if typed, err := a.GameByCode(ctx, "aaaaaaaa"); err != nil || typed.ID != g.ID {
		t.Errorf("GameByCode in lower case = %+v, %v; want game %d", typed, err, g.ID)
	}
	if removed, err := a.GameByCode(ctx, "BBBBBBBB"); err != nil || removed.State != "removed" {
		t.Errorf("GameByCode of a removed game = %+v, %v; want it, removed", removed, err)
	}
	if _, err := a.GameByCode(ctx, "ZZZZZZZZ"); !errors.Is(err, ErrNoGame) {
		t.Errorf("GameByCode of an unknown code = %v, want %v", err, ErrNoGame)
	}
}

func TestSeasonGroupsGiveEachGroupItsNextNumber(t *testing.T) {
	ctx := context.Background()
	pool, a := changeFixture(t)
	var slug, season, stage, name string
	_, _, group, _ := gameByCode(t, pool, "AAAAAAAA")
	if err := pool.QueryRow(ctx, `
		SELECT t.slug, se.label, st.label, gr.name
		FROM groups gr JOIN stages st USING (stage_id) JOIN tournaments se USING (tournament_id) JOIN series t USING (series_id)
		WHERE gr.group_id = $1`, *group).Scan(&slug, &season, &stage, &name); err != nil {
		t.Fatalf("finding the group: %v", err)
	}

	stages, err := a.SeasonGroups(ctx, slug, season)
	if err != nil {
		t.Fatalf("SeasonGroups(%s, %s): %v", slug, season, err)
	}
	var found *SeasonGroup
	for _, st := range stages {
		for i, g := range st.Groups {
			if g.ID == *group {
				if st.Label != stage {
					t.Errorf("group's stage = %q, want %q", st.Label, stage)
				}
				found = &st.Groups[i]
			}
		}
	}
	if found == nil || found.Name != name || found.Games != 2 || found.NextNumber != 3 {
		t.Errorf("group = %+v, want %q with 2 games, next number 3", found, name)
	}

	if _, err := a.SeasonGroups(ctx, slug, "no such season"); !errors.Is(err, ErrNoTournament) {
		t.Errorf("SeasonGroups of an unknown season = %v, want %v", err, ErrNoTournament)
	}
}

func TestEditGameChangeRemakesItAsTheSameChange(t *testing.T) {
	ctx := context.Background()
	pool, a := changeFixture(t)
	gameA, gameB := idOf(t, pool, "AAAAAAAA"), idOf(t, pool, "BBBBBBBB")
	_, _, group, _ := gameByCode(t, pool, "AAAAAAAA")
	only := func(want string) {
		t.Helper()
		changes, err := a.GameChanges(ctx)
		if err != nil || len(changes) != 1 {
			t.Fatalf("GameChanges = %+v, %v; want one change", changes, err)
		}
		c := changes[0]
		got := c.Action + " " + c.Reason
		if c.Game != nil {
			got += " game " + c.Game.Code + " " + c.Game.State
		}
		if c.Added != nil {
			got += " added " + c.Added.Code
		}
		if got != want {
			t.Errorf("change = %q, want %q", got, want)
		}
	}
	id, err := a.ReplaceGame(ctx, gameA, newEntry("NEWGAME001"), "Timed out")
	if err != nil {
		t.Fatalf("ReplaceGame: %v", err)
	}

	steps := []struct {
		name string
		edit NewGameChange
		want string
	}{
		{"a new replacement", NewGameChange{Action: "replace", Game: gameA, Entry: newEntry("NEWGAME002"), Reason: "Timed out, replayed"},
			"replace Timed out, replayed game AAAAAAAA replaced added NEWGAME002"},
		{"a removal of another game", NewGameChange{Action: "remove", Game: gameB, Reason: "Not a tournament game"},
			"remove Not a tournament game game BBBBBBBB removed"},
		{"an addition", NewGameChange{Action: "add", Group: *group, Entry: newEntry("NEWGAME003"), Reason: "Missing"},
			"add Missing added NEWGAME003"},
	}
	for _, s := range steps {
		if err := a.EditGameChange(ctx, id, s.edit); err != nil {
			t.Fatalf("EditGameChange to %s: %v", s.name, err)
		}
		only(s.want)
		if changes, _ := a.GameChanges(ctx); changes[0].ID != id {
			t.Errorf("edited to %s, id = %d, want %d", s.name, changes[0].ID, id)
		}
	}
	for code, want := range map[string]string{"AAAAAAAA": "played", "BBBBBBBB": "played"} {
		if state, _, _, _ := gameByCode(t, pool, code); state != want {
			t.Errorf("%s once the change is an addition = %q, want %q", code, state, want)
		}
	}
	for _, code := range []string{"NEWGAME001", "NEWGAME002"} {
		if n := gameCount(t, pool, code); n != 0 {
			t.Errorf("games coded %s once edited away = %d, want 0", code, n)
		}
	}
	if state, name, _, number := gameByCode(t, pool, "NEWGAME003"); state != "played" || name != "Intermezzo S24 - Bronze 1 game 3" || *number != 3 {
		t.Errorf("added game = %s %q number %d, want played \"Intermezzo S24 - Bronze 1 game 3\" number 3", state, name, *number)
	}

	if err := a.EditGameChange(ctx, id, NewGameChange{Action: "replace", Game: gameA, Entry: newEntry("BBBBBBBB"), Reason: "Again"}); !errors.Is(err, ErrCodeHeld) {
		t.Errorf("EditGameChange to a code we hold = %v, want %v", err, ErrCodeHeld)
	}
	only("add Missing added NEWGAME003")
	if err := a.EditGameChange(ctx, 999999, NewGameChange{Action: "remove", Game: gameB, Reason: "A reason"}); !errors.Is(err, ErrNoGameChange) {
		t.Errorf("EditGameChange of an unknown change = %v, want %v", err, ErrNoGameChange)
	}
	if _, err := a.RemoveGame(ctx, idOf(t, pool, "NEWGAME003"), "Not played after all"); err != nil {
		t.Fatalf("RemoveGame of the added game: %v", err)
	}
	if err := a.EditGameChange(ctx, id, NewGameChange{Action: "remove", Game: gameB, Reason: "A reason"}); !errors.Is(err, ErrChangedSince) {
		t.Errorf("EditGameChange of an addition removed since = %v, want %v", err, ErrChangedSince)
	}
}
