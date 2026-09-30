// Package httpapi holds the API's routes and handlers. The routes and types
// are generated from api/openapi.yaml; the handlers are here.
package httpapi

//go:generate go tool oapi-codegen -config ../../api/oapi-codegen.yaml ../../api/openapi.yaml

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"ttacommunity/backend/internal/store"
)

// Cache lifetimes. A run's answers are cached for five minutes while the site
// is in testing, so an answer whose shape changes never outlives a deploy by
// long. Which run is current is cached for a minute.
const (
	cacheRun     = "public, max-age=300"
	cacheCurrent = "public, max-age=60"
	cacheNone    = "no-store"
)

// Ratings is what the public ratings routes read.
type Ratings interface {
	Ping(ctx context.Context) error
	CurrentRun(ctx context.Context) (store.PublishedRun, error)
	Leaderboard(ctx context.Context, run int64, model string, format int) ([]store.LeaderboardRow, error)
	History(ctx context.Context, run int64, model string, format int, at *time.Time) (time.Time, []store.HistoryRow, error)
	Player(ctx context.Context, run int64, name string) (store.Player, error)
	PlayerHistory(ctx context.Context, run int64, name string, format int) (string, []store.HistoryPoint, error)
	PlayerMatches(ctx context.Context, run int64, name string, format, page int) (string, store.MatchPage, error)
	RatingsOf(ctx context.Context, run int64, model string, format int, names []string) (map[string]store.Rated, error)
	PlayerRivals(ctx context.Context, run int64, name string, format int) (string, []store.Rival, error)
	PlayerCareer(ctx context.Context, run int64, name string, format int) (string, []store.CareerYear, []store.ModelPeak, error)
	Versus(ctx context.Context, run int64, name, other string) (string, string, []store.PlayerMatch, error)
	PlayerMatrix(ctx context.Context, run int64, name string, format int, year *int) (string, []store.MatrixCell, error)
	YearBoard(ctx context.Context, run int64, year int, model string, format int) (store.YearBoard, error)
	Years(ctx context.Context, run int64) ([]store.Year, error)
	Calibration(ctx context.Context, run int64) ([]store.CalibrationRow, error)
	AnalysisRatings(ctx context.Context, run int64, format int) ([]store.ModelRating, map[string]bool, error)
	ComparedPlayers(ctx context.Context, run int64, format int, terms []string) ([]store.ComparedPlayer, error)
	Activity(ctx context.Context, run int64, inactiveOnOrBefore time.Time) (store.Activity, error)
	TournamentSeries(ctx context.Context, run int64, series string) (store.TournamentSeries, error)
	HallOfFame(ctx context.Context, run int64) (store.HallOfFame, error)
	PlayerAchievements(ctx context.Context, run int64, name string) (string, *store.AchievementSummary, []store.TournamentRecord, error)
	CommunityLeaderboard(ctx context.Context, run int64, edition string) ([]store.CommunityRow, error)
}

// NewMux returns the API's handler with every route in the spec registered.
func NewMux(ratings Ratings, log *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	registerRoutes(mux, ratings, log)
	return publicCORS(mux)
}

func registerRoutes(router ServeMux, ratings Ratings, log *slog.Logger) {
	strict := NewStrictHandlerWithOptions(&server{ratings: ratings, log: log}, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  badRequest,
		ResponseErrorHandlerFunc: internalError(log),
	})
	HandlerWithOptions(strict, StdHTTPServerOptions{BaseRouter: router, ErrorHandlerFunc: badRequest})
}

// publicCORS lets any site read the API. No route accepts credentials yet;
// signed-in routes will need an exact origin allowlist instead.
func publicCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		next.ServeHTTP(w, r)
	})
}

// badRequest answers a parameter the generated code could not parse, naming
// the parameter but never echoing its value.
func badRequest(w http.ResponseWriter, _ *http.Request, err error) {
	msg := "bad request"
	var invalid *InvalidParamFormatError
	var required *RequiredParamError
	switch {
	case errors.As(err, &invalid):
		msg = "bad parameter: " + invalid.ParamName
	case errors.As(err, &required):
		msg = "missing parameter: " + required.ParamName
	}
	writeError(w, http.StatusBadRequest, msg)
}

