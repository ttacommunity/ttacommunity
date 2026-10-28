package httpapi

import (
	"cmp"
	"context"
	"fmt"
	"regexp"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"ttacommunity/backend/internal/store"
)

// cgeSite is where CGE's tournament pages are.
const cgeSite = "https://account.czechgames.com"

// tournamentSlug is the spec's pattern for a tournament in a path.
var tournamentSlug = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// GetCatalogue returns every tournament with placed games.
func (s *server) GetCatalogue(ctx context.Context, req GetCatalogueRequestObject) (GetCatalogueResponseObject, error) {
	if req.Run < 1 {
		return GetCatalogue400JSONResponse{badRequestBody("bad parameter: run")}, nil
	}
	view, err := s.ratings.Catalogue(ctx, req.Run)
	if nf, ok := notFound(err); ok {
		return GetCatalogue404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}
	body := Catalogue{
		RunId:          req.Run,
		Tournaments:    make([]CatalogueEntry, 0, len(view.Tournaments)),
		CgeTournaments: make([]CGETournament, 0, len(view.CGE)),
	}
	for _, e := range view.Tournaments {
		body.Tournaments = append(body.Tournaments, CatalogueEntry{
			Slug: e.Slug, Name: e.Name, Card: tournamentCard(e.Card), Seasons: e.Seasons, Rankings: append([]string{}, e.Rankings...),
			Games: e.Span.Games, Players: e.Span.Players,
			FirstGame: openapi_types.Date{Time: e.Span.First}, LastGame: openapi_types.Date{Time: e.Span.Last},
		})
	}
	for _, c := range view.CGE {
		body.CgeTournaments = append(body.CgeTournaments, CGETournament{
			CgeTournament: c.ID, Name: c.Name, Status: c.Status, Organiser: c.Organiser,
			Started: optionalDate(c.Started), Finished: optionalDate(c.Finished),
			Url: fmt.Sprintf("%s/tournaments/detail/%d", cgeSite, c.ID), Tournament: c.Tournament, Rated: c.Rated,
		})
	}
	return GetCatalogue200JSONResponse{Body: body, Headers: GetCatalogue200ResponseHeaders{CacheControl: cacheRun}}, nil
}

// GetRecentPodiums returns the latest podiums of the twelve tournaments played last, the eight finished last first.
func (s *server) GetRecentPodiums(ctx context.Context, req GetRecentPodiumsRequestObject) (GetRecentPodiumsResponseObject, error) {
	if req.Run < 1 {
		return GetRecentPodiums400JSONResponse{badRequestBody("bad parameter: run")}, nil
	}
	podiums, err := s.ratings.RecentPodiums(ctx, req.Run, 12, 8)
	if nf, ok := notFound(err); ok {
		return GetRecentPodiums404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}
	body := RecentPodiums{RunId: req.Run, Podiums: make([]RecentPodium, 0, len(podiums))}
	for _, p := range podiums {
		out := RecentPodium{
			Slug: p.Slug, Name: p.Name, Season: p.Season, SeasonName: cmp.Or(p.SeasonName, "Season"),
			Finished: openapi_types.Date{Time: p.Finished}, Podium: make([]PodiumPlace, 0, len(p.Podium)),
		}
		for _, place := range p.Podium {
			out.Podium = append(out.Podium, PodiumPlace{
				Place: place.Place, Name: place.Name, Final: place.Final, Finished: openapi_types.Date{Time: place.Finished}, Source: place.Source,
			})
		}
		body.Podiums = append(body.Podiums, out)
	}
	return GetRecentPodiums200JSONResponse{Body: body, Headers: GetRecentPodiums200ResponseHeaders{CacheControl: cacheRun}}, nil
}

