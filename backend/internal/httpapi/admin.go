// Admin routes: every request under /v1/admin/ carries a Cloudflare Access token that must verify,
// and the Discord id in it must be an admin or organiser.

package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	openapi_types "github.com/oapi-codegen/runtime/types"

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

func (s *server) PostAdminGroup(ctx context.Context, req PostAdminGroupRequestObject) (PostAdminGroupResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	b := req.Body
	g := store.Group{Name: b.Name, Kind: string(b.Kind)}
	if b.Description != nil {
		g.Description = *b.Description
	}
	if b.Members != nil {
		g.Members = *b.Members
	}
	slug, err := s.admins.CreateGroup(ctx, g)
	switch {
	case errors.Is(err, store.ErrBadGroup), errors.Is(err, store.ErrGroupCycle):
		return PostAdminGroup400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrBadKind):
		return PostAdminGroup400JSONResponse{badRequest400(store.ErrBadKind.Error())}, nil
	case errors.Is(err, store.ErrNoTournament):
		return PostAdminGroup404JSONResponse{NotFoundJSONResponse{Body: Error{Error: "no such tournament"}, Headers: NotFoundResponseHeaders{CacheControl: cacheNone}}}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.group", slug, b); err != nil {
		return nil, err
	}
	return PostAdminGroup201JSONResponse{Body: CreatedGroup{Slug: slug}, Headers: PostAdminGroup201ResponseHeaders{CacheControl: cacheNone}}, nil
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
	sizes, err := s.admins.RunSizes(ctx)
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
			Protected: r.Protected, Deleting: r.Deleting, Bytes: sizes[r.ID],
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

const onlyAdminsTitle = "only admins set titles"

