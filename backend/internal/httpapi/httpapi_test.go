package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"ttacommunity/backend/internal/store"
)

type fakeRatings struct {
	run      store.PublishedRun
	runErr   error
	board    []store.LeaderboardRow
	boardErr error
	pingErr  error

	calls  int
	gotRun int64
	gotMod string
	gotFmt int
}

func (f *fakeRatings) Ping(context.Context) error {
	f.calls++
	return f.pingErr
}

func (f *fakeRatings) CurrentRun(context.Context) (store.PublishedRun, error) {
	f.calls++
	return f.run, f.runErr
}

func (f *fakeRatings) Leaderboard(_ context.Context, run int64, model string, format int) ([]store.LeaderboardRow, error) {
	f.calls++
	f.gotRun, f.gotMod, f.gotFmt = run, model, format
	return f.board, f.boardErr
}

func serve(t *testing.T, ratings Ratings, path string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	rec := httptest.NewRecorder()
	NewMux(ratings, log).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec, logs.String()
}

type recordingRouter struct {
	*http.ServeMux
	patterns []string
}

func (r *recordingRouter) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	r.patterns = append(r.patterns, pattern)
	r.ServeMux.HandleFunc(pattern, h)
}

func TestRoutesMatchTheSpec(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("reading the spec: %v", err)
	}
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parsing the spec: %v", err)
	}
	var want []string
	for path, ops := range spec.Paths {
		for method := range ops {
			want = append(want, strings.ToUpper(method)+" "+path)
		}
	}
	router := &recordingRouter{ServeMux: http.NewServeMux()}
	registerRoutes(router, &fakeRatings{}, slog.New(slog.DiscardHandler))

	slices.Sort(want)
	slices.Sort(router.patterns)
	if !slices.Equal(router.patterns, want) {
		t.Errorf("routes = %q, want the spec's %q", router.patterns, want)
	}
}

func TestHealthz(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		wantCode int
		wantBody string
	}{
		{name: "get is ok", method: http.MethodGet, path: "/healthz", wantCode: http.StatusOK, wantBody: `{"status":"ok"}` + "\n"},
		{name: "post is refused", method: http.MethodPost, path: "/healthz", wantCode: http.StatusMethodNotAllowed},
		{name: "unknown path is not found", method: http.MethodGet, path: "/nope", wantCode: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratings := &fakeRatings{pingErr: errors.New("database down")}
			rec := httptest.NewRecorder()
			NewMux(ratings, slog.New(slog.DiscardHandler)).ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))

			if rec.Code != tt.wantCode {
				t.Errorf("%s %s: code = %d, want %d", tt.method, tt.path, rec.Code, tt.wantCode)
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("%s %s: body = %q, want %q", tt.method, tt.path, rec.Body.String(), tt.wantBody)
			}
			if tt.wantCode == http.StatusOK && rec.Header().Get("Content-Type") != "application/json" {
				t.Errorf("%s %s: Content-Type = %q, want %q", tt.method, tt.path, rec.Header().Get("Content-Type"), "application/json")
			}
			if ratings.calls != 0 {
				t.Errorf("%s %s: touched the database %d times, want 0", tt.method, tt.path, ratings.calls)
			}
		})
	}
}

func TestReadyz(t *testing.T) {
	tests := []struct {
		name     string
		pingErr  error
		wantCode int
		wantBody string
	}{
		{name: "database answers", wantCode: http.StatusOK, wantBody: `{"status":"ok"}` + "\n"},
		{name: "database down", pingErr: errors.New("dial tcp 10.0.0.1:5432: refused"), wantCode: http.StatusServiceUnavailable,
			wantBody: `{"error":"not ready"}` + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratings := &fakeRatings{pingErr: tt.pingErr}
			rec := httptest.NewRecorder()
			NewMux(ratings, slog.New(slog.DiscardHandler)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

			if rec.Code != tt.wantCode {
				t.Errorf("code = %d, want %d", rec.Code, tt.wantCode)
			}
			if rec.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
			if got := rec.Header().Get("Cache-Control"); got != cacheNone {
				t.Errorf("Cache-Control = %q, want %q", got, cacheNone)
			}
			if ratings.calls != 1 {
				t.Errorf("pinged the database %d times, want 1", ratings.calls)
			}
		})
	}
}

