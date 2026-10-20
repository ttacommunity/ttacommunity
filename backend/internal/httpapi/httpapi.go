// Package httpapi holds the API's routes and handlers. The routes and types
// are generated from api/openapi.yaml; the handlers are here.
package httpapi

//go:generate go tool oapi-codegen -config ../../api/oapi-codegen.yaml ../../api/openapi.yaml

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"ttacommunity/backend/internal/analysis"
	"ttacommunity/backend/internal/runcache"
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

// Ratings is what the public ratings routes read: each route file's reads, together.
type Ratings interface {
	RunReads
	BoardReads
	PlayerReads
	YearReads
	AnalysisReads
	TournamentReads
	CatalogueReads
	BlitzReads
}

// BlitzReads are the site's blitz tournament reads: the shown editions, and one with its active entrants.
type BlitzReads interface {
	BlitzEditions(ctx context.Context) ([]store.BlitzEditionRow, error)
	BlitzEdition(ctx context.Context, slug, edition string) (store.BlitzEdition, error)
}

// RunReads are the reads every area shares: whether the database answers, the run the site shows, a run's last game.
type RunReads interface {
	Ping(ctx context.Context) error
	CurrentRun(ctx context.Context) (store.PublishedRun, error)
	RunLastGame(ctx context.Context, run int64) (*time.Time, error)
}

// BoardReads are the leaderboard's reads, here and in csv.go.
type BoardReads interface {
	Leaderboard(ctx context.Context, run int64, model string, format int) ([]store.LeaderboardRow, error)
	History(ctx context.Context, run int64, model string, format int, at *time.Time) (time.Time, []store.HistoryRow, error)
}

// PlayerReads are player.go's reads.
type PlayerReads interface {
	Player(ctx context.Context, run int64, name string) (store.Player, error)
	PlayerHistory(ctx context.Context, run int64, name string, format int) (string, []store.HistoryPoint, error)
	PlayerMatches(ctx context.Context, run int64, name string, format, page int) (string, store.MatchPage, error)
	RatingsOf(ctx context.Context, run int64, model string, format int, names []string) (map[string]store.Rated, error)
	HiddenNames(ctx context.Context, names []string) (map[string]bool, error)
	PlayerRivals(ctx context.Context, run int64, name string, format int) (string, []store.Rival, error)
	PlayerCareer(ctx context.Context, run int64, name string, format int, model string) (string, []store.CareerYear, *store.CareerPeak, error)
	Versus(ctx context.Context, run int64, name, other string) (string, string, []store.PlayerMatch, error)
	PlayerMatrix(ctx context.Context, run int64, name string, format int, year *int) (string, []store.MatrixCell, error)
	PlayerSeats(ctx context.Context, run int64, name string, unrated bool) (store.PlayerSeatStats, error)
}

// YearReads are year.go's reads, and csv.go's for one year.
type YearReads interface {
	YearBoard(ctx context.Context, run int64, year int, model string, format int) (store.YearBoard, error)
	Years(ctx context.Context, run int64) ([]store.Year, error)
}

// AnalysisReads are analysis.go's reads.
type AnalysisReads interface {
	Calibration(ctx context.Context, run int64) ([]store.CalibrationRow, error)
	CheckPublished(ctx context.Context, run int64) error
	AnalysisRatings(ctx context.Context, run int64, format int) ([]store.ModelRating, error)
	ActivePlayers(ctx context.Context, run int64) (map[string]bool, error)
	ComparedPlayers(ctx context.Context, run int64, format int, terms []string) ([]store.ComparedPlayer, error)
	Activity(ctx context.Context, run int64, inactiveOnOrBefore time.Time) (store.Activity, error)
	SeatAnalysis(ctx context.Context, run int64, format int, tier string) ([]store.SeatStatsGroup, error)
}

// TournamentReads are tournament.go's reads.
type TournamentReads interface {
	TournamentSeries(ctx context.Context, run int64, series string) (store.TournamentSeries, error)
	HallOfFame(ctx context.Context, run int64) (store.HallOfFame, error)
	PlayerAchievements(ctx context.Context, run int64, name string) (string, *store.AchievementSummary, []store.TournamentRecord, error)
	CommunityLeaderboard(ctx context.Context, run int64, edition string) ([]store.CommunityRow, error)
	CommunityLeaderboardLayout(ctx context.Context, run int64, slug string) (store.CommunityEditionLayout, error)
	CommunityEditions(ctx context.Context) ([]store.CommunityEdition, error)
}

