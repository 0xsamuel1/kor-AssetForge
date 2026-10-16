package handlers

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/kor-assetforge/apperrors"
	"github.com/yourusername/kor-assetforge/models"
	"github.com/yourusername/kor-assetforge/services"
	"github.com/yourusername/kor-assetforge/utils"
	"gorm.io/gorm"
)

// LiquidityHandler handles liquidity pool HTTP requests
type LiquidityHandler struct {
	db *gorm.DB
}

// NewLiquidityHandler creates a new LiquidityHandler
func NewLiquidityHandler(db *gorm.DB) *LiquidityHandler {
	return &LiquidityHandler{db: db}
}

type createPoolRequest struct {
	AssetAID       uint   `json:"asset_a_id" binding:"required,gt=0"`
	AssetBID       uint   `json:"asset_b_id" binding:"required,gt=0"`
	CreatorAddress string `json:"creator_address" binding:"required"`
	FeeBasisPoints int    `json:"fee_basis_points" binding:"omitempty,min=1,max=10000"`
}

type addLiquidityRequest struct {
	PoolID          uint   `json:"pool_id" binding:"required,gt=0"`
	ProviderAddress string `json:"provider_address" binding:"required"`
	AmountA         int64  `json:"amount_a" binding:"required,gt=0"`
	AmountB         int64  `json:"amount_b" binding:"required,gt=0"`
}

type removeLiquidityRequest struct {
	PoolID          uint   `json:"pool_id" binding:"required,gt=0"`
	ProviderAddress string `json:"provider_address" binding:"required"`
	LPTokens        int64  `json:"lp_tokens" binding:"required,gt=0"`
}

type swapRequest struct {
	PoolID        uint   `json:"pool_id" binding:"required,gt=0"`
	TraderAddress string `json:"trader_address" binding:"required"`
	InputAssetID  uint   `json:"input_asset_id" binding:"required,gt=0"`
	InputAmount   int64  `json:"input_amount" binding:"required,gt=0"`
	MinOutputAmount int64 `json:"min_output_amount" binding:"omitempty,gt=0"`
}

// CreatePool creates a new AMM liquidity pool
// @Summary Create liquidity pool
// @Description Create a new constant-product AMM pool for two assets
// @Tags liquidity
// @Accept json
// @Produce json
// @Param body body createPoolRequest true "Pool details"
// @Success 201 {object} models.LiquidityPool
// @Router /liquidity/pools [post]
func (h *LiquidityHandler) CreatePool(c *gin.Context) {
	var req createPoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.AssetAID == req.AssetBID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Pool assets must be different"})
		return
	}

	// Canonical ordering to avoid duplicate pools
	assetAID, assetBID := req.AssetAID, req.AssetBID
	if assetAID > assetBID {
		assetAID, assetBID = assetBID, assetAID
	}

	var existing models.LiquidityPool
	if err := h.db.Where("asset_a_id = ? AND asset_b_id = ?", assetAID, assetBID).
		First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Pool already exists for this asset pair", "pool": existing})
		return
	}

	feeBps := req.FeeBasisPoints
	if feeBps == 0 {
		feeBps = 30
	}

	pool := models.LiquidityPool{
		AssetAID:       assetAID,
		AssetBID:       assetBID,
		FeeBasisPoints: feeBps,
		CreatorAddress: req.CreatorAddress,
		Active:         true,
	}
	if err := h.db.Create(&pool).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.Wrap(err, apperrors.CodeDatabaseError, "Failed to create pool", http.StatusInternalServerError))
		return
	}

	c.JSON(http.StatusCreated, pool)
}

// ListPools returns all active liquidity pools
// @Summary List pools
// @Description Get all active AMM liquidity pools with optional asset filter
// @Tags liquidity
// @Param asset_id query int false "Filter by asset ID"
// @Success 200 {object} utils.Pagination
// @Router /liquidity/pools [get]
func (h *LiquidityHandler) ListPools(c *gin.Context) {
	type query struct {
		AssetID uint `form:"asset_id"`
		Page    int  `form:"page,default=1"`
		Limit   int  `form:"limit,default=20"`
	}
	var q query
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit < 1 || q.Limit > 100 {
		q.Limit = 20
	}

	db := h.db.Model(&models.LiquidityPool{}).Where("active = true")
	if q.AssetID != 0 {
		db = db.Where("asset_a_id = ? OR asset_b_id = ?", q.AssetID, q.AssetID)
	}

	var pools []models.LiquidityPool
	var total int64
	paginationRes, err := utils.Paginate(db, c, q.Page, q.Limit, &total, &pools)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch pools"})
		return
	}
	c.JSON(http.StatusOK, paginationRes)
}

