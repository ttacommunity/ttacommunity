package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/ratingdelta"
	"ttacommunity/backend/internal/store"
)

type fakePlayers struct {
	fakeRatings
	player  store.Player
	hist    []store.HistoryPoint
	page    store.MatchPage
	rivals  []store.Rival
	rated   map[string]store.Rated
	err     error
	asked   string
	format  int
	pageNo  int
	ratedAs string
}

func (f *fakePlayers) Player(_ context.Context, run int64, name string) (store.Player, error) {
	f.calls++
	f.gotRun, f.asked = run, name
	return f.player, f.err
}

func (f *fakePlayers) PlayerHistory(_ context.Context, run int64, name string, format int) (string, []store.HistoryPoint, error) {
	f.calls++
	f.gotRun, f.asked, f.format = run, name, format
	return "Ann", f.hist, f.err
}

func (f *fakePlayers) PlayerMatches(_ context.Context, run int64, name string, format, page int) (string, store.MatchPage, error) {
	f.calls++
	f.gotRun, f.asked, f.format, f.pageNo = run, name, format, page
	return "Ann", f.page, f.err
}

func (f *fakePlayers) RatingsOf(_ context.Context, _ int64, model string, format int, _ []string) (map[string]store.Rated, error) {
	f.ratedAs = model
	f.format = format
	return f.rated, nil
}

func (f *fakePlayers) PlayerRivals(_ context.Context, run int64, name string, format int) (string, []store.Rival, error) {
	f.calls++
	f.gotRun, f.asked, f.format = run, name, format
	return "Ann", f.rivals, f.err
}

func TestPlayer(t *testing.T) {
	last := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	pl := "PL"
	f := &fakePlayers{player: store.Player{
		Name: "Ann", Country: &pl, LastPlayed: &last, NewestGame: &last,
		Title: &store.Title{Code: "GM", Count: 2, Reason: "Royal League"},
		Peak:  &store.Peak{Code: "WC"},
		Games: map[int]int{0: 5, 3: 5},
		Ratings: []store.PlayerRating{
			{Model: "whr_softer", Format: 3, Rank: intp(4), Rating: 1800, RD: 40, CRating: 1680, Opponents: 12, WinRate: 55.5},
		},
	}}
	rec, _ := serve(t, f, "/v1/runs/7/players/ann")
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Errorf("GET player: %d, %q; want 200, five minutes", rec.Code, rec.Header().Get("Cache-Control"))
	}
	want := `{"country":"PL","games":[{"format":0,"games":5},{"format":2,"games":0},{"format":3,"games":5},{"format":4,"games":0}],` +
		`"last_played":"2026-08-31","name":"Ann","newest_game":"2026-08-31","peak":{"code":"WC","year":null},` +
		`"ratings":[{"c_rating":1680,"format":3,"model":"whr_softer","opponents":12,"rank":4,"rating":1800,"rd":40,"win_rate":55.5}],` +
		`"run_id":7,"title":{"code":"GM","count":2,"reason":"Royal League"}}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("GET player: body = %s\nwant %s", got, want)
	}
	if f.gotRun != 7 || f.asked != "ann" {
		t.Errorf("store asked for run %d, %q; want 7, ann", f.gotRun, f.asked)
	}
}

func TestPlayerRoutesRefuseBeforeTheStore(t *testing.T) {
	long := strings.Repeat("a", 201)
	tests := []struct{ name, path, want string }{
		{"bad run", "/v1/runs/0/players/ann", "bad parameter: run"},
		{"long name", "/v1/runs/7/players/" + long, "bad parameter: name"},
		{"history format", "/v1/runs/7/players/ann/history?format=5", "unknown format"},
		{"matches format", "/v1/runs/7/players/ann/matches?format=1", "unknown format"},
		{"matches retro model", "/v1/runs/7/players/ann/matches?model=glicko2_std_retro", "unknown model"},
		{"matches page", "/v1/runs/7/players/ann/matches?page=0", "bad parameter: page"},
		{"rivals format", "/v1/runs/7/players/ann/rivals?format=9", "unknown format"},
		{"rivals long name", "/v1/runs/7/players/" + long + "/rivals", "bad parameter: name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakePlayers{}
			rec, _ := serve(t, f, tt.path)
			if rec.Code != http.StatusBadRequest || rec.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("GET %s: %d, %q; want 400, no-store", tt.path, rec.Code, rec.Header().Get("Cache-Control"))
			}
			if got := strings.TrimSpace(rec.Body.String()); got != `{"error":"`+tt.want+`"}` {
				t.Errorf("GET %s: body = %s, want %q", tt.path, got, tt.want)
			}
			if f.calls != 0 {
				t.Errorf("GET %s reached the store", tt.path)
			}
		})
	}
}

func TestPlayerRoutesAnswerNotFound(t *testing.T) {
	for _, suffix := range []string{"", "/history", "/matches", "/rivals"} {
		for _, err := range []error{store.ErrPlayerNotFound, store.ErrRunNotFound} {
			path := "/v1/runs/7/players/zed" + suffix
			rec, _ := serve(t, &fakePlayers{err: err}, path)
			if rec.Code != http.StatusNotFound || rec.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("GET %s with %v: %d, %q; want 404, no-store", path, err, rec.Code, rec.Header().Get("Cache-Control"))
			}
			if got := strings.TrimSpace(rec.Body.String()); got != `{"error":"`+err.Error()+`"}` {
				t.Errorf("GET %s: body = %s, want %q", path, got, err.Error())
			}
		}
	}
}

func TestPlayerHistoryGroupsPointsByModel(t *testing.T) {
	jan, feb := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC), time.Date(2025, 2, 28, 0, 0, 0, 0, time.UTC)
	f := &fakePlayers{hist: []store.HistoryPoint{
		{Model: "glicko2_daneo", Date: jan, Rating: 1800, CRating: 1700},
		{Model: "glicko2_daneo", Date: feb, Rating: 1810, CRating: 1715},
		{Model: "whr", Date: jan, Rating: 1900, CRating: 1850},
	}}
	rec, _ := serve(t, f, "/v1/runs/7/players/ann/history?format=2")
	want := `{"format":2,"name":"Ann","run_id":7,"series":[` +
		`{"model":"glicko2_daneo","points":[{"c_rating":1700,"date":"2025-01-31","rating":1800},{"c_rating":1715,"date":"2025-02-28","rating":1810}]},` +
		`{"model":"whr","points":[{"c_rating":1850,"date":"2025-01-31","rating":1900}]}]}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Errorf("GET history: %d %s\nwant 200 %s", rec.Code, got, want)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" || f.format != 2 {
		t.Errorf("GET history: Cache-Control %q, format %d; want five minutes, 2", rec.Header().Get("Cache-Control"), f.format)
	}
}