// internalError logs what failed, by route, and answers with a fixed message:
// a database error can carry hosts and user names.
func internalError(log *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		log.Error("request failed", "route", r.Pattern, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", cacheNone)
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(Error{Error: msg})
}

type server struct {
	ratings Ratings
	log     *slog.Logger
}

// GetHealthz reports that the process is up. It checks no dependency, so a
// database outage does not get the pod restarted.
func (s *server) GetHealthz(context.Context, GetHealthzRequestObject) (GetHealthzResponseObject, error) {
	return GetHealthz200JSONResponse{Status: "ok"}, nil
}

// GetReadyz reports whether the ratings database answers, without saying why
// it does not.
func (s *server) GetReadyz(ctx context.Context, _ GetReadyzRequestObject) (GetReadyzResponseObject, error) {
	if err := s.ratings.Ping(ctx); err != nil {
		s.log.Warn("not ready", "err", err)
		return GetReadyz503JSONResponse{
			Body:    Error{Error: "not ready"},
			Headers: GetReadyz503ResponseHeaders{CacheControl: cacheNone},
		}, nil
	}
	return GetReadyz200JSONResponse{
		Body:    Health{Status: "ok"},
		Headers: GetReadyz200ResponseHeaders{CacheControl: cacheNone},
	}, nil
}

// GetCurrentRun returns the published run the site shows now.
func (s *server) GetCurrentRun(ctx context.Context, _ GetCurrentRunRequestObject) (GetCurrentRunResponseObject, error) {
	run, err := s.ratings.CurrentRun(ctx)
	if errors.Is(err, store.ErrNoPublishedRun) {
		return GetCurrentRun503JSONResponse{NoPublishedRunJSONResponse{
			Body:    Error{Error: err.Error()},
			Headers: NoPublishedRunResponseHeaders{CacheControl: cacheNone},
		}}, nil
	}
	if err != nil {
		return nil, err
	}
	return GetCurrentRun200JSONResponse{
		Body:    Run{RunId: run.ID, PublishedAt: run.PublishedAt.UTC()},
		Headers: GetCurrentRun200ResponseHeaders{CacheControl: cacheCurrent},
	}, nil
}

// GetLeaderboard returns one published run's leaderboard for a model and
// format, refusing unknown values before they reach the database.
func (s *server) GetLeaderboard(ctx context.Context, req GetLeaderboardRequestObject) (GetLeaderboardResponseObject, error) {
	model, format := Glicko2Daneo, N0
	if req.Params.Model != nil {
		model = *req.Params.Model
	}
	if req.Params.Format != nil {
		format = *req.Params.Format
	}
	switch {
	case req.Run < 1:
		return leaderboardRefused("bad parameter: run"), nil
	case !model.Valid():
		return leaderboardRefused("unknown model"), nil
	case !format.Valid():
		return leaderboardRefused("unknown format"), nil
	}

	rows, err := s.ratings.Leaderboard(ctx, req.Run, string(model), int(format))
	if errors.Is(err, store.ErrRunNotFound) {
		return GetLeaderboard404JSONResponse{NotFoundJSONResponse{
			Body:    Error{Error: err.Error()},
			Headers: NotFoundResponseHeaders{CacheControl: cacheNone},
		}}, nil
	}
	if err != nil {
		return nil, err
	}

	players := make([]LeaderboardRow, 0, len(rows))
	for _, p := range rows {
		row := LeaderboardRow{
			Rank: p.Rank, RankDelta: p.RankDelta, Name: p.Name, Rating: p.Rating, Rd: p.RD, CRating: p.CRating,
			Games: p.Games, Opponents: p.Opponents, Wins: p.Wins, Losses: p.Losses, Draws: p.Draws, WinRate: p.WinRate,
			Country: p.Country,
		}
		if p.Title != nil {
			row.Title = &Title{Code: TitleCode(p.Title.Code), Count: p.Title.Count, Reason: p.Title.Reason}
		}
		if p.LastPlayed != nil {
			row.LastPlayed = &openapi_types.Date{Time: *p.LastPlayed}
		}
		players = append(players, row)
	}
	return GetLeaderboard200JSONResponse{
		Body:    Leaderboard{RunId: req.Run, Model: model, Format: format, Players: players},
		Headers: GetLeaderboard200ResponseHeaders{CacheControl: cacheRun},
	}, nil
}

// GetHistory returns each player's rating at a past date, or at the run's
// previous period when no date is given.
func (s *server) GetHistory(ctx context.Context, req GetHistoryRequestObject) (GetHistoryResponseObject, error) {
	model, format := Glicko2Daneo, N0
	if req.Params.Model != nil {
		model = *req.Params.Model
	}
	if req.Params.Format != nil {
		format = *req.Params.Format
	}
	refused := func(msg string) GetHistoryResponseObject {
		return GetHistory400JSONResponse{BadRequestJSONResponse{
			Body:    Error{Error: msg},
			Headers: BadRequestResponseHeaders{CacheControl: cacheNone},
		}}
	}
	switch {
	case req.Run < 1:
		return refused("bad parameter: run"), nil
	case !model.Valid():
		return refused("unknown model"), nil
	case !format.Valid():
		return refused("unknown format"), nil
	}
	var at *time.Time
	if req.Params.At != nil {
		at = &req.Params.At.Time
	}

	on, rows, err := s.ratings.History(ctx, req.Run, string(model), int(format), at)
	if errors.Is(err, store.ErrRunNotFound) || errors.Is(err, store.ErrNoHistory) {
		return GetHistory404JSONResponse{NotFoundJSONResponse{
			Body:    Error{Error: err.Error()},
			Headers: NotFoundResponseHeaders{CacheControl: cacheNone},
		}}, nil
	}
	if err != nil {
		return nil, err
	}
	players := make([]HistoryRow, 0, len(rows))
	for _, h := range rows {
		players = append(players, HistoryRow{Name: h.Name, Rating: h.Rating, Rd: h.RD, CRating: h.CRating})
	}
	return GetHistory200JSONResponse{
		Body:    History{RunId: req.Run, Model: model, Format: format, At: openapi_types.Date{Time: on}, Players: players},
		Headers: GetHistory200ResponseHeaders{CacheControl: cacheRun},
	}, nil
}

func leaderboardRefused(msg string) GetLeaderboardResponseObject {
	return GetLeaderboard400JSONResponse{BadRequestJSONResponse{
		Body:    Error{Error: msg},
		Headers: BadRequestResponseHeaders{CacheControl: cacheNone},
	}}
}
