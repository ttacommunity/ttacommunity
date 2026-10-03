// Admin routes: every request under /v1/admin/ carries a Cloudflare Access token that must verify,
// and the Discord id in it must be an admin or organiser.

package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"ttacommunity/backend/internal/access"
	"ttacommunity/backend/internal/enginejob"
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
	AddSeason(ctx context.Context, slug, label string) error
	RemoveSeason(ctx context.Context, slug, label string) error
	Admins(ctx context.Context) ([]store.Admin, error)
	SetAdmin(ctx context.Context, a store.Admin) error
	RemoveAdmin(ctx context.Context, discordID string) error
	Runs(ctx context.Context) ([]store.AdminRun, error)
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
			seasons[j] = AdminSeason{Label: se.Label, Note: se.Note, HasGames: se.HasGames, Stages: stages}
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

func badRequest400(msg string) BadRequestJSONResponse {
	return BadRequestJSONResponse{Body: Error{Error: msg}, Headers: BadRequestResponseHeaders{CacheControl: cacheNone}}
}

func notFound404(msg string) NotFoundJSONResponse {
	return NotFoundJSONResponse{Body: Error{Error: msg}, Headers: NotFoundResponseHeaders{CacheControl: cacheNone}}
}

func (s *server) PostAdminSeason(ctx context.Context, req PostAdminSeasonRequestObject) (PostAdminSeasonResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	if req.Body == nil || strings.TrimSpace(req.Body.Label) == "" || len(req.Body.Label) > 64 {
		return PostAdminSeason400JSONResponse{badRequest400("a season needs a label")}, nil
	}
	label := strings.TrimSpace(req.Body.Label)
	switch err := s.admins.AddSeason(ctx, req.Tournament, label); {
	case errors.Is(err, store.ErrNoTournament):
		return PostAdminSeason404JSONResponse{notFound404("no such tournament")}, nil
	case errors.Is(err, store.ErrSeasonExists):
		return PostAdminSeason400JSONResponse{badRequest400("the tournament already has that season")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.season.add", req.Tournament, map[string]string{"label": label}); err != nil {
		return nil, err
	}
	return PostAdminSeason204Response{}, nil
}

func (s *server) DeleteAdminSeason(ctx context.Context, req DeleteAdminSeasonRequestObject) (DeleteAdminSeasonResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	switch err := s.admins.RemoveSeason(ctx, req.Tournament, req.Params.Label); {
	case errors.Is(err, store.ErrNoTournament):
		return DeleteAdminSeason404JSONResponse{notFound404("no such season")}, nil
	case errors.Is(err, store.ErrSeasonHasGames):
		return DeleteAdminSeason400JSONResponse{badRequest400("the season has games, so it stays")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.season.remove", req.Tournament, map[string]string{"label": req.Params.Label}); err != nil {
		return nil, err
	}
	return DeleteAdminSeason204Response{}, nil
}

const onlyAdminsRun = "only admins run the engine"

func conflict409(msg string) ConflictJSONResponse {
	return ConflictJSONResponse{Body: Error{Error: msg}, Headers: ConflictResponseHeaders{CacheControl: cacheNone}}
}

func (s *server) GetAdminRuns(ctx context.Context, _ GetAdminRunsRequestObject) (GetAdminRunsResponseObject, error) {
	if _, ok, err := adminOnly(ctx); err != nil || !ok {
		return GetAdminRuns403JSONResponse{forbidden(onlyAdminsRun)}, err
	}
	runs, err := s.admins.Runs(ctx)
	if err != nil {
		return nil, err
	}
	going, err := s.admins.EngineGoing(ctx)
	if err != nil {
		if !errors.Is(err, enginejob.ErrNotSetUp) {
			s.log.Error("asking the cluster whether the engine is going failed; runs are shown as recorded", "err", err)
		}
		going = true
	}
	out := make([]AdminRun, len(runs))
	for i, r := range runs {
		if r.Status == "running" && !going {
			r.Status, r.Progress = "failed", "the engine ended without finishing this run"
		}
		out[i] = AdminRun{
			Id: r.ID, Status: AdminRunStatus(r.Status), StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, PublishedAt: r.PublishedAt,
			Current: r.Current, Games: r.Games, Players: r.Players, EngineVersion: r.EngineVersion, Progress: r.Progress, ProgressAt: r.ProgressAt,
		}
	}
	return GetAdminRuns200JSONResponse{Body: out, Headers: GetAdminRuns200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PostAdminRun(ctx context.Context, _ PostAdminRunRequestObject) (PostAdminRunResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return PostAdminRun403JSONResponse{forbidden(onlyAdminsRun)}, err
	}
	job, err := s.admins.StartRun(ctx)
	switch {
	case errors.Is(err, enginejob.ErrRunning):
		return PostAdminRun409JSONResponse{conflict409("a rating run is already going")}, nil
	case errors.Is(err, enginejob.ErrNotSetUp):
		return PostAdminRun503JSONResponse{NotSetUpJSONResponse{Body: Error{Error: "starting runs is not set up here"}, Headers: NotSetUpResponseHeaders{CacheControl: cacheNone}}}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "run.start", job, map[string]any{}); err != nil {
		return nil, err
	}
	return PostAdminRun202JSONResponse{Body: StartedRun{Job: job}, Headers: PostAdminRun202ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PostAdminRunPublish(ctx context.Context, req PostAdminRunPublishRequestObject) (PostAdminRunPublishResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return PostAdminRunPublish403JSONResponse{forbidden(onlyAdminsRun)}, err
	}
	switch err := s.admins.Publish(ctx, req.Run); {
	case errors.Is(err, store.ErrRunNotFound):
		return PostAdminRunPublish404JSONResponse{notFound404("no such run")}, nil
	case errors.Is(err, store.ErrRunNotComplete):
		return PostAdminRunPublish409JSONResponse{conflict409("only a complete run can be published")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "run.publish", fmt.Sprint(req.Run), map[string]any{}); err != nil {
		return nil, err
	}
	return PostAdminRunPublish204Response{}, nil
}

func (s *server) GetAdminRunChanges(ctx context.Context, req GetAdminRunChangesRequestObject) (GetAdminRunChangesResponseObject, error) {
	if _, ok, err := adminOnly(ctx); err != nil || !ok {
		return GetAdminRunChanges403JSONResponse{forbidden(onlyAdminsRun)}, err
	}
	var against int64
	if req.Params.Against != nil {
		against = *req.Params.Against
	} else {
		runs, err := s.admins.Runs(ctx)
		if err != nil {
			return nil, err
		}
		against = defaultAgainst(runs, req.Run)
	}
	c, err := s.admins.RunChanges(ctx, req.Run, against)
	if errors.Is(err, store.ErrRunNotFound) {
		return GetAdminRunChanges404JSONResponse{notFound404("no such run, or none published to compare with")}, nil
	}
	if err != nil {
		return nil, err
	}
	biggest := make([]RunMove, len(c.Biggest))
	for i, m := range c.Biggest {
		biggest[i] = RunMove{Name: m.Name, Before: m.Before, After: m.After, Change: m.Change}
	}
	return GetAdminRunChanges200JSONResponse{Body: RunChanges{
		Run: req.Run, Against: against, Ratings: c.Ratings, RatingsBefore: c.RatingsBefore, New: c.New, Gone: c.Gone,
		Moved1: c.Moved1, Moved10: c.Moved10, Moved50: c.Moved50, Moved100: c.Moved100, Biggest: biggest,
	}, Headers: GetAdminRunChanges200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PostAdminRunStop(ctx context.Context, _ PostAdminRunStopRequestObject) (PostAdminRunStopResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return PostAdminRunStop403JSONResponse{forbidden(onlyAdminsRun)}, err
	}
	jobs, err := s.admins.StopRun(ctx)
	switch {
	case errors.Is(err, enginejob.ErrNotRunning):
		return PostAdminRunStop409JSONResponse{conflict409("no rating run is going")}, nil
	case errors.Is(err, enginejob.ErrNotSetUp):
		return PostAdminRunStop503JSONResponse{NotSetUpJSONResponse{Body: Error{Error: "stopping runs is not set up here"}, Headers: NotSetUpResponseHeaders{CacheControl: cacheNone}}}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "run.stop", strings.Join(jobs, ","), map[string]any{}); err != nil {
		return nil, err
	}
	return PostAdminRunStop204Response{}, nil
}

// defaultAgainst is the run another is compared with when none is asked for: the run the site showed before
// it. For a run never published that is the published run; for a published one, the run first published
// most recently before it. It is 0 when there is none.
func defaultAgainst(runs []store.AdminRun, run int64) int64 {
	var this *store.AdminRun
	var current int64
	for i, r := range runs {
		if r.ID == run {
			this = &runs[i]
		}
		if r.Current {
			current = r.ID
		}
	}
	if this == nil || this.PublishedAt == nil {
		return current
	}
	var before *store.AdminRun
	for i, r := range runs {
		if r.PublishedAt != nil && r.PublishedAt.Before(*this.PublishedAt) && (before == nil || r.PublishedAt.After(*before.PublishedAt)) {
			before = &runs[i]
		}
	}
	if before == nil {
		return 0
	}
	return before.ID
}