func TestPlayerMatchesPlacesAndEstimates(t *testing.T) {
	day := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	code := "ABC123"
	f := &fakePlayers{
		page: store.MatchPage{Page: 2, Pages: 3, Total: 61, Matches: []store.PlayerMatch{
			{ID: 9, Tournament: "Cup", Date: day, PlayerCount: 3, ReplayCode: &code,
				Seats: []store.Seat{{Name: "cy", Score: 50}, {Name: "Ann", Score: 70}, {Name: "bob", Score: 90}}},
		}},
		rated: map[string]store.Rated{"Ann": {Rating: 1800, RD: 45}, "bob": {Rating: 1900, RD: 35}},
	}
	rec, _ := serve(t, f, "/v1/runs/7/players/ann/matches?format=3&model=whr_softer&page=2")
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Fatalf("GET matches: %d, %q; want 200, five minutes", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if f.ratedAs != "glicko2_daneo_softer" || f.format != 3 || f.pageNo != 2 || f.asked != "ann" {
		t.Errorf("store asked for %s ratings, format %d, page %d, %q; want glicko2_daneo_softer, 3, 2, ann",
			f.ratedAs, f.format, f.pageNo, f.asked)
	}
	var got PlayerMatches
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if got.Name != "Ann" || got.Model != WhrSofter || got.Page != 2 || got.Pages != 3 || got.Total != 61 || len(got.Matches) != 1 {
		t.Fatalf("GET matches = %+v", got)
	}
	m := got.Matches[0]
	wantSeats := []Seat{{Name: "bob", Score: 90}, {Name: "Ann", Score: 70}, {Name: "cy", Score: 50}}
	if m.Placement != 2 || !slices.Equal(m.Seats, wantSeats) || m.ReplayCode == nil || *m.ReplayCode != code {
		t.Errorf("game = place %d, seats %v, replay %v; want 2, %v, %s", m.Placement, m.Seats, m.ReplayCode, wantSeats, code)
	}
	lookup := func(n string) (ratingdelta.Rating, bool) {
		r, ok := f.rated[n]
		return ratingdelta.Rating{Rating: r.Rating, RD: r.RD}, ok
	}
	want, _ := ratingdelta.Delta("glicko2_daneo", "Ann", 3,
		[]ratingdelta.Seat{{Name: "bob", Score: 90}, {Name: "Ann", Score: 70}, {Name: "cy", Score: 50}}, lookup)
	if m.RatingDelta == nil || *m.RatingDelta != want {
		t.Errorf("rating delta = %v, want %v (GlickoD's update for WHR)", m.RatingDelta, want)
	}
}

func TestPlayerMatchesDefaultsToGlickoDAllFormatsPageOne(t *testing.T) {
	f := &fakePlayers{page: store.MatchPage{Page: 1, Pages: 1}}
	rec, _ := serve(t, f, "/v1/runs/7/players/ann/matches")
	want := `{"format":0,"matches":[],"model":"glicko2_daneo","name":"Ann","page":1,"pages":1,"run_id":7,"total":0}`
	if got := strings.TrimSpace(rec.Body.String()); got != want || f.pageNo != 1 {
		t.Errorf("GET matches = %s (page %d asked); want %s", got, f.pageNo, want)
	}
}

func TestPlayerRivalsGivesWinRates(t *testing.T) {
	f := &fakePlayers{rivals: []store.Rival{
		{Name: "bob", Games: 7, Wins: 3, Losses: 3, Draws: 1},
		{Name: "cy", Games: 3, Wins: 2, Losses: 1},
	}}
	rec, _ := serve(t, f, "/v1/runs/7/players/ann/rivals?format=4")
	want := `{"format":4,"name":"Ann","rivals":[` +
		`{"draws":1,"games":7,"losses":3,"name":"bob","win_rate":50,"wins":3},` +
		`{"draws":0,"games":3,"losses":1,"name":"cy","win_rate":66.7,"wins":2}],"run_id":7}`
	if got := strings.TrimSpace(rec.Body.String()); got != want || f.format != 4 {
		t.Errorf("GET rivals = %s (format %d); want %s", got, f.format, want)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Errorf("GET rivals: Cache-Control = %q, want five minutes", rec.Header().Get("Cache-Control"))
	}
}

func intp(n int) *int { return &n }

type fakeCareer struct {
	fakePlayers
	years  []store.CareerYear
	peaks  []store.ModelPeak
	shared []store.PlayerMatch
	other  string
	cells  []store.MatrixCell
	year   *int
}

func (f *fakeCareer) PlayerCareer(_ context.Context, run int64, name string, format int) (string, []store.CareerYear, []store.ModelPeak, error) {
	f.calls++
	f.gotRun, f.asked, f.format = run, name, format
	return "Ann", f.years, f.peaks, f.err
}

func (f *fakeCareer) PlayerMatrix(_ context.Context, run int64, name string, format int, year *int) (string, []store.MatrixCell, error) {
	f.calls++
	f.gotRun, f.asked, f.format, f.year = run, name, format, year
	return "Ann", f.cells, f.err
}

func (f *fakeCareer) Versus(_ context.Context, run int64, name, other string) (string, string, []store.PlayerMatch, error) {
	f.calls++
	f.gotRun, f.asked, f.other = run, name, other
	return "Ann", "Bob", f.shared, f.err
}

func TestPlayerCareer(t *testing.T) {
	peak, yearEnd := 1990.5, 1901.5
	gm := "GM"
	f := &fakeCareer{
		years: []store.CareerYear{
			{Year: 2024, Opponents: 3, Wins: 1, Losses: 1, Draws: 1, PeakRating: &peak, Title: &gm, YearEnd: &yearEnd,
				CareerOpponents: 3, CareerWins: 1, CareerLosses: 1, CareerDraws: 1},
			{Year: 2025, Opponents: 3, Wins: 2, Losses: 1, CareerOpponents: 6, CareerWins: 3, CareerLosses: 2, CareerDraws: 1},
		},
		peaks: []store.ModelPeak{{Model: "whr", Rating: 2060.8, Rank: 2, Date: time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)}},
	}
	rec, _ := serve(t, f, "/v1/runs/7/players/ann/career?format=3")
	want := `{"format":3,"name":"Ann","peaks":[{"date":"2026-08-22","model":"whr","rank":2,"rating":2060.8}],"run_id":7,"years":[` +
		`{"career_draws":1,"career_losses":1,"career_opponents":3,"career_win_rate":50,"career_wins":1,` +
		`"draws":1,"losses":1,"opponents":3,"peak_rating":1990.5,"title":"GM","win_rate":50,"wins":1,"year":2024,"year_end_rating":1901.5},` +
		`{"career_draws":1,"career_losses":2,"career_opponents":6,"career_win_rate":58.3,"career_wins":3,` +
		`"draws":0,"losses":1,"opponents":3,"peak_rating":null,"title":null,"win_rate":66.7,"wins":2,"year":2025,"year_end_rating":null}]}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Errorf("GET career: %d %s\nwant 200 %s", rec.Code, got, want)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" || f.format != 3 || f.asked != "ann" {
		t.Errorf("GET career: Cache-Control %q, asked %q format %d; want five minutes, ann, 3", rec.Header().Get("Cache-Control"), f.asked, f.format)
	}
}

func TestPlayerVersusPlacesBothPlayers(t *testing.T) {
	day := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	f := &fakeCareer{shared: []store.PlayerMatch{{
		ID: 9, Tournament: "Worlds", Date: day, PlayerCount: 3,
		Seats: []store.Seat{{Name: "Ann", Score: 113}, {Name: "cy", Score: 96}, {Name: "Bob", Score: 112}},
	}}}
	rec, _ := serve(t, f, "/v1/runs/7/players/ann/versus/bob")
	want := `{"matches":[{"date":"2026-08-26","match_id":9,"name_place":1,"other_place":2,"player_count":3,` +
		`"seats":[{"name":"Ann","score":113},{"name":"Bob","score":112},{"name":"cy","score":96}],"tournament":"Worlds"}],` +
		`"name":"Ann","other":"Bob","run_id":7}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Errorf("GET versus: %d %s\nwant 200 %s", rec.Code, got, want)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" || f.asked != "ann" || f.other != "bob" {
		t.Errorf("GET versus: Cache-Control %q, asked %q and %q", rec.Header().Get("Cache-Control"), f.asked, f.other)
	}
}

