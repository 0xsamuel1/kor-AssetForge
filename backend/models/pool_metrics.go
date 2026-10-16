package models

import (
	"time"
)

// PoolMetrics represents computed analytics for a liquidity pool
type PoolMetrics struct {
	PoolID           uint      `json:"pool_id"`
	Volume24h        int64     `json:"volume_24h"` // in stroops of input asset(s)
	Volume7d         int64     `json:"volume_7d"`
	Volume30d        int64     `json:"volume_30d"`
	Fees24h          int64     `json:"fees_24h"` // in stroops of input asset(s)
	Fees7d           int64     `json:"fees_7d"`
	Fees30d          int64     `json:"fees_30d"`
	TVL              int64     `json:"tvl"` // Total Value Locked in stroops (ReserveA + ReserveB equivalent)
	APY              float64   `json:"apy"` // Annual Percentage Yield (percentage, e.g. 12.5 means 12.5%)
	ImpermanentLoss  float64   `json:"impermanent_loss"` // impermanent loss ratio (e.g. 0.05 means 5%)
	UpdatedAt        time.Time `json:"updated_at"`
}

// PoolComparison represents comparative data between two pools
type PoolComparison struct {
	PoolA        LiquidityPool `json:"pool_a"`
	PoolB        LiquidityPool `json:"pool_b"`
	MetricsA     PoolMetrics   `json:"metrics_a"`
	MetricsB     PoolMetrics   `json:"metrics_b"`
	APYDifference float64       `json:"apy_difference"` // MetricsA.APY - MetricsB.APY
	TVLDifference int64         `json:"tvl_difference"` // MetricsA.TVL - MetricsB.TVL
}
