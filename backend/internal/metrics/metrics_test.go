package metrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func scrape(t *testing.T, r *Registry) string {
	t.Helper()
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, _ := io.ReadAll(rec.Body)
	return string(body)
}

func TestObserveCountsARequestByRouteAndStatus(t *testing.T) {
	r := New()
	r.Observe("GET /v1/runs/current", 200, 30*time.Millisecond)
	r.Observe("GET /v1/runs/current", 200, 2*time.Second)
	r.Observe("GET /v1/runs/current", 500, time.Millisecond)

	body := scrape(t, r)

	for _, want := range []string{
		`ttacapi_http_request_duration_seconds_count{route="GET /v1/runs/current",status="200"} 2`,
		`ttacapi_http_request_duration_seconds_count{route="GET /v1/runs/current",status="500"} 1`,
		`ttacapi_http_request_duration_seconds_bucket{route="GET /v1/runs/current",status="200",le="0.05"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape lacks %q", want)
		}
	}
}

func TestAddPoolReportsTheConnectionsByPool(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/none?pool_max_conns=3")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	r := New()
	r.AddPool("ratings_read", pool)

	body := scrape(t, r)

	for _, want := range []string{
		`ttacapi_db_pool_max_connections{pool="ratings_read"} 3`,
		`ttacapi_db_pool_acquired_connections{pool="ratings_read"} 0`,
		`ttacapi_db_pool_idle_connections{pool="ratings_read"} 0`,
		`ttacapi_db_pool_acquires_total{pool="ratings_read"} 0`,
		`ttacapi_db_pool_empty_acquires_total{pool="ratings_read"} 0`,
		`ttacapi_db_pool_empty_acquire_wait_seconds_total{pool="ratings_read"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape lacks %q", want)
		}
	}
}

func TestTheScrapeCarriesTheGoRuntime(t *testing.T) {
	body := scrape(t, New())

	for _, want := range []string{"go_goroutines ", "process_resident_memory_bytes "} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape lacks %q", want)
		}
	}
}