// CatalogueReads are catalogue.go's reads.
type CatalogueReads interface {
	Catalogue(ctx context.Context, run int64) (store.CatalogueView, error)
	CatalogueTournament(ctx context.Context, run int64, slug string) (store.CatalogueTournament, error)
	CatalogueGames(ctx context.Context, run int64, slug string, f store.CatalogueGamesFilter) (store.CatalogueGames, error)
	RecentPodiums(ctx context.Context, run int64, played, shown int) ([]store.RecentPodium, error)
}

// Requests records each answered request: its route pattern, status code and how long it took.
// Status 0 is an answer aborted after it started.
type Requests interface {
	Observe(route string, status int, took time.Duration)
}

// NewMux returns the API's handler with every route in the spec registered. Admin routes go through
// the admin gate, without CORS; with admins nil they refuse every request. A panic answers 500.
// Every response carries an X-Request-Id, which a 500's body and log line repeat. Each request is
// reported to requests, timed by now, and logged when slower than slowRequest.
func NewMux(ratings Ratings, admins Admins, log *slog.Logger, requests Requests, now func() time.Time) http.Handler {
	mux := http.NewServeMux()
	registerRoutes(mux, ratings, admins, log, now)
	// limitBodies needs the server's own writer, so it sits outside observe's.
	routed := recoverPanics(routeAdmin(publicCORS(mux), adminGate(mux, admins, log)), mux, log)
	return withRequestID(withDeadline(limitBodies(observe(routed, mux, requests, now, log)), requestDeadline))
}

// slowRequest is how long an answer may take before it is logged as slow.
const slowRequest = time.Second

// observe reports each request to requests by the route mux matches and its status (0 when the
// answer was aborted), and logs one that took slowRequest or longer.
func observe(next http.Handler, mux *http.ServeMux, requests Requests, now func() time.Time, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		aborted := true
		defer func() {
			took := now().Sub(start)
			route := routeOf(mux, r)
			status := sw.status
			if aborted {
				status = 0
			}
			requests.Observe(route, status, took)
			if took >= slowRequest {
				log.Warn("slow request", "route", route, "status", status, "took_ms", took.Milliseconds(), "request_id", requestID(r.Context()))
			}
		}()
		next.ServeHTTP(sw, r)
		aborted = false
	})
}

