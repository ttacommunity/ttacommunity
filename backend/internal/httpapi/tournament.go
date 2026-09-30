package httpapi

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"
)

// GetTournamentSeries returns what a series' Hall of Fame page shows.
func (s *server) GetTournamentSeries(ctx context.Context, req GetTournamentSeriesRequestObject) (GetTournamentSeriesResponseObject, error) {
	switch {
	case req.Run < 1:
		return GetTournamentSeries400JSONResponse{badRequestBody("bad parameter: run")}, nil
	case !req.Series.Valid():
		return GetTournamentSeries400JSONResponse{badRequestBody("unknown series")}, nil
	}
	t, err := s.ratings.TournamentSeries(ctx, req.Run, string(req.Series))
	if nf, ok := notFound(err); ok {
		return GetTournamentSeries404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}

	body := TournamentSeries{
		RunId: req.Run, Series: req.Series, Games: t.Games, Seasons: t.Seasons,
		Board:         make([]TrophyRow, 0, len(t.Board)),
		SeasonRecords: make([]SeasonRecord, 0, len(t.SeasonRecords)),
		GameRecords:   make([]GameRecord, 0, len(t.GameRecords)),
	}
	for _, b := range t.Board {
		body.Board = append(body.Board, TrophyRow{
			Name: b.Name, Title: TrophyRowTitle(b.Title), Gold: b.Gold, Silver: b.Silver, Bronze: b.Bronze,
		})
	}
	for _, r := range t.SeasonRecords {
		body.SeasonRecords = append(body.SeasonRecords, SeasonRecord{
			Name: r.Name, WorldChampion: r.WorldChampion, Points: r.Points, Games: r.Games, Placings: r.Placings,
			Season: r.Season, Division: r.Division, LastPlayed: openapi_types.Date{Time: r.LastPlayed},
		})
	}
	for _, g := range t.GameRecords {
		opponents := make([]Seat, 0, len(g.Opponents))
		for _, o := range g.Opponents {
			opponents = append(opponents, Seat{Name: o.Name, Score: o.Score})
		}
		body.GameRecords = append(body.GameRecords, GameRecord{
			Name: g.Name, WorldChampion: g.WorldChampion, Score: g.Score, Opponents: opponents,
			Tournament: g.Tournament, Date: openapi_types.Date{Time: g.Date},
		})
	}
	return GetTournamentSeries200JSONResponse{
		Body: body, Headers: GetTournamentSeries200ResponseHeaders{CacheControl: cacheRun},
	}, nil
}
