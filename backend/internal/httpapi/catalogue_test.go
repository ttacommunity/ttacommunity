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
	games      store.CatalogueGames
	err        error
	gotSlug    string
	gotFilter  store.CatalogueGamesFilter
}

func (f *fakeCatalogue) CatalogueGames(_ context.Context, run int64, slug string, filter store.CatalogueGamesFilter) (store.CatalogueGames, error) {
	f.calls++
	f.gotRun, f.gotSlug, f.gotFilter = run, slug, filter
	return f.games, f.err
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
		Stages: []store.CatalogueStage{{Label: "Stage 7", Span: span(5, 2, "2025-02-03", "2025-02-24"), Note: "Played at the club."}},
		Podium: []store.PodiumPlace{{Place: 1, Name: "saru", Finished: finished, Source: "CGE /tournaments/detail/4991/7"}},
		CGE:    []store.CGELink{{Tournament: 4991}, {Tournament: 19, Season: 20}},
		Note:   "The first held in two halves.",
	}}, Ladder: true, TopTierWins: []store.LadderWin{{Name: "wolvs", Wins: 10, Source: "Hall of Fame"}, {Name: "Grozz", Wins: 3.5, Source: "Hall of Fame"}},
		Players: []string{"Grozz", "saru"}, Rules: []store.EditionRules{{CGETournament: 4991, Name: "Wimbledon 2024", Description: "2p knock-out."}}}}
	rec, _ := serve(t, f, "/v1/runs/7/catalogue/wimbledon")
	want := `{"ladder":true,"name":"Wimbledon","players":["Grozz","saru"],` +
		`"rules":[{"cge_tournament":4991,"description":"2p knock-out.","name":"Wimbledon 2024","url":"https://account.czechgames.com/tournaments/detail/4991"}],"run_id":7,"seasons":[{"cge_pages":["https://account.czechgames.com/tournaments/detail/4991","https://account.czechgames.com/tournaments/detail/19/20"],` +
		`"first_game":"2024-07-01","games":632,"label":"2024","last_game":"2025-02-24","note":"The first held in two halves.","players":128,` +
		`"podium":[{"final":"","finished":"2025-02-24","name":"saru","place":1,"source":"CGE /tournaments/detail/4991/7"}],` +
		`"stages":[{"first_game":"2025-02-03","games":5,"label":"Stage 7","last_game":"2025-02-24","note":"Played at the club.","players":2}]}],"slug":"wimbledon",` +
		`"top_tier_wins":[{"name":"wolvs","source":"Hall of Fame","wins":10},{"name":"Grozz","source":"Hall of Fame","wins":3.5}]}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Errorf("GET a tournament: %d %s\nwant 200 %s", rec.Code, got, want)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" || f.gotRun != 7 || f.gotSlug != "wimbledon" {
		t.Errorf("GET a tournament: Cache-Control %q; asked run %d, %q", rec.Header().Get("Cache-Control"), f.gotRun, f.gotSlug)
	}
}

func TestCatalogueGames(t *testing.T) {
	d, _ := time.Parse(time.DateOnly, "2025-02-24")
	f := &fakeCatalogue{games: store.CatalogueGames{Total: 2, Coded: 1, Page: 1, Pages: 1, Groups: []string{"Final"}, Games: []store.CatalogueGame{
		{ID: 9, Date: d, Season: "2024", Stage: "Stage 7", Group: "Final", Game: 1, State: "played", Code: "HEKANUJIMA",
			Seats: []store.GameSeat{{Name: "a440", Score: 112, Outcome: "finished"}, {Name: "kyshantry", Score: -5, Outcome: "timed_out"}}},
		{ID: 8, Date: d, Season: "2024", Stage: "Stage 7", State: "never_played", Seats: []store.GameSeat{{Name: "ann", Score: 0}}},
	}}}
	rec, _ := serve(t, f, "/v1/runs/7/catalogue/wimbledon/games?season=2024&stage=Stage+7&group=Final&player=a440&coded=true&page=2")
	want := `{"coded":1,"games":[` +
		`{"date":"2025-02-24","game":1,"group":"Final","match_id":9,"replay_code":"HEKANUJIMA","season":"2024",` +
		`"seats":[{"name":"a440","outcome":"finished","score":112},{"name":"kyshantry","outcome":"timed_out","score":-5}],"stage":"Stage 7","state":"played"},` +
		`{"date":"2025-02-24","game":null,"group":"","match_id":8,"replay_code":null,"season":"2024",` +
		`"seats":[{"name":"ann","outcome":null,"score":0}],"stage":"Stage 7","state":"never_played"}],` +
		`"groups":["Final"],"page":1,"pages":1,"run_id":7,"slug":"wimbledon","total":2}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Errorf("GET games: %d %s\nwant 200 %s", rec.Code, got, want)
	}
	wantFilter := store.CatalogueGamesFilter{Season: "2024", Stage: "Stage 7", Group: "Final", Player: "a440", Coded: true, Page: 2}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" || f.gotRun != 7 || f.gotSlug != "wimbledon" || f.gotFilter != wantFilter {
		t.Errorf("GET games: Cache-Control %q; asked run %d, %q, %+v", rec.Header().Get("Cache-Control"), f.gotRun, f.gotSlug, f.gotFilter)
	}
}

