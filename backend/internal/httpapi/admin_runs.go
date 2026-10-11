// Admin's rating runs: the list, starting and stopping the engine, publishing, comparing, protecting and deleting a run.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ttacommunity/backend/internal/enginejob"
	"ttacommunity/backend/internal/store"
)

const onlyAdminsRun = "only admins run the engine"

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
