package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

type fakeTournaments struct {
	fakeRatings
	series    store.TournamentSeries
	err       error
	gotSeries string
}

func (f *fakeTournaments) TournamentSeries(_ context.Context, run int64, series string) (store.TournamentSeries, error) {
	f.calls++
	f.gotRun, f.gotSeries = run, series
	return f.series, f.err
}

func TestTournamentSeries(t *testing.T) {
	day := time.Date(2025, 10, 20, 0, 0, 0, 0, time.UTC)
	wc, six, two, one := "WC", 6, 2, 1
	games, wins, rate := 90, 54, 59.3
	f := &fakeTournaments{series: store.TournamentSeries{
		Games: 31674, Seasons: 34,
		Board: []store.TrophyRow{
			{Name: "ann", WorldChampion: true, Title: &wc, Gold: &six, Silver: &two, Bronze: &one},
			{Name: "eve", Games: &games, Wins: &wins, WinRate: &rate},
		},
		SeasonRecords: []store.SeasonRecord{{
			Name: "bob", Points: 36, Games: 7, Placings: []int{6, 1, 0, 0}, Season: 31, Division: "Grandmaster", LastPlayed: day,
		}},
		GameRecords: []store.GameRecord{{
			Name: "cat", WorldChampion: true, Score: 388, Opponents: []store.Seat{{Name: "dan", Score: 334}},
			Tournament: "International S29 - Grandmaster game 2", Date: day,
		}},
	}}
	rec, _ := serve(t, f, "/v1/runs/7/tournaments/international")
	want := `{"board":[{"bronze":1,"details":null,"games":null,"gold":6,"name":"ann","silver":2,"title":"WC","win_rate":null,"wins":null,"world_champion":true},` +
		`{"bronze":null,"details":null,"games":90,"gold":null,"name":"eve","silver":null,"title":null,"win_rate":59.3,"wins":54,"world_champion":false}],` +
		`"game_records":[{"date":"2025-10-20","name":"cat","opponents":[{"name":"dan","score":334}],"score":388,` +
		`"tournament":"International S29 - Grandmaster game 2","world_champion":true}],"games":31674,"run_id":7,` +
		`"season_records":[{"division":"Grandmaster","games":7,"last_played":"2025-10-20","name":"bob","placings":[6,1,0,0],` +
		`"points":36,"season":31,"world_champion":false}],"seasons":34,"series":"international"}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Errorf("GET series: %d %s\nwant 200 %s", rec.Code, got, want)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" || f.gotRun != 7 || f.gotSeries != "international" {
		t.Errorf("GET series: Cache-Control %q; asked run %d series %q", rec.Header().Get("Cache-Control"), f.gotRun, f.gotSeries)
	}
}

func TestTournamentSeriesKnowsEachSeries(t *testing.T) {
	for _, series := range []string{"international", "intermezzo", "royal_league", "worlds", "grand_slams", "ladders"} {
		f := &fakeTournaments{}
		rec, _ := serve(t, f, "/v1/runs/7/tournaments/"+series)
		if rec.Code != http.StatusOK || f.gotSeries != series {
			t.Errorf("GET %s: %d, asked for %q", series, rec.Code, f.gotSeries)
		}
	}
}

func TestTournamentSeriesEmptyListsAreArrays(t *testing.T) {
	rec, _ := serve(t, &fakeTournaments{}, "/v1/runs/7/tournaments/international")
	for _, want := range []string{`"board":[]`, `"season_records":[]`, `"game_records":[]`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("GET series with no results: %s, want %s", rec.Body.String(), want)
		}
	}
}

func TestTournamentSeriesRefusals(t *testing.T) {
	for path, msg := range map[string]string{
		"/v1/runs/7/tournaments/snail_cup":     "unknown series",
		"/v1/runs/7/tournaments/International": "unknown series",
		"/v1/runs/0/tournaments/international": "bad parameter: run",
	} {
		f := &fakeTournaments{}
		rec, _ := serve(t, f, path)
		if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusBadRequest || got != `{"error":"`+msg+`"}` || f.calls != 0 {
			t.Errorf("GET %s: %d %s; want 400 %q before the store", path, rec.Code, got, msg)
		}
	}
	rec, _ := serve(t, &fakeTournaments{err: store.ErrRunNotFound}, "/v1/runs/9/tournaments/international")
	if rec.Code != http.StatusNotFound || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("GET series of an unknown run: %d %q, want 404 no-store", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

type fakeHallOfFame struct {
	fakeRatings
	hof store.HallOfFame
	err error
}

func (f *fakeHallOfFame) HallOfFame(_ context.Context, run int64) (store.HallOfFame, error) {
	f.calls++
	f.gotRun = run
	return f.hof, f.err
}

func TestHallOfFame(t *testing.T) {
	one, cup := 1, "2025 Wimbledon TTA Champion"
	f := &fakeHallOfFame{hof: store.HallOfFame{
		Series:     []store.SeriesCount{{Series: "worlds", Games: 31121, Seasons: 5}},
		Legends:    []store.Legend{{Name: "ann", WorldChampion: true, WorldTitles: 1, InternationalTitles: 6, Gold: 7, Silver: 2}},
		GrandSlams: []store.TrophyRow{{Name: "bob", Gold: &one, Silver: &one, Bronze: &one, Details: &cup}},
	}}
	rec, _ := serve(t, f, "/v1/runs/7/tournaments")
	want := `{"grand_slams":[{"bronze":1,"details":"2025 Wimbledon TTA Champion","games":null,"gold":1,"name":"bob","silver":1,"title":null,"win_rate":null,"wins":null,"world_champion":false}],` +
		`"ladders":[],"legends":[{"bronze":0,"gold":7,"intermezzo_titles":0,"international_titles":6,"name":"ann","royal_league_titles":0,"silver":2,"world_champion":true,"world_titles":1}],` +
		`"run_id":7,"series":[{"games":31121,"seasons":5,"series":"worlds"}]}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Errorf("GET Hall of Fame: %d %s\nwant 200 %s", rec.Code, got, want)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" || f.gotRun != 7 {
		t.Errorf("GET Hall of Fame: Cache-Control %q, asked run %d", rec.Header().Get("Cache-Control"), f.gotRun)
	}
}

