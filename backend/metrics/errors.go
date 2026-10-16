package metrics

// Error-rate metrics + error-budget helpers (issue #320).
//
// Technical HTTP metrics (http_requests_total) and the in-memory
// apperrors counters exist already; this file bridges them to Prometheus
// with endpoint + code labels so the 0.1% error budget in
// docs/ERROR_BUDGET_POLICY.md and monitoring/alerts/error_budget.yml can
// burn down from real data. Imported by backend/apperrors/tracking.go.

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// AppErrorsTotal counts application errors by code + endpoint.
	// Endpoint must be a route template (c.FullPath()), never raw IDs.
	AppErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "app_errors_total", Help: "Application errors by code and endpoint."},
		[]string{"code", "endpoint"},
	)
	// ErrorBudgetRemaining is set by a periodic reconciler (1 = full budget).
	ErrorBudgetRemaining = promauto.NewGaugeVec(
		prometheus.GaugeOpts{Name: "error_budget_remaining_ratio", Help: "Remaining error budget ratio (1 = intact)."},
		[]string{"service"},
	)
)

// RecordAppError increments the Prometheus error counter. Safe for hot paths.
func RecordAppError(code, endpoint string) {
	if code == "" {
		code = "internal_server_error"
	}
	if endpoint == "" {
		endpoint = "unknown"
	}
	AppErrorsTotal.WithLabelValues(code, endpoint).Inc()
}

// SetBudgetRemaining records remaining budget for a service (0..1).
func SetBudgetRemaining(service string, ratio float64) {
	if service == "" {
		service = "kor-assetforge"
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	ErrorBudgetRemaining.WithLabelValues(service).Set(ratio)
}
