// Admin's players: titles set by hand, which replace the engine's from its next run.
package httpapi

import (
	"context"
	"errors"
	"strings"

	"ttacommunity/backend/internal/store"
)

const onlyAdminsTitle = "only admins set titles"

func (s *server) GetAdminTitles(ctx context.Context, _ GetAdminTitlesRequestObject) (GetAdminTitlesResponseObject, error) {
	if _, ok, err := adminOnly(ctx); err != nil || !ok {
		return GetAdminTitles403JSONResponse{forbidden(onlyAdminsTitle)}, err
	}
	list, err := s.admins.TitleOverrides(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]TitleOverride, len(list))
	for i, o := range list {
		out[i] = TitleOverride{Name: o.Name, Title: AdminTitle(o.Title), Reason: o.Reason}
	}
	return GetAdminTitles200JSONResponse{Body: out, Headers: GetAdminTitles200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PutAdminTitle(ctx context.Context, req PutAdminTitleRequestObject) (PutAdminTitleResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return PutAdminTitle403JSONResponse{forbidden(onlyAdminsTitle)}, err
	}
	b := req.Body
	if b == nil || !b.Title.Valid() || strings.TrimSpace(b.Reason) == "" || len(b.Reason) > 300 || len(req.Player) > 200 {
		return PutAdminTitle400JSONResponse{badRequest400("a title the engine gives (not WC), or none, with a reason under 300 characters")}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetTitle(ctx, store.TitleOverride{Name: req.Player, Title: string(b.Title), Reason: b.Reason}); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "player.title", req.Player, b)
	})
	switch {
	case errors.Is(err, store.ErrNoPlayer):
		return PutAdminTitle404JSONResponse{notFound404("no player has that name")}, nil
	case errors.Is(err, store.ErrBadTitle):
		return PutAdminTitle400JSONResponse{badRequest400("a title the engine gives (not WC), or none, with a reason")}, nil
	case err != nil:
		return nil, err
	}
	return PutAdminTitle204Response{}, nil
}

func (s *server) DeleteAdminTitle(ctx context.Context, req DeleteAdminTitleRequestObject) (DeleteAdminTitleResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return DeleteAdminTitle403JSONResponse{forbidden(onlyAdminsTitle)}, err
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.ClearTitle(ctx, req.Player); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "player.title.clear", req.Player, map[string]any{})
	})
	switch {
	case errors.Is(err, store.ErrNoTitle):
		return DeleteAdminTitle404JSONResponse{notFound404("no title is set in admin for that player")}, nil
	case err != nil:
		return nil, err
	}
	return DeleteAdminTitle204Response{}, nil
}
