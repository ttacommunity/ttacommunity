package httpapi

import (
	"context"
	"encoding/json"
	"fmt"

	"ttacommunity/backend/internal/analysis"
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
	ratings, active, err := s.ratings.AnalysisRatings(ctx, req.Run, int(format))
	if nf, ok := notFound(err); ok {
		return GetDistribution404JSONResponse{nf}, nil
	}
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
	ratings, _, err := s.ratings.AnalysisRatings(ctx, req.Run, int(format))
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
