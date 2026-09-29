package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

type fakeYears struct {
	fakeRatings
	board store.YearBoard
	years []store.Year
	err   error
	year  int
}

func (f *fakeYears) YearBoard(_ context.Context, run int64, year int, model string, format int) (store.YearBoard, error) {
	f.calls++
	f.gotRun, f.year, f.gotMod, f.gotFmt = run, year, model, format
	return f.board, f.err
}

func (f *fakeYears) Years(_ context.Context, run int64) ([]store.Year, error) {
	f.calls++
	f.gotRun = run
	return f.years, f.err
}

func TestYearLeaderboard(t *testing.T) {
	snap, prev := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC), time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)
	last := time.Date(2024, 12, 20, 0, 0, 0, 0, time.UTC)
	pl, pc := "PL", 1500.0
	pr, prd := 1590.0, 30.0
	f := &fakeYears{board: store.YearBoard{
		Year: 2024, Snapshot: snap, Previous: &prev, Games: 2,
		Champions: []store.Champion{{Name: "bob"}},
		Rows: []store.YearRow{{
			Name: "ann", Rating: 1690, RD: 30, CRating: 1600, Country: &pl,
			Title:      &store.Title{Code: "GM", Count: 1, Reason: "Royal League"},
			Season:     store.Record{Opponents: 3, Losses: 2, Draws: 1},
			Career:     store.Record{Opponents: 6, Wins: 3, Losses: 2, Draws: 1},
			LastPlayed: &last, PreviousRating: &pr, PreviousRD: &prd, PreviousCRating: &pc,
		}},
	}}
	rec, _ := serve(t, f, "/v1/runs/7/years/2024/leaderboard?model=glicko2_std&format=3")
	want := `{"champions":[{"name":"bob","reigning":false}],"format":3,"games":2,"model":"glicko2_std","players":[` +
		`{"c_rating":1600,"career":{"draws":1,"losses":2,"opponents":6,"win_rate":58.3,"wins":3},"country":"PL",` +
		`"last_played":"2024-12-20","name":"ann","previous":{"c_rating":1500,"rating":1590,"rd":30},"rating":1690,"rd":30,` +
		`"season":{"draws":1,"losses":2,"opponents":3,"win_rate":16.7,"wins":0},"title":{"code":"GM","count":1,"reason":"Royal League"}}],` +
		`"previous":"2023-12-31","run_id":7,"snapshot":"2024-12-31","year":2024}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Errorf("GET year board: %d %s\nwant 200 %s", rec.Code, got, want)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" || f.year != 2024 || f.gotMod != "glicko2_std" || f.gotFmt != 3 {
		t.Errorf("GET year board: Cache-Control %q; asked %d %s %d", rec.Header().Get("Cache-Control"), f.year, f.gotMod, f.gotFmt)
	}
}

func TestYearLeaderboardDefaultsAndRefusals(t *testing.T) {
	f := &fakeYears{board: store.YearBoard{Year: 2024, Snapshot: time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)}}
	rec, _ := serve(t, f, "/v1/runs/7/years/2024/leaderboard")
	if rec.Code != http.StatusOK || f.gotMod != "glicko2_daneo" || f.gotFmt != 0 ||
		!strings.Contains(rec.Body.String(), `"players":[]`) || !strings.Contains(rec.Body.String(), `"previous":null`) {
		t.Errorf("GET year board with no options: %d %s (asked %s %d)", rec.Code, rec.Body.String(), f.gotMod, f.gotFmt)
	}
	for path, msg := range map[string]string{
		"/v1/runs/7/years/1999/leaderboard":           "bad parameter: year",
		"/v1/runs/7/years/x/leaderboard":              "bad parameter: year",
		"/v1/runs/7/years/2024/leaderboard?model=elo": "unknown model",
		"/v1/runs/7/years/2024/leaderboard?format=5":  "unknown format",
		"/v1/runs/0/years/2024/leaderboard":           "bad parameter: run",
		"/v1/runs/0/years":                            "bad parameter: run",
	} {
		f := &fakeYears{}
		rec, _ := serve(t, f, path)
		if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusBadRequest || got != `{"error":"`+msg+`"}` || f.calls != 0 {
			t.Errorf("GET %s: %d %s; want 400 %q before the store", path, rec.Code, got, msg)
		}
	}
	for _, err := range []error{store.ErrNoHistory, store.ErrRunNotFound} {
		rec, _ := serve(t, &fakeYears{err: err}, "/v1/runs/7/years/2019/leaderboard")
		if rec.Code != http.StatusNotFound || rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("GET year board with %v: %d %q; want 404 no-store", err, rec.Code, rec.Header().Get("Cache-Control"))
		}
	}
}

func TestYears(t *testing.T) {
	f := &fakeYears{years: []store.Year{{Year: 2026}, {Year: 2025, Champion: true}}}
	rec, _ := serve(t, f, "/v1/runs/7/years")
	want := `{"run_id":7,"years":[{"champion":false,"year":2026},{"champion":true,"year":2025}]}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Errorf("GET years: %d %s", rec.Code, got)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Errorf("GET years: Cache-Control %q, want five minutes", rec.Header().Get("Cache-Control"))
	}
	rec, _ = serve(t, &fakeYears{err: store.ErrRunNotFound}, "/v1/runs/9/years")
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET years of an unknown run: %d, want 404", rec.Code)
	}
}