func TestCatalogueGamesDefaultsAndEmpty(t *testing.T) {
	f := &fakeCatalogue{games: store.CatalogueGames{Page: 1, Pages: 1}}
	rec, _ := serve(t, f, "/v1/runs/7/catalogue/wimbledon/games")
	if f.gotFilter != (store.CatalogueGamesFilter{Page: 1}) {
		t.Errorf("GET games with no filters asked %+v, want page 1 only", f.gotFilter)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"games":[]`) || !strings.Contains(body, `"groups":[]`) {
		t.Errorf("GET no games: %s, want empty arrays", body)
	}
}

func TestCatalogueRefusals(t *testing.T) {
	for path, msg := range map[string]string{
		"/v1/runs/0/catalogue":                                                   "bad parameter: run",
		"/v1/runs/0/catalogue/wimbledon":                                         "bad parameter: run",
		"/v1/runs/7/catalogue/Wimbledon":                                         "bad parameter: tournament",
		"/v1/runs/7/catalogue/9cup":                                              "bad parameter: tournament",
		"/v1/runs/0/catalogue/wimbledon/games":                                   "bad parameter: run",
		"/v1/runs/7/catalogue/Wimbledon/games":                                   "bad parameter: tournament",
		"/v1/runs/7/catalogue/wimbledon/games?page=0":                            "bad parameter: page",
		"/v1/runs/7/catalogue/wimbledon/games?season=" + strings.Repeat("x", 65): "bad parameter: season",
		"/v1/runs/7/catalogue/wimbledon/games?player=" + strings.Repeat("x", 65): "bad parameter: player",
	} {
		f := &fakeCatalogue{}
		rec, _ := serve(t, f, path)
		if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusBadRequest || got != `{"error":"`+msg+`"}` || f.calls != 0 {
			t.Errorf("GET %s: %d %s; want 400 %q before the store", path, rec.Code, got, msg)
		}
	}
	for path, err := range map[string]error{
		"/v1/runs/9/catalogue":                 store.ErrRunNotFound,
		"/v1/runs/9/catalogue/wimbledon":       store.ErrRunNotFound,
		"/v1/runs/7/catalogue/snail_cup":       store.ErrTournamentNotFound,
		"/v1/runs/7/catalogue/snail_cup/games": store.ErrTournamentNotFound,
	} {
		rec, _ := serve(t, &fakeCatalogue{err: err}, path)
		if rec.Code != http.StatusNotFound || rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("GET %s: %d %q, want 404 no-store", path, rec.Code, rec.Header().Get("Cache-Control"))
		}
	}
}

func TestCatalogueTournamentSendsNoDatesForASeasonWithoutGames(t *testing.T) {
	f := &fakeCatalogue{tournament: store.CatalogueTournament{Slug: "international", Name: "International", Seasons: []store.CatalogueSeason{{
		Label: "1", Stages: []store.CatalogueStage{}, Podium: []store.PodiumPlace{}, CGE: []store.CGELink{}, Note: "Before the app.",
	}}}}
	rec, _ := serve(t, f, "/v1/runs/7/catalogue/international")
	if !strings.Contains(rec.Body.String(), `"first_game":null,"games":0,"label":"1","last_game":null,"note":"Before the app."`) {
		t.Errorf("a season without games = %s, want null dates and its note", rec.Body)
	}
}

func TestCatalogueTournamentSendsAdminsRegulationsOnlyWhenWritten(t *testing.T) {
	for _, tt := range []struct{ regulations, want string }{
		{"Best of three.", `"regulations":"Best of three."`},
		{"", `"regulations"`},
	} {
		f := &fakeCatalogue{tournament: store.CatalogueTournament{Slug: "wimbledon", Name: "Wimbledon", Regulations: tt.regulations}}
		rec, _ := serve(t, f, "/v1/runs/7/catalogue/wimbledon")
		if got := strings.Contains(rec.Body.String(), tt.want); got != (tt.regulations != "") {
			t.Errorf("regulations %q: body = %s", tt.regulations, rec.Body)
		}
	}
}
