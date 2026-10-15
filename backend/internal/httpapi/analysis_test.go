package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

type fakeAnalysis struct {
	fakeRatings
	calib      []store.CalibrationRow
	ratings    []store.ModelRating
	active     map[string]bool
	compared   []store.ComparedPlayer
	activity   store.Activity
	seatGroups []store.SeatStatsGroup
	err        error
	published  error

	ratingReads int
	gotTerms    []string
	gotCutoff   time.Time
	gotTier     string
}

func (f *fakeAnalysis) ComparedPlayers(_ context.Context, run int64, format int, terms []string) ([]store.ComparedPlayer, error) {
	f.calls++
	f.gotRun, f.gotFmt, f.gotTerms = run, format, terms
	return f.compared, f.err
}

func (f *fakeAnalysis) Activity(_ context.Context, run int64, cutoff time.Time) (store.Activity, error) {
	f.calls++
	f.gotRun, f.gotCutoff = run, cutoff
	return f.activity, f.err
}

func (f *fakeAnalysis) Calibration(_ context.Context, run int64) ([]store.CalibrationRow, error) {
	f.calls++
	f.gotRun = run
	return f.calib, f.err
}

func (f *fakeAnalysis) CheckPublished(_ context.Context, run int64) error {
	f.calls++
	f.gotRun = run
	return f.published
}

func (f *fakeAnalysis) AnalysisRatings(_ context.Context, run int64, format int) ([]store.ModelRating, error) {
	f.calls++
	f.ratingReads++
	f.gotRun, f.gotFmt = run, format
	if f.published != nil {
		return nil, f.published
	}
	return f.ratings, f.err
}

func (f *fakeAnalysis) ActivePlayers(_ context.Context, run int64) (map[string]bool, error) {
	f.gotRun = run
	return f.active, f.err
}

func (f *fakeAnalysis) SeatAnalysis(_ context.Context, run int64, format int, tier string) ([]store.SeatStatsGroup, error) {
	f.calls++
	f.gotRun, f.gotFmt, f.gotTier = run, format, tier
	return f.seatGroups, f.err
}