// GetPool returns a specific pool
// @Summary Get pool
// @Description Get liquidity pool details
// @Tags liquidity
// @Param id path int true "Pool ID"
// @Success 200 {object} models.LiquidityPool
// @Router /liquidity/pools/{id} [get]
func (h *LiquidityHandler) GetPool(c *gin.Context) {
	var uri struct {
		ID uint `uri:"id" binding:"required,gt=0"`
	}
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid pool ID"})
		return
	}

	var pool models.LiquidityPool
	if err := h.db.Preload("AssetA").Preload("AssetB").First(&pool, uri.ID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Pool not found"})
		return
	}

	c.JSON(http.StatusOK, pool)
}

// AddLiquidity deposits tokens into a pool and issues LP tokens
// @Summary Add liquidity
// @Description Deposit asset A and B into a pool, receive LP tokens
// @Tags liquidity
// @Accept json
// @Produce json
// @Param body body addLiquidityRequest true "Deposit details"
// @Success 200 {object} models.LiquidityPosition
// @Router /liquidity/add [post]
func (h *LiquidityHandler) AddLiquidity(c *gin.Context) {
	var req addLiquidityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var pool models.LiquidityPool
	if err := h.db.First(&pool, req.PoolID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Pool not found"})
		return
	}

	if !pool.Active {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Pool is inactive"})
		return
	}

	// Calculate LP tokens to issue using constant product
	var lpTokensToIssue int64
	if pool.TotalLPTokens == 0 {
		// Initial liquidity: geometric mean of deposits
		lpTokensToIssue = int64(math.Sqrt(float64(req.AmountA) * float64(req.AmountB)))
	} else {
		// Proportional to current reserves
		lpA := req.AmountA * pool.TotalLPTokens / pool.ReserveA
		lpB := req.AmountB * pool.TotalLPTokens / pool.ReserveB
		lpTokensToIssue = lpA
		if lpB < lpA {
			lpTokensToIssue = lpB
		}
	}

	if lpTokensToIssue <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Deposit too small to issue LP tokens"})
		return
	}

	var position models.LiquidityPosition
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		pool.ReserveA += req.AmountA
		pool.ReserveB += req.AmountB
		pool.TotalLPTokens += lpTokensToIssue
		if err := tx.Save(&pool).Error; err != nil {
			return err
		}

		// Upsert LP position
		err := tx.Where("pool_id = ? AND provider_address = ?", pool.ID, req.ProviderAddress).
			First(&position).Error
		if err == gorm.ErrRecordNotFound {
			position = models.LiquidityPosition{
				PoolID:          pool.ID,
				ProviderAddress: req.ProviderAddress,
			}
		} else if err != nil {
			return err
		}
		position.LPTokens += lpTokensToIssue
		position.DepositedA += req.AmountA
		position.DepositedB += req.AmountB

		if position.ID == 0 {
			return tx.Create(&position).Error
		}
		return tx.Save(&position).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add liquidity"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "Liquidity added successfully",
		"position":        position,
		"lp_tokens_issued": lpTokensToIssue,
	})
}

// RemoveLiquidity withdraws tokens from a pool by burning LP tokens
// @Summary Remove liquidity
// @Description Burn LP tokens and withdraw proportional asset A and B
// @Tags liquidity
// @Accept json
// @Produce json
// @Param body body removeLiquidityRequest true "Removal details"
// @Success 200 {object} map[string]interface{}
// @Router /liquidity/remove [post]
func (h *LiquidityHandler) RemoveLiquidity(c *gin.Context) {
	var req removeLiquidityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var pool models.LiquidityPool
	if err := h.db.First(&pool, req.PoolID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Pool not found"})
		return
	}

	var position models.LiquidityPosition
	if err := h.db.Where("pool_id = ? AND provider_address = ?", req.PoolID, req.ProviderAddress).
		First(&position).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Liquidity position not found"})
		return
	}

	if req.LPTokens > position.LPTokens {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Insufficient LP tokens"})
		return
	}

	if pool.TotalLPTokens == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Pool has no liquidity"})
		return
	}

	// Proportional withdrawal
	withdrawA := req.LPTokens * pool.ReserveA / pool.TotalLPTokens
	withdrawB := req.LPTokens * pool.ReserveB / pool.TotalLPTokens

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		pool.ReserveA -= withdrawA
		pool.ReserveB -= withdrawB
		pool.TotalLPTokens -= req.LPTokens
		if pool.ReserveA < 0 {
			pool.ReserveA = 0
		}
		if pool.ReserveB < 0 {
			pool.ReserveB = 0
		}
		if err := tx.Save(&pool).Error; err != nil {
			return err
		}
		position.LPTokens -= req.LPTokens
		return tx.Save(&position).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove liquidity"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Liquidity removed successfully",
		"amount_a":   withdrawA,
		"amount_b":   withdrawB,
		"position":   position,
	})
}

