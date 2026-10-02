package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

type fakeCatalogue struct {
	fakeRatings
	view       store.CatalogueView
	tournament store.CatalogueTournament
	err        error
	gotSlug    string
}

func (f *fakeCatalogue) Catalogue(_ context.Context, run int64) (store.CatalogueView, error) {
	f.calls++
	f.gotRun = run
	return f.view, f.err
}

func (f *fakeCatalogue) CatalogueTournament(_ context.Context, run int64, slug string) (store.CatalogueTournament, error) {
	f.calls++
	f.gotRun, f.gotSlug = run, slug
	return f.tournament, f.err
}

func span(games, players int, first, last string) store.GameSpan {
	parse := func(s string) time.Time { d, _ := time.Parse(time.DateOnly, s); return d }
	return store.GameSpan{Games: games, Players: players, First: parse(first), Last: parse(last)}
}

func TestCatalogue(t *testing.T) {
	finished, _ := time.Parse(time.DateOnly, "2025-11-19")
	f := &fakeCatalogue{view: store.CatalogueView{
		Tournaments: []store.CatalogueEntry{
			{Slug: "intermezzo", Name: "Intermezzo", Seasons: 30, Rankings: []string{"community"}, Span: span(25342, 1900, "2019-03-10", "2026-05-31")},
		},
		CGE: []store.CGEEntry{
			{ID: 5373, Name: "Nations Cup", Status: "finished", Finished: &finished, Tournament: "nations_cup"},
		},
	}}
	rec, _ := serve(t, f, "/v1/runs/7/catalogue")
	want := `{"cge_tournaments":[{"cge_tournament":5373,"finished":"2025-11-19","name":"Nations Cup","organiser":"","rated":false,"started":null,` +
		`"status":"finished","tournament":"nations_cup","url":"https://account.czechgames.com/tournaments/detail/5373"}],"run_id":7,` +
		`"tournaments":[{"first_game":"2019-03-10","games":25342,"last_game":"2026-05-31","name":"Intermezzo","players":1900,"rankings":["community"],"seasons":30,"slug":"intermezzo"}]}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Errorf("GET catalogue: %d %s\nwant 200 %s", rec.Code, got, want)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" || f.gotRun != 7 {
		t.Errorf("GET catalogue: Cache-Control %q, asked run %d", rec.Header().Get("Cache-Control"), f.gotRun)
	}
}

func TestCatalogueEmptyIsAnArray(t *testing.T) {
	rec, _ := serve(t, &fakeCatalogue{}, "/v1/runs/7/catalogue")
	if !strings.Contains(rec.Body.String(), `"tournaments":[]`) || !strings.Contains(rec.Body.String(), `"cge_tournaments":[]`) {
		t.Errorf("GET an empty catalogue: %s, want an empty array", rec.Body.String())
	}
}

func TestCatalogueTournament(t *testing.T) {
	finished, _ := time.Parse(time.DateOnly, "2025-02-24")
	f := &fakeCatalogue{tournament: store.CatalogueTournament{Slug: "wimbledon", Name: "Wimbledon", Seasons: []store.CatalogueSeason{{
		Label: "2024", Span: span(632, 128, "2024-07-01", "2025-02-24"),
		Stages: []store.CatalogueStage{{Label: "Stage 7", Span: span(5, 2, "2025-02-03", "2025-02-24")}},
		Podium: []store.PodiumPlace{{Place: 1, Name: "saru", Finished: finished, Source: "CGE /tournaments/detail/4991/7"}},
	}}}}
	rec, _ := serve(t, f, "/v1/runs/7/catalogue/wimbledon")
	want := `{"name":"Wimbledon","run_id":7,"seasons":[{"first_game":"2024-07-01","games":632,"label":"2024","last_game":"2025-02-24","players":128,` +
		`"podium":[{"final":"","finished":"2025-02-24","name":"saru","place":1,"source":"CGE /tournaments/detail/4991/7"}],` +
		`"stages":[{"first_game":"2025-02-03","games":5,"label":"Stage 7","last_game":"2025-02-24","players":2}]}],"slug":"wimbledon"}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Errorf("GET a tournament: %d %s\nwant 200 %s", rec.Code, got, want)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" || f.gotRun != 7 || f.gotSlug != "wimbledon" {
		t.Errorf("GET a tournament: Cache-Control %q; asked run %d, %q", rec.Header().Get("Cache-Control"), f.gotRun, f.gotSlug)
	}
}

func TestCatalogueRefusals(t *testing.T) {
	for path, msg := range map[string]string{
		"/v1/runs/0/catalogue":           "bad parameter: run",
		"/v1/runs/0/catalogue/wimbledon": "bad parameter: run",
		"/v1/runs/7/catalogue/Wimbledon": "bad parameter: tournament",
		"/v1/runs/7/catalogue/9cup":      "bad parameter: tournament",
	} {
		f := &fakeCatalogue{}
		rec, _ := serve(t, f, path)
		if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusBadRequest || got != `{"error":"`+msg+`"}` || f.calls != 0 {
			t.Errorf("GET %s: %d %s; want 400 %q before the store", path, rec.Code, got, msg)
		}
	}
	for path, err := range map[string]error{
		"/v1/runs/9/catalogue":           store.ErrRunNotFound,
		"/v1/runs/9/catalogue/wimbledon": store.ErrRunNotFound,
		"/v1/runs/7/catalogue/snail_cup": store.ErrTournamentNotFound,
	} {
		rec, _ := serve(t, &fakeCatalogue{err: err}, path)
		if rec.Code != http.StatusNotFound || rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("GET %s: %d %q, want 404 no-store", path, rec.Code, rec.Header().Get("Cache-Control"))
		}
	}
}
