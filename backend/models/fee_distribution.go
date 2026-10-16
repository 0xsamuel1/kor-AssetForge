package models

import "time"

// DistributionRecipientType identifies who a fee distribution allocation was paid to.
type DistributionRecipientType string

const (
	RecipientPlatform           DistributionRecipientType = "platform"
	RecipientLiquidityProvider  DistributionRecipientType = "liquidity_provider"
	RecipientTokenHolder        DistributionRecipientType = "token_holder"
)

// DistributionRunStatus tracks the lifecycle of a fee distribution run.
type DistributionRunStatus string

const (
	DistributionStatusPending   DistributionRunStatus = "pending"
	DistributionStatusCompleted DistributionRunStatus = "completed"
	DistributionStatusFailed    DistributionRunStatus = "failed"
)

// FeeDistributionRule defines how collected marketplace fees are split between
// the platform, liquidity providers, and token holders. Shares are expressed
// in basis points and must sum to 10000 (100%). Only one rule may be active
// at a time.
type FeeDistributionRule struct {
	ID                          uint      `gorm:"primaryKey" json:"id"`
	Name                        string    `gorm:"not null" json:"name"`
	PlatformShareBps            int       `gorm:"not null;default:0" json:"platform_share_bps"`
	LiquidityProvidersShareBps  int       `gorm:"not null;default:0" json:"liquidity_providers_share_bps"`
	TokenHoldersShareBps        int       `gorm:"not null;default:0" json:"token_holders_share_bps"`
	Active                      bool      `gorm:"default:true" json:"active"`
	CreatedAt                   time.Time `json:"created_at"`
	UpdatedAt                   time.Time `json:"updated_at"`
}

func (FeeDistributionRule) TableName() string {
	return "fee_distribution_rules"
}

// FeeDistributionRun records a single execution of the distribution process
// over a fee collection period, broken down by recipient category.
type FeeDistributionRun struct {
	ID                               uint                   `gorm:"primaryKey" json:"id"`
	RuleID                           uint                   `gorm:"not null;index" json:"rule_id"`
	Rule                             FeeDistributionRule    `gorm:"foreignKey:RuleID" json:"rule,omitempty"`
	PeriodStart                      time.Time              `gorm:"not null" json:"period_start"`
	PeriodEnd                        time.Time              `gorm:"not null" json:"period_end"`
	TotalFeesStroops                 int64                  `gorm:"not null;default:0" json:"total_fees_stroops"`
	PlatformAmountStroops            int64                  `gorm:"not null;default:0" json:"platform_amount_stroops"`
	LiquidityProvidersAmountStroops  int64                  `gorm:"not null;default:0" json:"liquidity_providers_amount_stroops"`
	TokenHoldersAmountStroops        int64                  `gorm:"not null;default:0" json:"token_holders_amount_stroops"`
	RecipientCount                  int                    `gorm:"not null;default:0" json:"recipient_count"`
	Status                          DistributionRunStatus  `gorm:"not null;default:'pending'" json:"status"`
	TriggeredBy                     string                 `gorm:"not null;default:'manual'" json:"triggered_by"`
	CreatedAt                       time.Time              `json:"created_at"`
	CompletedAt                     *time.Time             `json:"completed_at,omitempty"`
	Allocations                     []FeeDistributionAllocation `gorm:"foreignKey:RunID" json:"allocations,omitempty"`
}

func (FeeDistributionRun) TableName() string {
	return "fee_distribution_runs"
}

// FeeDistributionAllocation records the amount paid to a single recipient as
// part of a distribution run.
type FeeDistributionAllocation struct {
	ID               uint                      `gorm:"primaryKey" json:"id"`
	RunID            uint                      `gorm:"not null;index" json:"run_id"`
	RecipientType    DistributionRecipientType `gorm:"not null" json:"recipient_type"`
	RecipientAddress string                    `gorm:"not null;index" json:"recipient_address"`
	AmountStroops    int64                     `gorm:"not null" json:"amount_stroops"`
	TxHash           string                    `gorm:"index" json:"tx_hash,omitempty"`
	Status           string                    `gorm:"not null;default:'pending'" json:"status"`
	CreatedAt        time.Time                 `json:"created_at"`
}

func (FeeDistributionAllocation) TableName() string {
	return "fee_distribution_allocations"
}