// Swap executes a token swap through the AMM pool
// @Summary Swap tokens
// @Description Swap one asset for another through a liquidity pool using constant product formula
// @Tags liquidity
// @Accept json
// @Produce json
// @Param body body swapRequest true "Swap details"
// @Success 200 {object} models.PoolSwap
// @Router /liquidity/swap [post]
func (h *LiquidityHandler) Swap(c *gin.Context) {
	var req swapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var pool models.LiquidityPool
	if err := h.db.First(&pool, req.PoolID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Pool not found"})
		return
	}

	if !pool.Active {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Pool is inactive"})
		return
	}

	// Determine input/output reserves
	var reserveIn, reserveOut int64
	var outputAssetID uint
	if req.InputAssetID == pool.AssetAID {
		reserveIn = pool.ReserveA
		reserveOut = pool.ReserveB
		outputAssetID = pool.AssetBID
	} else if req.InputAssetID == pool.AssetBID {
		reserveIn = pool.ReserveB
		reserveOut = pool.ReserveA
		outputAssetID = pool.AssetAID
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input asset is not in this pool"})
		return
	}

	if reserveIn == 0 || reserveOut == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Pool has insufficient liquidity"})
		return
	}

	// Constant product formula: dx * (1 - fee) * reserveOut / (reserveIn + dx * (1 - fee))
	feeBps := int64(pool.FeeBasisPoints)
	amountInAfterFee := req.InputAmount * (10000 - feeBps) / 10000
	feeAmount := req.InputAmount - amountInAfterFee

	outputAmount := amountInAfterFee * reserveOut / (reserveIn + amountInAfterFee)

	if outputAmount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Swap output too small"})
		return
	}

	if req.MinOutputAmount > 0 && outputAmount < req.MinOutputAmount {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Slippage too high: output below minimum", "output": outputAmount, "minimum": req.MinOutputAmount})
		return
	}

	// Price impact in bps: (outputAmount / (reserveOut - outputAmount) - reserveIn / (reserveOut - reserveOut)) * 10000
	priceImpactBps := int(outputAmount * 10000 / reserveOut)

	var swap models.PoolSwap
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if req.InputAssetID == pool.AssetAID {
			pool.ReserveA += req.InputAmount
			pool.ReserveB -= outputAmount
		} else {
			pool.ReserveB += req.InputAmount
			pool.ReserveA -= outputAmount
		}
		if err := tx.Save(&pool).Error; err != nil {
			return err
		}

		// Distribute fee credit to active LP positions
		h.distributeFeeToProviders(tx, pool.ID, feeAmount)

		swap = models.PoolSwap{
			PoolID:         req.PoolID,
			TraderAddress:  req.TraderAddress,
			InputAssetID:   req.InputAssetID,
			OutputAssetID:  outputAssetID,
			InputAmount:    req.InputAmount,
			OutputAmount:   outputAmount,
			FeeAmount:      feeAmount,
			PriceImpactBps: priceImpactBps,
			CreatedAt:      time.Now(),
		}
		return tx.Create(&swap).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to execute swap"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "Swap executed successfully",
		"swap":           swap,
		"output_amount":  outputAmount,
		"fee_amount":     feeAmount,
		"price_impact_bps": priceImpactBps,
	})
}

