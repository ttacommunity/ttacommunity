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
	if !errors.Is(err, store.ErrRunNotFound) && !errors.Is(err, store.ErrPlayerNotFound) && !errors.Is(err, store.ErrTournamentNotFound) && !errors.Is(err, store.ErrNoCommunityEdition) {
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
	body.Badges = extraBadges(p.Badges)
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
// with the rating change the player page estimates for it. A seat whose
// player is hidden is named DeletedPlayer once its rating has been used.
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

	engine, softer := strings.CutSuffix(string(model), "_softer")
	ratedAs := engine
	if softer {
		ratedAs += "_softer"
	}
	names := []string{name}
	var queries []store.PlayerAt
	for _, m := range mp.Matches {
		month := m.Date.Format("2006-01")
		queries = append(queries, store.PlayerAt{Name: name, Month: month})
		for _, seat := range m.Seats {
			names = append(names, seat.Name)
			queries = append(queries, store.PlayerAt{Name: seat.Name, Month: month})
		}
	}
	historical, err := s.ratings.HistoricalRatingsOf(ctx, req.Run, ratedAs, int(format), queries)
	if err != nil {
		return nil, err
	}
	hidden, err := s.ratings.HiddenNames(ctx, names)
	if err != nil {
		return nil, err
	}

	matches := make([]PlayerMatch, 0, len(mp.Matches))
	for _, m := range mp.Matches {
		month := m.Date.Format("2006-01")
		byScore := slices.Clone(m.Seats)
		slices.SortStableFunc(byScore, func(a, b store.Seat) int { return compareDesc(a.Score, b.Score) })
		pm := PlayerMatch{
			MatchId: m.ID, Tournament: m.Tournament, Date: openapi_types.Date{Time: m.Date},
			PlayerCount: m.PlayerCount, ReplayCode: m.ReplayCode, Placement: 1, Seats: make([]Seat, 0, len(m.Seats)),
		}
		for i, seat := range byScore {
			if seat.Name == name {
				pm.Placement = i + 1
				break
			}
		}
		for _, seat := range m.Seats {
			shown := seat.Name
			if hidden[shown] {
				shown = store.DeletedPlayer
			}
			pm.Seats = append(pm.Seats, Seat{Name: shown, Score: seat.Score})
		}
		dseats := make([]ratingdelta.Seat, 0, len(byScore))
		for _, seat := range byScore {
			dseats = append(dseats, ratingdelta.Seat{Name: seat.Name, Score: seat.Score})
		}
		lookup := func(n string) (ratingdelta.Rating, bool) {
			r, ok := historical[store.PlayerAt{Name: n, Month: month}]
			return ratingdelta.Rating{Rating: r.Rating, RD: r.RD}, ok
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

// GetPlayerCareer returns a player's record each year and their peak in one model.
func (s *server) GetPlayerCareer(ctx context.Context, req GetPlayerCareerRequestObject) (GetPlayerCareerResponseObject, error) {
	format := formatOr(req.Params.Format)
	model := Glicko2Daneo
	if req.Params.Model != nil {
		model = *req.Params.Model
	}
	msg := playerRefusal(req.Run, req.Name, format)
	if msg == "" && !slices.Contains(store.PlayerPageModels, string(model)) {
		msg = "unknown model"
	}
	if msg != "" {
		return GetPlayerCareer400JSONResponse{badRequestBody(msg)}, nil
	}
	name, years, peak, err := s.ratings.PlayerCareer(ctx, req.Run, req.Name, int(format), string(model))
	if nf, ok := notFound(err); ok {
		return GetPlayerCareer404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}

	body := PlayerCareer{RunId: req.Run, Name: name, Format: format, Model: model, Years: []CareerYear{}}
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
	if peak != nil {
		body.Peak = &CareerPeak{Rating: peak.Rating, Rank: peak.Rank, Date: openapi_types.Date{Time: peak.Date}}
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
			PlayerCount: m.PlayerCount, ReplayCode: m.ReplayCode, Seats: make([]Seat, 0, len(seats)),
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

// GetPlayerSeats returns a player's seat statistics across formats.
func (s *server) GetPlayerSeats(ctx context.Context, req GetPlayerSeatsRequestObject) (GetPlayerSeatsResponseObject, error) {
	if msg := playerRefusal(req.Run, req.Name, N0); msg != "" {
		return GetPlayerSeats400JSONResponse{badRequestBody(msg)}, nil
	}
	unrated := req.Params.Unrated != nil && *req.Params.Unrated
	stats, err := s.ratings.PlayerSeats(ctx, req.Run, req.Name, unrated)
	if nf, ok := notFound(err); ok {
		return GetPlayerSeats404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}

	body := PlayerSeatStats{
		RunId:   req.Run,
		Name:    stats.Name,
		Unrated: stats.Unrated,
		Groups:  make([]PlayerSeatGroup, 0, len(stats.Groups)),
		Overall: PlayerSeatOverview{
			TotalGames: stats.Overall.TotalGames,
			TotalWins:  stats.Overall.TotalWins,
			TotalDraws: stats.Overall.TotalDraws,
			WinRate:    stats.Overall.WinRate,
			AvgCulture: stats.Overall.AvgCulture,
		},
	}

	if stats.Overall.StrongestSeat != nil {
		body.Overall.StrongestSeat = &PlayerSeatDelta{
			PlayerCount:  stats.Overall.StrongestSeat.PlayerCount,
			Seat:         stats.Overall.StrongestSeat.Seat,
			WinRate:      stats.Overall.StrongestSeat.WinRate,
			DeltaWinRate: stats.Overall.StrongestSeat.DeltaWinRate,
		}
	}
	if stats.Overall.WeakestSeat != nil {
		body.Overall.WeakestSeat = &PlayerSeatDelta{
			PlayerCount:  stats.Overall.WeakestSeat.PlayerCount,
			Seat:         stats.Overall.WeakestSeat.Seat,
			WinRate:      stats.Overall.WeakestSeat.WinRate,
			DeltaWinRate: stats.Overall.WeakestSeat.DeltaWinRate,
		}
	}

	for _, g := range stats.Groups {
		group := PlayerSeatGroup{
			PlayerCount: g.PlayerCount,
			TotalGames:  g.TotalGames,
			TotalWins:   g.TotalWins,
			TotalDraws:  g.TotalDraws,
			WinRate:     g.WinRate,
			AvgCulture:  g.AvgCulture,
			Seats:       make([]PlayerSeatRow, 0, len(g.Seats)),
		}
		for _, row := range g.Seats {
			sr := PlayerSeatRow{
				Seat:         row.Seat,
				Games:        row.Games,
				Wins:         row.Wins,
				Draws:        row.Draws,
				WinRate:      row.WinRate,
				DeltaWinRate: row.DeltaWinRate,
				AvgCulture:   row.AvgCulture,
				Places:       make([]SeatPlaceRate, 0, len(row.Places)),
			}
			for _, p := range row.Places {
				sr.Places = append(sr.Places, SeatPlaceRate{
					Place: p.Place,
					Count: p.Count,
					Rate:  p.Rate,
				})
			}
			group.Seats = append(group.Seats, sr)
		}
		body.Groups = append(body.Groups, group)
	}

	return GetPlayerSeats200JSONResponse{
		Body:    body,
		Headers: GetPlayerSeats200ResponseHeaders{CacheControl: cacheRun},
	}, nil
}
