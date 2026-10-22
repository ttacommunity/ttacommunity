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
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"ttacommunity/backend/internal/analysis"
	"ttacommunity/backend/internal/store"
)

type fakeRatings struct {
	// blitzEdition, when set, is what BlitzEdition answers for us_open 2026 (blitz_test.go).
	blitzEdition *store.BlitzEdition
	run          store.PublishedRun
	runErr       error
	lastGame     *time.Time
	// rated is what RatingsOf answers, by name.
	rated    map[string]store.Rated
	board    []store.LeaderboardRow
	boardErr error
	pingErr  error

	hist    []store.HistoryRow
	histAt  time.Time
	histErr error

	calls  int
	gotRun int64
	gotMod string
	gotFmt int
	gotAt  *time.Time
}

func (f *fakeRatings) History(_ context.Context, run int64, model string, format int, at *time.Time) (time.Time, []store.HistoryRow, error) {
	f.calls++
	f.gotRun, f.gotMod, f.gotFmt, f.gotAt = run, model, format, at
	return f.histAt, f.hist, f.histErr
}

func (f *fakeRatings) Ping(context.Context) error {
	f.calls++
	return f.pingErr
}

func (f *fakeRatings) RunLastGame(context.Context, int64) (*time.Time, error) {
	return f.lastGame, nil
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

func (f *fakeRatings) Player(context.Context, int64, string) (store.Player, error) {
	return store.Player{}, nil
}

func (f *fakeRatings) PlayerHistory(context.Context, int64, string, int) (string, []store.HistoryPoint, error) {
	return "", nil, nil
}

func (f *fakeRatings) PlayerMatches(context.Context, int64, string, int, int) (string, store.MatchPage, error) {
	return "", store.MatchPage{}, nil
}

func (f *fakeRatings) RatingsOf(_ context.Context, _ int64, _ string, _ int, names []string) (map[string]store.Rated, error) {
	got := map[string]store.Rated{}
	for _, n := range names {
		if r, ok := f.rated[n]; ok {
			got[n] = r
		}
	}
	return got, nil
}

func (f *fakeRatings) HistoricalRatingsOf(_ context.Context, _ int64, _ string, _ int, at []store.PlayerAt) (map[store.PlayerAt]store.Rated, error) {
	got := map[store.PlayerAt]store.Rated{}
	for _, a := range at {
		if r, ok := f.rated[a.Name]; ok {
			got[a] = r
		}
	}
	return got, nil
}

func (f *fakeRatings) HiddenNames(context.Context, []string) (map[string]bool, error) {
	return nil, nil
}

func (f *fakeRatings) PlayerCareer(context.Context, int64, string, int, string) (string, []store.CareerYear, *store.CareerPeak, error) {
	return "", nil, nil, nil
}

func (f *fakeRatings) PlayerSeats(context.Context, int64, string, bool) (store.PlayerSeatStats, error) {
	return store.PlayerSeatStats{}, nil
}

func (f *fakeRatings) Calibration(context.Context, int64) ([]store.CalibrationRow, error) {
	return nil, nil
}

func (f *fakeRatings) AnalysisRatings(context.Context, int64, int) ([]store.ModelRating, error) {
	return nil, nil
}

func (f *fakeRatings) CheckPublished(context.Context, int64) error {
	return nil
}

func (f *fakeRatings) ActivePlayers(context.Context, int64) (map[string]bool, error) {
	return nil, nil
}

func (f *fakeRatings) ComparedPlayers(context.Context, int64, int, []string) ([]store.ComparedPlayer, error) {
	return nil, nil
}

func (f *fakeRatings) Activity(context.Context, int64, time.Time) (store.Activity, error) {
	return store.Activity{}, nil
}

func (f *fakeRatings) SeatAnalysis(context.Context, int64, int, string) ([]store.SeatStatsGroup, error) {
	return nil, nil
}

func (f *fakeRatings) YearBoard(context.Context, int64, int, string, int) (store.YearBoard, error) {
	return store.YearBoard{}, nil
}

func (f *fakeRatings) Years(context.Context, int64) ([]store.Year, error) {
	return nil, nil
}

func (f *fakeRatings) PlayerMatrix(context.Context, int64, string, int, *int) (string, []store.MatrixCell, error) {
	return "", nil, nil
}

func (f *fakeRatings) Versus(context.Context, int64, string, string) (string, string, []store.PlayerMatch, error) {
	return "", "", nil, nil
}

func (f *fakeRatings) PlayerRivals(context.Context, int64, string, int) (string, []store.Rival, error) {
	return "", nil, nil
}

func (f *fakeRatings) TournamentSeries(context.Context, int64, string) (store.TournamentSeries, error) {
	panic("not faked")
}

func (f *fakeRatings) HallOfFame(context.Context, int64) (store.HallOfFame, error) {
	panic("not faked")
}

func (f *fakeRatings) CommunityLeaderboard(context.Context, int64, string) ([]store.CommunityRow, error) {
	panic("not faked")
}

func (f *fakeRatings) CommunityLeaderboardLayout(context.Context, int64, string) (store.CommunityEditionLayout, error) {
	panic("not faked")
}

func (f *fakeRatings) CommunityEditions(context.Context) ([]store.CommunityEdition, error) {
	return []store.CommunityEdition{}, nil
}

func (f *fakeRatings) Catalogue(context.Context, int64) (store.CatalogueView, error) {
	panic("not faked")
}

func (f *fakeRatings) CatalogueTournament(context.Context, int64, string) (store.CatalogueTournament, error) {
	panic("not faked")
}

func (f *fakeRatings) CatalogueGames(context.Context, int64, string, store.CatalogueGamesFilter) (store.CatalogueGames, error) {
	panic("not faked")
}

func (f *fakeRatings) RecentPodiums(context.Context, int64, int, int) ([]store.RecentPodium, error) {
	panic("not faked")
}

func (f *fakeRatings) PlayerAchievements(context.Context, int64, string) (string, *store.AchievementSummary, []store.TournamentRecord, error) {
	panic("not faked")
}

// readsOnly is a Ratings with only the parts a test sets: any other read panics, and the mux answers 500.
type readsOnly struct {
	RunReads
	BoardReads
	PlayerReads
	YearReads
	AnalysisReads
	TournamentReads
	CatalogueReads
	BlitzReads
}

type observed struct {
	route  string
	status int
	took   time.Duration
}

// recordedRequests keeps what the mux reports of each request.
type recordedRequests struct{ got []observed }

func (r *recordedRequests) Observe(route string, status int, took time.Duration) {
	r.got = append(r.got, observed{route, status, took})
}

// stepClock returns a clock that moves on by step each time it is read.
func stepClock(step time.Duration) func() time.Time {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	return func() time.Time {
		now = now.Add(step)
		return now
	}
}

// newMux is NewMux for tests that are not about what it reports of each request.
func newMux(ratings Ratings, admins Admins, log *slog.Logger) http.Handler {
	return NewMux(ratings, admins, nil, log, &recordedRequests{}, stepClock(time.Millisecond))
}

func serve(t *testing.T, ratings Ratings, path string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	rec := httptest.NewRecorder()
	newMux(ratings, nil, log).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
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
	registerRoutes(router, &fakeRatings{}, nil, nil, slog.New(slog.DiscardHandler), stepClock(time.Millisecond))

	slices.Sort(want)
	slices.Sort(router.patterns)
	if !slices.Equal(router.patterns, want) {
		t.Errorf("routes = %q, want the spec's %q", router.patterns, want)
	}
}

func TestTheModelsTheSiteNamesAreTheOnesTheEngineComputes(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("reading the spec: %v", err)
	}
	var spec struct {
		Components struct {
			Schemas struct {
				Model struct {
					Enum []string `yaml:"enum"`
				} `yaml:"Model"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parsing the spec: %v", err)
	}
	computed := []string{"glicko2_daneo", "glicko2_daneo_softer", "glicko2_std"}
	if got := spec.Components.Schemas.Model.Enum; !slices.Equal(got, computed) {
		t.Errorf("the spec's models = %q, want the engine's %q", got, computed)
	}
	for name, list := range map[string][]string{
		"CompareModels": store.CompareModels, "PlayerPageModels": store.PlayerPageModels, "analysis.Engines": analysis.Engines,
	} {
		for _, m := range list {
			if !slices.Contains(computed, m) {
				t.Errorf("%s names %q, which the engine no longer computes", name, m)
			}
		}
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
			newMux(ratings, nil, slog.New(slog.DiscardHandler)).ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))

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

func TestRobotsTxtKeepsCrawlersOffPlayersOnly(t *testing.T) {
	ratings := &fakeRatings{}
	rec := httptest.NewRecorder()
	newMux(ratings, nil, slog.New(slog.DiscardHandler)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/robots.txt", nil))

	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("GET /robots.txt: %d %q, want 200 text/plain", rec.Code, rec.Header().Get("Content-Type"))
	}
	lines := strings.Split(rec.Body.String(), "\n")
	if !slices.Contains(lines, "Disallow: /v1/runs/*/players/") || slices.Contains(lines, "Disallow: /") {
		t.Errorf("GET /robots.txt: body = %q, want only the player routes disallowed", rec.Body.String())
	}
	if ratings.calls != 0 {
		t.Errorf("GET /robots.txt: touched the database %d times, want 0", ratings.calls)
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
			newMux(ratings, nil, slog.New(slog.DiscardHandler)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

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
	first, last := time.Date(2017, 3, 4, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	finished, games, players := at.Add(-time.Minute), 135393, 3526
	tests := []struct {
		name      string
		ratings   *fakeRatings
		wantCode  int
		wantCache string
		wantBody  string
	}{
		{
			name: "published",
			ratings: &fakeRatings{run: store.PublishedRun{ID: 5, PublishedAt: at, FirstGame: &first, LastGame: &last,
				Games: &games, Players: &players, EngineVersion: "abc1234", FinishedAt: &finished}},
			wantCode:  http.StatusOK,
			wantCache: "public, max-age=60",
			wantBody: `{"engine_version":"abc1234","finished_at":"2026-09-27T03:59:00Z","first_game":"2017-03-04","games":135393,` +
				`"last_game":"2026-08-31","players":3526,"published_at":"2026-09-27T04:00:00Z","run_id":5}`,
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
			wantJSON(t, rec.Body.Bytes(), "", tt.wantBody)
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
	pl := "PL"
	titled := row
	titled.Name, titled.Country = "bob", &pl
	titled.Title = &store.Title{Code: "GM", Count: 3, Reason: "Played in Royal League Grandmaster Season 16"}
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
			ratings:  &fakeRatings{board: []store.LeaderboardRow{row, titled}},
			wantCode: http.StatusOK, wantCache: "public, max-age=300",
			wantCall: true, wantModel: "glicko2_daneo", wantFmt: 0,
			wantBody: `{"format":0,"model":"glicko2_daneo","players":[{"c_rating":1935.5,"country":null,"draws":1,"games":12,` +
				`"last_played":"2025-02-17","losses":4,"name":"ann","opponents":9,"rank":1,"rank_delta":-2,` +
				`"rating":2036.25,"rd":33.5,"title":null,"win_rate":58.3,"wins":7},` +
				`{"c_rating":1935.5,"country":"PL","draws":1,"games":12,` +
				`"last_played":"2025-02-17","losses":4,"name":"bob","opponents":9,"rank":1,"rank_delta":-2,` +
				`"rating":2036.25,"rd":33.5,"title":{"code":"GM","count":3,"reason":"Played in Royal League Grandmaster Season 16"},` +
				`"win_rate":58.3,"wins":7}],"run_id":7}`,
		},
		{
			name: "model and format chosen", path: "/v1/runs/7/leaderboard?model=glicko2_daneo_softer&format=2",
			ratings:  &fakeRatings{},
			wantCode: http.StatusOK, wantCache: "public, max-age=300",
			wantCall: true, wantModel: "glicko2_daneo_softer", wantFmt: 2,
			wantBody: `{"format":2,"model":"glicko2_daneo_softer","players":[],"run_id":7}`,
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
			wantJSON(t, rec.Body.Bytes(), "", tt.wantBody)
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
	_, logs := serve(t, ratings, "/v1/runs/7/leaderboard?model=glicko2_std")

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
	if strings.Contains(logs, "/v1/runs/7") || strings.Contains(logs, "model=glicko2_std") {
		t.Errorf("log %q carries the request path", logs)
	}
}

func TestRecoverPanics(t *testing.T) {
	tests := []struct {
		name     string
		handler  http.HandlerFunc
		wantCode int
		wantLog  string
	}{
		{"panic with an error", func(http.ResponseWriter, *http.Request) { panic(errors.New("kaboom")) }, http.StatusInternalServerError, "kaboom"},
		{"panic with a string", func(http.ResponseWriter, *http.Request) { panic("kaboom") }, http.StatusInternalServerError, "kaboom"},
		{"no panic", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }, http.StatusTeapot, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.Handle("GET /boom/{name}", tt.handler)
			var buf bytes.Buffer
			h := recoverPanics(mux, mux, slog.New(slog.NewJSONHandler(&buf, nil)))
			req := httptest.NewRequest(http.MethodGet, "/boom/Someplayer", nil)
			req.RemoteAddr = "203.0.113.9:4000"
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("code = %d, want %d", rec.Code, tt.wantCode)
			}
			logs := buf.String()
			if tt.wantLog == "" {
				if logs != "" {
					t.Errorf("log = %q, want none", logs)
				}
				return
			}
			wantJSON(t, rec.Body.Bytes(), "error", `"internal error"`)
			if !strings.Contains(logs, `"route":"GET /boom/{name}"`) || !strings.Contains(logs, tt.wantLog) {
				t.Errorf("log = %q, want the route and %q", logs, tt.wantLog)
			}
			if strings.Contains(logs, "Someplayer") || strings.Contains(logs, "203.0.113.9") {
				t.Errorf("log = %q, carries the path or the client IP", logs)
			}
			var line struct{ Stack string }
			if err := json.Unmarshal(buf.Bytes(), &line); err != nil || !strings.Contains(line.Stack, "httpapi_test.go") {
				t.Errorf("log stack = %q (%v), want one naming httpapi_test.go", line.Stack, err)
			}
		})
	}
}

func TestWithDeadline(t *testing.T) {
	tests := []struct {
		name   string
		parent time.Duration
		want   time.Duration
	}{
		{"no deadline yet", 0, 25 * time.Second},
		{"an earlier deadline is kept", time.Second, time.Second},
		{"a later deadline is shortened", time.Minute, 25 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got time.Time
			var ok bool
			h := withDeadline(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got, ok = r.Context().Deadline()
			}), 25*time.Second)
			ctx := context.Background()
			before := time.Now()
			if tt.parent > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.parent)
				defer cancel()
			}

			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil))

			after := time.Now()
			if !ok || got.Before(before.Add(tt.want)) || got.After(after.Add(tt.want)) {
				t.Errorf("deadline = %v (set %v), want %v from now", got, ok, tt.want)
			}
		})
	}
}

type deadlineRatings struct {
	fakeRatings
	hasDeadline bool
}

func (d *deadlineRatings) CurrentRun(ctx context.Context) (store.PublishedRun, error) {
	_, d.hasDeadline = ctx.Deadline()
	return store.PublishedRun{}, errors.New("no run")
}

func TestTheAPIGivesEachRequestADeadline(t *testing.T) {
	ratings := &deadlineRatings{}
	serve(t, ratings, "/v1/runs/current")

	if !ratings.hasDeadline {
		t.Error("GET /v1/runs/current: the handler's context has no deadline")
	}
}

type panickingRatings struct{ fakeRatings }

func (*panickingRatings) CurrentRun(context.Context) (store.PublishedRun, error) {
	panic("kaboom")
}

type panickingAdmins struct{ fakeAdmins }

func (*panickingAdmins) Runs(context.Context) ([]store.AdminRun, error) {
	panic("kaboom")
}

func TestTheAPIRecoversAPanicByRoute(t *testing.T) {
	tests := []struct {
		name    string
		ratings Ratings
		admins  Admins
		path    string
		route   string
	}{
		{"a public route", &panickingRatings{}, nil, "/v1/runs/current", "GET /v1/runs/current"},
		{"an admin route", &fakeRatings{}, &panickingAdmins{}, "/v1/admin/runs", "GET /v1/admin/runs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Cf-Access-Jwt-Assertion", "good-111111111111111111")
			rec := httptest.NewRecorder()

			newMux(tt.ratings, tt.admins, slog.New(slog.NewJSONHandler(&logs, nil))).ServeHTTP(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Errorf("code = %d, want %d", rec.Code, http.StatusInternalServerError)
			}
			if !strings.Contains(logs.String(), `"route":"`+tt.route+`"`) {
				t.Errorf("log = %q, want the route %q", logs.String(), tt.route)
			}
		})
	}
}

var requestIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

type failingAdmin struct{ fakeAdmins }

func (*failingAdmin) Admin(context.Context, string) (store.Admin, error) {
	return store.Admin{}, errors.New("connection refused")
}

func TestServerErrorsCarryTheRequestID(t *testing.T) {
	tests := []struct {
		name    string
		ratings Ratings
		admins  Admins
		path    string
	}{
		{"a handler's error", &fakeRatings{boardErr: errors.New("boom")}, nil, "/v1/runs/7/leaderboard"},
		{"a handler's panic", &panickingRatings{}, nil, "/v1/runs/current"},
		{"the admin gate's error", &fakeRatings{}, &failingAdmin{}, "/v1/admin/me"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Cf-Access-Jwt-Assertion", "good-111111111111111111")
			rec := httptest.NewRecorder()

			newMux(tt.ratings, tt.admins, slog.New(slog.NewJSONHandler(&logs, nil))).ServeHTTP(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Errorf("code = %d, want %d", rec.Code, http.StatusInternalServerError)
			}
			id := rec.Header().Get("X-Request-Id")
			if !requestIDPattern.MatchString(id) {
				t.Fatalf("X-Request-Id = %q, want 16 hex digits", id)
			}
			wantJSON(t, rec.Body.Bytes(), "request_id", strconv.Quote(id))
			var entry map[string]any
			if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
				t.Fatalf("log line %q: %v", logs.String(), err)
			}
			if entry["request_id"] != id {
				t.Errorf("log request_id = %v, want %q", entry["request_id"], id)
			}
		})
	}
}

func TestEachResponseGetsItsOwnRequestID(t *testing.T) {
	mux := newMux(&fakeRatings{}, nil, slog.New(slog.DiscardHandler))
	var ids []string
	for range 2 {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set("X-Request-Id", "0123456789abcdef")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		ids = append(ids, rec.Header().Get("X-Request-Id"))
	}

	for _, id := range ids {
		if !requestIDPattern.MatchString(id) || id == "0123456789abcdef" {
			t.Errorf("X-Request-Id = %q, want 16 hex digits of our own", id)
		}
	}
	if ids[0] == ids[1] {
		t.Errorf("two requests got the same X-Request-Id %q", ids[0])
	}
}

func TestTheAPIReportsEachRequestByRouteAndStatus(t *testing.T) {
	tests := []struct {
		name    string
		ratings Ratings
		admins  Admins
		path    string
		want    observed
	}{
		{"a public route", &fakeRatings{}, nil, "/healthz", observed{"GET /healthz", 200, time.Millisecond}},
		{"a failing route", &fakeRatings{runErr: errors.New("boom")}, nil, "/v1/runs/current", observed{"GET /v1/runs/current", 500, time.Millisecond}},
		{"an admin route", &fakeRatings{}, &fakeAdmins{}, "/v1/admin/runs", observed{"GET /v1/admin/runs", 200, time.Millisecond}},
		{"a panicking admin route", &fakeRatings{}, &panickingAdmins{}, "/v1/admin/runs", observed{"GET /v1/admin/runs", 500, time.Millisecond}},
		{"an admin request the gate refuses", &fakeRatings{}, nil, "/v1/admin/runs", observed{"GET /v1/admin/runs", 401, time.Millisecond}},
		{"a path with no route", &fakeRatings{}, nil, "/v1/nowhere", observed{"unrouted", 404, time.Millisecond}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests := &recordedRequests{}
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Cf-Access-Jwt-Assertion", "good-111111111111111111")

			NewMux(tt.ratings, tt.admins, nil, slog.New(slog.DiscardHandler), requests, stepClock(time.Millisecond)).ServeHTTP(httptest.NewRecorder(), req)

			if len(requests.got) != 1 || requests.got[0] != tt.want {
				t.Errorf("GET %s reported %+v, want one %+v", tt.path, requests.got, tt.want)
			}
		})
	}
}

func TestTheAPILogsASlowRequest(t *testing.T) {
	tests := []struct {
		name     string
		took     time.Duration
		wantLogs bool
	}{
		{"a quick answer", 999 * time.Millisecond, false},
		{"a slow answer", time.Second, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			rec := httptest.NewRecorder()

			NewMux(&fakeRatings{}, nil, nil, slog.New(slog.NewJSONHandler(&logs, nil)), &recordedRequests{}, stepClock(tt.took)).
				ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz?player=Someplayer", nil))

			if !tt.wantLogs {
				if logs.Len() != 0 {
					t.Errorf("log = %q, want none", logs.String())
				}
				return
			}
			var entry map[string]any
			if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
				t.Fatalf("log line %q: %v", logs.String(), err)
			}
			want := map[string]any{"msg": "slow request", "route": "GET /healthz", "status": float64(200),
				"took_ms": float64(tt.took.Milliseconds()), "request_id": rec.Header().Get("X-Request-Id")}
			for k, v := range want {
				if entry[k] != v {
					t.Errorf("log %s = %v, want %v", k, entry[k], v)
				}
			}
			if strings.Contains(logs.String(), "Someplayer") {
				t.Errorf("log %q carries the request path", logs.String())
			}
		})
	}
}

func TestRecoverPanicsLetsAnAbortThrough(t *testing.T) {
	var buf bytes.Buffer
	h := recoverPanics(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}), http.NewServeMux(), slog.New(slog.NewJSONHandler(&buf, nil)))
	defer func() {
		if v := recover(); v != http.ErrAbortHandler {
			t.Errorf("recovered %v, want http.ErrAbortHandler re-panicked", v)
		}
		if buf.Len() != 0 {
			t.Errorf("log = %q, want none", buf.String())
		}
	}()

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestAPanicAfterTheAnswerStartedAbortsIt(t *testing.T) {
	discard := slog.New(slog.DiscardHandler)
	h := observe(recoverPanics(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("PK"))
		panic("kaboom")
	}), http.NewServeMux(), discard), http.NewServeMux(), &recordedRequests{}, stepClock(time.Millisecond), discard)
	rec := httptest.NewRecorder()
	defer func() {
		if v := recover(); v != http.ErrAbortHandler {
			t.Errorf("recovered %v, want http.ErrAbortHandler", v)
		}
		if strings.Contains(rec.Body.String(), "internal error") {
			t.Errorf("body = %q, want no error written into the started answer", rec.Body)
		}
	}()

	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestHistory(t *testing.T) {
	previous := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	may := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	rows := []store.HistoryRow{{Name: "ann", Rating: 1690, RD: 30.5, CRating: 1598.5}}
	tests := []struct {
		name      string
		path      string
		ratings   *fakeRatings
		wantCode  int
		wantCache string
		wantAt    *time.Time
		wantBody  string
	}{
		{
			name: "a date", path: "/v1/runs/7/history?model=glicko2_std&format=2&at=2026-05-01",
			ratings:  &fakeRatings{hist: rows, histAt: may},
			wantCode: http.StatusOK, wantCache: "public, max-age=300", wantAt: &may,
			wantBody: `{"at":"2026-05-01","format":2,"model":"glicko2_std","players":[{"c_rating":1598.5,"name":"ann","rating":1690,"rd":30.5}],"run_id":7}`,
		},
		{
			name: "no date means the previous period", path: "/v1/runs/7/history",
			ratings:  &fakeRatings{histAt: previous},
			wantCode: http.StatusOK, wantCache: "public, max-age=300",
			wantBody: `{"at":"2026-07-31","format":0,"model":"glicko2_daneo","players":[],"run_id":7}`,
		},
		{
			name: "not a date", path: "/v1/runs/7/history?at=May",
			ratings: &fakeRatings{}, wantCode: http.StatusBadRequest, wantCache: "no-store",
			wantBody: `{"error":"bad parameter: at"}`,
		},
		{
			name: "unknown model", path: "/v1/runs/7/history?model=elo",
			ratings: &fakeRatings{}, wantCode: http.StatusBadRequest, wantCache: "no-store",
			wantBody: `{"error":"unknown model"}`,
		},
		{
			name: "run not published", path: "/v1/runs/9/history",
			ratings: &fakeRatings{histErr: store.ErrRunNotFound}, wantCode: http.StatusNotFound, wantCache: "no-store",
			wantBody: `{"error":"no published rating run has this id"}`,
		},
		{
			name: "no history", path: "/v1/runs/9/history",
			ratings: &fakeRatings{histErr: store.ErrNoHistory}, wantCode: http.StatusNotFound, wantCache: "no-store",
			wantBody: `{"error":"this run has no rating history for this model and format"}`,
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
			wantJSON(t, rec.Body.Bytes(), "", tt.wantBody)
			if (tt.ratings.gotAt == nil) != (tt.wantAt == nil) || (tt.wantAt != nil && !tt.ratings.gotAt.Equal(*tt.wantAt)) {
				t.Errorf("GET %s: store asked for at %v, want %v", tt.path, tt.ratings.gotAt, tt.wantAt)
			}
		})
	}
}

func TestLeaderboardCSV(t *testing.T) {
	played := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	ru, year := "RU", 2026
	rows := []store.LeaderboardRow{
		{Name: "Grebe", Rating: 1979.194, RD: 20.146, CRating: 1918.756, Opponents: 775, Wins: 551, Losses: 218, Draws: 6,
			WinRate: 71.48, LastPlayed: &played, Country: &ru,
			Title: &store.Title{Code: "GM", Count: 3, Reason: "Played in Royal League Emperor Season 9"},
			Peak:  &store.Peak{Code: "WC", Year: &year}},
		{Name: `-odd, "quoted"`, Rating: 1500, RD: 50, CRating: 1350, Opponents: 40, WinRate: 50, LastPlayed: &played},
		{Name: "retired", Rating: 1900, RD: 50, CRating: 1750, Opponents: 40, WinRate: 50, LastPlayed: &old},
	}
	f := &fakeRatings{board: rows}
	rec, _ := serve(t, f, "/v1/runs/7/leaderboard.csv?model=glicko2_daneo_softer&format=2")

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	for header, want := range map[string]string{
		"Content-Type":        "text/csv",
		"Cache-Control":       "public, max-age=300",
		"Content-Disposition": `attachment; filename="tta_ratings_glicko2_daneo_2p_softer_live_active.csv"`,
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if f.gotMod != "glicko2_daneo_softer" || f.gotFmt != 2 {
		t.Errorf("store asked for %s/%d, want glicko2_daneo_softer/2", f.gotMod, f.gotFmt)
	}
	want := "\ufeffRank,Player,Country,Title,Title_Name,Badge_Reason,Peak_Title,Peak_Year,Rating,RD,Conservative_Rating," +
		"Matches,Wins,Losses,Draws,Win_Rate,Status,Last_Played,Model,Format,Reset_Mode,Season\r\n" +
		"1,Grebe,RU,GM,Grandmaster,Played in Royal League Emperor Season 9,World Champion,2026,1979.19,20.15,1918.76," +
		"775,551,218,6,71.5%,Active,2026-08-28,GlickoD,2-Player (Duel),Season Reset,Live / All-Time\r\n" +
		`2,"'-odd, ""quoted""",,,,,,,1500.00,50.00,1350.00,40,0,0,0,50.0%,Active,2026-08-28,GlickoD,2-Player (Duel),Season Reset,Live / All-Time` + "\r\n"
	if got := rec.Body.String(); got != want {
		t.Errorf("body =\n%q\nwant\n%q", got, want)
	}
}

func TestLeaderboardCSVFollowsTheFilters(t *testing.T) {
	played := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	rows := []store.LeaderboardRow{
		{Name: "ann", CRating: 1900, Opponents: 5, LastPlayed: &played},
		{Name: "bob", CRating: 1800, Opponents: 50, LastPlayed: &played},
	}
	rec, _ := serve(t, &fakeRatings{board: rows}, "/v1/runs/7/leaderboard.csv?status=all&min_opps=0&search=an&sort=name&order=desc")
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\r\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[1], "1,ann,") {
		t.Errorf("rows = %q, want only ann, ranked 1", lines[1:])
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.HasSuffix(got, `_all.csv"`) {
		t.Errorf("Content-Disposition = %q, want the all-status file name", got)
	}
}

func TestLeaderboardCSVRefusals(t *testing.T) {
	for _, tt := range []struct{ path, want string }{
		{"/v1/runs/7/leaderboard.csv?status=gone", `{"error":"unknown status"}`},
		{"/v1/runs/7/leaderboard.csv?min_opps=-1", `{"error":"bad parameter: min_opps"}`},
		{"/v1/runs/7/leaderboard.csv?min_opps=1001", `{"error":"bad parameter: min_opps"}`},
		{"/v1/runs/7/leaderboard.csv?title=Q", `{"error":"unknown title"}`},
		{"/v1/runs/7/leaderboard.csv?sort=luck", `{"error":"unknown sort"}`},
		{"/v1/runs/7/leaderboard.csv?search=" + strings.Repeat("a", 201), `{"error":"bad parameter: search"}`},
	} {
		rec, _ := serve(t, &fakeRatings{}, tt.path)
		if rec.Code != http.StatusBadRequest || strings.TrimSpace(rec.Body.String()) != tt.want {
			t.Errorf("GET %s = %d %s, want 400 %s", tt.path, rec.Code, rec.Body.String(), tt.want)
		}
	}
	rec, _ := serve(t, &fakeRatings{boardErr: store.ErrRunNotFound}, "/v1/runs/9/leaderboard.csv")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unpublished run: code = %d, want 404", rec.Code)
	}
}

func TestLeaderboardSendsEachPlayersBadgesOnlyWhenTheyHaveSome(t *testing.T) {
	rank := 1
	f := &fakeRatings{run: store.PublishedRun{ID: 7}, board: []store.LeaderboardRow{
		{Rank: &rank, Name: "ann", Badges: []store.ExtraBadge{{Label: "Organiser", Description: "Runs a league"}}},
		{Name: "bob"},
	}}
	rec, _ := serve(t, f, "/v1/runs/7/leaderboard?model=glicko2_daneo&format=0")
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("GET leaderboard = %d %s", rec.Code, body)
	}
	if !strings.Contains(body, `"badges":[{"description":"Runs a league","label":"Organiser"}]`) || strings.Count(body, `"badges"`) != 1 {
		t.Errorf("leaderboard = %s, want ann's badge and no badges field for bob", body)
	}
}
