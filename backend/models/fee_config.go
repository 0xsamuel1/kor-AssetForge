package models

import (
	"time"

	"gorm.io/gorm"
)

// FeeType distinguishes the transaction type that a fee rule applies to.
type FeeType string

const (
	FeeTypeMarketplace FeeType = "marketplace"
	FeeTypeTransfer    FeeType = "transfer"
	FeeTypeStaking     FeeType = "staking"
	FeeTypeLiquidity   FeeType = "liquidity"
)

// FeeConfig stores a dynamic, admin-configurable fee rule. A rule is active
// when Active is true. Multiple rules for the same FeeType can coexist to
// implement tiered pricing by volume threshold.
type FeeConfig struct {
	ID                    uint           `gorm:"primaryKey" json:"id"`
	Name                  string         `gorm:"not null;uniqueIndex" json:"name"`
	FeeType               FeeType        `gorm:"not null;index" json:"fee_type"`
	BaseBasisPoints       int            `gorm:"not null" json:"base_basis_points"`        // Default fee in bps (1 bps = 0.01%)
	MinBasisPoints        int            `gorm:"not null;default:0" json:"min_basis_points"` // Floor after any discount
	MaxBasisPoints        int            `gorm:"not null" json:"max_basis_points"`          // Ceiling (sanity guard)
	VolumeTiers           string         `gorm:"type:text;not null;default:'[]'" json:"volume_tiers"` // JSON []VolumeTier
	Active                bool           `gorm:"not null;default:true;index" json:"active"`
	EffectiveFrom         time.Time      `gorm:"not null" json:"effective_from"`
	EffectiveUntil        *time.Time     `json:"effective_until,omitempty"`
	CreatedByAdminID      uint           `gorm:"not null" json:"created_by_admin_id"`
	UpdatedByAdminID      uint           `gorm:"default:0" json:"updated_by_admin_id"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"-"`
}

// VolumeTier defines a discount that applies when a user's 30-day volume
// (in stroops) exceeds the threshold.
type VolumeTier struct {
	MinVolumeStroops int64   `json:"min_volume_stroops"`
	DiscountBps      int     `json:"discount_bps"`  // Reduction from base, in basis points
}

// FeeAuditLog records every change made to a FeeConfig for compliance.
type FeeAuditLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	FeeConfigID  uint      `gorm:"not null;index" json:"fee_config_id"`
	AdminID      uint      `gorm:"not null;index" json:"admin_id"`
	Action       string    `gorm:"not null" json:"action"` // "created", "updated", "deactivated"
	PreviousJSON string    `gorm:"type:text" json:"previous_json,omitempty"`
	NewJSON      string    `gorm:"type:text" json:"new_json,omitempty"`
	Reason       string    `gorm:"type:text" json:"reason,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// EffectiveFee is a transient, non-persisted result returned from the
// fee service to callers that need to know the computed fee for a
// given transaction amount and user volume.
type EffectiveFee struct {
	FeeConfigID     uint    `json:"fee_config_id"`
	FeeType         FeeType `json:"fee_type"`
	BaseBasisPoints int     `json:"base_basis_points"`
	AppliedBps      int     `json:"applied_bps"`
	DiscountBps     int     `json:"discount_bps"`
	FeeAmount       int64   `json:"fee_amount"`        // in stroops
	NetAmount       int64   `json:"net_amount"`        // amount - fee, in stroops
	TierApplied     *VolumeTier `json:"tier_applied,omitempty"`
}
