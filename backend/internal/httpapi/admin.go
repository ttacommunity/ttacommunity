// Admin routes: every request under /v1/admin/ carries a Cloudflare Access token that must verify,
// and the Discord id in it must be an admin or organiser.

package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"ttacommunity/backend/internal/access"
	"ttacommunity/backend/internal/store"
)

// adminPrefix is where the admin routes sit.
const adminPrefix = "/v1/admin/"

// Admins verifies who is calling admin, makes their changes, and records them.
type Admins interface {
	Verify(ctx context.Context, token string) (access.Identity, error)
	Admin(ctx context.Context, discordID string) (store.Admin, error)
	Audit(ctx context.Context, discordID, action, target string, detail any) error
	// AllowedOrigin is the admin site's origin, the only one a change may come from.
	AllowedOrigin() string
	Tournaments(ctx context.Context) ([]store.AdminTournament, error)
	SetTournament(ctx context.Context, slug string, s store.TournamentSettings) error
	SetNote(ctx context.Context, slug, season, stage, note string) error
	Admins(ctx context.Context) ([]store.Admin, error)
	SetAdmin(ctx context.Context, a store.Admin) error
	RemoveAdmin(ctx context.Context, discordID string) error
}

type adminKey struct{}

// adminGate lets a request under adminPrefix through only for a verified admin or organiser,
// whom it puts in the context. With admins nil, every admin request is refused.
func adminGate(next http.Handler, admins Admins, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", cacheNone)
		if admins == nil {
			writeError(w, http.StatusUnauthorized, "admin is not configured")
			return
		}
		id, err := admins.Verify(r.Context(), r.Header.Get("Cf-Access-Jwt-Assertion"))
		if err != nil {
			log.Info("admin token refused")
			writeError(w, http.StatusUnauthorized, "sign in through ttac-admin")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead &&
			(admins.AllowedOrigin() == "" || r.Header.Get("Origin") != admins.AllowedOrigin()) {
			log.Info("admin change refused: not from the admin site")
			writeError(w, http.StatusForbidden, "changes come only from the admin site")
			return
		}
		a, err := admins.Admin(r.Context(), id.DiscordID)
		if errors.Is(err, store.ErrNotAdmin) {
			log.Info("admin refused: not an admin or organiser")
			writeError(w, http.StatusForbidden, "not an admin or organiser")
			return
		}
		if err != nil {
			log.Error("reading an admin failed", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), adminKey{}, a)))
	})
}

// adminFrom returns the admin the gate verified for this request.
func adminFrom(ctx context.Context) (store.Admin, bool) {
	a, ok := ctx.Value(adminKey{}).(store.Admin)
	return a, ok
}

// routeAdmin sends admin paths through the gate, and every other path to public.
func routeAdmin(public, admin http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, adminPrefix) {
			admin.ServeHTTP(w, r)
			return
		}
		public.ServeHTTP(w, r)
	})
}

func (s *server) GetAdminMe(ctx context.Context, _ GetAdminMeRequestObject) (GetAdminMeResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	return GetAdminMe200JSONResponse{
		Body:    AdminMe{Name: a.Name, Role: AdminMeRole(a.Role)},
		Headers: GetAdminMe200ResponseHeaders{CacheControl: cacheNone},
	}, nil
}

