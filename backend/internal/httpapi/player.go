package httpapi

import (
	"context"
	"errors"
	"slices"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"ttacommunity/backend/internal/ratingdelta"
	"ttacommunity/backend/internal/round"
	"ttacommunity/backend/internal/store"
)

// maxNameLength matches the spec's PlayerName, which the generated code does not check.
const maxNameLength = 200

// playerRefusal checks what every player route shares, returning the
// message to refuse with, or "" when the request may reach the store.
func playerRefusal(run int64, name string, format Format) string {
	switch {
	case run < 1:
		return "bad parameter: run"
	case len(name) > maxNameLength:
		return "bad parameter: name"
	case !format.Valid():
		return "unknown format"
	}
	return ""
}

func badRequestBody(msg string) BadRequestJSONResponse {
	return BadRequestJSONResponse{Body: Error{Error: msg}, Headers: BadRequestResponseHeaders{CacheControl: cacheNone}}
}

// notFound returns the 404 body for a store error that means one, or false.
func notFound(err error) (NotFoundJSONResponse, bool) {
	if !errors.Is(err, store.ErrRunNotFound) && !errors.Is(err, store.ErrPlayerNotFound) && !errors.Is(err, store.ErrTournamentNotFound) {
		return NotFoundJSONResponse{}, false
	}
	return NotFoundJSONResponse{Body: Error{Error: err.Error()}, Headers: NotFoundResponseHeaders{CacheControl: cacheNone}}, true
}

func formatOr(f *Format) Format {
	if f == nil {
		return N0
	}
	return *f
}

// GetPlayer returns one player's details and ratings in a published run.
func (s *server) GetPlayer(ctx context.Context, req GetPlayerRequestObject) (GetPlayerResponseObject, error) {
	if msg := playerRefusal(req.Run, req.Name, N0); msg != "" {
		return GetPlayer400JSONResponse{badRequestBody(msg)}, nil
	}
	p, err := s.ratings.Player(ctx, req.Run, req.Name)
	if nf, ok := notFound(err); ok {
		return GetPlayer404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}

	body := Player{RunId: req.Run, Name: p.Name, Country: p.Country, Games: []FormatGames{}, Ratings: []PlayerRating{}}
	if p.Title != nil {
		body.Title = &Title{Code: TitleCode(p.Title.Code), Count: p.Title.Count, Reason: p.Title.Reason}
	}
	if p.Peak != nil {
		body.Peak = &Peak{Code: TitleCode(p.Peak.Code), Year: p.Peak.Year}
	}
	if p.LastPlayed != nil {
		body.LastPlayed = &openapi_types.Date{Time: *p.LastPlayed}
	}
	if p.NewestGame != nil {
		body.NewestGame = &openapi_types.Date{Time: *p.NewestGame}
	}
	for _, f := range []Format{N0, N2, N3, N4} {
		body.Games = append(body.Games, FormatGames{Format: f, Games: p.Games[int(f)]})
	}
	for _, r := range p.Ratings {
		body.Ratings = append(body.Ratings, PlayerRating{
			Model: Model(r.Model), Format: Format(r.Format), Rank: r.Rank, Rating: r.Rating, Rd: r.RD,
			CRating: r.CRating, Opponents: r.Opponents, WinRate: r.WinRate,
		})
	}
	return GetPlayer200JSONResponse{Body: body, Headers: GetPlayer200ResponseHeaders{CacheControl: cacheRun}}, nil
}

// GetPlayerHistory returns a player's rating after each period, by model.
func (s *server) GetPlayerHistory(ctx context.Context, req GetPlayerHistoryRequestObject) (GetPlayerHistoryResponseObject, error) {
	format := formatOr(req.Params.Format)
	if msg := playerRefusal(req.Run, req.Name, format); msg != "" {
		return GetPlayerHistory400JSONResponse{badRequestBody(msg)}, nil
	}
	name, points, err := s.ratings.PlayerHistory(ctx, req.Run, req.Name, int(format))
	if nf, ok := notFound(err); ok {
		return GetPlayerHistory404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}

	series := []HistorySeries{}
	for _, h := range points {
		if len(series) == 0 || string(series[len(series)-1].Model) != h.Model {
			series = append(series, HistorySeries{Model: Model(h.Model), Points: []HistoryPoint{}})
		}
		last := &series[len(series)-1]
		last.Points = append(last.Points, HistoryPoint{Date: openapi_types.Date{Time: h.Date}, Rating: h.Rating, CRating: h.CRating})
	}
	return GetPlayerHistory200JSONResponse{
		Body:    PlayerHistory{RunId: req.Run, Name: name, Format: format, Series: series},
		Headers: GetPlayerHistory200ResponseHeaders{CacheControl: cacheRun},
	}, nil
}

