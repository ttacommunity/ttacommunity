// The leaderboard as a CSV file: the rows the site's table shows, in the
// original site's columns.
package httpapi

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ttacommunity/backend/internal/leaderboardview"
	"ttacommunity/backend/internal/store"
)

var csvHeader = []string{
	"Rank", "Player", "Country", "Title", "Title_Name", "Badge_Reason", "Peak_Title", "Peak_Year", "Rating", "RD",
	"Conservative_Rating", "Matches", "Wins", "Losses", "Draws", "Win_Rate", "Status", "Last_Played", "Model", "Format",
	"Reset_Mode", "Season",
}

var titleNames = map[string]string{
	"WC": "World Champion", "SGM": "Super GM", "GM": "Grandmaster", "M": "Master", "P": "Platinum", "G": "Gold", "S": "Silver", "B": "Bronze", "W": "Wood",
}

var engineNames = map[string]string{
	"glicko2_daneo": "GlickoD", "glicko2_std": "Glicko-2 Standard", "glicko2_mp": "Glicko-2 MP-Weighted",
	"glicko2_adapt": "Glicko-2 Adaptive-T", "whr": "Whole-History Rating",
}

var formatNames = map[Format]string{N0: "All Formats", N2: "2-Player (Duel)", N3: "3-Player", N4: "4-Player"}

// GetLeaderboardCsv returns the players the site's leaderboard shows for the
// same choices, as a CSV file named after them.
func (s *server) GetLeaderboardCsv(ctx context.Context, req GetLeaderboardCsvRequestObject) (GetLeaderboardCsvResponseObject, error) {
	p := req.Params
	model, format := Glicko2Daneo, N0
	if p.Model != nil {
		model = *p.Model
	}
	if p.Format != nil {
		format = *p.Format
	}
	q := leaderboardview.Query{Status: "active", MinOpps: 30, Sort: "rank", Order: "asc"}
	if p.Year != nil {
		q.MinOpps = 0 // upstream's archive has no minimum by default
	}
	stats := Season
	if p.StatsView != nil {
		stats = *p.StatsView
	}
	if p.Status != nil {
		q.Status = string(*p.Status)
	}
	if p.MinOpps != nil {
		q.MinOpps = *p.MinOpps
	}
	if p.Title != nil {
		q.Title = string(*p.Title)
	}
	if p.Search != nil {
		q.Search = *p.Search
	}
	if p.Sort != nil {
		q.Sort = string(*p.Sort)
	}
	if p.Order != nil {
		q.Order = string(*p.Order)
	}
	refused := func(msg string) GetLeaderboardCsvResponseObject {
		return GetLeaderboardCsv400JSONResponse{BadRequestJSONResponse{
			Body:    Error{Error: msg},
			Headers: BadRequestResponseHeaders{CacheControl: cacheNone},
		}}
	}
	switch {
	case req.Run < 1:
		return refused("bad parameter: run"), nil
	case !model.Valid():
		return refused("unknown model"), nil
	case !format.Valid():
		return refused("unknown format"), nil
	case p.Status != nil && !p.Status.Valid():
		return refused("unknown status"), nil
	case q.MinOpps < 0 || q.MinOpps > 1000:
		return refused("bad parameter: min_opps"), nil
	case p.Title != nil && !p.Title.Valid():
		return refused("unknown title"), nil
	case len(q.Search) > 200:
		return refused("bad parameter: search"), nil
	case p.Sort != nil && !p.Sort.Valid():
		return refused("unknown sort"), nil
	case p.Order != nil && !p.Order.Valid():
		return refused("unknown order"), nil
	case p.Year != nil && (*p.Year < 2000 || *p.Year > 2100):
		return refused("bad parameter: year"), nil
	case !stats.Valid():
		return refused("bad parameter: stats_view"), nil
	}

	var rows []store.LeaderboardRow
	var err error
	season, yearSlug := "Live / All-Time", "live"
	if p.Year != nil {
		var board store.YearBoard
		board, err = s.ratings.YearBoard(ctx, req.Run, *p.Year, string(model), int(format))
		rows, q.Played = leaderboardview.FromYear(board, string(stats))
		season, yearSlug = strconv.Itoa(*p.Year), strconv.Itoa(*p.Year)
	} else {
		var last *time.Time
		if last, err = s.ratings.RunLastGame(ctx, req.Run); err == nil {
			q.InactiveOnOrBefore = leaderboardview.InactiveCutoff(last)
			rows, err = s.ratings.Leaderboard(ctx, req.Run, string(model), int(format))
		}
	}
	if errors.Is(err, store.ErrRunNotFound) || errors.Is(err, store.ErrNoHistory) {
		return GetLeaderboardCsv404JSONResponse{NotFoundJSONResponse{
			Body:    Error{Error: err.Error()},
			Headers: NotFoundResponseHeaders{CacheControl: cacheNone},
		}}, nil
	}
	if err != nil {
		return nil, err
	}

	engine, reset := splitModel(string(model))
	var body bytes.Buffer
	body.WriteString("\ufeff") // a byte-order mark, so spreadsheets read UTF-8 names correctly
	w := csv.NewWriter(&body)
	w.UseCRLF = true
	if err := w.Write(csvHeader); err != nil {
		return nil, err
	}
	for _, r := range leaderboardview.Select(rows, q) {
		if err := w.Write(csvRow(r, engineNames[engine], formatNames[format], resetNames[reset], season)); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}

	slug := "all_formats"
	if format != N0 {
		slug = fmt.Sprintf("%dp", format)
	}
	name := fmt.Sprintf("tta_ratings_%s_%s_%s_%s_%s.csv", engine, slug, reset, yearSlug, q.Status)
	return GetLeaderboardCsv200TextcsvResponse{
		Body:          &body,
		ContentLength: int64(body.Len()),
		Headers: GetLeaderboardCsv200ResponseHeaders{
			CacheControl:       cacheRun,
			ContentDisposition: fmt.Sprintf("attachment; filename=%q", name),
		},
	}, nil
}

