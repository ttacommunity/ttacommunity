package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

// percentileBoard is four players: two the default filters count (ann a GM),
// one with too few opponents (an M) and one who last played over a year
// before the run's last game. Each plain rating is 300 above their conservative one.
func percentileBoard() *fakeRatings {
	last := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	recent, old := last.AddDate(0, -1, 0), last.AddDate(-2, 0, 0)
	row := func(name string, c float64, opps int, played time.Time, title string) store.LeaderboardRow {
		r := store.LeaderboardRow{Name: name, CRating: c, Rating: c + 300, Opponents: opps, LastPlayed: &played}
		if title != "" {
			r.Title = &store.Title{Code: title}
		}
		return r
	}
	return &fakeRatings{lastGame: &last, board: []store.LeaderboardRow{
		row("ann", 1650, 40, recent, "GM"), row("bob", 1510.26, 40, recent, ""), row("cid", 1700, 10, recent, "M"), row("dan", 1800, 40, old, ""),
	}}
}

func getPercentiles(t *testing.T, f *fakeRatings, path string) RatingPercentiles {
	t.Helper()
	rec, _ := serve(t, f, path)
	var got RatingPercentiles
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body.String())
	}
	if cache := rec.Header().Get("Cache-Control"); cache != "public, max-age=300" {
		t.Errorf("GET %s: Cache-Control = %q, want %q", path, cache, "public, max-age=300")
	}
	return got
}

func TestPercentilesCountTheLeaderboardsDefaultPlayers(t *testing.T) {
	f := percentileBoard()
	got := getPercentiles(t, f, "/v1/runs/7/analysis/percentiles?model=glicko2_std&format=2")
	if f.gotRun != 7 || f.gotMod != "glicko2_std" || f.gotFmt != 2 {
		t.Errorf("read run %d %s format %d, want 7 glicko2_std 2", f.gotRun, f.gotMod, f.gotFmt)
	}
	if got.Mean != 1580.1 {
		t.Errorf("mean = %v, want 1580.1", got.Mean)
	}
	if got.Players != 2 || got.Status != PercentileStatusActive || got.MinOpps != 30 || got.BinWidth != 50 || got.Rating != RatingBasisCRating || got.Title != nil {
		t.Errorf("players %d, status %q, min_opps %d, bin_width %d, rating %q, title %v; want 2 active 30 50 c_rating nil",
			got.Players, got.Status, got.MinOpps, got.BinWidth, got.Rating, got.Title)
	}
	if len(got.Bins) != 4 || got.Bins[0] != (RatingBin{From: 1500, Count: 1}) || got.Bins[3] != (RatingBin{From: 1650, Count: 1}) {
		t.Errorf("bins = %+v, want 1500 to 1650 with ann and bob", got.Bins)
	}
	if len(got.Marks) != 6 || got.Marks[0] != (PercentileMark{Percentile: 10, Value: 1510.3}) {
		t.Errorf("marks = %+v, want six, the 10th at bob's rating to one place", got.Marks)
	}
	if got.Player != nil {
		t.Errorf("player = %+v without one asked for", got.Player)
	}
}

func TestPercentilesFollowTheFilters(t *testing.T) {
	got := getPercentiles(t, percentileBoard(), "/v1/runs/7/analysis/percentiles?status=all&min_opps=0")
	if got.Players != 4 || got.Status != PercentileStatusAll || got.MinOpps != 0 {
		t.Errorf("status=all&min_opps=0: players %d, status %q, min_opps %d; want 4 all 0", got.Players, got.Status, got.MinOpps)
	}

	got = getPercentiles(t, percentileBoard(), "/v1/runs/7/analysis/percentiles?min_opps=0&title=M")
	if got.Players != 1 || got.Title == nil || *got.Title != TitleCodeM || got.Bins[0].From != 1700 {
		t.Errorf("title=M: players %d, title %v, bins %+v; want cid alone", got.Players, got.Title, got.Bins)
	}
}