func (s *server) GetAdminTournaments(ctx context.Context, _ GetAdminTournamentsRequestObject) (GetAdminTournamentsResponseObject, error) {
	ts, err := s.admins.Tournaments(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AdminTournament, len(ts))
	for i, t := range ts {
		seasons := make([]AdminSeason, len(t.Seasons))
		for j, se := range t.Seasons {
			stages := make([]AdminStage, len(se.Stages))
			for k, st := range se.Stages {
				stages[k] = AdminStage{Label: st.Label, Note: st.Note}
			}
			seasons[j] = AdminSeason{Label: se.Label, Note: se.Note, Stages: stages}
		}
		out[i] = AdminTournament{Slug: t.Slug, Name: t.Name, Visible: t.Visible, Rated: t.Rated, Community: t.Community, Seasons: seasons}
	}
	return GetAdminTournaments200JSONResponse{Body: out, Headers: GetAdminTournaments200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PatchAdminTournament(ctx context.Context, req PatchAdminTournamentRequestObject) (PatchAdminTournamentResponseObject, error) {
	a, ok := adminFrom(ctx)
	b := req.Body
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	if b == nil || (b.Visible == nil && b.Rated == nil && b.Community == nil) {
		return PatchAdminTournament400JSONResponse{BadRequestJSONResponse{Body: Error{Error: "nothing to change"}, Headers: BadRequestResponseHeaders{CacheControl: cacheNone}}}, nil
	}
	err := s.admins.SetTournament(ctx, req.Tournament, store.TournamentSettings{Visible: b.Visible, Rated: b.Rated, Community: b.Community})
	if errors.Is(err, store.ErrNoTournament) {
		return PatchAdminTournament404JSONResponse{NotFoundJSONResponse{Body: Error{Error: "no such tournament"}, Headers: NotFoundResponseHeaders{CacheControl: cacheNone}}}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.settings", req.Tournament, b); err != nil {
		return nil, err
	}
	return PatchAdminTournament204Response{}, nil
}

func (s *server) PutAdminNote(ctx context.Context, req PutAdminNoteRequestObject) (PutAdminNoteResponseObject, error) {
	a, ok := adminFrom(ctx)
	b := req.Body
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	if b == nil || b.Season == "" || len(b.Season) > 64 || len(b.Stage) > 64 || len(b.Note) > 2000 {
		return PutAdminNote400JSONResponse{BadRequestJSONResponse{Body: Error{Error: "a note needs a season, and stays under 2000 characters"}, Headers: BadRequestResponseHeaders{CacheControl: cacheNone}}}, nil
	}
	err := s.admins.SetNote(ctx, req.Tournament, b.Season, b.Stage, b.Note)
	if errors.Is(err, store.ErrNoTournament) {
		return PutAdminNote404JSONResponse{NotFoundJSONResponse{Body: Error{Error: "no such tournament or season"}, Headers: NotFoundResponseHeaders{CacheControl: cacheNone}}}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.note", req.Tournament, b); err != nil {
		return nil, err
	}
	return PutAdminNote204Response{}, nil
}

func forbidden(msg string) ForbiddenJSONResponse {
	return ForbiddenJSONResponse{Body: Error{Error: msg}, Headers: ForbiddenResponseHeaders{CacheControl: cacheNone}}
}

// adminOnly returns the signed-in admin, or ok false when they are an organiser.
func adminOnly(ctx context.Context) (store.Admin, bool, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return store.Admin{}, false, errors.New("admin route reached without the gate")
	}
	return a, a.Role == store.RoleAdmin, nil
}

func (s *server) GetAdminAdmins(ctx context.Context, _ GetAdminAdminsRequestObject) (GetAdminAdminsResponseObject, error) {
	if _, ok, err := adminOnly(ctx); err != nil || !ok {
		return GetAdminAdmins403JSONResponse{forbidden("only admins manage admin")}, err
	}
	list, err := s.admins.Admins(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AdminPerson, len(list))
	for i, a := range list {
		out[i] = AdminPerson{DiscordId: a.DiscordID, Name: a.Name, Role: AdminPersonRole(a.Role)}
	}
	return GetAdminAdmins200JSONResponse{Body: out, Headers: GetAdminAdmins200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PutAdminAdmin(ctx context.Context, req PutAdminAdminRequestObject) (PutAdminAdminResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return PutAdminAdmin403JSONResponse{forbidden("only admins manage admin")}, err
	}
	b := req.Body
	if b == nil || b.Name == "" || len(b.Name) > 64 || !b.Role.Valid() {
		return PutAdminAdmin400JSONResponse{BadRequestJSONResponse{Body: Error{Error: "a name and a role (admin or organiser)"}, Headers: BadRequestResponseHeaders{CacheControl: cacheNone}}}, nil
	}
	err = s.admins.SetAdmin(ctx, store.Admin{DiscordID: req.DiscordId, Name: b.Name, Role: string(b.Role)})
	if errors.Is(err, store.ErrLastAdmin) {
		return PutAdminAdmin403JSONResponse{forbidden("admin must keep at least one admin; make someone else admin first")}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "admin.set", req.DiscordId, b); err != nil {
		return nil, err
	}
	return PutAdminAdmin204Response{}, nil
}

func (s *server) DeleteAdminAdmin(ctx context.Context, req DeleteAdminAdminRequestObject) (DeleteAdminAdminResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return DeleteAdminAdmin403JSONResponse{forbidden("only admins manage admin")}, err
	}
	err = s.admins.RemoveAdmin(ctx, req.DiscordId)
	if errors.Is(err, store.ErrLastAdmin) {
		return DeleteAdminAdmin403JSONResponse{forbidden("admin must keep at least one admin; make someone else admin first")}, nil
	}
	if errors.Is(err, store.ErrNotAdmin) {
		return DeleteAdminAdmin404JSONResponse{NotFoundJSONResponse{Body: Error{Error: "not in admin"}, Headers: NotFoundResponseHeaders{CacheControl: cacheNone}}}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "admin.remove", req.DiscordId, map[string]any{}); err != nil {
		return nil, err
	}
	return DeleteAdminAdmin204Response{}, nil
}
