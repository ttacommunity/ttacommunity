// Admin routes: every request under /v1/admin/ carries a Cloudflare Access token that must verify,
// and the Discord id in it must be an admin or organiser. Here too: who is in admin, and the replies
// every admin route shares. Each resource's routes are in their own admin_*.go.

package httpapi

import (
	"context"
	"errors"
	"io"
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
	TournamentRows(ctx context.Context) ([]store.AdminTournamentRow, error)
	Tournament(ctx context.Context, slug string) (store.AdminTournament, error)
	// ChangeTournament sets every part of a change in one transaction: a refused part changes nothing.
	ChangeTournament(ctx context.Context, slug string, c store.TournamentChange) error
	// ChangeSeason sets every part of a season change in one transaction: a refused part changes nothing.
	ChangeSeason(ctx context.Context, slug, season string, c store.SeasonChange) error
	CreateGroup(ctx context.Context, g store.Group) (string, error)
	DataSummary(ctx context.Context) (store.DataSummary, error)
	CheckStandings(ctx context.Context) (store.StandingsCheck, error)
	// ExportZip writes every input table, as the load job's export, as a zip to w.
	ExportZip(ctx context.Context, w io.Writer) error
	Badges(ctx context.Context) ([]store.Badge, error)
	AddBadge(ctx context.Context, b store.Badge) (int64, error)
	SetBadge(ctx context.Context, b store.Badge) error
	RemoveBadge(ctx context.Context, id int64) error
	TitleOverrides(ctx context.Context) ([]store.TitleOverride, error)
	SetTitle(ctx context.Context, o store.TitleOverride) error
	ClearTitle(ctx context.Context, name string) error
	AddSeason(ctx context.Context, slug, label string) error
	RemoveSeason(ctx context.Context, slug, label string) error
	Admins(ctx context.Context) ([]store.Admin, error)
	SetAdmin(ctx context.Context, a store.Admin) error
	RemoveAdmin(ctx context.Context, discordID string) error
	Runs(ctx context.Context) ([]store.AdminRun, error)
	SetRunProtected(ctx context.Context, run int64, protected bool) error
	// StartDeleteRun marks a run for deletion; DeleteRun then removes it, in the background.
	StartDeleteRun(ctx context.Context, run int64, engineGoing bool) error
	DeleteRun(ctx context.Context, run int64) error
	RunSizes(ctx context.Context) (map[int64]int64, error)
	Publish(ctx context.Context, run int64) error
	RunChanges(ctx context.Context, run, before int64) (store.RunChanges, error)
	// StartRun starts the engine now and returns its Job's name, or enginejob.ErrRunning or ErrNotSetUp.
	StartRun(ctx context.Context) (string, error)
	// StopRun stops the engine's unfinished Jobs and returns their names, or enginejob.ErrNotRunning or ErrNotSetUp.
	StopRun(ctx context.Context) ([]string, error)
	// EngineGoing reports whether an engine Job has not finished, or returns enginejob.ErrNotSetUp.
	EngineGoing(ctx context.Context) (bool, error)
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
			log.Error("reading an admin failed", "request_id", requestID(r.Context()), "err", err)
			serverError(w, r)
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

func badRequest400(msg string) BadRequestJSONResponse {
	return BadRequestJSONResponse{Body: Error{Error: msg}, Headers: BadRequestResponseHeaders{CacheControl: cacheNone}}
}

func notFound404(msg string) NotFoundJSONResponse {
	return NotFoundJSONResponse{Body: Error{Error: msg}, Headers: NotFoundResponseHeaders{CacheControl: cacheNone}}
}

func conflict409(msg string) ConflictJSONResponse {
	return ConflictJSONResponse{Body: Error{Error: msg}, Headers: ConflictResponseHeaders{CacheControl: cacheNone}}
}
