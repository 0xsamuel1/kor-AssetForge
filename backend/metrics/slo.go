// Package metrics provides Prometheus instrumentation for SLA/SLO tracking and alerting.
package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// SLORequestsTotal counts all requests partitioned by endpoint, method, and status code.
	SLORequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "slo_http_requests_total",
		Help: "Total number of HTTP requests evaluated for SLO compliance.",
	}, []string{"endpoint", "method", "status"})

	// SLORequestDuration tracks latency with specific buckets for p90, p95, and p99 SLO evaluation (<500ms).
	SLORequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "slo_http_request_duration_seconds",
		Help:    "HTTP request latency distributions across endpoints for SLO tracking.",
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.2, 0.35, 0.5, 1.0, 2.0, 5.0},
	}, []string{"endpoint", "method"})

	// SLOBreachTotal counts requests exceeding SLA/SLO limits (latency > 500ms or 5xx server errors).
	SLOBreachTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "slo_breach_total",
		Help: "Count of requests violating SLO latency or availability thresholds.",
	}, []string{"endpoint", "violation_type"})

	// RedisCircuitBreakerState reports current circuit status: 0 = CLOSED, 1 = HALF_OPEN, 2 = OPEN.
	RedisCircuitBreakerState = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cache_redis_circuit_breaker_state",
		Help: "Current state of the Redis circuit breaker (0=CLOSED, 1=HALF_OPEN, 2=OPEN).",
	})

	// CacheDegradedRequests counts cache operations that degraded gracefully to DB.
	CacheDegradedRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cache_degraded_requests_total",
		Help: "Requests where Redis was unavailable and operations degraded to database.",
	}, []string{"tier", "reason"})

	// ServiceAvailabilityGauge reflects point-in-time health status (1 = Healthy, 0 = Degraded/Down).
	ServiceAvailabilityGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "service_availability_status",
		Help: "Service availability signal (1=Healthy, 0=Degraded/Down).",
	}, []string{"service"})
)

// RecordSLORequest records an endpoint request latency and status for SLO evaluation.
func RecordSLORequest(endpoint, method string, statusCode int, duration time.Duration) {
	statusStr := strconv.Itoa(statusCode)
	SLORequestsTotal.WithLabelValues(endpoint, method, statusStr).Inc()
	SLORequestDuration.WithLabelValues(endpoint, method).Observe(duration.Seconds())

	// SLO Threshold 1: Latency > 500ms (p99 violation indicator)
	if duration > 500*time.Millisecond {
		SLOBreachTotal.WithLabelValues(endpoint, "latency_p99_exceeded").Inc()
	}

	// SLO Threshold 2: Internal server errors (availability violation indicator)
	if statusCode >= 500 {
		SLOBreachTotal.WithLabelValues(endpoint, "error_5xx").Inc()
	}
}

// SetCircuitBreakerMetric updates the Prometheus gauge for circuit breaker state.
func SetCircuitBreakerMetric(state string) {
	switch state {
	case "CLOSED":
		RedisCircuitBreakerState.Set(0)
	case "HALF_OPEN":
		RedisCircuitBreakerState.Set(1)
	case "OPEN":
		RedisCircuitBreakerState.Set(2)
	}
}

// SLOMiddleware intercepts HTTP requests to record Prometheus SLO metrics automatically.
func SLOMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		endpoint := c.FullPath()
		if endpoint == "" {
			endpoint = "unknown"
		}

		duration := time.Since(start)
		RecordSLORequest(endpoint, c.Request.Method, c.Writer.Status(), duration)
	}
}
