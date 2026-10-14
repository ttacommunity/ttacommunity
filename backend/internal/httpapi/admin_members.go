// Admin's members: the community's people listed, added and changed by admins and organisers; banned, unbanned
// and removed by admins only.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

// nullableText is a string for a nullable field: nil for "".
func nullableText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *server) adminMember(m store.Member) AdminMember {
	offset, _ := blitz.Offset(m.Zone, s.now())
	return AdminMember{Id: m.ID, Name: m.Name, Player: m.Player, CgeUser: m.CGEUser, DiscordName: m.DiscordName, Email: m.Email,
		Country: nullableText(m.Country), Zone: m.Zone, UtcOffset: offset, Banned: m.Banned, BanReason: m.BanReason, BannedAt: m.BannedAt,
		Entries: m.Entries, CreatedAt: m.CreatedAt}
}

func (s *server) GetAdminMembers(ctx context.Context, _ GetAdminMembersRequestObject) (GetAdminMembersResponseObject, error) {
	members, err := s.admins.Members(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AdminMember, len(members))
	for i, m := range members {
		out[i] = s.adminMember(m)
	}
	return GetAdminMembers200JSONResponse{Body: out, Headers: GetAdminMembers200ResponseHeaders{CacheControl: cacheNone}}, nil
}

// memberZone checks a zone given for a member, Unknown when none, and says what is wrong with it.
func memberZone(zone *string) (string, string) {
	if zone == nil {
		return blitz.Unknown, ""
	}
	z := strings.TrimSpace(*zone)
	return z, zoneProblem(z)
}

func (s *server) PostAdminMember(ctx context.Context, req PostAdminMemberRequestObject) (PostAdminMemberResponseObject, error) {
	me, _ := adminFrom(ctx)
	if req.Body == nil {
		return PostAdminMember400JSONResponse{badRequest400("a member: name, and discord_name, email, country and zone as known")}, nil
	}
	zone, msg := memberZone(req.Body.Zone)
	if msg != "" {
		return PostAdminMember400JSONResponse{badRequest400(msg)}, nil
	}
	m := store.Member{Name: req.Body.Name, Zone: zone}
	for field, given := range map[*string]*string{&m.DiscordName: req.Body.DiscordName, &m.Email: req.Body.Email, &m.Country: req.Body.Country} {
		if given != nil {
			*field = *given
		}
	}
	id, err := s.admins.AddMember(ctx, m)
	var exists *store.MemberExistsError
	switch {
	case errors.As(err, &exists):
		return PostAdminMember409JSONResponse{conflict409(exists.Error())}, nil
	case errors.Is(err, store.ErrBadMember):
		return PostAdminMember400JSONResponse{badRequest400(err.Error())}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "member.add", fmt.Sprint(id), req.Body); err != nil {
		return nil, err
	}
	return PostAdminMember201JSONResponse{Body: MemberAdded{Id: id}, Headers: PostAdminMember201ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PatchAdminMember(ctx context.Context, req PatchAdminMemberRequestObject) (PatchAdminMemberResponseObject, error) {
	me, _ := adminFrom(ctx)
	b := req.Body
	if b == nil || (b.Name == nil && b.DiscordName == nil && b.Email == nil && b.Country == nil && b.Zone == nil && b.CgeUser == nil) {
		return PatchAdminMember400JSONResponse{badRequest400("nothing to change")}, nil
	}
	c := store.MemberChange{Name: b.Name, DiscordName: b.DiscordName, Email: b.Email, Country: b.Country, CGEUser: b.CgeUser}
	if b.Zone != nil {
		zone, msg := memberZone(b.Zone)
		if msg != "" {
			return PatchAdminMember400JSONResponse{badRequest400(msg)}, nil
		}
		c.Zone = &zone
	}
	err := s.admins.SetMember(ctx, req.Member, c)
	var exists *store.MemberExistsError
	switch {
	case errors.As(err, &exists), errors.Is(err, store.ErrCGEAccountTaken):
		return PatchAdminMember409JSONResponse{conflict409(err.Error())}, nil
	case errors.Is(err, store.ErrBadMember), errors.Is(err, store.ErrNoCGEAccount):
		return PatchAdminMember400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoMember):
		return PatchAdminMember404JSONResponse{notFound404("no such member")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "member.set", fmt.Sprint(req.Member), req.Body); err != nil {
		return nil, err
	}
	return PatchAdminMember204Response{}, nil
}

func (s *server) DeleteAdminMember(ctx context.Context, req DeleteAdminMemberRequestObject) (DeleteAdminMemberResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return DeleteAdminMember403JSONResponse{forbidden("only admins remove members")}, err
	}
	switch err := s.admins.RemoveMember(ctx, req.Member); {
	case errors.Is(err, store.ErrMemberInUse):
		return DeleteAdminMember409JSONResponse{conflict409(err.Error())}, nil
	case errors.Is(err, store.ErrNoMember):
		return DeleteAdminMember404JSONResponse{notFound404("no such member")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "member.remove", fmt.Sprint(req.Member), nil); err != nil {
		return nil, err
	}
	return DeleteAdminMember204Response{}, nil
}

func (s *server) PutAdminMemberBan(ctx context.Context, req PutAdminMemberBanRequestObject) (PutAdminMemberBanResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return PutAdminMemberBan403JSONResponse{forbidden("only admins ban members")}, err
	}
	if req.Body == nil {
		return PutAdminMemberBan400JSONResponse{badRequest400("a reason")}, nil
	}
	switch err := s.admins.BanMember(ctx, req.Member, req.Body.Reason); {
	case errors.Is(err, store.ErrBadMember):
		return PutAdminMemberBan400JSONResponse{badRequest400("a ban needs a reason of at most 1000 characters")}, nil
	case errors.Is(err, store.ErrNoMember):
		return PutAdminMemberBan404JSONResponse{notFound404("no such member")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "member.ban", fmt.Sprint(req.Member), req.Body); err != nil {
		return nil, err
	}
	return PutAdminMemberBan204Response{}, nil
}

func (s *server) DeleteAdminMemberBan(ctx context.Context, req DeleteAdminMemberBanRequestObject) (DeleteAdminMemberBanResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return DeleteAdminMemberBan403JSONResponse{forbidden("only admins lift bans")}, err
	}
	switch err := s.admins.UnbanMember(ctx, req.Member); {
	case errors.Is(err, store.ErrNoMember):
		return DeleteAdminMemberBan404JSONResponse{notFound404("no such member")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "member.unban", fmt.Sprint(req.Member), nil); err != nil {
		return nil, err
	}
	return DeleteAdminMemberBan204Response{}, nil
}
