package handlers

// Business-metrics hook (issue #319).
//
// Instead of editing every handler, this middleware infers business events
// from successfully completed requests (status < 400) and forwards them to
// the Prometheus business counters via services.AnalyticsTracker. Route
// templates (c.FullPath()) are used so metric cardinality stays bounded.
// Handlers that know exact amounts (volume/revenue) should additionally call
// the tracker directly with the real figures — this middleware records the
// occurrence, direct calls record the value.

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/services"
)

// BusinessMetricsMiddleware returns a gin middleware tracking business KPIs.
func BusinessMetricsMiddleware(tracker *services.AnalyticsTracker) gin.HandlerFunc {
	var t *services.AnalyticsTracker
	if tracker != nil {
		t = tracker
	} else {
		t = services.NewAnalyticsTracker()
	}
	return func(c *gin.Context) {
		c.Next()
		if c.Writer.Status() >= 400 {
			return
		}
		ctx := c.Request.Context()
		route := c.FullPath()
		method := c.Request.Method
		switch {
		case method == "POST" && strings.HasPrefix(route, "/api/v1/assets"):
			t.TrackAssetTokenized(ctx, c.Param("asset_type"))
		case strings.HasPrefix(route, "/api/v1/p2p/orders") && method == "POST":
			t.TrackOrderCreated(ctx, "p2p", "created")
		case strings.HasPrefix(route, "/api/v1/marketplace/"):
			t.TrackOrderCreated(ctx, "marketplace", "created")
		case strings.HasPrefix(route, "/api/v1/staking/"):
			t.TrackTransactionVolume(ctx, "staking", 0)
		case strings.HasPrefix(route, "/api/v1/kyc/"):
			t.TrackKYCResult(ctx, "submitted")
		}
	}
}
