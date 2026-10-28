// Admin's badges: the list, and making, changing, giving and removing a badge.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"ttacommunity/backend/internal/store"
)

const onlyAdminsBadge = "only admins manage badges"

// badgeSetting checks a badge's label (1 to 16 characters) and description (at most 300), or gives the refusal.
func badgeSetting(b *BadgeSetting) (store.Badge, string) {
	if b == nil {
		return store.Badge{}, "a badge needs a label"
	}
	label := strings.TrimSpace(b.Label)
	if label == "" || utf8.RuneCountInString(label) > 16 || len(b.Description) > 300 {
		return store.Badge{}, "a badge's label is 1 to 16 characters, its description at most 300"
	}
	return store.Badge{Label: label, Description: strings.TrimSpace(b.Description)}, ""
}

func conflictBadge() ConflictJSONResponse {
	return conflict409("another badge has that label")
}

func (s *server) GetAdminBadges(ctx context.Context, _ GetAdminBadgesRequestObject) (GetAdminBadgesResponseObject, error) {
	_, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return GetAdminBadges403JSONResponse{forbidden(onlyAdminsBadge)}, nil
	}
	list, err := s.admins.Badges(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AdminBadge, len(list))
	for i, b := range list {
		out[i] = AdminBadge{Id: b.ID, Label: b.Label, Description: b.Description, Players: append([]string{}, b.Players...)}
	}
	return GetAdminBadges200JSONResponse{Body: out, Headers: GetAdminBadges200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PostAdminBadge(ctx context.Context, req PostAdminBadgeRequestObject) (PostAdminBadgeResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PostAdminBadge403JSONResponse{forbidden(onlyAdminsBadge)}, nil
	}
	b, msg := badgeSetting(req.Body)
	if msg != "" {
		return PostAdminBadge400JSONResponse{badRequest400(msg)}, nil
	}
	var id int64
	err = s.admins.Change(ctx, func(tx Admins) error {
		var err error
		if id, err = tx.AddBadge(ctx, b); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "badge.add", b.Label, req.Body)
	})
	switch {
	case errors.Is(err, store.ErrBadgeExists):
		return PostAdminBadge409JSONResponse{conflictBadge()}, nil
	case errors.Is(err, store.ErrBadBadge):
		return PostAdminBadge400JSONResponse{badRequest400("a badge's label is 1 to 16 characters")}, nil
	case err != nil:
		return nil, err
	}
	return PostAdminBadge201JSONResponse{
		Body:    AdminBadge{Id: id, Label: b.Label, Description: b.Description, Players: []string{}},
		Headers: PostAdminBadge201ResponseHeaders{CacheControl: cacheNone},
	}, nil
}

func (s *server) PutAdminBadge(ctx context.Context, req PutAdminBadgeRequestObject) (PutAdminBadgeResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PutAdminBadge403JSONResponse{forbidden(onlyAdminsBadge)}, nil
	}
	b, msg := badgeSetting(req.Body)
	if msg != "" {
		return PutAdminBadge400JSONResponse{badRequest400(msg)}, nil
	}
	b.ID = req.Badge
	if req.Body.Players != nil {
		b.Players = append([]string{}, *req.Body.Players...)
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetBadge(ctx, b); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "badge.set", fmt.Sprint(req.Badge), req.Body)
	})
	switch {
	case errors.Is(err, store.ErrNoBadge):
		return PutAdminBadge404JSONResponse{notFound404("no such badge")}, nil
	case errors.Is(err, store.ErrBadgeExists):
		return PutAdminBadge409JSONResponse{conflictBadge()}, nil
	case errors.Is(err, store.ErrBadBadge):
		return PutAdminBadge400JSONResponse{badRequest400("a badge's label is 1 to 16 characters")}, nil
	case errors.Is(err, store.ErrNoPlayer):
		return PutAdminBadge404JSONResponse{notFound404("no player has one of those names")}, nil
	case err != nil:
		return nil, err
	}
	return PutAdminBadge204Response{}, nil
}

func (s *server) DeleteAdminBadge(ctx context.Context, req DeleteAdminBadgeRequestObject) (DeleteAdminBadgeResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteAdminBadge403JSONResponse{forbidden(onlyAdminsBadge)}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.RemoveBadge(ctx, req.Badge); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "badge.remove", fmt.Sprint(req.Badge), map[string]any{})
	})
	switch {
	case errors.Is(err, store.ErrNoBadge):
		return DeleteAdminBadge404JSONResponse{notFound404("no such badge")}, nil
	case err != nil:
		return nil, err
	}
	return DeleteAdminBadge204Response{}, nil
}
