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
	view                store.CatalogueView
	tournament          store.CatalogueTournament
	games               store.CatalogueGames
	podiums             []store.RecentPodium
	err                 error
	gotSlug             string
	gotFilter           store.CatalogueGamesFilter
	gotPlayed, gotShown int
}

func (f *fakeCatalogue) RecentPodiums(_ context.Context, run int64, played, shown int) ([]store.RecentPodium, error) {
	f.calls++
	f.gotRun, f.gotPlayed, f.gotShown = run, played, shown
	return f.podiums, f.err
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
	if rec.Code != http.StatusOK {
		t.Fatalf("GET catalogue = %d %s", rec.Code, rec.Body)
	}
	body := rec.Body.Bytes()
	wantJSON(t, body, "run_id", `7`)
	wantJSON(t, body, "cge_tournaments", `[{"cge_tournament":5373,"finished":"2025-11-19","name":"Nations Cup","organiser":"","rated":false,
		"started":null,"status":"finished","tournament":"nations_cup","url":"https://account.czechgames.com/tournaments/detail/5373"}]`)
	wantJSON(t, body, "tournaments", `[{"slug":"intermezzo","name":"Intermezzo","first_game":"2019-03-10","last_game":"2026-05-31",
		"games":25342,"players":1900,"seasons":30,"rankings":["community"]}]`)
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
		Label: "2024", InChampions: true, Span: span(632, 128, "2024-07-01", "2025-02-24"),
		Stages: []store.CatalogueStage{{Label: "Stage 7", Span: span(5, 2, "2025-02-03", "2025-02-24"), Note: "Played at the club."}},
		Podium: []store.PodiumPlace{{Place: 1, Name: "sora", Finished: finished, Source: "CGE /tournaments/detail/4991/7"}},
		CGE:    []store.CGELink{{Tournament: 4991}, {Tournament: 19, Season: 20}},
		Note:   "The first held in two halves.",
	}}, Ladder: true, TopTierWins: []store.LadderWin{{Name: "wren", Wins: 10, Source: "Hall of Fame"}, {Name: "Grebe", Wins: 3.5, Source: "Hall of Fame"}},
		Players: []string{"Grebe", "sora"}, Rules: []store.EditionRules{{CGETournament: 4991, Name: "Wimbledon 2024", Description: "2p knock-out."}}}}
	rec, _ := serve(t, f, "/v1/runs/7/catalogue/wimbledon")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET a tournament = %d %s", rec.Code, rec.Body)
	}
	body := rec.Body.Bytes()
	wantJSON(t, body, "run_id", `7`)
	wantJSON(t, body, "slug", `"wimbledon"`)
	wantJSON(t, body, "name", `"Wimbledon"`)
	wantJSON(t, body, "ladder", `true`)
	wantJSON(t, body, "players", `["Grebe","sora"]`)
	wantJSON(t, body, "rules", `[{"cge_tournament":4991,"description":"2p knock-out.","name":"Wimbledon 2024",
		"url":"https://account.czechgames.com/tournaments/detail/4991"}]`)
	wantJSON(t, body, "seasons.0", `{"label":"2024","first_game":"2024-07-01","last_game":"2025-02-24","games":632,"players":128,
		"note":"The first held in two halves.",
		"cge_pages":["https://account.czechgames.com/tournaments/detail/4991","https://account.czechgames.com/tournaments/detail/19/20"]}`)
	wantJSON(t, body, "seasons.0.podium", `[{"final":"","finished":"2025-02-24","name":"sora","place":1,"source":"CGE /tournaments/detail/4991/7"}]`)
	wantJSON(t, body, "seasons.0.stages", `[{"first_game":"2025-02-03","games":5,"label":"Stage 7","last_game":"2025-02-24",
		"note":"Played at the club.","players":2}]`)
	wantJSON(t, body, "top_tier_wins", `[{"name":"wren","source":"Hall of Fame","wins":10},{"name":"Grebe","source":"Hall of Fame","wins":3.5}]`)
	wantJSON(t, body, "stories", `[]`)
	wantJSON(t, body, "game_records", `[]`)
	if rec.Header().Get("Cache-Control") != "public, max-age=300" || f.gotRun != 7 || f.gotSlug != "wimbledon" {
		t.Errorf("GET a tournament: Cache-Control %q; asked run %d, %q", rec.Header().Get("Cache-Control"), f.gotRun, f.gotSlug)
	}
}

func TestRecentPodiums(t *testing.T) {
	finished, _ := time.Parse(time.DateOnly, "2025-02-24")
	f := &fakeCatalogue{podiums: []store.RecentPodium{
		{Slug: "wimbledon", Name: "Wimbledon", Season: "2024", Finished: finished,
			Podium: []store.PodiumPlace{{Place: 1, Name: "sora", Finished: finished, Source: "CGE"}}},
		{Slug: "slow_burn", Name: "Slow Burn", Season: "3", SeasonName: "Edition", Finished: finished, Podium: []store.PodiumPlace{}},
	}}
	rec, _ := serve(t, f, "/v1/runs/7/podiums")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET podiums = %d %s", rec.Code, rec.Body)
	}
	body := rec.Body.Bytes()
	wantJSON(t, body, "run_id", `7`)
	wantJSON(t, body, "podiums.0", `{"slug":"wimbledon","name":"Wimbledon","season":"2024","season_name":"Season","finished":"2025-02-24",
		"podium":[{"final":"","finished":"2025-02-24","name":"sora","place":1,"source":"CGE"}]}`)
	wantJSON(t, body, "podiums.1.season_name", `"Edition"`)
	if rec.Header().Get("Cache-Control") != "public, max-age=300" || f.gotRun != 7 || f.gotPlayed != 12 || f.gotShown != 8 {
		t.Errorf("GET podiums: Cache-Control %q; asked run %d for %d of the %d played last, want 8 of 12",
			rec.Header().Get("Cache-Control"), f.gotRun, f.gotShown, f.gotPlayed)
	}
}