func TestCalibrationParsesEachRowsBins(t *testing.T) {
	loss, acc := 0.61, 64.2
	f := &fakeAnalysis{calib: []store.CalibrationRow{{
		Mode: "walk_forward", Model: "whr_softer", Format: 3, Matches: 900, Brier: 0.2134, ECE: 0.0123, LogLoss: &loss, Accuracy: &acc,
		Bins: []byte(`{"labels": ["50-55%"], "ideal": [0.525], "preds": [0.52], "actuals": [0.53], "counts": [900]}`),
	}}}
	rec, _ := serve(t, f, "/v1/runs/7/analysis/calibration")
	want := `{"rows":[{"accuracy":64.2,"bins":{"actuals":[0.53],"counts":[900],"ideal":[0.525],"labels":["50-55%"],"preds":[0.52]},` +
		`"brier":0.2134,"ece":0.0123,"format":3,"log_loss":0.61,"matches":900,"mode":"walk_forward","model":"whr_softer"}],"run_id":7}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Errorf("GET calibration: %d %s\nwant 200 %s", rec.Code, got, want)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Errorf("GET calibration: Cache-Control %q", rec.Header().Get("Cache-Control"))
	}

	bad := &fakeAnalysis{calib: []store.CalibrationRow{{Mode: "standard", Model: "whr", Bins: []byte(`{"counts": "x"}`)}}}
	if rec, _ := serve(t, bad, "/v1/runs/7/analysis/calibration"); rec.Code != http.StatusInternalServerError {
		t.Errorf("GET calibration with unreadable bins: %d, want 500", rec.Code)
	}
}

func TestDistributionAndAgreementComputeFromTheRunsRatings(t *testing.T) {
	rank := 1
	ratings := []store.ModelRating{{Player: "ann", Model: "whr", Rating: 1650, Rank: &rank, Opponents: 20}}
	f := &fakeAnalysis{ratings: ratings, active: map[string]bool{"ann": true}}
	rec, _ := serve(t, f, "/v1/runs/7/analysis/distribution?format=2")
	var d Distribution
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET distribution: %d %s", rec.Code, rec.Body.String())
	}
	if f.gotFmt != 2 || d.Format != 2 || len(d.Engines) != 5 || d.Engines[4].Model != Whr || d.Engines[4].Counts[6] != 1 || d.Engines[4].ActiveMean != 1650 {
		t.Errorf("distribution = %+v (format asked %d)", d, f.gotFmt)
	}

	rec, _ = serve(t, &fakeAnalysis{ratings: ratings}, "/v1/runs/7/analysis/agreement")
	if got := strings.TrimSpace(rec.Body.String()); got != `{"format":0,"run_id":7,"sample":0,"top":[],"weakest":[]}` {
		t.Errorf("GET agreement with one player: %s", got)
	}
}

func TestAgreementIsKeptForEachRunAndFormat(t *testing.T) {
	f := &fakeAnalysis{}
	mux := newMux(f, nil, slog.New(slog.DiscardHandler))
	for _, path := range []string{
		"/v1/runs/7/analysis/agreement?format=2", "/v1/runs/7/analysis/agreement?format=2",
		"/v1/runs/7/analysis/agreement?format=3", "/v1/runs/7/analysis/agreement?format=3",
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body.String())
		}
	}
	if f.ratingReads != 2 {
		t.Errorf("formats 2 and 3, each asked twice, read the ratings %d times, want 2", f.ratingReads)
	}
}

func TestAgreementRefusesARunUnpublishedSinceItWasKept(t *testing.T) {
	f := &fakeAnalysis{}
	mux := newMux(f, nil, slog.New(slog.DiscardHandler))
	get := func() int {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/runs/7/analysis/agreement", nil))
		return rec.Code
	}
	if code := get(); code != http.StatusOK {
		t.Fatalf("GET agreement: %d, want 200", code)
	}
	f.published = store.ErrRunNotFound
	if code := get(); code != http.StatusNotFound {
		t.Errorf("GET agreement after the run was unpublished: %d, want 404", code)
	}
}

func TestAnalysisRoutesRefuseAndAnswerNotFound(t *testing.T) {
	for path, msg := range map[string]string{
		"/v1/runs/0/analysis/calibration":           "bad parameter: run",
		"/v1/runs/7/analysis/distribution?format=5": "unknown format",
		"/v1/runs/7/analysis/agreement?format=9":    "unknown format",
	} {
		f := &fakeAnalysis{}
		rec, _ := serve(t, f, path)
		if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusBadRequest || got != `{"error":"`+msg+`"}` || f.calls != 0 {
			t.Errorf("GET %s: %d %s; want 400 %q before the store", path, rec.Code, got, msg)
		}
	}
	for _, path := range []string{"/v1/runs/9/analysis/calibration", "/v1/runs/9/analysis/distribution", "/v1/runs/9/analysis/agreement"} {
		if rec, _ := serve(t, &fakeAnalysis{err: store.ErrRunNotFound}, path); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s of an unknown run: %d, want 404", path, rec.Code)
		}
	}
}

func TestComparedPlayersSplitsTheSearchAndRoundsAsUpstream(t *testing.T) {
	rank, pl, gm := 3, "PL", "GM"
	f := &fakeAnalysis{compared: []store.ComparedPlayer{{Name: "ann", Country: &pl, Title: &gm, Ratings: []store.ComparedRating{
		{Model: "whr_softer", Rank: &rank, Rating: 1812.25, RD: 45.349, CRating: 1721.35, Opponents: 40, WinRate: 61.25},
		{Model: "glicko2_std", Rating: 1500.04, RD: 350, CRating: 800, Opponents: 1, WinRate: 0},
	}}}}
	rec, _ := serve(t, f, "/v1/runs/7/analysis/players?format=3&search=%20Ann%20,,PL")
	want := `{"format":3,"players":[{"country":"PL","name":"ann","ratings":[` +
		`{"c_rating":1721.3,"model":"whr_softer","opponents":40,"rank":3,"rating":1812.2,"rd":45.3,"win_rate":61.25},` +
		`{"c_rating":800,"model":"glicko2_std","opponents":1,"rank":null,"rating":1500,"rd":350,"win_rate":0}],"title":"GM"}],"run_id":7}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Errorf("GET players: %d %s\nwant 200 %s", rec.Code, got, want)
	}
	if f.gotFmt != 3 || fmt.Sprint(f.gotTerms) != "[ann pl]" || rec.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Errorf("asked format %d, terms %q; Cache-Control %q", f.gotFmt, f.gotTerms, rec.Header().Get("Cache-Control"))
	}

	f = &fakeAnalysis{}
	rec, _ = serve(t, f, "/v1/runs/7/analysis/players")
	if got := strings.TrimSpace(rec.Body.String()); got != `{"format":0,"players":[],"run_id":7}` || f.gotTerms != nil {
		t.Errorf("GET players with no search: %s, terms %q, want the defaults (no terms)", got, f.gotTerms)
	}
}

