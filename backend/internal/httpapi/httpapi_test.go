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

func (f *fakeRatings) RatingsOf(context.Context, int64, string, int, []string) (map[string]store.Rated, error) {
	return nil, nil
}

func (f *fakeRatings) PlayerCareer(context.Context, int64, string, int) (string, []store.CareerYear, []store.ModelPeak, error) {
	return "", nil, nil, nil
}

func (f *fakeRatings) Versus(context.Context, int64, string, string) (string, string, []store.PlayerMatch, error) {
	return "", "", nil, nil
}

func (f *fakeRatings) PlayerRivals(context.Context, int64, string, int) (string, []store.Rival, error) {
	return "", nil, nil
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
			name: "model and format chosen", path: "/v1/runs/7/leaderboard?model=whr_softer&format=2",
			ratings:  &fakeRatings{},
			wantCode: http.StatusOK, wantCache: "public, max-age=300",
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
			name: "a date", path: "/v1/runs/7/history?model=whr&format=2&at=2026-05-01",
			ratings:  &fakeRatings{hist: rows, histAt: may},
			wantCode: http.StatusOK, wantCache: "public, max-age=31536000, immutable", wantAt: &may,
			wantBody: `{"at":"2026-05-01","format":2,"model":"whr","players":[{"c_rating":1598.5,"name":"ann","rating":1690,"rd":30.5}],"run_id":7}`,
		},
		{
			name: "no date means the previous period", path: "/v1/runs/7/history",
			ratings:  &fakeRatings{histAt: previous},
			wantCode: http.StatusOK, wantCache: "public, max-age=31536000, immutable",
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
			if got := strings.TrimSpace(rec.Body.String()); got != tt.wantBody {
				t.Errorf("GET %s: body = %q, want %q", tt.path, got, tt.wantBody)
			}
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
		{Name: "Grozz", Rating: 1979.194, RD: 20.146, CRating: 1918.756, Opponents: 775, Wins: 551, Losses: 218, Draws: 6,
			WinRate: 71.48, LastPlayed: &played, Country: &ru,
			Title: &store.Title{Code: "GM", Count: 3, Reason: "Played in Royal League Emperor Season 9"},
			Peak:  &store.Peak{Code: "WC", Year: &year}},
		{Name: `-odd, "quoted"`, Rating: 1500, RD: 50, CRating: 1350, Opponents: 40, WinRate: 50, LastPlayed: &played},
		{Name: "retired", Rating: 1900, RD: 50, CRating: 1750, Opponents: 40, WinRate: 50, LastPlayed: &old},
	}
	f := &fakeRatings{board: rows}
	rec, _ := serve(t, f, "/v1/runs/7/leaderboard.csv?model=glicko2_std_softer&format=2")

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	for header, want := range map[string]string{
		"Content-Type":        "text/csv",
		"Cache-Control":       "public, max-age=300",
		"Content-Disposition": `attachment; filename="tta_ratings_glicko2_std_2p_softer_live_active.csv"`,
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if f.gotMod != "glicko2_std_softer" || f.gotFmt != 2 {
		t.Errorf("store asked for %s/%d, want glicko2_std_softer/2", f.gotMod, f.gotFmt)
	}
	want := "\ufeffRank,Player,Country,Title,Title_Name,Badge_Reason,Peak_Title,Peak_Year,Rating,RD,Conservative_Rating," +
		"Matches,Wins,Losses,Draws,Win_Rate,Status,Last_Played,Model,Format,Reset_Mode,Season\r\n" +
		"1,Grozz,RU,GM,Grandmaster,Played in Royal League Emperor Season 9,World Champion,2026,1979.19,20.15,1918.76," +
		"775,551,218,6,71.5%,Active,2026-08-28,Glicko-2 Standard,2-Player (Duel),Season Reset,Live / All-Time\r\n" +
		`2,"'-odd, ""quoted""",,,,,,,1500.00,50.00,1350.00,40,0,0,0,50.0%,Active,2026-08-28,Glicko-2 Standard,2-Player (Duel),Season Reset,Live / All-Time` + "\r\n"
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
