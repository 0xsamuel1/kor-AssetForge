// Package metrics exposes search-specific Prometheus instrumentation.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	SearchFallbacks = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "search_fallback_total",
		Help: "Number of search operations served by the database because Elasticsearch was unavailable.",
	}, []string{"operation", "reason"})

	SearchBackendRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "search_backend_requests_total",
		Help: "Number of search operations served by each backend.",
	}, []string{"operation", "backend"})

	SearchBackendDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "search_backend_duration_seconds",
		Help:    "Search operation latency by backend.",
		Buckets: prometheus.DefBuckets,
	}, []string{"operation", "backend"})

	ElasticsearchAvailable = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "search_elasticsearch_available",
		Help: "Whether Elasticsearch is currently available to the search service (1 available, 0 unavailable).",
	})

	ElasticsearchRecoveries = promauto.NewCounter(prometheus.CounterOpts{
		Name: "search_elasticsearch_recoveries_total",
		Help: "Number of times search automatically recovered its Elasticsearch connection.",
	})

	SearchResultQuality = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "search_result_quality_score",
		Help:    "Observed search quality signals, normalized from 0 to 1.",
		Buckets: []float64{0, .1, .25, .5, .75, .9, 1},
	}, []string{"signal", "variant"})
)
