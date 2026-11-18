// The percentiles page: how the leaderboard's players spread, and where one stands.
package httpapi

import (
	"context"
	"slices"

	"ttacommunity/backend/internal/analysis"
	"ttacommunity/backend/internal/leaderboardview"
	"ttacommunity/backend/internal/round"
	"ttacommunity/backend/internal/store"
)

// GetRatingPercentiles returns how the chosen rating of the players the leaderboard
// shows for the same filters spreads, and where the asked-for player stands.
func (s *server) GetRatingPercentiles(ctx context.Context, req GetRatingPercentilesRequestObject) (GetRatingPercentilesResponseObject, error) {
	p := req.Params
	model, format, msg := boardParams(req.Run, p.Model, p.Format)
	status, minOpps, basis, width := PercentileStatusActive, 30, RatingBasisCRating, N50
	if p.Status != nil {
		status = *p.Status
	}
	if p.MinOpps != nil {
		minOpps = *p.MinOpps
	}
	if p.Rating != nil {
		basis = *p.Rating
	}
	if p.BinWidth != nil {
		width = *p.BinWidth
	}
	switch {
	case msg != "":
	case !status.Valid():
		msg = "unknown status"
	case minOpps < 0 || minOpps > 1000:
		msg = "bad parameter: min_opps"
	case p.Title != nil && !p.Title.Valid():
		msg = "unknown title"
	case !basis.Valid():
		msg = "unknown rating"
	case !width.Valid():
		msg = "bad parameter: bin_width"
	case p.Player != nil && len(*p.Player) > 100:
		msg = "bad parameter: player"
	}
	if msg != "" {
		return GetRatingPercentiles400JSONResponse{badRequestBody(msg)}, nil
	}

	last, err := s.ratings.RunLastGame(ctx, req.Run)
	var rows []store.LeaderboardRow
	if err == nil {
		rows, err = s.ratings.Leaderboard(ctx, req.Run, string(model), int(format))
	}
	if nf, ok := notFound(err); ok {
		return GetRatingPercentiles404JSONResponse{nf}, nil
	}
	if err != nil {
		return nil, err
	}

	q := leaderboardview.Query{Status: string(status), MinOpps: minOpps, InactiveOnOrBefore: leaderboardview.InactiveCutoff(last)}
	titles := heldTitles(leaderboardview.Select(rows, q))
	if p.Title != nil {
		q.Title = string(*p.Title)
	}
	shown := leaderboardview.Select(rows, q)
	value := func(r store.LeaderboardRow) float64 {
		if basis == RatingBasisRating {
			return r.Rating
		}
		return r.CRating
	}
	ratings := make([]float64, len(shown))
	for i, r := range shown {
		ratings[i] = value(r.LeaderboardRow)
	}
	spread := analysis.SpreadOf(ratings, int(width))
	body := RatingPercentiles{
		RunId: req.Run, Model: model, Format: format, Status: status, MinOpps: minOpps, Title: p.Title, Rating: basis, Titles: titles,
		Players: spread.Players, BinWidth: int(width), Bins: make([]RatingBin, 0, len(spread.Bins)), Marks: make([]PercentileMark, 0, len(spread.Marks)),
	}
	for _, b := range spread.Bins {
		body.Bins = append(body.Bins, RatingBin{From: b.From, Count: b.Count})
	}
	for _, m := range spread.Marks {
		body.Marks = append(body.Marks, PercentileMark{Percentile: m.Percentile, Value: round.Tenth(m.Value)})
	}

	if p.Player != nil {
		i := slices.IndexFunc(rows, func(r store.LeaderboardRow) bool { return r.Name == *p.Player })
		if i < 0 {
			return GetRatingPercentiles404JSONResponse{notFound404(store.ErrPlayerNotFound.Error())}, nil
		}
		v := value(rows[i])
		st := analysis.Place(ratings, v)
		counted := slices.ContainsFunc(shown, func(r leaderboardview.Row) bool { return r.Name == rows[i].Name })
		body.Player = &PlayerPercentile{Name: rows[i].Name, Value: v, Rank: st.Rank, Percentile: st.Percentile, Counted: counted}
	}
	return GetRatingPercentiles200JSONResponse{Body: body, Headers: GetRatingPercentiles200ResponseHeaders{CacheControl: cacheRun}}, nil
}

// heldTitles returns the title codes rows hold, sorted, for the title choice.
func heldTitles(rows []leaderboardview.Row) []TitleCode {
	titles := []TitleCode{}
	for _, r := range rows {
		if r.Title != nil && !slices.Contains(titles, TitleCode(r.Title.Code)) {
			titles = append(titles, TitleCode(r.Title.Code))
		}
	}
	slices.Sort(titles)
	return titles
}
