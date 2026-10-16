package models

import "time"

type FeeConfiguration struct {
	ID            uint              `gorm:"primaryKey" json:"id"`
	AssetType     string            `gorm:"not null;uniqueIndex" json:"asset_type"`
	BaseFeeBps    int16             `gorm:"default:50" json:"base_fee_bps"`
	Description   string            `gorm:"type:text" json:"description"`
	Active        bool              `gorm:"default:true" json:"active"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	DiscountTiers []FeeDiscountTier `gorm:"foreignKey:FeeConfigID" json:"discount_tiers,omitempty"`
}

type FeeDiscountTier struct {
	ID               uint             `gorm:"primaryKey" json:"id"`
	FeeConfigID      uint             `gorm:"not null;index" json:"fee_config_id"`
	FeeConfig        FeeConfiguration `gorm:"foreignKey:FeeConfigID" json:"fee_config,omitempty"`
	TierName         string           `gorm:"not null" json:"tier_name"`
	MinVolumeStroops int64            `gorm:"type:numeric" json:"min_volume_stroops"`
	MaxVolumeStroops int64            `gorm:"type:numeric" json:"max_volume_stroops"`
	DiscountBps      int16            `gorm:"default:0" json:"discount_bps"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

type FeeTransaction struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	AssetID            uint      `gorm:"not null;index" json:"asset_id"`
	Asset              Asset     `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
	UserAddress        string    `gorm:"not null;index" json:"user_address"`
	TransactionHash    string    `gorm:"index" json:"transaction_hash"`
	TransactionAmount  int64     `gorm:"not null" json:"transaction_amount"`
	AssetType          string    `gorm:"not null" json:"asset_type"`
	BaseFeeBps         int16     `gorm:"not null" json:"base_fee_bps"`
	AppliedDiscountBps int16     `gorm:"default:0" json:"applied_discount_bps"`
	TotalFeeStroops    int64     `gorm:"not null" json:"total_fee_stroops"`
	Status             string    `gorm:"default:'completed'" json:"status"`
	CreatedAt          time.Time `json:"created_at"`
}

type FeeReport struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	PeriodStart       time.Time `gorm:"not null;index" json:"period_start"`
	PeriodEnd         time.Time `gorm:"not null;index" json:"period_end"`
	TotalVolume       int64     `gorm:"default:0" json:"total_volume"`
	TotalFeeCollected int64     `gorm:"default:0" json:"total_fees_collected"`
	TransactionCount  int64     `gorm:"default:0" json:"transaction_count"`
	GeneratedAt       time.Time `json:"generated_at"`
}