func TestActivityUsesTheSiteCutoffAndRoundsAsUpstreamsSQL(t *testing.T) {
	gm, m := 1925.25, 1883.6666
	last := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	f := &fakeAnalysis{fakeRatings: fakeRatings{lastGame: &last}, activity: store.Activity{
		Active: 3, Inactive: 4,
		Formats:   []store.FormatCount{{Format: 2, Games: 10, Pairs: 10}, {Format: 3, Games: 20, Pairs: 60}},
		Titles:    []store.TitleCount{{Title: "GM", Players: 2}},
		Countries: []store.CountryRatings{{Country: "PL", Players: 5, GM: &gm, MPlus: &m, All: 1770.25}},
	}}
	rec, _ := serve(t, f, "/v1/runs/7/analysis/activity")
	want := `{"active":3,"countries":[{"all":1770.3,"country":"PL","gm":1925.3,"m_plus":1883.7,"p_plus":null,"players":5}],` +
		`"formats":[{"format":2,"games":10,"pairs":10},{"format":3,"games":20,"pairs":60}],"inactive":4,"run_id":7,"titles":[{"code":"GM","players":2}]}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Errorf("GET activity: %d %s\nwant 200 %s", rec.Code, got, want)
	}
	if !f.gotCutoff.Equal(time.Date(2025, 8, 31, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("activity cutoff = %v, want 12 months before the run's last game: 2025-08-31", f.gotCutoff)
	}
}

func TestComparedPlayersAndActivityRefuse(t *testing.T) {
	long := strings.Repeat("a", 201)
	for path, msg := range map[string]string{
		"/v1/runs/0/analysis/players":                "bad parameter: run",
		"/v1/runs/7/analysis/players?format=5":       "unknown format",
		"/v1/runs/7/analysis/players?search=" + long: "bad parameter: search",
		"/v1/runs/0/analysis/activity":               "bad parameter: run",
	} {
		f := &fakeAnalysis{}
		rec, _ := serve(t, f, path)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), msg) || f.calls != 0 {
			t.Errorf("GET %s: %d %s (store calls %d), want 400 %q", path[:min(len(path), 60)], rec.Code, rec.Body.String(), f.calls, msg)
		}
	}
	for _, path := range []string{"/v1/runs/9/analysis/players", "/v1/runs/9/analysis/activity"} {
		rec, _ := serve(t, &fakeAnalysis{err: store.ErrRunNotFound}, path)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s for an unknown run: %d, want 404", path, rec.Code)
		}
	}
}

func TestSeatAnalysisReturnsStatsAndCacheControl(t *testing.T) {
	f := &fakeAnalysis{seatGroups: []store.SeatStatsGroup{{
		PlayerCount: 2,
		TotalGames:  10,
		Seats: []store.SeatWinrateRow{{
			Seat:    1,
			Games:   10,
			Wins:    6,
			WinRate: 60.0,
			Places: []store.SeatPlaceRate{
				{Place: 1, Count: 6, Rate: 60.0},
				{Place: 2, Count: 4, Rate: 40.0},
			},
		}},
	}}}
	rec, _ := serve(t, f, "/v1/runs/7/analysis/seats?tier=grandmaster&format=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/runs/7/analysis/seats: %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheRun {
		t.Errorf("Cache-Control = %q, want %q", got, cacheRun)
	}
	want := `{"groups":[{"player_count":2,"seats":[{"draws":0,"games":10,"places":[{"count":6,"place":1,"rate":60},{"count":4,"place":2,"rate":40}],"seat":1,"win_rate":60,"wins":6}],"total_games":10}],"run_id":7,"tier":"grandmaster"}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("GET /v1/runs/7/analysis/seats = %s\nwant %s", got, want)
	}
	if f.gotRun != 7 || f.gotFmt != 2 || f.gotTier != "grandmaster" {
		t.Errorf("SeatAnalysis called with run=%d, fmt=%d, tier=%q; want 7, 2, 'grandmaster'", f.gotRun, f.gotFmt, f.gotTier)
	}
}

func TestSeatAnalysisRefuse(t *testing.T) {
	for path, msg := range map[string]string{
		"/v1/runs/0/analysis/seats":                   "bad parameter: run",
		"/v1/runs/7/analysis/seats?format=5":          "unknown format",
		"/v1/runs/7/analysis/seats?tier=unknown_tier": "bad parameter: tier",
	} {
		f := &fakeAnalysis{}
		rec, _ := serve(t, f, path)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), msg) || f.calls != 0 {
			t.Errorf("GET %s: %d %s (store calls %d), want 400 %q", path, rec.Code, rec.Body.String(), f.calls, msg)
		}
	}
	rec, _ := serve(t, &fakeAnalysis{err: store.ErrRunNotFound}, "/v1/runs/9/analysis/seats")
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /v1/runs/9/analysis/seats for an unknown run: %d, want 404", rec.Code)
	}
}