func TestHallOfFameRefusals(t *testing.T) {
	f := &fakeHallOfFame{}
	rec, _ := serve(t, f, "/v1/runs/0/tournaments")
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusBadRequest || got != `{"error":"bad parameter: run"}` || f.calls != 0 {
		t.Errorf("GET run 0: %d %s", rec.Code, got)
	}
	rec, _ = serve(t, &fakeHallOfFame{err: store.ErrRunNotFound}, "/v1/runs/9/tournaments")
	if rec.Code != http.StatusNotFound || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("GET an unknown run: %d %q, want 404 no-store", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

type fakeAchievements struct {
	fakeRatings
	summary *store.AchievementSummary
	records []store.TournamentRecord
	err     error
	asked   string
}

func (f *fakeAchievements) PlayerAchievements(_ context.Context, run int64, name string) (string, *store.AchievementSummary, []store.TournamentRecord, error) {
	f.calls++
	f.gotRun, f.asked = run, name
	return "Ann", f.summary, f.records, f.err
}

func TestPlayerAchievements(t *testing.T) {
	gold := "gold"
	f := &fakeAchievements{
		summary: &store.AchievementSummary{WorldTitles: 1, Gold: 2, Silver: 1, Accolades: []string{"2024 World Champion"}},
		records: []store.TournamentRecord{
			{Tournament: "Quick and Dirty (Q&D)", Season: "Career Trophy Hall of Fame", Division: "Championship",
				Placement: "2 Gold, 0 Silver, 1 Bronze", Points: "3 Medals", Medal: &gold,
				Details: "Q&D Hall of Fame Medalist", FinishDate: "Career / All-Time", Career: true},
			{Tournament: "TCL Round 31", Season: "Tournament", Division: "Main", Placement: "1 / 2", Points: "80 pts",
				Details: "Competed in Main &bull; Record", FinishDate: "2024-01-02"},
		},
	}
	rec, _ := serve(t, f, "/v1/runs/7/players/ann/achievements")
	want := `{"name":"Ann","records":[` +
		`{"career":true,"details":"Q\u0026D Hall of Fame Medalist","division":"Championship","finish_date":"Career / All-Time","medal":"gold","placement":"2 Gold, 0 Silver, 1 Bronze","points":"3 Medals","season":"Career Trophy Hall of Fame","tournament":"Quick and Dirty (Q\u0026D)"},` +
		`{"career":false,"details":"Competed in Main \u0026bull; Record","division":"Main","finish_date":"2024-01-02","medal":null,"placement":"1 / 2","points":"80 pts","season":"Tournament","tournament":"TCL Round 31"}],` +
		`"run_id":7,"summary":{"accolades":["2024 World Champion"],"bronze":0,"gold":2,"intermezzo_titles":0,"international_titles":0,"royal_league_titles":0,"silver":1,"world_titles":1}}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Errorf("GET achievements: %d %s\nwant 200 %s", rec.Code, got, want)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" || f.gotRun != 7 || f.asked != "ann" {
		t.Errorf("GET achievements: Cache-Control %q, asked run %d for %q", rec.Header().Get("Cache-Control"), f.gotRun, f.asked)
	}
}

func TestPlayerAchievementsWithoutMedalsOrRecords(t *testing.T) {
	rec, _ := serve(t, &fakeAchievements{}, "/v1/runs/7/players/ann/achievements")
	if got, want := strings.TrimSpace(rec.Body.String()), `{"name":"Ann","records":[],"run_id":7,"summary":null}`; rec.Code != http.StatusOK || got != want {
		t.Errorf("GET achievements of a player with none: %d %s, want 200 %s", rec.Code, got, want)
	}
}

func TestPlayerAchievementsRefusals(t *testing.T) {
	f := &fakeAchievements{}
	rec, _ := serve(t, f, "/v1/runs/0/players/ann/achievements")
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusBadRequest || got != `{"error":"bad parameter: run"}` || f.calls != 0 {
		t.Errorf("GET run 0: %d %s", rec.Code, got)
	}
	rec, _ = serve(t, f, "/v1/runs/7/players/"+strings.Repeat("a", 201)+"/achievements")
	if rec.Code != http.StatusBadRequest || f.calls != 0 {
		t.Errorf("GET a long name: %d, want 400 and no read", rec.Code)
	}
	for _, err := range []error{store.ErrRunNotFound, store.ErrPlayerNotFound} {
		rec, _ = serve(t, &fakeAchievements{err: err}, "/v1/runs/7/players/zed/achievements")
		if rec.Code != http.StatusNotFound || rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("GET with %v: %d %q, want 404 no-store", err, rec.Code, rec.Header().Get("Cache-Control"))
		}
	}
}
