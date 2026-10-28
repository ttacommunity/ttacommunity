// The signed-in player's own routes: reading themselves and the data we keep, deleting themselves, asking to be
// linked to a member, and setting their own country and zone.
package httpapi

import (
	"context"
	"errors"
	"strings"
	"time"

	"ttacommunity/backend/internal/store"
)

func (s *server) GetMe(ctx context.Context, _ GetMeRequestObject) (GetMeResponseObject, error) {
	l, err := accountFrom(ctx)
	if err != nil {
		return nil, err
	}
	me := Me{Provider: LoginProvider(l.Provider), Username: l.Username, DisplayName: l.DisplayName, Country: nullableText(l.Country), Zone: l.Zone,
		Logins: make([]MyLogin, len(l.Logins))}
	for i, w := range l.Logins {
		me.Logins[i] = MyLogin{Provider: LoginProvider(w.Provider), Username: w.Username, DisplayName: w.DisplayName, Email: w.Email}
	}
	if l.Member != nil {
		me.Member = &MyMember{Name: l.Member.Name, Player: l.Member.Player, Banned: l.Banned, BanReason: l.BanReason}
	}
	if l.Request != nil {
		me.Request = &MyLinkRequest{Name: l.Request.Name, Status: MyLinkRequestStatus(l.Request.Status), Reason: l.Request.Reason,
			RequestedAt: l.Request.RequestedAt}
	}
	if l.AdminRole != "" {
		role := MeAdminRole(l.AdminRole)
		me.AdminRole = &role
	}
	return GetMe200JSONResponse{Body: me, Headers: GetMe200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) GetMyData(ctx context.Context, _ GetMyDataRequestObject) (GetMyDataResponseObject, error) {
	hash, _ := ctx.Value(sessionKey{}).([]byte)
	k, err := s.signIns.KeptData(ctx, hash, s.now())
	if errors.Is(err, store.ErrNoSession) {
		return GetMyData401JSONResponse{SignedOutJSONResponse{Body: Error{Error: "not signed in"}, Headers: SignedOutResponseHeaders{CacheControl: cacheNone}}}, nil
	}
	if err != nil {
		return nil, err
	}
	a := k.Account
	d := MyData{
		Account: MyDataAccount{CreatedAt: a.CreatedAt.UTC(), Country: nullableText(a.Country), Zone: nullableText(a.Zone), LinkedAt: inUTC(a.LinkedAt), AutoLink: a.AutoLink},
		Logins:  make([]MyDataLogin, len(k.Logins)), Requests: make([]MyDataRequest, len(k.Requests)),
		Sessions: make([]MyDataSession, len(k.Sessions)), EmailLinks: make([]MyDataEmailLink, len(k.EmailLinks)),
	}
	if a.LinkedHow != "" {
		how := MyDataAccountLinkedHow(a.LinkedHow)
		d.Account.LinkedHow = &how
	}
	for i, l := range k.Logins {
		d.Logins[i] = MyDataLogin{Provider: LoginProvider(l.Provider), Id: l.Subject, Username: l.Username, DisplayName: l.DisplayName,
			Email: l.Email, CreatedAt: l.CreatedAt.UTC(), SignedInAt: l.SignedInAt.UTC()}
	}
	if m := k.Member; m != nil {
		d.Member = &MyDataMember{Name: m.Name, CgeUser: m.CGEUser, Player: m.Player, DiscordName: m.DiscordName, Email: m.Email, EmailCleared: k.MemberEmailCleared,
			Country: nullableText(m.Country), Zone: m.Zone, Banned: m.Banned, BanReason: m.BanReason, BannedAt: inUTC(m.BannedAt), CreatedAt: m.CreatedAt.UTC()}
	}
	for i, r := range k.Requests {
		d.Requests[i] = MyDataRequest{Name: r.Name, Status: MyDataRequestStatus(r.Status), Reason: r.Reason, RequestedAt: r.RequestedAt.UTC(), DecidedAt: inUTC(r.DecidedAt)}
	}
	for i, x := range k.Sessions {
		d.Sessions[i] = MyDataSession{Provider: LoginProvider(x.Provider), CreatedAt: x.CreatedAt.UTC(), EndsAt: x.Ends.UTC(), ThisBrowser: x.ThisBrowser}
	}
	for i, l := range k.EmailLinks {
		d.EmailLinks[i] = MyDataEmailLink{Address: l.Address, ReturnPath: l.ReturnPath, CreatedAt: l.CreatedAt.UTC(), ExpiresAt: l.ExpiresAt.UTC(),
			UsedAt: inUTC(l.UsedAt), KeptUntil: l.KeptUntil.UTC()}
	}
	return GetMyData200JSONResponse{Body: d, Headers: GetMyData200ResponseHeaders{CacheControl: cacheNone}}, nil
}

// inUTC is t in UTC, or nil for nil.
func inUTC(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func (s *server) DeleteMe(ctx context.Context, _ DeleteMeRequestObject) (DeleteMeResponseObject, error) {
	hash, _ := ctx.Value(sessionKey{}).([]byte)
	err := s.signIns.DeleteAccount(ctx, hash, s.now())
	if errors.Is(err, store.ErrNoSession) {
		return DeleteMe401JSONResponse{SignedOutJSONResponse{Body: Error{Error: "not signed in"}, Headers: SignedOutResponseHeaders{CacheControl: cacheNone}}}, nil
	}
	if err != nil {
		return nil, err
	}
	return signedOut{}, nil
}

func (s *server) PutMyLinkRequest(ctx context.Context, req PutMyLinkRequestRequestObject) (PutMyLinkRequestResponseObject, error) {
	l, err := accountFrom(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return PutMyLinkRequest400JSONResponse{badRequest400("the TTA username to be linked to: name")}, nil
	}
	err = s.signIns.RequestLink(ctx, l.ID, req.Body.Name, s.now())
	switch {
	case errors.Is(err, store.ErrBadLinkName):
		return PutMyLinkRequest400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrAlreadyLinked):
		return PutMyLinkRequest409JSONResponse{conflict409("you are linked to a member already")}, nil
	case errors.Is(err, store.ErrNameLinked):
		return PutMyLinkRequest409JSONResponse{conflict409(nameLinkedMessage(s.signIns.SupportAddress()))}, nil
	case err != nil:
		return nil, err
	}
	return PutMyLinkRequest204Response{}, nil
}

func (s *server) PatchMyDetails(ctx context.Context, req PatchMyDetailsRequestObject) (PatchMyDetailsResponseObject, error) {
	l, err := accountFrom(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return PatchMyDetails400JSONResponse{badRequest400("a country, a zone, or both")}, nil
	}
	if z := req.Body.Zone; z != nil {
		if msg := zoneProblem(strings.TrimSpace(*z)); msg != "" {
			return PatchMyDetails400JSONResponse{badRequest400(msg)}, nil
		}
	}
	err = s.signIns.SetOwnDetails(ctx, l.ID, req.Body.Country, req.Body.Zone)
	switch {
	case errors.Is(err, store.ErrBadMember):
		return PatchMyDetails400JSONResponse{badRequest400("a country as its two-letter ISO code (NZ), and a time zone")}, nil
	case err != nil:
		return nil, err
	}
	return PatchMyDetails204Response{}, nil
}

func (s *server) GetCountries(context.Context, GetCountriesRequestObject) (GetCountriesResponseObject, error) {
	return GetCountries200JSONResponse{Body: store.CountryCodes(), Headers: GetCountries200ResponseHeaders{CacheControl: cacheDay}}, nil
}
