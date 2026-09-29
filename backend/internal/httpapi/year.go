package httpapi

import (
	"context"
	"errors"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"ttacommunity/backend/internal/round"
	"ttacommunity/backend/internal/store"
)

// GetYears returns the years a run's ratings cover, for the season archive.
func (s *server) GetYears(ctx context.Context, req GetYearsRequestObject) (GetYearsResponseObject, error) {
	if req.Run < 1 {
		return GetYears400JSONResponse{badRequestBody("bad parameter: run")}, nil
	}
	years, err := s.ratings.Years(ctx, req.Run)
	if nf, ok := notFound(err); ok {
		return GetYears404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}
	if years == nil {
		years = []int{}
	}
	return GetYears200JSONResponse{Body: Years{RunId: req.Run, Years: years}, Headers: GetYears200ResponseHeaders{CacheControl: cacheRun}}, nil
}

// GetYearLeaderboard returns the ratings at the end of a year, for the season archive.
func (s *server) GetYearLeaderboard(ctx context.Context, req GetYearLeaderboardRequestObject) (GetYearLeaderboardResponseObject, error) {
	model, format := Glicko2Daneo, formatOr(req.Params.Format)
	if req.Params.Model != nil {
		model = *req.Params.Model
	}
	msg := ""
	switch {
	case req.Run < 1:
		msg = "bad parameter: run"
	case req.Year < 2000 || req.Year > 2100:
		msg = "bad parameter: year"
	case !model.Valid():
		msg = "unknown model"
	case !format.Valid():
		msg = "unknown format"
	}
	if msg != "" {
		return GetYearLeaderboard400JSONResponse{badRequestBody(msg)}, nil
	}

	b, err := s.ratings.YearBoard(ctx, req.Run, req.Year, string(model), int(format))
	if errors.Is(err, store.ErrNoHistory) || errors.Is(err, store.ErrRunNotFound) {
		return GetYearLeaderboard404JSONResponse{NotFoundJSONResponse{
			Body: Error{Error: err.Error()}, Headers: NotFoundResponseHeaders{CacheControl: cacheNone},
		}}, nil
	}
	if err != nil {
		return nil, err
	}

	body := YearLeaderboard{
		RunId: req.Run, Year: b.Year, Model: model, Format: format, Snapshot: openapi_types.Date{Time: b.Snapshot},
		Games: b.Games, Champions: []Champion{}, Players: make([]YearRow, 0, len(b.Rows)),
	}
	if b.Previous != nil {
		body.Previous = &openapi_types.Date{Time: *b.Previous}
	}
	for _, c := range b.Champions {
		body.Champions = append(body.Champions, Champion{Name: c.Name, Reigning: c.Reigning})
	}
	for _, y := range b.Rows {
		row := YearRow{
			Name: y.Name, Rating: y.Rating, Rd: y.RD, CRating: y.CRating, Country: y.Country,
			Season: yearRecord(y.Season), Career: yearRecord(y.Career),
		}
		if y.Title != nil {
			row.Title = &Title{Code: TitleCode(y.Title.Code), Count: y.Title.Count, Reason: y.Title.Reason}
		}
		if y.LastPlayed != nil {
			row.LastPlayed = &openapi_types.Date{Time: *y.LastPlayed}
		}
		if y.PreviousCRating != nil {
			row.Previous = &PreviousRating{Rating: *y.PreviousRating, Rd: *y.PreviousRD, CRating: *y.PreviousCRating}
		}
		body.Players = append(body.Players, row)
	}
	return GetYearLeaderboard200JSONResponse{Body: body, Headers: GetYearLeaderboard200ResponseHeaders{CacheControl: cacheRun}}, nil
}

// yearRecord adds the win rate upstream stores for a year, rounded by SQLite.
func yearRecord(r store.Record) YearRecord {
	rate := (float64(r.Wins) + 0.5*float64(r.Draws)) / float64(max(1, r.Opponents)) * 100
	return YearRecord{Opponents: r.Opponents, Wins: r.Wins, Losses: r.Losses, Draws: r.Draws, WinRate: round.TenthAway(rate)}
}
