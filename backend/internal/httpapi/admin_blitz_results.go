// Admin's blitz results: the game a match was played as, or the organisers' ruling on it.
package httpapi

import (
	"context"
	"errors"
	"fmt"

	"ttacommunity/backend/internal/store"
)

func (s *server) PutAdminBlitzMatchGame(ctx context.Context, req PutAdminBlitzMatchGameRequestObject) (PutAdminBlitzMatchGameResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PutAdminBlitzMatchGame400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	if req.Body == nil {
		return PutAdminBlitzMatchGame400JSONResponse{badRequest400("the game: share_code, a, b and played_on")}, nil
	}
	g := store.BlitzGame{ShareCode: req.Body.ShareCode, Played: req.Body.PlayedOn.Time,
		A: store.BlitzSide{Culture: req.Body.A.Culture, Outcome: string(req.Body.A.Outcome)},
		B: store.BlitzSide{Culture: req.Body.B.Culture, Outcome: string(req.Body.B.Outcome)}}
	err := s.admins.RecordBlitzGame(ctx, req.Tournament, req.Edition, req.Match, g)
	var taken *store.BlitzShareCodeError
	switch {
	case errors.As(err, &taken):
		return PutAdminBlitzMatchGame409JSONResponse{conflict409(taken.Error())}, nil
	case errors.Is(err, store.ErrBadBlitzGame), errors.Is(err, store.ErrBlitzBye):
		return PutAdminBlitzMatchGame400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PutAdminBlitzMatchGame404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzMatch):
		return PutAdminBlitzMatchGame404JSONResponse{notFound404("no such match")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "blitz.match.game", fmt.Sprint(req.Match), req.Body); err != nil {
		return nil, err
	}
	return PutAdminBlitzMatchGame204Response{}, nil
}

func (s *server) PutAdminBlitzMatchRuling(ctx context.Context, req PutAdminBlitzMatchRulingRequestObject) (PutAdminBlitzMatchRulingResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PutAdminBlitzMatchRuling400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	if req.Body == nil {
		return PutAdminBlitzMatchRuling400JSONResponse{badRequest400("the ruling: result and reason")}, nil
	}
	switch err := s.admins.RecordBlitzRuling(ctx, req.Tournament, req.Edition, req.Match, string(req.Body.Result), req.Body.Reason); {
	case errors.Is(err, store.ErrBadBlitzRuling), errors.Is(err, store.ErrBlitzBye):
		return PutAdminBlitzMatchRuling400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PutAdminBlitzMatchRuling404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzMatch):
		return PutAdminBlitzMatchRuling404JSONResponse{notFound404("no such match")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "blitz.match.ruling", fmt.Sprint(req.Match), req.Body); err != nil {
		return nil, err
	}
	return PutAdminBlitzMatchRuling204Response{}, nil
}
