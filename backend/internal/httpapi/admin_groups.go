// Admin's groups: making a group of related tournaments.
package httpapi

import (
	"context"
	"errors"

	"ttacommunity/backend/internal/store"
)

func (s *server) PostAdminGroup(ctx context.Context, req PostAdminGroupRequestObject) (PostAdminGroupResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	b := req.Body
	g := store.Group{Name: b.Name, Kind: string(b.Kind)}
	if b.Description != nil {
		g.Description = *b.Description
	}
	if b.Members != nil {
		g.Members = *b.Members
	}
	var slug string
	err := s.admins.Change(ctx, func(tx Admins) error {
		var err error
		if slug, err = tx.CreateGroup(ctx, g); err != nil {
			return err
		}
		return tx.Audit(ctx, a.DiscordID, "tournament.group", slug, b)
	})
	switch {
	case errors.Is(err, store.ErrBadGroup), errors.Is(err, store.ErrGroupCycle):
		return PostAdminGroup400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrBadKind):
		return PostAdminGroup400JSONResponse{badRequest400(store.ErrBadKind.Error())}, nil
	case errors.Is(err, store.ErrNoTournament):
		return PostAdminGroup404JSONResponse{NotFoundJSONResponse{Body: Error{Error: "no such tournament"}, Headers: NotFoundResponseHeaders{CacheControl: cacheNone}}}, nil
	case err != nil:
		return nil, err
	}
	return PostAdminGroup201JSONResponse{Body: CreatedGroup{Slug: slug}, Headers: PostAdminGroup201ResponseHeaders{CacheControl: cacheNone}}, nil
}
