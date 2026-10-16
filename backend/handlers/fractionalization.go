package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/apperrors"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/validator"
	"gorm.io/gorm"
)

type FractionHandler struct {
	db *gorm.DB
}

func NewFractionHandler(db *gorm.DB) *FractionHandler {
	return &FractionHandler{db: db}
}

type createFractionConfigRequest struct {
	AssetType              string  `json:"asset_type" binding:"required"`
	MinFractionSize        float64 `json:"min_fraction_size"`
	MaxFractionSize        float64 `json:"max_fraction_size"`
	MinInvestmentAmount    float64 `json:"min_investment_amount"`
	MaxFractionalOwners    int     `json:"max_fractional_owners"`
	RequireAccreditation   bool    `json:"require_accreditation"`
	MinHoldingPeriodDays   int     `json:"min_holding_period_days"`
	MaxHoldingPerOwner     float64 `json:"max_holding_per_owner_percent"`
	Enabled                bool    `json:"enabled"`
}

type updateFractionConfigRequest struct {
	MinFractionSize        *float64 `json:"min_fraction_size"`
	MaxFractionSize        *float64 `json:"max_fraction_size"`
	MinInvestmentAmount    *float64 `json:"min_investment_amount"`
	MaxFractionalOwners    *int     `json:"max_fractional_owners"`
	RequireAccreditation   *bool    `json:"require_accreditation"`
	MinHoldingPeriodDays   *int     `json:"min_holding_period_days"`
	MaxHoldingPerOwner     *float64 `json:"max_holding_per_owner_percent"`
	Enabled                *bool    `json:"enabled"`
}

type setAssetFractionLimitRequest struct {
	MinFractionSize  float64 `json:"min_fraction_size"`
	MaxFractionSize  float64 `json:"max_fraction_size"`
	MinInvestment    float64 `json:"min_investment"`
	MaxOwners        int     `json:"max_owners"`
	MaxPerOwner      float64 `json:"max_per_owner_percent"`
	OverrideGlobal   bool    `json:"override_global"`
}

func (h *FractionHandler) CreateConfig(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req createFractionConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeBadRequest, "Invalid request", http.StatusBadRequest))
		return
	}

	validator.SanitizeStruct(&req)

	config := models.FractionalizationConfig{
		AssetType:              req.AssetType,
		MinFractionSize:        defaultIfZero(req.MinFractionSize, 0.01),
		MaxFractionSize:        defaultIfZero(req.MaxFractionSize, 100.0),
		MinInvestmentAmount:    defaultIfZero(req.MinInvestmentAmount, 10.0),
		MaxFractionalOwners:    defaultIfZeroInt(req.MaxFractionalOwners, 1000),
		RequireAccreditation:   req.RequireAccreditation,
		MinHoldingPeriodDays:   req.MinHoldingPeriodDays,
		MaxHoldingPerOwner:     defaultIfZero(req.MaxHoldingPerOwner, 25.0),
		Enabled:                true,
		CreatedBy:              userID,
	}

	if err := h.db.Create(&config).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeDatabaseError, "Failed to create config", http.StatusInternalServerError))
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": config})
}

func (h *FractionHandler) ListConfigs(c *gin.Context) {
	var configs []models.FractionalizationConfig
	h.db.Find(&configs)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": configs})
}

func (h *FractionHandler) GetConfig(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid config ID"))
		return
	}

	var config models.FractionalizationConfig
	if err := h.db.First(&config, id).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("Config not found"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": config})
}

func (h *FractionHandler) GetConfigByAssetType(c *gin.Context) {
	assetType := c.Param("asset_type")
	var config models.FractionalizationConfig
	if err := h.db.Where("asset_type = ?", assetType).First(&config).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": h.getDefaultConfig(assetType)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": config})
}

func (h *FractionHandler) UpdateConfig(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid config ID"))
		return
	}

	var req updateFractionConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeBadRequest, "Invalid request", http.StatusBadRequest))
		return
	}

	var config models.FractionalizationConfig
	if err := h.db.First(&config, id).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("Config not found"))
		return
	}

	if req.MinFractionSize != nil {
		config.MinFractionSize = *req.MinFractionSize
	}
	if req.MaxFractionSize != nil {
		config.MaxFractionSize = *req.MaxFractionSize
	}
	if req.MinInvestmentAmount != nil {
		config.MinInvestmentAmount = *req.MinInvestmentAmount
	}
	if req.MaxFractionalOwners != nil {
		config.MaxFractionalOwners = *req.MaxFractionalOwners
	}
	if req.RequireAccreditation != nil {
		config.RequireAccreditation = *req.RequireAccreditation
	}
	if req.MinHoldingPeriodDays != nil {
		config.MinHoldingPeriodDays = *req.MinHoldingPeriodDays
	}
	if req.MaxHoldingPerOwner != nil {
		config.MaxHoldingPerOwner = *req.MaxHoldingPerOwner
	}
	if req.Enabled != nil {
		config.Enabled = *req.Enabled
	}

	h.db.Save(&config)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": config})
}

