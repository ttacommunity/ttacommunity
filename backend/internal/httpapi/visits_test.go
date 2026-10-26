package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ttacommunity/backend/internal/webanalytics"
)

// fakeVisits is a visit counter holding count, shown when ok.
type fakeVisits struct {
	count webanalytics.Count
	ok    bool
}

func (v fakeVisits) Latest() (webanalytics.Count, bool) { return v.count, v.ok }

func serveVisits(visits Visits) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux := NewMux(&fakeRatings{}, nil, nil, nil, visits, slog.New(slog.DiscardHandler), &recordedRequests{}, stepClock(time.Millisecond))
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/site/visits", nil))
	return rec
}

func TestSiteVisitsAnswersOnlyWithARecentCount(t *testing.T) {
	tests := []struct {
		name      string
		visits    Visits
		wantCode  int
		wantCache string
	}{
		{"counted", fakeVisits{webanalytics.Count{Visits: 190, Days: 28, AsOf: time.Date(2026, 10, 27, 5, 0, 0, 0, time.UTC)}, true}, http.StatusOK, "public, max-age=3600"},
		{"no count from the last day", fakeVisits{}, http.StatusServiceUnavailable, cacheNone},
		{"not counted by this API", nil, http.StatusNotFound, cacheNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveVisits(tt.visits)
			if rec.Code != tt.wantCode || rec.Header().Get("Cache-Control") != tt.wantCache {
				t.Errorf("GET /v1/site/visits = %d, Cache-Control %q; want %d, %q", rec.Code, rec.Header().Get("Cache-Control"), tt.wantCode, tt.wantCache)
			}
		})
	}
}

func TestSiteVisitsGivesTheCountItsDaysAndWhenItWasTaken(t *testing.T) {
	asOf := time.Date(2026, 10, 27, 16, 0, 0, 0, time.FixedZone("AEDT", 11*3600))
	rec := serveVisits(fakeVisits{webanalytics.Count{Visits: 190, Days: 28, AsOf: asOf}, true})
	wantJSON(t, rec.Body.Bytes(), "visits", `190`)
	wantJSON(t, rec.Body.Bytes(), "days", `28`)
	wantJSON(t, rec.Body.Bytes(), "as_of", `"2026-10-27T05:00:00Z"`)
}
