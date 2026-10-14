package httpapi

import (
	"context"
	"encoding/json"
	"fmt"

	"ttacommunity/backend/internal/analysis"
	"ttacommunity/backend/internal/leaderboardview"
	"ttacommunity/backend/internal/round"
)

// GetCalibration returns how well each model predicted games, in every mode and format.
func (s *server) GetCalibration(ctx context.Context, req GetCalibrationRequestObject) (GetCalibrationResponseObject, error) {
	if req.Run < 1 {
		return GetCalibration400JSONResponse{badRequestBody("bad parameter: run")}, nil
	}
	rows, err := s.ratings.Calibration(ctx, req.Run)
	if nf, ok := notFound(err); ok {
		return GetCalibration404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}
	body := Calibration{RunId: req.Run, Rows: make([]CalibrationRow, 0, len(rows))}
	for _, c := range rows {
		bins := CalibrationBins{Labels: []string{}, Ideal: []float64{}, Preds: []float64{}, Actuals: []float64{}, Counts: []int{}}
		if len(c.Bins) > 0 {
			if err := json.Unmarshal(c.Bins, &bins); err != nil {
				return nil, fmt.Errorf("reading %s %s bins: %w", c.Mode, c.Model, err)
			}
		}
		body.Rows = append(body.Rows, CalibrationRow{
			Mode: CalibrationRowMode(c.Mode), Model: Model(c.Model), Format: Format(c.Format), Matches: c.Matches,
			Brier: c.Brier, Ece: c.ECE, LogLoss: c.LogLoss, Accuracy: c.Accuracy, Bins: bins,
		})
	}
	return GetCalibration200JSONResponse{Body: body, Headers: GetCalibration200ResponseHeaders{CacheControl: cacheRun}}, nil
}

// GetDistribution returns how ratings spread in each continuous engine, in one format.
func (s *server) GetDistribution(ctx context.Context, req GetDistributionRequestObject) (GetDistributionResponseObject, error) {
	format := formatOr(req.Params.Format)
	if msg := analysisRefusal(req.Run, format); msg != "" {
		return GetDistribution400JSONResponse{badRequestBody(msg)}, nil
	}
	ratings, err := s.ratings.AnalysisRatings(ctx, req.Run, int(format))
	if nf, ok := notFound(err); ok {
		return GetDistribution404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}
	active, err := s.ratings.ActivePlayers(ctx, req.Run)
	if err != nil {
		return nil, err
	}
	d := analysis.Distribute(ratings, active)
	body := Distribution{RunId: req.Run, Format: format, Labels: d.Labels, Engines: make([]EngineDistribution, 0, len(analysis.Engines))}
	for _, e := range analysis.Engines {
		body.Engines = append(body.Engines, EngineDistribution{Model: Model(e), Counts: d.Counts[e], Mean: d.Means[e], ActiveMean: d.ActiveMeans[e]})
	}
	return GetDistribution200JSONResponse{Body: body, Headers: GetDistribution200ResponseHeaders{CacheControl: cacheRun}}, nil
}

// GetAgreement returns the closest and furthest pairs of the page models, in one format.
func (s *server) GetAgreement(ctx context.Context, req GetAgreementRequestObject) (GetAgreementResponseObject, error) {
	format := formatOr(req.Params.Format)
	if msg := analysisRefusal(req.Run, format); msg != "" {
		return GetAgreement400JSONResponse{badRequestBody(msg)}, nil
	}
	ratings, err := s.ratings.AnalysisRatings(ctx, req.Run, int(format))
	if nf, ok := notFound(err); ok {
		return GetAgreement404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}
	a := analysis.Agree(ratings)
	pairs := func(ps []analysis.Pair) []ModelPair {
		out := make([]ModelPair, 0, len(ps))
		for _, p := range ps {
			out = append(out, ModelPair{A: Model(p.A), B: Model(p.B), Pearson: p.Pearson, Spearman: p.Spearman})
		}
		return out
	}
	body := Agreement{RunId: req.Run, Format: format, Sample: a.Sample, Top: pairs(a.Top), Weakest: pairs(a.Weakest)}
	return GetAgreement200JSONResponse{Body: body, Headers: GetAgreement200ResponseHeaders{CacheControl: cacheRun}}, nil
}

func analysisRefusal(run int64, format Format) string {
	switch {
	case run < 1:
		return "bad parameter: run"
	case !format.Valid():
		return "unknown format"
	}
	return ""
}

