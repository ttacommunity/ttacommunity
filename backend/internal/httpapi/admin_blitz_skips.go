// Admin's blitz skips: an entrant sitting out the next Swiss round, and undoing it before that round is published.
package httpapi

import (
	"context"
	"errors"
	"fmt"

	"ttacommunity/backend/internal/store"
)

func (s *server) PostAdminBlitzSkip(ctx context.Context, req PostAdminBlitzSkipRequestObject) (PostAdminBlitzSkipResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PostAdminBlitzSkip404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	var round int
	err := s.admins.Change(ctx, func(tx Admins) error {
		var err error
		if round, err = tx.SkipBlitzRound(ctx, req.Tournament, req.Edition, req.Entrant); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.entrant.skip", fmt.Sprint(req.Entrant), map[string]int{"round": round})
	})
	switch {
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PostAdminBlitzSkip404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzEntrant):
		return PostAdminBlitzSkip404JSONResponse{notFound404("no such entrant")}, nil
	case errors.Is(err, store.ErrBlitzEntrantDropped), errors.Is(err, store.ErrBlitzSkipNotSwiss), errors.Is(err, store.ErrBlitzMatchDecided):
		return PostAdminBlitzSkip409JSONResponse{conflict409(err.Error())}, nil
	case err != nil:
		return nil, err
	}
	return PostAdminBlitzSkip201JSONResponse{Body: BlitzSkip{Round: round}, Headers: PostAdminBlitzSkip201ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) DeleteAdminBlitzSkip(ctx context.Context, req DeleteAdminBlitzSkipRequestObject) (DeleteAdminBlitzSkipResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return DeleteAdminBlitzSkip404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.UnskipBlitzRound(ctx, req.Tournament, req.Edition, req.Entrant); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.entrant.unskip", fmt.Sprint(req.Entrant), nil)
	})
	switch {
	case errors.Is(err, store.ErrNoBlitzEdition):
		return DeleteAdminBlitzSkip404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzEntrant):
		return DeleteAdminBlitzSkip404JSONResponse{notFound404("no such entrant")}, nil
	case errors.Is(err, store.ErrBlitzNotSkipping):
		return DeleteAdminBlitzSkip409JSONResponse{conflict409(err.Error())}, nil
	case err != nil:
		return nil, err
	}
	return DeleteAdminBlitzSkip204Response{}, nil
}
