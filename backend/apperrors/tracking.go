package apperrors

// Enhanced error tracking bridge (issue #320).
//
// IncrementErrorCounter/GetErrorMetrics (metrics.go) remain the in-memory
// source of truth used by tests and MetricsHandler. This file additionally
// mirrors every tracked error into Prometheus (backend/metrics/errors.go)
// with endpoint labels so error rates, the 0.1% budget, and burn-rate
// alerts operate on durable time-series instead of reset-on-restart maps.

import (
	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/metrics"
)

// endpointOf returns a low-cardinality endpoint label for metrics.
func endpointOf(c *gin.Context) string {
	if c == nil {
		return "unknown"
	}
	if route := c.FullPath(); route != "" {
		return route
	}
	if c.Request != nil && c.Request.URL != nil {
		return c.Request.URL.Path
	}
	return "unknown"
}

// TrackErrorWithEndpoint records an error both in-memory and in Prometheus.
func TrackErrorWithEndpoint(code ErrorCode, endpoint string) {
	if code == "" {
		code = CodeInternalServerError
	}
	if endpoint == "" {
		endpoint = "unknown"
	}
	IncrementErrorCounter(code)
	metrics.RecordAppError(string(code), endpoint)
}

// TrackGinError records the error for the current gin request.
func TrackGinError(c *gin.Context, code ErrorCode) {
	TrackErrorWithEndpoint(code, endpointOf(c))
}
