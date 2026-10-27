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
	g, msg := blitzGame(req.Body, s.now())
	if msg != "" {
		return PutAdminBlitzMatchGame400JSONResponse{badRequest400(msg)}, nil
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.RecordBlitzGame(ctx, req.Tournament, req.Edition, req.Match, g); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.match.game", fmt.Sprint(req.Match), req.Body)
	})
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
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.RecordBlitzRuling(ctx, req.Tournament, req.Edition, req.Match, string(req.Body.Result), req.Body.Reason); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.match.ruling", fmt.Sprint(req.Match), req.Body)
	})
	switch {
	case errors.Is(err, store.ErrBadBlitzRuling), errors.Is(err, store.ErrBlitzBye), errors.Is(err, store.ErrBlitzKnockoutWinner):
		return PutAdminBlitzMatchRuling400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PutAdminBlitzMatchRuling404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzMatch):
		return PutAdminBlitzMatchRuling404JSONResponse{notFound404("no such match")}, nil
	case err != nil:
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
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.ReplaceBlitzPlayer(ctx, req.Tournament, req.Edition, req.Round, req.Match, req.Body.Out, req.Body.In); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.match.replace", fmt.Sprint(req.Match), req.Body)
	})
	switch {
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
	return PutAdminBlitzMatchReplacement204Response{}, nil
}
