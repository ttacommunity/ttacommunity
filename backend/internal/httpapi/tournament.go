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
	body.GameRecords = gameRecords(t.GameRecords)
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

// GetPlayerAchievements returns what a player's achievements page shows.
func (s *server) GetPlayerAchievements(ctx context.Context, req GetPlayerAchievementsRequestObject) (GetPlayerAchievementsResponseObject, error) {
	if msg := playerRefusal(req.Run, req.Name, 0); msg != "" {
		return GetPlayerAchievements400JSONResponse{badRequestBody(msg)}, nil
	}
	name, summary, records, err := s.ratings.PlayerAchievements(ctx, req.Run, req.Name)
	if nf, ok := notFound(err); ok {
		return GetPlayerAchievements404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}

	body := PlayerAchievements{RunId: req.Run, Name: name, Records: make([]TournamentRecord, 0, len(records))}
	if summary != nil {
		body.Summary = &AchievementSummary{
			WorldTitles: summary.WorldTitles, InternationalTitles: summary.InternationalTitles,
			IntermezzoTitles: summary.IntermezzoTitles, RoyalLeagueTitles: summary.RoyalLeagueTitles,
			Gold: summary.Gold, Silver: summary.Silver, Bronze: summary.Bronze, Accolades: summary.Accolades,
		}
	}
	for _, r := range records {
		rec := TournamentRecord{
			Tournament: r.Tournament, Season: r.Season, Division: r.Division, Placement: r.Placement,
			Points: r.Points, Details: r.Details, FinishDate: r.FinishDate, Career: r.Career,
		}
		if r.Medal != nil {
			medal := TournamentRecordMedal(*r.Medal)
			rec.Medal = &medal
		}
		body.Records = append(body.Records, rec)
	}
	return GetPlayerAchievements200JSONResponse{
		Body: body, Headers: GetPlayerAchievements200ResponseHeaders{CacheControl: cacheRun},
	}, nil
}

// GetCommunityLeaderboard returns an edition of the community leaderboard.
func (s *server) GetCommunityLeaderboard(ctx context.Context, req GetCommunityLeaderboardRequestObject) (GetCommunityLeaderboardResponseObject, error) {
	switch {
	case req.Run < 1:
		return GetCommunityLeaderboard400JSONResponse{badRequestBody("bad parameter: run")}, nil
	case !req.Edition.Valid():
		return GetCommunityLeaderboard400JSONResponse{badRequestBody("unknown edition")}, nil
	}
	rows, err := s.ratings.CommunityLeaderboard(ctx, req.Run, string(req.Edition))
	if nf, ok := notFound(err); ok {
		return GetCommunityLeaderboard404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}
	body := CommunityLeaderboard{RunId: req.Run, Edition: req.Edition, Rows: make([]CommunityRow, 0, len(rows))}
	for _, r := range rows {
		row := CommunityRow{Rank: r.Rank, Name: r.Name, Country: r.Country, Points: r.Points, Events: r.Events, Scores: r.Scores}
		if row.Scores == nil {
			row.Scores = map[string]float64{}
		}
		if r.Title != nil {
			title := CommunityRowTitle(*r.Title)
			row.Title = &title
		}
		body.Rows = append(body.Rows, row)
	}
	return GetCommunityLeaderboard200JSONResponse{
		Body: body, Headers: GetCommunityLeaderboard200ResponseHeaders{CacheControl: cacheRun},
	}, nil
}

// gameRecords turns games' winning records into the API's.
func gameRecords(games []store.GameRecord) []GameRecord {
	out := make([]GameRecord, 0, len(games))
	for _, g := range games {
		opponents := make([]Seat, 0, len(g.Opponents))
		for _, o := range g.Opponents {
			opponents = append(opponents, Seat{Name: o.Name, Score: o.Score})
		}
		out = append(out, GameRecord{
			Name: g.Name, WorldChampion: g.WorldChampion, Score: g.Score, Opponents: opponents,
			Tournament: g.Tournament, Date: openapi_types.Date{Time: g.Date},
		})
	}
	return out
}