// GetComparedPlayers returns players matching a search, or upstream's defaults, across every model in one format.
func (s *server) GetComparedPlayers(ctx context.Context, req GetComparedPlayersRequestObject) (GetComparedPlayersResponseObject, error) {
	format := formatOr(req.Params.Format)
	search := ""
	if req.Params.Search != nil {
		search = *req.Params.Search
	}
	msg := analysisRefusal(req.Run, format)
	if msg == "" && len(search) > 200 {
		msg = "bad parameter: search"
	}
	if msg != "" {
		return GetComparedPlayers400JSONResponse{badRequestBody(msg)}, nil
	}
	players, err := s.ratings.ComparedPlayers(ctx, req.Run, int(format), leaderboardview.SearchTerms(search))
	if nf, ok := notFound(err); ok {
		return GetComparedPlayers404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}
	body := ComparedPlayers{RunId: req.Run, Format: format, Players: make([]ComparedPlayer, 0, len(players))}
	for _, p := range players {
		out := ComparedPlayer{Name: p.Name, Country: p.Country, Ratings: make([]ComparedRating, 0, len(p.Ratings))}
		if p.Title != nil {
			out.Title = (*TitleCode)(p.Title)
		}
		for _, r := range p.Ratings {
			out.Ratings = append(out.Ratings, ComparedRating{
				Model: Model(r.Model), Rank: r.Rank, Rating: round.Tenth(r.Rating), Rd: round.Tenth(r.RD),
				CRating: round.Tenth(r.CRating), Opponents: r.Opponents, WinRate: r.WinRate,
			})
		}
		body.Players = append(body.Players, out)
	}
	return GetComparedPlayers200JSONResponse{Body: body, Headers: GetComparedPlayers200ResponseHeaders{CacheControl: cacheRun}}, nil
}

// GetActivity returns a run's active players, games per format, titles and leading countries.
func (s *server) GetActivity(ctx context.Context, req GetActivityRequestObject) (GetActivityResponseObject, error) {
	if req.Run < 1 {
		return GetActivity400JSONResponse{badRequestBody("bad parameter: run")}, nil
	}
	last, err := s.ratings.RunLastGame(ctx, req.Run)
	if nf, ok := notFound(err); ok {
		return GetActivity404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}
	a, err := s.ratings.Activity(ctx, req.Run, leaderboardview.InactiveCutoff(last))
	if nf, ok := notFound(err); ok {
		return GetActivity404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}
	body := Activity{
		RunId: req.Run, Active: a.Active, Inactive: a.Inactive,
		Formats: make([]FormatGamesAndPairs, 0, len(a.Formats)), Titles: make([]TitlePlayers, 0, len(a.Titles)),
		Countries: make([]CountryRatings, 0, len(a.Countries)),
	}
	for _, f := range a.Formats {
		body.Formats = append(body.Formats, FormatGamesAndPairs{Format: Format(f.Format), Games: f.Games, Pairs: f.Pairs})
	}
	for _, t := range a.Titles {
		body.Titles = append(body.Titles, TitlePlayers{Code: TitleCode(t.Title), Players: t.Players})
	}
	// Upstream averages in SQLite, whose ROUND rounds halves away from zero.
	tenth := func(x *float64) *float64 {
		if x == nil {
			return nil
		}
		v := round.TenthAway(*x)
		return &v
	}
	for _, c := range a.Countries {
		body.Countries = append(body.Countries, CountryRatings{
			Country: c.Country, Players: c.Players, Gm: tenth(c.GM), MPlus: tenth(c.MPlus), PPlus: tenth(c.PPlus), All: round.TenthAway(c.All),
		})
	}
	return GetActivity200JSONResponse{Body: body, Headers: GetActivity200ResponseHeaders{CacheControl: cacheRun}}, nil
}

// GetSeatAnalysis returns empirical win rates and placement distributions per starting seat.
func (s *server) GetSeatAnalysis(ctx context.Context, req GetSeatAnalysisRequestObject) (GetSeatAnalysisResponseObject, error) {
	format := formatOr(req.Params.Format)
	if msg := analysisRefusal(req.Run, format); msg != "" {
		return GetSeatAnalysis400JSONResponse{badRequestBody(msg)}, nil
	}
	tier := "all"
	if req.Params.Tier != nil {
		if !req.Params.Tier.Valid() {
			return GetSeatAnalysis400JSONResponse{badRequestBody("bad parameter: tier")}, nil
		}
		tier = string(*req.Params.Tier)
	}

	groups, err := s.ratings.SeatAnalysis(ctx, req.Run, int(format), tier)
	if nf, ok := notFound(err); ok {
		return GetSeatAnalysis404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}

	body := SeatAnalysis{
		RunId:  req.Run,
		Tier:   SeatAnalysisTier(tier),
		Groups: make([]SeatStatsGroup, 0, len(groups)),
	}
	for _, g := range groups {
		sg := SeatStatsGroup{
			PlayerCount: g.PlayerCount,
			TotalGames:  g.TotalGames,
			Seats:       make([]SeatWinrateRow, 0, len(g.Seats)),
		}
		for _, row := range g.Seats {
			sr := SeatWinrateRow{
				Seat:    row.Seat,
				Games:   row.Games,
				Wins:    row.Wins,
				WinRate: row.WinRate,
				Places:  make([]SeatPlaceRate, 0, len(row.Places)),
			}
			for _, p := range row.Places {
				sr.Places = append(sr.Places, SeatPlaceRate{
					Place: p.Place,
					Count: p.Count,
					Rate:  p.Rate,
				})
			}
			sg.Seats = append(sg.Seats, sr)
		}
		body.Groups = append(body.Groups, sg)
	}

	return GetSeatAnalysis200JSONResponse{Body: body, Headers: GetSeatAnalysis200ResponseHeaders{CacheControl: cacheRun}}, nil
}