var resetNames = map[string]string{"continuous": "Continuous (Career)", "softer": "Season Reset"}

// splitModel returns a stored model's engine and its reset mode.
func splitModel(model string) (engine, reset string) {
	engine, reset = strings.TrimSuffix(model, "_retro"), "continuous"
	if e, ok := strings.CutSuffix(engine, "_softer"); ok {
		engine, reset = e, "softer"
	}
	return engine, reset
}

func csvRow(r leaderboardview.Row, model, format, reset, season string) []string {
	var country, title, titleName, reason, peak, peakYear, lastPlayed string
	if r.Country != nil {
		country = *r.Country
	}
	if r.Title != nil {
		title, titleName, reason = r.Title.Code, titleNames[r.Title.Code], r.Title.Reason
	}
	if r.Peak != nil {
		peak = titleNames[r.Peak.Code]
		if r.Peak.Year != nil {
			peakYear = strconv.Itoa(*r.Peak.Year)
		}
	}
	if r.LastPlayed != nil {
		lastPlayed = r.LastPlayed.Format(time.DateOnly)
	}
	status := "Inactive"
	if r.Active {
		status = "Active"
	}
	return []string{
		strconv.Itoa(r.DisplayRank), text(r.Name), country, title, titleName, text(reason), peak, peakYear,
		fmt.Sprintf("%.2f", r.Rating), fmt.Sprintf("%.2f", r.RD), fmt.Sprintf("%.2f", r.CRating),
		strconv.Itoa(r.Opponents), strconv.Itoa(r.Wins), strconv.Itoa(r.Losses), strconv.Itoa(r.Draws),
		fmt.Sprintf("%.1f%%", r.WinRate), status, lastPlayed, model, format, reset, season,
	}
}

// text keeps a spreadsheet from reading a cell as a formula (OWASP's CSV
// injection advice): names come from scraped data.
func text(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
