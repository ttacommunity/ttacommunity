package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

// The fake's game changes: game 404 and group 404 do not exist, game 409 is already changed, and an entry coded
// HELD, BAD or seating Nobody is refused; undoing change 404 finds none, 409 was matched by an extract, 410 changed since.

func (f *fakeAdmins) GameChanges(context.Context) ([]store.GameChange, error) {
	return []store.GameChange{{ID: 7, Action: "replace", Reason: "Timed out", By: "An Organiser",
		At:    time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC),
		Game:  &store.ChangedGame{ID: 3, Name: "Bronze 1 game 1", Code: "AAAAAAAA", State: "replaced", Date: time.Date(2024, 9, 16, 0, 0, 0, 0, time.UTC)},
		Added: &store.ChangedGame{ID: 9, Name: "Bronze 1 game 1", Code: "NEWGAME1", State: "played", Date: time.Date(2024, 9, 20, 0, 0, 0, 0, time.UTC)},
	}}, nil
}

func (f *fakeAdmins) gameEntry(g store.GameEntry) error {
	switch {
	case g.Code == "HELD":
		return store.ErrCodeHeld
	case g.Code == "BAD":
		return store.ErrBadGameEntry
	case len(g.Seats) > 0 && g.Seats[0].Player == "Nobody":
		return store.ErrUnknownPlayer
	}
	return nil
}

func gameToChange(game int64) error {
	switch game {
	case 404:
		return store.ErrNoGame
	case 409:
		return store.ErrGameChanged
	}
	return nil
}

func (f *fakeAdmins) AddGame(_ context.Context, group int64, g store.GameEntry, reason string) (int64, error) {
	if group == 404 {
		return 0, store.ErrNoGroup
	}
	if err := f.gameEntry(g); err != nil {
		return 0, err
	}
	f.notes = append(f.notes, fmt.Sprintf("add %s %s #%d to %d: %+v (%s)", g.Code, g.Date.Format(time.DateOnly), g.Number, group, g.Seats, reason))
	return 11, nil
}

func (f *fakeAdmins) RemoveGame(_ context.Context, game int64, reason string) (int64, error) {
	if err := gameToChange(game); err != nil {
		return 0, err
	}
	f.notes = append(f.notes, fmt.Sprintf("remove %d (%s)", game, reason))
	return 12, nil
}

func (f *fakeAdmins) ReplaceGame(_ context.Context, game int64, g store.GameEntry, reason string) (int64, error) {
	if err := gameToChange(game); err != nil {
		return 0, err
	}
	if err := f.gameEntry(g); err != nil {
		return 0, err
	}
	f.notes = append(f.notes, fmt.Sprintf("replace %d by %s (%s)", game, g.Code, reason))
	return 13, nil
}

func (f *fakeAdmins) UndoGameChange(_ context.Context, id int64) error {
	switch id {
	case 404:
		return store.ErrNoGameChange
	case 409:
		return store.ErrAddedGameLoaded
	case 410:
		return store.ErrChangedSince
	}
	f.notes = append(f.notes, fmt.Sprintf("undo %d", id))
	return nil
}

func (f *fakeAdmins) ChangeableGames(_ context.Context, filter store.GameFilter) ([]store.ChangeableGame, error) {
	f.notes = append(f.notes, fmt.Sprintf("games %+v", filter))
	first := 1
	return []store.ChangeableGame{{ID: 3, Name: "Bronze 1 game 1", Date: time.Date(2024, 9, 16, 0, 0, 0, 0, time.UTC), State: "played",
		Group: 5, Number: &first, Seats: []store.GameSeatEntry{{Player: "Player One", Place: 1, Outcome: "finished", Score: 120}, {Player: "Player Two", Place: 2}}}}, nil
}

func (f *fakeAdmins) SeasonGroups(_ context.Context, slug, season string) ([]store.SeasonStage, error) {
	if slug != "intermezzo" || season != "24" {
		return nil, store.ErrNoTournament
	}
	return []store.SeasonStage{{Label: "", Groups: []store.SeasonGroup{{ID: 5, Name: "Bronze 1", Games: 2, NextNumber: 3}}}}, nil
}