func TestCareerAndVersusRefuseAndAnswerNotFound(t *testing.T) {
	long := strings.Repeat("b", 201)
	for path, want := range map[string]string{
		"/v1/runs/7/players/ann/career?format=5":   "unknown format",
		"/v1/runs/0/players/ann/career":            "bad parameter: run",
		"/v1/runs/7/players/ann/versus/" + long:    "bad parameter: other",
		"/v1/runs/7/players/" + long + "/versus/b": "bad parameter: name",
	} {
		f := &fakeCareer{}
		rec, _ := serve(t, f, path)
		if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusBadRequest || got != `{"error":"`+want+`"}` || f.calls != 0 {
			t.Errorf("GET %s: %d %s (store calls %d); want 400 %q before the store", path, rec.Code, got, f.calls, want)
		}
	}
	for _, path := range []string{"/v1/runs/7/players/zed/career", "/v1/runs/7/players/ann/versus/zed"} {
		rec, _ := serve(t, &fakeCareer{fakePlayers: fakePlayers{err: store.ErrPlayerNotFound}}, path)
		if rec.Code != http.StatusNotFound || rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("GET %s: %d %q; want 404 no-store", path, rec.Code, rec.Header().Get("Cache-Control"))
		}
	}
}

func TestPlayerMatrix(t *testing.T) {
	f := &fakeCareer{cells: []store.MatrixCell{
		{Model: "glicko2_daneo", Rating: 2000, RD: 27.2, CRating: 1918.5, Rank: intp(3)},
		{Model: "whr_softer", Rating: 2050.1, RD: 12, CRating: 2014.1},
	}}
	rec, _ := serve(t, f, "/v1/runs/7/players/ann/matrix?format=2&year=2024")
	want := `{"cells":[{"c_rating":1918.5,"model":"glicko2_daneo","rank":3,"rating":2000,"rd":27.2},` +
		`{"c_rating":2014.1,"model":"whr_softer","rank":null,"rating":2050.1,"rd":12}],"format":2,"name":"Ann","run_id":7,"year":2024}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Errorf("GET matrix: %d %s\nwant 200 %s", rec.Code, got, want)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" || f.year == nil || *f.year != 2024 || f.format != 2 {
		t.Errorf("GET matrix: Cache-Control %q, year %v, format %d", rec.Header().Get("Cache-Control"), f.year, f.format)
	}

	live := &fakeCareer{}
	rec, _ = serve(t, live, "/v1/runs/7/players/ann/matrix")
	if got := strings.TrimSpace(rec.Body.String()); got != `{"cells":[],"format":0,"name":"Ann","run_id":7,"year":null}` || live.year != nil {
		t.Errorf("GET matrix without a year = %s (year asked %v); want the live cells", got, live.year)
	}

	for path, msg := range map[string]string{
		"/v1/runs/7/players/ann/matrix?year=1999": "bad parameter: year",
		"/v1/runs/7/players/ann/matrix?year=x":    "bad parameter: year",
		"/v1/runs/7/players/ann/matrix?format=5":  "unknown format",
	} {
		f := &fakeCareer{}
		rec, _ := serve(t, f, path)
		if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusBadRequest || got != `{"error":"`+msg+`"}` || f.calls != 0 {
			t.Errorf("GET %s: %d %s; want 400 %q before the store", path, rec.Code, got, msg)
		}
	}
	rec, _ = serve(t, &fakeCareer{fakePlayers: fakePlayers{err: store.ErrPlayerNotFound}}, "/v1/runs/7/players/zed/matrix")
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET matrix of an unknown player: %d, want 404", rec.Code)
	}
}