// GetPlayerMatches returns one page of a player's games, each placed and
// with the rating change the player page estimates for it.
func (s *server) GetPlayerMatches(ctx context.Context, req GetPlayerMatchesRequestObject) (GetPlayerMatchesResponseObject, error) {
	format := formatOr(req.Params.Format)
	model, page := Glicko2Daneo, 1
	if req.Params.Model != nil {
		model = *req.Params.Model
	}
	if req.Params.Page != nil {
		page = *req.Params.Page
	}
	msg := playerRefusal(req.Run, req.Name, format)
	switch {
	case msg != "":
	case !slices.Contains(store.PlayerPageModels, string(model)):
		msg = "unknown model"
	case page < 1:
		msg = "bad parameter: page"
	}
	if msg != "" {
		return GetPlayerMatches400JSONResponse{badRequestBody(msg)}, nil
	}

	name, mp, err := s.ratings.PlayerMatches(ctx, req.Run, req.Name, int(format), page)
	if nf, ok := notFound(err); ok {
		return GetPlayerMatches404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}

	// The page estimates WHR with GlickoD's update, on GlickoD's ratings.
	engine, softer := strings.CutSuffix(string(model), "_softer")
	if engine == "whr" {
		engine = "glicko2_daneo"
	}
	ratedAs := engine
	if softer {
		ratedAs += "_softer"
	}
	names := []string{name}
	for _, m := range mp.Matches {
		for _, seat := range m.Seats {
			names = append(names, seat.Name)
		}
	}
	rated, err := s.ratings.RatingsOf(ctx, req.Run, ratedAs, int(format), names)
	if err != nil {
		return nil, err
	}
	lookup := func(n string) (ratingdelta.Rating, bool) {
		r, ok := rated[n]
		return ratingdelta.Rating{Rating: r.Rating, RD: r.RD}, ok
	}

	matches := make([]PlayerMatch, 0, len(mp.Matches))
	for _, m := range mp.Matches {
		seats := slices.Clone(m.Seats)
		slices.SortStableFunc(seats, func(a, b store.Seat) int { return compareDesc(a.Score, b.Score) })
		pm := PlayerMatch{
			MatchId: m.ID, Tournament: m.Tournament, Date: openapi_types.Date{Time: m.Date},
			PlayerCount: m.PlayerCount, ReplayCode: m.ReplayCode, Placement: 1, Seats: make([]Seat, 0, len(seats)),
		}
		dseats := make([]ratingdelta.Seat, 0, len(seats))
		for i, seat := range seats {
			if seat.Name == name {
				pm.Placement = i + 1
			}
			pm.Seats = append(pm.Seats, Seat{Name: seat.Name, Score: seat.Score})
			dseats = append(dseats, ratingdelta.Seat{Name: seat.Name, Score: seat.Score})
		}
		if d, ok := ratingdelta.Delta(engine, name, m.PlayerCount, dseats, lookup); ok {
			pm.RatingDelta = &d
		}
		matches = append(matches, pm)
	}
	return GetPlayerMatches200JSONResponse{
		Body: PlayerMatches{
			RunId: req.Run, Name: name, Format: format, Model: model,
			Page: mp.Page, Pages: mp.Pages, Total: mp.Total, Matches: matches,
		},
		Headers: GetPlayerMatches200ResponseHeaders{CacheControl: cacheRun},
	}, nil
}

// GetPlayerRivals returns a player's record against the opponents they met most.
func (s *server) GetPlayerRivals(ctx context.Context, req GetPlayerRivalsRequestObject) (GetPlayerRivalsResponseObject, error) {
	format := formatOr(req.Params.Format)
	if msg := playerRefusal(req.Run, req.Name, format); msg != "" {
		return GetPlayerRivals400JSONResponse{badRequestBody(msg)}, nil
	}
	name, got, err := s.ratings.PlayerRivals(ctx, req.Run, req.Name, int(format))
	if nf, ok := notFound(err); ok {
		return GetPlayerRivals404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}
	rivals := make([]Rival, 0, len(got))
	for _, v := range got {
		rate := (float64(v.Wins) + 0.5*float64(v.Draws)) / float64(max(1, v.Games)) * 100
		rivals = append(rivals, Rival{
			Name: v.Name, Games: v.Games, Wins: v.Wins, Losses: v.Losses, Draws: v.Draws, WinRate: round.Tenth(rate),
		})
	}
	return GetPlayerRivals200JSONResponse{
		Body:    PlayerRivals{RunId: req.Run, Name: name, Format: format, Rivals: rivals},
		Headers: GetPlayerRivals200ResponseHeaders{CacheControl: cacheRun},
	}, nil
}

