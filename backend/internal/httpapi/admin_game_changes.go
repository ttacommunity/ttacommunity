// Admin's game changes: adding a game to a group, removing or replacing a game, editing or undoing a change, and finding the
// games and groups a change names.
package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"ttacommunity/backend/internal/store"
)

// GameChangeAdmin is the games changed in admin: listed, made (returning the change's id), edited and undone, and the game
// and groups a change can name.
type GameChangeAdmin interface {
	GameChanges(ctx context.Context) ([]store.GameChange, error)
	AddGame(ctx context.Context, group int64, g store.GameEntry, reason string) (int64, error)
	RemoveGame(ctx context.Context, game int64, reason string) (int64, error)
	ReplaceGame(ctx context.Context, game int64, g store.GameEntry, reason string) (int64, error)
	UndoGameChange(ctx context.Context, id int64) error
	EditGameChange(ctx context.Context, id int64, c store.NewGameChange) error
	GameByCode(ctx context.Context, code string) (store.ChangeableGame, error)
	SeasonGroups(ctx context.Context, slug, season string) ([]store.SeasonStage, error)
}

func (s *server) GetAdminGameChanges(ctx context.Context, _ GetAdminGameChangesRequestObject) (GetAdminGameChangesResponseObject, error) {
	list, err := s.admins.GameChanges(ctx)
	if err != nil {
		return nil, err
	}
	changed := func(g *store.ChangedGame) *ChangedGame {
		if g == nil {
			return nil
		}
		return &ChangedGame{Id: g.ID, Name: g.Name, Code: g.Code, State: GameState(g.State), Date: openapi_types.Date{Time: g.Date}}
	}
	out := make([]GameChange, len(list))
	for i, c := range list {
		out[i] = GameChange{Id: c.ID, Action: GameChangeAction(c.Action), Game: changed(c.Game), Added: changed(c.Added),
			Reason: c.Reason, At: c.At, By: c.By}
	}
	return GetAdminGameChanges200JSONResponse{Body: out, Headers: GetAdminGameChanges200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PostAdminGameChange(ctx context.Context, req PostAdminGameChangeRequestObject) (PostAdminGameChangeResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	b := req.Body
	if b == nil || !gameChangeComplete(*b) {
		return PostAdminGameChange400JSONResponse{badRequest400("add takes a group and an entry, remove a game, replace a game and an entry")}, nil
	}
	var id int64
	err := s.admins.Change(ctx, func(tx Admins) error {
		var err error
		c := newGameChange(*b)
		switch b.Action {
		case GameChangeAdd:
			id, err = tx.AddGame(ctx, c.Group, c.Entry, c.Reason)
		case GameChangeRemove:
			id, err = tx.RemoveGame(ctx, c.Game, c.Reason)
		case GameChangeReplace:
			id, err = tx.ReplaceGame(ctx, c.Game, c.Entry, c.Reason)
		}
		if err != nil {
			return err
		}
		return tx.Audit(ctx, a.DiscordID, "game."+string(b.Action), strconv.FormatInt(id, 10), b)
	})
	switch changeRefusal(err) {
	case http.StatusBadRequest:
		return PostAdminGameChange400JSONResponse{badRequest400(err.Error())}, nil
	case http.StatusNotFound:
		return PostAdminGameChange404JSONResponse{notFound404(err.Error())}, nil
	case http.StatusConflict:
		return PostAdminGameChange409JSONResponse{conflict409(err.Error())}, nil
	}
	if err != nil {
		return nil, err
	}
	return PostAdminGameChange201JSONResponse{Body: CreatedGameChange{Id: id},
		Headers: PostAdminGameChange201ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PutAdminGameChange(ctx context.Context, req PutAdminGameChangeRequestObject) (PutAdminGameChangeResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	b := req.Body
	if b == nil || !gameChangeComplete(*b) {
		return PutAdminGameChange400JSONResponse{badRequest400("add takes a group and an entry, remove a game, replace a game and an entry")}, nil
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.EditGameChange(ctx, req.Change, newGameChange(*b)); err != nil {
			return err
		}
		return tx.Audit(ctx, a.DiscordID, "game.edit", strconv.FormatInt(req.Change, 10), b)
	})
	switch changeRefusal(err) {
	case http.StatusBadRequest:
		return PutAdminGameChange400JSONResponse{badRequest400(err.Error())}, nil
	case http.StatusNotFound:
		return PutAdminGameChange404JSONResponse{notFound404(err.Error())}, nil
	case http.StatusConflict:
		return PutAdminGameChange409JSONResponse{conflict409(err.Error())}, nil
	}
	if err != nil {
		return nil, err
	}
	return PutAdminGameChange204Response{}, nil
}

// changeRefusal is the status a refused game change answers with, or 0 for an error that is no refusal (or none).
func changeRefusal(err error) int {
	switch {
	case errors.Is(err, store.ErrBadGameEntry), errors.Is(err, store.ErrUnknownPlayer):
		return http.StatusBadRequest
	case errors.Is(err, store.ErrNoGame), errors.Is(err, store.ErrNoGroup), errors.Is(err, store.ErrNoGameChange):
		return http.StatusNotFound
	case errors.Is(err, store.ErrGameChanged), errors.Is(err, store.ErrCodeHeld), errors.Is(err, store.ErrNumberHeld),
		errors.Is(err, store.ErrAddedGameLoaded), errors.Is(err, store.ErrChangedSince):
		return http.StatusConflict
	}
	return 0
}

// newGameChange is the store's form of a complete request.
func newGameChange(b GameChangeRequest) store.NewGameChange {
	c := store.NewGameChange{Action: string(b.Action), Reason: b.Reason}
	if b.Game != nil {
		c.Game = *b.Game
	}
	if b.Group != nil {
		c.Group = *b.Group
	}
	if b.Entry != nil {
		c.Entry = gameEntry(*b.Entry)
	}
	return c
}

// gameChangeComplete reports whether the request gives what its action takes, and nothing another action takes.
func gameChangeComplete(b GameChangeRequest) bool {
	switch b.Action {
	case GameChangeAdd:
		return b.Group != nil && b.Entry != nil && b.Game == nil
	case GameChangeRemove:
		return b.Game != nil && b.Entry == nil && b.Group == nil
	case GameChangeReplace:
		return b.Game != nil && b.Entry != nil && b.Group == nil
	}
	return false
}

// gameEntry is the store's form of a game typed in admin.
func gameEntry(e GameEntry) store.GameEntry {
	g := store.GameEntry{Code: e.Code, Date: e.Date.Time, Seats: make([]store.GameSeatEntry, len(e.Seats))}
	if e.Number != nil {
		g.Number = *e.Number
	}
	for i, s := range e.Seats {
		g.Seats[i] = store.GameSeatEntry{Player: s.Player, Place: s.Place, Outcome: string(s.Outcome), Score: s.Score}
	}
	return g
}

func (s *server) DeleteAdminGameChange(ctx context.Context, req DeleteAdminGameChangeRequestObject) (DeleteAdminGameChangeResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.UndoGameChange(ctx, req.Change); err != nil {
			return err
		}
		return tx.Audit(ctx, a.DiscordID, "game.undo", strconv.FormatInt(req.Change, 10), map[string]any{})
	})
	switch changeRefusal(err) {
	case http.StatusNotFound:
		return DeleteAdminGameChange404JSONResponse{notFound404(err.Error())}, nil
	case http.StatusConflict:
		return DeleteAdminGameChange409JSONResponse{conflict409(err.Error())}, nil
	}
	if err != nil {
		return nil, err
	}
	return DeleteAdminGameChange204Response{}, nil
}

func (s *server) GetAdminGame(ctx context.Context, req GetAdminGameRequestObject) (GetAdminGameResponseObject, error) {
	g, err := s.admins.GameByCode(ctx, req.Code)
	if errors.Is(err, store.ErrNoGame) {
		return GetAdminGame404JSONResponse{notFound404("no game we hold has that code")}, nil
	}
	if err != nil {
		return nil, err
	}
	out := ChangeableGame{Id: g.ID, Name: g.Name, Code: g.Code, Date: openapi_types.Date{Time: g.Date}, State: GameState(g.State),
		Number: g.Number, Seats: make([]ChangeableSeat, len(g.Seats))}
	if g.Group != 0 {
		out.Group = &g.Group
	}
	for i, st := range g.Seats {
		out.Seats[i] = ChangeableSeat{Player: st.Player, Place: st.Place, Score: st.Score}
		if st.Outcome != "" {
			o := SeatOutcome(st.Outcome)
			out.Seats[i].Outcome = &o
		}
	}
	return GetAdminGame200JSONResponse{Body: out, Headers: GetAdminGame200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) GetAdminSeasonGroups(ctx context.Context, req GetAdminSeasonGroupsRequestObject) (GetAdminSeasonGroupsResponseObject, error) {
	stages, err := s.admins.SeasonGroups(ctx, req.Tournament, req.Season)
	if errors.Is(err, store.ErrNoTournament) {
		return GetAdminSeasonGroups404JSONResponse{notFound404("no such season, or it has no group")}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]SeasonStage, len(stages))
	for i, st := range stages {
		out[i] = SeasonStage{Label: st.Label, Groups: make([]SeasonGroup, len(st.Groups))}
		for j, g := range st.Groups {
			out[i].Groups[j] = SeasonGroup{Id: g.ID, Name: g.Name, Games: g.Games, NextNumber: g.NextNumber}
		}
	}
	return GetAdminSeasonGroups200JSONResponse{Body: out, Headers: GetAdminSeasonGroups200ResponseHeaders{CacheControl: cacheNone}}, nil
}
