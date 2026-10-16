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
	backend     services.SearchBackend
	suggestions *services.SearchSuggestionService
	mu          sync.RWMutex
	analytics   []services.SearchAnalyticsEvent // in-memory ring; replace with DB or queue in production
}

// NewSearchHandler constructs a SearchHandler. Pass a DBSearchBackend or
// ESSearchBackend (or any SearchBackend) as the backend.
func NewSearchHandler(backend services.SearchBackend) *SearchHandler {
	return &SearchHandler{backend: backend, suggestions: services.NewSearchSuggestionService(backend)}
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
// @Param category query string false "Category slug (repeatable)"
// @Param tag query string false "Tag slug (repeatable)"
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
	req.UserID = c.GetUint("user_id")

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
	sh.recordAnalytics(req, result.Total, tookMs)
	sh.suggestions.RecordSearch(req.UserID, req.Query)

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

// Autocomplete handles GET /api/v1/search/autocomplete. Unlike the legacy
// suggestions endpoint, an empty query is valid and returns recent/popular
// searches for immediate display on focus.
func (sh *SearchHandler) Autocomplete(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	result, err := sh.suggestions.Autocomplete(c.Request.Context(), c.GetUint("user_id"), c.Query("q"), limit)
	if err != nil {
		Logger.Error("search autocomplete failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "autocomplete unavailable"})
		return
	}
	c.JSON(http.StatusOK, result)
}

// Facets returns filter values and counts for the search UI.
func (sh *SearchHandler) Facets(c *gin.Context) {
	result, err := sh.backend.Search(c.Request.Context(), &services.SearchRequest{Page: 1, Limit: 1})
	if err != nil {
		Logger.Error("search facets failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "facets unavailable"})
		return
	}
	toValues := func(buckets []services.FacetBucket) []gin.H {
		values := make([]gin.H, 0, len(buckets))
		for _, bucket := range buckets {
			values = append(values, gin.H{"label": bucket.Value, "value": bucket.Value, "count": bucket.Count})
		}
		return values
	}
	c.JSON(http.StatusOK, []gin.H{
		{"name": "Asset type", "key": "asset_type", "values": toValues(result.Facets.AssetTypes)},
		{"name": "Category", "key": "category", "values": toValues(result.Facets.Categories)},
		{"name": "Tag", "key": "tag", "values": toValues(result.Facets.Tags)},
		{"name": "Location", "key": "location", "values": toValues(result.Facets.Locations)},
		{"name": "Verified", "key": "verified", "values": toValues(result.Facets.Verified)},
	})
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
	c.JSON(http.StatusOK, gin.H{
		"events": sh.analytics,
		"total":  len(sh.analytics),
	})
}

func (sh *SearchHandler) recordAnalytics(req services.SearchRequest, count int64, tookMs float64) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
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
	if len(req.Categories) > 0 {
		filters["category"] = req.Categories
	}
	if len(req.Tags) > 0 {
		filters["tag"] = req.Tags
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
		Query:       req.Query,
		Filters:     filters,
		ResultCount: count,
		TookMs:      tookMs,
		Timestamp:   time.Now().UTC(),
	}
	if req.UserID != 0 {
		evt.UserID = strconv.FormatUint(uint64(req.UserID), 10)
	}

	// Keep the last 1000 events in memory.
	if len(sh.analytics) >= 1000 {
		sh.analytics = sh.analytics[1:]
	}
	sh.analytics = append(sh.analytics, evt)
}