// statusWriter remembers the status code a handler answers with: 200 unless it writes another first.
type statusWriter struct {
	http.ResponseWriter
	status  int
	written bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.written {
		w.status, w.written = code, true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.written = true
	return w.ResponseWriter.Write(b)
}

// started reports whether the answer's status has gone out, after which it cannot change.
func (w *statusWriter) started() bool {
	return w.written
}

// Unwrap lets http.ResponseController reach the server's writer.
func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func registerRoutes(router ServeMux, ratings Ratings, admins Admins, log *slog.Logger, now func() time.Time) {
	strict := NewStrictHandlerWithOptions(&server{ratings: ratings, admins: admins, log: log, agreement: runcache.New[Format, analysis.Agreement](), now: now}, nil, StrictHTTPServerOptions{
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

// recoverPanics answers a handler's panic with a 500 and logs it by the route mux matches, with its
// stack. Left to net/http, the panic is logged as plain text with the client's IP.
// http.ErrAbortHandler is re-panicked, so net/http still aborts the response.
func recoverPanics(next http.Handler, mux *http.ServeMux, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == http.ErrAbortHandler {
				panic(v)
			}
			if v != nil {
				log.Error("handler panicked", "route", routeOf(mux, r), "request_id", requestID(r.Context()), "panic", fmt.Sprint(v), "stack", string(debug.Stack()))
				serverError(w, r)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// routeOf returns the pattern mux matches r with, or "unrouted" when none does (an unknown path or
// method). Unlike r.Pattern, it holds outside the mux and on a request a middleware has copied.
func routeOf(mux *http.ServeMux, r *http.Request) string {
	if _, route := mux.Handler(r); route != "" {
		return route
	}
	return "unrouted"
}

type requestIDKey struct{}

// withRequestID gives each request a random id, in its context and the X-Request-Id response header.
// A client's own X-Request-Id is ignored: it would put text anyone chooses in the log.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 8)
		_, _ = rand.Read(b) // never fails: crypto/rand crashes the program instead
		id := hex.EncodeToString(b)
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

// requestID returns the id withRequestID gave the request, or "" without one.
func requestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// requestDeadline cancels a request's work, its queries included. It is under
// the server's WriteTimeout, so a handler sees the cancel while it can still answer.
const requestDeadline = 25 * time.Second

// withDeadline gives each request's context a deadline d away, or keeps an
// earlier one.
func withDeadline(next http.Handler, d time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// maxBody is the most any request body may hold. The largest body a route
// takes, a tournament's settings, is tens of kilobytes at most.
const maxBody = 1 << 20

// limitBodies stops reading a request body past maxBody. GETs carry no body,
// so they are limited too rather than branched on.
func limitBodies(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		next.ServeHTTP(w, r)
	})
}

// badRequest answers a parameter the generated code could not parse, naming
// the parameter but never echoing its value.
func badRequest(w http.ResponseWriter, _ *http.Request, err error) {
	msg := "bad request"
	var invalid *InvalidParamFormatError
	var required *RequiredParamError
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
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
		log.Error("request failed", "route", r.Pattern, "request_id", requestID(r.Context()), "err", err)
		serverError(w, r)
	}
}

// serverError answers 500 with a fixed message and the request's id, which the caller logs. An answer
// already started is aborted instead: an error written into it would end it as if it were whole.
func serverError(w http.ResponseWriter, r *http.Request) {
	if s, ok := w.(interface{ started() bool }); ok && s.started() {
		panic(http.ErrAbortHandler)
	}
	body := Error{Error: "internal error"}
	if id := requestID(r.Context()); id != "" {
		body.RequestId = &id
	}
	writeErrorBody(w, http.StatusInternalServerError, body)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeErrorBody(w, code, Error{Error: msg})
}

func writeErrorBody(w http.ResponseWriter, code int, body Error) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", cacheNone)
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

type server struct {
	ratings   Ratings
	admins    Admins
	log       *slog.Logger
	agreement *runcache.Cache[Format, analysis.Agreement]
	// now is the clock, for what depends on the time of the request (a time zone's offset today).
	now func() time.Time
}

// GetHealthz reports that the process is up. It checks no dependency, so a
// database outage does not get the pod restarted.
func (s *server) GetHealthz(context.Context, GetHealthzRequestObject) (GetHealthzResponseObject, error) {
	return GetHealthz200JSONResponse{Status: "ok"}, nil
}

// robotsTxt keeps crawlers off the player routes, which every crawled player
// page calls several times; the site's pages stay crawlable.
const robotsTxt = "User-agent: *\nDisallow: /v1/runs/*/players/\n"

// GetRobotsTxt answers the rules for search crawlers.
func (s *server) GetRobotsTxt(context.Context, GetRobotsTxtRequestObject) (GetRobotsTxtResponseObject, error) {
	return GetRobotsTxt200TextResponse{Body: robotsTxt, Headers: GetRobotsTxt200ResponseHeaders{CacheControl: cacheRun}}, nil
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
	utc := func(t *time.Time) *time.Time {
		if t == nil {
			return nil
		}
		u := t.UTC()
		return &u
	}
	date := func(t *time.Time) *openapi_types.Date {
		if t == nil {
			return nil
		}
		return &openapi_types.Date{Time: *t}
	}
	return GetCurrentRun200JSONResponse{
		Body: Run{RunId: run.ID, PublishedAt: run.PublishedAt.UTC(), EngineVersion: run.EngineVersion, FinishedAt: utc(run.FinishedAt),
			Games: run.Games, Players: run.Players, FirstGame: date(run.FirstGame), LastGame: date(run.LastGame)},
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
		row.Badges = extraBadges(p.Badges)
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

// extraBadges turns a player's badges into the API's, or nil for none, so the field is left out.
func extraBadges(bs []store.ExtraBadge) *[]ExtraBadge {
	if len(bs) == 0 {
		return nil
	}
	out := make([]ExtraBadge, len(bs))
	for i, b := range bs {
		out[i] = ExtraBadge{Label: b.Label, Description: b.Description}
	}
	return &out
}
