// Admin's best of 3: whether a knockout round's matches may be best of 3, and one match's games as an organiser sets them.
package httpapi

import (
	"context"
	"errors"
	"fmt"

	"ttacommunity/backend/internal/store"
)

func (s *server) PutAdminBlitzRoundBestOf(ctx context.Context, req PutAdminBlitzRoundBestOfRequestObject) (PutAdminBlitzRoundBestOfResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PutAdminBlitzRoundBestOf404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	if req.Body == nil {
		return PutAdminBlitzRoundBestOf400JSONResponse{badRequest400("best_of: 1 or 3")}, nil
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetBlitzRoundBestOf(ctx, req.Tournament, req.Edition, req.Round, req.Body.BestOf); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.round.best_of", fmt.Sprint(req.Round), req.Body)
	})
	switch {
	case errors.Is(err, store.ErrBadBlitzBestOf):
		return PutAdminBlitzRoundBestOf400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PutAdminBlitzRoundBestOf404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzRound):
		return PutAdminBlitzRoundBestOf404JSONResponse{notFound404("no such round")}, nil
	case errors.Is(err, store.ErrBlitzBestOfNotOffered), errors.Is(err, store.ErrBlitzBestOfAgreed):
		return PutAdminBlitzRoundBestOf409JSONResponse{conflict409(err.Error())}, nil
	case err != nil:
		return nil, err
	}
	return PutAdminBlitzRoundBestOf204Response{}, nil
}

func (s *server) PutAdminBlitzMatchBestOf(ctx context.Context, req PutAdminBlitzMatchBestOfRequestObject) (PutAdminBlitzMatchBestOfResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PutAdminBlitzMatchBestOf404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	if req.Body == nil {
		return PutAdminBlitzMatchBestOf400JSONResponse{badRequest400("best_of: 1 or 3")}, nil
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetBlitzMatchBestOf(ctx, req.Tournament, req.Edition, req.Match, req.Body.BestOf); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.match.best_of", fmt.Sprint(req.Match), req.Body)
	})
	switch {
	case errors.Is(err, store.ErrBadBlitzBestOf), errors.Is(err, store.ErrBlitzBye):
		return PutAdminBlitzMatchBestOf400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PutAdminBlitzMatchBestOf404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzMatch):
		return PutAdminBlitzMatchBestOf404JSONResponse{notFound404("no such match")}, nil
	case errors.Is(err, store.ErrBlitzBestOfNotOffered), errors.Is(err, store.ErrBlitzBestOfPlayed):
		return PutAdminBlitzMatchBestOf409JSONResponse{conflict409(err.Error())}, nil
	case err != nil:
		return nil, err
	}
	return PutAdminBlitzMatchBestOf204Response{}, nil
}
