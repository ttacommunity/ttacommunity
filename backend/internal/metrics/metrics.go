// Package metrics keeps the api's Prometheus metrics, served on their own listener:
// request durations by route and status, the database pools, and the Go runtime.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry holds the api's metrics.
type Registry struct {
	reg      *prometheus.Registry
	requests *prometheus.HistogramVec
}

// New returns a Registry with the request durations and the Go runtime's and process's metrics.
func New() *Registry {
	r := &Registry{
		reg: prometheus.NewRegistry(),
		requests: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "ttacapi_http_request_duration_seconds",
			Help:    "How long the api took to answer, by route pattern and status code.",
			Buckets: prometheus.DefBuckets,
		}, []string{"route", "status"}),
	}
	r.reg.MustRegister(r.requests, collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return r
}

// Observe records one answered request. route is the mux's pattern, never the path.
func (r *Registry) Observe(route string, status int, took time.Duration) {
	r.requests.WithLabelValues(route, strconv.Itoa(status)).Observe(took.Seconds())
}

// AddPool reports pool's connections, labelled with its name.
func (r *Registry) AddPool(name string, pool *pgxpool.Pool) {
	labels := prometheus.Labels{"pool": name}
	gauge := func(metric, help string, value func(*pgxpool.Stat) float64) prometheus.Collector {
		return prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: metric, Help: help, ConstLabels: labels},
			func() float64 { return value(pool.Stat()) })
	}
	counter := func(metric, help string, value func(*pgxpool.Stat) float64) prometheus.Collector {
		return prometheus.NewCounterFunc(prometheus.CounterOpts{Name: metric, Help: help, ConstLabels: labels},
			func() float64 { return value(pool.Stat()) })
	}
	r.reg.MustRegister(
		gauge("ttacapi_db_pool_acquired_connections", "Connections in use.",
			func(s *pgxpool.Stat) float64 { return float64(s.AcquiredConns()) }),
		gauge("ttacapi_db_pool_idle_connections", "Connections open and free.",
			func(s *pgxpool.Stat) float64 { return float64(s.IdleConns()) }),
		gauge("ttacapi_db_pool_max_connections", "The most connections the pool opens.",
			func(s *pgxpool.Stat) float64 { return float64(s.MaxConns()) }),
		counter("ttacapi_db_pool_acquires_total", "Connections taken from the pool.",
			func(s *pgxpool.Stat) float64 { return float64(s.AcquireCount()) }),
		counter("ttacapi_db_pool_empty_acquires_total", "Connections waited for, because none was free.",
			func(s *pgxpool.Stat) float64 { return float64(s.EmptyAcquireCount()) }),
		counter("ttacapi_db_pool_empty_acquire_wait_seconds_total", "Time spent waiting for a free connection.",
			func(s *pgxpool.Stat) float64 { return s.EmptyAcquireWaitTime().Seconds() }),
	)
}

// Handler serves the metrics in Prometheus's text format.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{})
}
