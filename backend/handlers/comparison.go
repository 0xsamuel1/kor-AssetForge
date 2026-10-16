package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/services"
	"gorm.io/gorm"
)

// ComparisonHandler exposes endpoints for side-by-side comparison of multiple
// assets, with comparison history (#167).
type ComparisonHandler struct {
	Service *services.ComparisonService
}

// NewComparisonHandler creates a ComparisonHandler.
func NewComparisonHandler(db *gorm.DB) *ComparisonHandler {
	return &ComparisonHandler{Service: services.NewComparisonService(db)}
}

// Compare compares 2-10 assets side by side. When the caller is authenticated and
// save=true, the comparison is persisted to their history.
// POST /api/v1/comparisons
func (h *ComparisonHandler) Compare(c *gin.Context) {
	var req struct {
		AssetIDs []uint   `json:"asset_ids" binding:"required"`
		Criteria []string `json:"criteria"`
		Save     bool     `json:"save"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.Service.Compare(req.AssetIDs, req.Criteria)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Save {
		if userID := currentUserID(c); userID != 0 {
			if _, err := h.Service.SaveHistory(userID, req.AssetIDs, result.Criteria); err != nil {
				// History persistence is best-effort; don't fail the comparison.
				c.Header("X-Comparison-History", "save-failed")
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// ListHistory returns the authenticated user's saved comparisons.
// GET /api/v1/comparisons/history
func (h *ComparisonHandler) ListHistory(c *gin.Context) {
	userID := currentUserID(c)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	records, err := h.Service.ListHistory(userID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch comparison history"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": records})
}

// GetHistory re-runs a saved comparison and returns fresh results.
// GET /api/v1/comparisons/history/:id
func (h *ComparisonHandler) GetHistory(c *gin.Context) {
	userID := currentUserID(c)
	historyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid history id"})
		return
	}
	record, result, err := h.Service.GetHistory(userID, uint(historyID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "comparison not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"history": record, "comparison": result}})
}
