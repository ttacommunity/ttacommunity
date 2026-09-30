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
	f := &fakeTournaments{series: store.TournamentSeries{
		Games: 31674, Seasons: 34,
		Board: []store.TrophyRow{{Name: "ann", Title: "WC", Gold: 6, Silver: 2, Bronze: 1}},
		SeasonRecords: []store.SeasonRecord{{
			Name: "bob", Points: 36, Games: 7, Placings: []int{6, 1, 0, 0}, Season: 31, Division: "Grandmaster", LastPlayed: day,
		}},
		GameRecords: []store.GameRecord{{
			Name: "cat", WorldChampion: true, Score: 388, Opponents: []store.Seat{{Name: "dan", Score: 334}},
			Tournament: "International S29 - Grandmaster game 2", Date: day,
		}},
	}}
	rec, _ := serve(t, f, "/v1/runs/7/tournaments/international")
	want := `{"board":[{"bronze":1,"gold":6,"name":"ann","silver":2,"title":"WC"}],` +
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
	for _, series := range []string{"international", "intermezzo", "royal_league"} {
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
		"/v1/runs/7/tournaments/worlds":        "unknown series",
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