// distributeFeeToProviders credits fee proportionally to LP positions
func (h *LiquidityHandler) distributeFeeToProviders(tx *gorm.DB, poolID uint, feeAmount int64) {
	var positions []models.LiquidityPosition
	if err := tx.Where("pool_id = ? AND lp_tokens > 0", poolID).Find(&positions).Error; err != nil {
		return
	}

	var totalLP int64
	for _, p := range positions {
		totalLP += p.LPTokens
	}
	if totalLP == 0 {
		return
	}

	for i := range positions {
		share := positions[i].LPTokens * feeAmount / totalLP
		positions[i].FeesEarned += share
		tx.Save(&positions[i])
	}
}

// GetLPPositions returns LP positions for a provider
// @Summary LP positions
// @Description Get all liquidity positions for a provider address
// @Tags liquidity
// @Param address query string true "Provider address"
// @Success 200 {array} models.LiquidityPosition
// @Router /liquidity/positions [get]
func (h *LiquidityHandler) GetLPPositions(c *gin.Context) {
	type query struct {
		Address string `form:"address" binding:"required"`
		Page    int    `form:"page,default=1"`
		Limit   int    `form:"limit,default=20"`
	}
	var q query
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := h.db.Model(&models.LiquidityPosition{}).
		Preload("Pool").
		Where("provider_address = ? AND lp_tokens > 0", q.Address)

	var positions []models.LiquidityPosition
	var total int64
	paginationRes, err := utils.Paginate(db, c, q.Page, q.Limit, &total, &positions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch positions"})
		return
	}
	c.JSON(http.StatusOK, paginationRes)
}

// GetSwapHistory returns swap history for a pool or trader
// @Summary Swap history
// @Description Get swap history for a pool or trader address
// @Tags liquidity
// @Param pool_id query int false "Pool ID"
// @Param address query string false "Trader address"
// @Success 200 {object} utils.Pagination
// @Router /liquidity/swaps [get]
func (h *LiquidityHandler) GetSwapHistory(c *gin.Context) {
	type query struct {
		PoolID  uint   `form:"pool_id"`
		Address string `form:"address"`
		Page    int    `form:"page,default=1"`
		Limit   int    `form:"limit,default=20"`
	}
	var q query
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := h.db.Model(&models.PoolSwap{}).Order("created_at DESC")
	if q.PoolID != 0 {
		db = db.Where("pool_id = ?", q.PoolID)
	}
	if q.Address != "" {
		db = db.Where("trader_address = ?", q.Address)
	}

	var swaps []models.PoolSwap
	var total int64
	paginationRes, err := utils.Paginate(db, c, q.Page, q.Limit, &total, &swaps)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch swap history"})
		return
	}
	c.JSON(http.StatusOK, paginationRes)
}

// GetPoolAnalytics retrieves computed pool analytics/metrics
// @Summary Get pool analytics
// @Description Get computed analytics (APY, volume, TVL, impermanent loss) for a liquidity pool
// @Tags liquidity
// @Param id path int true "Pool ID"
// @Success 200 {object} models.PoolMetrics
// @Router /liquidity/pools/{id}/analytics [get]
func (h *LiquidityHandler) GetPoolAnalytics(c *gin.Context) {
	var uri struct {
		ID uint `uri:"id" binding:"required,gt=0"`
	}
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid pool ID"})
		return
	}

	analyticsService := services.NewPoolAnalyticsService(h.db)
	metrics, err := analyticsService.CalculatePoolMetrics(uri.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Pool not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// ComparePools compares two liquidity pools
// @Summary Compare pools
// @Description Compare APY, TVL, and metrics of two liquidity pools
// @Tags liquidity
// @Param pool_a query int true "Pool A ID"
// @Param pool_b query int true "Pool B ID"
// @Success 200 {object} models.PoolComparison
// @Router /liquidity/pools/compare [get]
func (h *LiquidityHandler) ComparePools(c *gin.Context) {
	poolAStr := c.Query("pool_a")
	poolBStr := c.Query("pool_b")

	if poolAStr == "" || poolBStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pool_a and pool_b query parameters are required"})
		return
	}

	var poolAID, poolBID uint
	if _, err := fmt.Sscanf(poolAStr, "%d", &poolAID); err != nil || poolAID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid pool_a ID"})
		return
	}
	if _, err := fmt.Sscanf(poolBStr, "%d", &poolBID); err != nil || poolBID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid pool_b ID"})
		return
	}

	analyticsService := services.NewPoolAnalyticsService(h.db)
	comparison, err := analyticsService.ComparePools(poolAID, poolBID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, comparison)
}
