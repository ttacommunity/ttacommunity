// Admin's blitz resets: a match's result, the latest round's results, or the whole edition, as if never played.
package httpapi

import (
	"context"
	"errors"
	"fmt"

	"ttacommunity/backend/internal/store"
)

func (s *server) DeleteAdminBlitzMatchResult(ctx context.Context, req DeleteAdminBlitzMatchResultRequestObject) (DeleteAdminBlitzMatchResultResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return DeleteAdminBlitzMatchResult404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.ResetBlitzResult(ctx, req.Tournament, req.Edition, req.Match); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.match.reset", fmt.Sprint(req.Match), nil)
	})
	switch {
	case errors.Is(err, store.ErrNoBlitzEdition):
		return DeleteAdminBlitzMatchResult404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzMatch):
		return DeleteAdminBlitzMatchResult404JSONResponse{notFound404("no such match")}, nil
	case errors.Is(err, store.ErrBlitzBye), errors.Is(err, store.ErrBlitzNotLatest):
		return DeleteAdminBlitzMatchResult409JSONResponse{conflict409(err.Error())}, nil
	case err != nil:
		return nil, err
	}
	return DeleteAdminBlitzMatchResult204Response{}, nil
}

func (s *server) DeleteAdminBlitzRoundResults(ctx context.Context, req DeleteAdminBlitzRoundResultsRequestObject) (DeleteAdminBlitzRoundResultsResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return DeleteAdminBlitzRoundResults404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.ResetBlitzRound(ctx, req.Tournament, req.Edition, req.Round); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.round.reset", fmt.Sprint(req.Round), nil)
	})
	switch {
	case errors.Is(err, store.ErrNoBlitzEdition):
		return DeleteAdminBlitzRoundResults404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzRound):
		return DeleteAdminBlitzRoundResults404JSONResponse{notFound404("no such round")}, nil
	case errors.Is(err, store.ErrBlitzNotLatest):
		return DeleteAdminBlitzRoundResults409JSONResponse{conflict409(err.Error())}, nil
	case err != nil:
		return nil, err
	}
	return DeleteAdminBlitzRoundResults204Response{}, nil
}

func (s *server) PostAdminBlitzReset(ctx context.Context, req PostAdminBlitzResetRequestObject) (PostAdminBlitzResetResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PostAdminBlitzReset404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.ResetBlitzEdition(ctx, req.Tournament, req.Edition); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.reset", blitzEditionKey(req.Tournament, req.Edition), nil)
	})
	switch {
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PostAdminBlitzReset404JSONResponse{notFound404("no such blitz edition")}, nil
	case err != nil:
		return nil, err
	}
	return PostAdminBlitzReset204Response{}, nil
}
