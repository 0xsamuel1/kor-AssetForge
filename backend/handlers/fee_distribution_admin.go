package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/yourusername/kor-assetforge/apperrors"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
)

// FeeDistributionAdminHandler exposes administrator endpoints for configuring
// and triggering automated fee distribution to platform, liquidity provider,
// and token holder stakeholders.
type FeeDistributionAdminHandler struct {
	distributionService *services.FeeDistributionService
}

// NewFeeDistributionAdminHandler creates a new FeeDistributionAdminHandler.
func NewFeeDistributionAdminHandler(db *gorm.DB) *FeeDistributionAdminHandler {
	return &FeeDistributionAdminHandler{distributionService: services.NewFeeDistributionService(db)}
}

// CreateRuleRequest defines a new fee distribution rule.
type CreateRuleRequest struct {
	Name                       string `json:"name" binding:"required"`
	PlatformShareBps           int    `json:"platform_share_bps" binding:"min=0,max=10000"`
	LiquidityProvidersShareBps int    `json:"liquidity_providers_share_bps" binding:"min=0,max=10000"`
	TokenHoldersShareBps       int    `json:"token_holders_share_bps" binding:"min=0,max=10000"`
}

// RunDistributionRequest triggers a distribution run for a given period.
type RunDistributionRequest struct {
	PeriodStart time.Time `json:"period_start" binding:"required"`
	PeriodEnd   time.Time `json:"period_end" binding:"required"`
}

// CreateRule creates a new fee distribution rule.
// @Summary Create a fee distribution rule
// @Description Defines how collected fees are split between platform, liquidity providers, and token holders (shares in basis points, must sum to 10000)
// @Tags fee-admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param rule body handlers.CreateRuleRequest true "Distribution rule"
// @Success 201 {object} models.FeeDistributionRule
// @Failure 400 {object} apperrors.ErrorResponse
// @Router /admin/fee-distribution/rules [post]
func (h *FeeDistributionAdminHandler) CreateRule(c *gin.Context) {
	var req CreateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("Invalid request data", err))
		return
	}

	rule := &models.FeeDistributionRule{
		Name:                       req.Name,
		PlatformShareBps:           req.PlatformShareBps,
		LiquidityProvidersShareBps: req.LiquidityProvidersShareBps,
		TokenHoldersShareBps:       req.TokenHoldersShareBps,
	}

	if err := h.distributionService.CreateRule(rule); err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError(err.Error()))
		return
	}

	c.JSON(http.StatusCreated, rule)
}

// ListRules lists all configured fee distribution rules.
// @Summary List fee distribution rules
// @Tags fee-admin
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /admin/fee-distribution/rules [get]
func (h *FeeDistributionAdminHandler) ListRules(c *gin.Context) {
	rules, err := h.distributionService.ListRules()
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to list distribution rules"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

// ActivateRule marks a rule as the active distribution rule.
// @Summary Activate a fee distribution rule
// @Tags fee-admin
// @Security BearerAuth
// @Produce json
// @Param id path int true "Rule ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} apperrors.ErrorResponse
// @Router /admin/fee-distribution/rules/{id}/activate [post]
func (h *FeeDistributionAdminHandler) ActivateRule(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	if err := h.distributionService.ActivateRule(id); err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Distribution rule activated"})
}

// RunDistribution triggers an automated fee distribution run for the given period.
// @Summary Run a fee distribution
// @Description Computes and records the fee split for a period using the active rule, allocating amounts to platform, liquidity providers, and token holders
// @Tags fee-admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body handlers.RunDistributionRequest true "Distribution period"
// @Success 201 {object} models.FeeDistributionRun
// @Failure 400 {object} apperrors.ErrorResponse
// @Router /admin/fee-distribution/runs [post]
func (h *FeeDistributionAdminHandler) RunDistribution(c *gin.Context) {
	var req RunDistributionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.AbortWithError(c, apperrors.NewValidationError("Invalid request data", err))
		return
	}

	if !req.PeriodEnd.After(req.PeriodStart) {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError("period_end must be after period_start"))
		return
	}

	run, err := h.distributionService.RunDistribution(req.PeriodStart, req.PeriodEnd, "manual")
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewBadRequestError(err.Error()))
		return
	}

	c.JSON(http.StatusCreated, run)
}

// ListRuns lists recent fee distribution runs.
// @Summary List fee distribution runs
// @Tags fee-admin
// @Security BearerAuth
// @Produce json
// @Param limit query int false "Maximum number of runs to return (default 20)"
// @Success 200 {object} map[string]interface{}
// @Router /admin/fee-distribution/runs [get]
func (h *FeeDistributionAdminHandler) ListRuns(c *gin.Context) {
	limit := 20
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}

	runs, err := h.distributionService.ListRuns(limit)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewInternalError("Failed to list distribution runs"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"runs": runs})
}

// GetRun returns a single distribution run with its per-recipient allocations.
// @Summary Get a fee distribution run
// @Tags fee-admin
// @Security BearerAuth
// @Produce json
// @Param id path int true "Run ID"
// @Success 200 {object} models.FeeDistributionRun
// @Failure 404 {object} apperrors.ErrorResponse
// @Router /admin/fee-distribution/runs/{id} [get]
func (h *FeeDistributionAdminHandler) GetRun(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	run, err := h.distributionService.GetRun(id)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.NewNotFoundError("Distribution run not found"))
		return
	}

	c.JSON(http.StatusOK, run)
}
