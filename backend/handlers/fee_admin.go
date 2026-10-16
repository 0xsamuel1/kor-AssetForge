package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
	"gorm.io/gorm"
)

// FeeAdminHandler handles admin endpoints for dynamic fee configuration.
type FeeAdminHandler struct {
	db         *gorm.DB
	feeService *services.FeeService
}

// NewFeeAdminHandler creates a new FeeAdminHandler.
func NewFeeAdminHandler(db *gorm.DB, feeService *services.FeeService) *FeeAdminHandler {
	return &FeeAdminHandler{
		db:         db,
		feeService: feeService,
	}
}

// createFeeConfigRequest is the request body for creating a fee config.
type createFeeConfigRequest struct {
	Name            string          `json:"name" binding:"required"`
	FeeType         models.FeeType  `json:"fee_type" binding:"required"`
	BaseBasisPoints int             `json:"base_basis_points" binding:"required,min=0"`
	MinBasisPoints  int             `json:"min_basis_points" binding:"min=0"`
	MaxBasisPoints  int             `json:"max_basis_points" binding:"required,min=0"`
	VolumeTiers     json.RawMessage `json:"volume_tiers"`
	EffectiveFrom   *time.Time      `json:"effective_from"`
	EffectiveUntil  *time.Time      `json:"effective_until"`
	Reason          string          `json:"reason"`
}

// CreateFeeConfig handles POST /api/v1/admin/fees
// @Summary Create fee configuration (admin)
// @Description Create a new dynamic fee rule with optional volume tiers
// @Tags fees
// @Accept json
// @Produce json
// @Param body body createFeeConfigRequest true "Fee configuration"
// @Success 201 {object} models.FeeConfig
// @Router /admin/fees [post]
func (h *FeeAdminHandler) CreateFeeConfig(c *gin.Context) {
	adminID := getRequestingAdminID(c)

	var req createFeeConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tiersJSON := "[]"
	if len(req.VolumeTiers) > 0 && string(req.VolumeTiers) != "null" {
		tiersJSON = string(req.VolumeTiers)
	}

	cfg := &models.FeeConfig{
		Name:            req.Name,
		FeeType:         req.FeeType,
		BaseBasisPoints: req.BaseBasisPoints,
		MinBasisPoints:  req.MinBasisPoints,
		MaxBasisPoints:  req.MaxBasisPoints,
		VolumeTiers:     tiersJSON,
		Active:          true,
		EffectiveFrom:   resolveTime(req.EffectiveFrom, time.Now()),
		EffectiveUntil:  req.EffectiveUntil,
	}

	if err := h.feeService.CreateFeeConfig(cfg, adminID, req.Reason); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, cfg)
}

// updateFeeConfigRequest is the body for updating an existing fee config.
type updateFeeConfigRequest struct {
	Name            string          `json:"name" binding:"required"`
	FeeType         models.FeeType  `json:"fee_type" binding:"required"`
	BaseBasisPoints int             `json:"base_basis_points" binding:"min=0"`
	MinBasisPoints  int             `json:"min_basis_points" binding:"min=0"`
	MaxBasisPoints  int             `json:"max_basis_points" binding:"min=0"`
	VolumeTiers     json.RawMessage `json:"volume_tiers"`
	EffectiveFrom   *time.Time      `json:"effective_from"`
	EffectiveUntil  *time.Time      `json:"effective_until"`
	Active          *bool           `json:"active"`
	Reason          string          `json:"reason"`
}

// UpdateFeeConfig handles PUT /api/v1/admin/fees/:id
// @Summary Update fee configuration (admin)
// @Description Modify an existing fee rule
// @Tags fees
// @Accept json
// @Produce json
// @Param id path int true "Fee config ID"
// @Param body body updateFeeConfigRequest true "Updated fee configuration"
// @Success 200 {object} models.FeeConfig
// @Router /admin/fees/{id} [put]
func (h *FeeAdminHandler) UpdateFeeConfig(c *gin.Context) {
	adminID := getRequestingAdminID(c)

	id, err := parseIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid fee config ID"})
		return
	}

	var req updateFeeConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tiersJSON := "[]"
	if len(req.VolumeTiers) > 0 && string(req.VolumeTiers) != "null" {
		tiersJSON = string(req.VolumeTiers)
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}

	updates := &models.FeeConfig{
		Name:            req.Name,
		FeeType:         req.FeeType,
		BaseBasisPoints: req.BaseBasisPoints,
		MinBasisPoints:  req.MinBasisPoints,
		MaxBasisPoints:  req.MaxBasisPoints,
		VolumeTiers:     tiersJSON,
		Active:          active,
		EffectiveFrom:   resolveTime(req.EffectiveFrom, time.Now()),
		EffectiveUntil:  req.EffectiveUntil,
	}

	cfg, err := h.feeService.UpdateFeeConfig(id, updates, adminID, req.Reason)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) || err.Error() == "fee config not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, cfg)
}

