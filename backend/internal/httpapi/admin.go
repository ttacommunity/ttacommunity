// Admin routes: every request under /v1/admin/ carries a Cloudflare Access token that must verify,
// and the Discord id in it must be an admin or organiser.

package httpapi

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
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
	Tournaments(ctx context.Context) ([]store.AdminTournament, error)
	SetTournament(ctx context.Context, slug string, s store.TournamentSettings) error
	CreateGroup(ctx context.Context, g store.Group) (string, error)
	SetMembers(ctx context.Context, group string, members []string) error
	SeasonCounts(ctx context.Context, slug string) ([]store.SeasonCount, error)
	SetSeasonRated(ctx context.Context, slug, season string, rated bool) error
	SetSeasonPenalty(ctx context.Context, slug, season string, penalty float64) error
	SetSeasonInChampions(ctx context.Context, slug, season string, in bool) error
	SetSeasonInCommunity(ctx context.Context, slug, season string, in bool) error
	SetNote(ctx context.Context, slug, season, stage, note string) error
	DataSummary(ctx context.Context) (store.DataSummary, error)
	CheckStandings(ctx context.Context) (store.StandingsCheck, error)
	// ExportZip writes every input table, as the load job's export, as a zip to w.
	ExportZip(ctx context.Context, w io.Writer) error
	Badges(ctx context.Context) ([]store.Badge, error)
	AddBadge(ctx context.Context, b store.Badge) (int64, error)
	SetBadge(ctx context.Context, b store.Badge) error
	RemoveBadge(ctx context.Context, id int64) error
	GiveBadge(ctx context.Context, id int64, name string) error
	TakeBadge(ctx context.Context, id int64, name string) error
	TitleOverrides(ctx context.Context) ([]store.TitleOverride, error)
	SetTitle(ctx context.Context, o store.TitleOverride) error
	ClearTitle(ctx context.Context, name string) error
	SetCard(ctx context.Context, slug string, c store.Card) error
	SetStories(ctx context.Context, slug string, list []store.Story) error
	SetRegulations(ctx context.Context, slug, text string) error
	SetPodium(ctx context.Context, slug, season string, podium []store.PodiumEntry, note string) error
	ClearPodium(ctx context.Context, slug, season string) error
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
				stages[k] = AdminStage{Label: st.Label, OriginalName: st.OriginalName, Note: st.Note}
			}
			podium, suggested := podiumEntries(se.Podium), podiumEntries(se.Suggested)
			seasons[j] = AdminSeason{Label: se.Label, OriginalName: se.OriginalName, Note: se.Note, HasGames: se.HasGames, Rated: se.Rated, InChampions: se.InChampions, InCommunity: se.InCommunity,
				TimeoutPenalty: se.TimeoutPenalty, Stages: stages, Podium: podium, PodiumNote: se.PodiumNote,
				SuggestedPodium: suggested, SuggestedFrom: AdminSeasonSuggestedFrom(se.SuggestedFrom)}
		}
		out[i] = AdminTournament{Slug: t.Slug, Name: t.Name, OriginalName: t.OriginalName, Parent: t.Parent, Description: t.Description, IsGroup: t.IsGroup,
			Games: t.Games, FirstGame: dateOf(t.FirstGame), LastGame: dateOf(t.LastGame), Kind: TournamentKind(t.Kind),
			TopDivisions: append([]string{}, t.TopDivisions...), Visible: t.Visible, Rated: t.Rated, Community: t.Community,
			Points2p: append(PlacePoints{}, t.Points[2]...), Points3p: append(PlacePoints{}, t.Points[3]...), Points4p: append(PlacePoints{}, t.Points[4]...),
			SeasonName: t.SeasonName, StageName: t.StageName, Card: tournamentCard(t.Card), Stories: storyList(t.Stories), RatedPlayerCounts: append([]int{}, t.RatedPlayerCounts...), Regulations: t.Regulations, CgeRegulations: t.CGERegulations, Seasons: seasons}
	}
	return GetAdminTournaments200JSONResponse{Body: out, Headers: GetAdminTournaments200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PatchAdminTournament(ctx context.Context, req PatchAdminTournamentRequestObject) (PatchAdminTournamentResponseObject, error) {
	a, ok := adminFrom(ctx)
	b := req.Body
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	if b == nil || (b.Visible == nil && b.Rated == nil && b.Community == nil && b.RatedPlayerCounts == nil &&
		b.Kind == nil && b.TopDivisions == nil && b.Points2p == nil && b.Points3p == nil && b.Points4p == nil &&
		b.SeasonName == nil && b.StageName == nil && b.Parent == nil && b.Description == nil && b.Name == nil) {
		return PatchAdminTournament400JSONResponse{BadRequestJSONResponse{Body: Error{Error: "nothing to change"}, Headers: BadRequestResponseHeaders{CacheControl: cacheNone}}}, nil
	}
	settings := store.TournamentSettings{Visible: b.Visible, Rated: b.Rated, Community: b.Community, SeasonName: b.SeasonName,
		StageName: b.StageName, Parent: b.Parent, Description: b.Description, Name: b.Name}
	if b.RatedPlayerCounts != nil {
		settings.RatedPlayerCounts = *b.RatedPlayerCounts
	}
	if b.Kind != nil {
		settings.Kind = string(*b.Kind)
	}
	if b.TopDivisions != nil {
		settings.TopDivisions = *b.TopDivisions
	}
	for players, points := range map[int]*PlacePoints{2: b.Points2p, 3: b.Points3p, 4: b.Points4p} {
		if points != nil {
			if settings.Points == nil {
				settings.Points = map[int][]float64{}
			}
			settings.Points[players] = append([]float64{}, *points...)
		}
	}
	err := s.admins.SetTournament(ctx, req.Tournament, settings)
	if errors.Is(err, store.ErrBadScoring) {
		return PatchAdminTournament400JSONResponse{badRequest400(store.ErrBadScoring.Error())}, nil
	}
	if errors.Is(err, store.ErrBadLevelName) {
		return PatchAdminTournament400JSONResponse{badRequest400(store.ErrBadLevelName.Error())}, nil
	}
	if errors.Is(err, store.ErrBadPlayerCounts) {
		return PatchAdminTournament400JSONResponse{badRequest400("rated player counts are one or more of 2, 3 and 4")}, nil
	}
	if errors.Is(err, store.ErrBadKind) {
		return PatchAdminTournament400JSONResponse{badRequest400(store.ErrBadKind.Error())}, nil
	}
	if errors.Is(err, store.ErrGroupCycle) || errors.Is(err, store.ErrBadGroup) || errors.Is(err, store.ErrNotAGroup) {
		return PatchAdminTournament400JSONResponse{badRequest400(err.Error())}, nil
	}
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

func (s *server) GetAdminSeasonCounts(ctx context.Context, req GetAdminSeasonCountsRequestObject) (GetAdminSeasonCountsResponseObject, error) {
	counts, err := s.admins.SeasonCounts(ctx, req.Tournament)
	if errors.Is(err, store.ErrNoTournament) {
		return GetAdminSeasonCounts404JSONResponse{NotFoundJSONResponse{Body: Error{Error: "no such tournament"}, Headers: NotFoundResponseHeaders{CacheControl: cacheNone}}}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]SeasonCount, len(counts))
	for i, c := range counts {
		stages := make([]StageCount, len(c.Stages))
		for j, st := range c.Stages {
			stages[j] = StageCount{Stage: st.Stage, Number: st.Number, Games: st.Games, Finished: st.Finished, Players: st.Players}
		}
		out[i] = SeasonCount{Season: c.Season, Games: c.Games, Players: c.Players, Stages: stages}
	}
	return GetAdminSeasonCounts200JSONResponse{Body: out, Headers: GetAdminSeasonCounts200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PutAdminGroupMembers(ctx context.Context, req PutAdminGroupMembersRequestObject) (PutAdminGroupMembersResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	err := s.admins.SetMembers(ctx, req.Tournament, req.Body.Members)
	switch {
	case errors.Is(err, store.ErrNotAGroup), errors.Is(err, store.ErrGroupCycle):
		return PutAdminGroupMembers400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoTournament):
		return PutAdminGroupMembers404JSONResponse{NotFoundJSONResponse{Body: Error{Error: "no such tournament"}, Headers: NotFoundResponseHeaders{CacheControl: cacheNone}}}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.members", req.Tournament, req.Body); err != nil {
		return nil, err
	}
	return PutAdminGroupMembers204Response{}, nil
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
			Protected: r.Protected, Deleting: r.Deleting,
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

func (s *server) PutAdminPodium(ctx context.Context, req PutAdminPodiumRequestObject) (PutAdminPodiumResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	b := req.Body
	if b == nil || b.Season == "" || len(b.Season) > 64 || len(b.Note) > 500 || len(b.Podium) == 0 || len(b.Podium) > 3 {
		return PutAdminPodium400JSONResponse{badRequest400("a podium needs a season, 1st to 3rd, and a note under 500 characters")}, nil
	}
	podium := make([]store.PodiumEntry, len(b.Podium))
	for i, e := range b.Podium {
		if len(e.Name) > 64 {
			return PutAdminPodium400JSONResponse{badRequest400("a player's name is at most 64 characters")}, nil
		}
		podium[i] = store.PodiumEntry{Place: e.Place, Name: e.Name}
	}
	switch err := s.admins.SetPodium(ctx, req.Tournament, b.Season, podium, strings.TrimSpace(b.Note)); {
	case errors.Is(err, store.ErrNoTournament):
		return PutAdminPodium404JSONResponse{notFound404("no such tournament or season")}, nil
	case errors.Is(err, store.ErrBadPodium):
		return PutAdminPodium400JSONResponse{badRequest400("a podium is 1st to 3rd, each place and player once")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.podium", req.Tournament, b); err != nil {
		return nil, err
	}
	return PutAdminPodium204Response{}, nil
}

func (s *server) DeleteAdminPodium(ctx context.Context, req DeleteAdminPodiumRequestObject) (DeleteAdminPodiumResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	switch err := s.admins.ClearPodium(ctx, req.Tournament, req.Params.Season); {
	case errors.Is(err, store.ErrNoTournament):
		return DeleteAdminPodium404JSONResponse{notFound404("no such tournament or season")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.podium.clear", req.Tournament, map[string]string{"season": req.Params.Season}); err != nil {
		return nil, err
	}
	return DeleteAdminPodium204Response{}, nil
}

func (s *server) PutAdminRegulations(ctx context.Context, req PutAdminRegulationsRequestObject) (PutAdminRegulationsResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	if req.Body == nil || len(req.Body.Text) > 10000 {
		return PutAdminRegulations400JSONResponse{badRequest400("regulations are at most 10,000 characters")}, nil
	}
	switch err := s.admins.SetRegulations(ctx, req.Tournament, req.Body.Text); {
	case errors.Is(err, store.ErrNoTournament):
		return PutAdminRegulations404JSONResponse{notFound404("no such tournament")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.regulations", req.Tournament, req.Body); err != nil {
		return nil, err
	}
	return PutAdminRegulations204Response{}, nil
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
	switch err := s.admins.SetBadge(ctx, b); {
	case errors.Is(err, store.ErrNoBadge):
		return PutAdminBadge404JSONResponse{notFound404("no such badge")}, nil
	case errors.Is(err, store.ErrBadgeExists):
		return PutAdminBadge409JSONResponse{conflictBadge()}, nil
	case errors.Is(err, store.ErrBadBadge):
		return PutAdminBadge400JSONResponse{badRequest400("a badge's label is 1 to 16 characters")}, nil
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

func (s *server) PutAdminBadgePlayer(ctx context.Context, req PutAdminBadgePlayerRequestObject) (PutAdminBadgePlayerResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return PutAdminBadgePlayer403JSONResponse{forbidden(onlyAdminsBadge)}, err
	}
	switch err := s.admins.GiveBadge(ctx, req.Badge, req.Player); {
	case errors.Is(err, store.ErrNoBadge):
		return PutAdminBadgePlayer404JSONResponse{notFound404("no such badge")}, nil
	case errors.Is(err, store.ErrNoPlayer):
		return PutAdminBadgePlayer404JSONResponse{notFound404("no player has that name")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "badge.give", fmt.Sprint(req.Badge), map[string]string{"player": req.Player}); err != nil {
		return nil, err
	}
	return PutAdminBadgePlayer204Response{}, nil
}

func (s *server) DeleteAdminBadgePlayer(ctx context.Context, req DeleteAdminBadgePlayerRequestObject) (DeleteAdminBadgePlayerResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return DeleteAdminBadgePlayer403JSONResponse{forbidden(onlyAdminsBadge)}, err
	}
	switch err := s.admins.TakeBadge(ctx, req.Badge, req.Player); {
	case errors.Is(err, store.ErrNoBadge):
		return DeleteAdminBadgePlayer404JSONResponse{notFound404("the player does not hold that badge")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "badge.take", fmt.Sprint(req.Badge), map[string]string{"player": req.Player}); err != nil {
		return nil, err
	}
	return DeleteAdminBadgePlayer204Response{}, nil
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

func (s *server) PutAdminCard(ctx context.Context, req PutAdminCardRequestObject) (PutAdminCardResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	const tooLong = "a card's icon is at most 8 characters, its description, subtitle and cadence 60 and its information box 1,000"
	b := req.Body
	if b == nil {
		return PutAdminCard400JSONResponse{badRequest400(tooLong)}, nil
	}
	switch err := s.admins.SetCard(ctx, req.Tournament, store.Card{Icon: b.Icon, Tagline: b.Tagline, Summary: b.Summary, Subtitle: b.Subtitle, Cadence: b.Cadence}); {
	case errors.Is(err, store.ErrNoTournament):
		return PutAdminCard404JSONResponse{notFound404("no such tournament")}, nil
	case errors.Is(err, store.ErrBadCard):
		return PutAdminCard400JSONResponse{badRequest400(tooLong)}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.card", req.Tournament, b); err != nil {
		return nil, err
	}
	return PutAdminCard204Response{}, nil
}

// storyList turns a tournament's stories into the API's.
func storyList(list []store.Story) []Story {
	out := make([]Story, len(list))
	for i, st := range list {
		out[i] = Story{Player: st.Player, Badge: st.Badge, Title: st.Title, Text: st.Text}
	}
	return out
}

func (s *server) PutAdminStories(ctx context.Context, req PutAdminStoriesRequestObject) (PutAdminStoriesResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	const bad = "at most 12 stories, each with a player, a title (120 characters) and the story (1,000), a badge of up to 60"
	if req.Body == nil || len(req.Body.Stories) > 12 {
		return PutAdminStories400JSONResponse{badRequest400(bad)}, nil
	}
	list := make([]store.Story, len(req.Body.Stories))
	for i, st := range req.Body.Stories {
		list[i] = store.Story{Player: st.Player, Badge: st.Badge, Title: st.Title, Text: st.Text}
	}
	switch err := s.admins.SetStories(ctx, req.Tournament, list); {
	case errors.Is(err, store.ErrNoTournament):
		return PutAdminStories404JSONResponse{notFound404("no such tournament")}, nil
	case errors.Is(err, store.ErrBadStory):
		return PutAdminStories400JSONResponse{badRequest400(bad)}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.stories", req.Tournament, req.Body); err != nil {
		return nil, err
	}
	return PutAdminStories204Response{}, nil
}

func (s *server) PutAdminSeasonRating(ctx context.Context, req PutAdminSeasonRatingRequestObject) (PutAdminSeasonRatingResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	b := req.Body
	if b == nil || b.Season == "" || len(b.Season) > 64 {
		return PutAdminSeasonRating400JSONResponse{badRequest400("a season, and whether it is rated")}, nil
	}
	switch err := s.admins.SetSeasonRated(ctx, req.Tournament, b.Season, b.Rated); {
	case errors.Is(err, store.ErrNoTournament):
		return PutAdminSeasonRating404JSONResponse{notFound404("no such tournament or season")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.season.rated", req.Tournament, b); err != nil {
		return nil, err
	}
	return PutAdminSeasonRating204Response{}, nil
}

func (s *server) PutAdminSeasonPenalty(ctx context.Context, req PutAdminSeasonPenaltyRequestObject) (PutAdminSeasonPenaltyResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	b := req.Body
	if b == nil || b.Season == "" || len(b.Season) > 64 {
		return PutAdminSeasonPenalty400JSONResponse{badRequest400("a season, and its time-out penalty")}, nil
	}
	switch err := s.admins.SetSeasonPenalty(ctx, req.Tournament, b.Season, b.TimeoutPenalty); {
	case errors.Is(err, store.ErrBadPenalty):
		return PutAdminSeasonPenalty400JSONResponse{badRequest400(store.ErrBadPenalty.Error())}, nil
	case errors.Is(err, store.ErrNoTournament):
		return PutAdminSeasonPenalty404JSONResponse{notFound404("no such tournament or season")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.season.penalty", req.Tournament, b); err != nil {
		return nil, err
	}
	return PutAdminSeasonPenalty204Response{}, nil
}

func (s *server) PutAdminSeasonChampions(ctx context.Context, req PutAdminSeasonChampionsRequestObject) (PutAdminSeasonChampionsResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	b := req.Body
	if b == nil || b.Season == "" || len(b.Season) > 64 {
		return PutAdminSeasonChampions400JSONResponse{badRequest400("a season, and whether its winner is among the champions")}, nil
	}
	switch err := s.admins.SetSeasonInChampions(ctx, req.Tournament, b.Season, b.InChampions); {
	case errors.Is(err, store.ErrNoTournament):
		return PutAdminSeasonChampions404JSONResponse{notFound404("no such tournament or season")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.season.champions", req.Tournament, b); err != nil {
		return nil, err
	}
	return PutAdminSeasonChampions204Response{}, nil
}

func (s *server) PutAdminSeasonCommunity(ctx context.Context, req PutAdminSeasonCommunityRequestObject) (PutAdminSeasonCommunityResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	b := req.Body
	if b == nil || b.Season == "" || len(b.Season) > 64 {
		return PutAdminSeasonCommunity400JSONResponse{badRequest400("a season, and whether it counts in the community leaderboard")}, nil
	}
	switch err := s.admins.SetSeasonInCommunity(ctx, req.Tournament, b.Season, b.InCommunity); {
	case errors.Is(err, store.ErrNoTournament):
		return PutAdminSeasonCommunity404JSONResponse{notFound404("no such tournament or season")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.season.community", req.Tournament, b); err != nil {
		return nil, err
	}
	return PutAdminSeasonCommunity204Response{}, nil
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

func (s *server) PutAdminRunProtected(ctx context.Context, req PutAdminRunProtectedRequestObject) (PutAdminRunProtectedResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil || !ok {
		return PutAdminRunProtected403JSONResponse{forbidden(onlyAdminsRun)}, err
	}
	if req.Body == nil {
		return PutAdminRunProtected400JSONResponse{badRequest400("say whether the run is protected")}, nil
	}
	switch err := s.admins.SetRunProtected(ctx, req.Run, req.Body.Protected); {
	case errors.Is(err, store.ErrRunNotFound):
		return PutAdminRunProtected404JSONResponse{notFound404("no such run")}, nil
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
	return PutAdminRunProtected204Response{}, nil
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

func (s *server) GetAdminRunSizes(ctx context.Context, _ GetAdminRunSizesRequestObject) (GetAdminRunSizesResponseObject, error) {
	if _, ok, err := adminOnly(ctx); err != nil || !ok {
		return GetAdminRunSizes403JSONResponse{forbidden(onlyAdminsRun)}, err
	}
	sizes, err := s.admins.RunSizes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RunSize, 0, len(sizes))
	for run, bytes := range sizes {
		out = append(out, RunSize{Run: run, Bytes: bytes})
	}
	slices.SortFunc(out, func(a, b RunSize) int { return cmp.Compare(a.Run, b.Run) })
	return GetAdminRunSizes200JSONResponse{Body: out, Headers: GetAdminRunSizes200ResponseHeaders{CacheControl: cacheNone}}, nil
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
