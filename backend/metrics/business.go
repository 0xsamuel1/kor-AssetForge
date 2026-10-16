package metrics

// Business KPI metrics (issue #319).
//
// Technical RED metrics live in backend/monitoring/prometheus.go and
// backend/handlers/metrics.go. This file adds the business layer:
// DAU/MAU signals, transaction volumes, and revenue — exported to
// Prometheus (scraped from /metrics) and visualized in
// monitoring/dashboards/business_metrics.json.

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// AssetsTokenizedTotal counts successful tokenizations by asset type.
	AssetsTokenizedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "assets_tokenized_total", Help: "Total assets tokenized."},
		[]string{"asset_type"},
	)
	// MarketplaceOrdersTotal counts P2P/marketplace orders by side + status.
	MarketplaceOrdersTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "marketplace_orders_total", Help: "Marketplace orders created."},
		[]string{"side", "status"},
	)
	// TransactionVolumeXLMTotal accumulates traded volume in XLM.
	TransactionVolumeXLMTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "transaction_volume_xlm_total", Help: "Cumulative transaction volume in XLM."},
		[]string{"kind"},
	)
	// FeesCollectedXLMTotal accumulates platform revenue in XLM.
	FeesCollectedXLMTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "fees_collected_xlm_total", Help: "Cumulative fees collected in XLM."},
		[]string{"source"},
	)
	// KYCVerificationsTotal counts KYC outcomes.
	KYCVerificationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "kyc_verifications_total", Help: "KYC verifications by result."},
		[]string{"result"},
	)
	// WebhookDeliveriesTotal counts outgoing webhook attempts by status.
	WebhookDeliveriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "webhook_deliveries_total", Help: "Outgoing webhook deliveries by status."},
		[]string{"status"},
	)
	// ActiveUsers tracks currently active users (set by tracker, e.g. distinct
	// authenticated user_ids seen in a window). DAU/MAU are derived in
	// PromQL with countDistinct-style queries over this gauge + audit data.
	ActiveUsers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{Name: "active_users", Help: "Active users by window."},
		[]string{"window"},
	)
)

// Record helpers — thin wrappers so handlers/services don't touch prom API.

func IncAssetsTokenized(assetType string) {
	if assetType == "" {
		assetType = "unknown"
	}
	AssetsTokenizedTotal.WithLabelValues(assetType).Inc()
}

func IncOrderCreated(side, status string) {
	if side == "" {
		side = "unknown"
	}
	if status == "" {
		status = "created"
	}
	MarketplaceOrdersTotal.WithLabelValues(side, status).Inc()
}

func AddTransactionVolume(kind string, amountXLM float64) {
	if kind == "" {
		kind = "trade"
	}
	if amountXLM < 0 {
		return
	}
	TransactionVolumeXLMTotal.WithLabelValues(kind).Add(amountXLM)
}

func AddRevenue(source string, amountXLM float64) {
	if source == "" {
		source = "fees"
	}
	if amountXLM < 0 {
		return
	}
	FeesCollectedXLMTotal.WithLabelValues(source).Add(amountXLM)
}

func IncKYCVerified(result string) {
	if result == "" {
		result = "unknown"
	}
	KYCVerificationsTotal.WithLabelValues(result).Inc()
}

func IncWebhookDelivery(status string) {
	if status == "" {
		status = "unknown"
	}
	WebhookDeliveriesTotal.WithLabelValues(status).Inc()
}

// SetActiveUsers sets the active-user gauge for a window ("dau"/"mau").
func SetActiveUsers(window string, n float64) {
	if window == "" {
		window = "dau"
	}
	ActiveUsers.WithLabelValues(window).Set(n)
}
