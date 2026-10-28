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

// Cache lifetimes, at Cloudflare and in browsers. A new run is new URLs, so nothing is
// purged; a run's answers last five minutes as they include player details edited between
// runs, and which run is current lasts a minute (docs/architecture.md#data-flow-for-ratings).
const (
	cacheRun     = "public, max-age=300"
	cacheCurrent = "public, max-age=60"
	cacheDay     = "public, max-age=86400"
	cacheNone    = "no-store"
)

// Requests records each answered request: its route pattern, status code and how long it took.
// Status 0 is an answer aborted after it started.
type Requests interface {
	Observe(route string, status int, took time.Duration)
}

// NewMux returns the API's handler with every route in the spec registered. Admin routes go through
// the admin gate, without CORS; with admins nil they refuse every request. Sign-in routes go through
// their own gate, with CORS for the site alone; with signIns nil they refuse every request. The
// Discord bot's endpoint takes only requests Discord signed, without CORS; with discord nil it is a 404.
// With visits nil the visit count is a 404. A panic answers 500. Every response carries an X-Request-Id,
// which a 500's body and log line repeat. Each request is reported to requests, timed by now, and logged
// when slower than slowRequest.
func NewMux(ratings Ratings, admins Admins, signIns SignIns, discord DiscordApp, visits Visits, log *slog.Logger, requests Requests, now func() time.Time) http.Handler {
	mux := http.NewServeMux()
	registerRoutes(mux, ratings, admins, signIns, discord, visits, log, now)
	// limitBodies needs the server's own writer, so it sits outside observe's.
	public := routeDiscord(routeSignIns(publicCORS(mux), signInGate(mux, signIns, now, log)), discordGate(mux, discord, log))
	routed := recoverPanics(routeAdmin(public, adminGate(mux, admins, log)), mux, log)
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

func registerRoutes(router ServeMux, ratings Ratings, admins Admins, signIns SignIns, discord DiscordApp, visits Visits, log *slog.Logger, now func() time.Time) {
	srv := &server{ratings: ratings, admins: admins, signIns: signIns, discord: discord, visits: visits, log: log, agreement: runcache.New[Format, analysis.Agreement](), now: now}
	strict := NewStrictHandlerWithOptions(srv, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  badRequest,
		ResponseErrorHandlerFunc: internalError(log),
	})
	HandlerWithOptions(strict, StdHTTPServerOptions{BaseRouter: router, ErrorHandlerFunc: badRequest})
}

// publicCORS lets any site read the public API, without credentials. The sign-in
// routes have their own gate, which allows the site alone, with credentials.
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
				// The value's type only: a database error's text can carry its host and user.
				log.Error("handler panicked", "route", routeOf(mux, r), "request_id", requestID(r.Context()), "panic", fmt.Sprintf("%T", v), "stack", string(debug.Stack()))
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
	signIns   SignIns
	discord   DiscordApp
	visits    Visits
	log       *slog.Logger
	agreement *runcache.Cache[Format, analysis.Agreement]
	// now is the clock, for what depends on the time of the request (a time zone's offset today).
	now func() time.Time
}

// GetHealthz reports that the process is up. It checks no dependency, so a
// database outage does not get the pod restarted.
func (s *server) GetHealthz(context.Context, GetHealthzRequestObject) (GetHealthzResponseObject, error) {
	return GetHealthz200JSONResponse{Body: Health{Status: "ok"}, Headers: GetHealthz200ResponseHeaders{CacheControl: cacheNone}}, nil
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
	date := func(t *time.Time) *openapi_types.Date {
		if t == nil {
			return nil
		}
		return &openapi_types.Date{Time: *t}
	}
	return GetCurrentRun200JSONResponse{
		Body: Run{RunId: run.ID, PublishedAt: run.PublishedAt.UTC(), EngineVersion: run.EngineVersion, FinishedAt: inUTC(run.FinishedAt),
			Games: run.Games, Players: run.Players, FirstGame: date(run.FirstGame), LastGame: date(run.LastGame)},
		Headers: GetCurrentRun200ResponseHeaders{CacheControl: cacheCurrent},
	}, nil
}
