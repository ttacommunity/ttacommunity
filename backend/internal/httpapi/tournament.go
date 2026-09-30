package httpapi

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"ttacommunity/backend/internal/store"
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
	body.Board = trophyRows(t.Board)
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

func trophyRows(board []store.TrophyRow) []TrophyRow {
	out := make([]TrophyRow, 0, len(board))
	for _, b := range board {
		row := TrophyRow{
			Name: b.Name, WorldChampion: b.WorldChampion, Gold: b.Gold, Silver: b.Silver, Bronze: b.Bronze,
			Details: b.Details, Games: b.Games, Wins: b.Wins, WinRate: b.WinRate,
		}
		if b.Title != nil {
			title := TrophyRowTitle(*b.Title)
			row.Title = &title
		}
		out = append(out, row)
	}
	return out
}

// GetHallOfFame returns what the Hall of Fame overview shows.
func (s *server) GetHallOfFame(ctx context.Context, req GetHallOfFameRequestObject) (GetHallOfFameResponseObject, error) {
	if req.Run < 1 {
		return GetHallOfFame400JSONResponse{badRequestBody("bad parameter: run")}, nil
	}
	h, err := s.ratings.HallOfFame(ctx, req.Run)
	if nf, ok := notFound(err); ok {
		return GetHallOfFame404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}
	body := HallOfFame{
		RunId: req.Run, Series: make([]SeriesCount, 0, len(h.Series)), Legends: make([]Legend, 0, len(h.Legends)),
		GrandSlams: trophyRows(h.GrandSlams), Ladders: trophyRows(h.Ladders),
	}
	for _, c := range h.Series {
		body.Series = append(body.Series, SeriesCount{Series: Series(c.Series), Games: c.Games, Seasons: c.Seasons})
	}
	for _, l := range h.Legends {
		body.Legends = append(body.Legends, Legend{
			Name: l.Name, WorldChampion: l.WorldChampion, WorldTitles: l.WorldTitles,
			InternationalTitles: l.InternationalTitles, IntermezzoTitles: l.IntermezzoTitles,
			RoyalLeagueTitles: l.RoyalLeagueTitles, Gold: l.Gold, Silver: l.Silver, Bronze: l.Bronze,
		})
	}
	return GetHallOfFame200JSONResponse{Body: body, Headers: GetHallOfFame200ResponseHeaders{CacheControl: cacheRun}}, nil
}
