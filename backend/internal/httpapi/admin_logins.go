// Admin's view of players' accounts: listed with their ways in, their requests to be linked approved or refused, and
// an account unlinked from its member by admins and organisers; removed by admins only.
package httpapi

import (
	"context"
	"errors"
	"fmt"

	"ttacommunity/backend/internal/store"
)

func (s *server) GetAdminAccounts(ctx context.Context, _ GetAdminAccountsRequestObject) (GetAdminAccountsResponseObject, error) {
	rows, err := s.admins.Accounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AdminAccount, len(rows))
	for i, a := range rows {
		logins := make([]AdminAccountLogin, len(a.Logins))
		for j, l := range a.Logins {
			logins[j] = AdminAccountLogin{Provider: LoginProvider(l.Provider), Username: l.Username, DisplayName: l.DisplayName, Email: l.Email, SignedInAt: l.SignedInAt}
		}
		out[i] = AdminAccount{Id: a.ID, Logins: logins, MemberId: a.MemberID, Member: a.Member, SignedInAt: a.SignedInAt, AutoLink: a.AutoLink}
		if a.LinkedHow != "" {
			how := AdminAccountLinkedHow(a.LinkedHow)
			out[i].LinkedHow = &how
		}
		if r := a.Request; r != nil {
			out[i].Request = &AdminLinkRequest{Id: r.ID, Name: r.Name, Country: r.Country, Zone: r.Zone, Member: r.Member, MemberDiscord: r.MemberDiscord,
				MemberEmail: r.MemberEmail, MemberCountry: r.MemberCountry, MemberZone: r.MemberZone, RequestedAt: r.RequestedAt}
		}
		if r := a.Refused; r != nil {
			out[i].Refused = &AdminLinkRefused{Name: r.Name, Reason: r.Reason}
		}
	}
	return GetAdminAccounts200JSONResponse{Body: out, Headers: GetAdminAccounts200ResponseHeaders{CacheControl: cacheNone}}, nil
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
	if err := s.admins.Audit(ctx, me.DiscordID, "link.approve", fmt.Sprint(req.LinkRequest), map[string]any{"member": got.MemberID, "created": got.Created}); err != nil {
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

func (s *server) DeleteAdminAccountMember(ctx context.Context, req DeleteAdminAccountMemberRequestObject) (DeleteAdminAccountMemberResponseObject, error) {
	me, _ := adminFrom(ctx)
	if err := s.admins.UnlinkAccount(ctx, req.Account); errors.Is(err, store.ErrNoAccount) {
		return DeleteAdminAccountMember404JSONResponse{notFound404("no such account")}, nil
	} else if err != nil {
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "account.unlink", fmt.Sprint(req.Account), nil); err != nil {
		return nil, err
	}
	return DeleteAdminAccountMember204Response{}, nil
}

func (s *server) DeleteAdminAccount(ctx context.Context, req DeleteAdminAccountRequestObject) (DeleteAdminAccountResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteAdminAccount403JSONResponse{forbidden("only an admin removes an account")}, nil
	}
	if err := s.admins.RemoveAccount(ctx, req.Account); errors.Is(err, store.ErrNoAccount) {
		return DeleteAdminAccount404JSONResponse{notFound404("no such account")}, nil
	} else if err != nil {
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "account.remove", fmt.Sprint(req.Account), nil); err != nil {
		return nil, err
	}
	return DeleteAdminAccount204Response{}, nil
}
