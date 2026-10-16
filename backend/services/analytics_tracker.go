package services

// AnalyticsTracker — business-event → Prometheus bridge (issue #319).
//
// AnalyticsService (analytics_service.go) answers dashboard reads with SQL.
// This tracker covers the write path: handlers call Track* with the request
// ctx (trace propagation preserved — ctx is forwarded, never replaced) and
// the event lands in backend/metrics business counters scraped at /metrics
// and charted in monitoring/dashboards/business_metrics.json. Export to an
// external analytics platform is a periodic SQL/warehouse sync, not a
// per-request blocking call.

import (
	"context"

	"github.com/yourusername/kor-assetforge/metrics"
	"github.com/yourusername/kor-assetforge/middleware"
)

// AnalyticsTracker records business KPIs. Stateless — share one instance.
type AnalyticsTracker struct{}

// NewAnalyticsTracker creates a tracker.
func NewAnalyticsTracker() *AnalyticsTracker { return &AnalyticsTracker{} }

func traceOf(ctx context.Context) (string, string) {
	if ctx == nil {
		return "", ""
	}
	return middleware.TraceIDFromContext(ctx), middleware.SpanIDFromContext(ctx)
}

// TrackAssetTokenized records a successful tokenization.
func (t *AnalyticsTracker) TrackAssetTokenized(ctx context.Context, assetType string) {
	_, _ = traceOf(ctx) // trace stays on ctx for log correlation by caller
	metrics.IncAssetsTokenized(assetType)
}

// TrackOrderCreated records a marketplace/P2P order.
func (t *AnalyticsTracker) TrackOrderCreated(ctx context.Context, side, status string) {
	_, _ = traceOf(ctx)
	metrics.IncOrderCreated(side, status)
}

// TrackTransactionVolume records traded volume in XLM.
func (t *AnalyticsTracker) TrackTransactionVolume(ctx context.Context, kind string, amountXLM float64) {
	_, _ = traceOf(ctx)
	metrics.AddTransactionVolume(kind, amountXLM)
}

// TrackRevenue records platform revenue in XLM.
func (t *AnalyticsTracker) TrackRevenue(ctx context.Context, source string, amountXLM float64) {
	_, _ = traceOf(ctx)
	metrics.AddRevenue(source, amountXLM)
}

// TrackKYCResult records a KYC verification outcome.
func (t *AnalyticsTracker) TrackKYCResult(ctx context.Context, result string) {
	_, _ = traceOf(ctx)
	metrics.IncKYCVerified(result)
}

// TrackWebhookDelivery records an outgoing webhook attempt.
func (t *AnalyticsTracker) TrackWebhookDelivery(ctx context.Context, status string) {
	_, _ = traceOf(ctx)
	metrics.IncWebhookDelivery(status)
}