func TestAdminGameChanges(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	posts := []struct{ body, id string }{
		{`{"action": "add", "group": 5, "reason": "Played outside the tournament", "entry": {"code": "NEWGAME1", "date": "2024-09-20", "number": 4,
			"seats": [{"player": "Player One", "place": 1, "outcome": "finished", "score": 120}, {"player": "Player Two", "place": 2, "outcome": "timed_out", "score": 0}]}}`, "11"},
		{`{"action": "remove", "game": 3, "reason": "Not a tournament game"}`, "12"},
		{`{"action": "replace", "game": 4, "reason": "Timed out", "entry": {"code": "NEWGAME2", "date": "2024-09-21",
			"seats": [{"player": "Player One", "place": 1, "outcome": "finished", "score": 90}, {"player": "Player Two", "place": 2, "outcome": "resigned", "score": 0}]}}`, "13"},
	}
	for _, p := range posts {
		rec := adminDo(t, f, http.MethodPost, "/v1/admin/game-changes", site, p.body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST %s = %d %s, want 201", p.body, rec.Code, rec.Body)
		}
		wantJSON(t, rec.Body.Bytes(), "id", p.id)
	}
	if rec := adminDo(t, f, http.MethodDelete, "/v1/admin/game-changes/12", site, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d %s, want 204", rec.Code, rec.Body)
	}
	want := "add NEWGAME1 2024-09-20 #4 to 5: [{Player:Player One Place:1 Outcome:finished Score:120} {Player:Player Two Place:2 Outcome:timed_out Score:0}] (Played outside the tournament)" +
		"|remove 3 (Not a tournament game)|replace 4 by NEWGAME2 (Timed out)|undo 12"
	if got := strings.Join(f.notes, "|"); got != want {
		t.Errorf("changes = %q, want %q", got, want)
	}
	if got, want := strings.Join(f.audits, "|"), org+" game.add 11|"+org+" game.remove 12|"+org+" game.replace 13|"+org+" game.undo 12"; got != want {
		t.Errorf("audits = %q, want %q", got, want)
	}

	rec := adminDo(t, f, http.MethodGet, "/v1/admin/game-changes", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET game changes = %d %s, want 200", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "0", `{"id": 7, "action": "replace", "by": "An Organiser", "reason": "Timed out",
		"game": {"code": "AAAAAAAA", "state": "replaced", "date": "2024-09-16"}, "added": {"id": 9, "code": "NEWGAME1"}}`)
}

func TestAdminFindsGamesAndGroupsToChange(t *testing.T) {
	f := &fakeAdmins{}
	rec := adminDo(t, f, http.MethodGet, "/v1/admin/games?code=AAAAAAAA", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET games by code = %d %s, want 200", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "0", `{"id": 3, "code": "", "group": 5, "number": 1,
		"seats": [{"player": "Player One", "place": 1, "outcome": "finished", "score": 120}, {"player": "Player Two", "outcome": null}]}`)
	if rec := adminDo(t, f, http.MethodGet, "/v1/admin/games?group=5", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("GET games by group = %d %s, want 200", rec.Code, rec.Body)
	}
	if got, want := strings.Join(f.notes, "|"), "games {Code:AAAAAAAA Group:0}|games {Code: Group:5}"; got != want {
		t.Errorf("asked = %q, want %q", got, want)
	}

	rec = adminDo(t, f, http.MethodGet, "/v1/admin/tournaments/intermezzo/seasons/24/groups", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET season groups = %d %s, want 200", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "0", `{"label": "", "groups": [{"id": 5, "name": "Bronze 1", "games": 2, "next_number": 3}]}`)
}

func TestAdminRefusesAGameChangeItCannotMake(t *testing.T) {
	const site = "https://ttac-admin.example"
	const entry = `"entry": {"code": "%s", "date": "2024-09-20", "seats": [{"player": "%s", "place": 1, "outcome": "finished", "score": 1}, {"player": "Player Two", "place": 2, "outcome": "finished", "score": 0}]}`
	good := fmt.Sprintf(entry, "NEWGAME1", "Player One")
	tests := []struct {
		name, method, path, origin, body string
		want                             int
	}{
		{"an unknown action", http.MethodPost, "/v1/admin/game-changes", site, `{"action": "swap", "game": 3, "reason": "A reason"}`, http.StatusBadRequest},
		{"add without a group", http.MethodPost, "/v1/admin/game-changes", site, `{"action": "add", "reason": "A reason", ` + good + `}`, http.StatusBadRequest},
		{"add without an entry", http.MethodPost, "/v1/admin/game-changes", site, `{"action": "add", "group": 5, "reason": "A reason"}`, http.StatusBadRequest},
		{"add naming a game", http.MethodPost, "/v1/admin/game-changes", site, `{"action": "add", "group": 5, "game": 3, "reason": "A reason", ` + good + `}`, http.StatusBadRequest},
		{"remove without a game", http.MethodPost, "/v1/admin/game-changes", site, `{"action": "remove", "reason": "A reason"}`, http.StatusBadRequest},
		{"remove with an entry", http.MethodPost, "/v1/admin/game-changes", site, `{"action": "remove", "game": 3, "reason": "A reason", ` + good + `}`, http.StatusBadRequest},
		{"replace without an entry", http.MethodPost, "/v1/admin/game-changes", site, `{"action": "replace", "game": 3, "reason": "A reason"}`, http.StatusBadRequest},
		{"replace naming a group", http.MethodPost, "/v1/admin/game-changes", site, `{"action": "replace", "game": 3, "group": 5, "reason": "A reason", ` + good + `}`, http.StatusBadRequest},
		{"a bad entry", http.MethodPost, "/v1/admin/game-changes", site, `{"action": "add", "group": 5, "reason": "A reason", ` + fmt.Sprintf(entry, "BAD", "Player One") + `}`, http.StatusBadRequest},
		{"an unknown player", http.MethodPost, "/v1/admin/game-changes", site, `{"action": "replace", "game": 3, "reason": "A reason", ` + fmt.Sprintf(entry, "NEWGAME1", "Nobody") + `}`, http.StatusBadRequest},
		{"no such group", http.MethodPost, "/v1/admin/game-changes", site, `{"action": "add", "group": 404, "reason": "A reason", ` + good + `}`, http.StatusNotFound},
		{"no such game", http.MethodPost, "/v1/admin/game-changes", site, `{"action": "remove", "game": 404, "reason": "A reason"}`, http.StatusNotFound},
		{"a game already changed", http.MethodPost, "/v1/admin/game-changes", site, `{"action": "replace", "game": 409, "reason": "A reason", ` + good + `}`, http.StatusConflict},
		{"a code we hold", http.MethodPost, "/v1/admin/game-changes", site, `{"action": "add", "group": 5, "reason": "A reason", ` + fmt.Sprintf(entry, "HELD", "Player One") + `}`, http.StatusConflict},
		{"a change from another site", http.MethodPost, "/v1/admin/game-changes", "https://evil.example", `{"action": "remove", "game": 3, "reason": "A reason"}`, http.StatusForbidden},
		{"undo an unknown change", http.MethodDelete, "/v1/admin/game-changes/404", site, "", http.StatusNotFound},
		{"undo an addition an extract matched", http.MethodDelete, "/v1/admin/game-changes/409", site, "", http.StatusConflict},
		{"undo an addition changed since", http.MethodDelete, "/v1/admin/game-changes/410", site, "", http.StatusConflict},
		{"undo from another site", http.MethodDelete, "/v1/admin/game-changes/12", "https://evil.example", "", http.StatusForbidden},
		{"games by neither code nor group", http.MethodGet, "/v1/admin/games", "", "", http.StatusBadRequest},
		{"games by both code and group", http.MethodGet, "/v1/admin/games?code=AAAAAAAA&group=5", "", "", http.StatusBadRequest},
		{"groups of an unknown season", http.MethodGet, "/v1/admin/tournaments/intermezzo/seasons/99/groups", "", "", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			if rec := adminDo(t, f, tt.method, tt.path, tt.origin, tt.body); rec.Code != tt.want {
				t.Errorf("%s %s %s = %d %s, want %d", tt.method, tt.path, tt.body, rec.Code, rec.Body, tt.want)
			}
			if len(f.audits) != 0 || len(f.notes) > 0 && tt.method != http.MethodGet {
				t.Errorf("audits = %q, changes = %q; want none", f.audits, f.notes)
			}
		})
	}
}
