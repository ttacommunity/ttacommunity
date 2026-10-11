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

// Observe records one answered request. route is the mux's pattern, never the path. Status 0, an
// answer aborted after it started, is labelled "aborted".
func (r *Registry) Observe(route string, status int, took time.Duration) {
	label := strconv.Itoa(status)
	if status == 0 {
		label = "aborted"
	}
	r.requests.WithLabelValues(route, label).Observe(took.Seconds())
}

// AddPool reports pool's connections, labelled with its name.
func (r *Registry) AddPool(name string, pool *pgxpool.Pool) {
	labels := prometheus.Labels{"pool": name}
	metric := func(name, help string, kind prometheus.ValueType, value func(*pgxpool.Stat) float64) poolMetric {
		return poolMetric{desc: prometheus.NewDesc(name, help, nil, labels), kind: kind, value: value}
	}
	r.reg.MustRegister(&poolCollector{pool: pool, metrics: []poolMetric{
		metric("ttacapi_db_pool_acquired_connections", "Connections in use.", prometheus.GaugeValue,
			func(s *pgxpool.Stat) float64 { return float64(s.AcquiredConns()) }),
		metric("ttacapi_db_pool_idle_connections", "Connections open and free.", prometheus.GaugeValue,
			func(s *pgxpool.Stat) float64 { return float64(s.IdleConns()) }),
		metric("ttacapi_db_pool_max_connections", "The most connections the pool opens.", prometheus.GaugeValue,
			func(s *pgxpool.Stat) float64 { return float64(s.MaxConns()) }),
		metric("ttacapi_db_pool_acquires_total", "Connections taken from the pool.", prometheus.CounterValue,
			func(s *pgxpool.Stat) float64 { return float64(s.AcquireCount()) }),
		metric("ttacapi_db_pool_empty_acquires_total", "Connections waited for, because none was free.", prometheus.CounterValue,
			func(s *pgxpool.Stat) float64 { return float64(s.EmptyAcquireCount()) }),
		metric("ttacapi_db_pool_empty_acquire_wait_seconds_total", "Time spent waiting for a free connection.", prometheus.CounterValue,
			func(s *pgxpool.Stat) float64 { return s.EmptyAcquireWaitTime().Seconds() }),
	}})
}

// poolMetric is one number read from a pool's stats.
type poolMetric struct {
	desc  *prometheus.Desc
	kind  prometheus.ValueType
	value func(*pgxpool.Stat) float64
}

// poolCollector reports a pool's metrics from one reading of its stats per scrape, so they agree.
type poolCollector struct {
	pool    *pgxpool.Pool
	metrics []poolMetric
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range c.metrics {
		ch <- m.desc
	}
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.pool.Stat()
	for _, m := range c.metrics {
		ch <- prometheus.MustNewConstMetric(m.desc, m.kind, m.value(s))
	}
}

// Handler serves the metrics in Prometheus's text format.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{})
}