// DeactivateFeeConfig handles DELETE /api/v1/admin/fees/:id
// @Summary Deactivate fee configuration (admin)
// @Description Mark a fee rule as inactive (soft-deactivate, not deleted)
// @Tags fees
// @Param id path int true "Fee config ID"
// @Success 200 {object} map[string]string
// @Router /admin/fees/{id} [delete]
func (h *FeeAdminHandler) DeactivateFeeConfig(c *gin.Context) {
	adminID := getRequestingAdminID(c)

	id, err := parseIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid fee config ID"})
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)

	if err := h.feeService.DeactivateFeeConfig(id, adminID, body.Reason); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "fee config not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "fee config deactivated successfully"})
}

// ListFeeConfigs handles GET /api/v1/admin/fees
// @Summary List fee configurations (admin)
// @Description Retrieve all fee configs with optional type filter
// @Tags fees
// @Param fee_type query string false "Fee type filter"
// @Param page query int false "Page number"
// @Param limit query int false "Page size"
// @Success 200 {object} map[string]interface{}
// @Router /admin/fees [get]
func (h *FeeAdminHandler) ListFeeConfigs(c *gin.Context) {
	feeType := c.Query("fee_type")
	page := queryIntDefault(c, "page", 1)
	limit := queryIntDefault(c, "limit", 20)
	if limit > 100 {
		limit = 100
	}

	configs, total, err := h.feeService.ListFeeConfigs(feeType, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  configs,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetFeeConfig handles GET /api/v1/admin/fees/:id
// @Summary Get fee configuration by ID (admin)
// @Tags fees
// @Param id path int true "Fee config ID"
// @Success 200 {object} models.FeeConfig
// @Router /admin/fees/{id} [get]
func (h *FeeAdminHandler) GetFeeConfig(c *gin.Context) {
	id, err := parseIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid fee config ID"})
		return
	}

	var cfg models.FeeConfig
	if err := h.db.First(&cfg, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "fee config not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch fee config"})
		return
	}

	c.JSON(http.StatusOK, cfg)
}

// GetFeeAuditLog handles GET /api/v1/admin/fees/:id/audit
// @Summary Get fee config audit log (admin)
// @Tags fees
// @Param id path int true "Fee config ID"
// @Success 200 {object} map[string]interface{}
// @Router /admin/fees/{id}/audit [get]
func (h *FeeAdminHandler) GetFeeAuditLog(c *gin.Context) {
	id, err := parseIDParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid fee config ID"})
		return
	}

	page := queryIntDefault(c, "page", 1)
	limit := queryIntDefault(c, "limit", 20)

	logs, total, err := h.feeService.GetAuditLog(id, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  logs,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// PreviewFee handles GET /api/v1/fees/preview
// @Summary Preview effective fee for a transaction
// @Description Returns the computed fee for a given transaction amount and user's 30-day volume
// @Tags fees
// @Param fee_type query string true "Fee type"
// @Param amount query int true "Transaction amount in stroops"
// @Param user_volume query int false "User's 30-day volume in stroops"
// @Success 200 {object} models.EffectiveFee
// @Router /fees/preview [get]
func (h *FeeAdminHandler) PreviewFee(c *gin.Context) {
	feeTypeStr := c.Query("fee_type")
	amountStr := c.Query("amount")
	userVolumeStr := c.DefaultQuery("user_volume", "0")

	if feeTypeStr == "" || amountStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "fee_type and amount are required"})
		return
	}

	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil || amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "amount must be a positive integer"})
		return
	}

	userVolume, _ := strconv.ParseInt(userVolumeStr, 10, 64)

	result, err := h.feeService.CalculateFee(models.FeeType(feeTypeStr), amount, userVolume)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetActiveFee handles GET /api/v1/fees/active
// @Summary Get the active fee config for a fee type
// @Tags fees
// @Param fee_type query string true "Fee type"
// @Success 200 {object} models.FeeConfig
// @Router /fees/active [get]
func (h *FeeAdminHandler) GetActiveFee(c *gin.Context) {
	feeTypeStr := c.Query("fee_type")
	if feeTypeStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "fee_type is required"})
		return
	}

	cfg, err := h.feeService.GetActiveFeeConfig(models.FeeType(feeTypeStr))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, cfg)
}

// getRequestingAdminID extracts the authenticated user's ID from the gin context.
// The JWT middleware stores it as "user_id".
func getRequestingAdminID(c *gin.Context) uint {
	if val, exists := c.Get("user_id"); exists {
		if id, ok := val.(uint); ok {
			return id
		}
	}
	return 0
}

// parseIDParam parses a uint path parameter from the context.
func parseIDParam(c *gin.Context, key string) (uint, error) {
	raw := c.Param(key)
	val, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint(val), nil
}

// queryIntDefault returns query param as int or a default value.
func queryIntDefault(c *gin.Context, key string, def int) int {
	raw := c.Query(key)
	if raw == "" {
		return def
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val < 1 {
		return def
	}
	return val
}

// resolveTime returns t if non-nil, otherwise the fallback.
func resolveTime(t *time.Time, fallback time.Time) time.Time {
	if t != nil {
		return *t
	}
	return fallback
}