func (s *server) GetAdminTitles(ctx context.Context, _ GetAdminTitlesRequestObject) (GetAdminTitlesResponseObject, error) {
	if _, ok, err := adminOnly(ctx); err != nil || !ok {
		return GetAdminTitles403JSONResponse{forbidden(onlyAdminsTitle)}, err
	}
	list, err := s.admins.TitleOverrides(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]TitleOverride, len(list))
	for i, o := range list {
		out[i] = TitleOverride{Name: o.Name, Title: AdminTitle(o.Title), Reason: o.Reason}
	}
	return GetAdminTitles200JSONResponse{Body: out, Headers: GetAdminTitles200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PutAdminTitle(ctx context.Context, req PutAdminTitleRequestObject) (PutAdminTitleResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return PutAdminTitle403JSONResponse{forbidden(onlyAdminsTitle)}, err
	}
	b := req.Body
	if b == nil || !b.Title.Valid() || strings.TrimSpace(b.Reason) == "" || len(b.Reason) > 300 || len(req.Player) > 200 {
		return PutAdminTitle400JSONResponse{badRequest400("a title the engine gives (not WC), or none, with a reason under 300 characters")}, nil
	}
	switch err := s.admins.SetTitle(ctx, store.TitleOverride{Name: req.Player, Title: string(b.Title), Reason: b.Reason}); {
	case errors.Is(err, store.ErrNoPlayer):
		return PutAdminTitle404JSONResponse{notFound404("no player has that name")}, nil
	case errors.Is(err, store.ErrBadTitle):
		return PutAdminTitle400JSONResponse{badRequest400("a title the engine gives (not WC), or none, with a reason")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "player.title", req.Player, b); err != nil {
		return nil, err
	}
	return PutAdminTitle204Response{}, nil
}

func (s *server) DeleteAdminTitle(ctx context.Context, req DeleteAdminTitleRequestObject) (DeleteAdminTitleResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return DeleteAdminTitle403JSONResponse{forbidden(onlyAdminsTitle)}, err
	}
	switch err := s.admins.ClearTitle(ctx, req.Player); {
	case errors.Is(err, store.ErrNoTitle):
		return DeleteAdminTitle404JSONResponse{notFound404("no title is set in admin for that player")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "player.title.clear", req.Player, map[string]any{}); err != nil {
		return nil, err
	}
	return DeleteAdminTitle204Response{}, nil
}

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
	if _, ok, err := adminOnly(ctx); err != nil || !ok {
		return GetAdminBadges403JSONResponse{forbidden(onlyAdminsBadge)}, err
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
	if err != nil || !ok {
		return PostAdminBadge403JSONResponse{forbidden(onlyAdminsBadge)}, err
	}
	b, msg := badgeSetting(req.Body)
	if msg != "" {
		return PostAdminBadge400JSONResponse{badRequest400(msg)}, nil
	}
	id, err := s.admins.AddBadge(ctx, b)
	switch {
	case errors.Is(err, store.ErrBadgeExists):
		return PostAdminBadge409JSONResponse{conflictBadge()}, nil
	case errors.Is(err, store.ErrBadBadge):
		return PostAdminBadge400JSONResponse{badRequest400("a badge's label is 1 to 16 characters")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "badge.add", b.Label, req.Body); err != nil {
		return nil, err
	}
	return PostAdminBadge201JSONResponse{
		Body:    AdminBadge{Id: id, Label: b.Label, Description: b.Description, Players: []string{}},
		Headers: PostAdminBadge201ResponseHeaders{CacheControl: cacheNone},
	}, nil
}

func (s *server) PutAdminBadge(ctx context.Context, req PutAdminBadgeRequestObject) (PutAdminBadgeResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return PutAdminBadge403JSONResponse{forbidden(onlyAdminsBadge)}, err
	}
	b, msg := badgeSetting(req.Body)
	if msg != "" {
		return PutAdminBadge400JSONResponse{badRequest400(msg)}, nil
	}
	b.ID = req.Badge
	if req.Body.Players != nil {
		b.Players = append([]string{}, *req.Body.Players...)
	}
	switch err := s.admins.SetBadge(ctx, b); {
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
	if err := s.admins.Audit(ctx, me.DiscordID, "badge.set", fmt.Sprint(req.Badge), req.Body); err != nil {
		return nil, err
	}
	return PutAdminBadge204Response{}, nil
}

func (s *server) DeleteAdminBadge(ctx context.Context, req DeleteAdminBadgeRequestObject) (DeleteAdminBadgeResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return DeleteAdminBadge403JSONResponse{forbidden(onlyAdminsBadge)}, err
	}
	switch err := s.admins.RemoveBadge(ctx, req.Badge); {
	case errors.Is(err, store.ErrNoBadge):
		return DeleteAdminBadge404JSONResponse{notFound404("no such badge")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "badge.remove", fmt.Sprint(req.Badge), map[string]any{}); err != nil {
		return nil, err
	}
	return DeleteAdminBadge204Response{}, nil
}

func (s *server) GetAdminData(ctx context.Context, _ GetAdminDataRequestObject) (GetAdminDataResponseObject, error) {
	d, err := s.admins.DataSummary(ctx)
	if err != nil {
		return nil, err
	}
	body := DataSummary{
		Games: d.Games, NeverPlayed: d.NeverPlayed, Unplaced: d.Unplaced, Players: d.Players,
		LastLoad: d.LastLoad, LastLoadRefused: d.LastLoadRefused,
	}
	if d.LatestGame != nil {
		body.LatestGame = &openapi_types.Date{Time: *d.LatestGame}
	}
	return GetAdminData200JSONResponse{Body: body, Headers: GetAdminData200ResponseHeaders{CacheControl: cacheNone}}, nil
}

// tournamentCard turns a tournament's card into the API's.
func tournamentCard(c store.Card) TournamentCard {
	return TournamentCard{Icon: c.Icon, Tagline: c.Tagline, Summary: c.Summary, Subtitle: c.Subtitle, Cadence: c.Cadence}
}

// storyList turns a tournament's stories into the API's.
func storyList(list []store.Story) []Story {
	out := make([]Story, len(list))
	for i, st := range list {
		out[i] = Story{Player: st.Player, Badge: st.Badge, Title: st.Title, Text: st.Text}
	}
	return out
}

func (s *server) GetAdminExport(ctx context.Context, _ GetAdminExportRequestObject) (GetAdminExportResponseObject, error) {
	me, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "data.export", "all", map[string]any{}); err != nil {
		return nil, err
	}
	r, w := io.Pipe()
	go func() {
		<-ctx.Done() // a download given up stops the export: its next write fails
		r.CloseWithError(ctx.Err())
	}()
	go func() {
		if err := s.admins.ExportZip(ctx, w); err != nil {
			s.log.Error("exporting the data failed", "err", err)
			w.CloseWithError(err)
			return
		}
		w.Close()
	}()
	return GetAdminExport200ApplicationzipResponse{
		Body: r,
		Headers: GetAdminExport200ResponseHeaders{
			CacheControl: cacheNone, ContentDisposition: `attachment; filename="ttacommunity-data.zip"`,
		},
	}, nil
}

func (s *server) GetAdminStandingsCheck(ctx context.Context, _ GetAdminStandingsCheckRequestObject) (GetAdminStandingsCheckResponseObject, error) {
	c, err := s.admins.CheckStandings(ctx)
	if err != nil {
		return nil, err
	}
	body := StandingsCheck{Stages: []StageCheck{}, StagesOnlyOnCge: c.StagesOnlyOnCGE}
	for _, st := range c.Stages {
		out := StageCheck{CgeTournament: st.CGETournament, CgeSeason: st.CGESeason, Name: st.Name,
			Players: st.Players, Agree: st.Agree, Differences: []StandingDifference{}}
		for _, d := range st.Differences {
			out.Differences = append(out.Differences, StandingDifference{
				CgeUser: d.CGEUser, Player: d.Player, Kind: StandingDifferenceKind(d.Kind),
				CgeGames: d.CGEGames, HeldGames: d.HeldGames, HeldNeverPlayed: d.HeldNeverPlayed, CgePoints: d.CGEPoints,
				CgePlaces: places(d.CGEPlaces), HeldPlaces: places(d.HeldPlaces),
			})
		}
		body.Stages = append(body.Stages, out)
	}
	return GetAdminStandingsCheck200JSONResponse{Body: body, Headers: GetAdminStandingsCheck200ResponseHeaders{CacheControl: cacheNone}}, nil
}

// places turns a player's places into the API's, null for a size not played.
func places(p store.Places) Places {
	size := func(counts []int) *[]int {
		if counts == nil {
			return nil
		}
		return &counts
	}
	return Places{P2: size(p.P2), P3: size(p.P3), P4: size(p.P4)}
}

// deleteRunTimeout bounds a background delete: the largest runs take minutes.
const deleteRunTimeout = time.Hour

func (s *server) PatchAdminRun(ctx context.Context, req PatchAdminRunRequestObject) (PatchAdminRunResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return PatchAdminRun403JSONResponse{forbidden(onlyAdminsRun)}, err
	}
	if req.Body == nil {
		return PatchAdminRun400JSONResponse{badRequest400("say whether the run is protected")}, nil
	}
	switch err := s.admins.SetRunProtected(ctx, req.Run, req.Body.Protected); {
	case errors.Is(err, store.ErrRunNotFound):
		return PatchAdminRun404JSONResponse{notFound404("no such run")}, nil
	case err != nil:
		return nil, err
	}
	action := "run.unprotect"
	if req.Body.Protected {
		action = "run.protect"
	}
	if err := s.admins.Audit(ctx, me.DiscordID, action, fmt.Sprint(req.Run), map[string]any{}); err != nil {
		return nil, err
	}
	return PatchAdminRun204Response{}, nil
}