// GetCatalogueTournament returns one tournament's seasons, stages and podiums.
func (s *server) GetCatalogueTournament(ctx context.Context, req GetCatalogueTournamentRequestObject) (GetCatalogueTournamentResponseObject, error) {
	switch {
	case req.Run < 1:
		return GetCatalogueTournament400JSONResponse{badRequestBody("bad parameter: run")}, nil
	case !tournamentSlug.MatchString(req.Tournament):
		return GetCatalogueTournament400JSONResponse{badRequestBody("bad parameter: tournament")}, nil
	}
	t, err := s.ratings.CatalogueTournament(ctx, req.Run, req.Tournament)
	if nf, ok := notFound(err); ok {
		return GetCatalogueTournament404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}
	body := CatalogueTournament{
		RunId: req.Run, Slug: t.Slug, Name: t.Name, Card: tournamentCard(t.Card), Rated: t.Rated, GameRecords: gameRecords(t.GameRecords), Stories: storyList(t.Stories), Seasons: make([]CatalogueSeason, 0, len(t.Seasons)),
		Ladder: t.Ladder, TopTierWins: make([]LadderWin, 0, len(t.TopTierWins)), Players: append([]string{}, t.Players...),
		SeasonName: cmp.Or(t.SeasonName, "Season"), StageName: cmp.Or(t.StageName, "Stage"),
	}
	if t.Regulations != "" {
		body.Regulations = &t.Regulations
	}
	body.Rules = make([]EditionRules, 0, len(t.Rules))
	for _, e := range t.Rules {
		body.Rules = append(body.Rules, EditionRules{
			CgeTournament: e.CGETournament, Name: e.Name, Description: e.Description,
			Url: fmt.Sprintf("%s/tournaments/detail/%d", cgeSite, e.CGETournament),
		})
	}
	for _, w := range t.TopTierWins {
		body.TopTierWins = append(body.TopTierWins, LadderWin{Name: w.Name, Wins: w.Wins, Source: w.Source})
	}
	for _, season := range t.Seasons {
		out := CatalogueSeason{
			Label: season.Label, InChampions: season.InChampions, Games: season.Span.Games, Players: season.Span.Players,
			Stages: make([]CatalogueStage, 0, len(season.Stages)), Podium: make([]PodiumPlace, 0, len(season.Podium)),
			CgePages: make([]string, 0, len(season.CGE)),
		}
		if season.Span.Games > 0 {
			out.FirstGame = &openapi_types.Date{Time: season.Span.First}
			out.LastGame = &openapi_types.Date{Time: season.Span.Last}
		}
		if season.Note != "" {
			out.Note = &season.Note
		}
		for _, l := range season.CGE {
			page := fmt.Sprintf("%s/tournaments/detail/%d", cgeSite, l.Tournament)
			if l.Season > 0 {
				page += fmt.Sprintf("/%d", l.Season)
			}
			out.CgePages = append(out.CgePages, page)
		}
		for _, st := range season.Stages {
			stage := CatalogueStage{
				Label: st.Label, Games: st.Span.Games, Players: st.Span.Players,
				FirstGame: openapi_types.Date{Time: st.Span.First}, LastGame: openapi_types.Date{Time: st.Span.Last},
			}
			if st.Note != "" {
				stage.Note = &st.Note
			}
			out.Stages = append(out.Stages, stage)
		}
		for _, p := range season.Podium {
			out.Podium = append(out.Podium, PodiumPlace{
				Place: p.Place, Name: p.Name, Final: p.Final, Finished: openapi_types.Date{Time: p.Finished}, Source: p.Source,
			})
		}
		body.Seasons = append(body.Seasons, out)
	}
	return GetCatalogueTournament200JSONResponse{
		Body: body, Headers: GetCatalogueTournament200ResponseHeaders{CacheControl: cacheRun},
	}, nil
}

// GetCatalogueGames returns one page of a tournament's games, narrowed as asked.
func (s *server) GetCatalogueGames(ctx context.Context, req GetCatalogueGamesRequestObject) (GetCatalogueGamesResponseObject, error) {
	f := store.CatalogueGamesFilter{Page: 1}
	text := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	f.Season, f.Stage, f.Group, f.Player = text(req.Params.Season), text(req.Params.Stage), text(req.Params.Group), text(req.Params.Player)
	if req.Params.Coded != nil {
		f.Coded = *req.Params.Coded
	}
	if req.Params.State != nil {
		if !req.Params.State.Valid() {
			return GetCatalogueGames400JSONResponse{badRequestBody("bad parameter: state")}, nil
		}
		f.State = string(*req.Params.State)
	}
	if req.Params.Page != nil {
		f.Page = *req.Params.Page
	}
	var msg string
	switch {
	case req.Run < 1:
		msg = "bad parameter: run"
	case !tournamentSlug.MatchString(req.Tournament):
		msg = "bad parameter: tournament"
	case len(f.Season) > 64:
		msg = "bad parameter: season"
	case len(f.Stage) > 64:
		msg = "bad parameter: stage"
	case len(f.Group) > 64:
		msg = "bad parameter: group"
	case len(f.Player) > 64:
		msg = "bad parameter: player"
	case f.Page < 1:
		msg = "bad parameter: page"
	}
	if msg != "" {
		return GetCatalogueGames400JSONResponse{badRequestBody(msg)}, nil
	}
	got, err := s.ratings.CatalogueGames(ctx, req.Run, req.Tournament, f)
	if nf, ok := notFound(err); ok {
		return GetCatalogueGames404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}
	body := CatalogueGames{
		RunId: req.Run, Slug: req.Tournament, Page: got.Page, Pages: got.Pages, Total: got.Total, Coded: got.Coded,
		Groups: append([]string{}, got.Groups...), Games: make([]CatalogueGame, 0, len(got.Games)),
	}
	for _, g := range got.Games {
		out := CatalogueGame{
			MatchId: g.ID, Date: openapi_types.Date{Time: g.Date}, Season: g.Season, Stage: g.Stage, Group: g.Group,
			State: GameState(g.State), Seats: make([]GameSeat, 0, len(g.Seats)), Rated: g.Rated,
		}
		if g.Game > 0 {
			out.Game = &g.Game
		}
		if g.Code != "" {
			out.ReplayCode = &g.Code
		}
		for _, seat := range g.Seats {
			gs := GameSeat{Name: seat.Name, Score: seat.Score}
			if seat.Outcome != "" {
				o := SeatOutcome(seat.Outcome)
				gs.Outcome = &o
			}
			out.Seats = append(out.Seats, gs)
		}
		body.Games = append(body.Games, out)
	}
	return GetCatalogueGames200JSONResponse{
		Body: body, Headers: GetCatalogueGames200ResponseHeaders{CacheControl: cacheRun},
	}, nil
}

// optionalDate returns t as an API date, or nil for none.
func optionalDate(t *time.Time) *openapi_types.Date {
	if t == nil {
		return nil
	}
	return &openapi_types.Date{Time: *t}
}