func TestCurrentRun(t *testing.T) {
	at := time.Date(2026, 9, 27, 4, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		ratings   *fakeRatings
		wantCode  int
		wantCache string
		wantBody  string
	}{
		{
			name:      "published",
			ratings:   &fakeRatings{run: store.PublishedRun{ID: 5, PublishedAt: at}},
			wantCode:  http.StatusOK,
			wantCache: "public, max-age=60",
			wantBody:  `{"published_at":"2026-09-27T04:00:00Z","run_id":5}`,
		},
		{
			name:      "nothing published",
			ratings:   &fakeRatings{runErr: store.ErrNoPublishedRun},
			wantCode:  http.StatusServiceUnavailable,
			wantCache: "no-store",
			wantBody:  `{"error":"no rating run has been published yet"}`,
		},
		{
			name:      "database down",
			ratings:   &fakeRatings{runErr: errors.New("dial tcp 10.0.0.5:5432: user ratings_api refused")},
			wantCode:  http.StatusInternalServerError,
			wantCache: "no-store",
			wantBody:  `{"error":"internal error"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, _ := serve(t, tt.ratings, "/v1/runs/current")

			if rec.Code != tt.wantCode {
				t.Errorf("code = %d, want %d", rec.Code, tt.wantCode)
			}
			if got := rec.Header().Get("Cache-Control"); got != tt.wantCache {
				t.Errorf("Cache-Control = %q, want %q", got, tt.wantCache)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.wantBody {
				t.Errorf("body = %q, want %q", got, tt.wantBody)
			}
		})
	}
}

func TestLeaderboard(t *testing.T) {
	rank := 1
	played := time.Date(2025, 2, 17, 0, 0, 0, 0, time.UTC)
	row := store.LeaderboardRow{
		Rank: &rank, RankDelta: -2, Name: "ann", Rating: 2036.25, RD: 33.5, CRating: 1935.5,
		Games: 12, Opponents: 9, Wins: 7, Losses: 4, Draws: 1, WinRate: 58.3, LastPlayed: &played,
	}
	tests := []struct {
		name      string
		path      string
		ratings   *fakeRatings
		wantCode  int
		wantCache string
		wantCall  bool
		wantModel string
		wantFmt   int
		wantBody  string
	}{
		{
			name: "defaults", path: "/v1/runs/7/leaderboard",
			ratings:  &fakeRatings{board: []store.LeaderboardRow{row}},
			wantCode: http.StatusOK, wantCache: "public, max-age=31536000, immutable",
			wantCall: true, wantModel: "glicko2_daneo", wantFmt: 0,
			wantBody: `{"format":0,"model":"glicko2_daneo","players":[{"c_rating":1935.5,"draws":1,"games":12,` +
				`"last_played":"2025-02-17","losses":4,"name":"ann","opponents":9,"rank":1,"rank_delta":-2,` +
				`"rating":2036.25,"rd":33.5,"win_rate":58.3,"wins":7}],"run_id":7}`,
		},
		{
			name: "model and format chosen", path: "/v1/runs/7/leaderboard?model=whr_softer&format=2",
			ratings:  &fakeRatings{},
			wantCode: http.StatusOK, wantCache: "public, max-age=31536000, immutable",
			wantCall: true, wantModel: "whr_softer", wantFmt: 2,
			wantBody: `{"format":2,"model":"whr_softer","players":[],"run_id":7}`,
		},
		{
			name: "unknown model", path: "/v1/runs/7/leaderboard?model=elo",
			ratings: &fakeRatings{}, wantCode: http.StatusBadRequest, wantCache: "no-store",
			wantBody: `{"error":"unknown model"}`,
		},
		{
			name: "unknown format", path: "/v1/runs/7/leaderboard?format=5",
			ratings: &fakeRatings{}, wantCode: http.StatusBadRequest, wantCache: "no-store",
			wantBody: `{"error":"unknown format"}`,
		},
		{
			name: "format not a number", path: "/v1/runs/7/leaderboard?format=two",
			ratings: &fakeRatings{}, wantCode: http.StatusBadRequest, wantCache: "no-store",
			wantBody: `{"error":"bad parameter: format"}`,
		},
		{
			name: "run not a number", path: "/v1/runs/latest/leaderboard",
			ratings: &fakeRatings{}, wantCode: http.StatusBadRequest, wantCache: "no-store",
			wantBody: `{"error":"bad parameter: run"}`,
		},
		{
			name: "run below one", path: "/v1/runs/0/leaderboard",
			ratings: &fakeRatings{}, wantCode: http.StatusBadRequest, wantCache: "no-store",
			wantBody: `{"error":"bad parameter: run"}`,
		},
		{
			name: "run not published", path: "/v1/runs/9/leaderboard",
			ratings:  &fakeRatings{boardErr: store.ErrRunNotFound},
			wantCode: http.StatusNotFound, wantCache: "no-store", wantCall: true, wantModel: "glicko2_daneo",
			wantBody: `{"error":"no published rating run has this id"}`,
		},
		{
			name: "database down", path: "/v1/runs/7/leaderboard",
			ratings:  &fakeRatings{boardErr: errors.New("dial tcp 10.0.0.5:5432: user ratings_api refused")},
			wantCode: http.StatusInternalServerError, wantCache: "no-store", wantCall: true, wantModel: "glicko2_daneo",
			wantBody: `{"error":"internal error"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, _ := serve(t, tt.ratings, tt.path)

			if rec.Code != tt.wantCode {
				t.Errorf("GET %s: code = %d, want %d", tt.path, rec.Code, tt.wantCode)
			}
			if got := rec.Header().Get("Cache-Control"); got != tt.wantCache {
				t.Errorf("GET %s: Cache-Control = %q, want %q", tt.path, got, tt.wantCache)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("GET %s: Content-Type = %q, want %q", tt.path, got, "application/json")
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.wantBody {
				t.Errorf("GET %s: body = %q, want %q", tt.path, got, tt.wantBody)
			}
			if called := tt.ratings.calls > 0; called != tt.wantCall {
				t.Fatalf("GET %s: store called = %t, want %t", tt.path, called, tt.wantCall)
			}
			if tt.wantCall && (tt.ratings.gotMod != tt.wantModel || tt.ratings.gotFmt != tt.wantFmt) {
				t.Errorf("GET %s: store asked for %s/%d, want %s/%d",
					tt.path, tt.ratings.gotMod, tt.ratings.gotFmt, tt.wantModel, tt.wantFmt)
			}
		})
	}
}

func TestPublicRoutesAllowAnyOriginWithoutCredentials(t *testing.T) {
	for _, path := range []string{"/v1/runs/current", "/v1/runs/7/leaderboard"} {
		rec, _ := serve(t, &fakeRatings{}, path)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("GET %s: Access-Control-Allow-Origin = %q, want %q", path, got, "*")
		}
		if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
			t.Errorf("GET %s: Access-Control-Allow-Credentials = %q, want none", path, got)
		}
	}
}

func TestErrorsLogTheRouteNotThePath(t *testing.T) {
	ratings := &fakeRatings{boardErr: errors.New("boom")}
	_, logs := serve(t, ratings, "/v1/runs/7/leaderboard?model=whr")

	var entry map[string]any
	if err := json.Unmarshal([]byte(logs), &entry); err != nil {
		t.Fatalf("log line %q: %v", logs, err)
	}
	if entry["route"] != "GET /v1/runs/{run}/leaderboard" {
		t.Errorf("log route = %v, want %q", entry["route"], "GET /v1/runs/{run}/leaderboard")
	}
	if !strings.Contains(logs, "boom") {
		t.Errorf("log %q does not carry the error", logs)
	}
	if strings.Contains(logs, "/v1/runs/7") || strings.Contains(logs, "model=whr") {
		t.Errorf("log %q carries the request path", logs)
	}
}