func (s *server) DeleteAdminRun(ctx context.Context, req DeleteAdminRunRequestObject) (DeleteAdminRunResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return DeleteAdminRun403JSONResponse{forbidden(onlyAdminsRun)}, err
	}
	going, err := s.admins.EngineGoing(ctx)
	if err != nil {
		if !errors.Is(err, enginejob.ErrNotSetUp) {
			s.log.Error("asking the cluster whether the engine is going failed; a running run is kept", "err", err)
		}
		going = true
	}
	switch err := s.admins.StartDeleteRun(ctx, req.Run, going); {
	case errors.Is(err, store.ErrRunNotFound):
		return DeleteAdminRun404JSONResponse{notFound404("no such run")}, nil
	case errors.Is(err, store.ErrRunProtected):
		return DeleteAdminRun409JSONResponse{conflict409("the run is protected")}, nil
	case errors.Is(err, store.ErrRunCurrent):
		return DeleteAdminRun409JSONResponse{conflict409("the run is the one the site shows")}, nil
	case errors.Is(err, store.ErrRunRunning):
		return DeleteAdminRun409JSONResponse{conflict409("the run is still running")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "run.delete", fmt.Sprint(req.Run), map[string]any{}); err != nil {
		return nil, err
	}
	go func(run int64) {
		ctx, cancel := context.WithTimeout(context.Background(), deleteRunTimeout)
		defer cancel()
		if err := s.admins.DeleteRun(ctx, run); err != nil {
			s.log.Error("deleting a run failed; it stays marked, to delete again", "run", run, "err", err)
		}
	}(req.Run)
	return DeleteAdminRun202Response{}, nil
}

// dateOf returns t as an API date, or nil for none.
func dateOf(t *time.Time) *openapi_types.Date {
	if t == nil {
		return nil
	}
	return &openapi_types.Date{Time: *t}
}

// podiumEntries returns a podium as the API's, empty for none.
func podiumEntries(p []store.PodiumEntry) []PodiumEntry {
	out := make([]PodiumEntry, len(p))
	for i, e := range p {
		out[i] = PodiumEntry{Place: e.Place, Name: e.Name}
	}
	return out
}