// GetPlayerCareer returns a player's record each year and each model's peak.
func (s *server) GetPlayerCareer(ctx context.Context, req GetPlayerCareerRequestObject) (GetPlayerCareerResponseObject, error) {
	format := formatOr(req.Params.Format)
	if msg := playerRefusal(req.Run, req.Name, format); msg != "" {
		return GetPlayerCareer400JSONResponse{badRequestBody(msg)}, nil
	}
	name, years, peaks, err := s.ratings.PlayerCareer(ctx, req.Run, req.Name, int(format))
	if nf, ok := notFound(err); ok {
		return GetPlayerCareer404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}

	body := PlayerCareer{RunId: req.Run, Name: name, Format: format, Years: []CareerYear{}, Peaks: []ModelPeak{}}
	for _, y := range years {
		// Upstream stores the yearly win rate rounded by SQLite.
		rate := (float64(y.Wins) + 0.5*float64(y.Draws)) / float64(max(1, y.Opponents)) * 100
		careerRate := (float64(y.CareerWins) + 0.5*float64(y.CareerDraws)) / float64(max(1, y.CareerOpponents)) * 100
		cy := CareerYear{
			Year: y.Year, Opponents: y.Opponents, Wins: y.Wins, Losses: y.Losses, Draws: y.Draws,
			WinRate: round.TenthAway(rate), PeakRating: y.PeakRating, YearEndRating: y.YearEnd,
			CareerOpponents: y.CareerOpponents, CareerWins: y.CareerWins, CareerLosses: y.CareerLosses,
			CareerDraws: y.CareerDraws, CareerWinRate: round.TenthAway(careerRate),
		}
		if y.Title != nil {
			code := TitleCode(*y.Title)
			cy.Title = &code
		}
		body.Years = append(body.Years, cy)
	}
	for _, p := range peaks {
		body.Peaks = append(body.Peaks, ModelPeak{Model: Model(p.Model), Rating: p.Rating, Rank: p.Rank, Date: openapi_types.Date{Time: p.Date}})
	}
	return GetPlayerCareer200JSONResponse{Body: body, Headers: GetPlayerCareer200ResponseHeaders{CacheControl: cacheRun}}, nil
}

// GetPlayerMatrix returns a player's rating in each page model, now or at a year's end.
func (s *server) GetPlayerMatrix(ctx context.Context, req GetPlayerMatrixRequestObject) (GetPlayerMatrixResponseObject, error) {
	format := formatOr(req.Params.Format)
	msg := playerRefusal(req.Run, req.Name, format)
	if y := req.Params.Year; msg == "" && y != nil && (*y < 2000 || *y > 2100) {
		msg = "bad parameter: year"
	}
	if msg != "" {
		return GetPlayerMatrix400JSONResponse{badRequestBody(msg)}, nil
	}
	name, cells, err := s.ratings.PlayerMatrix(ctx, req.Run, req.Name, int(format), req.Params.Year)
	if nf, ok := notFound(err); ok {
		return GetPlayerMatrix404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}
	body := PlayerMatrix{RunId: req.Run, Name: name, Format: format, Year: req.Params.Year, Cells: []MatrixCell{}}
	for _, c := range cells {
		body.Cells = append(body.Cells, MatrixCell{Model: Model(c.Model), Rating: c.Rating, Rd: c.RD, CRating: c.CRating, Rank: c.Rank})
	}
	return GetPlayerMatrix200JSONResponse{Body: body, Headers: GetPlayerMatrix200ResponseHeaders{CacheControl: cacheRun}}, nil
}

// GetPlayerVersus returns two players' latest shared games, with where each finished.
func (s *server) GetPlayerVersus(ctx context.Context, req GetPlayerVersusRequestObject) (GetPlayerVersusResponseObject, error) {
	msg := playerRefusal(req.Run, req.Name, N0)
	if msg == "" && len(req.Other) > maxNameLength {
		msg = "bad parameter: other"
	}
	if msg != "" {
		return GetPlayerVersus400JSONResponse{badRequestBody(msg)}, nil
	}
	name, other, shared, err := s.ratings.Versus(ctx, req.Run, req.Name, req.Other)
	if nf, ok := notFound(err); ok {
		return GetPlayerVersus404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}

	matches := make([]VersusMatch, 0, len(shared))
	for _, m := range shared {
		seats := slices.Clone(m.Seats)
		slices.SortStableFunc(seats, func(a, b store.Seat) int { return compareDesc(a.Score, b.Score) })
		vm := VersusMatch{
			MatchId: m.ID, Tournament: m.Tournament, Date: openapi_types.Date{Time: m.Date},
			PlayerCount: m.PlayerCount, Seats: make([]Seat, 0, len(seats)),
		}
		for i, seat := range seats {
			switch seat.Name {
			case name:
				vm.NamePlace = i + 1
			case other:
				vm.OtherPlace = i + 1
			}
			vm.Seats = append(vm.Seats, Seat{Name: seat.Name, Score: seat.Score})
		}
		matches = append(matches, vm)
	}
	return GetPlayerVersus200JSONResponse{
		Body:    PlayerVersus{RunId: req.Run, Name: name, Other: other, Matches: matches},
		Headers: GetPlayerVersus200ResponseHeaders{CacheControl: cacheRun},
	}, nil
}

func compareDesc(a, b float64) int {
	switch {
	case a > b:
		return -1
	case a < b:
		return 1
	}
	return 0
}