func TestPercentilesListTheTitlesHeldBeforeTheTitleFilter(t *testing.T) {
	got := getPercentiles(t, percentileBoard(), "/v1/runs/7/analysis/percentiles?min_opps=0&title=M")
	if len(got.Titles) != 2 || got.Titles[0] != TitleCodeGM || got.Titles[1] != TitleCodeM {
		t.Errorf("titles = %v, want [GM M]", got.Titles)
	}
}

func TestPercentilesCountTheChosenRatingInTheChosenBins(t *testing.T) {
	got := getPercentiles(t, percentileBoard(), "/v1/runs/7/analysis/percentiles?rating=rating&bin_width=100&player=ann")
	if got.Rating != RatingBasisRating || got.BinWidth != 100 {
		t.Errorf("rating %q, bin_width %d; want rating 100", got.Rating, got.BinWidth)
	}
	if len(got.Bins) != 2 || got.Bins[0] != (RatingBin{From: 1800, Count: 1}) || got.Bins[1] != (RatingBin{From: 1900, Count: 1}) {
		t.Errorf("bins = %+v, want bob's 1810.26 and ann's 1950 in 100-point bins", got.Bins)
	}
	if got.Player == nil || got.Player.Value != 1950 {
		t.Errorf("player = %+v, want ann at her plain rating 1950", got.Player)
	}
}

func TestPercentilesPlaceTheAskedForPlayer(t *testing.T) {
	tests := []struct {
		name   string
		player string
		want   PlayerPercentile
	}{
		{"counted", "ann", PlayerPercentile{Name: "ann", Value: 1650, Rank: 1, Percentile: 50, Counted: true}},
		{"left out by the filters", "cid", PlayerPercentile{Name: "cid", Value: 1700, Rank: 1, Percentile: 100, Counted: false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getPercentiles(t, percentileBoard(), "/v1/runs/7/analysis/percentiles?player="+tt.player)
			if got.Player == nil || *got.Player != tt.want {
				t.Errorf("player = %+v, want %+v", got.Player, tt.want)
			}
		})
	}
}

func TestPercentilesRefuseBadInputBeforeTheStore(t *testing.T) {
	tests := []struct {
		name, query string
	}{
		{"run", "/v1/runs/0/analysis/percentiles"},
		{"model", "/v1/runs/7/analysis/percentiles?model=elo"},
		{"format", "/v1/runs/7/analysis/percentiles?format=5"},
		{"status", "/v1/runs/7/analysis/percentiles?status=retired"},
		{"too few opponents", "/v1/runs/7/analysis/percentiles?min_opps=-1"},
		{"too many opponents", "/v1/runs/7/analysis/percentiles?min_opps=1001"},
		{"long name", "/v1/runs/7/analysis/percentiles?player=" + strings.Repeat("a", 101)},
		{"title", "/v1/runs/7/analysis/percentiles?title=XX"},
		{"rating", "/v1/runs/7/analysis/percentiles?rating=elo"},
		{"bin width", "/v1/runs/7/analysis/percentiles?bin_width=30"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := percentileBoard()
			rec, _ := serve(t, f, tt.query)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("GET %s: %d, want 400", tt.query, rec.Code)
			}
			if f.calls != 0 {
				t.Errorf("GET %s reached the store %d times", tt.query, f.calls)
			}
		})
	}
}

func TestPercentilesAnswer404ForAnUnknownRunOrPlayer(t *testing.T) {
	tests := []struct {
		name string
		f    *fakeRatings
		path string
	}{
		{"unknown run", &fakeRatings{boardErr: store.ErrRunNotFound}, "/v1/runs/7/analysis/percentiles"},
		{"unknown player", percentileBoard(), "/v1/runs/7/analysis/percentiles?player=eve"},
		{"a name is matched exactly", percentileBoard(), "/v1/runs/7/analysis/percentiles?player=Ann"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec, _ := serve(t, tt.f, tt.path); rec.Code != http.StatusNotFound {
				t.Errorf("GET %s: %d %s, want 404", tt.path, rec.Code, rec.Body.String())
			}
		})
	}
}
