// Package metrics exposes oracle-specific Prometheus instrumentation.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var ( // OracleRequestsTotal counts price fetch attempts by provider and outcome
	// (ok, stale, error, invalid_price).
	OracleRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "oracle_requests_total",
		Help: "Number of oracle price fetch attempts by provider and outcome.",
	}, []string{"provider", "outcome"})

	// OracleStalenessSeconds is the observed age of the last accepted price.
	OracleStalenessSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "oracle_price_age_seconds",
		Help: "Age in seconds of the most recently accepted price feed, by symbol.",
	}, []string{"symbol"})

	// OracleStaleFeeds is 1 while a symbol has no usable (fresh) price.
	OracleStaleFeeds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "oracle_stale_feed",
		Help: "Whether the price feed for a symbol is currently stale (1 stale, 0 fresh).",
	}, []string{"symbol"})
)
