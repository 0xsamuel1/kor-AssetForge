package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/services"
	"go.uber.org/zap"
)

// SearchHandler provides asset search and suggestion endpoints.
type SearchHandler struct {
	backend   services.SearchBackend
	analytics []services.SearchAnalyticsEvent // in-memory ring; replace with DB or queue in production
	clicks    map[string]int
	mu        sync.RWMutex
}

// NewSearchHandler constructs a SearchHandler. Pass a DBSearchBackend or
// ESSearchBackend (or any SearchBackend) as the backend.
func NewSearchHandler(backend services.SearchBackend) *SearchHandler {
	return &SearchHandler{backend: backend, clicks: make(map[string]int)}
}

// Search handles GET /api/v1/search/assets
// @Summary Search assets
// @Description Perform a full-text search on assets with various filters
// @Tags search
// @Accept json
// @Produce json
// @Param q query string false "Search term"
// @Param asset_type query string false "Filter by asset type"
// @Param min_price query float64 false "Minimum price"
// @Param max_price query float64 false "Maximum price"
// @Param location query string false "Filter by location (repeatable)"
// @Param created_from query string false "Assets created on or after YYYY-MM-DD"
// @Param created_to query string false "Assets created on or before YYYY-MM-DD"
// @Param metadata.{field} query string false "Exact custom metadata match"
// @Param verified query boolean false "Filter by verified status"
// @Param sort_by query string false "Sort field"
// @Param order query string false "Sort order"
// @Param page query int false "Page number"
// @Param limit query int false "Page size"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 500 {object} apperrors.ErrorResponse
// @Router /search/assets [get]
func (sh *SearchHandler) Search(c *gin.Context) {
	var req services.SearchRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Metadata = map[string]string{}
	for key, values := range c.Request.URL.Query() {
		if len(values) > 0 && len(key) > len("metadata.") && key[:len("metadata.")] == "metadata." {
			req.Metadata[key[len("metadata."):]] = values[len(values)-1]
		}
	}
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	start := time.Now()
	result, err := sh.backend.Search(c.Request.Context(), &req)
	if err != nil {
		Logger.Error("asset search failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "search unavailable"})
		return
	}

	tookMs := float64(time.Since(start).Microseconds()) / 1000.0
	result.Took = tookMs

	// Record analytics (fire-and-forget).
	result.AnalyticsEventID = sh.recordAnalytics(req, result.Total, tookMs)

	c.JSON(http.StatusOK, result)
}

// Suggest handles GET /api/v1/search/suggestions
// @Summary Suggest search terms
// @Description Get auto-complete suggestions based on a search prefix
// @Tags search
// @Accept json
// @Produce json
// @Param q query string true "Search prefix"
// @Param limit query int false "Max suggestions"
// @Success 200 {object} []string
// @Failure 400 {object} apperrors.ErrorResponse
// @Failure 500 {object} apperrors.ErrorResponse
// @Router /search/suggestions [get]
func (sh *SearchHandler) Suggest(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q param required"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit <= 0 || limit > 20 {
		limit = 10
	}

	result, err := sh.backend.Suggest(c.Request.Context(), query, limit)
	if err != nil {
		Logger.Error("search suggest failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "suggest unavailable"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// SearchAnalytics handles GET /api/v1/search/analytics
// @Summary Get search analytics
// @Description Returns the last N recorded search events for observability.
// @Tags search
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /search/analytics [get]
func (sh *SearchHandler) SearchAnalytics(c *gin.Context) {
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	zeroResults := 0
	queryCounts := map[string]int{}
	clicks := 0
	for _, event := range sh.analytics {
		if event.ResultCount == 0 {
			zeroResults++
		}
		queryCounts[event.Query]++
		clicks += sh.clicks[event.EventID]
	}
	clickRate := float64(0)
	if len(sh.analytics) > 0 {
		clickRate = float64(clicks) / float64(len(sh.analytics))
	}
	c.JSON(http.StatusOK, gin.H{
		"events":               sh.analytics,
		"total":                len(sh.analytics),
		"zero_result_searches": zeroResults,
		"clicks":               clicks,
		"click_through_rate":   clickRate,
		"popular_queries":      queryCounts,
	})
}

// RecordSearchClick attributes a result click to a previously recorded search.
func (sh *SearchHandler) RecordSearchClick(c *gin.Context) {
	var req struct {
		EventID string `json:"event_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.EventID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "event_id is required"})
		return
	}
	sh.mu.Lock()
	defer sh.mu.Unlock()
	for _, event := range sh.analytics {
		if event.EventID == req.EventID {
			sh.clicks[req.EventID]++
			c.JSON(http.StatusAccepted, gin.H{"event_id": req.EventID, "clicks": sh.clicks[req.EventID]})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "search event not found"})
}

func (sh *SearchHandler) recordAnalytics(req services.SearchRequest, count int64, tookMs float64) string {
	filters := map[string]interface{}{}
	if req.AssetType != "" || len(req.AssetTypes) > 0 {
		filters["asset_type"] = fmt.Sprint(req.AssetTypes)
	}
	if req.Verified != nil {
		filters["verified"] = *req.Verified
	}
	if req.MinPrice != nil {
		filters["min_price"] = *req.MinPrice
	}
	if req.MaxPrice != nil {
		filters["max_price"] = *req.MaxPrice
	}
	if len(req.Locations) > 0 || req.Location != "" {
		filters["location"] = fmt.Sprint(req.Locations)
	}
	if req.CreatedFrom != nil {
		filters["created_from"] = req.CreatedFrom.Format(time.RFC3339)
	}
	if req.CreatedTo != nil {
		filters["created_to"] = req.CreatedTo.Format(time.RFC3339)
	}
	if len(req.Metadata) > 0 {
		filters["metadata"] = req.Metadata
	}

	evt := services.SearchAnalyticsEvent{
		EventID:     fmt.Sprintf("search-%d", time.Now().UnixNano()),
		Query:       req.Query,
		Filters:     filters,
		ResultCount: count,
		TookMs:      tookMs,
	}

	// Keep the last 1000 events in memory.
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if len(sh.analytics) >= 1000 {
		sh.analytics = sh.analytics[1:]
	}
	sh.analytics = append(sh.analytics, evt)
	return evt.EventID
}
