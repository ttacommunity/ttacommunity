// Admin's view of players' sign-ins: listed, their requests to be linked approved or refused, and a sign-in
// unlinked from its member by admins and organisers; removed by admins only.
package httpapi

import (
	"context"
	"errors"
	"fmt"

	"ttacommunity/backend/internal/store"
)

func (s *server) GetAdminLogins(ctx context.Context, _ GetAdminLoginsRequestObject) (GetAdminLoginsResponseObject, error) {
	rows, err := s.admins.Logins(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AdminLogin, len(rows))
	for i, l := range rows {
		out[i] = AdminLogin{Id: l.ID, Provider: LoginProvider(l.Provider), Username: l.Username, DisplayName: l.DisplayName,
			MemberId: l.MemberID, Member: l.Member, SignedInAt: l.SignedInAt, AutoLink: l.AutoLink}
		if l.LinkedHow != "" {
			how := AdminLoginLinkedHow(l.LinkedHow)
			out[i].LinkedHow = &how
		}
		if r := l.Request; r != nil {
			out[i].Request = &AdminLinkRequest{Id: r.ID, Name: r.Name, Country: r.Country, Zone: r.Zone, Member: r.Member, MemberDiscord: r.MemberDiscord,
				MemberCountry: r.MemberCountry, MemberZone: r.MemberZone, RequestedAt: r.RequestedAt}
		}
		if r := l.Refused; r != nil {
			out[i].Refused = &AdminLinkRefused{Name: r.Name, Reason: r.Reason}
		}
	}
	return GetAdminLogins200JSONResponse{Body: out, Headers: GetAdminLogins200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PutAdminLinkApproval(ctx context.Context, req PutAdminLinkApprovalRequestObject) (PutAdminLinkApprovalResponseObject, error) {
	me, _ := adminFrom(ctx)
	got, err := s.admins.ApproveLink(ctx, req.LinkRequest, s.now())
	switch {
	case errors.Is(err, store.ErrNoRequest):
		return PutAdminLinkApproval404JSONResponse{notFound404("no such request waiting for an answer")}, nil
	case errors.Is(err, store.ErrAlreadyLinked), errors.Is(err, store.ErrMemberHasAccount),
		errors.Is(err, store.ErrBadMember):
		return PutAdminLinkApproval409JSONResponse{conflict409(err.Error())}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "link.approve", fmt.Sprint(req.LinkRequest), got); err != nil {
		return nil, err
	}
	return PutAdminLinkApproval200JSONResponse{Body: LinkApproved{Member: got.Member, Created: got.Created},
		Headers: PutAdminLinkApproval200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PutAdminLinkRefusal(ctx context.Context, req PutAdminLinkRefusalRequestObject) (PutAdminLinkRefusalResponseObject, error) {
	me, _ := adminFrom(ctx)
	if req.Body == nil {
		return PutAdminLinkRefusal400JSONResponse{badRequest400("a reason the player sees: reason, which may be empty")}, nil
	}
	err := s.admins.RefuseLink(ctx, req.LinkRequest, req.Body.Reason, s.now())
	switch {
	case errors.Is(err, store.ErrBadReason):
		return PutAdminLinkRefusal400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoRequest):
		return PutAdminLinkRefusal404JSONResponse{notFound404("no such request waiting for an answer")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "link.refuse", fmt.Sprint(req.LinkRequest), req.Body); err != nil {
		return nil, err
	}
	return PutAdminLinkRefusal204Response{}, nil
}

func (s *server) DeleteAdminLoginMember(ctx context.Context, req DeleteAdminLoginMemberRequestObject) (DeleteAdminLoginMemberResponseObject, error) {
	me, _ := adminFrom(ctx)
	if err := s.admins.UnlinkLogin(ctx, req.Login); errors.Is(err, store.ErrNoLogin) {
		return DeleteAdminLoginMember404JSONResponse{notFound404("no such sign-in")}, nil
	} else if err != nil {
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "login.unlink", fmt.Sprint(req.Login), nil); err != nil {
		return nil, err
	}
	return DeleteAdminLoginMember204Response{}, nil
}

func (s *server) DeleteAdminLogin(ctx context.Context, req DeleteAdminLoginRequestObject) (DeleteAdminLoginResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteAdminLogin403JSONResponse{forbidden("only an admin removes a sign-in")}, nil
	}
	if err := s.admins.RemoveLogin(ctx, req.Login); errors.Is(err, store.ErrNoLogin) {
		return DeleteAdminLogin404JSONResponse{notFound404("no such sign-in")}, nil
	} else if err != nil {
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "login.remove", fmt.Sprint(req.Login), nil); err != nil {
		return nil, err
	}
	return DeleteAdminLogin204Response{}, nil
}
