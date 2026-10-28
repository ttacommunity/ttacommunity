// The boards: the leaderboard and its history, by model and format.
package httpapi

import (
	"context"
	"errors"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"ttacommunity/backend/internal/store"
)

// boardParams is a board's model and format, Glicko-2 (DANeo) and N0 when not given, and the message refusing a bad
// run, model or format, checked in that order ("" when all are good).
func boardParams(run int64, m *Model, f *Format) (Model, Format, string) {
	model := Glicko2Daneo
	if m != nil {
		model = *m
	}
	format := formatOr(f)
	switch {
	case run < 1:
		return model, format, "bad parameter: run"
	case !model.Valid():
		return model, format, "unknown model"
	case !format.Valid():
		return model, format, "unknown format"
	}
	return model, format, ""
}

// GetLeaderboard returns one published run's leaderboard for a model and
// format, refusing unknown values before they reach the database.
func (s *server) GetLeaderboard(ctx context.Context, req GetLeaderboardRequestObject) (GetLeaderboardResponseObject, error) {
	model, format, msg := boardParams(req.Run, req.Params.Model, req.Params.Format)
	if msg != "" {
		return GetLeaderboard400JSONResponse{badRequestBody(msg)}, nil
	}

	rows, err := s.ratings.Leaderboard(ctx, req.Run, string(model), int(format))
	if errors.Is(err, store.ErrRunNotFound) {
		return GetLeaderboard404JSONResponse{NotFoundJSONResponse{
			Body:    Error{Error: err.Error()},
			Headers: NotFoundResponseHeaders{CacheControl: cacheNone},
		}}, nil
	}
	if err != nil {
		return nil, err
	}

	players := make([]LeaderboardRow, 0, len(rows))
	for _, p := range rows {
		row := LeaderboardRow{
			Rank: p.Rank, RankDelta: p.RankDelta, Name: p.Name, Rating: p.Rating, Rd: p.RD, CRating: p.CRating,
			Games: p.Games, Opponents: p.Opponents, Wins: p.Wins, Losses: p.Losses, Draws: p.Draws, WinRate: p.WinRate,
			Country: p.Country,
		}
		if p.Title != nil {
			row.Title = &Title{Code: TitleCode(p.Title.Code), Count: p.Title.Count, Reason: p.Title.Reason}
		}
		if p.LastPlayed != nil {
			row.LastPlayed = &openapi_types.Date{Time: *p.LastPlayed}
		}
		row.Badges = extraBadges(p.Badges)
		players = append(players, row)
	}
	return GetLeaderboard200JSONResponse{
		Body:    Leaderboard{RunId: req.Run, Model: model, Format: format, Players: players},
		Headers: GetLeaderboard200ResponseHeaders{CacheControl: cacheRun},
	}, nil
}

// GetHistory returns each player's rating at a past date, or at the run's
// previous period when no date is given.
func (s *server) GetHistory(ctx context.Context, req GetHistoryRequestObject) (GetHistoryResponseObject, error) {
	model, format, msg := boardParams(req.Run, req.Params.Model, req.Params.Format)
	if msg != "" {
		return GetHistory400JSONResponse{badRequestBody(msg)}, nil
	}
	var at *time.Time
	if req.Params.At != nil {
		at = &req.Params.At.Time
	}

	on, rows, err := s.ratings.History(ctx, req.Run, string(model), int(format), at)
	if errors.Is(err, store.ErrRunNotFound) || errors.Is(err, store.ErrNoHistory) {
		return GetHistory404JSONResponse{NotFoundJSONResponse{
			Body:    Error{Error: err.Error()},
			Headers: NotFoundResponseHeaders{CacheControl: cacheNone},
		}}, nil
	}
	if err != nil {
		return nil, err
	}
	players := make([]HistoryRow, 0, len(rows))
	for _, h := range rows {
		players = append(players, HistoryRow{Name: h.Name, Rating: h.Rating, Rd: h.RD, CRating: h.CRating})
	}
	return GetHistory200JSONResponse{
		Body:    History{RunId: req.Run, Model: model, Format: format, At: openapi_types.Date{Time: on}, Players: players},
		Headers: GetHistory200ResponseHeaders{CacheControl: cacheRun},
	}, nil
}

// extraBadges turns a player's badges into the API's, or nil for none, so the field is left out.
func extraBadges(bs []store.ExtraBadge) *[]ExtraBadge {
	if len(bs) == 0 {
		return nil
	}
	out := make([]ExtraBadge, len(bs))
	for i, b := range bs {
		out[i] = ExtraBadge{Label: b.Label, Description: b.Description}
	}
	return &out
}
