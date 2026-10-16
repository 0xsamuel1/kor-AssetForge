package services

import (
	"math"
	"time"

	"github.com/yourusername/kor-assetforge/models"
	"gorm.io/gorm"
)

type PoolAnalyticsService struct {
	db *gorm.DB
}

// NewPoolAnalyticsService creates a new pool analytics service
func NewPoolAnalyticsService(db *gorm.DB) *PoolAnalyticsService {
	return &PoolAnalyticsService{db: db}
}

// CalculatePoolMetrics calculates the APY, TVL, volume, and fees for a pool
func (s *PoolAnalyticsService) CalculatePoolMetrics(poolID uint) (*models.PoolMetrics, error) {
	var pool models.LiquidityPool
	if err := s.db.First(&pool, poolID).Error; err != nil {
		return nil, err
	}

	now := time.Now()

	// Compute volumes and fees
	vol24h, fee24h, err := s.getVolumeAndFees(poolID, now.Add(-24*time.Hour))
	if err != nil {
		return nil, err
	}
	vol7d, fee7d, err := s.getVolumeAndFees(poolID, now.Add(-7*24*time.Hour))
	if err != nil {
		return nil, err
	}
	vol30d, fee30d, err := s.getVolumeAndFees(poolID, now.Add(-30*24*time.Hour))
	if err != nil {
		return nil, err
	}

	// TVL: 2 * ReserveB in terms of Asset B, or ReserveA + ReserveB if they are 1:1.
	// For standard metric calculations, we use 2 * ReserveB
	tvl := pool.ReserveA + pool.ReserveB
	if pool.ReserveB > 0 {
		tvl = pool.ReserveB * 2
	}

	// APY = (annualized fees / TVL) * 100
	var apy float64
	if tvl > 0 {
		// Annualize 7d fees
		annualizedFees := float64(fee7d) * 52.14
		apy = (annualizedFees / float64(tvl)) * 100.0
	}

	// Average impermanent loss across all positions
	var avgIL float64
	var positions []models.LiquidityPosition
	if err := s.db.Where("pool_id = ?", poolID).Find(&positions).Error; err == nil && len(positions) > 0 {
		var sumIL float64
		var count int
		for _, pos := range positions {
			il, err := s.CalculatePositionImpermanentLoss(pos.ID)
			if err == nil {
				sumIL += il
				count++
			}
		}
		if count > 0 {
			avgIL = sumIL / float64(count)
		}
	}

	return &models.PoolMetrics{
		PoolID:          poolID,
		Volume24h:       vol24h,
		Volume7d:        vol7d,
		Volume30d:       vol30d,
		Fees24h:         fee24h,
		Fees7d:          fee7d,
		Fees30d:         fee30d,
		TVL:             tvl,
		APY:             apy,
		ImpermanentLoss: avgIL,
		UpdatedAt:       now,
	}, nil
}

// CalculatePositionImpermanentLoss calculates impermanent loss for a specific position
func (s *PoolAnalyticsService) CalculatePositionImpermanentLoss(positionID uint) (float64, error) {
	var pos models.LiquidityPosition
	if err := s.db.Preload("Pool").First(&pos, positionID).Error; err != nil {
		return 0, err
	}

	if pos.DepositedA == 0 || pos.DepositedB == 0 || pos.Pool.ReserveA == 0 || pos.Pool.ReserveB == 0 {
		return 0, nil
	}

	// Initial price ratio: DepositedB / DepositedA
	initialPriceRatio := float64(pos.DepositedB) / float64(pos.DepositedA)

	// Current price ratio: ReserveB / ReserveA
	currentPriceRatio := float64(pos.Pool.ReserveB) / float64(pos.Pool.ReserveA)

	// Price ratio change factor k
	k := currentPriceRatio / initialPriceRatio

	// Impermanent Loss formula: (2 * sqrt(k)) / (1 + k) - 1
	il := (2.0 * math.Sqrt(k)) / (1.0 + k) - 1.0

	// We return the absolute loss or negative percentage
	return math.Abs(il), nil
}

// ComparePools compares two liquidity pools and calculates differences
func (s *PoolAnalyticsService) ComparePools(poolAID, poolBID uint) (*models.PoolComparison, error) {
	var poolA, poolB models.LiquidityPool
	if err := s.db.First(&poolA, poolAID).Error; err != nil {
		return nil, err
	}
	if err := s.db.First(&poolB, poolBID).Error; err != nil {
		return nil, err
	}

	metricsA, err := s.CalculatePoolMetrics(poolAID)
	if err != nil {
		return nil, err
	}

	metricsB, err := s.CalculatePoolMetrics(poolBID)
	if err != nil {
		return nil, err
	}

	return &models.PoolComparison{
		PoolA:         poolA,
		PoolB:         poolB,
		MetricsA:      *metricsA,
		MetricsB:      *metricsB,
		APYDifference: metricsA.APY - metricsB.APY,
		TVLDifference: metricsA.TVL - metricsB.TVL,
	}, nil
}

func (s *PoolAnalyticsService) getVolumeAndFees(poolID uint, since time.Time) (int64, int64, error) {
	var result struct {
		Volume int64 `gorm:"column:volume"`
		Fees   int64 `gorm:"column:fees"`
	}

	err := s.db.Model(&models.PoolSwap{}).
		Where("pool_id = ? AND created_at >= ?", poolID, since).
		Select("COALESCE(SUM(input_amount), 0) as volume, COALESCE(SUM(fee_amount), 0) as fees").
		Scan(&result).Error

	return result.Volume, result.Fees, err
}
