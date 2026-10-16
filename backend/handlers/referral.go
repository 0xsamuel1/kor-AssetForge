package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/services"
	"gorm.io/gorm"
)

// ReferralHandler exposes endpoints for the user referral program: code
// retrieval, applying a code, viewing stats, and admin qualification (#170).
type ReferralHandler struct {
	Service *services.ReferralService
}

// NewReferralHandler creates a ReferralHandler.
func NewReferralHandler(db *gorm.DB) *ReferralHandler {
	return &ReferralHandler{Service: services.NewReferralService(db)}
}

// GetMyCode returns (creating if needed) the authenticated user's referral code.
// GET /api/v1/referrals/code
func (h *ReferralHandler) GetMyCode(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	code, err := h.Service.GetOrCreateCode(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get referral code"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": code})
}

// ApplyCode links the authenticated user as a referee of the given code.
// POST /api/v1/referrals/apply
func (h *ReferralHandler) ApplyCode(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	referral, err := h.Service.ApplyReferral(userID, req.Code, c.ClientIP())
	if err != nil {
		c.JSON(referralErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": referral})
}

// GetStats returns the authenticated user's referral statistics.
// GET /api/v1/referrals/stats
func (h *ReferralHandler) GetStats(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	stats, err := h.Service.Stats(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute referral stats"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": stats})
}

// ListReferrals returns the referrals made by the authenticated user.
// GET /api/v1/referrals
func (h *ReferralHandler) ListReferrals(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	referrals, err := h.Service.ListReferrals(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch referrals"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": referrals})
}

// QualifyReferral marks a referee's referral as qualified and distributes
// tiered rewards. Admin-only.
// POST /api/v1/admin/referrals/:refereeId/qualify
func (h *ReferralHandler) QualifyReferral(c *gin.Context) {
	refereeID, err := strconv.ParseUint(c.Param("refereeId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid referee id"})
		return
	}
	referral, err := h.Service.QualifyReferral(uint(refereeID))
	if err != nil {
		c.JSON(referralErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": referral})
}

// referralErrorStatus maps referral domain errors to HTTP status codes.
func referralErrorStatus(err error) int {
	switch {
	case errors.Is(err, services.ErrReferralNotFound):
		return http.StatusNotFound
	case errors.Is(err, services.ErrSelfReferral),
		errors.Is(err, services.ErrAlreadyReferred),
		errors.Is(err, services.ErrReferralCodeUsed),
		errors.Is(err, services.ErrReferralInactive):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
