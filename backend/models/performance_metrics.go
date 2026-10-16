package models

import (
	"time"

	"gorm.io/gorm"
)

// AssetDividend records an income/dividend distribution paid out for an asset.
// It is the data source for dividend-yield calculations (#169).
type AssetDividend struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	AssetID    uint      `gorm:"not null;index" json:"asset_id"`
	Asset      Asset     `gorm:"foreignKey:AssetID" json:"-"`
	AmountUSD  float64   `gorm:"not null" json:"amount_usd"`
	Currency   string    `gorm:"not null;default:'USD'" json:"currency"`
	Note       string    `gorm:"type:text" json:"note,omitempty"`
	PaidAt     time.Time `gorm:"not null;index" json:"paid_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// PerformanceMetric is a computed snapshot of an asset's performance indicators
// over a date range. Snapshots are produced on demand and by the daily
// background job, providing historical performance tracking (#169).
type PerformanceMetric struct {
	ID      uint `gorm:"primaryKey" json:"id"`
	AssetID uint `gorm:"not null;index" json:"asset_id"`
	Asset   Asset `gorm:"foreignKey:AssetID" json:"-"`

	PeriodStart time.Time `gorm:"not null" json:"period_start"`
	PeriodEnd   time.Time `gorm:"not null" json:"period_end"`

	InitialValuationUSD float64 `json:"initial_valuation_usd"`
	CurrentValuationUSD float64 `json:"current_valuation_usd"`

	// ROI is the total return over the period as a ratio (0.10 = +10%).
	ROI float64 `json:"roi"`
	// AppreciationRate is the annualized price appreciation as a ratio.
	AppreciationRate float64 `json:"appreciation_rate"`
	// DividendYield is annualized dividend income divided by current valuation.
	DividendYield float64 `json:"dividend_yield"`
	// AnnualizedReturn combines appreciation and dividend yield.
	AnnualizedReturn float64 `json:"annualized_return"`
	// Volatility is the standard deviation of period-over-period returns.
	Volatility float64 `json:"volatility"`
	// TotalDividendsUSD is the dividend income paid during the period.
	TotalDividendsUSD float64 `json:"total_dividends_usd"`
	// BenchmarkROI is the platform-wide average ROI over the same period.
	BenchmarkROI float64 `json:"benchmark_roi"`
	// ExcessReturn is ROI minus BenchmarkROI (alpha).
	ExcessReturn float64 `json:"excess_return"`

	ComputedAt time.Time      `gorm:"index" json:"computed_at"`
	CreatedAt  time.Time      `json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}
