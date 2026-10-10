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
	board  store.YearBoard
	years  []store.Year
	err    error
	calls  int
	gotRun int64
	year   int
	gotMod string
	gotFmt int
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
	rec, _ := serve(t, readsOnly{YearReads: f}, "/v1/runs/7/years/2024/leaderboard?model=glicko2_std&format=3")
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
	rec, _ := serve(t, readsOnly{YearReads: f}, "/v1/runs/7/years/2024/leaderboard")
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
		rec, _ := serve(t, readsOnly{YearReads: f}, path)
		if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusBadRequest || got != `{"error":"`+msg+`"}` || f.calls != 0 {
			t.Errorf("GET %s: %d %s; want 400 %q before the store", path, rec.Code, got, msg)
		}
	}
	for _, err := range []error{store.ErrNoHistory, store.ErrRunNotFound} {
		rec, _ := serve(t, readsOnly{YearReads: &fakeYears{err: err}}, "/v1/runs/7/years/2019/leaderboard")
		if rec.Code != http.StatusNotFound || rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("GET year board with %v: %d %q; want 404 no-store", err, rec.Code, rec.Header().Get("Cache-Control"))
		}
	}
}

func TestYears(t *testing.T) {
	f := &fakeYears{years: []store.Year{{Year: 2026}, {Year: 2025, Champion: true}}}
	rec, _ := serve(t, readsOnly{YearReads: f}, "/v1/runs/7/years")
	want := `{"run_id":7,"years":[{"champion":false,"year":2026},{"champion":true,"year":2025}]}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Errorf("GET years: %d %s", rec.Code, got)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Errorf("GET years: Cache-Control %q, want five minutes", rec.Header().Get("Cache-Control"))
	}
	rec, _ = serve(t, readsOnly{YearReads: &fakeYears{err: store.ErrRunNotFound}}, "/v1/runs/9/years")
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET years of an unknown run: %d, want 404", rec.Code)
	}
}

func yearCSVBoard() store.YearBoard {
	last := time.Date(2024, 12, 20, 0, 0, 0, 0, time.UTC)
	return store.YearBoard{Year: 2024, Snapshot: time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC), Rows: []store.YearRow{
		{Name: "bob", Rating: 1740, RD: 30, CRating: 1650, Career: store.Record{Opponents: 4, Wins: 1, Losses: 3}},
		{Name: "ann", Rating: 1690, RD: 30, CRating: 1600, LastPlayed: &last,
			Season: store.Record{Opponents: 3, Losses: 2, Draws: 1}, Career: store.Record{Opponents: 6, Wins: 3, Losses: 2, Draws: 1}},
	}}
}

func TestLeaderboardCSVForAYear(t *testing.T) {
	f := &fakeYears{board: yearCSVBoard()}
	rec, _ := serve(t, readsOnly{YearReads: f}, "/v1/runs/7/leaderboard.csv?year=2024&model=whr&format=2")
	if rec.Code != http.StatusOK || f.year != 2024 || f.gotMod != "whr" || f.gotFmt != 2 {
		t.Fatalf("GET year CSV: %d; asked %d %s %d", rec.Code, f.year, f.gotMod, f.gotFmt)
	}
	lines := strings.Split(strings.TrimSpace(strings.TrimPrefix(rec.Body.String(), "\xef\xbb\xbf")), "\r\n")
	want := "1,ann,,,,,,,1690.00,30.00,1600.00,3,0,2,1,16.7%,Active,2024-12-20,Whole-History Rating,2-Player (Duel),Continuous (Career),2024"
	if len(lines) != 2 || lines[1] != want {
		t.Errorf("year CSV rows = %q\nwant the header and %q (bob did not play in 2024)", lines, want)
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="tta_ratings_whr_2p_continuous_2024_active.csv"` {
		t.Errorf("Content-Disposition = %q", got)
	}

	rec, _ = serve(t, readsOnly{YearReads: &fakeYears{board: yearCSVBoard()}}, "/v1/runs/7/leaderboard.csv?year=2024&stats_view=career&status=all")
	lines = strings.Split(strings.TrimSpace(strings.TrimPrefix(rec.Body.String(), "\xef\xbb\xbf")), "\r\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[1], "1,bob,,,,,,,1740.00,30.00,1650.00,4,1,3,0,25.0%,Inactive,") ||
		!strings.HasPrefix(lines[2], "2,ann,,,,,,,1690.00,30.00,1600.00,6,3,2,1,58.3%,Active,") {
		t.Errorf("career CSV of everyone = %q", lines)
	}
}

func TestLeaderboardCSVRefusesABadYear(t *testing.T) {
	for path, msg := range map[string]string{
		"/v1/runs/7/leaderboard.csv?year=1999":                  "bad parameter: year",
		"/v1/runs/7/leaderboard.csv?year=2024&stats_view=total": "bad parameter: stats_view",
	} {
		f := &fakeYears{}
		rec, _ := serve(t, readsOnly{YearReads: f}, path)
		if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusBadRequest || got != `{"error":"`+msg+`"}` || f.calls != 0 {
			t.Errorf("GET %s: %d %s; want 400 %q before the store", path, rec.Code, got, msg)
		}
	}
	rec, _ := serve(t, readsOnly{YearReads: &fakeYears{err: store.ErrNoHistory}}, "/v1/runs/7/leaderboard.csv?year=2019")
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET year CSV without history: %d, want 404", rec.Code)
	}
}