func (h *FractionHandler) DeleteConfig(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid config ID"))
		return
	}

	if err := h.db.Delete(&models.FractionalizationConfig{}, id).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("Config not found"))
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *FractionHandler) SetAssetFractionLimits(c *gin.Context) {
	var req setAssetFractionLimitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeBadRequest, "Invalid request", http.StatusBadRequest))
		return
	}

	assetID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid asset ID"))
		return
	}

	var asset models.Asset
	if err := h.db.First(&asset, assetID).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("Asset not found"))
		return
	}

	limit := models.AssetFractionLimit{
		AssetID:          uint(assetID),
		MinFractionSize:  req.MinFractionSize,
		MaxFractionSize:  req.MaxFractionSize,
		MinInvestment:    req.MinInvestment,
		MaxOwners:        req.MaxOwners,
		MaxPerOwner:      req.MaxPerOwner,
		OverrideGlobal:   req.OverrideGlobal,
	}

	if err := h.db.Where("asset_id = ?", assetID).Assign(&limit).FirstOrCreate(&models.AssetFractionLimit{}).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewDatabaseError("Failed to set limits", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": limit})
}

func (h *FractionHandler) GetAssetFractionLimits(c *gin.Context) {
	assetID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Invalid asset ID"))
		return
	}

	var limit models.AssetFractionLimit
	if err := h.db.Where("asset_id = ?", assetID).First(&limit).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": limit})
}

func (h *FractionHandler) ValidateFractionalization(c *gin.Context) {
	var req struct {
		AssetID      uint    `json:"asset_id" binding:"required"`
		FractionSize float64 `json:"fraction_size" binding:"required"`
		InvestorID   uint    `json:"investor_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeBadRequest, "Invalid request", http.StatusBadRequest))
		return
	}

	var asset models.Asset
	if err := h.db.First(&asset, req.AssetID).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("Asset not found"))
		return
	}

	config, err := h.GetApplicableConfig(asset.AssetType)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("Fractionalization not configured for this asset type"))
		return
	}

	var limit models.AssetFractionLimit
	hasOverride := h.db.Where("asset_id = ? AND override_global = ?", req.AssetID, true).First(&limit).Error == nil

	validation := make(map[string]interface{})
	validation["valid"] = true
	validation["checks"] = []string{}

	minSize := config.MinFractionSize
	maxSize := config.MaxFractionSize
	if hasOverride {
		if limit.MinFractionSize > 0 {
			minSize = limit.MinFractionSize
		}
		if limit.MaxFractionSize > 0 {
			maxSize = limit.MaxFractionSize
		}
	}

	if req.FractionSize < minSize {
		validation["valid"] = false
		validation["checks"] = append(validation["checks"].([]string), "Fraction size below minimum")
	}
	if req.FractionSize > maxSize {
		validation["valid"] = false
		validation["checks"] = append(validation["checks"].([]string), "Fraction size above maximum")
	}

	var totalOwners int64
	h.db.Model(&models.UserBalance{}).Where("asset_id = ? AND balance > 0", req.AssetID).Count(&totalOwners)
	if totalOwners >= int64(config.MaxFractionalOwners) {
		validation["valid"] = false
		validation["checks"] = append(validation["checks"].([]string), "Maximum number of fractional owners reached")
	}

	if config.RequireAccreditation {
		var investor models.User
		if err := h.db.First(&investor, req.InvestorID).Error; err != nil || !investor.AccreditedInvestor {
			validation["valid"] = false
			validation["checks"] = append(validation["checks"].([]string), "Investor accreditation required")
		}
	}

	validation["config"] = config
	c.JSON(http.StatusOK, gin.H{"success": true, "data": validation})
}

func (h *FractionHandler) GetApplicableConfig(assetType string) (*models.FractionalizationConfig, error) {
	var config models.FractionalizationConfig
	if err := h.db.Where("asset_type = ?", assetType).First(&config).Error; err != nil {
		return nil, err
	}
	return &config, nil
}

func (h *FractionHandler) getDefaultConfig(assetType string) models.FractionalizationConfig {
	return models.FractionalizationConfig{
		AssetType:            assetType,
		MinFractionSize:      0.01,
		MaxFractionSize:      100.0,
		MinInvestmentAmount:  10.0,
		MaxFractionalOwners:  1000,
		RequireAccreditation: false,
		MinHoldingPeriodDays: 0,
		MaxHoldingPerOwner:   25.0,
		Enabled:              true,
	}
}

func defaultIfZero(val, defaultVal float64) float64 {
	if val == 0 {
		return defaultVal
	}
	return val
}

func defaultIfZeroInt(val, defaultVal int) int {
	if val == 0 {
		return defaultVal
	}
	return val
}
