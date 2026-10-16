package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/apperrors"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
	"github.com/yourusername/kor-assetforge/validator"
	"gorm.io/gorm"
)

type RecommendationHandler struct {
	db            *gorm.DB
	recommendationService *services.RecommendationService
}

func NewRecommendationHandler(db *gorm.DB) *RecommendationHandler {
	return &RecommendationHandler{
		db:            db,
		recommendationService: services.NewRecommendationService(db),
	}
}

type recordInteractionRequest struct {
	AssetID         uint                    `json:"asset_id" binding:"required"`
	InteractionType models.InteractionType  `json:"interaction_type" binding:"required"`
	Metadata        map[string]interface{}  `json:"metadata,omitempty"`
}

type updatePreferencesRequest struct {
	Categories    []string `json:"categories"`
	Tags          []string `json:"tags"`
	PriceRangeMin float64  `json:"price_range_min"`
	PriceRangeMax float64  `json:"price_range_max"`
	RiskTolerance string   `json:"risk_tolerance"`
}

func (h *RecommendationHandler) GetRecommendations(c *gin.Context) {
	userID := c.GetUint("user_id")

	recommendations, err := h.recommendationService.GetRecommendations(userID, 20)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to fetch recommendations"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"recommendations": recommendations,
	})
}

func (h *RecommendationHandler) RecordInteraction(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req recordInteractionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeBadRequest, "Invalid request", http.StatusBadRequest))
		return
	}

	validator.SanitizeStruct(&req)

	if err := h.recommendationService.RecordInteraction(userID, req.AssetID, req.InteractionType, req.Metadata); err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to record interaction"))
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "Interaction recorded"})
}

func (h *RecommendationHandler) MarkRecommendationViewed(c *gin.Context) {
	userID := c.GetUint("user_id")

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid recommendation ID"))
		return
	}

	if err := h.recommendationService.MarkAsViewed(userID, uint(id)); err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("Recommendation not found"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *RecommendationHandler) GetContentBasedRecommendations(c *gin.Context) {
	userID := c.GetUint("user_id")

	recommendations, err := h.recommendationService.GetContentBasedRecommendations(userID, 20)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to fetch recommendations"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"recommendations": recommendations,
	})
}

func (h *RecommendationHandler) UpdatePreferences(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req updatePreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeBadRequest, "Invalid request", http.StatusBadRequest))
		return
	}

	validator.SanitizeStruct(&req)

	if err := h.recommendationService.UpdateUserPreferences(userID, req.Categories, req.Tags, req.PriceRangeMin, req.PriceRangeMax, req.RiskTolerance); err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to update preferences"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Preferences updated"})
}
