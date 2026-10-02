package httpapi

import (
	"context"
	"fmt"
	"regexp"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
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
			Slug: e.Slug, Name: e.Name, Seasons: e.Seasons, Rankings: append([]string{}, e.Rankings...),
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
	body := CatalogueTournament{RunId: req.Run, Slug: t.Slug, Name: t.Name, Seasons: make([]CatalogueSeason, 0, len(t.Seasons))}
	for _, season := range t.Seasons {
		out := CatalogueSeason{
			Label: season.Label, Games: season.Span.Games, Players: season.Span.Players,
			FirstGame: openapi_types.Date{Time: season.Span.First}, LastGame: openapi_types.Date{Time: season.Span.Last},
			Stages: make([]CatalogueStage, 0, len(season.Stages)), Podium: make([]PodiumPlace, 0, len(season.Podium)),
		}
		for _, st := range season.Stages {
			out.Stages = append(out.Stages, CatalogueStage{
				Label: st.Label, Games: st.Span.Games, Players: st.Span.Players,
				FirstGame: openapi_types.Date{Time: st.Span.First}, LastGame: openapi_types.Date{Time: st.Span.Last},
			})
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

// optionalDate returns t as an API date, or nil for none.
func optionalDate(t *time.Time) *openapi_types.Date {
	if t == nil {
		return nil
	}
	return &openapi_types.Date{Time: *t}
}
