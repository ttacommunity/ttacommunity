// Admin's blitz results: the game a match was played as, or the organisers' ruling on it.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

func (s *server) PutAdminBlitzMatchGame(ctx context.Context, req PutAdminBlitzMatchGameRequestObject) (PutAdminBlitzMatchGameResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PutAdminBlitzMatchGame400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	if req.Body == nil {
		return PutAdminBlitzMatchGame400JSONResponse{badRequest400("the game: share_code, a, b, finished_on, finished_time and zone")}, nil
	}
	finished, err := blitz.FinishedAt(req.Body.FinishedOn.Format(time.DateOnly), req.Body.FinishedTime, strings.TrimSpace(req.Body.Zone))
	if err != nil {
		return PutAdminBlitzMatchGame400JSONResponse{badRequest400(err.Error())}, nil
	}
	g := store.BlitzGame{ShareCode: req.Body.ShareCode, Finished: finished,
		A: store.BlitzSide{Culture: req.Body.A.Culture, Outcome: string(req.Body.A.Outcome)},
		B: store.BlitzSide{Culture: req.Body.B.Culture, Outcome: string(req.Body.B.Outcome)}}
	err = s.admins.RecordBlitzGame(ctx, req.Tournament, req.Edition, req.Match, g)
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
	case errors.Is(err, store.ErrBadBlitzRuling), errors.Is(err, store.ErrBlitzBye), errors.Is(err, store.ErrBlitzKnockoutWinner):
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

func (s *server) PutAdminBlitzMatchReplacement(ctx context.Context, req PutAdminBlitzMatchReplacementRequestObject) (PutAdminBlitzMatchReplacementResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PutAdminBlitzMatchReplacement400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	if req.Body == nil || req.Body.Out <= 0 || req.Body.In <= 0 {
		return PutAdminBlitzMatchReplacement400JSONResponse{badRequest400("the replacement: out and in")}, nil
	}
	switch err := s.admins.ReplaceBlitzPlayer(ctx, req.Tournament, req.Edition, req.Round, req.Match, req.Body.Out, req.Body.In); {
	case errors.Is(err, store.ErrBadBlitzMatch):
		return PutAdminBlitzMatchReplacement400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrBlitzNotKnockout), errors.Is(err, store.ErrBlitzNotDropped), errors.Is(err, store.ErrBlitzMatchDecided):
		return PutAdminBlitzMatchReplacement409JSONResponse{conflict409(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PutAdminBlitzMatchReplacement404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzRound):
		return PutAdminBlitzMatchReplacement404JSONResponse{notFound404("no such round")}, nil
	case errors.Is(err, store.ErrNoBlitzMatch):
		return PutAdminBlitzMatchReplacement404JSONResponse{notFound404("no such match")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "blitz.match.replace", fmt.Sprint(req.Match), req.Body); err != nil {
		return nil, err
	}
	return PutAdminBlitzMatchReplacement204Response{}, nil
}
