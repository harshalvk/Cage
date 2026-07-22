package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cage_http_requests_total",
			Help: "Total HTTP requests, labeled by route and status code",
		},
		[]string{"route", "method", "status"},
	)

	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cage_http_requests_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"route", "method"},
	)

	SandboxesActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "cage_sandboxes_active",
			Help: "Current number of running sandboxes",
		},
	)

	PoolHits = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "cage_pool_hits_total",
			Help: "Sandbox creations served from the warm pool",
		},
	)

	PoolMisses = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "cage_pool_misses_total",
			Help: "Sandbox creations that required a cold Docker create",
		},
	)

	SandboxesReaped = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "cage_sandboxes_reaped_total",
			Help: "Total sandboxes killed by the reaper for expiry",
		},
	)
)

func init() {
	prometheus.MustRegister(
		RequestsTotal,
		RequestDuration,
		SandboxesActive,
		PoolHits,
		PoolMisses,
		SandboxesReaped,
	)
}