func TestRecentPodiumsEmptyIsAnArray(t *testing.T) {
	rec, _ := serve(t, &fakeCatalogue{}, "/v1/runs/7/podiums")
	if !strings.Contains(rec.Body.String(), `"podiums":[]`) {
		t.Errorf("GET no podiums: %s, want an empty array", rec.Body.String())
	}
}

func TestCatalogueTournamentNamesItsSeasonsAndStages(t *testing.T) {
	for _, tt := range []struct {
		name, season, stage, wantSeason, wantStage string
	}{
		{"the defaults", "", "", `"Season"`, `"Stage"`},
		{"its own words", "Edition", "Season", `"Edition"`, `"Season"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeCatalogue{tournament: store.CatalogueTournament{Slug: "slow_burn", Name: "Slow Burn", SeasonName: tt.season, StageName: tt.stage}}
			rec, _ := serve(t, f, "/v1/runs/7/catalogue/slow_burn")
			wantJSON(t, rec.Body.Bytes(), "season_name", tt.wantSeason)
			wantJSON(t, rec.Body.Bytes(), "stage_name", tt.wantStage)
		})
	}
}

func TestCatalogueGames(t *testing.T) {
	d, _ := time.Parse(time.DateOnly, "2025-02-24")
	f := &fakeCatalogue{games: store.CatalogueGames{Total: 2, Coded: 1, Page: 1, Pages: 1, Groups: []string{"Final"}, Games: []store.CatalogueGame{
		{ID: 9, Date: d, Season: "2024", Stage: "Stage 7", Group: "Final", Game: 1, State: "played", Code: "HEKANUJIMA",
			Seats: []store.GameSeat{{Name: "alder440", Score: 112, Outcome: "finished"}, {Name: "kittiwake", Score: -5, Outcome: "timed_out"}}},
		{ID: 8, Date: d, Season: "2024", Stage: "Stage 7", State: "never_played", Seats: []store.GameSeat{{Name: "ann", Score: 0}}},
	}}}
	rec, _ := serve(t, f, "/v1/runs/7/catalogue/wimbledon/games?season=2024&stage=Stage+7&group=Final&player=alder440&coded=true&page=2")
	want := `{"coded":1,"games":[` +
		`{"date":"2025-02-24","game":1,"group":"Final","match_id":9,"rated":false,"replay_code":"HEKANUJIMA","season":"2024",` +
		`"seats":[{"name":"alder440","outcome":"finished","score":112},{"name":"kittiwake","outcome":"timed_out","score":-5}],"stage":"Stage 7","state":"played"},` +
		`{"date":"2025-02-24","game":null,"group":"","match_id":8,"rated":false,"replay_code":null,"season":"2024",` +
		`"seats":[{"name":"ann","outcome":null,"score":0}],"stage":"Stage 7","state":"never_played"}],` +
		`"groups":["Final"],"page":1,"pages":1,"run_id":7,"slug":"wimbledon","total":2}`
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != want {
		t.Errorf("GET games: %d %s\nwant 200 %s", rec.Code, got, want)
	}
	wantFilter := store.CatalogueGamesFilter{Season: "2024", Stage: "Stage 7", Group: "Final", Player: "alder440", Coded: true, Page: 2}
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
		"/v1/runs/0/podiums":                                                     "bad parameter: run",
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
		"/v1/runs/9/podiums":                   store.ErrRunNotFound,
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
	if !strings.Contains(rec.Body.String(), `"first_game":null,"games":0,"in_champions":false,"label":"1","last_game":null,"note":"Before the app."`) {
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

func TestCatalogueSendsEachTournamentsCard(t *testing.T) {
	card := store.Card{Icon: "🎾", Tagline: "Grass-court slam", Summary: "Played each July.", Subtitle: "Knock-out", Cadence: "Annual"}
	f := &fakeCatalogue{
		view:       store.CatalogueView{Tournaments: []store.CatalogueEntry{{Slug: "wimbledon", Name: "Wimbledon", Rankings: []string{}, Card: card}}},
		tournament: store.CatalogueTournament{Slug: "wimbledon", Name: "Wimbledon", Card: card},
	}
	want := `"card":{"cadence":"Annual","icon":"🎾","subtitle":"Knock-out","summary":"Played each July.","tagline":"Grass-court slam"}`
	for _, path := range []string{"/v1/runs/7/catalogue", "/v1/runs/7/catalogue/wimbledon"} {
		rec, _ := serve(t, f, path)
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("GET %s = %s, want the card", path, rec.Body)
		}
	}
}
