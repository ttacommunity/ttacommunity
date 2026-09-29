package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"ttacommunity/backend/internal/store"
)

type fakeAnalysis struct {
	fakeRatings
	calib   []store.CalibrationRow
	ratings []store.ModelRating
	active  map[string]bool
	err     error
}

func (f *fakeAnalysis) Calibration(_ context.Context, run int64) ([]store.CalibrationRow, error) {
	f.calls++
	f.gotRun = run
	return f.calib, f.err
}

func (f *fakeAnalysis) AnalysisRatings(_ context.Context, run int64, format int) ([]store.ModelRating, map[string]bool, error) {
	f.calls++
	f.gotRun, f.gotFmt = run, format
	return f.ratings, f.active, f.err
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
