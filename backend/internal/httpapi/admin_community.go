// Admin routes for community leaderboard editions: list, create, edit.

package httpapi

import (
	"context"
	"errors"
	"strings"

	"ttacommunity/backend/internal/store"
)

const onlyAdminsCommunity = "only admins manage community editions"

// GetAdminCommunityEditions returns all editions (visible or not) for admin management.
func (s *server) GetAdminCommunityEditions(ctx context.Context, _ GetAdminCommunityEditionsRequestObject) (GetAdminCommunityEditionsResponseObject, error) {
	_, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return GetAdminCommunityEditions403JSONResponse{forbidden(onlyAdminsCommunity)}, nil
	}
	list, err := s.admins.AdminCommunityEditions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CommunityEditionItem, len(list))
	for i, e := range list {
		out[i] = communityEditionItem(e)
	}
	return GetAdminCommunityEditions200JSONResponse(out), nil
}

// PostAdminCommunityEdition creates a new community edition.
func (s *server) PostAdminCommunityEdition(ctx context.Context, req PostAdminCommunityEditionRequestObject) (PostAdminCommunityEditionResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PostAdminCommunityEdition403JSONResponse{forbidden(onlyAdminsCommunity)}, nil
	}
	if req.Body == nil {
		return PostAdminCommunityEdition400JSONResponse{badRequest400("edition data is required")}, nil
	}
	b := req.Body
	slug := strings.TrimSpace(b.Slug)
	name := strings.TrimSpace(b.Name)
	if slug == "" || name == "" {
		return PostAdminCommunityEdition400JSONResponse{badRequest400("slug and name are required")}, nil
	}
	counting := b.CountingEvents
	if counting <= 0 {
		counting = 8
	}
	ed := store.CommunityEdition{
		Slug:           slug,
		Name:           name,
		Title:          strings.TrimSpace(b.Title),
		ShortName:      strings.TrimSpace(b.ShortName),
		Badge:          strings.TrimSpace(b.Badge),
		Maintainer:     strings.TrimSpace(b.Maintainer),
		MaintainerNote: strings.TrimSpace(b.MaintainerNote),
		Year:           b.Year,
		CountingEvents: counting,
		Visible:        b.Visible,
		SortOrder:      b.SortOrder,
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.CreateCommunityEdition(ctx, ed); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "community_edition.add", ed.Slug, ed)
	})
	if err != nil {
		return nil, err
	}
	return PostAdminCommunityEdition201Response{}, nil
}

// PatchAdminCommunityEdition updates an existing edition's attributes.
func (s *server) PatchAdminCommunityEdition(ctx context.Context, req PatchAdminCommunityEditionRequestObject) (PatchAdminCommunityEditionResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PatchAdminCommunityEdition403JSONResponse{forbidden(onlyAdminsCommunity)}, nil
	}
	slug := strings.TrimSpace(req.Edition)
	if slug == "" {
		return PatchAdminCommunityEdition400JSONResponse{badRequest400("edition slug is required")}, nil
	}
	if req.Body == nil {
		return PatchAdminCommunityEdition400JSONResponse{badRequest400("edition change is required")}, nil
	}
	b := req.Body
	chg := store.CommunityEditionChange{
		Name:           b.Name,
		Title:          b.Title,
		ShortName:      b.ShortName,
		Badge:          b.Badge,
		Maintainer:     b.Maintainer,
		MaintainerNote: b.MaintainerNote,
		Year:           b.Year,
		CountingEvents: b.CountingEvents,
		Visible:        b.Visible,
		SortOrder:      b.SortOrder,
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetCommunityEdition(ctx, slug, chg); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "community_edition.edit", slug, b)
	})
	if errors.Is(err, store.ErrNoCommunityEdition) {
		return PatchAdminCommunityEdition404JSONResponse{notFound404("no such edition")}, nil
	}
	if err != nil {
		return nil, err
	}
	return PatchAdminCommunityEdition204Response{}, nil
}
