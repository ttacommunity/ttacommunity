// Admin's members: the community's people listed, added and changed by admins and organisers; banned, unbanned
// and removed by admins only.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

// fieldsSet is what the audit keeps of a change to members or entrants: the names of the fields the bodies set, never
// their values, which are personal details an account's deletion must not leave behind.
func fieldsSet(bodies ...any) ([]string, error) {
	set := map[string]bool{}
	for _, b := range bodies {
		j, err := json.Marshal(b)
		if err != nil {
			return nil, err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(j, &fields); err != nil {
			return nil, err
		}
		for k := range fields {
			set[k] = true
		}
	}
	return slices.Sorted(maps.Keys(set)), nil
}

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
	set, err := fieldsSet(req.Body)
	if err != nil {
		return nil, err
	}
	var id int64
	err = s.admins.Change(ctx, func(tx Admins) error {
		var err error
		if id, err = tx.AddMember(ctx, m); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "member.add", fmt.Sprint(id), map[string][]string{"set": set})
	})
	var exists *store.MemberExistsError
	switch {
	case errors.As(err, &exists):
		return PostAdminMember409JSONResponse{conflict409(exists.Error())}, nil
	case errors.Is(err, store.ErrBadMember):
		return PostAdminMember400JSONResponse{badRequest400(err.Error())}, nil
	case err != nil:
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
	set, err := fieldsSet(req.Body)
	if err != nil {
		return nil, err
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetMember(ctx, req.Member, c); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "member.set", fmt.Sprint(req.Member), map[string][]string{"set": set})
	})
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
	return PatchAdminMember204Response{}, nil
}

func (s *server) DeleteAdminMember(ctx context.Context, req DeleteAdminMemberRequestObject) (DeleteAdminMemberResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteAdminMember403JSONResponse{forbidden("only admins remove members")}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.RemoveMember(ctx, req.Member); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "member.remove", fmt.Sprint(req.Member), nil)
	})
	switch {
	case errors.Is(err, store.ErrMemberInUse), errors.Is(err, store.ErrMemberSignedIn):
		return DeleteAdminMember409JSONResponse{conflict409(err.Error())}, nil
	case errors.Is(err, store.ErrNoMember):
		return DeleteAdminMember404JSONResponse{notFound404("no such member")}, nil
	case err != nil:
		return nil, err
	}
	return DeleteAdminMember204Response{}, nil
}

func (s *server) PutAdminMemberBan(ctx context.Context, req PutAdminMemberBanRequestObject) (PutAdminMemberBanResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PutAdminMemberBan403JSONResponse{forbidden("only admins ban members")}, nil
	}
	if req.Body == nil {
		return PutAdminMemberBan400JSONResponse{badRequest400("a reason")}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.BanMember(ctx, req.Member, req.Body.Reason); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "member.ban", fmt.Sprint(req.Member), req.Body)
	})
	switch {
	case errors.Is(err, store.ErrBadMember):
		return PutAdminMemberBan400JSONResponse{badRequest400("a ban needs a reason of at most 1000 characters")}, nil
	case errors.Is(err, store.ErrNoMember):
		return PutAdminMemberBan404JSONResponse{notFound404("no such member")}, nil
	case err != nil:
		return nil, err
	}
	return PutAdminMemberBan204Response{}, nil
}

func (s *server) DeleteAdminMemberBan(ctx context.Context, req DeleteAdminMemberBanRequestObject) (DeleteAdminMemberBanResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteAdminMemberBan403JSONResponse{forbidden("only admins lift bans")}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.UnbanMember(ctx, req.Member); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "member.unban", fmt.Sprint(req.Member), nil)
	})
	switch {
	case errors.Is(err, store.ErrNoMember):
		return DeleteAdminMemberBan404JSONResponse{notFound404("no such member")}, nil
	case err != nil:
		return nil, err
	}
	return DeleteAdminMemberBan204Response{}, nil
}
